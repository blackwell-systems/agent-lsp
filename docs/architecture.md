# Architecture

agent-lsp is a [Model Context Protocol](https://modelcontextprotocol.io/) server that wraps a Language Server Protocol subprocess. This document describes the package structure, key patterns, and internal design decisions.

---

## Package Structure

```
cmd/main.go                        ← CLI entrypoint
internal/lsp/                      ← LSP subprocess wrapper
  client.go                        ← LSPClient struct; startWatcher/stopWatcher (fsnotify auto-watch)
  framing.go                       ← Content-Length framing
  diagnostics.go                   ← WaitForDiagnostics
  normalize.go                     ← NormalizeDocumentSymbols, NormalizeCompletion, NormalizeCodeActions
internal/tools/                    ← 31 MCP tool handlers
  helpers.go                       ← WithDocument[T], shared utilities
  analysis.go                      ← get_diagnostics, hover, completions, signatures, code actions, symbols
  navigation.go                    ← definition, references, implementation, declaration
  callhierarchy.go                 ← call_hierarchy
  typehierarchy.go                 ← type_hierarchy
  inlayhints.go                    ← get_inlay_hints
  highlights.go                    ← get_document_highlights
  capabilities.go                  ← get_server_capabilities
  detect.go                        ← detect_lsp_servers
  session.go, workspace.go, utilities.go, semantic_tokens.go, fuzzy.go, simulation.go
internal/resources/                ← Resource + subscription handlers
internal/extensions/               ← Extension registry
internal/types/types.go            ← Shared types (TypeHierarchyItem, InlayHint, DocumentHighlight,
                                     DocumentSymbol, CompletionList, CodeAction)
internal/logging/logging.go        ← MCP logging bridge
extensions/<language>/             ← Per-language extensions
```

### Layer rules

- `cmd/main.go` owns the MCP server lifecycle and routes requests to handlers
- `internal/tools/` and `internal/resources/` both import from `internal/lsp/` and `internal/types/` — they do not import from each other
- `internal/lsp/` has no upward dependencies — it only imports from `internal/types/`
- `extensions/` imports from `internal/tools/` for re-exported utilities

---

## The `WithDocument` Pattern

Most tool handlers need to open a file before querying the language server. The `WithDocument` helper encapsulates this:

```go
func WithDocument[T any](
  client *lsp.LSPClient,
  filePath string,
  languageID string,
  callback func(*lsp.LSPClient, string) (T, error),
) (T, error)
```

Used by the majority of tool handlers:

```go
func handleGoToDefinition(client *lsp.LSPClient, args GoToDefinitionArgs) (mcp.CallToolResult, error) {
    return tools.WithDocument(client, args.FilePath, args.LanguageID, func(c *lsp.LSPClient, fileURI string) (mcp.CallToolResult, error) {
        locations, err := c.GetDefinition(fileURI, lsp.Position{
            Line:      args.Line - 1,      // tools use 1-based; LSP uses 0-based
            Character: args.Column - 1,
        })
        if err != nil {
            return mcp.CallToolResult{}, err
        }
        data, _ := json.Marshal(locations)
        return mcp.CallToolResult{Content: []mcp.Content{{Type: "text", Text: string(data)}}}, nil
    })
}
```

Handlers that do not follow this pattern (e.g. `open_document`, `get_diagnostics`, `get_workspace_symbols`, `get_server_capabilities`, `detect_lsp_servers`) manage the LSP client directly — they either don't require a file path or have different lifecycle semantics.

---

## Auto-Watch

When `start_lsp` initializes the LSP client, `startWatcher(rootDir)` is called
automatically. A goroutine watches the workspace root recursively using
[fsnotify](https://github.com/fsnotify/fsnotify) and forwards file system events
to the LSP server via `workspace/didChangeWatchedFiles`. Events are debounced
with a 150ms window and batched into a single notification. Standard
build/cache directories (`node_modules`, `vendor`, `target`, `.git`, etc.) are
skipped. Dynamically-created subdirectories are added to the watcher on creation.

`stopWatcher()` is called during `Shutdown` and at the beginning of each
`startWatcher` call (to replace a previous watcher on `start_lsp` reinit).

This means the `did_change_watched_files` tool is not required for normal
editing workflows. The auto-watcher keeps the LSP index fresh without manual
calls.

---

## LSP Response Normalization

`internal/lsp/normalize.go` centralises the handling of LSP responses that
have multiple valid shapes per spec:

- **`NormalizeDocumentSymbols(raw)`** — converts `DocumentSymbol[] |
  SymbolInformation[]` to `[]types.DocumentSymbol`. Discriminates on the
  presence of `selectionRange`. When `SymbolInformation[]` is returned,
  performs a two-pass tree reconstruction using `containerName` to attach
  children to parents.

- **`NormalizeCompletion(raw)`** — converts `CompletionItem[] | CompletionList`
  to `types.CompletionList`. Discriminates on the presence of an `items` field.

- **`NormalizeCodeActions(raw)`** — converts `(Command | CodeAction)[]` to
  `[]types.CodeAction`. Discriminates each element by checking whether the
  `command` field is a JSON string (bare `Command`) or an object (`CodeAction`).
  Bare commands are wrapped in a synthetic `CodeAction`.

---

## URI Handling

LSP uses `file://` URIs throughout. Two utilities handle the conversion:

```go
// path → URI  (for sending to the LSP server)
CreateFileURI("/path/to/file.go")  // → "file:///path/to/file.go"

// URI → path  (for reading results from the LSP server)
URIToFilePath("file:///path/to/file.go")  // → "/path/to/file.go"
```

`URIToFilePath` uses `url.Parse(uri).Path` rather than string slicing, which correctly handles percent-encoded characters and is robust to non-standard URI forms.

**Position coordinates:** Tool inputs are 1-based (line 1, column 1 = first character). LSP is 0-based internally. The conversion `args.Line - 1` / `args.Column - 1` happens inside each handler. Argument validation rejects `line: 0` and `column: 0` with a clear error.

---

## LSP Client Lifecycle

```
start_lsp (tool call)
    ↓
LSPClient.Initialize(rootDir)
    ↓
exec.Command(lspServerPath)
    ↓
SendRequest("initialize", capabilities)
    ↓  ← server may send window/workDoneProgress/create, workspace/configuration here
    ↓  ← these server-initiated requests are handled in handleServerRequest()
receive initialize response
    ↓
client.initialized = true
SendNotification("initialized", {})
    ↓
tool calls now available
```

`initialized` is set to `true` before `initialized` is sent (not after) to prevent a race where the server's first request arrives in the window between sending `initialized` and setting the flag.

When the LSP subprocess crashes, the exit handler:
1. Sets `initialized = false`
2. Rejects all pending response channels immediately (callers fail fast instead of waiting for timeouts)
3. Logs the last 4KB of stderr for diagnosis

---

## Resource Subscription System

Resources expose LSP data over MCP's subscribe/unsubscribe mechanism. When a client subscribes to a diagnostic resource, the server sends `notifications/resources/updated` each time diagnostics change for that file.

```
client → resources/subscribe { uri: "lsp-diagnostics:///path/to/file.go" }
                                              ↓
                              lspClient.SubscribeToDiagnostics(callback)
                                              ↓
                              callback stored in subscription context Map
                                              ↓
          later: LSP server sends textDocument/publishDiagnostics
                                              ↓
                              callback fires → server.Notification("notifications/resources/updated")
                                              ↓
client ← notifications/resources/updated { uri: "lsp-diagnostics:///path/to/file.go" }
                                              ↓
client → resources/read { uri: "lsp-diagnostics:///path/to/file.go" }
                                              ↓
client ← current diagnostics JSON
```

The subscription callback is stored in a `map[string]SubscriptionContext` server-side so it can be correctly removed on unsubscribe.

---

## WaitForDiagnostics

`WaitForDiagnostics(client *LSPClient, targetURIs []string, timeoutMs int)` is used by `get_diagnostics` to wait for the language server to finish publishing diagnostics after a document is opened. It resolves when:

1. All target URIs have received at least one diagnostic notification *after* the initial snapshot (the first notification is excluded — it's the server's pre-existing state)
2. No further diagnostic notifications arrive for 500ms (the "stabilisation" window)
3. OR the optional timeout is exceeded

An empty `targetURIs` slice resolves immediately.

---

## Extension System

Language-specific extensions are registered at compile time via `init()` functions. An extension lives at `extensions/<language-id>/` and registers itself through `extensions.RegisterFactory`:

```go
// extensions/haskell/haskell.go
func init() {
    extensions.RegisterFactory("haskell", func() extensions.Extension {
        return &HaskellExtension{}
    })
}
```

An extension can implement any subset of the extension interface:

```go
type Extension interface {
    ToolHandlers() map[string]ToolHandler
    ResourceHandlers() map[string]ResourceHandler
    SubscriptionHandlers() map[string]ResourceHandler
    PromptHandlers() map[string]interface{}
    // Note: ToolDefinitions, UnsubscriptionHandlers, PromptDefinitions are
    // deferred features not yet implemented.
}
```

All features are namespaced by language ID automatically. Extensions take precedence over core handlers in case of name conflicts.

Unlike the TypeScript implementation which uses dynamic `import()` at runtime, Go extensions are registered at compile time — unused extensions have zero runtime cost, and there is no filesystem scan or dynamic loading.
