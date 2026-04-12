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

### curl | sh
```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```
Detects OS and architecture, downloads the matching binary from GitHub Releases, installs to `/usr/local/bin`.

### npm
```bash
npm install -g @blackwell-systems/agent-lsp
```
Uses the optionalDependencies pattern (same as esbuild): a root package with a JS shim and six platform-specific packages each containing the native binary. npm installs only the package matching the current platform.

Published automatically by the `npm-publish` CI job after GoReleaser completes.

**Packages:**
- `@blackwell-systems/agent-lsp` — root (install this)
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

Images are mirrored to Docker Hub automatically on every release. Tags: `latest`, `edge`, semver (`0.1.2`, `0.1`), and per-language (`go`, `typescript`, `python`, `ruby`, `cpp`, `php`, `web`, `backend`, `fullstack`, `full`).

## MCP registries

### Official MCP Registry
Published automatically via `mcp-publisher` in CI using GitHub OIDC (no secrets required). PulseMCP ingests from the official registry weekly.

**Server name:** `io.github.blackwell-systems/agent-lsp`
**Status:** Live as of v0.1.2 — verified at `registry.modelcontextprotocol.io`

```bash
curl "https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.blackwell-systems/agent-lsp"
```

### Smithery / Glama
A `smithery.yaml` in the repo root enables indexing on Smithery and Glama. These platforms auto-discover servers from GitHub and npm.

### mcpservers.org
Manually submitted. Free listing.

## Release pipeline

Every `git tag v*` push triggers three sequential CI jobs:

```
release          → GoReleaser: binaries, GitHub Release, Homebrew formula
npm-publish      → downloads binaries from GitHub Release, publishes 7 npm packages
mcp-registry-publish → publishes metadata to official MCP Registry (GitHub OIDC)
```

Docker images are built and pushed (GHCR + Docker Hub) in a separate `docker.yml` workflow. It triggers on both `main` branch pushes (publishes the `:edge` tag) and `v*` version tags (publishes `:latest`, semver tags, and per-language tags).

## Planned

| Channel | Notes |
|---------|-------|
| **Windows install script** | PowerShell script + Scoop/Chocolatey package |
| **Nix flake** | `nix run github:blackwell-systems/agent-lsp` |
| **Awesome MCP Servers** | PR to the curated GitHub list |
| **VS Code extension** | Zero-CLI-setup path for Copilot/Continue/Cline users |
