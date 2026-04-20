# IDE Integration Plan

## Current State

agent-lsp works with any IDE that has an MCP client via stdio or HTTP+SSE transport. No plugin required.

**Known working:**
- VS Code (Continue, Cline, Claude Code extension)
- JetBrains 2025.2+ (bundled MCP server plugin)
- Cursor (native MCP)
- Windsurf (native MCP)
- Neovim (mcp.nvim)

## Immediate Action: Verify JetBrains 2025.2 Built-in MCP

JetBrains 2025.2+ ships a bundled MCP plugin. Check if agent-lsp works with just a config entry before building anything custom.

Reference:
- [JetBrains MCP Server docs](https://www.jetbrains.com/help/idea/mcp-server.html)
- [JetBrains/mcp-server-plugin source](https://github.com/JetBrains/mcp-server-plugin)
- [MCP Language Service Tools plugin](https://plugins.jetbrains.com/plugin/27888-mcp-language-service-tools)

If it works out of the box, the deliverable is documentation, not a plugin.

## Passive Mode (`--connect`)

Connect to an existing language server TCP socket instead of spawning a new process. Eliminates duplicate indexing when running alongside an IDE.

```
agent-lsp --connect go:localhost:9999 typescript:localhost:9998
```

Some language servers support multi-client TCP connections:
- gopls: `gopls -listen=:9999`
- rust-analyzer: supports stdio multiplexing
- pyright: no native TCP listener (would need a proxy)

This is IDE-agnostic and the highest-leverage feature for IDE integration.

## JetBrains Plugin (if needed beyond built-in MCP)

### Stack

- Kotlin + Gradle (IntelliJ Platform Gradle Plugin 2.x)
- Target: IntelliJ 2023.1+ (broad compatibility), 2025.2+ (built-in MCP)
- Single dependency: `com.intellij.modules.platform` (works across all JetBrains IDEs)
- Covers: GoLand, IntelliJ IDEA, PyCharm, WebStorm, CLion, Rider, PhpStorm, RubyMine

### Key APIs

| Purpose | API | Notes |
|---------|-----|-------|
| Process lifecycle | `OSProcessHandler` + `GeneralCommandLine` | Start/stop agent-lsp binary |
| Process events | `ExecutionManager#EXECUTION_TOPIC` | Monitor start/stop/exit |
| Actions (command palette) | `AnAction` in `plugin.xml` | Skills as IDE actions |
| Tool windows (results) | `ToolWindowFactory` + `ContentManager` | Lazy-loaded, tabbed panels |
| Gutter annotations | `LineMarkerProvider` + `NavigationGutterIconBuilder` | Blast-radius indicators, caller counts |
| Console output | `ColoredProcessHandler` | ANSI color support for logs |

### Gutter Annotation Notes

- Implement `RelatedItemLineMarkerProvider`
- Return leaf PSI elements only (e.g., `PsiIdentifier`), not parent nodes
- Two-pass processing: visible elements first, then rest of file
- Register via `com.intellij.codeInsight.lineMarkerProvider`

### Plugin Scope

**Phase 1: Launcher (minimal)**
- Auto-start agent-lsp on project open
- Auto-configure MCP connection for JetBrains AI / Continue
- Settings panel for language server configuration
- Status bar indicator (connected/disconnected)

**Phase 2: Native UI**
- Skills as command palette actions (Find Action)
- Speculative execution results as inline diff preview
- Blast-radius tool window (impact analysis results)
- Gutter icons: caller count, reference count on exported symbols

**Phase 3: Deep Integration**
- Passive mode: connect to IDE's running language servers
- Code lens: "3 callers | blast radius: low" annotations
- Inline diagnostic delta preview during refactoring

### AI Provider Configuration

The plugin does NOT connect to AI providers directly. agent-lsp is a tool server. The AI provider connection is handled by whichever MCP client the user has:

- JetBrains AI Assistant (bundled)
- Continue.dev (third-party plugin)
- Any future MCP client

The plugin's install experience:
1. Install plugin from JetBrains Marketplace
2. Plugin detects which MCP client is present
3. Plugin auto-configures agent-lsp as a tool server
4. User's existing AI assistant gains 50 LSP tools

Zero AI provider configuration required.

## VS Code Extension

Same phased approach as JetBrains. TypeScript-based.

### Key Differences from JetBrains

| Aspect | JetBrains | VS Code |
|--------|-----------|---------|
| Language | Kotlin | TypeScript |
| Build | Gradle | npm/webpack |
| Actions | `AnAction` | `commands` in `package.json` |
| Tool windows | `ToolWindowFactory` | `WebviewPanel` |
| Gutter | `LineMarkerProvider` | `CodeLensProvider` |
| MCP clients | JetBrains AI, Continue | Continue, Cline, Copilot |
| User base | Enterprise, Go/Java heavy | Broader, more AI tooling options |

## Neovim Plugin

Lua-based. Simplest of the three because Neovim exposes LSP clients directly via `vim.lsp.buf_get_clients()`. Passive mode could proxy through existing LSP connections without TCP sockets.

## Build Priority

1. Verify JetBrains 2025.2 works with zero plugin (documentation only)
2. Passive mode (`--connect`) in agent-lsp core (IDE-agnostic)
3. JetBrains plugin Phase 1 (launcher)
4. VS Code extension Phase 1 (launcher)
5. Phase 2 features based on adoption
