# MCP Client Configuration

Copy-paste configs for connecting agent-lsp to your AI tool.

## Claude Code

The easiest path is `agent-lsp init`, which writes the entry for you. Or register it with the Claude Code CLI: `claude mcp add agent-lsp -- agent-lsp` (or `claude mcp add --scope user agent-lsp -- agent-lsp` for all projects).

To edit the config by hand, add this to your project's `.mcp.json` (Claude Code keeps user-scoped servers in `~/.claude.json`; `~/.claude/settings.json` is not read for MCP servers):

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

## Codex and the ChatGPT desktop app

Codex (CLI and IDE extension) and the ChatGPT desktop app share one MCP configuration. Run `agent-lsp init` and choose `11` (user: `$CODEX_HOME/config.toml`, default `~/.codex/config.toml`) or `12` (project: `.codex/config.toml`, read for trusted projects). With the `codex` CLI installed, the user target runs `codex mcp add` for you.

To configure it by hand, either run:

```bash
codex mcp add agent-lsp -- agent-lsp go:gopls typescript:typescript-language-server,--stdio
```

or add a table to the config file:

```toml
[mcp_servers.agent-lsp]
command = "agent-lsp"
args = ["go:gopls", "typescript:typescript-language-server,--stdio"]
```

In the ChatGPT desktop app the server also appears under Settings, MCP servers. Codex reads skill awareness rules from `AGENTS.md`, which `init` writes (`$CODEX_HOME/AGENTS.md` for the user target, `AGENTS.md` in the project for the project target).

## Pi

Pi has no built-in MCP support; install the adapter package once:

```bash
pi install npm:pi-mcp-adapter
```

The adapter reads the shared standard MCP files. For a single project, `agent-lsp init` (choice 9) writes `.mcp.json` in the project root — the same file Claude Code uses:

```json
{
  "mcpServers": {
    "agent-lsp": {
      "type": "stdio",
      "command": "agent-lsp",
      "args": ["go:gopls"]
    }
  }
}
```

For all projects, `agent-lsp init` (choice 10) writes the same entry to `~/.config/mcp/mcp.json`. Skill awareness rules are written to `AGENTS.md` (project) or `~/.pi/agent/AGENTS.md` (global). Restart Pi (or run `/reload`) afterwards.

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
export AGENT_LSP_TOKEN=your-secret-token
agent-lsp --http --port 8080
```

The token is read from `AGENT_LSP_TOKEN`. Avoid the `--token` flag: it exposes the credential in the process list. Without a token the server refuses to start. The server binds `127.0.0.1` by default; pass `--listen-addr 0.0.0.0` to accept remote connections.

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

See [Docker documentation](https://github.com/blackwell-systems/agent-lsp/blob/main/DOCKER.md) for containerized deployments.

## Verifying the Connection

After configuring, test with any tool:

```
Use the start_lsp tool to initialize the workspace at /path/to/your/project
```

If the language server starts successfully, agent-lsp is connected and ready.
