# Code Quality Audit — agent-lsp

**Date:** 2026-04-10
**Inspector version:** 0.8.0
**Repo root:** `/path/to/agent-lsp`

---

## Summary

- **Audited:** Full codebase — `cmd/agent-lsp/`, `internal/lsp/`, `internal/tools/`, `internal/session/`, `internal/resources/`, `internal/config/`, `internal/logging/`, `internal/extensions/`, `internal/types/`, `internal/uri/`
- **LSP Tier:** 1B (get_references) — Tier 1A (get_change_impact) was denied by the sandbox. All symbol-level findings verified via `mcp__lsp__get_references` or `mcp__lsp__get_info_on_location`.
- **Layer map:**
  ```
  cmd/agent-lsp/        ← MCP server lifecycle, tool registration, signal handling
  internal/tools/       ← MCP tool handlers; imports lsp/, session/, types/
  internal/resources/   ← MCP resource handlers; imports lsp/, types/
  internal/session/     ← Speculative execution; imports lsp/, types/, logging/
  internal/lsp/         ← LSP subprocess client; imports types/, logging/, uri/
  internal/config/      ← Argument parsing and inference; no internal imports
  internal/extensions/  ← Extension registry; imports types/, logging/
  internal/logging/     ← Log bridge; no internal imports
  internal/types/       ← Shared value types; no internal imports
  internal/uri/         ← URI utilities; no internal imports

  Boundary rules (from docs/architecture.md):
    internal/lsp/ must not import internal/tools/ or internal/session/
    internal/tools/ and internal/resources/ must not import each other
    internal/extensions/ imports only from internal/types/
  ```
- **Highest severity:** error
- **Signal:** The codebase is well-structured with clear layering and strong test coverage. Three findings of note: `HandleGetDiagnostics` skips path-traversal validation present in every other handler; `StartForLanguage` drops the caller's context when shutting down the old server; and three separate language-ID-from-extension functions in `internal/lsp/` with inconsistent coverage create a latent routing gap. No panics in goroutines, no `init()` side effects beyond env-var read, no dead exported symbols found.

---

## internal/lsp/

### Finding 1

**coverage_gap** · error · confidence: high
`internal/tools/analysis.go:24` · [LSP hover: confirmed]

**What:** `HandleGetDiagnostics` calls `CreateFileURI(filePath)` directly without first calling `ValidateFilePath`. Every other tool handler in `internal/tools/` that takes a `file_path` argument calls `ValidateFilePath` (either via `WithDocument` or explicitly). The omission means a caller can supply a path outside the workspace root — such as `../../etc/passwd` — and the handler will construct a valid `file://` URI and pass it to `ReopenDocument`, which will then read the file from disk. The path-traversal guard in `WithDocument` is intentionally bypassed.

**Fix:** Add a `ValidateFilePath(filePath, client.RootDir())` call at the top of the `filePath != ""` branch in `HandleGetDiagnostics`, consistent with every other handler.

---

### Finding 2

**context_propagation** · warning · confidence: high
`internal/lsp/manager.go:190` · [LSP findReferences: 7 references]

**What:** `StartForLanguage` accepts `ctx context.Context` from its caller but uses `context.Background()` when shutting down the old client before restarting it. If the caller's context is already cancelled (e.g., a timed-out `restart_lsp_server` call), the shutdown attempt proceeds with an unbounded context. The `Initialize` call on the new client correctly uses `ctx`, making the pattern inconsistent within the same function.

```go
// line 190: ignores ctx, uses Background()
_ = e.client.Shutdown(context.Background())
```

**Fix:** Pass `ctx` to `e.client.Shutdown(ctx)`. The old-client shutdown on restart is not a fire-and-forget; it should respect the caller's deadline.

---

### Finding 3

**duplicate_semantics** · warning · confidence: high
`internal/lsp/manager.go:257` and `internal/lsp/client.go:938`

`[LSP findReferences for LanguageIDFromPath: 4 references (3 non-test, 1 test)]`
`[LSP findReferences for languageIDFromURI: called only inside client.go]`

**What:** Three separate functions in `internal/lsp/` perform extension-to-language-ID mapping:

| Function | Location | Visibility | Coverage |
|---|---|---|---|
| `inferLanguageID` | `manager.go:222` | unexported | ~10 ext from ServerEntry.Extensions[0] |
| `LanguageIDFromPath` | `manager.go:257` | exported | ~10 ext via `filepath.Ext` |
| `languageIDFromURI` | `client.go:938` | unexported | ~13 ext (adds `c`, `cpp`, `java`) |

`LanguageIDFromPath` is used by `internal/tools/change_impact.go` for language routing but lacks `c`, `cpp`, `java`, `ruby` — languages the server supports. If `HandleGetChangeImpact` encounters a `.c` or `.java` file, `LanguageIDFromPath` returns `"plaintext"`, producing an incorrect `languageID` for `WithDocument`. The comment at line 256 says "Canonical implementation shared by internal/lsp and internal/tools (E5 deduplication)" but it covers fewer languages than the private function in the same package.

**Fix:** Consolidate to a single function and extend it to cover all 30 CI-verified languages consistently. `languageIDFromURI` is the most complete — promote or align it.

---

### Finding 4

**scope_analysis** · warning · confidence: high
`internal/lsp/client.go` (2317 lines)

**What:** `LSPClient` in `client.go` combines seven distinct responsibilities in one file:
1. Subprocess lifecycle (`start`, `Initialize`, `Shutdown`, `Restart`)
2. JSON-RPC request/response correlation (`sendRequest`, `sendNotification`, `rejectPending`, `writeRaw`)
3. Server-initiated request dispatch (`dispatch`, `handlePublishDiagnostics`, `handleProgress`, `workspace/applyEdit`, etc.)
4. Document tracking (`OpenDocument`, `CloseDocument`, `ReopenDocument`, `ReopenAllDocuments`)
5. Workspace folder management (`AddWorkspaceFolder`, `RemoveWorkspaceFolder`, `GetWorkspaceFolders`)
6. File watcher (`startWatcher`, `stopWatcher`, `addWatcherRoot`)
7. All 20+ LSP operation methods (`GetReferences`, `GetDefinition`, `GetSemanticTokens`, etc.)

The watcher goroutine (responsibility 6) and workspace folder management (responsibility 5) are independently testable and have clearly bounded interfaces. Peer files in `internal/lsp/` (`diagnostics.go`, `normalize.go`, `framing.go`) are each under 110 lines and handle single concerns. At 2317 lines, `client.go` is an outlier by roughly 20x.

**Fix:** Extract `startWatcher`/`stopWatcher`/`addWatcherRoot` and the `workspaceFolder` helpers into a `watcher.go` file. Extract the 20+ LSP operation methods into an `operations.go` file. The struct definition and lifecycle methods can remain in `client.go`.

---

## internal/tools/

### Finding 5

**coverage_gap** · warning · confidence: high
`internal/tools/change_impact.go:91`

**What:** In `HandleGetChangeImpact`, reference errors from `GetReferences` are silently discarded:

```go
locs, _ := WithDocument[[]types.Location](ctx, client, filePath, langID, func(fURI string) ([]types.Location, error) {
    return client.GetReferences(ctx, fURI, pos, false)
})
```

If `WithDocument` or `GetReferences` fails for any exported symbol (e.g., the file is unreadable, or the server times out), `locs` is `nil` and the symbol is silently excluded from the result. The caller receives a `summary` line that may report an inflated count of "0 test references" for that symbol without indicating the error. The outer `warnings` slice captures file-level errors but not symbol-level reference failures.

**Fix:** Capture the error and append to `warnings`: `if err != nil { warnings = append(warnings, fmt.Sprintf("warning: references for %s: %s", sym.Name, err)) }`.

---

### Finding 6

**error_wrapping** · warning · confidence: high
`internal/lsp/client.go:470` · [LSP hover: confirmed]

**What:** `writeRaw` returns `err` bare from `c.stdin.Write(...)`:

```go
func (c *LSPClient) writeRaw(body []byte) error {
    ...
    _, err := c.stdin.Write(EncodeMessage(body))
    return err          // line 470: no context
}
```

`writeRaw` is called from `sendRequest`, `sendNotification`, and `sendResponse`. When it fails, callers receive an opaque I/O error with no indication of which method or which field was being written. The established convention in this codebase is `fmt.Errorf("context: %w", err)` (visible throughout `Initialize`, `start`, and all exported methods).

**Fix:** `return fmt.Errorf("write to LSP stdin: %w", err)`.

---

### Finding 7

**error_wrapping** · warning · confidence: high
`internal/lsp/client.go:533` and `542` (in `sendNotification`) · [LSP hover: confirmed]

**What:** `sendNotification` has two bare `return err` paths — one from `json.Marshal` and one from `writeRaw`:

```go
func (c *LSPClient) sendNotification(method string, params interface{}) error {
    p, err := json.Marshal(params)
    if err != nil {
        return err          // line 533: no method name in error
    }
    ...
    return c.writeRaw(body) // line 542: already discussed above
}
```

When `json.Marshal` fails, the error gives no indication of which notification method was being sent.

**Fix:** `return fmt.Errorf("sendNotification %q marshal params: %w", method, err)`.

---

### Finding 8

**init_side_effects** · warning · confidence: high
`internal/logging/logging.go:54` · [LSP hover: confirmed]

**What:** The `init()` function in `internal/logging/` reads `os.Getenv("LOG_LEVEL")` and conditionally mutates `currentLevel`, a package-level global variable:

```go
func init() {
    if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
        if _, ok := logLevelPriority[envLevel]; ok {
            currentLevel = envLevel
        } else {
            initWarning = "..."
        }
    }
}
```

Reading an environment variable in `init()` couples test setup to module import order — any test that imports `internal/logging/` will inherit whatever `LOG_LEVEL` was set in the process environment. This is the standard Go pattern for such initialization, but it means tests that import this package can have their log level silently overridden. The severity is reduced because the side effect is deterministic and cannot fail.

**Fix:** Document the import-order behavior explicitly; or move the env-var read to a `ConfigureFromEnv()` function called from `cmd/agent-lsp/main.go`, leaving `init()` as a pure compile-time constant setup.

---

### Finding 9

**test_coverage** · warning · confidence: high
`internal/lsp/client.go` — `WaitForFileIndexed` · [LSP findReferences: multiple test references in lsp/ package only; no integration test for stabilization window]

**What:** `WaitForFileIndexed` is a complex function implementing a two-phase wait (first diagnostic notification, then 1500ms stability window) that is central to the correctness of `GetReferences`. It is exercised indirectly through integration tests but has no dedicated unit test that verifies the stabilization window behavior, the timeout path, or the context-cancellation path. `WaitForDiagnostics` in the same package has six unit tests in `diagnostics_test.go` that directly exercise equivalent logic.

**Fix:** Add unit tests for `WaitForFileIndexed` mirroring the structure of `diagnostics_test.go` — at minimum: timeout path (no notification arrives), context cancellation, and the stability window reset on repeated notifications.

---

### Finding 10

**test_coverage** · warning · confidence: high
`internal/tools/build.go` — `RunBuild` (exported) · [LSP findReferences: 2 references — `build.go:47` and `build_test.go:174`]

**What:** `RunBuild` is exported but has only one integration test call at `build_test.go:174` and one non-test caller (the `HandleRunBuild` wrapper). The test at line 174 exercises only the Go runner against real workspace state. The `parseBuildErrors` dispatch for TypeScript, Rust, Python, C#, Swift, Zig, and Kotlin has no unit tests — each language parser is exercised only by the real-build integration path.

**Fix:** Add table-driven unit tests for each `parseBuildErrors` case using synthetic build output, independent of a real compiler.

---

### Finding 11

**cross_field_consistency** · warning · confidence: high
`internal/session/types.go` — `SimulationSession` fields `Status` and `DirtyErr`

**What:** `SimulationSession.DirtyErr` is only meaningful when `Status == StatusDirty`, but there is no invariant that enforces this. `MarkDirty` correctly sets both fields atomically. However, `IsTerminal()` checks `StatusDirty` without inspecting `DirtyErr`, and callers can access `session.DirtyErr` without checking `IsDirty()` first. If the status is `StatusCreated` and a caller reads `session.DirtyErr`, they receive `nil` with no indication that the session is healthy — which is valid, but the inverse scenario (a non-nil `DirtyErr` with non-dirty status) is not structurally prevented.

**Fix:** Consider an accessor `DirtyError() (error, bool)` that returns `DirtyErr` only when `IsDirty()` is true, making the field dependency explicit.

---

## All Findings

| Severity | Confidence | Check Type | Finding | Location |
|----------|------------|------------|---------|----------|
| error | high | coverage_gap | `HandleGetDiagnostics` skips `ValidateFilePath`; path traversal not blocked | `internal/tools/analysis.go:24` |
| warning | high | context_propagation | `StartForLanguage` uses `context.Background()` for old-client shutdown despite receiving `ctx` | `internal/lsp/manager.go:190` |
| warning | high | duplicate_semantics | Three extension-to-language-ID functions with inconsistent language coverage | `internal/lsp/manager.go:257`, `client.go:938` |
| warning | high | scope_analysis | `client.go` combines 7 responsibilities across 2317 lines; peers are <110 lines | `internal/lsp/client.go` |
| warning | high | coverage_gap | Reference errors silently discarded per symbol in `HandleGetChangeImpact` | `internal/tools/change_impact.go:91` |
| warning | high | error_wrapping | `writeRaw` returns bare I/O error with no context | `internal/lsp/client.go:470` |
| warning | high | error_wrapping | `sendNotification` returns bare marshal error without method name | `internal/lsp/client.go:533` |
| warning | high | init_side_effects | `init()` reads `LOG_LEVEL` env and mutates package-level `currentLevel` | `internal/logging/logging.go:54` |
| warning | high | test_coverage | `WaitForFileIndexed` has no unit tests for stability window or timeout paths | `internal/lsp/client.go:1048` |
| warning | high | test_coverage | `RunBuild`/`parseBuildErrors` has no unit tests for 7 of 8 language parsers | `internal/tools/build.go:126` |
| warning | high | cross_field_consistency | `DirtyErr` field meaningful only when `Status==StatusDirty`; no accessor enforces this | `internal/session/types.go:125` |

---

## Checks Applied

All 14 checks from the taxonomy were applied:

| Check | Result |
|-------|--------|
| dead_symbol | No dead exported symbols found. `FindTestFiles`, `RunBuild`, `RunTests` all have non-test callers. `LanguageIDFromPath` has 3 non-test callers. |
| layer_violation | No violations. All imports follow the documented boundary rules. `internal/lsp/` does not import from `internal/tools/` or `internal/session/`. |
| scope_analysis | Finding 4 — `client.go` (2317 lines, 7 responsibilities) |
| coverage_gap | Findings 1 and 5 |
| silent_failure | Finding 5 (reference errors discarded with `_`). No other silent failures; `_ = Shutdown(...)` calls are documented best-effort shutdowns at boundaries. |
| duplicate_semantics | Finding 3 |
| cross_field_consistency | Finding 11 |
| test_coverage | Findings 9 and 10 |
| error_wrapping | Findings 6 and 7 |
| doc_drift | No drift found. `ClientResolver`, `SessionExecutor`, `HandleGetWorkspaceSymbols`, and `WaitForFileIndexed` docs match their signatures. [LSP hover: confirmed] |
| interface_saturation | No saturation found. `ClientResolver` has 4 methods; `SessionExecutor` has 2; `Extension` has 4. All within normal bounds. [LSP findReferences: confirmed] |
| panic_not_recovered | No unrecovered panics in goroutines. `readLoop` goroutine has `recover()` at line 242. `startWatcher` goroutine has `recover()` at line 1976. Signal handler in `main.go` is not a goroutine risk. |
| context_propagation | Finding 2. One instance in `manager.go:190`. The `context.Background()` in `client.go:333` (`workspace/applyEdit` handler) is documented as intentional (H4 pattern — readLoop has no per-request context). |
| init_side_effects | Finding 8. One `init()` in `internal/logging/logging.go` reads env var and mutates global. |

---

## Not Checked — Out of Scope

- `cross_repo_dead_symbol` — `--consumer-repos` flag was not provided; check not applicable.
- `skills/` directory (SKILL.md files) — prompt-only content, not Go code; no checks apply.
- `test/` integration test fixtures — third-party fixture code, not part of this codebase's authorship.

## Not Checked — Tooling Constraints

- `mcp__lsp__get_change_impact` — denied by sandbox. Fell back to Tier 1B (`get_references`) for all `dead_symbol` and `test_coverage` checks. All symbol-level findings include LSP findReferences annotation.
