# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, Semantic Versioning.

## [Unreleased]

### Added
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
- `apply_edit` argument field is `workspace_edit` (matches TypeScript schema)
- `execute_command` argument field is `args` (matches TypeScript schema)
- `get_references` `include_declaration` defaults to `false` (matches TypeScript schema)
- `GetInfoOnLocation` hover parsing handles all four LSP `MarkupContent` shapes (string, MarkupContent, MarkedString, MarkedString array)
- `WaitForDiagnostics` timeout 25,000ms (matches TypeScript reference)
- `applyEditsToFile` sends correct incremented version number in `textDocument/didChange`
- `format_document` and `format_range` default `tab_size` is 2 (matches TypeScript schema)
- `did_change_watched_files` accepts empty `changes` array per LSP spec
