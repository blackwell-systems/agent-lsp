---
title: Installation
---

# Installation

## Recommended: install script

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```

## Homebrew (macOS / Linux)

```bash
brew install blackwell-systems/tap/agent-lsp
```

## npm

```bash
npm install -g @blackwell-systems/agent-lsp
```

## Go install

```bash
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
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

See [distribution](../distribution.md) for the full Docker tag list and HTTP mode setup.

## Windows

### PowerShell (no admin required)

```powershell
iwr -useb https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.ps1 | iex
```

### Scoop

```powershell
scoop bucket add blackwell-systems https://github.com/blackwell-systems/agent-lsp
scoop install blackwell-systems/agent-lsp
```

### Winget

```powershell
winget install BlackwellSystems.agent-lsp
```

## Verify your installation

```bash
agent-lsp doctor
```

This probes each configured language server and reports capabilities. Fix any failures before proceeding. See [language support](../language-support.md) for install commands and server-specific notes.
