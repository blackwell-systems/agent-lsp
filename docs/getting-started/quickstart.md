---
title: Quick Start
---

# Quick Start

After [installing agent-lsp](installation.md), follow these steps to get up and running.

## 1. Install language servers

Install the servers for your stack:

| Language | Server | Install |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |

Full list of 30 supported languages in [language support](../language-support.md).

## 2. Configure your AI tool

```bash
agent-lsp init
```

This detects language servers on your PATH, asks which AI tool you use, and writes the correct MCP config. For CI or scripted use: `agent-lsp init --non-interactive`.

The generated config looks like:

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

## 3. Install skills

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git /tmp/agent-lsp-skills
cd /tmp/agent-lsp-skills/skills && ./install.sh --copy
```

Skills are prompt files copied into your AI tool's configuration -- `--copy` means the clone can be safely deleted afterward.

Skills are multi-tool workflows that encode reliable procedures -- blast-radius check before edit, speculative preview before write, test run after change. See the [skills reference](../skills.md) for the full list.

## 4. Start working

Your AI agent calls tools automatically. The first call initializes the workspace:

```
start_lsp(root_dir="/your/project")
```

This is what the agent does, not something you type. Then use any of the 60 tools. The session stays warm; no restart needed when switching files.

**What to try first:** Ask your AI agent to "find all references to [some function]" or "rename [function] to [new name]." The agent will call `find_references` or the `/lsp-rename` skill automatically. You can also ask "what calls this function?" (triggers `find_callers`) or "check for errors in this file" (triggers `get_diagnostics`).

## Verify setup

At any point, run:

```bash
agent-lsp doctor
```

This probes each configured language server and reports capabilities. Fix any failures before proceeding.
