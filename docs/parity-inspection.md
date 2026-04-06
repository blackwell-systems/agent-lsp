# Parity Inspection: LSP-MCP-GO vs TypeScript Reference

**Inspector version:** 0.2.0
**Date:** 2026-04-06
**Repo root:** `/Users/dayna.blackwell/code/LSP-MCP-GO`
**Reference:** `/Users/dayna.blackwell/code/LSP-MCP`
**Checks applied:** `cross_field_consistency`, `doc_drift`, `dead_symbol`, `silent_failure`, `error_wrapping`

---

## Summary

- **Audited:** `internal/lsp/`, `internal/tools/`, `internal/types/types.go`, `internal/resources/`, `internal/extensions/`, `cmd/lsp-mcp-go/`, `extensions/haskell/`
- **Layer map:**
  ```
  cmd/lsp-mcp-go → internal/tools, internal/resources, internal/extensions, internal/lsp, internal/logging
  internal/tools → internal/lsp, internal/types, internal/logging
  internal/resources → internal/lsp, internal/types, internal/logging
  internal/extensions → internal/types, internal/logging
  internal/lsp → internal/types, internal/logging
  extensions/haskell → internal/extensions, internal/types
  Boundary: internal/lsp has no upward deps (only internal/types, internal/logging)
  ```
  No layer violations found.
- **Highest severity:** error
- **Signal:** Four concrete correctness bugs — a broken argument key (`apply_edit` can never reach its handler), a dead struct field used as version tracking storage but never read, two dead exported functions with no callers, and a diagnostic `Code` type narrower than the LSP spec — alongside a `WaitForDiagnostics` snapshot-skip gap relative to the TypeScript reference.

---

## internal/lsp/client.go

**cross_field_consistency** · error · confidence: high
`internal/lsp/client.go:99` · [LSP unavailable — Grep fallback, reduced confidence on zero-use claim; Grep exhaustive across all .go files]
What: `LSPClient.docVers map[string]int` is declared at line 99 and initialized at line 126 (`docVers: make(map[string]int)`), but is never read or written anywhere in the codebase outside these two sites. Version tracking for open documents is handled instead through the `docMeta.version` field inside `openDocs map[string]docMeta`. The two fields represent the same concept — per-document version counter — and having both creates an inconsistency: any code that queries `docVers` would get stale zeros while `docMeta.version` holds the real values.
Fix: Remove the `docVers` field and its initialization, or reconcile by removing the redundant `docMeta.version` field. Only one version-tracking mechanism should exist.

---

**doc_drift** · warning · confidence: reduced (LSP hover unavailable — Read fallback)
`internal/lsp/client.go:792` · [LSP unavailable — Read fallback]
What: The doc comment for `WaitForFileIndexed` says "Waits until the URI has received at least one diagnostic notification, then waits for a 1500ms quiet window." The comment does not describe the behavior when diagnostics are already in the cache at subscription time — specifically, `SubscribeToDiagnostics` immediately replays cached diagnostics to the new subscriber (line 695–709), which means if the file already has cached diagnostics, the first `<-stabilize` receive unblocks instantly from the replay, and the 1500ms stability window starts immediately without waiting for any new notification. The TypeScript `waitForFileIndexed` (lspClient.ts:1455) explicitly handles this case: `if (this.documentDiagnostics.has(uri)) { armStability(); }` — it documents that the cached case arms stability immediately. The Go doc says "waits until the URI has received at least one notification" but the implementation fires on replayed cache hits too.
Fix: Update the doc comment to note that if diagnostics are already cached for the URI, the stability window starts immediately (matching the TypeScript behavior note at line 1455).

---

**silent_failure** · error · confidence: high
`internal/lsp/client.go:1087` · [LSP unavailable — Grep fallback]
What: In `GetSignatureHelp`, the line `_ = json.Unmarshal(result, &v)` silently discards the unmarshal error. If the LSP response is malformed JSON, `v` remains `nil` and the function returns `nil, nil` — the caller gets a nil result with no indication that deserialization failed. The same pattern appears at lines 1148, 1177, and 1193 (`RenameSymbol`, `PrepareRename`, `ExecuteCommand`). The TypeScript reference propagates these as normal returns (the result is always `response ?? null`) without a separate unmarshal step, but the Go path introduces a silent failure that the TS path doesn't have.
Fix: Return the unmarshal error instead of discarding it: `if err := json.Unmarshal(result, &v); err != nil { return nil, fmt.Errorf("unmarshal response: %w", err) }`.

---

**error_wrapping** · warning · confidence: high
`internal/lsp/client.go:560` · [LSP unavailable — Grep fallback]
What: `Shutdown` calls `c.sendRequest(ctx, "shutdown", nil)` and returns the error bare: `return err` (line 562). The error from `sendRequest` carries no context about what operation failed — a caller sees only the timeout or network error string. The codebase's established wrapping convention (used in `Initialize`, `start`, etc.) is `fmt.Errorf("operation name: %w", err)`.
Fix: Wrap: `return fmt.Errorf("shutdown request: %w", err)`.

---

## internal/lsp/framing.go

**dead_symbol** · warning · confidence: high
`internal/lsp/framing.go:62` · [LSP unavailable — Grep fallback, exhaustive file search]
What: `sep := []byte("\r\n\r\n")` is assigned at line 62 and immediately blanked at line 74 with `_ = sep`. The variable is never used for anything; the separator bytes are found by a manual loop over `buf[i]`..`buf[i+3]` rather than by using `sep`. This is dead code masquerading as a named constant.
Fix: Remove the `sep` allocation and the `_ = sep` blank assignment. The bytes.Index or bytes.Equal approach with the named constant could be used as a cleanup, but the core fix is removing the dead variable.

---

## internal/lsp/diagnostics.go

**doc_drift** · error · confidence: reduced (LSP hover unavailable — Read fallback)
`internal/lsp/diagnostics.go:11` · [LSP unavailable — Read fallback]
What: The doc comment says `WaitForDiagnostics` "ignores the initial snapshot, requires one fresh notification per URI." The implementation does NOT skip the initial snapshot. When `SubscribeToDiagnostics(cb)` is called (line 54), the client immediately replays all cached diagnostics to `cb` (client.go line 700–708). For any URI already in the cache, this replay fires `cb`, sets `received[uri] = true`, and updates `lastEvent` — so the function treats the cached snapshot as "fresh." The TypeScript reference (`waitForDiagnostics.ts:44–76`) explicitly tracks `sawInitialSnapshot` per URI and skips the first callback for each URI as the initial snapshot. The Go implementation has no equivalent skip, meaning it can exit immediately after subscribing if all URIs are already cached, without waiting for any new diagnostic activity.
Fix: Implement the initial-snapshot skip as in the TypeScript reference — track a `sawSnapshot map[string]bool` per URI and discard the first callback for each URI.

---

## internal/tools/workspace.go + cmd/lsp-mcp-go/server.go

**cross_field_consistency** · error · confidence: high
`cmd/lsp-mcp-go/server.go:202` and `internal/tools/workspace.go:180` · [LSP unavailable — Grep fallback]
What: `apply_edit` is broken end-to-end. The `ApplyEditArgs` struct registered in `server.go` maps the workspace edit to the JSON key `"edit"` (`Edit interface{} \`json:"edit"\``). But `toolArgsToMap` serializes this struct to `map[string]interface{}` and passes it to `HandleApplyEdit`, which reads `args["workspace_edit"]` (workspace.go line 180). These keys do not match. Every call to `apply_edit` will hit `edit, ok := args["workspace_edit"]` with `ok = false` and immediately return `types.ErrorResult("workspace_edit is required")`, regardless of what the caller passes. The TypeScript schema (`ApplyEditArgsSchema`) uses the key `workspace_edit` consistently throughout.
Fix: Change `ApplyEditArgs.Edit` in server.go to use `json:"workspace_edit"`, matching the handler's lookup key and the TypeScript schema field name.

---

**silent_failure** · error · confidence: high
`internal/tools/workspace.go:102` · [LSP unavailable — Grep fallback]
What: In `HandleFormatDocument`, `toInt(args, "tab_size")` errors are silently discarded: `if v, err := toInt(args, "tab_size"); err == nil { tabSize = v }`. If a caller passes a non-integer for `tab_size`, the error is swallowed and the default (2) is used without any indication. The TypeScript Zod schema uses `z.coerce.number()` which would also silently coerce, but the Go handler uses `toInt` which explicitly returns an error — that error should surface. The same pattern appears in `HandleFormatRange` at line 146.
Fix: Return an error to the caller: `if v, err := toInt(args, "tab_size"); err != nil { return types.ErrorResult(err.Error()), nil } else { tabSize = v }`.

---

## internal/resources/resources.go

**dead_symbol** · error · confidence: high
`internal/resources/resources.go:229` and `internal/resources/resources.go:278` · [LSP unavailable — Grep fallback, exhaustive search across all non-test .go files]
What: `GenerateResourceList` (line 229) and `ResourceTemplates` (line 278) are exported functions with zero callers outside their own file. No import of the `resources` package references either symbol; `server.go` only calls `resources.HandleDiagnosticsResource`, `resources.HandleHoverResource`, and `resources.HandleCompletionsResource`. Both functions build resource listings that should feed MCP's `resources/list` and `resources/templates/list` endpoints, but those endpoints are not wired in `server.go`. The go-sdk MCP server used by the Go implementation currently has resources registered statically via `server.AddResource`; the dynamic listing functions have no connection to the server.
Fix: Either wire `GenerateResourceList` and `ResourceTemplates` into the server's resource listing endpoints, or mark them unexported (lowercase) until they are wired. As dead exported symbols they represent an untested, unexercised code path.

---

**error_wrapping** · warning · confidence: high
`internal/resources/resources.go:65` · [LSP unavailable — Grep fallback]
What: `WaitForDiagnostics` errors in the all-files branch are logged and suppressed rather than returned: `if err := lsp.WaitForDiagnostics(ctx, client, uris, 10000); err != nil { logging.Log(...) }`. The same pattern appears at line 96. A context cancellation (`ctx.Err()`) would be swallowed here, allowing the function to continue and return potentially stale diagnostics. The `HandleGetDiagnostics` tool handler in analysis.go (line 27) does return the error from `WaitForDiagnostics`, so there is inconsistency in how this error is treated across the two callers.
Fix: Propagate the error: `return ResourceResult{}, fmt.Errorf("waiting for diagnostics: %w", err)` — or at minimum check for `ctx.Err()` specifically.

---

## internal/types/types.go

**cross_field_consistency** · warning · confidence: high
`internal/types/types.go:34` · [LSP unavailable — Grep fallback]
What: `LSPDiagnostic.Code` is typed `string`, but the LSP specification (§3.17.1) defines `code` as `integer | string` — it can be either. The TypeScript reference reflects this: `code?: number | string`. Several language servers (notably rust-analyzer, clangd) emit integer diagnostic codes. If an integer code arrives in the JSON, Go's `json.Unmarshal` will fail to decode it into a `string` field and the `Code` field will be silently left empty (no error — `omitempty` silences the absence). Callers that rely on code values for filtering will see empty strings for all integer-coded diagnostics.
Fix: Change `Code` to `interface{}` or define a custom JSON unmarshaler that accepts both `string` and `int`/`float64`. The TypeScript uses `code?: number | string`.

---

## internal/logging/logging.go

**dead_symbol** · warning · confidence: high
`internal/logging/logging.go:58` · [LSP unavailable — Grep fallback, exhaustive search]
What: `SetServer(sender interface{})` is exported but has zero callers in the non-test codebase. `server.go` calls `logging.MarkServerInitialized()` but never calls `logging.SetServer(...)`. As a result, `mcpServer` is always `nil` and the `logSender` path in `Log` (lines 111–115) is never taken. All log messages route to stderr permanently even after the MCP server is running, which is a parity gap — the TypeScript implementation routes log messages through MCP `logging/message` notifications after initialization.
Fix: Call `logging.SetServer(server)` from `server.go` after the server is created, passing the MCP server instance (or a thin wrapper satisfying `logSender`). Alternatively, document that `SetServer` is intentionally unused if the current stderr-only behavior is deliberate.

---

## internal/lsp/client.go (initialize capabilities)

**cross_field_consistency** · warning · confidence: high
`internal/lsp/client.go:474` · [LSP unavailable — Grep fallback]
What: The Go `Initialize` capabilities block omits `clientInfo` (which the TypeScript sends as `{ name: "lsp-mcp-server", version: "0.3.0" }`) and omits `initializationOptions` (TypeScript sends tsserver and preferences for TypeScript language servers). While these are optional per spec, some language servers use `clientInfo` for feature negotiation. Additionally the TypeScript sends `didChangeConfiguration: { dynamicRegistration: true }` and `didChangeWatchedFiles: { dynamicRegistration: true }` in `workspace` capabilities; the Go implementation omits both. This means language servers that check for dynamic watched-files registration will not set up file watchers when connected to the Go client, potentially missing on-disk file change notifications.
Fix: Add `clientInfo`, `initializationOptions` (configurable), `workspace.didChangeConfiguration`, and `workspace.didChangeWatchedFiles` to the Go `Initialize` params to match the TypeScript reference.

---

## internal/lsp/client.go (restart root dir)

**coverage_gap** · warning · confidence: high
`internal/tools/session.go:46` · [LSP unavailable — Grep fallback]
What: `HandleRestartLspServer` calls `client.Restart(ctx, rootDir)` where `rootDir` is obtained via `args["root_dir"].(string)` with a blank fallback on type-assertion failure. If `root_dir` is absent from args, `rootDir` is `""` and `Restart` calls `Initialize(ctx, "")`, which constructs `rootURI = "file://"` — a malformed URI with no path. The LSP server receives an empty workspace root and may fail silently or index the wrong directory. The TypeScript reference handles this in `restart`: if `rootDirectory` is not provided, it reinitializes without a root or logs that initialization must be called separately.
Fix: Validate that `rootDir` is non-empty when provided; if absent, either reject the call with a clear error message or preserve the previously-used root directory. The current behavior silently passes an empty string to the LSP `initialize` request.

---

## All Findings

| Severity | Confidence | Check Type | Finding | Location |
|----------|------------|------------|---------|----------|
| error | high | cross_field_consistency | `apply_edit` broken: server.go maps `"edit"` but handler reads `"workspace_edit"` — tool always fails | `cmd/lsp-mcp-go/server.go:202` + `internal/tools/workspace.go:180` |
| error | high | dead_symbol | `GenerateResourceList` and `ResourceTemplates` exported but never called; resource listing not wired to MCP server | `internal/resources/resources.go:229,278` |
| error | high | silent_failure | `GetSignatureHelp`, `RenameSymbol`, `PrepareRename`, `ExecuteCommand` silently discard `json.Unmarshal` errors | `internal/lsp/client.go:1087,1148,1177,1193` |
| error | reduced | doc_drift | `WaitForDiagnostics` doc says "ignores initial snapshot" but implementation does not skip cached-replay notifications | `internal/lsp/diagnostics.go:11` |
| error | high | silent_failure | `HandleFormatDocument`/`HandleFormatRange` silently swallow `toInt` errors for `tab_size` | `internal/tools/workspace.go:102,146` |
| warning | high | cross_field_consistency | `LSPDiagnostic.Code` typed `string` but LSP spec is `integer \| string`; integer codes silently lost | `internal/types/types.go:34` |
| warning | high | cross_field_consistency | `LSPClient.docVers` field declared and initialized but never read; redundant with `docMeta.version` | `internal/lsp/client.go:99` |
| warning | high | dead_symbol | `logging.SetServer` exported but never called; MCP log routing permanently falls through to stderr | `internal/logging/logging.go:58` |
| warning | high | cross_field_consistency | `Initialize` missing `clientInfo`, `initializationOptions`, `workspace.didChangeConfiguration`, `workspace.didChangeWatchedFiles` vs TypeScript reference | `internal/lsp/client.go:474` |
| warning | high | coverage_gap | `restart_lsp_server` passes empty `rootDir` to `Initialize` when `root_dir` arg absent; constructs malformed `rootURI = "file://"` | `internal/tools/session.go:46` |
| warning | high | dead_symbol | `framing.go` allocates `sep := []byte("\r\n\r\n")` then immediately blanks it with `_ = sep`; never used | `internal/lsp/framing.go:62` |
| warning | high | error_wrapping | `Shutdown` returns bare `err` with no operation context | `internal/lsp/client.go:560` |
| warning | high | error_wrapping | `WaitForDiagnostics` errors logged and suppressed in resource handlers instead of returned | `internal/resources/resources.go:65,96` |
| warning | reduced | doc_drift | `WaitForFileIndexed` doc says "waits until URI received one notification" but fires on cached replay | `internal/lsp/client.go:792` |

---

## Not Checked — Out of Scope

- Test files (`*_test.go`, `integration_test.go`) — explicitly excluded per audit instructions
- `test/` directory contents — explicitly excluded per audit instructions
- `extensions/haskell/` beyond confirming interface compliance with `types.Extension`
- TypeScript-side findings — this audit covers only the Go implementation
- `init_side_effects` check — `logging.go:init()` reads `LOG_LEVEL` env var; this is a known pattern and was not in the requested check list
- `layer_violation`, `scope_analysis`, `test_coverage`, `duplicate_semantics`, `panic_not_recovered`, `context_propagation`, `interface_saturation` — not in the `--checks` filter

## Not Checked — Tooling Constraints

- LSP `findReferences` and `hover` via `mcp__lsp-mcp__get_references` / `mcp__lsp-mcp__start_lsp`: the MCP tool surface is not available as a bash command in this environment. All symbol-level checks used exhaustive Grep across the full non-test Go source tree as fallback. Findings derived from Grep are annotated `[LSP unavailable — Grep fallback, reduced confidence]`. The Grep searches were repo-wide and exhaustive for the specific symbol names, providing high practical confidence even without LSP confirmation.
- TypeScript LSP analysis: no `tsserver` available for hover on the reference implementation; TypeScript analysis relied on reading source files directly.
