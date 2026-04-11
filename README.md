# agent-lsp

[![Blackwell Systems](https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg)](https://github.com/blackwell-systems)
[![LSP 3.17](https://img.shields.io/badge/LSP-3.17-blue.svg)](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
[![Languages](https://img.shields.io/badge/languages-30_CI--verified-brightgreen.svg)](#multi-language-support)
[![CI Coverage](https://img.shields.io/badge/CI--verified_tools-50%2F50-brightgreen.svg)](#tools)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Agent Skills](assets/badge-agentskills.svg)](https://agentskills.io)

**agent-lsp makes code operations reliable for AI agents.**

It is a **stateful runtime** over real language servers, not a bridge. It keeps the language server's semantic index warm and adds a **skill layer** that turns multi-step code operations into single, correct workflows.

Most MCP-LSP tools fail in practice:

- **Stateless bridges** — no session, no context, no cross-file awareness
- **Raw tools** — agents skip steps or use them incorrectly

The tools exist. The workflow doesn't reliably happen.

agent-lsp fixes both. The **persistent session** indexes your workspace once and keeps it warm. The **skill layer** encodes correct tool sequences so workflows actually happen.

**Example:** call `/lsp-rename` and it will validate the rename, preview all affected files, show diagnostic impact, and apply atomically. One command. No missed steps.

**50 tools. 49 CI-verified end-to-end. 30 languages.** Built to [LSP 3.17 spec](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/).

**Work across all your projects in one session.** Point your AI at `~/code/`. One agent-lsp process routes `.go` to gopls, `.ts` to typescript-language-server, `.py` to pyright — no reconfiguration when you switch projects.

## Skills

Raw tools get ignored. Skills get used. Each skill encodes the correct tool sequence so workflows actually happen without per-prompt orchestration instructions.

| Skill | Purpose |
|-------|---------|
| `/lsp-safe-edit` | Speculative preview before disk write; before/after diagnostic diff; surfaces code actions on errors |
| `/lsp-edit-export` | Safe editing of exported symbols — finds all callers first |
| `/lsp-edit-symbol` | Edit a named symbol without knowing its file or position |
| `/lsp-rename` | `prepare_rename` safety gate, preview all sites, confirm, apply atomically |
| `/lsp-verify` | Diagnostics + build + tests after every edit |
| `/lsp-simulate` | Test changes in-memory without touching the file |
| `/lsp-impact` | Blast-radius analysis before touching a symbol or file |
| `/lsp-dead-code` | Detect zero-reference exports before cleanup |
| `/lsp-implement` | Find all concrete implementations of an interface |
| `/lsp-docs` | Three-tier documentation: hover → offline toolchain → source |
| `/lsp-cross-repo` | Find all usages of a library symbol across consumer repos |
| `/lsp-local-symbols` | File-scoped symbol list, usage search, and type info |
| `/lsp-test-correlation` | Find and run only tests that cover an edited file |
| `/lsp-format-code` | Format a file or selection via the language server formatter |

```bash
cd skills && ./install.sh
```

## Docker

```bash
# Go
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls

# TypeScript
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio

# Python
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio
```

Images are also mirrored to Docker Hub (`blackwellsystems/agent-lsp`). See [DOCKER.md](./DOCKER.md) for full tag list and docker-compose setup.

## Installation

```bash
# curl | sh (Linux / macOS)
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh

# Homebrew
brew install blackwell-systems/tap/agent-lsp

# npm
npm install -g @blackwell-systems/agent-lsp

# Go install
go install github.com/blackwell-systems/agent-lsp@latest
```

## Quick start

```bash
agent-lsp init
```

Detects language servers on your PATH, asks which AI tool you use, and writes the correct MCP config. For CI or scripted use: `agent-lsp init --non-interactive`.

## Setup

### Step 1: Install language servers

Install the servers for your stack. Common ones:

| Language | Server | Install |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |

Full list of 30 supported languages in [docs/language-support.md](./docs/language-support.md).

### Step 2: Add to your AI config

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

Each arg is `language:server-binary` (comma-separate server args).

### Step 3: Start working

```
start_lsp(root_dir="/your/project")
```

Then use any of the 50 tools. The session stays warm; no restart needed when switching files.

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
| Distribution | **single Go binary** | Node.js or Bun runtime required |

## Use Cases

- **Multi-project sessions**: point your AI at `~/code/`, work across any project without reconfiguring
- **Polyglot development**: Go backend + TypeScript frontend + Python scripts in one session
- **Large monorepos**: one server handles all languages, routes by file extension
- **Code migration**: refactor across repos with full cross-repo reference tracking
- **CI pipelines**: validate against real language server behavior

## Multi-Language Support

30 languages, CI-verified end-to-end against real language servers on every CI run. No other MCP-LSP implementation has an equivalent test matrix.

See [docs/language-support.md](./docs/language-support.md) for the full coverage matrix and per-language CI notes.

## Tools

50 tools covering navigation, analysis, refactoring, speculative execution, and session lifecycle. All CI-verified.

See [docs/tools.md](./docs/tools.md) for the full reference with parameters and examples.

## Further reading

- [docs/tools.md](./docs/tools.md) — full tool reference
- [docs/language-support.md](./docs/language-support.md) — language coverage matrix
- [docs/speculative-execution.md](./docs/speculative-execution.md) — simulate-before-apply workflows
- [docs/lsp-conformance.md](./docs/lsp-conformance.md) — LSP 3.17 spec coverage
- [docs/architecture.md](./docs/architecture.md) — Go package structure and internals
- [docs/ci-notes.md](./docs/ci-notes.md) — CI quirks and test harness details
- [docs/distribution.md](./docs/distribution.md) — install channels and release pipeline
- [DOCKER.md](./DOCKER.md) — Docker tags, compose, and volume caching

## Development

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git
cd agent-lsp && go build ./...
go test ./...                   # unit tests
go test ./... -tags integration # integration tests (requires language servers)
```

## Library Usage

The `pkg/lsp`, `pkg/session`, and `pkg/types` packages expose a stable Go API for using agent-lsp's LSP client directly without running the MCP server.

```go
import "github.com/blackwell-systems/agent-lsp/pkg/lsp"

client := lsp.NewLSPClient("gopls", []string{})
client.Initialize(ctx, "/path/to/workspace")
defer client.Shutdown(ctx)

locs, err := client.GetDefinition(ctx, fileURI, lsp.Position{Line: 10, Character: 4})
```

See [docs/architecture.md](./docs/architecture.md) for the full package API.

## License

MIT
