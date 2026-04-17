# Docker Distribution

agent-lsp ships pre-built images on [GitHub Container Registry](https://github.com/blackwell-systems/agent-lsp/pkgs/container/agent-lsp) (`ghcr.io/blackwell-systems/agent-lsp`) and [Docker Hub](https://hub.docker.com/r/blackwellsystems/agent-lsp) (`blackwellsystems/agent-lsp`). Both registries carry the same images and tags. GHCR is the primary registry; Docker Hub is a mirror updated automatically on every release.

## All available tags

| Tag | Contents | Approx. Size |
|-----|----------|--------------|
| `latest` | agent-lsp binary only | ~50 MB |
| `go` | Go + gopls | ~200 MB |
| `typescript` | Node.js + typescript-language-server | ~300 MB |
| `python` | Node.js + pyright-langserver | ~300 MB |
| `ruby` | Ruby + solargraph | ~400 MB |
| `cpp` | clangd | ~150 MB |
| `php` | Node.js + intelephense | ~300 MB |
| `web` | TypeScript + Python | ~400 MB |
| `backend` | Go + Python | ~500 MB |
| `fullstack` | Go + TypeScript + Python | ~600 MB |
| `full` | Go, TypeScript, Python, Ruby, C/C++, PHP | ~1–2 GB |

> **Warning:** The `full` image is approximately 1–2 GB. Unless you need all baked-in language servers immediately available, prefer a per-language or combo tag.

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
| Need a combination not listed above | Build a custom image — see below |

If your language isn't listed above, use `:latest` with the `LSP_SERVERS` env var — see [Runtime Install](#runtime-install-lsp_servers).

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

## Per-Language Tags

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

## Custom Images

If you need a combination not covered by the pre-built tags — for example Go + Ruby, or TypeScript + Rust — extend `:latest` with your own Dockerfile:

```dockerfile
FROM ghcr.io/blackwell-systems/agent-lsp:latest

# Go + gopls
RUN apt-get update && apt-get install -y --no-install-recommends curl && \
    LATEST_GO=$(curl -fsSL "https://go.dev/VERSION?m=text" | head -1) && \
    curl -fsSL "https://dl.google.com/go/${LATEST_GO}.linux-amd64.tar.gz" | tar -C /usr/local -xz && \
    GOPATH=/usr/local /usr/local/go/bin/go install golang.org/x/tools/gopls@latest && \
    rm -rf /var/lib/apt/lists/*
ENV PATH=$PATH:/usr/local/go/bin

# Ruby + solargraph
RUN apt-get update && apt-get install -y --no-install-recommends ruby ruby-dev build-essential && \
    gem install solargraph && \
    rm -rf /var/lib/apt/lists/*
```

Build and run:

```bash
docker build -t agent-lsp-custom .
docker run --rm -i \
  -v /your/project:/workspace \
  agent-lsp-custom \
  go:gopls ruby:solargraph
```

The `docker/Dockerfile.lang` in the repo shows the exact install patterns used for each supported language server — use it as a reference when adding new servers.

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

## HTTP Mode

By default agent-lsp communicates over stdio — the MCP client spawns the process directly. HTTP mode lets agent-lsp run as a persistent service that remote clients connect to over HTTP+SSE. This is the right choice when:

- The container runs on a remote host or VM and the MCP client is elsewhere
- You want a shared server for multiple agents connecting concurrently
- CI pipelines need a long-lived LSP index without cold-start cost on every run

### Starting in HTTP mode

```bash
# docker run — bind port 8080, auth via env var
docker run --rm \
  -p 8080:8080 \
  -v /your/project:/workspace \
  -e AGENT_LSP_TOKEN=your-secret-token \
  ghcr.io/blackwell-systems/agent-lsp:go \
  --http --port 8080 go:gopls
```

> **Auth token:** Always set `AGENT_LSP_TOKEN`. When unset, the server starts unauthenticated and logs a warning. Pass the token as an environment variable — never via `--token` on the command line, which would expose it in `ps` output.

### Hardened run (recommended for production)

```bash
docker run --rm \
  -p 8080:8080 \
  -v /your/project:/workspace:ro \
  -e AGENT_LSP_TOKEN=your-secret-token \
  --cap-drop=ALL \
  --read-only \
  --tmpfs /tmp \
  ghcr.io/blackwell-systems/agent-lsp:go \
  --http --port 8080 go:gopls
```

The image already runs as uid/gid 65532 (`nonroot`) — `--cap-drop=ALL` drops all remaining Linux capabilities. Mount the workspace `:ro` if you only need read-only analysis.

### MCP client configuration for HTTP mode

```json
{
  "mcpServers": {
    "lsp": {
      "type": "http",
      "url": "http://localhost:8080",
      "headers": {
        "Authorization": "Bearer your-secret-token"
      }
    }
  }
}
```

### docker-compose HTTP service

The included `docker/docker-compose.yml` has a ready-made `agent-lsp-http` service:

```bash
# Set token in .env
echo "AGENT_LSP_TOKEN=your-secret-token" >> .env
echo "WORKSPACE_DIR=/your/project" >> .env

# Start HTTP service
docker compose -f docker/docker-compose.yml up agent-lsp-http
```

The service binds `${AGENT_LSP_HTTP_PORT:-8080}:8080` and reads the token from `AGENT_LSP_TOKEN` in the environment — it does not pass the token as a CLI argument.

### Port configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--http` | — | Enable HTTP+SSE transport (off by default) |
| `--port N` | `8080` | TCP port to listen on (1–65535) |
| `AGENT_LSP_TOKEN` | — | Bearer token for auth (empty = no auth, not recommended) |

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

## Security

The agent-lsp image is hardened by default:

- **Non-root:** Runs as uid/gid 65532 (`nonroot`). No `sudo`, no root shell.
- **No credential in process list:** Auth token is read from the `AGENT_LSP_TOKEN` environment variable, not from `--token` CLI arg (which would be visible in `ps aux` and `/proc/<pid>/cmdline`).
- **HTTP timeouts:** The HTTP server enforces `ReadHeaderTimeout: 10s` and `ReadTimeout: 30s` to prevent Slowloris-style resource exhaustion.
- **Entrypoint whitelist:** `LSP_SERVERS` installs are dispatched through a strict package-manager whitelist — no `eval` of arbitrary shell strings.

Additional hardening you can apply at runtime:

```bash
--cap-drop=ALL          # drop all Linux capabilities
-v /project:/workspace:ro  # read-only workspace if no write-back needed
--read-only --tmpfs /tmp   # read-only root filesystem
```

## Notes

- The workspace is mounted read-write so code actions (quick fixes, auto-imports) can modify files; mount `:ro` if only read-only analysis is needed
- The `agent-lsp` binary is statically linked — the container image needs only the language server binaries, not a Go runtime
- File change detection behavior depends on the language server; no container-specific watcher configuration is needed for the MCP server itself
- Per-language images use a two-stage build: the binary is compiled in a Go builder stage and copied into a `debian:bookworm-slim` base; only the language server tools and the static binary end up in the final image
- `HOME` is set to `/tmp` (writable by the `nonroot` user); language servers that cache to `$HOME` will use `/tmp`
