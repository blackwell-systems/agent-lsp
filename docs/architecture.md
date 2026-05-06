# Architecture

agent-lsp is a [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that wraps one or more [Language Server Protocol](https://microsoft.github.io/language-server-protocol/) (LSP) subprocesses. MCP is a standard for exposing tools to AI agents; LSP is a standard for code intelligence (completions, go-to-definition, diagnostics, etc.) backed by language-specific servers like `gopls` or `typescript-language-server`. Both protocols use [JSON-RPC 2.0](https://www.jsonrpc.org/specification) as their wire format. This document describes the package structure, key patterns, and internal design decisions.

---

## Key Terms

| Term | Definition |
|------|-----------|
| **Workspace root** | The top-level directory of a project (containing `go.mod`, `package.json`, `Cargo.toml`, etc.). The LSP server indexes files relative to this path. |
| **Language ID** | A short string identifying a programming language (`"go"`, `"typescript"`, `"python"`, etc.). Used when opening documents so the LSP server applies the correct grammar and analysis. |
| **Diagnostic** | An error, warning, or hint reported by the language server for a specific location in a file. Diagnostics arrive asynchronously via `textDocument/publishDiagnostics` notifications. |
| **Code action** | A suggested fix or refactor offered by the language server for a given diagnostic or cursor range (e.g. "add missing import", "extract function"). |
| **Symbol** | A named code element: function, type, variable, constant, method, interface, etc. LSP exposes symbols at document scope (`textDocument/documentSymbol`) and workspace scope (`workspace/symbol`). |
| **Workspace edit** | A structured set of text edits across one or more files, returned by LSP operations like rename or code actions. |
| **URI** | A `file://` identifier for a source file (e.g. `file:///home/user/project/main.go`). LSP uses URIs instead of bare file paths in all requests and responses. |

---

## System Overview

At runtime, agent-lsp consists of two layers of processes communicating over pipes (JSON-RPC messages framed with `Content-Length` headers, the standard LSP wire format):

```
AI agent (Claude Code, Cursor, etc.)
    │
    │  JSON-RPC over stdio (or HTTP+SSE)
    ▼
agent-lsp process  (the MCP server, one long-lived Go binary)
    │
    │  JSON-RPC over stdin/stdout pipes  (one pipe pair per language)
    ├──────────────────────────────────────────────────────────────────┐
    ▼                                                                  ▼
gopls subprocess                                         typescript-language-server subprocess
(indexes .go files)                                      (indexes .ts/.tsx files)
```

**For languages that need sustained indexing (Python, TypeScript on large repos),** a daemon broker process persists between sessions:

```
AI agent (session 1)              AI agent (session 2, minutes later)
    │                                 │
    ▼                                 ▼
agent-lsp (ephemeral)             agent-lsp (ephemeral)
    │                                 │
    │  Unix socket                    │  Unix socket (same daemon, already warm)
    ▼                                 ▼
daemon-broker (persistent, one per root+language)
    │
    │  stdin/stdout pipes
    ▼
pyright-langserver (persistent, fully indexed)
```

The daemon auto-spawns on first `start_lsp` for Python/TypeScript, indexes the workspace in the background, and serves queries to all subsequent sessions instantly. Auto-exits after 30 minutes of inactivity.

**What agent-lsp does:**

1. Speaks MCP to the AI agent, exposing 53 tools the agent can call.
2. Translates each tool call into one or more LSP JSON-RPC requests, sent over stdin/stdout pipes to the appropriate language server subprocess.
3. Maintains a persistent session: the language server index stays warm across all tool calls, all files, all packages. There is no cold-start on each request.
4. Adds a speculative execution layer on top: edits can be applied in-memory to the live LSP state, evaluated for diagnostic impact, then committed to disk or discarded, without ever touching the file system until explicitly requested.
5. Ships a skills layer: prompt documents that tell Claude how to orchestrate multi-step workflows using the tools correctly.
6. Enforces skill phase ordering at runtime: when an agent activates a skill, the phase tracker monitors tool calls and blocks out-of-order operations (e.g., writing to disk before simulating).

The binary is a single statically-linked Go executable. No Node.js runtime. No per-request process spawn.

---

## Package Structure

```
cmd/agent-lsp/
  main.go               ← CLI entrypoint; argument parsing, signal handling, panic recovery;
                           --version flag prints Version (injected by GoReleaser, falls back to "dev");
                           dispatches to runInit, runDoctor, daemon-broker, daemon-status, daemon-stop
  version.go            ← var Version = "dev"; set at build time via -ldflags="-X main.Version=x.y.z"
  doc.go                ← package-level doc comment
  init.go               ← runInit: interactive `agent-lsp init` subcommand; generates mcp.json config
  init_test.go          ← tests for init subcommand
  doctor.go             ← runDoctor: `agent-lsp doctor` subcommand; starts each configured LSP server,
                           checks capabilities, and reports which tools are supported/unsupported
  doctor_test.go        ← tests for doctor subcommand
  daemon.go             ← Daemon CLI subcommands: daemon-broker (persistent broker process),
                           daemon-status (list active daemons), daemon-stop (terminate daemons)
  server.go             ← MCP server construction; tool/resource registration; mcpSessionSender;
                           HTTP transport via --http flag (Streamable HTTP + optional Bearer token auth);
                           addToolWithPhaseCheck[T] generic wrapper for phase enforcement;
                           PhaseTracker initialization with BuiltinSkills();
                           (tool registration was extracted from server.go in a decomposition wave;
                           server.go now delegates to the five tool files below)
  helpers.go            ← shared helpers for the cmd layer (toolArgsToMap, clientForFile, autoInitClient)
  http_test.go          ← tests for HTTP transport and --http/--port/--token flag parsing
  schema_fix.go         ← fixes nullable array types in JSON Schemas for Gemini compatibility;
                           collapses Types: ["null","array"] to Type: "array"
  tools_navigation.go   ← 10 navigation tools: go_to_definition, go_to_type_definition,
                           go_to_implementation, go_to_declaration, go_to_symbol,
                           rename_symbol, prepare_rename, get_document_highlights,
                           call_hierarchy, type_hierarchy
  tools_analysis.go     ← 13 analysis tools: get_info_on_location, get_completions,
                           get_signature_help, get_code_actions, get_document_symbols,
                           get_workspace_symbols, get_references, get_inlay_hints,
                           get_semantic_tokens, get_symbol_source, get_symbol_documentation,
                           get_change_impact, get_cross_repo_references
  tools_workspace.go    ← 19 workspace/lifecycle tools: start_lsp, restart_lsp_server,
                           add_workspace_folder, remove_workspace_folder, list_workspace_folders,
                           open_document, close_document, get_diagnostics, get_server_capabilities,
                           detect_lsp_servers, run_build, run_tests, get_tests_for_file,
                           set_log_level, apply_edit, execute_command, did_change_watched_files,
                           format_document, format_range
  tools_session.go      ← 8 simulation/session tools: create_simulation_session, simulate_edit,
                           evaluate_session, simulate_chain, commit_session, discard_session,
                           destroy_session, simulate_edit_atomic
  tools_phase.go        ← 3 phase enforcement tools: activate_skill, deactivate_skill,
                           get_skill_phase; checkPhasePermission helper
  audit_helpers.go      ← Diagnostic snapshot helpers for audit trail (pre/post edit)

internal/phase/
  types.go         ← EnforcementMode, PhaseDefinition, SkillPhaseConfig, PhaseViolation, PhaseStatus
  matcher.go       ← MatchToolPattern, MatchesAny: glob matching (trailing * wildcard)
  tracker.go       ← Tracker: thread-safe state machine (activate, deactivate, check+record, status);
                     auto-advances phases based on tool call patterns
  skills.go        ← Built-in phase configs for lsp-rename (3 phases), lsp-refactor (5 phases),
                     lsp-safe-edit (4 phases), lsp-verify (5 phases)

internal/config/
  config.go        ← ServerEntry + Config types for multi-server JSON config
  parse.go         ← Argument parsing (single-server, multi-server, --config, auto-detect)
  infer.go         ← InferWorkspaceRoot: walks up from a file to find go.mod/package.json/etc.
  autodetect.go    ← AutodetectServers: scans PATH for known language server binaries

internal/audit/
  audit.go         ← Logger: buffered JSONL audit trail writer; Record types; ResolvePath
                     (--audit-log flag → AGENT_LSP_AUDIT_LOG env → ~/.agent-lsp/audit.jsonl)
  audit_test.go    ← tests for audit logger

internal/httpauth/
  auth.go          ← BearerTokenMiddleware: HTTP middleware enforcing Bearer token authentication
                     for --http mode; constant-time comparison via crypto/subtle
  auth_test.go     ← tests for Bearer token middleware

internal/lsp/
  client.go        ← LSPClient: subprocess lifecycle, JSON-RPC framing, request/response
                     correlation, server-initiated requests, file watcher;
                     NewDaemonClient: socket-connected client for daemon mode
  manager.go       ← ServerManager: multi-server registry, ClientForFile routing by extension;
                     startOrConnectDaemon: transparent daemon lifecycle for Python/TypeScript
  resolver.go      ← ClientResolver interface
  framing.go       ← Content-Length framing (FrameReader / FrameWriter)
  diagnostics.go   ← WaitForDiagnostics: stabilization wait with timeout
  normalize.go     ← NormalizeDocumentSymbols, NormalizeCompletion, NormalizeCodeActions
  daemon.go        ← DaemonInfo, NeedsDaemon, FindRunningDaemon, WriteDaemonInfo,
                     ListDaemons, StopDaemon, CleanupStaleDaemons
  daemon_broker.go ← RunBroker: persistent subprocess that owns the language server,
                     listens on a Unix socket, proxies JSON-RPC, tracks readiness,
                     auto-exits after 30 min inactivity
  scope.go         ← GenerateScopeConfig: generates pyrightconfig.json / tsconfig.json
                     to limit workspace indexing scope for large repos
  warmup.go        ← Multi-signal readiness gate for daemon clients: diagnostics,
                     hover canary, adaptive first-query timeout

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
  change_impact.go ← get_change_impact (enumerate exported symbols, resolve references, partition test/non-test callers)
  cross_repo.go    ← get_cross_repo_references (add consumer repos as workspace folders, partition references by repo)
  workspace.go     ← workspace folder management (add/remove/list)
  workspace_folders.go ← add_workspace_folder, remove_workspace_folder, list_workspace_folders
  session.go       ← start_lsp, open_document, close_document, restart_lsp_server
  utilities.go     ← apply_edit, execute_command, did_change_watched_files, set_log_level,
                     format_document, format_range, rename_symbol, prepare_rename
  fuzzy.go         ← fuzzy matching utilities for workspace symbol lookup
  position_pattern.go ← position_pattern argument handling (e.g. "func Foo"); LineScope
                       (line_scope_start/line_scope_end) for disambiguating duplicate matches
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

internal/uri/
  uri.go           ← URIToPath: RFC 3986-correct file:// URI → path conversion (url.Parse-based);
                     ApplyRangeEdit: canonical in-memory range edit shared by lsp and session packages

internal/logging/
  logging.go       ← Log, SetServer, SetLevel, SetLevelFromEnv, MarkServerInitialized;
                     MCP notification bridge; SetLevelFromEnv called explicitly from main()
                     (init() is a no-op; no init-time side effects)

internal/extensions/
  registry.go      ← ExtensionRegistry; Activate, RegisterFactory, GetToolHandlers, etc.

pkg/
  lsp/
    lsp.go         ← type aliases re-exporting internal/lsp types (LSPClient, ServerManager,
                     ClientResolver, Position, etc.)
    lsp_test.go    ← smoke tests verifying alias targets are non-nil
    doc.go         ← package-level doc comment
  session/
    session.go     ← type aliases re-exporting internal/session types (SessionManager,
                     SimulationSession, SessionStatus, etc.)
    session_test.go ← smoke tests verifying alias targets are non-nil
    doc.go         ← package-level doc comment
  types/
    doc.go         ← package-level doc comment + all 32 type aliases, 5 constants, 2 constructor vars
    types_test.go  ← smoke tests verifying alias targets are non-nil

Nine internal packages (`lsp`, `session`, `tools`, `resources`, `types`, `uri`,
`logging`, `extensions`, `phase`) have a `doc.go` with a package-level doc comment.
`internal/config`, `internal/audit`, and `internal/httpauth` use inline file-level comments instead.

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
  lsp-explore/     ← Symbol exploration: hover + implementations + call hierarchy + references
  lsp-understand/  ← Deep codebase understanding and navigation
  lsp-refactor/    ← Multi-step refactoring workflows
  lsp-extract-function/ ← Extract code into a new function
  lsp-fix-all/     ← Fix all diagnostics in a file or workspace
  lsp-generate/    ← Generate code with LSP-aware validation

experiments/token-savings/    Reproducible benchmark: LSP vs grep/read token cost
```

---

## Public API (pkg/)

Three packages under `pkg/` expose a stable, importable, pkg.go.dev-indexed
public API without requiring callers to run the MCP server:

| Package | Import path | What it provides |
|---------|-------------|-----------------|
| `pkg/types` | `github.com/blackwell-systems/agent-lsp/pkg/types` | All LSP wire types, symbol types, tool response envelope |
| `pkg/lsp` | `github.com/blackwell-systems/agent-lsp/pkg/lsp` | `LSPClient`, `ServerManager`, `ClientResolver` interface |
| `pkg/session` | `github.com/blackwell-systems/agent-lsp/pkg/session` | `SessionManager`, session lifecycle types, speculative execution API |

Every type in `pkg/` is a **type alias** of the corresponding `internal/`
type. This means values are interchangeable: a `pkg/types.Position` can be
passed to any function expecting `internal/types.Position` without conversion.

The `pkg/` packages contain no logic; they are purely re-export layers. All
implementation lives in `internal/`. This design keeps the public surface
minimal and allows the internal implementation to evolve without breaking
external callers as long as the alias targets are preserved.

Each `pkg/` package includes a smoke test (`lsp_test.go`, `session_test.go`,
`types_test.go`) that verifies alias targets are non-nil at compile time. This
ensures the re-export layer stays consistent with `internal/` as the
implementation evolves.

### Layer rules

- `cmd/agent-lsp/` owns the MCP server lifecycle and routes requests to handlers via the five tool files
- `internal/tools/` and `internal/resources/` import from `internal/lsp/`, `internal/session/`, and `internal/types/`. They do not import from each other
- `internal/lsp/` imports from `internal/types/`, `internal/logging/`, and `internal/uri/` (no upward dependencies)
- `internal/session/` imports from `internal/lsp/`, `internal/types/`, `internal/logging/`, and `internal/uri/`
- `internal/uri/` imports only from `internal/types/`, serving as the canonical URI/path conversion layer
- `internal/extensions/` imports from `internal/types/` only
- `extensions/<language>/` (when present) imports from `internal/tools/` for re-exported utilities

---

## Process Model

Understanding the process model is the most important prerequisite for reading the rest of this document.

**MCP server process (agent-lsp binary):**

- One process, long-lived, started once by the AI client.
- Communicates with the AI via JSON-RPC over **stdio** (default) or **HTTP+SSE** (`--http` flag).
- Owns all Go code in this repo: MCP server, LSP clients, session manager, tool handlers.

**LSP subprocess(es):**

- One subprocess per configured language server (e.g. `gopls`, `typescript-language-server`).
- Spawned by `LSPClient.Initialize` via `exec.Command`. Each subprocess gets its own `stdin`/`stdout` pipe pair.
- Communicate with the Go process using LSP JSON-RPC with **Content-Length framing** (each message is preceded by a `Content-Length: N\r\n\r\n` header giving the byte length of the JSON body; this is the standard LSP wire format, not HTTP).
- Remain running for the lifetime of the MCP session. The index stays warm; no subprocess is spawned per tool call.

**Daemon broker process (Python/TypeScript only):**

Language servers like pyright and tsserver need minutes of background indexing before `textDocument/references` works. In direct mode, agent-lsp spawns a fresh language server subprocess per MCP session. When the session ends, the subprocess dies, and its entire workspace index is lost. The next session spawns a fresh subprocess that starts indexing from scratch. For gopls this is fine (Go modules index in seconds), but for pyright on a 1,000-file Python repo, indexing takes minutes. Every session pays the full cost, and reference queries time out because the server is never ready in time.

The daemon broker solves this by decoupling the language server's lifetime from the MCP session's lifetime. The broker is a separate process (`agent-lsp daemon-broker`) that:

1. **Spawns the language server once** as its own child process (e.g. `pyright-langserver --stdio`), communicating via stdin/stdout pipes.
2. **Performs LSP `initialize`** with the workspace root, then waits for the server to finish indexing.
3. **Listens on a Unix domain socket** at `~/.cache/agent-lsp/daemons/<hash>/daemon.sock`.
4. **Accepts connections from agent-lsp sessions.** Each connection is a JSON-RPC channel using Content-Length framing (same wire format as LSP stdio). The broker reads requests from the socket, forwards them to the language server's stdin, reads responses from the language server's stdout, and writes them back to the socket.
5. **Tracks readiness.** Once the language server finishes indexing, the broker writes `"ready": true` to `daemon.json`. Agent-lsp clients check this flag before issuing reference queries.
6. **Manages its own lifetime.** When the last socket client disconnects, the broker starts a 30-minute inactivity timer. If no new client connects, the broker sends `shutdown` + `exit` to the language server, removes its socket and state files, and exits. If a new client connects, the timer resets.

The key insight: the broker process is detached from the agent-lsp process (`Setsid: true` on the subprocess). When the MCP session ends and agent-lsp exits, the broker and its language server keep running. The next `start_lsp` call finds the existing broker via `FindRunningDaemon` (checks PID liveness + socket reachability), connects to the socket, and gets an already-indexed language server instantly.

- One broker per (rootDir, languageID) pair. The directory hash ensures different workspaces get different daemons.
- State files live at `~/.cache/agent-lsp/daemons/<hash>/`: `daemon.json` (metadata), `daemon.sock` (Unix socket), `daemon.pid` (process ID).
- Go, Rust, C, and other fast-indexing servers bypass daemon mode entirely. `NeedsDaemon()` returns false for these languages, so the direct subprocess model is used with zero overhead.

**Communication direction (direct mode, e.g. Go/Rust):**

```
AI agent ──MCP JSON-RPC──► agent-lsp ──LSP JSON-RPC──► gopls subprocess
AI agent ◄──MCP JSON-RPC── agent-lsp ◄──LSP JSON-RPC── gopls subprocess
```

**Communication direction (daemon mode, e.g. Python/TypeScript):**

```
AI agent ──MCP JSON-RPC──► agent-lsp ──JSON-RPC──► daemon-broker ──LSP JSON-RPC──► pyright
                            (Unix socket)              (stdin/stdout pipes)
```

In direct mode, the Go process communicates with language servers through `os/exec` pipe pairs. In daemon mode, agent-lsp connects to the broker via a Unix domain socket; the broker owns the pipe pair to the language server.

---

## HTTP Transport Mode

By default, agent-lsp communicates with the AI client over stdio. The `--http` flag switches to an HTTP+SSE transport using the MCP SDK's `StreamableHTTPHandler`, suitable for containerized deployments and remote access.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--http` | off | Enable HTTP transport instead of stdio |
| `--port <N>` | `8080` | TCP port to listen on (1--65535) |
| `--listen-addr <IP>` | `127.0.0.1` | Bind address; must be a valid IP |
| `--token <S>` | (none) | Bearer token for authentication (prefer `AGENT_LSP_TOKEN` env var to avoid process-list exposure) |
| `--no-auth` | off | Run without authentication; only permitted on loopback addresses |

### Authentication

When a token is configured (via `AGENT_LSP_TOKEN` env var or `--token` flag), every request except `/health` must include an `Authorization: Bearer <token>` header. The comparison uses `crypto/subtle.ConstantTimeCompare` to prevent timing side-channels. Mismatched tokens receive HTTP 401 with `{"error":"unauthorized"}`.

When no token is configured and `--no-auth` is not set, the server refuses to start. `--no-auth` is only permitted with a loopback bind address (`127.0.0.1`).

### Endpoints

| Path | Auth | Description |
|------|------|-------------|
| `/` | Bearer token | MCP Streamable HTTP endpoint (JSON-RPC + SSE) |
| `/health` | None | Health check; returns `{"status":"ok"}` (for container orchestration probes) |

### Timeouts and limits

| Parameter | Value |
|-----------|-------|
| Read header timeout | 10s |
| Read timeout | 30s |
| Write timeout | 60s |
| Idle timeout | 120s |
| Max request body | 4 MB |

Security headers (`X-Content-Type-Options: nosniff`, `Cache-Control: no-store`) are applied to all responses.

### Graceful shutdown

When the context is cancelled (signal received), the HTTP server calls `Shutdown` with a 5-second deadline, draining in-flight requests before exiting.

---

## Tool Registration Model

53 MCP tools are exposed to the AI agent. In MCP, a "tool" is a named function with a JSON Schema for its arguments that the AI can invoke via a JSON-RPC `tools/call` request. Tools are defined in five files under `cmd/agent-lsp/` and dispatched through a shared pattern.

### How a tool is defined

Each tool is registered via `mcp.AddTool` with three arguments:

1. A `*mcp.Tool` schema: name, description, and MCP annotations (hints like `ReadOnlyHint` and `DestructiveHint` that tell the AI client whether the tool modifies state)
2. A typed args struct: Go struct with JSON tags and `jsonschema` annotations that generate the tool's JSON Schema for the AI
3. A handler closure: receives the parsed args, calls an `internal/tools` handler, and converts the result to `*mcp.CallToolResult`

```go
mcp.AddTool(d.server, &mcp.Tool{
    Name:        "go_to_definition",
    Description: "...",
    Annotations: &mcp.ToolAnnotations{
        Title:           "Go to Definition",
        ReadOnlyHint:    true,
        DestructiveHint: boolPtr(false),
    },
}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToDefinitionArgs) (*mcp.CallToolResult, any, error) {
    r, err := tools.HandleGoToDefinition(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
    return makeCallToolResult(r), nil, err
})
```

### The five registration files

| File | Tools registered | Count |
|------|-----------------|-------|
| `tools_workspace.go` | Session lifecycle, build/test, workspace management | 19 |
| `tools_navigation.go` | go_to_definition, references, call hierarchy, rename | 10 |
| `tools_analysis.go` | hover, diagnostics, completions, symbols, change impact | 13 |
| `tools_session.go` | Speculative execution (simulate, evaluate, commit) | 8 |
| `tools_phase.go` | Phase enforcement (activate_skill, deactivate_skill, get_skill_phase) | 3 |

All five registration functions are called from `Run()` in `server.go` via the `toolDeps` bundle, which carries shared dependencies: the MCP server, the client resolver, the session manager, the phase tracker, and the `clientForFileWithAutoInit` closure. The 50 non-phase tools are wrapped via `addToolWithPhaseCheck` (generic wrapper that checks phase permissions before each handler). The 3 phase tools use raw `mcp.AddTool` to avoid circular enforcement.

### Handler separation

Tool registration (`cmd/agent-lsp/tools_*.go`) is separate from tool implementation (`internal/tools/*.go`). The `cmd/` layer owns schema definitions, MCP plumbing, and args-to-map conversion. The `internal/tools/` layer owns the actual LSP interaction logic, knows nothing about MCP, and is testable independently.

---

## Concurrency Model

agent-lsp is a concurrent Go program. A single process manages multiple long-lived goroutines, channels, and synchronization primitives to handle parallel LSP communication, file watching, diagnostic tracking, and speculative execution, all without blocking the MCP request path.

### Goroutine Architecture

At steady state, agent-lsp runs the following goroutines per LSP subprocess:

```
                        ┌──────────────────────────────────────────────┐
                        │          agent-lsp process (Go)              │
                        │                                              │
  MCP tool call ───────►│  main goroutine (MCP server dispatch)        │
                        │      │                                       │
                        │      ├─► tool handler goroutine (per call)   │
                        │      │       │                               │
                        │      │       ▼                               │
                        │      │   sendRequest ──► pendingRequest{ch}  │
                        │      │       │              ▲                │
                        │      │       │ blocks       │ unblocks      │
                        │      │       ▼              │                │
                        │  ┌───┴───────────────────────┴──────────┐    │
                        │  │  Per-LSP-client goroutines            │    │
                        │  │                                       │    │
                        │  │  readLoop ◄──── stdout pipe ◄── gopls │    │
                        │  │    │  parses JSON-RPC frames          │    │
                        │  │    │  dispatches responses → pending   │    │
                        │  │    │  dispatches notifications →       │    │
                        │  │    │    diagnostics / progress         │    │
                        │  │                                       │    │
                        │  │  drainStderr ◄── stderr pipe          │    │
                        │  │    buffers last 4KB for crash reports  │    │
                        │  │                                       │    │
                        │  │  exit-monitor                         │    │
                        │  │    calls rejectPending on crash        │    │
                        │  │                                       │    │
                        │  │  file watcher (fsnotify)              │    │
                        │  │    debounce 150ms → didChangeWatched  │    │
                        │  └───────────────────────────────────────┘    │
                        │                                              │
                        │  audit writeLoop ◄── chan Record (buffered)  │
                        │    non-blocking JSONL writer                  │
                        │                                              │
                        └──────────────────────────────────────────────┘
```

In multi-server mode, the readLoop/drainStderr/exit-monitor/watcher set is duplicated per language server subprocess. All goroutines are supervised: panics in `readLoop` and `startWatcher` are caught by `defer recover()`, logged with stack traces, and the server stays alive.

### Channel Patterns

agent-lsp uses four distinct channel patterns for different coordination needs:

**1. Request/Response Correlation (one-shot channels)**

Every outgoing LSP request gets a unique ID and a pair of buffered channels:

```go
type pendingRequest struct {
    ch  chan json.RawMessage  // buffered(1): response payload
    err chan error            // buffered(1): error (timeout, crash)
}
```

`sendRequest` creates the channels, stores them in `pending[id]`, writes the JSON-RPC frame to stdin, then blocks on a four-arm `select`: response received (`<-ch`), error received (`<-errCh`), per-method timeout via `time.NewTimer` (`<-timer.C`), or context cancellation (`<-ctx.Done()`). When `readLoop` parses a response with that ID, it sends on `ch` and the caller unblocks. This gives O(1) dispatch with no polling.

**2. Per-Session Semaphore (channel as mutex)**

The `SerializedExecutor` uses a `chan struct{}` with buffer size 1 as a per-session mutex:

```go
// Independent sessions never block each other.
// Only concurrent operations on the SAME session serialize.
sessionLocks map[string]chan struct{}  // session ID → semaphore

func (e *SerializedExecutor) Acquire(ctx context.Context, s *SimulationSession) error {
    ch := e.lockFor(s)
    select {
    case ch <- struct{}{}:  // acquired
        return nil
    case <-ctx.Done():      // cancelled
        return ctx.Err()
    }
}
```

This is more flexible than `sync.Mutex` because it respects context cancellation. A tool call that times out releases the caller immediately rather than deadlocking.

**3. Non-Blocking Audit Logger (buffered producer/consumer)**

The audit trail uses a buffered channel as a non-blocking queue between tool handlers (producers) and the disk writer (consumer):

```go
type Logger struct {
    ch   chan Record     // buffered (256 in practice): tool handlers send here
    file *os.File
    done chan struct{}   // closed when writeLoop exits
    noop bool           // true when no audit path is configured (discard all records)
}

// Tool handler (hot path) — never blocks
func (l *Logger) Log(r Record) {
    if l.noop { return }
    select {
    case l.ch <- r:    // enqueued
    default:           // channel full — log warning, drop record (non-blocking guarantee)
    }
}

// Background goroutine — drains to disk as JSONL
func (l *Logger) writeLoop() {
    defer close(l.done)
    enc := json.NewEncoder(l.file)
    for r := range l.ch { enc.Encode(r) }
}
```

Tool handlers have zero-latency audit logging; they never wait for disk I/O. The `done` channel provides a clean shutdown signal: `Close()` closes `ch`, waits on `<-done`, then closes the file. When no audit path is configured (empty `--audit-log` flag and no `AGENT_LSP_AUDIT_LOG` env var), `NewLogger` returns a no-op logger that discards all records with zero overhead.

**4. Progress Token Coordination (sync.Cond)**

Language servers report long-running work (like indexing a workspace) via `$/progress` notifications, each tagged with a token. agent-lsp tracks active tokens to know when the server is ready. Workspace readiness tracking uses `sync.Cond` instead of channels because the signal is level-triggered (not edge-triggered): any number of goroutines may be waiting, and they should all wake when the condition becomes true.

```go
progressMu     sync.Mutex
progressTokens map[interface{}]struct{} // active begin tokens
progressCond   *sync.Cond              // signalled when progressTokens becomes empty

// waitForWorkspaceReady blocks until all $/progress tokens complete or 60s elapses.
// WaitForWorkspaceReadyTimeout(ctx, timeout) is the variant with a configurable deadline.
func (c *LSPClient) waitForWorkspaceReady(ctx context.Context) {
    c.WaitForWorkspaceReadyTimeout(ctx, 60*time.Second)
}
```

When `readLoop` dispatches a `$/progress end` notification, it removes the token and calls `progressCond.Broadcast()`. All waiters (potentially multiple concurrent `start_lsp` or `get_references` calls) wake up and re-check.

### Concurrency Safety Summary

| Resource | Protection | Why not the other |
|---|---|---|
| `pending` request map | `sync.Mutex` | Simple map guard; no context-aware blocking needed |
| Per-session LSP state | Channel semaphore | Must respect `ctx.Done()` for timeout; `sync.Mutex` would deadlock |
| Progress tokens | `sync.Cond` | Multiple waiters need broadcast wake; channels are one-shot |
| Diagnostic callbacks | `sync.RWMutex` on slice | Write-lock to subscribe/unsubscribe; read-lock to iterate during publish |
| Session manager map | `sync.RWMutex` | Reads (evaluate) vastly outnumber writes (create/destroy) |
| Audit log | Buffered channel | Non-blocking producer guarantee; no mutex contention on hot path |
| File watcher state | `sync.Mutex` | Guards `watcherStop` channel to prevent data race on reinit |

### Process Orchestration on Crash

When a language server subprocess crashes, the recovery sequence cascades across goroutines:

```
gopls exits unexpectedly
    │
    ▼
exit-monitor goroutine detects cmd.Wait() error
    │
    ├──► rejectPending(err)
    │       iterates all pending[id] channels
    │       sends error on each errCh
    │       clears the pending map
    │       → all blocked sendRequest callers unblock with error
    │
    ├──► sets initialized = false
    │       → subsequent tool calls fail fast with "not initialized"
    │
    └──► logs last 4KB of stderr at error level
            → crash diagnostics visible in MCP log stream
```

No goroutine leaks. No orphaned subprocesses. All callers fail fast rather than hanging until timeout.

---

## Audit Trail

The audit subsystem (`internal/audit/`) writes a structured log of every tool invocation as a JSONL file, providing a complete record of what the agent did, when, and whether it succeeded.

### Configuration

The audit log path is resolved in priority order:

1. `--audit-log <path>` flag
2. `AGENT_LSP_AUDIT_LOG` environment variable
3. `~/.agent-lsp/audit.jsonl` (default)

If the resolved path is empty (no flag, no env var, and `$HOME` is unset), the logger runs in no-op mode: all `Log` calls are discarded with zero overhead.

### Record structure

Each JSONL line is a `Record` with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | ISO 8601 timestamp of the tool invocation |
| `tool` | string | MCP tool name (e.g. `"go_to_definition"`, `"simulate_edit_atomic"`) |
| `session_id` | string | Speculative session ID, if applicable |
| `files` | string[] | File paths involved in the operation |
| `edit_summary` | object | For mutating tools: mode, file path, old/new text preview, rename details |
| `diagnostics_before` | object | Error/warning counts and files checked before the operation |
| `diagnostics_after` | object | Error/warning counts and files checked after the operation |
| `net_delta` | object | Change in error and warning counts |
| `success` | bool | Whether the tool call succeeded |
| `error_message` | string | Error description on failure |
| `duration_ms` | int | Wall-clock duration of the tool call in milliseconds |

### Concurrency design

The logger uses a buffered channel (capacity 256) as a non-blocking queue between tool handlers (producers) and a single background goroutine (consumer) that encodes records to disk. Tool handlers never block on disk I/O. If the buffer fills (sustained burst faster than disk throughput), records are dropped with a warning rather than blocking the hot path. See the [Concurrency Model](#concurrency-model) section for the channel implementation details.

### Shutdown

`Close()` closes the channel, waits for the background goroutine to drain all remaining records via the `done` channel, then closes the file. The parent directory is created automatically (`os.MkdirAll`) on first open.

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
agent-lsp --config /path/to/agent-lsp.json

# Auto-detect: scans PATH for known language server binaries
agent-lsp
```

### Config file format

The `--config` flag accepts a JSON file with a single `servers` array. Each entry specifies the file extensions it handles, the command to launch, and an optional language ID (inferred from the first extension when omitted):

```json
{
  "servers": [
    {
      "extensions": ["go"],
      "command": ["gopls"],
      "language_id": "go"
    },
    {
      "extensions": ["ts", "tsx", "js", "jsx"],
      "command": ["typescript-language-server", "--stdio"],
      "language_id": "typescript"
    },
    {
      "extensions": ["py"],
      "command": ["pylsp"]
    }
  ]
}
```

`extensions` values are without the leading dot. `command` is `[binary, arg1, arg2, ...]`, matching the `exec.Command` calling convention.

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
3. Calls `client.OpenDocument(ctx, fileURI, content, languageID)`, which sends `textDocument/didOpen` (LSP notification telling the server to start tracking a file) if the file is new, or `textDocument/didChange` (LSP notification with updated content) if already tracked
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

The speculative execution layer lets callers apply edits to files in an isolated LSP view, evaluate the diagnostic impact, and then commit or discard, without touching disk until explicitly requested.

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
created → mutated → evaluating → evaluated → committed → destroyed
                                           ↘ discarded → destroyed
                ↘ dirty (on LSP error)     → destroyed
```

`committed`, `discarded`, `destroyed`, and `dirty` are terminal states. `dirty` means the LSP state diverged from the in-memory content (e.g., `OpenDocument` failed mid-edit) and the session must be destroyed. `destroyed` is the final state after `Destroy()` removes the session from the manager.

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

`simulate_edit_atomic` (tool: `mcp__lsp__simulate_edit_atomic`) is a convenience wrapper that creates a session, applies one edit, evaluates, discards (to revert LSP state), and destroys, all in a single call. Useful for quick pre-flight checks before applying a real edit.

### Chained edits

`simulate_chain` applies a sequence of edits and evaluates after each step. It returns a `ChainResult` with per-step `NetDelta` values and `SafeToApplyThroughStep`, the index of the last step where `NetDelta == 0`.

### Sequence diagram

A full speculative execution cycle, showing how the session layer interacts with the LSP client and file system:

```
Agent                    SessionManager              LSP Client              Disk
  │                           │                          │                     │
  │  create_simulation_session│                          │                     │
  │──────────────────────────►│                          │                     │
  │          session_id       │                          │                     │
  │◄──────────────────────────│                          │                     │
  │                           │                          │                     │
  │  simulate_edit(file, range, text)                    │                     │
  │──────────────────────────►│                          │                     │
  │                           │  (first edit for file)   │                     │
  │                           │  WaitForDiagnostics      │                     │
  │                           │─────────────────────────►│                     │
  │                           │  baseline snapshot       │                     │
  │                           │◄─────────────────────────│                     │
  │                           │                          │                     │
  │                           │  read file content       │                     │  
  │                           │────────────────────────────────────────────────►│
  │                           │  original content        │                     │
  │                           │◄────────────────────────────────────────────────│
  │                           │                          │                     │
  │                           │  apply edit in memory    │                     │
  │                           │  didChange (new content) │                     │
  │                           │─────────────────────────►│                     │
  │        edit_applied       │                          │                     │
  │◄──────────────────────────│                          │                     │
  │                           │                          │                     │
  │  evaluate_session         │                          │                     │
  │──────────────────────────►│                          │                     │
  │                           │  WaitForDiagnostics      │                     │
  │                           │─────────────────────────►│                     │
  │                           │  current diagnostics     │                     │
  │                           │◄─────────────────────────│                     │
  │                           │                          │                     │
  │                           │  DiffDiagnostics         │                     │
  │                           │  (baseline vs current)   │                     │
  │   { net_delta, errors_introduced, confidence }       │                     │
  │◄──────────────────────────│                          │                     │
  │                           │                          │                     │
  │  ┌─── if net_delta == 0 ──┐                          │                     │
  │  │                        │                          │                     │
  │  │  commit_session(apply: true)                      │                     │
  │  │───────────────────────►│                          │                     │
  │  │                        │  write files             │                     │
  │  │                        │────────────────────────────────────────────────►│
  │  │                        │  didChange (disk content)│                     │
  │  │                        │─────────────────────────►│                     │
  │  │  { files_written }     │                          │                     │
  │  │◄──────────────────────-│                          │                     │
  │  │                        │                          │                     │
  │  └─── if net_delta > 0 ───┐                          │                     │
  │  │                        │                          │                     │
  │  │  discard_session       │                          │                     │
  │  │───────────────────────►│                          │                     │
  │  │                        │  revert to original      │                     │
  │  │                        │  didChange (orig content)│                     │
  │  │                        │─────────────────────────►│                     │
  │  │  { discarded }         │                          │                     │
  │  │◄──────────────────────-│                          │       (untouched)   │
  │  └────────────────────────┘                          │                     │
  │                           │                          │                     │
  │  destroy_session          │                          │                     │
  │──────────────────────────►│                          │                     │
  │        { destroyed }      │                          │                     │
  │◄──────────────────────────│                          │                     │
```

Key points from the diagram:
- **Disk is never touched until `commit_session(apply: true)`**. The discard path leaves the file system unchanged.
- **Lazy baseline**: the first `simulate_edit` for a file triggers a `WaitForDiagnostics` snapshot and file read. Subsequent edits to the same file within the session skip this.
- **Two `WaitForDiagnostics` calls**: one for the baseline (before edit), one for evaluation (after edit). The diff between these two snapshots produces `net_delta`.
- **`didChange` notifications** keep the LSP server in sync with in-memory state at every step, including revert on discard.

### SerializedExecutor

`SerializedExecutor` ensures that only one goroutine operates on a session's LSP state at a time. `Acquire` blocks until the session is available; `Release` frees it. This prevents interleaved `didChange` / `publishDiagnostics` from different concurrent tool calls corrupting the diagnostic snapshot.

---

## Error Handling

Errors in agent-lsp propagate through three layers. Understanding this flow is important for agents interpreting tool responses and for contributors debugging issues.

### Layer 1: LSP errors

The language server may return JSON-RPC error responses (e.g., method not found, invalid params) or crash entirely.

| Scenario | Behavior |
|---|---|
| JSON-RPC error `-32601` (MethodNotFound) | Warning logged; tool returns `IsError: true` with the error message |
| JSON-RPC error `-32002` (ServerNotInitialized) | Warning logged; usually means `start_lsp` was not called |
| Subprocess crash | Exit-monitor goroutine calls `rejectPending`; all pending tool calls receive the exit error; `initialized` set to false |
| Request timeout | Context deadline exceeded; tool returns `IsError: true` |

### Layer 2: Tool handler errors

Each tool handler in `internal/tools/` returns a `types.ToolResult`. Two constructors control what the agent sees:

```go
// Success: agent receives the JSON payload
types.TextResult(`{"references": [...]}`)

// Error: agent receives IsError=true with the message
types.ErrorResult("LSP client not initialized; call start_lsp first")
```

Common error patterns in tool handlers:

| Check | When | Error message |
|---|---|---|
| `CheckInitialized(client)` | Every tool that needs an LSP session | "LSP client not initialized; call start_lsp first" |
| `ValidateFilePath(path, rootDir)` | Any tool with a `file_path` param | "path traversal not allowed" or "file not found" |
| Line/column validation | Navigation and analysis tools | "line must be >= 1" / "column must be >= 1" |
| Capability check | Before calling unsupported LSP methods | "server does not support [method]" |

### Layer 3: MCP response

`makeCallToolResult` in `server.go` converts `types.ToolResult` to `*mcp.CallToolResult`. The conversion preserves the `IsError` flag, so the AI agent sees:

```json
{
  "content": [{ "type": "text", "text": "LSP client not initialized; call start_lsp first" }],
  "isError": true
}
```

When `IsError` is `true`, well-behaved agents treat this as a recoverable error and adjust (e.g., calling `start_lsp` first). When `IsError` is `false`, the `content` text is the tool's JSON response payload.

If the Go handler itself returns a non-nil `error` (third return value from the handler closure), the MCP framework converts it to an internal error response before it reaches the agent. This path is reserved for unexpected panics or serialization failures, not normal tool errors.

---

## File Watcher

When `start_lsp` initializes the LSP client, `startWatcher(rootDir)` is called automatically. A goroutine watches the workspace root recursively using [fsnotify](https://github.com/fsnotify/fsnotify) (a cross-platform Go file-system notification library), which uses the platform-native mechanism (`inotify` on Linux, `kqueue` on BSD/macOS, `FSEvents` on macOS for Go 1.23+). File system events are:

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

**Key distinction:** Skills are not Go code and are not part of the compiled binary. They are prompt documents, Markdown files that tell Claude how to orchestrate the MCP tools in the correct multi-step sequence. They live in the `skills/` directory of this repo and are installed separately into the AI client's skill directory (`~/.claude/skills/`).

The boundary is clear:

```
Go binary (agent-lsp)         skills/ directory
─────────────────────         ──────────────────
Exposes 53 MCP tools          Encodes correct tool sequences
Handles one tool call         Handles multi-step workflows
Knows nothing about skills    Knows exactly which tools to call
```

The `skills/` directory contains Agent Skills, structured directories that Claude Code loads as slash commands. Each skill is a directory containing a `SKILL.md` file in the [AgentSkills](https://github.com/anthropics/agent-skills) format:

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

Skills are not Go code. They are prompt documents that tell Claude how to orchestrate the MCP tools exposed by this server. They exist in this repo so they ship alongside the server binary and stay in sync with the tool API.

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
| `lsp-safe-edit` | Edit with before/after diagnostic diff; `simulate_chain` refactor preview before disk write |
| `lsp-simulate` | Speculative edit session (create/apply/evaluate/commit/discard) |
| `lsp-impact` | Blast-radius: file-level `get_change_impact` entry + references + call hierarchy + type hierarchy |
| `lsp-implement` | Find all concrete implementations of an interface |
| `lsp-rename` | Two-phase rename: preview all sites, then apply atomically |
| `lsp-edit-symbol` | Edit a symbol by name without knowing its file/position |
| `lsp-edit-export` | Edit exported symbols after finding all callers first |
| `lsp-dead-code` | Find exported symbols with zero references |
| `lsp-docs` | Fetch toolchain documentation (`go doc`, `pydoc`, etc.) |
| `lsp-format-code` | Format a file or range |
| `lsp-local-symbols` | List all symbols in a file |
| `lsp-cross-repo` | Navigate references across multiple repositories via `get_cross_repo_references` |
| `lsp-test-correlation` | Map source files to their test files |
| `lsp-explore` | Symbol exploration: hover + implementations + call hierarchy + references in one pass |
| `lsp-understand` | Deep codebase understanding and navigation |
| `lsp-refactor` | Multi-step refactoring workflows |
| `lsp-extract-function` | Extract code into a new function |
| `lsp-fix-all` | Fix all diagnostics in a file or workspace |
| `lsp-generate` | Generate code with LSP-aware validation |

---

## URI Handling

LSP uses `file://` URIs throughout. URI conversion is handled by two layers:

**`internal/uri` package (canonical, shared by `internal/lsp` and `internal/session`):**

```go
// URI → path — RFC 3986-correct, url.Parse-based, percent-decoded
uri.URIToPath("file:///path/to/file.go")  // → "/path/to/file.go"
uri.URIToPath("file:///path/to/foo%20bar") // → "/path/to/foo bar"
```

**`internal/tools/helpers.go` (used by tool handlers):**

```go
// path → URI (for sending to the LSP server)
CreateFileURI("/path/to/file.go")  // → "file:///path/to/file.go"

// URI → path (for reading results from the LSP server)
URIToFilePath("file:///path/to/file.go")  // → "/path/to/file.go"
```

All conversions use `url.URL` / `url.Parse` rather than string slicing. This correctly handles percent-encoded characters (e.g. spaces in paths → `%20`) and handles non-standard URI forms correctly.

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
                          (LSP notification: the server pushes diagnostic updates to the client)
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
sendRequest("initialize", {capabilities, rootUri, workspaceFolders})
    ↓  server may send window/workDoneProgress/create, workspace/configuration here
    ↓  these are handled in dispatch() → handleServerRequest before initialize returns
receive initialize response
    ↓  captures serverCapabilities, semantic token legend
client.initialized = true
sendNotification("initialized", {})
    ↓
startWatcher(rootDir)
    ↓
tool calls now available
```

`initialized` is set to `true` before `initialized` notification is sent (not after) to prevent a race where the server's first request arrives in the window between sending `initialized` and setting the flag.

### Request/response correlation

Each outgoing request is assigned a monotonically-increasing integer ID. A `pendingRequest` struct holding `ch chan json.RawMessage` and `err chan error` is stored in `c.pending[id]`. `readLoop` calls `dispatch()` on every incoming frame; when `dispatch` sees a response message (has `id`, no `method`), it resolves the pending channel.

Per-method timeouts are applied to each `sendRequest` call. `textDocument/references` gets 120s (full workspace indexing); `initialize` gets 300s (cold-start JVM servers).

### Crash recovery

When the LSP subprocess exits:
1. The exit-monitor goroutine calls `rejectPending(err)`, closing all open pending channels with the exit error so callers fail fast rather than waiting for timeouts
2. `initialized` is set to `false`
3. The last 4KB of stderr is logged at `error` level

---

## WaitForDiagnostics

`WaitForDiagnostics(ctx, client, targetURIs []string, timeoutMs int)` is used by `get_diagnostics`, `evaluate_session`, and resource handlers to wait for the language server to finish publishing diagnostics after a document is opened or modified.

It resolves when:

1. All target URIs have received at least one diagnostic notification *after* the initial snapshot (the first notification is excluded; it is the server's pre-existing state for that file)
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

Extensions take precedence over core handlers in case of name conflicts. All features are namespaced by language ID automatically. Unlike dynamic plugin systems, Go extensions are registered at compile time. Unused extensions have zero runtime cost and there is no filesystem scan or `dlopen`.

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
2. Walks the tree with `findInnermostSymbol`, which recursively finds the deepest symbol whose `Range` contains the 0-based cursor position
3. Reads the file from disk and slices the lines corresponding to `sym.Range.Start.Line` to `sym.Range.End.Line` (0-based, inclusive)
4. Returns `SymbolSourceResult{SymbolName, SymbolKind, StartLine, EndLine, Source}` with 1-based line numbers

This is useful for agents that want to read a function body without manually counting lines.

---

## See also

- [Home](index.md): project overview, setup, and quick start
- [docs/tools.md](./tools.md): full tool reference with parameters and examples
- [docs/skills.md](./skills.md): skill reference with workflows and composition patterns
- [docs/language-support.md](./language-support.md): language coverage matrix
