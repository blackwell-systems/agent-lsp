# Distribution

This document describes how agent-lsp is distributed, what is automated, and what is still planned.

## Current channels

### GitHub Releases
Pre-built binaries for all platforms, published automatically by GoReleaser on every `v*` tag.

| Platform | Architecture |
|----------|-------------|
| macOS | arm64, amd64 |
| Linux | arm64, amd64 |
| Windows | arm64, amd64 |

### Homebrew
```bash
brew install blackwell-systems/tap/agent-lsp
```
Formula in [blackwell-systems/homebrew-tap](https://github.com/blackwell-systems/homebrew-tap) is updated automatically by GoReleaser on every release.

### curl | sh (macOS / Linux)
```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```
Detects OS and architecture, downloads the matching binary from GitHub Releases, installs to `/usr/local/bin`.

### PowerShell (Windows)
```powershell
iwr -useb https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.ps1 | iex
```
Detects amd64/arm64, downloads the matching zip from GitHub Releases, installs to `%LOCALAPPDATA%\agent-lsp`, adds to user PATH. No admin required.

### Scoop (Windows)
```powershell
scoop bucket add blackwell-systems https://github.com/blackwell-systems/agent-lsp
scoop install blackwell-systems/agent-lsp
```
Manifest at `bucket/agent-lsp.json` in this repo (the repo doubles as the Scoop bucket). `autoupdate` is configured, so `scoop update agent-lsp` picks up new releases automatically.

### Winget (Windows)
```powershell
winget install BlackwellSystems.agent-lsp
```
Manifests at `winget/manifests/b/BlackwellSystems/agent-lsp/`. Submit new versions as a PR to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs). Copy the `winget/manifests/` directory structure, update version and hashes.

### npm
```bash
npm install -g @blackwell-systems/agent-lsp
```
Uses the optionalDependencies pattern (same as esbuild): a root package with a JS shim and six platform-specific packages each containing the native binary. npm installs only the package matching the current platform.

Published automatically by the `npm-publish` CI job after GoReleaser completes.

**Packages:**
- `@blackwell-systems/agent-lsp` (root; install this)
- `@blackwell-systems/agent-lsp-darwin-arm64`
- `@blackwell-systems/agent-lsp-darwin-x64`
- `@blackwell-systems/agent-lsp-linux-arm64`
- `@blackwell-systems/agent-lsp-linux-x64`
- `@blackwell-systems/agent-lsp-win32-x64`
- `@blackwell-systems/agent-lsp-win32-arm64`

### Docker (GHCR + Docker Hub)
```bash
# GHCR
docker pull ghcr.io/blackwell-systems/agent-lsp:latest

# Docker Hub
docker pull blackwellsystems/agent-lsp:latest

# Base image (same content, two registries)
docker pull ghcr.io/blackwell-systems/agent-lsp:latest

# Language-specific images
docker pull ghcr.io/blackwell-systems/agent-lsp:go
docker pull ghcr.io/blackwell-systems/agent-lsp:typescript
docker pull ghcr.io/blackwell-systems/agent-lsp:python

# Combo images
docker pull ghcr.io/blackwell-systems/agent-lsp:fullstack
```

All images are multi-arch (`linux/amd64` + `linux/arm64`) via Docker manifest lists. Native performance on Apple Silicon and AWS Graviton, with no Rosetta/QEMU emulation. Built and pushed to both registries automatically by GoReleaser on every `v*` tag. Tags: `latest`, `base`, semver (`0.1.2`, `0.1`), and per-language (`go`, `typescript`, `python`, `ruby`, `cpp`, `php`, `web`, `backend`, `fullstack`, `full`).

## MCP registries

### Official MCP Registry
Published automatically via `mcp-publisher` in CI using GitHub OIDC (no secrets required). PulseMCP ingests from the official registry weekly.

**Server name:** `io.github.blackwell-systems/agent-lsp`
**Status:** Live as of v0.1.2, verified at `registry.modelcontextprotocol.io`

```bash
curl "https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.blackwell-systems/agent-lsp"
```

### Glama
Listed at [glama.ai/mcp/servers/blackwell-systems/agent-lsp](https://glama.ai/mcp/servers/blackwell-systems/agent-lsp). Profile managed via `glama.json` in repo root. Score badge: A grade. Build verified; server passes Glama's automated inspection checks.

### PyPI
```bash
pip install agent-lsp
```
Platform-specific wheels containing the Go binary. Each wheel is tagged with the correct platform (e.g. `macosx_11_0_arm64`, `manylinux2014_x86_64`), so pip resolves the right one automatically. No Go toolchain required. Built and published automatically by the `pypi-publish` CI job on every release tag. View at [pypi.org/project/agent-lsp](https://pypi.org/project/agent-lsp/).

### Go install
```bash
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
```
Requires a Go toolchain. Builds from source and installs to `$GOPATH/bin`.

### Smithery
`smithery.yaml` in the repo root enables auto-indexing on Smithery. Auto-discovered from GitHub.

### cursor.directory
Submitted. Cursor detects 21 skill components from SKILL.md files. Listed under Developer Tools.

### mcpservers.org
Manually submitted. Free listing.

### Awesome MCP Servers
Listed. [PR #5145](https://github.com/punkpeye/awesome-mcp-servers/pull/5145) merged 2026-04-23. Badge added to README.

## Documentation site

**URL:** [agent-lsp.com](https://agent-lsp.com)

Built with mkdocs-material from the `docs/` folder. Deployed to GitHub Pages automatically on every push to `main` via `.github/workflows/docs.yml`. Custom domain via Cloudflare DNS (CNAME → `blackwell-systems.github.io`).

## Release pipeline

Every `git tag v*` push triggers three sequential CI jobs:

```
release              → GoReleaser: binaries, GitHub Release, Homebrew formula,
                       all 11 Docker images (GHCR + Docker Hub)
npm-publish          → downloads binaries from GitHub Release, publishes 7 npm packages
mcp-registry-publish → publishes metadata to official MCP Registry (GitHub OIDC)
```

Docker images are built inside the `release` job by GoReleaser (`dockers:` section). 22 images (11 tags × 2 architectures) are built and combined into 11 multi-arch manifest lists via `docker_manifests`. Base images build first so downstream images can pull them as their `FROM` layer.

## Marketing and Discovery

| Channel | Status | Notes |
|---------|--------|-------|
| **LinkedIn** | Ready | Token savings post drafted. |
| **Hacker News** | Not submitted | Token savings blog post is HN-ready. |
| **Reddit** | Not submitted | r/LocalLLaMA, r/golang, r/programming. |
| **Go Weekly** | Not submitted | Submit blog post link. |
| **Twitter/X** | Not active | Thread format works for the data. |
| **glama.ai** | Listed (A grade) | MCP server discovery. |
| **Product Hunt** | Not launched | Save for bigger release. |
| **YouTube** | Not started | "LSP vs grep side by side" demo. |

### Awesome Lists

| List | Stars | Status | Section |
|------|------:|--------|---------|
| punkpeye/awesome-mcp-servers | 86K | **Listed** | Already on the list |
| bradAGI/awesome-cli-coding-agents | 308 | **PR #75 open** | Agent infrastructure |
| appcypher/awesome-mcp-servers | 5.5K | Branch pushed, needs PR from browser | Development Tools |
| wong2/awesome-mcp-servers | 4K | Not submitted | Community Servers |
| TensorBlock/awesome-mcp-servers | 656 | Not submitted | Code Analysis & Quality |
| ai-boost/awesome-harness-engineering | 705 | Not submitted | Skills & MCP |
| ai-for-developers/awesome-ai-coding-tools | 1.7K | Not submitted | Code Search and Navigation |
| rohitg00/awesome-devops-mcp-servers | 979 | Not submitted | Coding Agents |
| hesreallyhim/awesome-claude-code | 42K | Not submitted | Tooling > Orchestrators |
| avelino/awesome-go | 172K | **Blocked until Sep 2026** (5-month history req) | Go Tools |

## Planned

| Channel | Notes |
|---------|-------|
| **Nix flake** | `nix run github:blackwell-systems/agent-lsp` |
| **mcp.so** | Top Google result for "MCP servers"; direct submission |
| **VS Code extension** | Zero-CLI-setup path for Copilot/Continue/Cline users |
