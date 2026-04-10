# lsp-mcp-go Code Quality Audit — Inspection 2

## Summary

- **Audited:** `/Users/dayna.blackwell/workspace/code/LSP-MCP-GO` (full repo)
- **Layer map:**
  ```
  cmd/lsp-mcp-go → internal/tools, internal/resources, internal/session,
                    internal/extensions, internal/lsp, internal/logging, internal/types
  internal/tools  → internal/lsp, internal/types, internal/session
  internal/resources → internal/lsp, internal/types
  internal/session → internal/lsp, internal/types
  internal/extensions → internal/types
  internal/lsp → internal/types, internal/logging
  internal/logging → (stdlib only)
  extensions/haskell → internal/extensions, internal/types
  Boundary: internal/lsp must not import internal/tools or above.
  Boundary: internal/types must not import any internal/* package.
  ```
- **Highest severity:** error
- **Signal:** Eight findings across dead symbols, context propagation, duplicate semantics, doc drift, and silent failures. No architectural boundary violations. The most consequential issues are the context propagation gap in the session executor (any slow session operation ignores request cancellation) and two dead functions in the resources package that prevent per-file resource discovery from working in the running binary.

---

## Tooling Note

The built-in `LSP` tool was invoked for all symbol-level checks (findReferences, hover) and returned `No such tool available: LSP` each time. All symbol-level findings therefore use Grep as fallback and carry `confidence: reduced`. Grep misses aliased calls, interface dispatch, and dynamically constructed references; each finding notes this explicitly.

---

## internal/resources/resources.go

**dead_symbol** · error · confidence: reduced
`internal/resources/resources.go:217` and `internal/resources/resources.go:266`
[LSP unavailable — Grep fallback, reduced confidence]
What: `generateResourceList` (line 217) and `resourceTemplates` (line 266) are unexported functions defined in the `resources` package. Grep across all non-test `.go` files in the repo finds zero call sites outside of `internal/resources/resources_test.go`. The MCP server in `cmd/lsp-mcp-go/server.go` registers the three resource handlers directly via `server.AddResource` and never calls either function. As a result, MCP clients that call `resources/list` receive only the three statically registered URIs. The dynamic per-file entries that `generateResourceList` would produce (per-file `lsp-diagnostics://`, `lsp-hover://`, and `lsp-completions://` URIs keyed to open documents) are never surfaced. The URI templates that `resourceTemplates` would expose are also unreachable.
Fix: Wire `generateResourceList` into the MCP resources/list handler so that open-document-specific resource entries are visible to callers. Wire `resourceTemplates` into the resources/templates/list handler. If the static registrations in `server.go` are the intended permanent design, delete both functions and their test coverage.

---

## internal/extensions/registry.go

**dead_symbol** · warning · confidence: reduced
`internal/extensions/registry.go:76`
[LSP unavailable — Grep fallback, reduced confidence]
What: `ExtensionRegistry.Deactivate` is an exported method with no call sites in any non-test file. Grep across all production `.go` files finds only the definition; the single test call is in `registry_test.go`. The server never deactivates extensions at runtime — extensions are activated once at startup and persist for the process lifetime.
Fix: If runtime deactivation is not a planned feature, unexport or delete the method. If it is planned, add a comment marking it as reserved for future use so callers understand the intent.

---

## internal/session/executor.go

**context_propagation** · error · confidence: reduced
`internal/session/executor.go:21`
[LSP unavailable — Grep fallback, reduced confidence]
What: `SerializedExecutor.Acquire(ctx context.Context, s *SimulationSession) error` accepts a context but calls `e.mu.Lock()` unconditionally and ignores the context entirely. If the MCP request context is cancelled or times out while the mutex is held by a concurrent session (e.g., during a slow `Evaluate` call with its 8-second default timeout), callers of `ApplyEdit`, `Evaluate`, and `Discard` will block indefinitely waiting for the mutex — they will not respect the cancellation. All three callers pass the live tool-handler context, which carries the MCP client's request deadline.
Fix: Replace the bare `e.mu.Lock()` with a context-aware pattern. A clean approach is to replace `sync.Mutex` with a buffered channel semaphore (`make(chan struct{}, 1)`) which can be selected on alongside `ctx.Done()`:
```go
func (e *SerializedExecutor) Acquire(ctx context.Context, s *SimulationSession) error {
    select {
    case e.sem <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
func (e *SerializedExecutor) Release(s *SimulationSession) {
    <-e.sem
}
```

---

## internal/session/manager.go

**duplicate_semantics** · warning · confidence: reduced
`internal/session/manager.go:425`
[LSP unavailable — Grep fallback, reduced confidence]
What: `applyRangeEdit` (line 425) is self-described via its own comment as "adapted from `LSPClient.applyEditsToFile`" and implements the same line-splitting, character-clamping, and splice algorithm across ~45 lines. Any bug found and fixed in `applyEditsToFile` will not automatically be propagated to `applyRangeEdit`. The comment acknowledges the duplication but does not create a tracked obligation to keep them in sync.
Fix: Extract the single-edit core into an unexported helper in `internal/lsp` (or a shared utility) and have both functions delegate to it. Alternatively, document the intentional duplication with an explicit cross-reference comment in both files so that reviewers know to update both when the algorithm changes.

---

## internal/lsp/client.go

**doc_drift** · warning · confidence: reduced
`internal/lsp/client.go:481`
[LSP unavailable — Grep fallback, reduced confidence]
What: The doc comment block immediately above `RootDir()` at lines 481–482 reads:
```
// Initialize starts the LSP process and performs the LSP handshake.
// RootDir returns the workspace root directory set during Initialize.
```
The first line is the doc comment for `Initialize`, which appears three lines later at line 487 with the identical first line. The correct sole doc comment for `RootDir` is only the second line. The duplication was produced by a copy-paste when the function was inserted between the two `Initialize` comment/body blocks.
Fix: Replace lines 481–482 with only:
```
// RootDir returns the workspace root directory set during Initialize.
```

---

**silent_failure** · warning · confidence: reduced
`internal/lsp/client.go:275`
[LSP unavailable — Grep fallback, reduced confidence]
What: In the `"workspace/configuration"` branch of `dispatch`, the params are unmarshaled with `_ = json.Unmarshal(msg.Params, &p)`. If unmarshal fails (malformed params from the LSP server), `p.Items` stays zero-length and the response is `[]interface{}{}` — an empty array. The LSP 3.17 spec requires the `workspace/configuration` response array length to match the `items` array length in the request. Responding with `[]` when `items` has N entries causes the language server to receive null for all N requested configuration scopes. For most servers this falls back to defaults silently, but servers that gate behavior on their own configuration (e.g., gopls analysis enable/disable flags) may operate with wrong settings for the entire session.
Fix: Log the unmarshal error at debug level before constructing the nulls response. The fallback to an empty array is acceptable for robustness; making the failure observable is the actionable change.

---

**silent_failure** · warning · confidence: reduced
`internal/lsp/client.go:1412`
[LSP unavailable — Grep fallback, reduced confidence]
What: In `applyDocumentChanges`, the discriminator unmarshal `_ = json.Unmarshal(entry, &disc)` silently ignores errors. If an entry in the `documentChanges` array cannot be parsed, `disc.Kind` stays `""` and the entry falls into the `default` `TextDocumentEdit` branch unconditionally. A malformed entry will then reach `applyEditsToFile` with a potentially empty or mismatched URI, which either silently no-ops or attempts to edit the wrong file.
Fix: Check the error from `json.Unmarshal(entry, &disc)`. On failure, skip the entry with a debug-level log or propagate an error. A no-op skip is acceptable for protocol robustness; making the failure visible is the actionable change.

---

## internal/logging/logging.go

**init_side_effects** · warning · confidence: reduced
`internal/logging/logging.go:47`
[LSP unavailable — Grep fallback, reduced confidence]
What: The package-level `init()` function writes to `os.Stderr` via `fmt.Fprintf` when `LOG_LEVEL` is set to an unrecognized value. Writing to stderr from `init()` fires before any test harness is ready and before the MCP server is initialized. Any test that imports the `logging` package — directly or transitively — with an invalid `LOG_LEVEL` environment variable will emit noise to stderr during `go test`, potentially masking genuine test failures on CI systems that scan stderr for unexpected output.
Fix: Defer the stderr write to the first call to `Log()`, or move the environment-variable initialization check into an explicitly called `Configure()` function invoked by `main()`.

---

## All Findings

| Severity | Confidence | Check Type | Finding | Location |
|----------|------------|------------|---------|----------|
| error | reduced | dead_symbol | `generateResourceList` and `resourceTemplates` have no production callers — per-file resource discovery broken in running binary | `internal/resources/resources.go:217,266` |
| error | reduced | context_propagation | `SerializedExecutor.Acquire` ignores `ctx`; all session operations block indefinitely under cancellation | `internal/session/executor.go:21` |
| warning | reduced | dead_symbol | `ExtensionRegistry.Deactivate` is exported but has no production callers | `internal/extensions/registry.go:76` |
| warning | reduced | duplicate_semantics | `applyRangeEdit` duplicates `applyEditsToFile` algorithm; acknowledged in comment but not tracked | `internal/session/manager.go:425` |
| warning | reduced | doc_drift | `RootDir()` carries the `Initialize` doc comment as its first line | `internal/lsp/client.go:481` |
| warning | reduced | silent_failure | `workspace/configuration` params unmarshal error silently ignored; wrong-length config response possible | `internal/lsp/client.go:275` |
| warning | reduced | silent_failure | `applyDocumentChanges` discriminator unmarshal failure silently falls into `TextDocumentEdit` branch | `internal/lsp/client.go:1412` |
| warning | reduced | init_side_effects | `init()` writes to `os.Stderr` on invalid `LOG_LEVEL`; pollutes test output before harness is ready | `internal/logging/logging.go:47` |

---

## Not Checked — Out of Scope

- Test files (`*_test.go`) were read to confirm production call sites but were not themselves audited.
- `internal/config/` (argument parsing, autodetect) — no check type applies strongly to pure parsing/validation logic; no findings raised after reading.
- `internal/lsp/framing.go` — Content-Length framing; clean, no findings applicable.
- `internal/session/differ.go` — diagnostic diff logic; clean, no findings applicable.
- `extensions/haskell/` — stub extension with all-nil handlers; clean by inspection.
- `test/multi_lang_test.go` — integration test harness; excluded from scope.
- `interface_saturation` on `ClientResolver` (4 methods, all used by `server.go`) — not raised; 4 methods is within normal bounds.
- `test_coverage` — not applied; a full exported-symbol coverage sweep requires reliable LSP to be meaningful.

---

## Not Checked — Tooling Constraints

- **LSP `findReferences` and `hover`:** The built-in `LSP` tool was invoked for all symbol-level checks and returned `No such tool available: LSP` each time. All symbol-level checks fell back to Grep. All findings carry `confidence: reduced`. Grep cannot detect aliased calls, interface-dispatch call sites, or dynamically constructed symbol references.
- **`layer_violation`:** Verified by reading all import blocks against the committed layer map. No violations found. `internal/lsp` does not import `internal/tools` or `cmd`. `internal/types` imports only stdlib.
- **`panic_not_recovered`:** No `panic(` calls in production code. `runWithRecovery` in `cmd/lsp-mcp-go/main.go` handles panics from `server.Run` with a deferred `recover()`. No findings.
- **`SimulateChain` scope override:** The `SimulateChain` loop (session/manager.go:265) evaluates using hardcoded `"file"` scope regardless of the caller-supplied `timeoutMs`, silently ignoring workspace-scope evaluation for chained edits. This behavioral gap was noted during reading but not raised as a finding at reduced confidence — it may be intentional design given the per-step evaluation model.
