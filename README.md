# lsp-mcp-go

[![Blackwell Systems](https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg)](https://github.com/blackwell-systems)
[![CI](https://github.com/blackwell-systems/lsp-mcp-go/actions/workflows/ci.yml/badge.svg)](https://github.com/blackwell-systems/lsp-mcp-go/actions)
[![LSP 3.17](https://img.shields.io/badge/LSP-3.17-blue.svg)](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
[![Languages](https://img.shields.io/badge/languages-7_verified-green.svg)](#multi-language-support)
[![Tools](https://img.shields.io/badge/tools-24-blue.svg)](#tools)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Language servers are the intelligence layer behind IDE features — autocompletion, go-to-definition, inline errors, find-all-references. They run as background processes and understand code at a semantic level: types, symbols, scope, and cross-file relationships. Every major editor uses them silently. lsp-mcp-go exposes that same intelligence to agents through the MCP protocol.

lsp-mcp-go turns language servers into queryable infrastructure for agents.

The most complete MCP server for language intelligence — built for agents, not just protocol passthrough. **24 tools** spanning navigation, diagnostics, refactoring, and formatting. CI-verified across **7 languages**. Built directly against the [LSP 3.17 specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/).

Unlike typical MCP-LSP bridges, lsp-mcp-go maintains a **persistent language server session** — agents operate on a fully indexed, stateful workspace with real-time diagnostics and cross-file reasoning, not a cold-started stub that forgets context between calls.

Designed for agentic workflows where correctness, persistence, and cross-language reliability are required.

The LSP layer built for long-running agentic workflows.

**Token efficiency (critical for LLM agents):** Language servers maintain a pre-built index of the entire workspace. Instead of loading files into context to find usages, trace types, or locate definitions, agents query the index directly — `get_references` returns the 12 call sites without pasting 5 files, `get_info_on_location` returns the type signature at one position without loading the module, `get_diagnostics` returns only the errors without reading every file. The persistent session means indexing happens once on `start_lsp`; every subsequent query hits the warm index.

## Installation

```bash
go install github.com/blackwell-systems/lsp-mcp-go@latest
```

This installs the `lsp-mcp-go` binary to `$GOPATH/bin` (typically `~/go/bin`). Make sure that directory is on your `PATH`.

## Why lsp-mcp-go

| | lsp-mcp-go | other MCP-LSP implementations |
|--|---------|---------------------|
| Languages (CI-verified) | **7** | 1–2 |
| Tools | **24** | 3–5 |
| LSP spec compliance | **3.17, built to spec** | ad hoc |
| Connection model | **persistent** | per-request |
| Cross-file references | **✓** | rarely |
| Real-time diagnostic subscriptions | **✓** | ✗ |
| Distribution | **single Go binary** | Node.js runtime required |

## Use Cases

- Agent-driven analysis across large, multi-language repositories
- Safe, workspace-wide refactoring with full context
- CI pipelines that validate against real language server behavior
- Code intelligence without relying on an IDE

## Quick Start

```json
{
  "mcpServers": {
    "lsp": {
      "type": "stdio",
      "command": "lsp-mcp-go",
      "args": ["<language-id>", "<path-to-lsp-binary>", "<lsp-args>"]
    }
  }
}
```

**TypeScript:**
```json
{ "args": ["typescript", "typescript-language-server", "--stdio"] }
```

**Go:**
```json
{ "args": ["go", "gopls"] }
```

**Rust:**
```json
{ "args": ["rust", "rust-analyzer"] }
```

## Multi-Language Support

Every language below is integration-tested on every CI run — `start_lsp`, `open_document`, `get_diagnostics`, and `get_info_on_location` all verified against the real language server binary:

| Language | Server | Install |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| Java | `jdtls` | [eclipse.jdt.ls snapshots](https://download.eclipse.org/jdtls/snapshots/) |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| PHP | `intelephense` | `npm i -g intelephense` |

## Tools

All tools require `start_lsp` to be called first.

### Session
| Tool | Description |
|------|-------------|
| `start_lsp` | Start the language server with a project root |
| `restart_lsp_server` | Restart without restarting the MCP server |
| `open_document` | Open a file for tracking (required before position queries) |
| `close_document` | Stop tracking a file |

### Analysis
| Tool | Description |
|------|-------------|
| `get_diagnostics` | Errors and warnings — omit `file_path` for whole project |
| `get_info_on_location` | Hover info (type signatures, docs) at a position |
| `get_completions` | Completion suggestions at a position |
| `get_signature_help` | Function signature and active parameter at a call site |
| `get_code_actions` | Quick fixes and refactors for a range |
| `get_document_symbols` | All symbols in a file (functions, classes, variables) |
| `get_workspace_symbols` | Search symbols by name across the workspace |

### Navigation
| Tool | Description |
|------|-------------|
| `get_references` | All references to a symbol across the workspace |
| `go_to_definition` | Jump to where a symbol is defined |
| `go_to_type_definition` | Jump to the type definition of a symbol |
| `go_to_implementation` | Jump to all implementations of an interface or abstract method |
| `go_to_declaration` | Jump to the declaration of a symbol (distinct from definition — e.g. C/C++ headers) |

### Refactoring
| Tool | Description |
|------|-------------|
| `rename_symbol` | Get a `WorkspaceEdit` for renaming a symbol across the workspace |
| `prepare_rename` | Validate a rename is possible before committing |
| `format_document` | Get `TextEdit[]` formatting edits for a file |
| `format_range` | Get `TextEdit[]` formatting edits for a selection |
| `apply_edit` | Apply a `WorkspaceEdit` to disk (use with `rename_symbol` or `format_document`) |
| `execute_command` | Execute a server-side command (e.g. from a code action) |

### Utilities
| Tool | Description |
|------|-------------|
| `did_change_watched_files` | Notify the server when files change on disk outside the editor |
| `set_log_level` | Change log verbosity at runtime |

**Recommended agent workflow:**
```
start_lsp(root_dir="/your/project")
open_document(file_path=..., language_id=...)
get_diagnostics()                          # whole project, no file_path
get_info_on_location(...) / get_references(...)
close_document(...)
```

**Keeping the index fresh during active editing:**

Without notifying the language server of file changes, its index becomes stale immediately — `get_references`, `get_diagnostics`, and hover info will reflect the old state, not what was just written. After every file edit, call:

```
did_change_watched_files(changes=[
  { uri: "file:///absolute/path/to/file.go", type: 2 }
])
```

Type values: `1` = created, `2` = changed, `3` = deleted. The server re-reads from disk and updates its index — no restart required. In long sessions where files change frequently, this is the difference between a reliable index and silently stale results.

**Rename workflow** (`prepare_rename` → `rename_symbol` → `apply_edit`):
```
prepare_rename(file_path=..., line=..., column=...)   # confirm rename is valid at this position
rename_symbol(file_path=..., line=..., column=..., new_name="newName")  # returns WorkspaceEdit
apply_edit(edit=<WorkspaceEdit>)                      # writes all changed files to disk
did_change_watched_files(changes=[...])               # notify server of the disk changes
```

**Language IDs:** `typescript`, `typescriptreact`, `javascript`, `javascriptreact`, `python`, `go`, `rust`, `java`, `c`, `cpp`, `php`

## Resources

Diagnostic resources support real-time subscriptions — the server sends `notifications/resources/updated` when diagnostics change for a subscribed file.

| Scheme | Description |
|--------|-------------|
| `lsp-diagnostics://` | All open files |
| `lsp-diagnostics:///path/to/file` | Specific file (subscribable) |
| `lsp-hover:///path/to/file?line=N&column=N&language_id=X` | Hover at position |
| `lsp-completions:///path/to/file?line=N&column=N&language_id=X` | Completions at position |

**Subscribing to real-time diagnostics:**
```json
{ "method": "resources/subscribe", "params": { "uri": "lsp-diagnostics:///path/to/file.go" } }
```
The server sends `notifications/resources/updated` each time the language server publishes new diagnostics for that file. Read the resource after each notification to get the current diagnostic list:
```json
{ "method": "resources/read", "params": { "uri": "lsp-diagnostics:///path/to/file.go" } }
```

## LSP 3.17 Conformance

lsp-mcp-go is implemented directly against the [LSP 3.17 specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/) and validated through integration testing against real language servers. Coverage includes:

- Full lifecycle (`initialize` → `initialized` → `shutdown`) with graceful SIGINT/SIGTERM handling
- Progress protocol — workspace-ready detection waits for all `$/progress` tokens to complete before sending references
- Server-initiated requests (`workspace/configuration`, `window/workDoneProgress/create`, dynamic registration) — all correctly responded to, unblocking servers that gate workspace loading on these responses
- Correct JSON-RPC framing, error code handling, and response shape normalization across hover, completion, code actions, and diagnostics

See [docs/lsp-conformance.md](./docs/lsp-conformance.md) for the full method coverage matrix and spec section references.

See [docs/tools.md](./docs/tools.md) for the full tool reference with example inputs and outputs.

See [docs/architecture.md](./docs/architecture.md) for the Go package structure, `WithDocument` pattern, URI handling, and resource subscription internals.

## Extensions

Language-specific extensions add tools, prompts, and resource handlers, registered automatically by language ID at startup.

To add an extension, create `extensions/<language-id>/` implementing any subset of the extension interface and register it via `extensions.RegisterFactory` in an `init()` function. All features are namespaced by language ID.

## Development

```bash
git clone https://github.com/blackwell-systems/lsp-mcp-go.git
cd lsp-mcp-go && go build ./...
go test ./...                   # all unit test suites
go test ./... -tags integration # integration tests (requires language servers)
```

## License

MIT
