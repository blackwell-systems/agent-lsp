# agent-lsp

**The most complete MCP server for language intelligence.** 53 tools, 30 CI-verified languages, 21 agent workflows. Single Go binary.

---

AI agents make incorrect code changes because they can't see the full picture: who calls this function, what breaks if I rename it, does the build still pass. Language servers have the answers, but existing MCP bridges either cold-start on every request or expose raw tools that agents use incorrectly.

agent-lsp is a **stateful runtime** over real language servers. It indexes your workspace once, keeps the index warm, and adds a **skill layer** that encodes correct multi-step operations so they actually complete.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```

See all [installation methods](getting-started/installation.md) or jump to the [quick start guide](getting-started/quickstart.md).

## How the pieces fit together

[LSP](https://microsoft.github.io/language-server-protocol/) (Language Server Protocol) is how editors get code intelligence: completions, diagnostics, go-to-definition. [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) is the standard way AI tools like Claude Code discover and call external tools. agent-lsp bridges the two: language server intelligence, accessible to AI agents.

## How it works

One agent-lsp process manages your language servers. Point your AI at `~/code/`. It routes `.go` to gopls, `.ts` to typescript-language-server, `.py` to pyright. No reconfiguration when you switch projects. The session stays warm across files, packages, and repositories.

## Tested, not assumed

Every other MCP-LSP implementation lists supported languages in a config file. None of them run the actual language server in CI to verify it works.

agent-lsp CI runs **30 real language servers** against real fixture codebases on every push: Go, Python, TypeScript, Rust, Java, C, C++, C#, Ruby, PHP, Kotlin, Swift, Scala, Zig, Lua, Elixir, Gleam, Clojure, Dart, Terraform, Nix, Prisma, SQL, MongoDB, and more. When we say "works with gopls," that's a verified, automated claim, not a hope.

## Speculative execution

Simulate changes in memory before writing to disk. No other MCP-LSP implementation has this.

`simulate_edit_atomic` previews the diagnostic impact of any edit. You see exactly what breaks before the file is touched. `simulate_chain` evaluates a sequence of dependent edits and reports which step first introduces an error.

Read more in the [speculative execution docs](speculative-execution.md).

## Phase enforcement

Skills tell agents the correct order of operations. Phase enforcement makes the runtime *block* violations instead of trusting the agent to follow instructions.

When an agent activates a skill, every tool call is checked against the current phase's permissions. Calling `apply_edit` during blast-radius analysis returns an error with specific recovery guidance, not silence. Phases advance automatically as the agent progresses through the workflow.

No other MCP tool provider enforces workflow ordering at runtime. Read more in the [phase enforcement docs](phase-enforcement.md).

## Works with

| AI Tool | Transport | Config |
|---------|-----------|--------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | stdio | `mcpServers` in `.mcp.json` |
| [Continue](https://continue.dev) | stdio | `mcpServers` in `config.json` |
| [Cline](https://github.com/cline/cline) | stdio | `mcpServers` in settings |
| [Cursor](https://cursor.com) | stdio | `mcpServers` in settings |
| Any MCP client | HTTP+SSE | `--http --port 8080` with Bearer token auth |

## What's unique

| Capability | Details |
|------------|---------|
| Tools | **53** |
| Languages (CI-verified) | **30**, end-to-end integration tests on every push |
| Agent workflows (skills) | **21**, named multi-step procedures, discoverable via MCP `prompts/list` |
| Speculative execution | **8 tools**, simulate changes before writing to disk |
| Phase enforcement | **4 skills**, runtime blocks out-of-order tool calls with recovery guidance |
| Connection model | **persistent**, warm index across files and projects |
| Call hierarchy | single tool, direction param |
| Type hierarchy | CI-verified |
| Cross-repo references | multi-root workspace |
| Auto-watch | always-on, debounced file watching |
| HTTP+SSE transport | bearer token auth, non-root Docker |
| Distribution | **single Go binary**, 8 install channels |
