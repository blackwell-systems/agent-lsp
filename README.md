# agent-lsp

[![Blackwell Systems](https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg)](https://github.com/blackwell-systems)
[![CI](https://github.com/blackwell-systems/agent-lsp/actions/workflows/ci.yml/badge.svg)](https://github.com/blackwell-systems/agent-lsp/actions)
[![LSP 3.17](https://img.shields.io/badge/LSP-3.17-blue.svg)](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
[![Languages](https://img.shields.io/badge/languages-30_CI--verified-brightgreen.svg)](#multi-language-support)
[![Tools](https://img.shields.io/badge/tools-50-blue.svg)](#tools)
[![CI Coverage](https://img.shields.io/badge/CI--verified_tools-34%2F50-brightgreen.svg)](#tools)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Agent Skills](assets/badge-agentskills.svg)](https://agentskills.io)

Language servers are the intelligence layer behind IDE features: go-to-definition, find-all-references, inline errors, completions. They understand code semantically: types, symbols, scope, cross-file relationships. AI agents should be able to use them. Most can't, for two reasons.

**First, existing MCP-LSP implementations are stateless bridges.** They cold-start the language server on every call, which means no warm index, no cross-file awareness, and no way to maintain session state across a multi-step workflow. The agent pays the indexing cost every time.

**Second, raw tools don't get used.** You can expose 50 tools to an agent, but in non-SDK human-in-the-loop workflows, agents routinely skip them, even when available. A safe rename requires `prepare_rename` → `rename_symbol` → `apply_edit` in sequence. An agent that has to reason its way to the correct sequence on every invocation will often skip steps or use the wrong tool. The tools exist but the workflow doesn't reliably happen.

agent-lsp solves both problems. It is a **stateful runtime** over real language servers, not a bridge. It maintains a persistent warm session and adds a **skill layer** that wraps correct tool sequences into single-command workflows agents actually use.

**50 tools** across navigation, analysis, refactoring, and formatting; **34 CI-verified** end-to-end against real language servers across **30 languages**. Built to [LSP 3.17 spec](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/).

**Work across all your projects in one AI session.** Point your AI assistant at your `~/code/` directory. One agent-lsp process automatically routes `.go` files to gopls, `.ts` files to typescript-language-server, `.py` to pyright; no reconfiguration when you switch projects.

**Persistent session, warm index.** Unlike per-request bridges, agent-lsp maintains a live language server session. `start_lsp` indexes the workspace once; every subsequent call hits the warm index. `get_references` returns all 12 call sites without loading files into context. `get_diagnostics` returns only the errors. `get_info_on_location` returns the type signature at one position without loading the module. The language server's index stays fresh automatically: agent-lsp watches the workspace using kernel-level filesystem events (inotify/kqueue/FSEvents) and forwards changes to keep the session synchronized. High-churn directories (`.git/`, `node_modules/`, etc.) are excluded; rapid edits are debounced at 150ms. No `did_change_watched_files` calls required.

**Fuzzy position fallback.** When an AI assistant gets a line/column slightly wrong, `go_to_definition`, `get_references`, and `rename_symbol` fall back to workspace symbol search by hover name and retry, returning results instead of silently returning empty.

**Semantic token classification.** `get_semantic_tokens` classifies every token in a range as `function`, `parameter`, `variable`, `type`, `keyword`, etc.; the same data an IDE uses to colorize code. No other MCP-LSP server exposes this.

## Skills

The skill layer is the behavioral reliability layer. Raw tools get ignored; skills get used. Each skill encodes the correct tool sequence for a workflow; the agent reads the skill, follows the steps, and uses the tools in the right order without per-prompt orchestration instructions. This is the difference between tools that are available and workflows that actually happen.

Fourteen skills ship with agent-lsp:

| Skill | Purpose |
|-------|---------|
| `/lsp-safe-edit` | Speculative preview before disk write (`simulate_edit_atomic`); refactor/rename preview via `simulate_chain`; before/after diagnostic diff; surfaces code actions on introduced errors; multi-file aware |
| `/lsp-edit-export` | Safe editing of exported symbols; finds all callers first |
| `/lsp-edit-symbol` | Edit a named symbol without knowing its file or position |
| `/lsp-rename` | `prepare_rename` safety gate, preview all sites, confirm, then apply atomically |
| `/lsp-verify` | Full three-layer check: diagnostics + build + tests; apply code actions on errors |
| `/lsp-simulate` | Speculative editing: test changes without touching the file |
| `/lsp-impact` | Blast-radius analysis before renaming or deleting a symbol; accepts a file path to surface all exported-symbol impact at once via `get_change_impact` |
| `/lsp-dead-code` | Detect zero-reference exports and unreachable symbols |
| `/lsp-implement` | Find all concrete implementations of an interface or abstract type |
| `/lsp-docs` | Three-tier documentation lookup: hover, offline toolchain (`go doc`, `pydoc`), source |
| `/lsp-cross-repo` | Multi-root cross-repo caller analysis: find all usages of a library symbol across consumer repos in a single `get_cross_repo_references` call; partitioned by repo |
| `/lsp-local-symbols` | File-scoped analysis: list all symbols, find all usages within the file, get type info; faster than workspace search for local queries |
| `/lsp-test-correlation` | Find and run only the tests that cover an edited file; faster than the full suite for targeted post-edit verification |
| `/lsp-format-code` | Format a file or selection using the language server's formatter (`gofmt`, `prettier`, `rustfmt`); full-file or range, applies edits to disk |

Skills work with any MCP client that supports tool use, not just Claude Code.

```bash
cd skills && ./install.sh
```

See [docs/tools.md](./docs/tools.md) for full parameter details.

## Docker

Pre-built images on GitHub Container Registry for all major languages:

```bash
# Go
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls

# TypeScript
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio

# Python
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio

# Multi-language (runtime install)
docker run --rm -i -v /your/project:/workspace \
  -e LSP_SERVERS=gopls,typescript-language-server \
  ghcr.io/blackwell-systems/agent-lsp:latest \
  go:gopls typescript:typescript-language-server,--stdio
```

See [DOCKER.md](./DOCKER.md) for full tier documentation, per-language tags,
docker-compose setup, and volume caching.

## Installation

**Requires Go 1.21+.** [Install Go](https://go.dev/dl/) if needed.

```bash
go install github.com/blackwell-systems/agent-lsp@latest
```

To use agent-lsp as a library in your Go program (without running the MCP
server), import the `pkg/` packages directly — see [Library Usage](#library-usage) below.

If `agent-lsp` isn't found after install, add Go's bin directory to your PATH:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"   # add to ~/.zshrc or ~/.bashrc to persist
```

## Setup

### Step 1: Install language servers for your stack

agent-lsp runs on top of real language servers. Install the servers for your stack and agent-lsp handles the rest.

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
| Gleam | `gleam` (built-in) | [GitHub releases](https://github.com/gleam-lang/gleam/releases) |
| Elixir | `elixir-ls` | [GitHub releases](https://github.com/elixir-lsp/elixir-ls/releases) |
| Prisma | `prisma-language-server` | `npm i -g @prisma/language-server` |
| SQL | `sqls` | `go install github.com/sqls-server/sqls@latest` |
| Clojure | `clojure-lsp` | [GitHub releases](https://github.com/clojure-lsp/clojure-lsp/releases) |
| Nix | `nil` | [GitHub releases](https://github.com/oxalica/nil/releases) |
| Dart | `dart language-server` | Ships with Dart SDK (`brew install dart`) |
| MongoDB | `mongodb-language-server` | `npm i -g @mongodb-js/mongodb-language-server` |

### Step 2: Add to your AI config

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

### Step 3: Start working

Once your AI session opens, call `start_lsp` with your project root to initialize:

```
start_lsp(root_dir="/your/project")
```

Then use any of the 50 tools. The session persists; no need to restart when switching files.

## Why agent-lsp

| | agent-lsp | other MCP-LSP implementations |
|--|---------|---------------------|
| Languages (CI-verified) | **30** (end-to-end integration tests) | config-listed, untested |
| Tools | **50** | 3–18 |
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

- **Multi-project AI sessions**: point your AI assistant at `~/code/`, work across any project without reconfiguring
- **Polyglot development**: Go backend + TypeScript frontend + Python scripts in one session
- **Large monorepos**: one server handles all languages, routes by file extension
- **Code migration**: refactor across repos (e.g., extracting a Go library used by 3 services)
- **CI pipelines**: validate against real language server behavior


## Multi-Language Support

Every language below is integration-tested on every CI run with a real language server binary and a real fixture codebase. The test harness verifies **Tier 1** (`start_lsp`, `open_document`, `get_diagnostics`, `get_info_on_location`) and **Tier 2** (27 tools including navigation, analysis, refactoring, workspace, and session lifecycle) for each language. No other MCP-LSP implementation has an equivalent test matrix; competitors list supported languages in config examples but do not run integration tests against them.

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
| Gleam | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| Elixir | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| Prisma | — | — | — | — | — | — | — | — | — | — | — | — | — |
| SQL | pass | pass | pass | pass | pass | pass | — | — | — | pass | — | — | — |
| Clojure | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| Nix | pass | pass | — | — | pass | pass | — | — | — | pass | — | — | — |
| Dart | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| MongoDB | pass | — | — | — | pass | pass | — | — | — | pass | — | — | — |

Java Tier 2 is skipped when jdtls does not finish indexing within the CI timeout (a known jdtls cold-start characteristic, not a tool bug). Scala (metals) runs in a separate CI job with `continue-on-error: true` and a 30-minute timeout; metals requires sbt compilation on first start and results are informational. Swift (`sourcekit-lsp`) runs on a `macos-latest` runner since sourcekit-lsp ships with Xcode. Prisma runs with `continue-on-error: true`; the language server requires VS Code extension host features and is under active investigation. SQL (sqls) requires a live PostgreSQL service container; the CI job provisions postgres:16 automatically. `type_hierarchy` is tested on Java (jdtls) and TypeScript (typescript-language-server); TypeScript skips when the server does not return a hierarchy item at the configured position. Clojure (`clojure-lsp`), Nix (`nil`), Dart (`dart language-server`), and MongoDB (`mongodb-language-server`) CI-verified as of the ci-coverage-expansion IMPL. Nix runs with `continue-on-error: true` (Nix installer is slow in CI; nil installs via nix profile). MongoDB language server is extracted from the `mongodb-js/vscode` VS Code extension VSIX at `dist/languageServer.js`; the CI job has `continue-on-error: true` since the extracted server may behave differently outside VS Code extension host context. MongoDB requires a live `mongo:7` service container; the CI job provisions it automatically.

## Tools

All tools require `start_lsp` to be called first.

**CI coverage:** The following tools are end-to-end integration-tested against real language servers on every CI run across all 30 languages (34 tools in the multi-language harness; 47/50 total across all test suites):

- **Tier 1** (4 tools, all 30 languages): `start_lsp`, `open_document`, `get_diagnostics`, `get_info_on_location`
- **Tier 2** (34 tools): `get_document_symbols`, `go_to_definition`, `get_references`, `get_completions`, `get_workspace_symbols`, `format_document`, `go_to_declaration`, `type_hierarchy`, `get_info_on_location`, `call_hierarchy`, `get_semantic_tokens`, `get_signature_help`, `get_document_highlights`, `get_inlay_hints`, `get_code_actions`, `prepare_rename`, `rename_symbol`, `get_server_capabilities`, `add_workspace_folder`, `go_to_type_definition`, `go_to_implementation`, `format_range`, `apply_edit`, `detect_lsp_servers`, `close_document`, `did_change_watched_files`, `run_build`, `run_tests`, `get_tests_for_file`, `get_symbol_source`, `go_to_symbol`, `restart_lsp_server`, `set_log_level`, `execute_command`

Speculative session tools (`create_simulation_session`, `simulate_edit`, `simulate_edit_atomic`, `simulate_chain`, `evaluate_session`, `commit_session`, `discard_session`, `destroy_session`) are covered by `TestSpeculativeSessions` in `test/speculative_test.go`. 47 of 50 tools are covered across the three test suites; `get_change_impact` and `get_cross_repo_references` will be added in a future CI run.

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
| `get_diagnostics` | Errors and warnings; omit `file_path` for whole project |
| `get_info_on_location` | Hover info (type signatures, docs) at a position |
| `get_completions` | Completion suggestions at a position |
| `get_signature_help` | Function signature and active parameter at a call site |
| `get_code_actions` | Quick fixes and refactors for a range |
| `get_document_symbols` | All symbols in a file (functions, classes, variables) |
| `get_workspace_symbols` | Search symbols by name across the workspace; `detail_level=hover` enriches results with type signatures and docs without loading files into context |
| `get_semantic_tokens` | Classify tokens in a range as function/parameter/variable/type/keyword/etc; the same data IDEs use for syntax highlighting |
| `get_inlay_hints` | Inline type annotations and parameter name labels for a range; inferred type hints IDEs overlay on source code (Type and Parameter kinds) |
| `get_change_impact` | Enumerate all exported symbols in one or more files, resolve their references across the workspace, and partition callers into test vs non-test; use before editing a file to understand blast radius |
| `get_cross_repo_references` | Find all references to a library symbol across one or more consumer repos; adds consumer roots as workspace folders and partitions results by repo; use before changing a shared library API |

### Navigation
| Tool | Description |
|------|-------------|
| `get_references` | All references to a symbol across the workspace |
| `get_document_highlights` | All occurrences of a symbol in the current file; file-scoped, instant, returns read/write/text kinds; faster than `get_references` for local usage analysis |
| `go_to_definition` | Jump to where a symbol is defined (with fuzzy position fallback) |
| `go_to_type_definition` | Jump to the type definition of a symbol |
| `go_to_implementation` | Jump to all implementations of an interface or abstract method |
| `go_to_declaration` | Jump to the declaration of a symbol (distinct from definition; e.g. C/C++ headers) |
| `call_hierarchy` | Callers and/or callees of a function; `direction: "incoming"`, `"outgoing"`, or `"both"` (default) |
| `type_hierarchy` | Supertypes and/or subtypes of a type; `direction: "supertypes"`, `"subtypes"`, or `"both"` (default) |

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
| `did_change_watched_files` | Manually notify the server of file changes; not needed for normal edits (auto-watch handles those); use when an external process changes files outside the session |
| `get_server_capabilities` | Return the server capability map and classify every tool as supported or unsupported; use before calling capability-gated tools |
| `set_log_level` | Change log verbosity at runtime |
| `detect_lsp_servers` | Scan a workspace for source languages and check PATH for installed LSP servers; returns detected languages, server paths, and a `suggested_config` array ready to paste into your MCP config |

### Workspace
| Tool | Description |
|------|-------------|
| `add_workspace_folder` | Add a directory to the LSP workspace; enables cross-repo references, definitions, and diagnostics across library + consumer repos in one session |
| `remove_workspace_folder` | Remove a directory from the LSP workspace |
| `list_workspace_folders` | Return the current workspace folder list |

### Speculative Execution
Safe what-if analysis: simulate edits in-memory, evaluate diagnostic changes (errors introduced/resolved), then commit or discard atomically. No disk writes until you call `commit_session`.

| Tool | Description |
|------|-------------|
| `create_simulation_session` | Create a session with baseline diagnostics for a file |
| `simulate_edit` | Apply an in-memory edit to the session (no disk write) |
| `simulate_edit_atomic` | Apply an edit, evaluate diagnostics, and discard in one call; returns net error delta; accepts optional `session_id` to reuse an existing session |
| `simulate_chain` | Apply a sequence of edits and evaluate after each step; use as a **refactor preview** or **safe rename preview** — chain definition + call-site edits, check `cumulative_delta == 0`, commit or discard |
| `evaluate_session` | Compare current in-memory diagnostics against baseline; returns errors introduced and resolved |
| `commit_session` | Write the session's edits to disk |
| `discard_session` | Revert in-memory edits without touching disk |
| `destroy_session` | Release all session resources |

See [docs/speculative-execution.md](./docs/speculative-execution.md) for session lifecycle examples and refactor/rename preview workflows.

**Recommended agent workflow:**
```
start_lsp(root_dir="/your/project")
open_document(file_path=..., language_id=...)
get_diagnostics()                          # whole project, no file_path
get_info_on_location(...) / get_references(...)
close_document(...)
```

**Keeping the index fresh:**

agent-lsp watches the workspace root for file changes and automatically notifies the language server; no `did_change_watched_files` calls required after edits. The watcher skips high-churn directories (`.git/`, `node_modules/`, `target/`, etc.) and debounces rapid edits at 150ms.

`did_change_watched_files` is still available for cases where files are changed by an external process that the watcher may not see immediately, or for explicit control over change notifications.

**Rename workflow** (`prepare_rename` → `rename_symbol` → `apply_edit`):
```
prepare_rename(file_path=..., line=..., column=...)        # confirm rename is valid at this position
rename_symbol(file_path=..., line=..., column=..., new_name="newName")  # returns WorkspaceEdit
apply_edit(edit=<WorkspaceEdit>)                           # writes all changed files to disk
# auto-watch notifies the server automatically — no did_change_watched_files needed
```

**Language IDs:** `typescript`, `typescriptreact`, `javascript`, `javascriptreact`, `python`, `go`, `rust`, `java`, `kotlin`, `scala`, `swift`, `lua`, `zig`, `terraform`, `c`, `cpp`, `csharp`, `php`, `ruby`, `css`, `html`, `yaml`, `json`, `dockerfile`, `gleam`, `elixir`, `prisma`

## Resources

Diagnostic resources support real-time subscriptions; the server sends `notifications/resources/updated` when diagnostics change for a subscribed file.

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
- Progress protocol: workspace-ready detection waits for all `$/progress` tokens to complete before sending references
- Server-initiated requests (`workspace/configuration`, `window/workDoneProgress/create`, dynamic registration): all correctly responded to, unblocking servers that gate workspace loading on these responses
- Correct JSON-RPC framing, error code handling, and response shape normalization across hover, completion, code actions, and diagnostics

See [docs/lsp-conformance.md](./docs/lsp-conformance.md) for the full method coverage matrix and spec section references.

See [docs/tools.md](./docs/tools.md) for the full tool reference with example inputs and outputs.

See [docs/architecture.md](./docs/architecture.md) for the Go package structure, `WithDocument` pattern, URI handling, and resource subscription internals.

## Extensions

Language-specific extensions add tools, prompts, and resource handlers, registered automatically by language ID at startup.

To add an extension, create `extensions/<language-id>/` implementing any subset of the extension interface and register it via `extensions.RegisterFactory` in an `init()` function. All features are namespaced by language ID.

## Roadmap

| Feature | Status | Description |
|---------|--------|-------------|
| **Prebuilt binaries** | Planned | GoReleaser publishing `.tar.gz`/`.zip` binaries for Linux, macOS, and Windows to GitHub Releases on every tag — eliminates the `go install` requirement for non-Go developers |
| **Homebrew tap** | Planned | `brew install blackwell-systems/tap/agent-lsp` — one-command install for Mac users, backed by GoReleaser artifacts |
| **`curl \| sh` installer** | Planned | `curl -fsSL .../install.sh \| sh` — detects OS/arch, downloads the correct binary from GitHub Releases, places it on PATH; standard entry point for Linux and CI environments |
| **`agent-lsp init`** | Planned | Interactive setup command: runs `detect_lsp_servers`, asks which AI tool you use (Claude Code, Cursor, etc.), and writes the correct MCP config block — turns manual setup into one command |
| **Docker Hub distribution** | Planned | Mirror published images to Docker Hub (`docker pull agentlsp/agent-lsp:go`) for discoverability, pull count visibility, and access to users who default to Hub over ghcr.io |

## Development

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git
cd agent-lsp && go build ./...
go test ./...                   # all unit test suites
go test ./... -tags integration # integration tests (requires language servers)
```

## Library Usage

The `pkg/lsp`, `pkg/session`, and `pkg/types` packages expose a stable
public API for using agent-lsp's LSP client and speculative execution engine
directly from Go programs, without running the MCP server.

### Import the LSP client

```go
import (
    "context"
    "github.com/blackwell-systems/agent-lsp/pkg/lsp"
)

client := lsp.NewLSPClient("gopls", []string{})
if err := client.Initialize(ctx, "/path/to/workspace"); err != nil {
    log.Fatal(err)
}
defer client.Shutdown(ctx)

locs, err := client.GetDefinition(ctx, fileURI, lsp.Position{Line: 10, Character: 4})
```

### Import types

```go
import "github.com/blackwell-systems/agent-lsp/pkg/types"

var pos types.Position = types.Position{Line: 0, Character: 0}
```

### Speculative editing (simulate-before-apply)

```go
import (
    "github.com/blackwell-systems/agent-lsp/pkg/lsp"
    "github.com/blackwell-systems/agent-lsp/pkg/session"
)

mgr := session.NewSessionManager(client) // client is a *lsp.LSPClient
id, _ := mgr.CreateSession(ctx, "/workspace", "go")
mgr.ApplyEdit(ctx, id, fileURI, editRange, newText)
result, _ := mgr.Evaluate(ctx, id, "file", 3000)
if result.NetDelta == 0 {
    mgr.Commit(ctx, id, "", true)
} else {
    mgr.Discard(ctx, id)
}
```

All `pkg/` types are aliases of the internal implementation types and are
fully interchangeable with them.

## License

MIT
