# agent-lsp

[![Blackwell Systems](https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg)](https://github.com/blackwell-systems)
[![CI](https://github.com/blackwell-systems/agent-lsp/actions/workflows/ci.yml/badge.svg)](https://github.com/blackwell-systems/agent-lsp/actions)
[![LSP 3.17](https://img.shields.io/badge/LSP-3.17-blue.svg)](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
[![Languages](https://img.shields.io/badge/languages-22_CI--verified-brightgreen.svg)](#multi-language-support)
[![Tools](https://img.shields.io/badge/tools-47-blue.svg)](#tools)
[![CI Coverage](https://img.shields.io/badge/CI--verified_tools-28%2F47-brightgreen.svg)](#tools)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Agent Skills](assets/badge-agentskills.svg)](https://agentskills.io)

agent-lsp is a stateful runtime over real language servers — not a bridge. It maintains a persistent, warm session, reshapes LSP into agent-oriented workflows, and adds a transactional execution layer for safe speculative edits.

Language servers are the intelligence layer behind IDE features — go-to-definition, find-all-references, inline errors, completions. They understand code semantically: types, symbols, scope, cross-file relationships.

**47 tools** across navigation, analysis, refactoring, and formatting — **28 CI-verified** end-to-end against real language servers across **22 languages**. Built to [LSP 3.17 spec](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/).

**Work across all your projects in one AI session.** Point your AI assistant at your `~/code/` directory. One agent-lsp process automatically routes `.go` files to gopls, `.ts` files to typescript-language-server, `.py` to pyright — no reconfiguration when you switch projects.

**Persistent session, warm index.** Unlike per-request bridges, agent-lsp maintains a live language server session. `start_lsp` indexes the workspace once; every subsequent call hits the warm index. `get_references` returns all 12 call sites without loading files into context. `get_diagnostics` returns only the errors. `get_info_on_location` returns the type signature at one position without loading the module. The language server's index stays fresh automatically — agent-lsp watches the workspace using kernel-level filesystem events (inotify/kqueue/FSEvents) and forwards changes to keep the session synchronized. High-churn directories (`.git/`, `node_modules/`, etc.) are excluded; rapid edits are debounced at 150ms. No `did_change_watched_files` calls required.

**Fuzzy position fallback.** When an AI assistant gets a line/column slightly wrong, `go_to_definition`, `get_references`, and `rename_symbol` fall back to workspace symbol search by hover name and retry — returning results instead of silently returning empty.

**Semantic token classification.** `get_semantic_tokens` classifies every token in a range as `function`, `parameter`, `variable`, `type`, `keyword`, etc. — the same data an IDE uses to colorize code. No other MCP-LSP server exposes this.

## Skills

Ten agent-native skills compose agent-lsp tools into single-command workflows:

| Skill | Purpose |
|-------|---------|
| `/lsp-safe-edit` | Wrap any edit with before/after diagnostic diff |
| `/lsp-edit-export` | Safe editing of exported symbols — finds all callers first |
| `/lsp-edit-symbol` | Edit a named symbol without knowing its file or position |
| `/lsp-rename` | Two-phase rename: preview all sites, confirm, then apply |
| `/lsp-verify` | Full three-layer check: diagnostics + build + tests; apply code actions on errors |
| `/lsp-simulate` | Speculative editing — test changes without touching the file |
| `/lsp-impact` | Blast-radius analysis before renaming or deleting a symbol |
| `/lsp-dead-code` | Detect zero-reference exports and unreachable symbols |
| `/lsp-implement` | Find all concrete implementations of an interface or abstract type |
| `/lsp-docs` | Three-tier documentation lookup: hover → offline toolchain (`go doc`, `pydoc`) → source |

Skills work with any MCP client that supports tool use, not just Claude Code.

```bash
cd skills && ./install.sh
```

### Recent additions

`get_symbol_source` returns the full source text of the innermost symbol at a position — functions, methods, structs, and classes — directly from the language server index without reading files into context. `get_symbol_documentation` dispatches to the language toolchain (`go doc`, `pydoc`, `cargo doc`) for offline documentation when hover results are incomplete. MCP log notifications are now forwarded to the connected client via `notifications/message` using the standard logging level protocol. See [docs/tools.md](./docs/tools.md) for parameter details.

## Installation

**Requires Go 1.21+** — [install Go](https://go.dev/dl/) if needed.

```bash
go install github.com/blackwell-systems/agent-lsp@latest
```

If `agent-lsp` isn't found after install, add Go's bin directory to your PATH:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"   # add to ~/.zshrc or ~/.bashrc to persist
```

## Setup

### Step 1 — Install language servers for your stack

agent-lsp runs on top of real language servers — install the servers for your stack and agent-lsp handles the rest.

| Language | Server | Install |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |
| PHP | `intelephense` | `npm i -g intelephense` |
| Java | `jdtls` | [eclipse.jdt.ls snapshots](https://download.eclipse.org/jdtls/snapshots/) |
| YAML | `yaml-language-server` | `npm i -g yaml-language-server` |
| JSON | `vscode-json-language-server` | `npm i -g vscode-langservers-extracted` |
| Dockerfile | `docker-langserver` | `npm i -g dockerfile-language-server-nodejs` |
| C# | `csharp-ls` | `dotnet tool install -g csharp-ls` |
| Kotlin | `kotlin-language-server` | [GitHub releases](https://github.com/fwcd/kotlin-language-server/releases) |
| Lua | `lua-language-server` | [GitHub releases](https://github.com/LuaLS/lua-language-server/releases) |
| Swift | `sourcekit-lsp` | Ships with Xcode / Swift toolchain |
| Zig | `zls` | [GitHub releases](https://github.com/zigtools/zls/releases) (match Zig version) |
| CSS | `vscode-css-language-server` | `npm i -g vscode-langservers-extracted` |
| HTML | `vscode-html-language-server` | `npm i -g vscode-langservers-extracted` |
| Terraform | `terraform-ls` | [releases.hashicorp.com](https://releases.hashicorp.com/terraform-ls/) |
| Scala | `metals` | `cs install metals` ([Coursier](https://get-coursier.io)) |

### Step 2 — Add to your AI config

Add to `.mcp.json` (project) or your AI tool's global MCP config. List only the languages you use:

```json
{
  "mcpServers": {
    "lsp": {
      "type": "stdio",
      "command": "agent-lsp",
      "args": [
        "go:gopls",
        "typescript:typescript-language-server,--stdio",
        "python:pyright-langserver,--stdio"
      ]
    }
  }
}
```

Each arg is `language:server-binary` (comma-separate server args). Single language? Use `"args": ["go", "gopls"]`. Complex setup with many servers or per-server options? Use `"args": ["--config", "/path/to/agent-lsp.json"]`.

### Step 3 — Start working

Once your AI session opens, call `start_lsp` with your project root to initialize:

```
start_lsp(root_dir="/your/project")
```

Then use any of the 47 tools. The session persists — no need to restart when switching files.

## Why agent-lsp

| | agent-lsp | other MCP-LSP implementations |
|--|---------|---------------------|
| Languages (CI-verified) | **22** (end-to-end integration tests) | config-listed, untested |
| Tools | **47** | 3–18 |
| Multi-server routing | **✓** (one process, many languages) | varies |
| LSP spec compliance | **3.17, built to spec** | ad hoc |
| Connection model | **persistent** (warm index) | per-request or cold-start |
| Cross-file references | **✓** | rarely |
| Real-time diagnostic subscriptions | **✓** | ✗ |
| Semantic token classification | **✓** | ✗ (only one competitor) |
| Call hierarchy | **✓** (single tool, direction param) | ✗ or 3 separate tools |
| Type hierarchy | **✓** (single tool, direction param) | ✗ or untested |
| Fuzzy position fallback | **✓** | ✗ or partial |
| Auto-watch (index stays fresh) | **✓** (always-on, debounced) | ✗ (manual notify required) |
| Multi-root / cross-repo | **✓** (`add_workspace_folder`) | ✗ or single-workspace only |
| Path traversal prevention | **✓** | ✗ |
| Distribution | **single Go binary** | Node.js or Bun runtime required |

## Use Cases

- **Multi-project AI sessions** — point your AI assistant at `~/code/`, work across any project without reconfiguring
- **Polyglot development** — Go backend + TypeScript frontend + Python scripts in one session
- **Large monorepos** — one server handles all languages, routes by file extension
- **Code migration** — refactor across repos (e.g., extracting a Go library used by 3 services)
- **CI pipelines** — validate against real language server behavior


## Multi-Language Support

Every language below is integration-tested on every CI run with a real language server binary and a real fixture codebase. The test harness verifies **Tier 1** (`start_lsp`, `open_document`, `get_diagnostics`, `get_info_on_location`) and **Tier 2** (27 tools including navigation, analysis, refactoring, workspace, and session lifecycle) for each language. No other MCP-LSP implementation has an equivalent test matrix — competitors list supported languages in config examples but do not run integration tests against them.

Tier 2 results per language from the latest CI run:

| Language | Tier 1 | symbols | definition | references | completions | workspace | format | declaration | type_hierarchy | hover | call_hier | sem_tok | sig_help |
|----------|--------|---------|------------|------------|-------------|-----------|--------|-------------|----------------|-------|-----------|---------|----------|
| TypeScript | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | pass |
| Python | pass | pass | pass | pass | pass | pass | — | — | — | pass | pass | pass | — |
| Go | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Rust | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | — |
| Java | pass | — | — | — | — | — | — | — | pass | pass | pass | — | — |
| C | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| PHP | pass | pass | pass | pass | pass | pass | — | — | — | pass | — | — | — |
| C++ | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| JavaScript | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| Ruby | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| YAML | pass | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| JSON | pass | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| Dockerfile | pass | — | — | — | pass | pass | — | — | — | pass | — | — | — |
| C# | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Kotlin | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Lua | pass | pass | — | — | pass | pass | — | — | — | pass | — | — | — |
| Swift | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Zig | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| CSS | pass | pass | — | — | pass | pass | pass | — | — | pass | — | — | — |
| HTML | pass | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| Terraform | pass | pass | pass | — | pass | pass | pass | — | — | pass | — | — | — |
| Scala | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |

Java Tier 2 is skipped when jdtls does not finish indexing within the CI timeout (a known jdtls cold-start characteristic, not a tool bug). Scala (metals) runs in a separate CI job with `continue-on-error: true` and a 30-minute timeout — metals requires sbt compilation on first start; results are informational. Swift (`sourcekit-lsp`) runs on a `macos-latest` runner since sourcekit-lsp ships with Xcode. `type_hierarchy` is tested on Java (jdtls) and TypeScript (typescript-language-server); TypeScript skips when the server does not return a hierarchy item at the configured position.

## Tools

All tools require `start_lsp` to be called first.

**CI coverage:** The following tools are end-to-end integration-tested against real language servers on every CI run across all 22 languages:

- **Tier 1** (4 tools, all 22 languages): `start_lsp`, `open_document`, `get_diagnostics`, `get_info_on_location`
- **Tier 2** (28 tools): `get_document_symbols`, `go_to_definition`, `get_references`, `get_completions`, `get_workspace_symbols`, `format_document`, `go_to_declaration`, `type_hierarchy`, `get_info_on_location`, `call_hierarchy`, `get_semantic_tokens`, `get_signature_help`, `get_document_highlights`, `get_inlay_hints`, `get_code_actions`, `prepare_rename`, `rename_symbol`, `get_server_capabilities`, `add_workspace_folder`, `go_to_type_definition`, `go_to_implementation`, `format_range`, `apply_edit`, `detect_lsp_servers`, `close_document`, `did_change_watched_files`, `run_build`, `run_tests`

Speculative session tools (`create_simulation_session`, `simulate_edit`, `simulate_edit_atomic`, `simulate_chain`, `evaluate_session`, `commit_session`, `discard_session`, `destroy_session`) are covered by `TestSpeculativeSessions` in `test/speculative_test.go`. Remaining tools (`restart_lsp_server`, `execute_command`, `set_log_level`) are unit tested.

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
| `get_workspace_symbols` | Search symbols by name across the workspace; `detail_level=hover` enriches results with type signatures and docs without loading files into context |
| `get_semantic_tokens` | Classify tokens in a range as function/parameter/variable/type/keyword/etc — same data IDEs use for syntax highlighting |
| `get_inlay_hints` | Inline type annotations and parameter name labels for a range — inferred type hints IDEs overlay on source code (Type and Parameter kinds) |

### Navigation
| Tool | Description |
|------|-------------|
| `get_references` | All references to a symbol across the workspace |
| `get_document_highlights` | All occurrences of a symbol in the current file — file-scoped, instant, returns read/write/text kinds; faster than `get_references` for local usage analysis |
| `go_to_definition` | Jump to where a symbol is defined (with fuzzy position fallback) |
| `go_to_type_definition` | Jump to the type definition of a symbol |
| `go_to_implementation` | Jump to all implementations of an interface or abstract method |
| `go_to_declaration` | Jump to the declaration of a symbol (distinct from definition — e.g. C/C++ headers) |
| `call_hierarchy` | Callers and/or callees of a function — `direction: "incoming"`, `"outgoing"`, or `"both"` (default) |
| `type_hierarchy` | Supertypes and/or subtypes of a type — `direction: "supertypes"`, `"subtypes"`, or `"both"` (default) |

### Refactoring
| Tool | Description |
|------|-------------|
| `rename_symbol` | Get a `WorkspaceEdit` for renaming a symbol across the workspace (with fuzzy position fallback) |
| `prepare_rename` | Validate a rename is possible before committing |
| `format_document` | Get `TextEdit[]` formatting edits for a file |
| `format_range` | Get `TextEdit[]` formatting edits for a selection |
| `apply_edit` | Apply a `WorkspaceEdit` to disk (use with `rename_symbol` or `format_document`) |
| `execute_command` | Execute a server-side command (e.g. from a code action) |

### Utilities
| Tool | Description |
|------|-------------|
| `did_change_watched_files` | Manually notify the server of file changes — not needed for normal edits (auto-watch handles those); use when an external process changes files outside the session |
| `get_server_capabilities` | Return the server capability map and classify every tool as supported or unsupported — use before calling capability-gated tools |
| `set_log_level` | Change log verbosity at runtime |
| `detect_lsp_servers` | Scan a workspace for source languages and check PATH for installed LSP servers — returns detected languages, server paths, and a `suggested_config` array ready to paste into your MCP config |

### Workspace
| Tool | Description |
|------|-------------|
| `add_workspace_folder` | Add a directory to the LSP workspace — enables cross-repo references, definitions, and diagnostics across library + consumer repos in one session |
| `remove_workspace_folder` | Remove a directory from the LSP workspace |
| `list_workspace_folders` | Return the current workspace folder list |

### Speculative Execution
Safe what-if analysis — simulate edits in-memory, evaluate diagnostic changes (errors introduced/resolved), then commit or discard atomically. No disk writes until you call `commit_session`.

| Tool | Description |
|------|-------------|
| `create_simulation_session` | Create a session with baseline diagnostics for a file |
| `simulate_edit` | Apply an in-memory edit to the session (no disk write) |
| `simulate_edit_atomic` | Apply an edit, evaluate diagnostics, and discard in one call — returns net error delta; accepts optional `session_id` to reuse an existing session |
| `simulate_chain` | Apply a sequence of edits and evaluate after each step |
| `evaluate_session` | Compare current in-memory diagnostics against baseline — returns errors introduced and resolved |
| `commit_session` | Write the session's edits to disk |
| `discard_session` | Revert in-memory edits without touching disk |
| `destroy_session` | Release all session resources |

See [docs/speculative-execution.md](./docs/speculative-execution.md) for full workflow examples.

**Recommended agent workflow:**
```
start_lsp(root_dir="/your/project")
open_document(file_path=..., language_id=...)
get_diagnostics()                          # whole project, no file_path
get_info_on_location(...) / get_references(...)
close_document(...)
```

**Keeping the index fresh:**

agent-lsp watches the workspace root for file changes and automatically notifies the language server — no `did_change_watched_files` calls required after edits. The watcher skips high-churn directories (`.git/`, `node_modules/`, `target/`, etc.) and debounces rapid edits at 150ms.

`did_change_watched_files` is still available for cases where files are changed by an external process that the watcher may not see immediately, or for explicit control over change notifications.

**Rename workflow** (`prepare_rename` → `rename_symbol` → `apply_edit`):
```
prepare_rename(file_path=..., line=..., column=...)        # confirm rename is valid at this position
rename_symbol(file_path=..., line=..., column=..., new_name="newName")  # returns WorkspaceEdit
apply_edit(edit=<WorkspaceEdit>)                           # writes all changed files to disk
# auto-watch notifies the server automatically — no did_change_watched_files needed
```

**Language IDs:** `typescript`, `typescriptreact`, `javascript`, `javascriptreact`, `python`, `go`, `rust`, `java`, `kotlin`, `scala`, `swift`, `lua`, `zig`, `terraform`, `c`, `cpp`, `csharp`, `php`, `ruby`, `css`, `html`, `yaml`, `json`, `dockerfile`

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

agent-lsp is implemented directly against the [LSP 3.17 specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/) and validated through integration testing against real language servers. Coverage includes:

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
git clone https://github.com/blackwell-systems/agent-lsp.git
cd agent-lsp && go build ./...
go test ./...                   # all unit test suites
go test ./... -tags integration # integration tests (requires language servers)
```

## License

MIT
