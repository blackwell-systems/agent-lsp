# Troubleshooting

## Language server won't start

**Symptom:** `start_lsp` returns an error or times out.

**Check 1: Is the language server installed?**

agent-lsp orchestrates existing language servers. It doesn't bundle them. Verify the server is on your PATH:

```bash
# Go
gopls version

# TypeScript
typescript-language-server --version

# Python
pyright --version

# Rust
rust-analyzer --version

# Java
# jdtls is typically installed via VS Code or manually
```

If the server is installed but not on `PATH`, or you want a custom binary or
flags for one language while keeping auto-detection for the rest, run agent-lsp
with `--merge-config` and point the entry at the full path. See
[Choosing which language servers to run](mcp-clients.md#choosing-which-language-servers-to-run).

**Check 2: Spawn logs**

agent-lsp captures language server startup output:

```bash
cat ~/.cache/agent-lsp/spawn-logs/<language>.log
```

This shows the exact command used and any startup errors.

**Check 3: Increase broker timeout**

On slow machines or large workspaces:

```bash
export AGENT_LSP_BROKER_TIMEOUT_MS=60000  # 60 seconds
```

## "broker did not start within 10 seconds" (Windows)

This was a known issue fixed in v0.12.0. Upgrade to the latest version:

```bash
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
```

If the issue persists, check the spawn logs and ensure the broker timeout is sufficient.

## Empty results from queries

**Symptom:** `find_references`, `go_to_definition`, etc. return empty results.

**Cause:** The language server hasn't finished indexing.

**Fix:** Use `start_lsp` with `ready_timeout_seconds`:

```json
{"tool": "start_lsp", "args": {"root_dir": "/path/to/project", "ready_timeout_seconds": 30}}
```

This blocks until the language server reports its workspace index is complete. Servers like jdtls (Java) can take 30-60 seconds to import Gradle/Maven projects.

## "No client for file" errors

**Cause:** agent-lsp doesn't know which language server handles the file.

**Fix:** Call `start_lsp` with the project root before querying files:

```json
{"tool": "start_lsp", "args": {"root_dir": "/path/to/project"}}
```

agent-lsp auto-detects the language from file extensions. If auto-detection fails, specify `language_id` explicitly:

```json
{"tool": "start_lsp", "args": {"root_dir": "/path/to/project", "language_id": "go"}}
```

## Daemon mode issues

**Check daemon status:**

```bash
cat ~/.cache/agent-lsp/daemon.json
```

This shows running daemon PIDs and socket paths.

**Kill stale daemons:**

If a daemon is stuck, remove the PID file and restart:

```bash
rm ~/.cache/agent-lsp/daemon.json
```

## MCP client can't find agent-lsp

**Check:** Is `agent-lsp` on your PATH?

```bash
which agent-lsp
```

If not, install it:

```bash
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
```

Or use the full path in your MCP client config:

```json
{
  "mcpServers": {
    "agent-lsp": {
      "command": "/full/path/to/agent-lsp",
      "args": []
    }
  }
}
```

## Debug logging

Enable verbose logging to diagnose issues:

```bash
export AGENT_LSP_LOG_LEVEL=debug
```

Or at runtime via the `set_log_level` tool:

```json
{"tool": "set_log_level", "args": {"level": "debug"}}
```

See [Environment Variables](../reference/env-vars.md) for all configuration options.
