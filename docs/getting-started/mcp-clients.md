# MCP Client Configuration

Copy-paste configs for connecting agent-lsp to your AI tool.

## Claude Code

Add to your project's `.mcp.json` or global `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "agent-lsp": {
      "command": "agent-lsp",
      "args": [],
      "env": {}
    }
  }
}
```

Then run `claude` in your project directory. agent-lsp will appear as an available MCP server. GCF output is enabled by default (30-51% fewer tokens).

## Cursor

Add to `.cursor/mcp.json` in your project root:

```json
{
  "mcpServers": {
    "agent-lsp": {
      "command": "agent-lsp",
      "args": []
    }
  }
}
```

Restart Cursor after adding the config. agent-lsp tools will appear in the tool picker.

## Windsurf

Add to your Windsurf MCP configuration:

```json
{
  "mcpServers": {
    "agent-lsp": {
      "command": "agent-lsp",
      "args": []
    }
  }
}
```

## Continue.dev

Add to `.continue/config.yaml`:

```yaml
mcpServers:
  - name: agent-lsp
    command: agent-lsp
```

## Choosing which language servers to run

The `args` you pass to `agent-lsp` select the language servers. There are four modes:

| Mode | Command / `args` | Behavior |
|------|------------------|----------|
| Auto-detect (default) | `[]` | Scans `PATH` for known servers (gopls, pyright, rust-analyzer, ...) and runs whatever it finds. |
| Explicit | `["go:gopls", "python:pyright-langserver,--stdio"]` | Runs exactly the listed servers. Each arg is `language:binary` (comma-separate the server's own args). |
| Config file | `["--config", "/path/to/agent-lsp.json"]` | Runs exactly the servers in the JSON file. Replaces auto-detection entirely. |
| **Merge config** | `["--merge-config", "/path/to/agent-lsp.json"]` | Auto-detects installed servers, then overlays the JSON file on top. |

### `--merge-config`: auto-detect plus overrides

Use `--merge-config` when you want auto-detection for most languages but need to
add a custom server or override a specific one, without re-listing every language
that `--config` would require.

Auto-detection forms the base set. Each entry in the config file then either
**overrides** an auto-detected server (matched by `language_id`, falling back to
the first entry in `extensions`) or **adds** a new one. Auto-detected servers you
do not mention are kept.

```json
{
  "servers": [
    { "language_id": "go", "extensions": ["go"], "command": ["/opt/gopls-nightly"] },
    { "language_id": "lua", "extensions": ["lua"], "command": ["lua-language-server"] }
  ]
}
```

With the file above and `--merge-config`:

- **go** uses `/opt/gopls-nightly` instead of the `gopls` found on `PATH` (override).
- **lua** is added, even though auto-detection does not know it (add).
- **python**, **typescript**, and every other auto-detected server keep running unchanged.

```json
{
  "mcpServers": {
    "agent-lsp": {
      "command": "agent-lsp",
      "args": ["--merge-config", "/path/to/agent-lsp.json"]
    }
  }
}
```

The config file format is identical to `--config` (a `{"servers": [...]}` object);
the only difference is that `--config` replaces auto-detection while `--merge-config`
extends it.

## HTTP Mode (Remote / Docker)

For remote deployments or shared servers, run agent-lsp in HTTP mode:

```bash
agent-lsp --http --port 8080 --token "$AGENT_LSP_TOKEN"
```

Then configure your MCP client to connect via HTTP:

```json
{
  "mcpServers": {
    "agent-lsp": {
      "url": "http://localhost:8080",
      "headers": {
        "Authorization": "Bearer your-secret-token"
      }
    }
  }
}
```

See [Docker documentation](../../DOCKER.md) for containerized deployments.

## Verifying the Connection

After configuring, test with any tool:

```
Use the start_lsp tool to initialize the workspace at /path/to/your/project
```

If the language server starts successfully, agent-lsp is connected and ready.
