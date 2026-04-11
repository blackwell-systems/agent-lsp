# Roadmap

## Distribution

| Feature | Status | Description |
|---------|--------|-------------|
| **Prebuilt binaries** | Done (v0.1.0) | GoReleaser publishing `.tar.gz`/`.zip` binaries for Linux, macOS, and Windows to GitHub Releases — eliminates the `go install` requirement for non-Go developers |
| **`agent-lsp init`** | Done (v0.1.0) | Interactive setup wizard: detects installed language servers, asks which AI tool you use, writes the correct MCP config — turns manual setup into one command |
| **Homebrew tap** | In progress | `brew install blackwell-systems/tap/agent-lsp` — formula exists, sha256s need updating after each release |
| **`curl \| sh` installer** | Planned | `curl -fsSL .../install.sh \| sh` — detects OS/arch, downloads the correct binary from GitHub Releases, places it on PATH |
| **Docker Hub mirroring** | Planned | Mirror published images to Docker Hub for discoverability and pull count visibility |

## Extensions

| Feature | Status | Description |
|---------|--------|-------------|
| **Go extension** | Planned | `go.test_run`, `go.test_coverage`, `go.mod_graph`, `go.mod_why`, `go.vulncheck`, `go.lint`, `go.escape_analysis`, `go.generate` — language-specific tools beyond what gopls exposes |
| **TypeScript extension** | Planned | `tsconfig.json` diagnostics, type coverage report |
| **Rust extension** | Planned | `cargo check` integration, crate dependency tree |

## Transport

| Feature | Status | Description |
|---------|--------|-------------|
| **HTTP/SSE transport** | Planned (v0.2) | Run agent-lsp as a persistent HTTP server; enables remote deployments, Docker without `-i`, and multi-client sessions sharing one warm index |
