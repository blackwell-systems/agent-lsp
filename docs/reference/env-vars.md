# Environment Variables

All environment variables are optional. agent-lsp works with no configuration.

## Runtime

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_LSP_OUTPUT_FORMAT` | `gcf` | Output encoding for tool responses. [GCF tabular encoding](../guide/gcf-integration.md) saves 30-51% tokens. Set to `json` to revert. |
| `AGENT_LSP_BROKER_TIMEOUT_MS` | `30000` | Timeout in milliseconds for the daemon broker to start. Increase on slow machines or when language servers take long to initialize. |
| `AGENT_LSP_TOKEN` | (none) | Bearer token for HTTP mode authentication. Required when running with `--http`. Never pass on the command line; use this env var instead. |

## Auto-watcher

The auto-watcher keeps the index fresh by watching the workspace for file changes. On macOS the fsnotify kqueue backend opens one file descriptor per watched file, so these caps bound fd usage on trees that contain large data/cache directories (see [#18](https://github.com/blackwell-systems/agent-lsp/issues/18)).

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_LSP_DISABLE_WATCHER` | (unset) | Set to `1`/`true` to disable the auto-watcher entirely. Clients still receive explicit `did_change_watched_files`. Use when a workspace is very large or contains actively-written data directories. |
| `AGENT_LSP_WATCH_MAX_DIR_ENTRIES` | `2048` | Skip watching any single directory with more than this many entries (data/cache dirs that would open thousands of fds on macOS). |
| `AGENT_LSP_WATCH_MAX_ENTRIES` | `50000` | Global budget on total watched entries (≈ file descriptors on macOS). Once reached, the walk stops and remaining files rely on explicit refresh. Raise on very large source trees. |
| `AGENT_LSP_WATCH_MAX_FDS` | `60000` | Runtime guard: the watcher samples the process's open file descriptors every 30s and tears itself down if the count exceeds this, catching descriptors that fsnotify opens for files *created while running* (which the startup budget above cannot see). Well below the macOS per-process limit (245,760). Raise on very large source trees. |

## Daemon Mode

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_LSP_DAEMON_DIR` | `~/.cache/agent-lsp` | Directory for daemon PID files, socket info, and spawn logs. |

## Debug

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_LSP_LOG_LEVEL` | `warning` | Log level: `debug`, `info`, `warning`, `error`. Also configurable at runtime via the `set_log_level` tool. |
| `AGENT_LSP_AUDIT_LOG` | (none) | Path to write JSON audit log of all tool calls. Useful for debugging and performance analysis. |
