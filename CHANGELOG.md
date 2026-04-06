# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, Semantic Versioning.

## [Unreleased]

### Added
- Multi-server routing — single `lsp-mcp-go` process manages multiple language servers; routes tool calls to the correct server by file extension. Supports inline arg-pairs (`go:gopls typescript:tsserver,--stdio`) and `--config lsp-mcp.json`; backward-compatible with existing single-server invocation
- `call_hierarchy` tool — single tool with `direction: "incoming" | "outgoing" | "both"` (default: both); hides the two-step LSP prepare/query protocol behind one call; returns typed JSON with `items`, `incoming`, `outgoing`
- Fuzzy position fallback for `go_to_definition` and `get_references` — when a direct position lookup returns empty, falls back to workspace symbol search by hover name and retries at each candidate; handles AI assistant position imprecision without correctness regression
- Path traversal prevention — `ValidateFilePath` in `WithDocument` resolves all `..` components and verifies the result is within the workspace root; stores `rootDir` on `LSPClient` (set during `Initialize`)
- `types.CallHierarchyItem`, `types.CallHierarchyIncomingCall`, `types.CallHierarchyOutgoingCall` — typed protocol structs for call hierarchy responses
- `types.TextEdit`, `types.SymbolInformation`, `types.SemanticToken` — typed protocol structs; `FormatDocument`/`FormatRange` and `GetWorkspaceSymbols` migrated from `interface{}` to typed returns
- `types.SymbolKind`, `types.SymbolTag` — integer enum types used across call hierarchy and symbol structs
- Tool count: 24 → 25 (26 pending semantic tokens)

### Added (LSP 3.17 spec compliance)
- `workspace/applyEdit` server-initiated request handler — client now responds `ApplyWorkspaceEditResult{applied:true}` instead of null; servers using this for code actions (e.g. file creation/rename) no longer silently fail
- `documentChanges` resource operations: `CreateFile`, `RenameFile`, `DeleteFile` entries now executed (discriminated by `kind` field); previously only `TextDocumentEdit` was processed
- `$/progress report` kind handled — intermediate progress notifications are now logged at debug level instead of silently discarded
- `PrepareRename` `bool` capability case — `renameProvider: true` (no options object) no longer incorrectly sends `textDocument/prepareRename`; correctly returns nil when `prepareProvider` not declared
- `uriToPath` now uses `url.Parse` for RFC 3986-correct percent-decoding — fixes file reads/writes for workspaces with spaces or special characters in path (was using raw string slicing, leaving `%20` literal)
- Removed deprecated `rootPath` from `initialize` params — superseded by `rootUri` and `workspaceFolders`

### Added
- Multi-language integration test harness — Go port of `multi-lang.test.js` using `mcp.CommandTransport` + `ClientSession.CallTool` from the official Go MCP SDK
- Tier 1 tests (start_lsp, open_document, get_diagnostics, get_info_on_location) for all 7 languages: TypeScript, Python, Go, Rust, Java, C, PHP
- Tier 2 tests (get_document_symbols, go_to_definition, get_references, get_completions, get_workspace_symbols, format_document, go_to_declaration) for all 7 languages
- Test fixtures for all 7 languages with cross-file greeter files for `get_references` coverage
- GitHub Actions CI: `test` job (unit tests, every PR) and `multi-lang-test` job (full 7-language matrix)
- `WaitForDiagnostics` initial-snapshot skip — matches TypeScript `sawInitialSnapshot` behavior; prevents early exit when URIs are already cached
- `Initialize` now sends `clientInfo`, `workspace.didChangeConfiguration`, and `workspace.didChangeWatchedFiles` capabilities to match TypeScript reference
- Initial Go port of LSP-MCP — full 1:1 implementation with TypeScript reference
- All 24 tools: session (4), analysis (7), navigation (5), refactoring (6), utilities (2)
- `WithDocument[T]` generic helper — Go equivalent of the TypeScript `withDocument` pattern
- Single binary distribution via `go install github.com/blackwell-systems/lsp-mcp-go@latest`
- Buffer-based LSP message framing with byte-accurate `Content-Length` parsing (no UTF-8/UTF-16 mismatch)
- `WaitForDiagnostics` with 500ms stabilisation window
- `WaitForFileIndexed` with 1500ms stability window — lets gopls finish cross-package indexing before issuing `get_references`
- Extension registry with compile-time factory registration via `init()`
- `SubscriptionHandlers` and `PromptHandlers` on the `Extension` interface
- Full 14-method LSP request timeout table matching the TypeScript reference
- `$/progress` tracking for workspace-ready detection
- Server-initiated request handling: `window/workDoneProgress/create`, `workspace/configuration`, `client/registerCapability`
- Graceful SIGINT/SIGTERM shutdown with LSP `shutdown` + `exit` sequence
- `GetCodeActions` passes overlapping diagnostics in context per LSP 3.17 §3.16.8
- `SubscribeToDiagnostics` replays current diagnostic snapshot to new subscribers
- `ReopenDocument` fallback to disk read on untracked URI

### Fixed
- `FormattedLocation` JSON field names match TypeScript response shape (`file`, `line`, `column`, `end_line`, `end_column`)
- `apply_edit` argument field is `workspace_edit` in both handler and server registration (was `edit` in `ApplyEditArgs` struct, causing every call to fail silently)
- `execute_command` argument field is `args` (matches TypeScript schema)
- `get_references` `include_declaration` defaults to `false` (matches TypeScript schema)
- `GetInfoOnLocation` hover parsing handles all four LSP `MarkupContent` shapes (string, MarkupContent, MarkedString, MarkedString array)
- `WaitForDiagnostics` timeout 25,000ms (matches TypeScript reference)
- `applyEditsToFile` sends correct incremented version number in `textDocument/didChange`
- `format_document` and `format_range` default `tab_size` is 2 (matches TypeScript schema)
- `format_document` and `format_range` now surface invalid `tab_size` argument errors to callers instead of silently using the default
- `did_change_watched_files` accepts empty `changes` array per LSP spec
- `restart_lsp_server` rejects missing `root_dir` with a clear error instead of sending malformed `rootURI = "file://"` to the LSP server
- `GetSignatureHelp`, `RenameSymbol`, `PrepareRename`, `ExecuteCommand` now propagate JSON unmarshal errors instead of returning `nil, nil` on malformed LSP responses
- `LSPDiagnostic.Code` changed from `string` to `interface{}` — integer codes from rust-analyzer, clangd, etc. are no longer silently dropped
- Removed dead `docVers` field from `LSPClient` (version tracking uses `docMeta.version`)
- `Shutdown` error now wrapped with operation context
- `GenerateResourceList` and `ResourceTemplates` made unexported — they had no external callers and were not wired to the MCP server
- `WaitForDiagnostics` errors in resource handlers now propagate instead of being logged and suppressed
- Removed dead `sep` variable in `framing.go` (`tryParse` allocated `[]byte("\r\n\r\n")` then immediately blanked it)
