# Docker

Run lsp-mcp-go in a container with a fixed Go version, resource limits, and
no dependency on your local environment. The Go binary is statically linked —
the final image only needs the language server binaries, not a Go runtime.

## Setup

1. Copy the example env file and set your project path:
   ```bash
   cp .env.example .env
   # Edit .env: set WORKSPACE_DIR to the absolute path of your project
   ```

2. Build and start:
   ```bash
   docker compose up --build
   ```

## MCP Client Configuration

Point your MCP client at the running container:

```json
{
  "mcpServers": {
    "lsp": {
      "command": "docker",
      "args": [
        "compose",
        "-f", "/path/to/lsp-mcp-go/docker-compose.yml",
        "run", "--rm", "lsp-mcp-go"
      ],
      "env": {
        "WORKSPACE_DIR": "/path/to/your/project"
      },
      "workingDirectory": "/path/to/lsp-mcp-go"
    }
  }
}
```

## Using a Different Language Server

The default image installs `typescript-language-server`. To use a different
language server, override `CMD` at runtime:

```bash
# Go (gopls must be installed in the container or on PATH)
docker run --rm -i -v /your/project:/workspace lsp-mcp-go go gopls

# Rust (rust-analyzer must be installed in the container or on PATH)
docker run --rm -i -v /your/project:/workspace lsp-mcp-go rust rust-analyzer

# Python
docker run --rm -i -v /your/project:/workspace lsp-mcp-go python pyright-langserver --stdio
```

Or extend the Dockerfile to bake in your language server:

```dockerfile
FROM debian:bookworm-slim
# copy the lsp-mcp-go binary from a build stage, then add:
RUN apt-get install -y rust-analyzer
CMD ["rust", "rust-analyzer"]
```

## Resource Limits

Defaults (adjust in `docker-compose.yml` for larger projects):

| Limit | Default |
|-------|---------|
| Memory limit | 4 GB |
| Memory reservation | 1 GB |
| CPU limit | 2 cores |
| CPU reservation | 0.5 cores |

Note: No heap size configuration needed — the Go binary has no managed heap tuning equivalent to Node's `--max-old-space-size`.

## Notes

- The workspace is mounted read-write so code actions (quick fixes, auto-imports) can modify files
- The `lsp-mcp-go` binary is statically linked — the container image only needs language server binaries installed, not a Go runtime
- File change detection behavior depends on the language server; no container-specific watcher configuration is needed for the MCP server itself
