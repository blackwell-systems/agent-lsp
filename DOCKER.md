# Docker Distribution

agent-lsp ships pre-built images on [GitHub Container Registry](https://github.com/blackwell-systems/agent-lsp/pkgs/container/agent-lsp) (`ghcr.io/blackwell-systems/agent-lsp`) and [Docker Hub](https://hub.docker.com/r/blackwellsystems/agent-lsp) (`blackwellsystems/agent-lsp`). Images are organized into four tiers: a base image with only the binary, per-language images with one server baked in, combination images for common polyglot stacks, and a full image with every supported server installed. Pick the smallest image that covers your stack.

Both registries carry the same images and tags. GHCR is the primary registry; Docker Hub is a mirror updated automatically on every release.

## Quick Start

```bash
# GitHub Container Registry (primary)
docker run --rm -i \
  -v /your/project:/workspace \
  ghcr.io/blackwell-systems/agent-lsp:go \
  go:gopls

# Docker Hub (mirror — same image, same tags)
docker run --rm -i \
  -v /your/project:/workspace \
  blackwellsystems/agent-lsp:go \
  go:gopls
```

Replace `:go` with any per-language tag and adjust the trailing argument to match (see [Per-Language Tags](#per-language-tags)).

## Which tag should I use?

| I want to... | Tag |
|---|---|
| Try it quickly / install language servers at runtime | `:latest` |
| Use with Go only | `:go` |
| Use with TypeScript or JavaScript only | `:typescript` |
| Use with Python only | `:python` |
| Use with TypeScript + Python (web frontend stack) | `:web` |
| Use with Go + Python (backend stack) | `:backend` |
| Use with Go + TypeScript + Python | `:fullstack` |
| Use with Ruby, C/C++, or PHP | `:ruby` / `:cpp` / `:php` |
| Need all servers ready with no install delay | `:full` (1–2 GB) |

If your language isn't listed above, use `:latest` with the `LSP_SERVERS` env var — see [Runtime Install](#runtime-install-lsp_servers).

## Image Tiers

| Tag | Contents | Approx. Size |
|-----|----------|--------------|
| `latest` | agent-lsp binary only | ~50 MB |
| `go`, `typescript`, `python`, `ruby`, `cpp`, `php` | One language server baked in | ~150–500 MB |
| `web` | TypeScript + Python | ~400 MB |
| `backend` | Go + Python | ~500 MB |
| `fullstack` | Go + TypeScript + Python | ~600 MB |
| `full` | All automatable servers (Go, TypeScript, Python, Ruby, C/C++, PHP) | ~1–2 GB |

> **Warning:** The `full` image is approximately 1–2 GB. Unless you need all
> baked-in language servers immediately available, prefer a per-language tag or
> a combo tag (`web`, `backend`, `fullstack`).

## Per-Language Tags

These tags are published to ghcr.io and have a language server baked in.

| Language | Tag | Command |
|----------|-----|---------|
| Go | `go` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls` |
| TypeScript | `typescript` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio` |
| Python | `python` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio` |
| Ruby | `ruby` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:ruby ruby:solargraph` |
| C / C++ | `cpp` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:cpp cpp:clangd` |
| PHP | `php` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:php php:intelephense` |

The following languages require platform-specific toolchains that can't be reliably baked into a generic Debian image. Use [`LSP_SERVERS`](#runtime-install-lsp_servers) to install them at runtime, or build your own image on top of `:latest`:

| Language | `LSP_SERVERS` value | Notes |
|----------|--------------------|----|
| Rust | `rust-analyzer` | Install via `rustup component add rust-analyzer` |
| Java | `jdtls` | Download from eclipse.org/jdtls |
| C# | `csharp-ls` | `csharp-ls` NuGet package lacks global tool manifest |
| Kotlin | `kotlin-language-server` | Binary release from GitHub |
| Dart | `dart` | Requires Google's apt PPA |
| Scala | `metals` | Requires sbt compilation |
| Lua | `lua-language-server` | Binary release from GitHub |
| Elixir | `elixir-ls` | Build from source |
| Clojure | `clojure-lsp` | Binary release from GitHub |
| Zig | `zls` | Must match your Zig version exactly |
| Haskell | `haskell-language-server-wrapper` | Install via GHCup |
| Swift | `sourcekit-lsp` | Ships with Xcode; macOS only |

## Runtime Install (LSP_SERVERS)

Use the `:latest` image and install language servers at container start via the `LSP_SERVERS` environment variable. The entrypoint script reads `LSP_SERVERS`, looks up each name in the built-in registry (`docker/lsp-servers.yaml`), installs any server not already on PATH, and caches the install to `/var/cache/lsp-servers/` so subsequent starts are instant.

```bash
docker run --rm -i \
  -v /your/project:/workspace \
  -v lsp-server-cache:/var/cache/lsp-servers \
  -e LSP_SERVERS=gopls,typescript-language-server \
  ghcr.io/blackwell-systems/agent-lsp:latest \
  go:gopls typescript:typescript-language-server,--stdio
```

The named volume `lsp-server-cache` persists installed servers across container restarts. On the first run, servers are downloaded and cached; subsequent runs skip the install and start immediately.

**Supported `LSP_SERVERS` values** (comma-separated binary names):

`gopls`, `typescript-language-server`, `pyright-langserver`, `rust-analyzer`, `clangd`, `solargraph`, `intelephense`, `csharp-ls`, `lua-language-server`, `zls`, `kotlin-language-server`, `metals`, `elixir-ls`, `clojure-lsp`, `haskell-language-server-wrapper`, `sourcekit-lsp`, `jdtls`, `dart`

## Volume Caching

Mount a named volume at `/var/cache/lsp-servers` to persist language server installations across container restarts:

```bash
docker volume create lsp-server-cache

docker run --rm -i \
  -v /your/project:/workspace \
  -v lsp-server-cache:/var/cache/lsp-servers \
  -e LSP_SERVERS=gopls,typescript-language-server \
  ghcr.io/blackwell-systems/agent-lsp:latest \
  go:gopls typescript:typescript-language-server,--stdio
```

Without the volume, `LSP_SERVERS` installs run on every container start.

## MCP Client Configuration

### docker run (one-shot)

```json
{
  "mcpServers": {
    "lsp": {
      "type": "stdio",
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "/your/project:/workspace",
        "ghcr.io/blackwell-systems/agent-lsp:go",
        "go:gopls"
      ]
    }
  }
}
```

### docker compose run

```json
{
  "mcpServers": {
    "lsp": {
      "command": "docker",
      "args": [
        "compose",
        "-f", "/path/to/agent-lsp/docker/docker-compose.yml",
        "run", "--rm", "agent-lsp"
      ],
      "env": {
        "WORKSPACE_DIR": "/path/to/your/project"
      },
      "workingDirectory": "/path/to/agent-lsp"
    }
  }
}
```

## docker-compose Setup

1. Copy the example env file and set your project path:

   ```bash
   cp .env.example .env   # stays at repo root
   # Edit .env: set WORKSPACE_DIR to the absolute path of your project
   ```

2. Start the container:

   ```bash
   docker compose -f docker/docker-compose.yml up
   ```

The `docker/docker-compose.yml` uses the `web` combo image by default (TypeScript + Python). Edit the `image:` field to use a different tag.

## Resource Limits

Default resource limits (adjust in `docker/docker-compose.yml` for larger projects):

| Limit | Default |
|-------|---------|
| Memory limit | 4 GB |
| Memory reservation | 1 GB |
| CPU limit | 2 cores |
| CPU reservation | 0.5 cores |

No heap size configuration is needed — the Go binary has no managed heap tuning equivalent to Node's `--max-old-space-size`.

## Notes

- The workspace is mounted read-write so code actions (quick fixes, auto-imports) can modify files
- The `agent-lsp` binary is statically linked — the container image needs only the language server binaries, not a Go runtime
- File change detection behavior depends on the language server; no container-specific watcher configuration is needed for the MCP server itself
- Per-language images use a two-stage build: the binary is compiled in a Go builder stage and copied into a `debian:bookworm-slim` base; only the language server tools and the static binary end up in the final image
