# Architecture

agent-lsp is a [Model Context Protocol](https://modelcontextprotocol.io/) server that wraps one or more Language Server Protocol subprocesses. This document describes the package structure, key patterns, and internal design decisions.

---

## Package Structure

```
cmd/agent-lsp/
  main.go          ← CLI entrypoint; argument parsing, signal handling, panic recovery
  server.go        ← MCP server construction; tool/resource registration; mcpSessionSender

internal/config/
  config.go        ← ServerEntry + Config types for multi-server JSON config
  parse.go         ← Argument parsing (single-server, multi-server, --config, auto-detect)
  infer.go         ← InferWorkspaceRoot: walks up from a file to find go.mod/package.json/etc.
  autodetect.go    ← AutodetectServers: scans PATH for known language server binaries

internal/lsp/
  client.go        ← LSPClient: subprocess lifecycle, JSON-RPC framing, request/response
                     correlation, server-initiated requests, file watcher
  manager.go       ← ServerManager: multi-server registry, ClientForFile routing by extension
  resolver.go      ← ClientResolver interface
  framing.go       ← Content-Length framing (FrameReader / FrameWriter)
  diagnostics.go   ← WaitForDiagnostics: stabilization wait with timeout
  normalize.go     ← NormalizeDocumentSymbols, NormalizeCompletion, NormalizeCodeActions

internal/session/
  manager.go       ← SessionManager: create/apply/evaluate/commit/discard/destroy sessions
  types.go         ← SimulationSession, SessionStatus, EvaluationResult, ChainResult, etc.
  executor.go      ← SerializedExecutor: serializes concurrent LSP access within a session
  differ.go        ← DiffDiagnostics: baseline vs. current diagnostic comparison

internal/tools/
  helpers.go       ← WithDocument[T], CreateFileURI, URIToFilePath, ValidateFilePath,
                     CheckInitialized
  analysis.go      ← get_diagnostics, hover, completions, signatures, code actions, symbols
  navigation.go    ← definition, references, implementation, declaration, type_definition
  callhierarchy.go ← call_hierarchy (incoming/outgoing)
  typehierarchy.go ← type_hierarchy (supertypes/subtypes)
  inlayhints.go    ← get_inlay_hints
  highlights.go    ← get_document_highlights
  semantic_tokens.go ← get_semantic_tokens
  capabilities.go  ← get_server_capabilities
  detect.go        ← detect_lsp_servers
  documentation.go ← get_symbol_documentation (dispatches to go doc, pydoc, cargo doc)
  symbol_source.go ← get_symbol_source (extracts source text for a symbol at a position)
  symbol_path.go   ← go_to_symbol (fuzzy workspace symbol → definition)
  simulation.go    ← Tool handlers for the speculative execution layer
  build.go         ← run_build, run_tests, get_tests_for_file
  workspace.go     ← workspace folder management (add/remove/list)
  workspace_folders.go ← add_workspace_folder, remove_workspace_folder, list_workspace_folders
  session.go       ← start_lsp, open_document, close_document, restart_lsp_server
  utilities.go     ← apply_edit, execute_command, did_change_watched_files, set_log_level,
                     format_document, format_range, rename_symbol, prepare_rename
  fuzzy.go         ← fuzzy matching utilities for workspace symbol lookup
  position_pattern.go ← position_pattern argument handling (e.g. "func Foo")
  runner.go        ← build/test runner dispatch table

internal/resources/
  resources.go     ← HandleDiagnosticsResource, HandleHoverResource, HandleCompletionsResource;
                     ResourceTemplates()
  subscriptions.go ← HandleSubscribeDiagnostics, HandleUnsubscribeDiagnostics

internal/types/
  types.go         ← Shared concrete types: Position, Range, Location, LSPDiagnostic,
                     DocumentSymbol, CompletionList, CodeAction, CallHierarchyItem,
                     TypeHierarchyItem, InlayHint, DocumentHighlight, SemanticToken,
                     ToolResult, Extension interface

internal/logging/
  logging.go       ← Log, SetServer, SetLevel, MarkServerInitialized; MCP notification bridge

internal/extensions/
  registry.go      ← ExtensionRegistry; Activate, RegisterFactory, GetToolHandlers, etc.

skills/            ← Agent Skills (SKILL.md directories)
  install.sh       ← Installer: symlinks or copies skill dirs to ~/.claude/skills/
  lsp-verify/      ← Three-layer verification (diagnostics + build + tests)
  lsp-safe-edit/   ← Edit with before/after diagnostic diff
  lsp-simulate/    ← Speculative edit session management
  lsp-impact/      ← Blast-radius analysis (references + call hierarchy + type hierarchy)
  lsp-implement/   ← Find all concrete implementations of an interface
  lsp-rename/      ← Two-phase safe rename (preview then apply)
  lsp-edit-symbol/ ← Edit a named symbol without knowing its coordinates
  lsp-edit-export/ ← Edit exported symbols after finding all callers
  lsp-dead-code/   ← Find exported symbols with zero references
  lsp-docs/        ← Fetch toolchain documentation for a symbol
  lsp-format-code/ ← Format a file or range
  lsp-local-symbols/ ← List all symbols in a file
  lsp-cross-repo/  ← Cross-repository navigation
  lsp-test-correlation/ ← Map source files to test files
```

### Layer rules

- `cmd/agent-lsp/` owns the MCP server lifecycle and routes requests to handlers
- `internal/tools/` and `internal/resources/` import from `internal/lsp/`, `internal/session/`, and `internal/types/` — they do not import from each other
- `internal/lsp/` imports only from `internal/types/` and `internal/logging/` — no upward dependencies
- `internal/session/` imports from `internal/lsp/`, `internal/types/`, and `internal/logging/`
- `internal/extensions/` imports from `internal/types/` only
- `extensions/<language>/` imports from `internal/tools/` for re-exported utilities

---

## Request Lifecycle

A typical MCP tool call flows as follows:

```
MCP client → JSON-RPC over stdio
    ↓
server.go: mcp.Server dispatches to the registered tool handler
    ↓
clientForFileWithAutoInit(filePath)
    ↓  resolves the correct *LSPClient for this file (single or multi-server)
    ↓  auto-inits the workspace if no start_lsp has been called yet
    ↓
tools.HandleXxx(ctx, client, args)
    ↓
tools.WithDocument[T](ctx, client, filePath, languageID, cb)
    ↓  reads file from disk, sends textDocument/didOpen (or didChange), returns URI
    ↓
client.GetXxx(ctx, fileURI, position)
    ↓  writes JSON-RPC request with Content-Length framing to the LSP subprocess stdin
    ↓  blocks on pendingRequest channel
    ↓
LSP subprocess responds → readLoop() → dispatch() → unblocks pending channel
    ↓
handler receives json.RawMessage result
    ↓  (normalize.go normalizes polymorphic response shapes)
    ↓
types.ToolResult{Content: [{type:"text", text: JSON}]}
    ↓
server.go: makeCallToolResult converts to *mcp.CallToolResult
    ↓
MCP client receives JSON-RPC response
```

Handlers that do not use `WithDocument` (e.g., `get_diagnostics`, `open_document`, `get_workspace_symbols`, `get_server_capabilities`, `detect_lsp_servers`, `run_build`, `get_symbol_documentation`) manage the LSP client directly because they either do not require a file path or have different lifecycle semantics (build tools, toolchain commands).

---

## Multi-Server Routing

### Invocation modes

The binary accepts four invocation forms:

```bash
# Single-server (legacy): language-id and binary explicitly provided
agent-lsp go gopls

# Multi-server: colon-separated language:binary pairs
agent-lsp go:gopls typescript:typescript-language-server,--stdio

# Config file: JSON with a "servers" array
agent-lsp --config /path/to/lsp-mcp.json

# Auto-detect: scans PATH for known language server binaries
agent-lsp
```

### ClientResolver interface

```go
type ClientResolver interface {
    ClientForFile(filePath string) *LSPClient  // route by file extension
    DefaultClient() *LSPClient                 // primary/only client
    AllClients() []*LSPClient
    Shutdown(ctx context.Context) error
}
```

`ServerManager` is the sole implementation. In single-server mode the extension map is empty, so `ClientForFile` always falls back to `DefaultClient`. In multi-server mode each `managedEntry` carries a set of lowercase, dot-stripped extensions (e.g. `{"go": true}`, `{"ts": true, "tsx": true}`).

`ClientForFile` does a linear scan of entries comparing `filepath.Ext(filePath)` against each entry's extension set. The first match wins. If no match is found, it falls back to `entries[0].client`.

### csResolver wrapper

`server.go` wraps the real resolver in a `csResolver` that layers `clientState` (a mutex-guarded `*LSPClient`) on top. `start_lsp` writes the freshly initialized client into `clientState` so tools that call `DefaultClient()` immediately after `start_lsp` see the correct instance.

### Auto-init

If a tool handler receives a `file_path` argument and no client has been initialized yet, `autoInitClient` calls `config.InferWorkspaceRoot(filePath)` (walks up looking for `go.mod`, `package.json`, `Cargo.toml`, etc.) and invokes `sm.StartAll(ctx, root)` automatically. This allows tools to work without an explicit `start_lsp` call when the workspace root is unambiguous.

---

## The `WithDocument` Pattern

Most tool handlers need to open a file before querying the language server. The `WithDocument` helper encapsulates this in a single call:

```go
func WithDocument[T any](
    ctx context.Context,
    client *lsp.LSPClient,
    filePath string,
    languageID string,
    cb func(fileURI string) (T, error),
) (T, error)
```

Internally it:
1. Calls `ValidateFilePath` to resolve to an absolute path and reject path traversal
2. Reads the file content from disk
3. Calls `client.OpenDocument(ctx, fileURI, content, languageID)` — which sends `textDocument/didOpen` if the file is new or `textDocument/didChange` if already tracked
4. Invokes the callback with the `file://` URI

Usage example:

```go
locations, err := tools.WithDocument[[]types.Location](ctx, client, args.FilePath, args.LanguageID,
    func(fileURI string) ([]types.Location, error) {
        return client.GetDefinition(ctx, fileURI, lsp.Position{
            Line:      args.Line - 1,   // 1-based → 0-based
            Character: args.Column - 1,
        })
    })
```

**Position coordinates:** Tool inputs are 1-based (line 1, column 1 = first character). LSP is 0-based internally. The conversion `args.Line - 1` / `args.Column - 1` happens inside each handler. Argument validation rejects `line: 0` and `column: 0` with a clear error.

---

## Speculative Execution Layer

The speculative execution layer lets callers apply edits to files in an isolated LSP view, evaluate the diagnostic impact, and then commit or discard — without touching disk until explicitly requested.

### Package layout

```
internal/session/
  types.go    ← SimulationSession, SessionStatus state machine, result types
  manager.go  ← SessionManager: full session lifecycle
  executor.go ← SerializedExecutor: one active operation per session
  differ.go   ← DiffDiagnostics: baseline vs. current comparison
```

### Session state machine

```
created → mutated → evaluating → evaluated → committed
                                           ↘ discarded
                ↘ dirty (on LSP error)
```

`committed` and `discarded` are terminal states. `dirty` means the LSP state diverged from the in-memory content (e.g., `OpenDocument` failed mid-edit) and the session must be destroyed.

### Session lifecycle

```go
// 1. Create an isolated session
sessionID, _ := mgr.CreateSession(ctx, "/workspace/root", "go")

// 2. Apply one or more range edits (in-memory + LSP didChange)
mgr.ApplyEdit(ctx, sessionID, "file:///workspace/root/foo.go", rng, newText)

// 3. Evaluate: wait for diagnostics to stabilise, diff against baseline
result, _ := mgr.Evaluate(ctx, sessionID, "file", 3000)
// result.NetDelta == 0 → safe to apply

// 4a. Commit: write to disk and notify LSP
mgr.Commit(ctx, sessionID, "", true)

// 4b. Or discard: revert LSP in-memory state to original
mgr.Discard(ctx, sessionID)

// 5. Destroy: remove from manager
mgr.Destroy(ctx, sessionID)
```

### Lazy baseline

The first `ApplyEdit` call for a given file URI within a session:
1. Waits for diagnostics to stabilize (up to 3s) via `WaitForDiagnostics`
2. Snapshots the current diagnostics as the baseline
3. Reads the file content from disk into `session.Contents[uri]`
4. Stores the original content in `session.OriginalContents[uri]` (used by Discard)
5. Opens the document in the LSP client

### Atomic variant

`simulate_edit_atomic` (tool: `mcp__lsp__simulate_edit_atomic`) is a convenience wrapper that creates a session, applies one edit, evaluates, discards (to revert LSP state), and destroys — all in a single call. Useful for quick pre-flight checks before applying a real edit.

### Chained edits

`simulate_chain` applies a sequence of edits and evaluates after each step. It returns a `ChainResult` with per-step `NetDelta` values and `SafeToApplyThroughStep` — the index of the last step where `NetDelta == 0`.

### SerializedExecutor

`SerializedExecutor` ensures that only one goroutine operates on a session's LSP state at a time. `Acquire` blocks until the session is available; `Release` frees it. This prevents interleaved `didChange` / `publishDiagnostics` from different concurrent tool calls corrupting the diagnostic snapshot.

---

## File Watcher

When `start_lsp` initializes the LSP client, `startWatcher(rootDir)` is called automatically. A goroutine watches the workspace root recursively using [fsnotify](https://github.com/fsnotify/fsnotify), which uses the platform-native mechanism (`inotify` on Linux, `kqueue` on BSD/macOS, `FSEvents` on macOS for Go 1.23+). File system events are:

1. Deduplicated per path into a `map[string]fsnotify.Op` (pending set)
2. Flushed as a single `workspace/didChangeWatchedFiles` notification after a **150ms debounce** (`time.AfterFunc`)
3. LSP change type is mapped: `Create→1`, `Write→2`, `Remove|Rename→3`

**Exclusion list** (`watcherSkipDirs`):

```
.git  node_modules  target  build  dist  vendor  __pycache__  .venv  venv
```

All directories whose names start with `.` (except `.` itself) are also skipped. Dynamically-created subdirectories are added to the watcher on the `Create` event.

`stopWatcher()` closes the stop channel, triggering a final flush of any pending events before the goroutine exits. It is called during `Shutdown` and at the beginning of each `startWatcher` call to replace a stale watcher on `start_lsp` reinit.

The auto-watcher means the `did_change_watched_files` tool is not required for normal editing workflows.

---

## MCP Log Notifications (`mcpSessionSender`)

Before the MCP session is established, internal log calls write to stderr. Once the client connects and the `initialized` notification arrives, logs route through MCP `logging/message` notifications.

The bridge uses a narrow adapter:

```go
// mcpSessionSender adapts *mcp.ServerSession to the logging.logSender interface.
type mcpSessionSender struct{ ss *mcp.ServerSession }

func (s *mcpSessionSender) LogMessage(level, logger, message string) error {
    data, _ := json.Marshal(message)
    return s.ss.Log(context.Background(), &mcp.LoggingMessageParams{
        Level:  mcp.LoggingLevel(level),
        Logger: logger,
        Data:   json.RawMessage(data),
    })
}
```

`server.go` wires this in the `InitializedHandler` callback:

```go
InitializedHandler: func(_ context.Context, req *mcp.InitializedRequest) {
    logging.SetServer(&mcpSessionSender{ss: req.Session})
    logging.MarkServerInitialized()
},
```

`logging.Log` checks `serverInitialized` before attempting the MCP send. This prevents races during startup where `SetServer` has been called but the session is not yet ready to receive notifications.

**Log levels** follow the MCP spec (8 levels: `debug info notice warning error critical alert emergency`). The minimum level is configurable via `set_log_level` or the `LOG_LEVEL` environment variable. Internally the server emits `debug`, `info`, `warning`, `error`, and `critical`; the other four are accepted by `SetLevel` but never self-generated.

---

## Skills Layer

The `skills/` directory contains Agent Skills — structured directories that Claude Code loads as slash commands. Each skill is a directory containing a `SKILL.md` file in the [AgentSkills](https://github.com/anthropics/agent-skills) format:

```
skills/
  lsp-verify/
    SKILL.md     ← frontmatter (name, description, allowed-tools) + prompt body
  lsp-safe-edit/
    SKILL.md
  ...
  install.sh     ← installer: symlinks skills/ dirs into ~/.claude/skills/
```

**SKILL.md format:**

```markdown
---
name: lsp-verify
description: <one-line description for skill discovery>
allowed-tools: mcp__lsp__get_diagnostics mcp__lsp__run_build ...
---

# lsp-verify: Three-Layer Verification
...prompt body with instructions for the agent...
```

Skills are not Go code — they are prompt documents that tell Claude how to orchestrate the MCP tools exposed by this server. They exist in this repo so they ship alongside the server binary and stay in sync with the tool API.

**Installing skills:**

```bash
./skills/install.sh          # symlink all skills to ~/.claude/skills/
./skills/install.sh --copy   # copy instead of symlink
./skills/install.sh --force  # overwrite existing
```

The installer scans for `SKILL.md` files up to two levels deep, creates `~/.claude/skills/` if needed, and symlinks (or copies) each skill directory.

### Skills provided

| Skill | Purpose |
|-------|---------|
| `lsp-verify` | Three-layer verification: diagnostics + build + tests |
| `lsp-safe-edit` | Edit with before/after diagnostic diff |
| `lsp-simulate` | Speculative edit session (create/apply/evaluate/commit/discard) |
| `lsp-impact` | Blast-radius: references + call hierarchy + type hierarchy |
| `lsp-implement` | Find all concrete implementations of an interface |
| `lsp-rename` | Two-phase rename: preview all sites, then apply atomically |
| `lsp-edit-symbol` | Edit a symbol by name without knowing its file/position |
| `lsp-edit-export` | Edit exported symbols after finding all callers first |
| `lsp-dead-code` | Find exported symbols with zero references |
| `lsp-docs` | Fetch toolchain documentation (`go doc`, `pydoc`, etc.) |
| `lsp-format-code` | Format a file or range |
| `lsp-local-symbols` | List all symbols in a file |
| `lsp-cross-repo` | Navigate references across multiple repositories |
| `lsp-test-correlation` | Map source files to their test files |

---

## URI Handling

LSP uses `file://` URIs throughout. Two utilities handle the conversion:

```go
// path → URI (for sending to the LSP server)
CreateFileURI("/path/to/file.go")  // → "file:///path/to/file.go"

// URI → path (for reading results from the LSP server)
URIToFilePath("file:///path/to/file.go")  // → "/path/to/file.go"
```

Both use `url.URL` / `url.Parse` rather than string slicing. This correctly handles percent-encoded characters (e.g. spaces in paths → `%20`) and is robust to non-standard URI forms.

`ValidateFilePath` additionally rejects path traversal: if `rootDir` is non-empty, the resolved absolute path must be equal to `rootDir` or have `rootDir/` as a prefix.

---

## Resource Subscription System

Resources expose LSP data over MCP's subscribe/unsubscribe mechanism. Three resource templates are registered:

| URI Template | Description |
|---|---|
| `lsp-diagnostics:///{filePath}` | Diagnostics for a file (or all open files if path empty) |
| `lsp-hover:///{filePath}?line={line}&column={column}&language_id={language_id}` | Hover info at position |
| `lsp-completions:///{filePath}?line={line}&column={column}&language_id={language_id}` | Completions at position |

### Diagnostic subscription flow

```
client → resources/subscribe { uri: "lsp-diagnostics:///path/to/file.go" }
                                          ↓
                          resources.HandleSubscribeDiagnostics(client, uri, notify)
                                          ↓
                          client.SubscribeToDiagnostics(callback)
                              callback stored in DiagnosticUpdateCallback slice
                                          ↓
          later: LSP subprocess sends textDocument/publishDiagnostics
                                          ↓
                          LSPClient.handlePublishDiagnostics → fires all callbacks
                                          ↓
                          callback → notify(updatedURI)
                                          ↓
                          server.go → ss.Notify("notifications/resources/updated")
                                          ↓
client ← notifications/resources/updated { uri: "lsp-diagnostics:///path/to/file.go" }
                                          ↓
client → resources/read { uri: "lsp-diagnostics:///path/to/file.go" }
                                          ↓
client ← current diagnostics JSON
```

The subscription callback is stored by reference so it can be removed precisely on unsubscribe (`client.UnsubscribeFromDiagnostics(sub.Callback)`).

Two subscription scopes exist:
- **Specific file:** fires only when `updatedURI == fileURI`
- **All files:** fires for any `updatedURI` that starts with `file://`

---

## LSP Client Lifecycle

```
start_lsp (tool call)
    ↓
sm.StartAll(ctx, rootDir) or sm.StartForLanguage(ctx, rootDir, languageID)
    ↓
LSPClient.Initialize(ctx, rootDir)
    ↓
exec.Command(lspServerPath, lspServerArgs...)
    ↓  spawns subprocess; connects stdin/stdout/stderr pipes
    ↓  starts readLoop goroutine, drainStderr goroutine, exit-monitor goroutine
    ↓
SendRequest("initialize", {capabilities, rootUri, workspaceFolders})
    ↓  server may send window/workDoneProgress/create, workspace/configuration here
    ↓  these are handled in dispatch() → handleServerRequest before initialize returns
receive initialize response
    ↓  captures serverCapabilities, semantic token legend
client.initialized = true
SendNotification("initialized", {})
    ↓
startWatcher(rootDir)
    ↓
tool calls now available
```

`initialized` is set to `true` before `initialized` notification is sent (not after) to prevent a race where the server's first request arrives in the window between sending `initialized` and setting the flag.

### Request/response correlation

Each outgoing request is assigned a monotonically-increasing integer ID. A `pendingRequest` struct holding `ch chan json.RawMessage` and `err chan error` is stored in `c.pending[id]`. `readLoop` calls `dispatch()` on every incoming frame; when `dispatch` sees a response message (has `id`, no `method`), it resolves the pending channel.

Per-method timeouts are applied to each `SendRequest` call. `textDocument/references` gets 120s (full workspace indexing); `initialize` gets 300s (cold-start JVM servers).

### Crash recovery

When the LSP subprocess exits:
1. The exit-monitor goroutine calls `rejectPending(err)`, closing all open pending channels with the exit error so callers fail fast rather than waiting for timeouts
2. `initialized` is set to `false`
3. The last 4KB of stderr is logged at `error` level

---

## WaitForDiagnostics

`WaitForDiagnostics(ctx, client, targetURIs []string, timeoutMs int)` is used by `get_diagnostics`, `evaluate_session`, and resource handlers to wait for the language server to finish publishing diagnostics after a document is opened or modified.

It resolves when:

1. All target URIs have received at least one diagnostic notification *after* the initial snapshot (the first notification is excluded — it is the server's pre-existing state for that file)
2. No further diagnostic notifications arrive for **500ms** (the stabilization window)
3. OR the optional `timeoutMs` is exceeded

An empty `targetURIs` slice resolves immediately (no wait needed).

---

## LSP Response Normalization

`internal/lsp/normalize.go` centralizes handling of LSP responses that have multiple valid shapes per spec, converting them to concrete Go types before they reach tool handlers.

### `NormalizeDocumentSymbols(raw json.RawMessage) ([]types.DocumentSymbol, error)`

Converts `DocumentSymbol[] | SymbolInformation[]` to `[]types.DocumentSymbol`.

- Discriminates on the presence of `selectionRange` in the first element
- When `SymbolInformation[]` is returned, performs a three-pass tree reconstruction:
  - Pass 1: create a `DocumentSymbol` for each item, build a `name → *DocumentSymbol` map
  - Pass 2: attach children to parents via `containerName`
  - Pass 3: collect root nodes (those with no parent) by dereferencing pointers after all children are wired

### `NormalizeCompletion(raw json.RawMessage) (types.CompletionList, error)`

Converts `CompletionItem[] | CompletionList` to `types.CompletionList`. Discriminates on the presence of an `items` field.

### `NormalizeCodeActions(raw json.RawMessage) ([]types.CodeAction, error)`

Converts `(Command | CodeAction)[]` to `[]types.CodeAction`. Discriminates each element by checking whether the `command` field's first non-whitespace byte is a double-quote (bare `Command` string) or not (absent/null/object `CodeAction`). Bare commands are wrapped in a synthetic `CodeAction`.

### Why normalization exists

Before `normalize.go`, handlers received `[]interface{}` from `json.Unmarshal` and had to type-assert their way through arbitrary JSON trees. This was fragile and made the response structure opaque to callers. Concrete types give handlers compile-time safety and make the wire format explicit. The normalization is centralized rather than per-handler because the same polymorphism appears in multiple places (e.g. `get_document_symbols`, `get_symbol_source` both need `DocumentSymbol`).

---

## Extension System

Language-specific extensions are registered at compile time via `init()` functions. An extension lives at `extensions/<language-id>/` and calls `extensions.RegisterFactory`:

```go
// extensions/haskell/haskell.go
func init() {
    extensions.RegisterFactory("haskell", func() extensions.Extension {
        return &HaskellExtension{}
    })
}
```

An extension implements any subset of the `Extension` interface (defined in `internal/types/types.go`):

```go
type Extension interface {
    ToolHandlers() map[string]ToolHandler
    ResourceHandlers() map[string]ResourceHandler
    SubscriptionHandlers() map[string]ResourceHandler
    PromptHandlers() map[string]interface{}
}
```

Extensions take precedence over core handlers in case of name conflicts. All features are namespaced by language ID automatically. Unlike dynamic plugin systems, Go extensions are registered at compile time — unused extensions have zero runtime cost and there is no filesystem scan or `dlopen`.

`cmd/agent-lsp/main.go` calls `registry.Activate(languageID)` for each configured server after parsing arguments.

---

## `get_symbol_documentation`

`HandleGetSymbolDocumentation` (in `internal/tools/documentation.go`) fetches canonical documentation by shelling out to the language's own toolchain rather than going through LSP hover:

| Language | Command |
|---|---|
| Go | `go doc [pkg] Symbol` |
| Python | `python3 -m pydoc Symbol` |
| Rust | `cargo doc --no-deps --message-format short` |

For Go, `findGoMod` walks up from the file's directory to locate `go.mod` and constructs a fully-qualified package path (e.g. `github.com/foo/bar/internal/baz Symbol`) so `go doc` resolves the symbol correctly within modules.

ANSI escape codes are stripped from output. A `Signature` field is extracted from the first matching declaration line (`func`, `type`, `var`, `const` for Go; first non-empty line for Python). TypeScript and JavaScript are explicitly unsupported (LSP hover is the right tool there).

---

## `get_symbol_source`

`HandleGetSymbolSource` (in `internal/tools/symbol_source.go`) extracts the full source text of the symbol at a given cursor position:

1. Calls `client.GetDocumentSymbols` via `WithDocument` to get the normalized symbol tree
2. Walks the tree with `findInnermostSymbol` — recursively finds the deepest symbol whose `Range` contains the 0-based cursor position
3. Reads the file from disk and slices the lines corresponding to `sym.Range.Start.Line` to `sym.Range.End.Line` (0-based, inclusive)
4. Returns `SymbolSourceResult{SymbolName, SymbolKind, StartLine, EndLine, Source}` with 1-based line numbers

This is useful for agents that want to read a function body without manually counting lines.
