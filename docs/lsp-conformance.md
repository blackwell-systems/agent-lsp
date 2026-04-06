# LSP 3.17 Conformance

lsp-mcp was built directly against the [LSP 3.17 specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/). Each protocol area was implemented by reading the relevant spec section and verified through integration testing against real language servers (gopls, rust-analyzer, typescript-language-server, pyright, jdtls, clangd, intelephense).

The spec section links below are anchored directly into the specification.

---

## Method Coverage Matrix

Every LSP 3.17 method and its MCP surface. "Protocol only" means the method is correctly handled at the transport layer (capabilities declared, responses sent) but not exposed as an MCP tool.

### Text Document Methods

| LSP Method | Spec | MCP Tool | Status |
|-----------|------|----------|--------|
| `textDocument/didOpen` | §3.15.7 | `open_document` | ✓ |
| `textDocument/didClose` | §3.15.9 | `close_document` | ✓ |
| `textDocument/publishDiagnostics` | §3.17.1 | `get_diagnostics` | ✓ |
| `textDocument/hover` | §3.15.11 | `get_info_on_location` | ✓ |
| `textDocument/completion` | §3.15.13 | `get_completions` | ✓ |
| `textDocument/signatureHelp` | §3.15.14 | `get_signature_help` | ✓ |
| `textDocument/definition` | §3.15.2 | `go_to_definition` | ✓ |
| `textDocument/references` | §3.15.8 | `get_references` | ✓ |
| `textDocument/documentSymbol` | §3.15.20 | `get_document_symbols` | ✓ |
| `textDocument/codeAction` | §3.15.22 | `get_code_actions` | ✓ |
| `textDocument/formatting` | §3.15.16 | `format_document` | ✓ |
| `textDocument/rename` | §3.15.19 | `rename_symbol` | ✓ |
| `textDocument/typeDefinition` | §3.15.3 | `go_to_type_definition` | ✓ |
| `textDocument/implementation` | §3.15.4 | `go_to_implementation` | ✓ |
| `textDocument/declaration` | §3.15.5 | `go_to_declaration` | ✓ |
| `textDocument/prepareRename` | §3.15.19 | `prepare_rename` | ✓ |
| `textDocument/selectionRange` | §3.15.29 | — | ✗ not yet implemented |
| `textDocument/foldingRange` | §3.15.28 | — | ✗ not yet implemented |
| `textDocument/documentHighlight` | §3.15.10 | — | ✗ not yet implemented |
| `textDocument/rangeFormatting` | §3.15.17 | `format_range` | ✓ |
| `textDocument/codeLens` | §3.15.21 | — | ✗ not yet implemented |
| `textDocument/inlayHint` | §3.17.11 | — | ✗ not yet implemented |
| `textDocument/semanticTokens` | §3.16.12 | — | ✗ not yet implemented |

### Workspace Methods

| LSP Method | Spec | MCP Tool | Status |
|-----------|------|----------|--------|
| `workspace/symbol` | §3.15.21 | `get_workspace_symbols` | ✓ |
| `workspace/configuration` | §3.16.14 | — | ✓ protocol only (server-initiated) |
| `workspace/executeCommand` | §3.16.13 | `execute_command` | ✓ |
| `workspace/didChangeWatchedFiles` | §3.16.8 | `did_change_watched_files` | ✓ |

### Protocol Infrastructure

| Area | Status |
|------|--------|
| Lifecycle (`initialize` → `initialized` → `shutdown`) | ✓ |
| Progress protocol (`$/progress` begin/report/end) | ✓ |
| `window/workDoneProgress/create` (server-initiated) | ✓ |
| `client/registerCapability` (server-initiated) | ✓ |
| Unrecognized server requests | ✓ (null response) |
| Message framing (Content-Length, UTF-8 byte count) | ✓ |
| JSON-RPC 2.0 shapes | ✓ |
| LSP error codes (-32601, -32002) | ✓ |
| Process crash → pending promise rejection | ✓ |

---

## [Lifecycle](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#lifeCycleMessages) (§3.15.1–3.15.4)

- Correct `initialize` → `initialized` → `shutdown` sequence
- Graceful async shutdown via `SIGINT`/`SIGTERM` — the LSP subprocess is never orphaned on exit
- Client capabilities declared for every feature used: `hover`, `completion`, `references`, `definition`, `implementation`, `typeDefinition`, `codeAction`, `publishDiagnostics`, `window.workDoneProgress`, `workspace.configuration`
- Server capabilities checked before sending requests — if a server doesn't declare `hoverProvider`, `completionProvider`, `referencesProvider`, or `codeActionProvider`, the request is skipped rather than being sent and silently returning empty results
- `initialize` timeout set to 300s to accommodate JVM-based servers (jdtls) that require 60-90s for cold OSGi container startup
- LSP process crash immediately rejects all pending promises — callers fail fast rather than waiting for individual timeouts

---

## [Progress Protocol](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#progress) (§3.18)

- `window/workDoneProgress/create` — the progress token is pre-registered in `activeProgressTokens` before the response is sent, so subsequent `$/progress` notifications are always recognized
- `$/progress` begin/report/end — all three `WorkDoneProgress` kinds are handled:
  - `begin`: token added to active set, title logged
  - `report`: intermediate progress logged
  - `end`: token removed; when active set reaches zero, workspace-ready resolvers are notified
- `waitForWorkspaceReady()` blocks `textDocument/references` requests until all active progress tokens complete, ensuring gopls has finished workspace indexing before reference queries are sent

---

## Server-Initiated Requests

All three server-initiated request types sent by gopls (and common in other LSP servers) are handled:

### [`workspace/configuration`](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#workspace_configuration) (§3.16.14)

Responds with an array of `null` values matching `params.items.length`. Without this response, gopls blocks workspace loading and `$/progress end` never fires, so `waitForWorkspaceReady()` would hang indefinitely.

### `window/workDoneProgress/create`

Responds with `null` (the required result). The progress token is extracted from `params.token` and pre-registered before responding, ensuring the subsequent `$/progress begin` notification is recognized.

### `client/registerCapability`

Responds with `null`. Dynamic capability registration is acknowledged without modifying any state.

All unrecognized server-initiated requests also receive a `null` response to unblock the server rather than timing out.

---

## [Message Framing](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#baseProtocol) (§3.4)

- Content-Length header uses the UTF-8 byte length of the content (not the character count)
- Delimiter is `\r\n\r\n` as required
- Buffer overflow (>10MB) discards the entire buffer rather than keeping tail bytes, which would guarantee starting mid-message

---

## [JSON-RPC 2.0](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#baseProtocol) (§3.3)

- Request shape: `{ jsonrpc: "2.0", id, method, params? }` — correct
- Response shape: `{ jsonrpc: "2.0", id, result? | error? }` — correct
- Notification shape: `{ jsonrpc: "2.0", method, params? }` (no `id`) — correct
- IDs are monotonically incrementing integers

---

## [Error Codes](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#responseMessage) (§3.6)

LSP-defined error codes are handled distinctly:

| Code | Name | Handling |
|------|------|----------|
| `-32601` | MethodNotFound | Logged as `warning` — indicates an unsupported feature |
| `-32002` | ServerNotInitialized | Logged as `warning` — indicates a sequencing issue |
| All others | — | Logged at `debug` level |

---

## Response Shape Normalization

### [`textDocument/hover`](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_hover) (§3.15.11)

The `Hover.contents` field can be one of three shapes. All are handled in priority order:

1. **`MarkupContent`** (current spec): `{ kind: "markdown" | "plaintext", value: string }` — `kind` is checked first to distinguish rendering intent
2. **`MarkedString[]`** (deprecated): array of `string | { language, value }` — joined with newlines
3. **Plain string** (deprecated MarkedString): returned as-is

### [`textDocument/completion`](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_completion) (§3.15.13)

Both response shapes are handled:
- `CompletionItem[]` — returned directly
- `CompletionList` (`{ isIncomplete: boolean, items: CompletionItem[] }`) — `items` extracted

### [`textDocument/codeAction`](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_codeAction) (§3.15.22)

`CodeActionContext.diagnostics` is populated with diagnostics from `documentDiagnostics` whose range overlaps the requested range, enabling diagnostic-specific quick fixes. Sending an empty array would prevent servers from offering fixes tied to visible errors.

### [`textDocument/publishDiagnostics`](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_publishDiagnostics) (§3.17.1)

- `versionSupport: false` declared in client capabilities — server omits the optional `version` field
- `uri` and `diagnostics` destructured correctly; `uri` validated as a string before processing
- Diagnostics stored per-URI and used to populate `codeAction` context and `waitForFileIndexed` readiness detection

---

## Previously Non-Conformant (Fixed)

These issues were identified via spec audit and corrected:

| Issue | Spec Reference | Fix |
|-------|---------------|-----|
| `notifications/resources/update` (wrong method name) | MCP spec | Corrected to `notifications/resources/updated` |
| `UnsubscribeRequest.params.context` (field doesn't exist in MCP schema) | MCP spec | Subscription contexts now tracked server-side in a `Map<uri, context>` |
| `process.on('exit', async)` — await never completes | §3.15.4 | Replaced with SIGINT/SIGTERM handlers |
| `workspace/configuration` not responded to | §3.16.14 | Added handler; this was blocking gopls workspace loading |
| `window/workDoneProgress/create` response in wrong code path | §3.18 | Moved to server-initiated request handler block |
| `rootPath` sent in `initialize` params | §3.15.1 | Removed (deprecated, superseded by `rootUri`) |
| Empty `diagnostics: []` in `codeAction` context | §3.15.22 | Replaced with overlapping diagnostics filter |
| `MarkupContent.kind` ignored in hover response | §3.15.11 | `kind` now checked before accessing `value` |
