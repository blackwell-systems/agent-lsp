# Docker Distribution

agent-lsp ships pre-built images on [GitHub Container Registry](https://github.com/blackwell-systems/agent-lsp/pkgs/container/agent-lsp) (`ghcr.io/blackwell-systems/agent-lsp`). Images are organized into four tiers: a base image with only the binary, per-language images with one server baked in, combination images for common polyglot stacks, and a full image with every supported server installed. Pick the smallest image that covers your stack.

## Quick Start

```bash
docker run --rm -i \
  -v /your/project:/workspace \
  ghcr.io/blackwell-systems/agent-lsp:go \
  go:gopls
```

Replace `:go` with any per-language tag and adjust the trailing argument to match (see [Per-Language Tags](#per-language-tags)).

## Image Tiers

| Tag | Contents | Approx. Size |
|-----|----------|--------------|
| `latest` / `base` | agent-lsp binary only | ~50 MB |
| `go`, `typescript`, `python`, ... | One language server baked in | ~150–500 MB |
| `web` | TypeScript + Python | ~400 MB |
| `backend` | Go + Python | ~500 MB |
| `fullstack` | Go + TypeScript + Python | ~600 MB |
| `full` | All 18 supported servers | ~2–3 GB |

> **Warning:** The `full` image is approximately 2–3 GB. Unless you need all
> language servers immediately available, prefer a per-language tag or a combo
> tag (`web`, `backend`, `fullstack`).

## Per-Language Tags

One `docker run` per language. Mount your project at `/workspace` and pass the language and server binary as arguments.

| Language | Tag | Command |
|----------|-----|---------|
| Go | `go` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls` |
| TypeScript | `typescript` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio` |
| Python | `python` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio` |
| Rust | `rust` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:rust rust:rust-analyzer` |
| Java | `java` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:java java:jdtls` |
| Ruby | `ruby` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:ruby ruby:solargraph` |
| C / C++ | `cpp` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:cpp cpp:clangd` |
| C# | `csharp` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:csharp csharp:csharp-ls` |
| Kotlin | `kotlin` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:kotlin kotlin:kotlin-language-server` |
| PHP | `php` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:php php:intelephense` |
| Scala | `scala` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:scala scala:metals` |
| Lua | `lua` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:lua lua:lua-language-server` |
| Elixir | `elixir` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:elixir elixir:elixir-ls` |
| Clojure | `clojure` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:clojure clojure:clojure-lsp` |
| Dart | `dart` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:dart dart:dart` |
| Zig | `zig` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:zig zig:zls` |
| Haskell | `haskell` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:haskell haskell:haskell-language-server-wrapper` |
| Swift | `swift` | `docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:swift swift:sourcekit-lsp` |

## Runtime Install (LSP_SERVERS)

Use the `base` / `latest` image and install language servers at container start via the `LSP_SERVERS` environment variable. The entrypoint script reads `LSP_SERVERS`, looks up each name in the built-in registry (`docker/lsp-servers.yaml`), installs any server not already on PATH, and caches the install to `/var/cache/lsp-servers/` so subsequent starts are instant.

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
        "-f", "/path/to/agent-lsp/docker-compose.yml",
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
   cp .env.example .env
   # Edit .env: set WORKSPACE_DIR to the absolute path of your project
   ```

2. Start the container:

   ```bash
   docker compose up
   ```

The `docker-compose.yml` uses the `web` combo image by default (TypeScript + Python). Edit the `image:` field to use a different tag.

## Resource Limits

Default resource limits (adjust in `docker-compose.yml` for larger projects):

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
