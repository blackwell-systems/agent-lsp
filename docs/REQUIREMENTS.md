# LSP-MCP-GO Requirements

## Project

A complete Go port of the LSP-MCP server (`/Users/dayna.blackwell/code/LSP-MCP`).
The TypeScript implementation is the authoritative spec — behavior, tool names, argument
schemas, and response formats must be identical so existing MCP clients work without changes.

## Language & Runtime

- Go 1.22+
- Single compiled binary: `lsp-mcp-go`
- No runtime dependencies (language servers are external processes)

## CLI Interface

```
lsp-mcp-go <language-id> <lsp-server-binary> [lsp-server-args...]
```

Same as the TypeScript version. Binary validates the LSP server path exists before starting.

## MCP SDK

`github.com/modelcontextprotocol/go-sdk` v1.4.1 (official Anthropic Go SDK).
Use `mcp.StdioTransport{}` for the server transport.

## LSP Client

- Spawns the LSP server binary as a subprocess, communicates over stdin/stdout
- Buffer-based message framing: `Content-Length: N\r\n\r\n` + N bytes UTF-8 body
- Handles server-initiated requests: `window/workDoneProgress/create`, `workspace/configuration`, `client/registerCapability`
- Progress tracking: waits for all `$/progress` work tokens to complete before sending cross-file queries
- On subprocess crash: reject all pending response promises immediately, log last 4KB of stderr
- stdin/stdout/stderr error handlers (EPIPE prevention)
- `initialized = true` set BEFORE sending `initialized` notification (race fix)

## Tools (24 total — all must be ported)

### Session
- `start_lsp` — initialize LSP server with root_dir
- `restart_lsp_server` — restart without restarting MCP server
- `open_document` — open file for tracking
- `close_document` — stop tracking file

### Analysis
- `get_diagnostics` — errors/warnings; omit file_path for whole project
- `get_info_on_location` — hover info at position
- `get_completions` — completion suggestions at position
- `get_signature_help` — function signature at call site
- `get_code_actions` — quick fixes for a range
- `get_document_symbols` — all symbols in a file
- `get_workspace_symbols` — search symbols across workspace

### Navigation
- `get_references` — all references to a symbol
- `go_to_definition` — jump to definition (normalize LocationLink[] → Location[])
- `go_to_type_definition` — jump to type definition
- `go_to_implementation` — jump to all implementations
- `go_to_declaration` — jump to declaration (C/C++ headers)

### Refactoring
- `rename_symbol` — returns WorkspaceEdit
- `prepare_rename` — validate rename is valid at position
- `format_document` — returns TextEdit[]
- `format_range` — returns TextEdit[] for a selection
- `apply_edit` — applies WorkspaceEdit to disk
- `execute_command` — server-side command execution

### Utilities
- `did_change_watched_files` — notify server of file changes on disk
- `set_log_level` — change log verbosity at runtime

## Tool Argument Schemas

All positions are 1-indexed (line/column ≥ 1). Range tools validate start ≤ end.
Mirror the Zod schemas in `/Users/dayna.blackwell/code/LSP-MCP/src/types/index.ts`.
Use Go structs with json tags. Validate at handler entry, return structured errors.

## withDocument Pattern

16 of 24 tool handlers follow this pattern:
1. Check LSP client initialized
2. Read file from disk
3. Create file URI (`file:///absolute/path`)
4. Call `openDocument` on LSP client
5. Execute query
6. Return formatted result

Implement as a generic helper: `withDocument[T](lspClient, filePath, languageId, callback)`.

## Response Format

All tools return `{ content: [{ type: "text", text: <JSON string> }] }`.
`go_to_definition` and navigation tools return `[]Location` with `{ file, line, column, end_line, end_column }` (1-indexed, path not URI).
`get_diagnostics` waits for diagnostic stabilisation (500ms quiet window after last notification, or timeout).

## Resources

- `lsp-diagnostics://` — all open files
- `lsp-diagnostics:///path/to/file` — specific file (subscribable)
- `lsp-hover:///path/to/file?line=N&column=N&language_id=X`
- `lsp-completions:///path/to/file?line=N&column=N&language_id=X`

Real-time subscription: send `notifications/resources/updated` when diagnostics change.

## Logging

MCP logging bridge: map Go's log levels to MCP `logging/message` notifications.
`LOG_LEVEL` env var controls verbosity (debug/info/notice/warning/error/critical).
Override console output so it routes through MCP protocol, not raw stderr.

## Extension System

Per-language extensions loaded at startup by language ID.
Extension files at `extensions/<language-id>.go` (or compiled in via a registry).
Each extension can provide additional tool handlers, resource handlers, prompt handlers.
Extensions take precedence over core handlers on name conflict.

## Process Lifecycle

- SIGINT/SIGTERM: graceful shutdown (send LSP `shutdown` + `exit`, then exit 0)
- `unhandledRejection` equivalent: recover from panics in goroutines, log and continue
- `start_lsp` tool: return `{isError: true}` on failure rather than panicking

## Module

`github.com/blackwell-systems/lsp-mcp-go`

## Tests

Go-native integration tests (`testing` package).
Spawn the compiled binary as a subprocess, communicate via MCP stdio protocol.
Mirror the 7-language Tier 1 + Tier 2 test matrix from the TypeScript test suite.
Tests skip gracefully if language server binary is not found on PATH.

## Reference Implementation

All behavior details: `/Users/dayna.blackwell/code/LSP-MCP/`
Key files:
- `src/lspClient.ts` — LSP subprocess and framing
- `src/tools/index.ts` — all 24 tool handlers + withDocument
- `src/resources/index.ts` — resource handlers
- `src/shared/` — shared utilities
- `src/types/index.ts` — argument schemas
