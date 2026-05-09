# agent-lsp

**MCP server for language intelligence.** Give AI agents structured access to language servers: go-to-definition, find-all-references, rename, diagnostics, completions, call hierarchy, and speculative execution.

53 tools. 22 agent workflows. 30 CI-verified languages. Single Go binary.

## Why

AI coding agents navigate code with grep and file reads. This wastes 5-34x more tokens than necessary and produces 92-99% false positives on symbol lookups. Language servers already have the answers; agent-lsp makes them accessible via MCP.

## Install

```bash
pip install agent-lsp
```

Or via other channels:

```bash
brew install blackwell-systems/tap/agent-lsp    # macOS/Linux
npm install -g @blackwell-systems/agent-lsp     # npm
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```

## Usage

```bash
# Start with auto-detection
agent-lsp

# Explicit language server
agent-lsp go:gopls typescript:typescript-language-server,--stdio

# HTTP mode
agent-lsp --http --port 8080 go:gopls
```

Then configure your AI tool's MCP settings to point at agent-lsp.

## What it does

- **53 tools** covering navigation, analysis, refactoring, diagnostics, formatting, speculative execution, build, test, and more
- **22 agent workflows** (skills) that encode correct multi-step operations: rename safely, analyze blast radius, simulate edits before applying
- **30 CI-verified languages**: Go, Python, TypeScript, Rust, Java, C, C++, C#, Ruby, PHP, Kotlin, Swift, Scala, Zig, Lua, Elixir, Gleam, and more
- **Speculative execution**: preview edits in memory, see what breaks before touching disk
- **Phase enforcement**: blocks out-of-order operations so agents follow correct workflows

## Token savings

Measured across 5 codebases (Go, TypeScript, Python):

| Codebase | Lines | Savings |
|----------|------:|--------:|
| agent-lsp | 15K | 5x |
| Hono (TypeScript) | 24K | 13x |
| FastAPI (Python) | 33K | 2x |
| Next.js (TypeScript) | 196K | 5x |
| HashiCorp Consul (Go) | 319K | 34x |

Full experiment: [agent-lsp.com/token-savings](https://agent-lsp.com/token-savings)

## Works with

Claude Code, Cursor, Windsurf, GitHub Copilot, and any MCP-compatible client.

## Links

- [GitHub](https://github.com/blackwell-systems/agent-lsp)
- [Documentation](https://agent-lsp.com)
- [Token savings experiment](https://agent-lsp.com/token-savings)
- [Changelog](https://github.com/blackwell-systems/agent-lsp/blob/main/CHANGELOG.md)
