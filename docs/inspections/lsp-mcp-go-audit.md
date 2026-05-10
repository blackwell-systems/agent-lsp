# agent-lsp Code Quality Audit

**Date:** 2026-04-06
**Inspector version:** 0.2.0

---

## Summary

- **Audited:** `internal/session/`, `internal/tools/`, `cmd/agent-lsp/server.go`, `internal/lsp/`, `internal/resources/`, `internal/types/`, `internal/extensions/`, `internal/config/`, `cmd/agent-lsp/main.go`
- **Layer map:**
  ```
  cmd/agent-lsp → internal/tools, internal/resources, internal/session, internal/lsp, internal/types, internal/extensions, internal/config
  internal/tools → internal/lsp, internal/types, internal/logging
  internal/session → internal/lsp, internal/types, internal/logging
  internal/resources → internal/lsp, internal/types, internal/logging
  internal/lsp → internal/types, internal/logging, internal/config
  internal/extensions → internal/types, internal/logging
  Boundary: internal/lsp must not import cmd/*; internal/* must not import each other in cycles
  ```
- **Highest severity:** error
- **Signal:** The codebase is well-structured and largely correct; six bugs warrant immediate attention — the most critical is a key mismatch that silently drops `execute_command` arguments in every call, a `uriToPath` divergence that breaks percent-encoded paths in the session package, and unguarded `session.Status` mutations that race with the per-session mutex methods.

---

## `cmd/agent-lsp/server.go`

**coverage_gap** · error · confidence: high
`cmd/agent-lsp/server.go:108` · [LSP unavailable — Grep fallback]
What: `clientForFile` ignores both the `resolver` and `filePath` parameters and always returns `cs.get()`. The function signature advertises multi-server routing (it accepts a `resolver` and `filePath`) but the body is `return cs.get()`. This means multi-server mode never routes by file extension at this call site — all tools that use `clientForFile` (open_document, close_document, get_diagnostics, inspect_symbol, get_completions, get_signature_help, suggest_fixes, list_symbols, find_references, go_to_definition, go_to_type_definition, go_to_implementation, go_to_declaration, rename_symbol, prepare_rename, format_document, format_range) always use the default client regardless of the file's language. The comment at the function acknowledges this: `TODO: Multi-server routing should update both cs and resolver when servers start.`
Fix: Route through `resolver.ClientForFile(filePath)` instead of `cs.get()`, consistent with how `csResolver.ClientForFile` delegates to the real resolver.

---

**coverage_gap** · error · confidence: high
`cmd/agent-lsp/server.go:240-242` · [LSP unavailable — Grep fallback]
What: `ExecuteCommandArgs` declares the arguments field with JSON tag `"arguments"`, so `toolArgsToMap` serializes the field as `{"arguments": [...]}`. However, `HandleExecuteCommand` reads `args["args"].([]interface{})` — key `"args"` not `"arguments"`. The type-assert always fails silently, so `cmdArgs` is always `nil` regardless of what the caller provides. Every `execute_command` call that passes arguments silently discards them, forwarding an empty argument list to the LSP server instead.
Fix: Change the lookup in `HandleExecuteCommand` from `args["args"]` to `args["arguments"]`.

---

**scope_analysis** · warning · confidence: high
`cmd/agent-lsp/server.go:72-103` · [LSP unavailable — Grep fallback]
What: `makeCallToolResult` performs two distinct responsibilities: JSON-marshaling the intermediate `types.ToolResult` to bytes, then re-parsing a purpose-built anonymous struct to extract `Content` and `IsError` fields, then re-building an `*mcp.CallToolResult`. This double round-trip exists because the layer boundary prevents `cmd/` from importing the MCP SDK types into `internal/types`. The function is 32 lines of error-path logic for what is structurally a type adapter.
Fix: Define a concrete adapter type in `internal/types` or accept that the double marshal is the intentional bridge, but document the reason. The function is not a scope problem per se — it is doing one thing — but the dual-parse is a fragile implementation that could silently drop content if the anonymous struct shape drifts from `types.ToolResult`.

---

## `internal/tools/` package

**duplicate_semantics** · warning · confidence: high
`internal/tools/simulation.go:40,76,105,165,192,213,234,313` · [LSP unavailable — Grep fallback]
What: Eight sites in `simulation.go` use `data, _ := json.Marshal(result)` discarding the marshal error, while all other tool files in the same package use the explicit two-step pattern `data, mErr := json.Marshal(result); if mErr != nil { return ErrorResult(...) }`. The session tool handlers apply an inconsistent error-handling convention. In practice, marshaling a `struct` with well-typed fields will not fail, but the divergence makes the code harder to audit and deviates from the established codebase pattern without documentation.
Fix: Apply the same explicit error check used everywhere else in the package, or document why the silent discard is safe for session types.

---

**coverage_gap** · error · confidence: high
`internal/tools/simulation.go:62-64` · [LSP unavailable — Grep fallback]
What: `HandleSimulateEdit` extracts `new_text` as `args["new_text"].(string)` with `if !ok` guard, but the guard only checks for a type mismatch — it does not check for the key being absent from the map. When `new_text` is a missing key (not a zero string), `ok` is `false` and the error message is `"new_text is required"`. This is correct. However, the behavior is inconsistent with other required string fields in the same function: `session_id` and `file_path` both use `!ok || field == ""`. A caller passing `new_text: ""` (empty string) would pass this guard and issue an edit with empty content, whereas the description says the field is required.
Fix: Add an `|| newText == ""` guard only if empty string should be treated as missing. If empty string is a valid replacement text (deletion), document that explicitly.

---

**duplicate_semantics** · warning · confidence: high
`internal/tools/helpers.go:76` and `internal/resources/resources.go:91,152,205` · [LSP unavailable — Grep fallback]
What: URI construction is done three different ways across the codebase. `internal/tools/helpers.go` provides `CreateFileURI` (using `url.URL{Scheme: "file", Path: filePath}.String()` — correctly handles encoding). `internal/resources/resources.go` uses bare string concatenation `"file://" + filePath` and `"file://" + path` at three separate sites. `internal/session/manager.go` uses `strings.TrimPrefix(uri, "file://")` for the reverse. The tools package uses `URIToFilePath` (which uses `url.Parse`) while the session package uses a bare string strip. Callers that hit the resources path will produce URIs without encoding while callers that go through tools will get encoded URIs.
Fix: Route all URI construction through `tools.CreateFileURI` and `tools.URIToFilePath` (or export them from a shared `internal/uriutil` package). The session package's `uriToPath` should use `url.Parse` like the tools version, not `strings.TrimPrefix`.

---

**coverage_gap** · warning · confidence: high
`internal/tools/session.go:63-82` (HandleOpenDocument) · [LSP unavailable — Grep fallback]
What: `HandleOpenDocument` calls `CreateFileURI(filePath)` without calling `ValidateFilePath` first. All other position-based tool handlers use `WithDocument` which calls `ValidateFilePath` internally. `HandleOpenDocument` is the path for direct document opens and it bypasses the path-traversal check. An attacker who can call `open_document` with a relative or traversal path would open arbitrary files on the server.
Fix: Call `ValidateFilePath(filePath, client.RootDir())` before `CreateFileURI` in `HandleOpenDocument`, consistent with how `WithDocument` does it.

---

## `internal/session/` package

**coverage_gap** · error · confidence: high
`internal/session/manager.go:192-197` · [LSP unavailable — Grep fallback]
What: In `Evaluate`, `session.Status` is read and written without holding the session's own `mu` lock. The session struct has a `sync.Mutex` (`s.mu`) that guards `Status`, `DirtyErr` (used by `IsDirty()` and `IsTerminal()`). However, the manager methods read and set `session.Status` directly — bypassing the per-session mutex — while `MarkDirty`, `IsDirty`, and `IsTerminal` do use the mutex. This is a data race: `MarkDirty` (which can be called from LSP error paths) and the manager's status mutations can run concurrently, causing corrupted state or a Go race detector failure.
Specifically:
- `ApplyEdit` at line 161: `session.Status = StatusMutated` without `session.mu`
- `Evaluate` at lines 197, 241: `session.Status = StatusEvaluating` / `StatusEvaluated` without `session.mu`
- `Commit` at line 342: `session.Status = StatusCommitted` without `session.mu`
- `Discard` at line 388: `session.Status = StatusDiscarded` without `session.mu`
- `Destroy` at line 406: `session.Status = StatusDestroyed` without `session.mu`
Fix: Either guard all direct `session.Status` mutations with `session.mu.Lock()`, or consolidate status transitions into methods on `SimulationSession` the way `MarkDirty` is already done.

---

**duplicate_semantics** · error · confidence: high
`internal/session/manager.go:466-470` vs `internal/lsp/client.go:1797-1806` · [LSP unavailable — Grep fallback]
What: Two `uriToPath` functions exist in different packages with different behavior. `lsp.uriToPath` uses `url.Parse` and falls back to string trimming, correctly handling percent-encoded characters (e.g. paths with spaces `%20`). `session.uriToPath` uses only `strings.TrimPrefix(uri, "file://")` without decoding. The session package's version has a comment: `"For production, use proper URI decoding."` Paths with spaces or other special characters will fail in the session package — `os.ReadFile(path)` will receive a path with literal `%20` instead of a space.
Fix: Replace `session.uriToPath` with the `url.Parse`-based implementation, or export `lsp.uriToPath` and have `session` use it.

---

**coverage_gap** · error · confidence: high
`internal/session/manager.go:353-392` (Discard) · [LSP unavailable — Grep fallback]
What: `Discard` re-reads each file from disk using `os.ReadFile(path)` to restore the LSP server's view of the file. However, the path is computed via `uriToPath(uri)` which (per the finding above) does not percent-decode. More critically, the function assumes the on-disk file is the pre-edit baseline. If the caller has written the file to disk between `ApplyEdit` and `Discard` (e.g. an external tool), the "revert" will use the modified disk content, not the original baseline. The original content is already stored in `session.Baselines[uri]` (indirectly — the baseline tracks diagnostics, not content), but `session.Contents[uri]` at baseline time is never preserved. After `ApplyEdit`, `session.Contents[uri]` holds the post-edit content, so `Discard` cannot use it for revert either. It relies entirely on the disk file not having changed.
Fix: At baseline capture time in `ApplyEdit` (around line 131), store the original disk content in a `OriginalContents` map separate from `Contents`. Use that in `Discard` instead of re-reading from disk.

---

**silent_failure** · error · confidence: high
`internal/session/manager.go:335-338` (Commit, apply=true) · [LSP unavailable — Grep fallback]
What: In `Commit` with `apply=true`, if `session.Client.OpenDocument` fails (the LSP notification after writing files), the error is only logged as a warning — the function continues and returns `CommitResult` with `Status=committed`. The LSP server's view of the written file is now stale (it still holds the old content) but the caller receives no error signal. Subsequent `get_diagnostics` calls will reflect stale LSP state.
Fix: Propagate the OpenDocument error at line 335, or at minimum set `session.MarkDirty` so callers know the session is in an inconsistent state.

---

**dead_symbol** · warning · confidence: high
`internal/session/types.go:24` · [LSP unavailable — Grep fallback]
What: `StatusTimedOut SessionStatus = "timed_out"` is defined but never assigned. A search across the entire repository finds no assignment of this constant — it appears in the `const` block but no code path transitions a session into this state.
Fix: Remove the constant, or document which future code path will use it.

---

**dead_symbol** · warning · confidence: high
`internal/session/types.go:33` · [LSP unavailable — Grep fallback]
What: `ConfidenceStale Confidence = "stale"` is defined but never assigned or referenced in any code path. Same pattern as `StatusTimedOut`.
Fix: Remove the constant, or document which future code path will use it.

---

**test_coverage** · warning · confidence: high
`internal/session/manager.go` — `Evaluate`, `Commit`, `Discard`, `SimulateChain` · [LSP unavailable — Grep fallback]
What: The four substantive `SessionManager` methods that drive the core simulation lifecycle have no unit tests in `manager_test.go`. The test file tests only `NewSessionManager`, `CreateSession` (nil client path), `GetSession`, `applyRangeEdit` (4 cases), and one dirty-state commit check. `Evaluate`, `Commit` (apply=true and apply=false), `Discard`, and `SimulateChain` are untested.
Fix: Add unit tests using a mock `LSPClient` for each of the four methods, covering at least the status-transition guards and error paths.

---

## `internal/lsp/client.go`

**scope_analysis** · warning · confidence: high
`internal/lsp/client.go:1393-1479` (ApplyWorkspaceEdit) · [LSP unavailable — Grep fallback]
What: `ApplyWorkspaceEdit` handles four distinct concerns: type normalization (marshal/unmarshal to map), file-operation dispatch for `documentChanges` (create/rename/delete/text-edit), fallback to the `changes` map, and `didChange` notification (delegated). The `documentChanges` branch alone has four discriminated sub-cases, each with its own unmarshal. The function is 86 lines. By comparison, peer functions in this file average 15–25 lines.
Fix: Extract the `documentChanges` dispatch into a private `applyDocumentChanges(raw []json.RawMessage)` helper. Not an error, but a natural boundary.

---

**coverage_gap** · warning · confidence: high
`internal/lsp/client.go:280-289` (dispatch, `workspace/applyEdit`) · [LSP unavailable — Grep fallback]
What: When handling the server-initiated `workspace/applyEdit` request, if `ApplyWorkspaceEdit` returns an error it is silently discarded with `_ =`. The LSP spec requires the client to respond with `ApplyWorkspaceEditResult{applied: bool, failureReason: string}`. The current code always responds `{applied: true}` even if the edit failed. Downstream — the LSP server may issue further edits or actions based on the assumption the edit was applied, corrupting state.
Fix: Check the error from `ApplyWorkspaceEdit`, and if it fails, respond with `{applied: false, failureReason: err.Error()}`.

---

**coverage_gap** · warning · confidence: high
`internal/lsp/client.go:482-488` (Initialize) · [LSP unavailable — Grep fallback]
What: `rootURI` is constructed as `"file://" + rootDir` (bare string concatenation, not through `url.URL`). If `rootDir` contains special characters (spaces, Unicode), the resulting URI will be malformed. This is the only URI construction in `client.go` that does not use `url.URL` — all others use `url.Parse`/`url.URL` via `uriToPath`.
Fix: Use `url.URL{Scheme: "file", Path: rootDir}.String()` consistent with `CreateFileURI` in `tools/helpers.go`.

---

**init_side_effects** · warning · confidence: high
`extensions/haskell/` (imported via `_ "github.com/blackwell-systems/agent-lsp/extensions/haskell"` in main.go) · [LSP unavailable — Grep fallback]
What: The `init()` function in the haskell extension calls `extensions.RegisterFactory(...)`, which mutates a global map (`factories`) under a mutex. This is the intended design — it is documented in the architecture. The pattern is not a bug; it is flagged here for completeness because `init()` global mutation is present. The registration is deterministic and has no I/O.
Fix: None — this is the documented design. Noted for completeness.

---

## `internal/resources/resources.go`

**duplicate_semantics** · warning · confidence: high
`internal/resources/resources.go:116-167` and `169-225` · [LSP unavailable — Grep fallback]
What: `HandleHoverResource` and `HandleCompletionsResource` share an identical 20-line prefix: parse URI, extract `line`, `column`, `language_id` query params, convert to `strconv.Atoi`, validate, convert to `types.Position`. This inline parse-and-validate block is duplicated verbatim in both functions. The only difference after line 145 (in both functions) is the LSP operation called and the result marshaling.
Fix: Extract a private `parseResourceQueryParams(uri string) (filePath string, pos types.Position, languageID string, err error)` helper, used by both handlers.

---

**dead_symbol** · warning · confidence: reduced
`internal/resources/resources.go:229` — `generateResourceList` · [LSP unavailable — Grep fallback]
What: `generateResourceList` is defined but Grep finds no call site in any non-test file. The function is not exported. It builds a list of `ResourceEntry` objects from open documents, but nothing in `server.go` calls it — the server registers resources statically via `server.AddResource`.
Fix: Confirm whether this function was intended for a `resources/list` handler. If not wired, either wire it or remove it.

---

**dead_symbol** · warning · confidence: reduced
`internal/resources/resources.go:278` — `resourceTemplates` · [LSP unavailable — Grep fallback]
What: `resourceTemplates` is defined but not called anywhere. Same pattern as `generateResourceList`.
Fix: Same as above — wire or remove.

---

## `internal/types/types.go`

**duplicate_semantics** · error · confidence: high
`internal/types/types.go:146-151` vs `internal/extensions/registry.go:88-129` (Extension interface) · [LSP unavailable — Grep fallback]
What: Two distinct `Extension` interface definitions exist. `internal/types/types.go` defines:
```go
type Extension interface {
    ToolHandlers() map[string]ToolHandler
    ResourceHandlers() map[string]ResourceHandler
    SubscriptionHandlers() map[string]ResourceHandler
    PromptHandlers() map[string]interface{}
}
```
The architecture document (`docs/architecture.md`) documents a different, larger interface:
```go
type Extension interface {
    ToolHandlers() map[string]ToolHandler
    ToolDefinitions() []mcp.Tool
    ResourceHandlers() map[string]ResourceHandler
    SubscriptionHandlers() map[string]SubscriptionHandler
    UnsubscriptionHandlers() map[string]UnsubscriptionHandler
    PromptDefinitions() []mcp.Prompt
    PromptHandlers() map[string]PromptHandler
}
```
The types used in `registry.go` (`SubscriptionHandlers`, `PromptHandlers`) match the `types.Extension` interface, but `registry.go` calls them on `types.Extension` values. The architecture doc describes `SubscriptionHandlers` returning `map[string]SubscriptionHandler` but the type returns `map[string]ResourceHandler`. The documented interface and the implemented interface are out of sync — doc drift — and the `types.Extension` interface is missing `ToolDefinitions`, `UnsubscriptionHandlers`, and `PromptDefinitions`.
Fix: Reconcile the implemented interface in `types.go` with the documented interface in `docs/architecture.md`, or update the docs. The missing methods suggest features were dropped or deferred without updating the design doc.

---

**doc_drift** · warning · confidence: high
`docs/architecture.md:15` · [LSP unavailable — Grep fallback]
What: The architecture document states "24 MCP tool handlers" in the package description. The server registers 34 tools (counted from `server.go`). The tools section of the README lists 26. The three sources disagree on tool count.
Fix: Update `docs/architecture.md` to reflect the current tool count.

---

## `internal/lsp/manager.go`

**error_wrapping** · warning · confidence: high
`internal/lsp/manager.go:144-157` (Shutdown) · [LSP unavailable — Grep fallback]
What: `Shutdown` iterates all clients, calls `client.Shutdown`, and on error only keeps the last error (`lastErr = err`). Prior errors from earlier clients are silently discarded. The caller receives at most one error even if three of five language server shutdowns fail.
Fix: Use a multi-error accumulator (e.g. `errors.Join` in Go 1.20+) to preserve all shutdown errors.

---

## Cross-cutting: All Tool Handlers

**scope_analysis** · warning · confidence: high
All files in `internal/tools/` · [LSP unavailable — Grep fallback]
What: All 20+ tool handlers that operate on a file path follow an identical 5-step pattern inline:
1. `CheckInitialized(client)`
2. Extract and validate `file_path`
3. Extract and validate position or range
4. Infer `language_id`, defaulting to `"plaintext"`
5. Call `WithDocument` or LSP method

Steps 2 and 4 are copy-pasted verbatim across `analysis.go`, `navigation.go`, `workspace.go`, `callhierarchy.go`, `semantic_tokens.go`. A caller that adds a new tool handler must remember to replicate all five steps. The `extractPosition` and `extractRange` helpers address step 3 well, but steps 2 and 4 remain inline.
Fix: Consider a `extractFileArgs(args map[string]interface{}) (filePath, languageID string, err error)` helper for steps 2 and 4, keeping each handler's body to: validate args, call WithDocument, marshal result.

---

## All Findings

| Severity | Confidence | Check Type | Finding | Location |
|----------|------------|------------|---------|----------|
| error | high | coverage_gap | `clientForFile` ignores resolver; all tools use default client regardless of file language | `cmd/agent-lsp/server.go:108` |
| error | high | coverage_gap | `execute_command` arguments silently dropped: struct field `"arguments"` vs handler key `"args"` | `cmd/agent-lsp/server.go:242`, `internal/tools/workspace.go:207` |
| error | high | coverage_gap | `session.Status` mutated without per-session mutex in 5 locations; races with `MarkDirty`/`IsTerminal`/`IsDirty` | `internal/session/manager.go:161,197,241,342,388,406` |
| error | high | duplicate_semantics | `session.uriToPath` strips `"file://"` without percent-decoding; diverges from `lsp.uriToPath` which uses `url.Parse` | `internal/session/manager.go:466` vs `internal/lsp/client.go:1797` |
| error | high | coverage_gap | `Discard` re-reads from disk instead of using saved baseline content; fails if file changed between edit and discard | `internal/session/manager.go:353` |
| error | high | silent_failure | `Commit` apply=true: `OpenDocument` LSP notification error is logged only as warning; session marked committed while LSP state is stale | `internal/session/manager.go:335` |
| error | high | duplicate_semantics | Two distinct `Extension` interface definitions (`types.Extension` vs arch doc); missing `ToolDefinitions`, `UnsubscriptionHandlers`, `PromptDefinitions` | `internal/types/types.go:146` |
| error | high | coverage_gap | `HandleOpenDocument` bypasses `ValidateFilePath`; path-traversal check skipped for direct document opens | `internal/tools/session.go:77` |
| error | high | coverage_gap | `workspace/applyEdit` server request: error from `ApplyWorkspaceEdit` is discarded; server always receives `applied:true` even on failure | `internal/lsp/client.go:286` |
| warning | high | duplicate_semantics | `json.Marshal` error silently discarded (`_, _`) in 8 session tool handler sites; all peer handlers in package use explicit error check | `internal/tools/simulation.go:40,76,105,165,192,213,234,313` |
| warning | high | duplicate_semantics | URI construction via bare `"file://" + path` at 3 resource sites vs `url.URL`-based `CreateFileURI` in tools | `internal/resources/resources.go:91,152,205` |
| warning | high | coverage_gap | `rootURI` in Initialize uses bare string concat `"file://" + rootDir`; not percent-encoded | `internal/lsp/client.go:488` |
| warning | high | test_coverage | `Evaluate`, `Commit`, `Discard`, `SimulateChain` — four core SessionManager methods have no unit tests | `internal/session/manager.go:172,304,353,257` |
| warning | high | dead_symbol | `StatusTimedOut` defined, never assigned | `internal/session/types.go:24` |
| warning | high | dead_symbol | `ConfidenceStale` defined, never assigned | `internal/session/types.go:33` |
| warning | high | scope_analysis | `ApplyWorkspaceEdit` has 4 distinct dispatch sub-cases (86 lines); naturally splits at `applyDocumentChanges` boundary | `internal/lsp/client.go:1393` |
| warning | high | duplicate_semantics | `HandleHoverResource` and `HandleCompletionsResource` share identical 20-line URI parse/validate prefix | `internal/resources/resources.go:116,169` |
| warning | high | error_wrapping | `Shutdown` discards all but the last error when multiple language servers fail shutdown | `internal/lsp/manager.go:144` |
| warning | high | doc_drift | Architecture doc says "24 MCP tool handlers"; server registers 34 | `docs/architecture.md:15` |
| warning | high | doc_drift | `Extension` interface in `types.go` missing `ToolDefinitions`, `UnsubscriptionHandlers`, `PromptDefinitions` compared to arch doc | `internal/types/types.go:146` |
| warning | high | coverage_gap | `new_text` in `HandleSimulateEdit` allows empty string (deletion); inconsistent guard vs other required string fields | `internal/tools/simulation.go:62` |
| warning | high | scope_analysis | file_path extract + language_id default repeated verbatim in 12+ tool handlers; candidate for shared helper | `internal/tools/analysis.go`, `navigation.go`, `workspace.go`, `callhierarchy.go`, `semantic_tokens.go` |
| warning | reduced | dead_symbol | `generateResourceList` defined but never called | `internal/resources/resources.go:229` |
| warning | reduced | dead_symbol | `resourceTemplates` defined but never called | `internal/resources/resources.go:278` |

---

## Not Checked — Out of Scope

- `extensions/haskell/` implementation details (only its registration was checked)
- `internal/config/` parse logic and autodetect
- `internal/logging/` package internals
- `integration_test.go` and `test/multi_lang_test.go` correctness
- Layer violations were not found; the declared layer map is respected by all imports checked

## Not Checked — Tooling Constraints

- **LSP findReferences / hover:** The `mcp__lsp-mcp__start_lsp` tool was not available during this audit session (the inspector does not have MCP tool invocation capability in this environment). All symbol-level findings use Grep fallback. Confidence is annotated as "reduced" for dead_symbol findings on `generateResourceList` and `resourceTemplates` because Grep may miss callers in generated code or dynamic dispatch. All other findings are structural (key mismatch, mutex gap, interface mismatch) and are high-confidence regardless of LSP tool availability.
- **Race detector confirmation:** The `session.Status` race finding is based on static code analysis (direct field mutation without mutex vs. mutex-guarded methods on the same field). It was not confirmed by running `go test -race`.
