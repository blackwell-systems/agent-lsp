<p align="center">
  <img src="assets/social-preview.png" alt="agent-lsp" width="600">
</p>

<p align="center">
  <a href="#tools"><img src="https://img.shields.io/badge/CI--verified_tools-65%2F65-brightgreen.svg" alt="CI Coverage"></a>
  <a href="#multi-language-support"><img src="https://img.shields.io/badge/languages-30_CI--verified-brightgreen.svg" alt="Languages"></a>
  <a href="https://github.com/blackwell-systems/mcp-assert"><img src="https://raw.githubusercontent.com/blackwell-systems/mcp-assert/main/assets/badge-passing.svg?v=3" alt="mcp-assert: passing" height="20"></a>
  <a href="https://agentskills.io"><img src="assets/badge-agentskills.svg" alt="Agent Skills"></a>
  <a href="https://github.com/blackwell-systems/agent-lsp"><img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/assets/downloads-badge.json" alt="downloads"></a>
  <br>
  <a href="https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/"><img src="https://img.shields.io/badge/LSP-3.17-blue.svg" alt="LSP 3.17"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/punkpeye/awesome-mcp-servers"><img src="https://img.shields.io/badge/Awesome-MCP%20Servers-fc60a8" alt="Awesome MCP Servers"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

**Code intelligence infrastructure for AI agents.** 65 tools, 30 CI-verified languages, 24 agent workflows. Single Go binary.

## What is it?

agent-lsp is an **MCP server** that orchestrates existing LSP servers (gopls, rust-analyzer, jdtls, etc.) into agent-native workflows.

**Not an LSP server** — it's an orchestration layer that manages language servers and exposes batch operations, speculative editing, and multi-step workflows via MCP tools.

**Architecture:**
- **Language servers** (gopls, rust-analyzer, etc.) → provide code intelligence
- **agent-lsp** (MCP server) → orchestrates workflows, maintains warm runtime
- **AI agents** → consume via MCP protocol

## Why agent-lsp?

**Persistent warm runtime**  
Language servers stay indexed across agent sessions. First session: indexes workspace (~10s for typical projects). Subsequent sessions: instant. No cold-start penalty on each request.

**Batch operations**  
`blast_radius` → one call returns all exports + all callers (test vs non-test partitioned). Without orchestration: 20+ sequential LSP calls.

**Speculative editing**  
`simulate_edit` → preview changes in memory, check diagnostic delta, apply or discard. Test edits before touching disk.

**Workflow orchestration**  
24 skills that chain LSP operations into complete pipelines:
- `/lsp-refactor` → impact analysis → preview → apply → verify build → run tests
- `/lsp-safe-edit` → preview → diagnostic diff → apply if safe
- `/lsp-verify` → LSP diagnostics → build → test suite

**Multi-language, single session**  
One agent-lsp process routes `.go` to gopls, `.ts` to tsserver, `.py` to pyright. No reconfiguration between projects. Session persists across files and repositories.

> [!TIP]
> **Token-optimized output:** Tool responses encoded in [GCF](https://github.com/blackwell-systems/gcf) instead of JSON. 79% fewer input tokens, 63% fewer output tokens, [90.7% LLM comprehension accuracy](https://gcformat.com/guide/benchmarks.html) where JSON averages 53.6%. Tested across 10 models and 3 providers.

**How the pieces fit together:** [LSP](https://microsoft.github.io/language-server-protocol/) (Language Server Protocol) is how editors get code intelligence: completions, diagnostics, go-to-definition. [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) is the standard way AI tools like Claude Code discover and call external tools. agent-lsp bridges the two: language server intelligence, accessible to AI agents.

## Use it when

- Building agentic code generation systems
- Automating refactors across large codebases
- CI tooling that needs programmatic code intelligence
- Any workflow where sequential LSP calls are too slow or complex

### What agents say

We asked AI agents to evaluate agent-lsp across 10 coding tasks (find callers, rename safely, preview edits, detect dead code) and write an honest assessment. Four different models, four independent evaluations, same conclusion:

> **Claude (Opus 4.6):** "I would recommend agent-lsp for any workflow involving refactoring, impact analysis, or safe editing. The standout tools are `blast_radius` (blast radius in one call, with test/non-test partitioning that would take 5-10 grep commands to replicate), `go_to_implementation` (type-checked interface satisfaction that grep simply cannot do), and the simulation session workflow (speculative type-checking without touching disk, which has no grep/read equivalent at all)."

> **Cursor (auto):** "I would recommend agent-lsp for heavy refactors and code navigation because the rename, references, implementations, call hierarchy, and simulation tools remove a lot of brittle grep/manual-edit work and make changes safer."

> **GPT-5.5 (via Codex):** "I would recommend agent-lsp for symbol-aware work: references, implementations, rename previews, diagnostics, and large-file structure are materially faster and less error-prone than grep/read loops."

> **Gemini 2.5 Pro (via Gemini CLI):** "I would highly recommend agent-lsp because it provides a level of semantic awareness that standard text-searching tools simply cannot match. The ability to perform high-confidence renames, find interface implementations, and preview the diagnostic impact of edits without writing to disk significantly reduces the risk of introducing regressions."

### Tested, not assumed

Every other MCP-LSP implementation lists supported languages in a config file. None of them run the actual language server in CI to verify it works.

agent-lsp CI runs **30 real language servers** against real fixture codebases on every push: Go, Python, TypeScript, Rust, Java, C, C++, C#, Ruby, PHP, Kotlin, Swift, Scala, Zig, Lua, Elixir, Gleam, Clojure, Dart, Terraform, Nix, Prisma, SQL, MongoDB, and more. When we say "works with gopls," that's a verified, automated claim, not a hope.

### Speculative execution

Simulate changes in memory before writing to disk. No other MCP-LSP implementation has this.

`preview_edit` previews the diagnostic impact of any edit. You see exactly what breaks before the file is touched. `simulate_chain` evaluates a sequence of dependent edits (rename a function, update all callers, change the return type) and reports which step first introduces an error.

8 speculative execution tools. See [docs/guide/speculative-execution.md](./docs/guide/speculative-execution.md) for the full workflow.

### Token savings

Structured LSP responses use **5-34x fewer tokens** than grep/read on the same tasks. On HashiCorp Consul (319K lines), a blast-radius analysis uses 17.7MB via grep vs 841KB via LSP, reducing 5,534 tool calls to 119. Savings scale with codebase size. See [docs/guide/token-savings.md](./docs/guide/token-savings.md) for the full experiment across five codebases.

### Token-optimized output (GCF)

agent-lsp supports [GCF (Graph Compact Format)](https://github.com/blackwell-systems/gcf) as an optional output format. GCF replaces JSON field-name repetition with positional encoding:

| Tool | JSON | GCF | Savings |
|------|------|-----|---------|
| `list_symbols` (10) | ~334 tokens | ~165 tokens | **50.6%** |
| `find_references` (50) | ~858 tokens | ~437 tokens | **49.1%** |
| `get_diagnostics` (5) | ~213 tokens | ~133 tokens | **37.6%** |
| `blast_radius` (5) | ~526 tokens | ~365 tokens | **30.6%** |

GCF is enabled by default. To revert to JSON:

```bash
export AGENT_LSP_OUTPUT_FORMAT=json
```

Savings grow with record count (30-51% measured). Benchmark: `go run scripts/gcf-benchmark.go`. See [docs/guide/gcf-integration.md](./docs/guide/gcf-integration.md) for architecture details.

### Why orchestration matters

AI agents make incorrect code changes because they can't see the full picture: who calls this function, what breaks if I rename it, does the build still pass. Language servers have the answers, but raw LSP tools require 20+ sequential calls and complex orchestration logic.

agent-lsp solves this by encoding correct multi-step operations into single calls and skills. `blast_radius` does what would take an agent 20+ calls in one. `/lsp-refactor` chains impact → preview → apply → verify → test without per-prompt orchestration.

### Persistent daemon mode

Python and TypeScript projects need minutes of background indexing before `find_references` works. agent-lsp automatically spawns a persistent daemon broker that survives between sessions, so the workspace stays indexed. First session: daemon starts and indexes (~10s for FastAPI). Subsequent sessions: instant connection to the warm daemon. Auto-exits after 30 minutes of inactivity. Go, Rust, and other fast-indexing languages bypass this entirely (zero overhead).

### Phase enforcement

Skills tell agents the correct order of operations. Phase enforcement makes the runtime *block* violations instead of trusting the agent to follow instructions.

When an agent activates a skill, every tool call is checked against the current phase's permissions. Calling `apply_edit` during blast-radius analysis doesn't silently proceed; it returns an error with specific recovery guidance ("complete the blast_radius phase first, allowed tools: [blast_radius, find_references]"). Phases advance automatically as the agent calls tools from later phases.

No other MCP tool provider enforces workflow ordering at runtime. See [docs/guide/phase-enforcement.md](./docs/guide/phase-enforcement.md).

### Concurrency analysis

The inspector includes 4 concurrency checks that work across 25 languages in 4 concurrency families (goroutine, thread, async, actor):

- **Unrecovered concurrent entry**: goroutines/threads/tasks without recovery
- **Unchecked shared state**: bare type assertions on sync.Map, ConcurrentHashMap
- **Channel never closed**: channels/queues created but never closed (goroutine leaks)
- **Shared field without sync**: fields accessed from concurrent contexts without synchronization

`blast_radius` annotates symbols with `sync_guarded: true` when the parent type has a mutex. `find_callers` with `cross_concurrent: true` traces call chains through goroutine/thread boundaries. The `/lsp-concurrency-audit` skill produces a field-level safety report for any type.

### Auto-diagnostics

Symbol edit tools (`replace_symbol_body`, `insert_after_symbol`, `insert_before_symbol`, `safe_delete_symbol`) automatically return `errors_after` and `warnings_after` counts. Agents know immediately whether an edit broke something without a separate `get_diagnostics` call.

`safe_apply_edit` combines preview + apply in one call: previews speculatively, applies to disk only if `net_delta == 0` (no new errors). One tool call instead of three.

### Works with

| AI Tool | Transport | Config |
|---------|-----------|--------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | stdio | `mcpServers` in `.mcp.json` |
| [Continue](https://continue.dev) | stdio | `mcpServers` in `config.json` |
| [Cline](https://github.com/cline/cline) | stdio | `mcpServers` in settings |
| [Cursor](https://cursor.com) | stdio | `mcpServers` in settings |
| Any MCP client | HTTP+SSE | `--http --port 8080` with Bearer token auth |

## Skills

Raw tools get ignored. Skills get used. Each skill encodes the correct tool sequence so workflows actually happen without per-prompt orchestration instructions. Skills are available as [AgentSkills](https://github.com/anthropics/agent-skills) slash commands and as MCP prompts via `prompts/list` / `prompts/get` for any MCP client.

See [docs/guide/skills.md](./docs/guide/skills.md) for full descriptions and usage guidance.

**Before you change anything**

| Skill | Purpose |
|-------|---------|
| `/lsp-impact` | Blast-radius analysis before touching a symbol or file |
| `/lsp-implement` | Find all concrete implementations of an interface |
| `/lsp-dead-code` | Detect zero-reference exports before cleanup |

**Editing safely**

| Skill | Purpose |
|-------|---------|
| `/lsp-safe-edit` | Speculative preview before disk write; before/after diagnostic diff; surfaces code actions on errors |
| `/lsp-simulate` | Test changes in-memory without touching the file |
| `/lsp-edit-symbol` | Edit a named symbol without knowing its file or position |
| `/lsp-edit-export` | Safe editing of exported symbols, finds all callers first |
| `/lsp-rename` | `prepare_rename` safety gate, preview all sites, confirm, apply atomically |

**Getting started**

| Skill | Purpose |
|-------|---------|
| `/lsp-onboard` | First-session project onboarding: detect languages, map packages, find entry points and hotspots, check diagnostics |

**Understanding unfamiliar code**

| Skill | Purpose |
|-------|---------|
| `/lsp-explore` | "Tell me about this symbol": hover + implementations + call hierarchy + references in one pass |
| `/lsp-understand` | Deep-dive Code Map for a symbol or file: type info, call hierarchy, references, source |
| `/lsp-docs` | Three-tier documentation: hover → offline toolchain → source |
| `/lsp-cross-repo` | Find all usages of a library symbol across consumer repos |
| `/lsp-local-symbols` | File-scoped symbol list, usage search, and type info |

**After editing**

| Skill | Purpose |
|-------|---------|
| `/lsp-verify` | Diagnostics + build + tests after every edit |
| `/lsp-fix-all` | Apply quick-fix code actions for all diagnostics in a file |
| `/lsp-test-correlation` | Find and run only tests that cover an edited file |
| `/lsp-format-code` | Format a file or selection via the language server formatter |

**Generating code**

| Skill | Purpose |
|-------|---------|
| `/lsp-generate` | Trigger server-side code generation (interface stubs, test skeletons, mocks) |
| `/lsp-extract-function` | Extract a code block into a named function via code actions |

**Full workflow**

| Skill | Purpose |
|-------|---------|
| `/lsp-refactor` | End-to-end refactor: blast-radius → preview → apply → verify → test |
| `/lsp-inspect` | Full code quality audit (12 checks): dead symbols, test coverage, error handling, doc drift, concurrency safety |
| `/lsp-concurrency-audit` | Field-level concurrency safety audit for a type: traces concurrent access, flags unsynced fields |

## Docker

**Stdio mode** (MCP client spawns the container directly):

```bash
# Go
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls

# TypeScript
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio

# Python
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio
```

**HTTP mode** (persistent service, remote clients connect over HTTP+SSE):

```bash
docker run --rm \
  -p 8080:8080 \
  -v /your/project:/workspace \
  -e AGENT_LSP_TOKEN=your-secret-token \
  ghcr.io/blackwell-systems/agent-lsp:go \
  --http --port 8080 go:gopls
```

Images run as a non-root user (uid 65532) by default. Set `AGENT_LSP_TOKEN` via environment variable, never `--token` on the command line. Images are also mirrored to Docker Hub (`blackwellsystems/agent-lsp`). See [DOCKER.md](./DOCKER.md) for the full tag list, HTTP mode setup, and security hardening options.

## Setup

### Step 1: Install agent-lsp

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```

<details>
<summary>Alternative install methods</summary>

**macOS / Linux**

```bash
brew install blackwell-systems/tap/agent-lsp
```

**Windows**

```powershell
# PowerShell (no admin required)
iwr -useb https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.ps1 | iex

# Scoop
scoop bucket add blackwell-systems https://github.com/blackwell-systems/agent-lsp
scoop install blackwell-systems/agent-lsp

# Winget
winget install BlackwellSystems.agent-lsp
```

**All platforms**

```bash
# pip
pip install agent-lsp

# npm
npm install -g @blackwell-systems/agent-lsp

# Go install
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
```

</details>

### Step 2: Install language servers

Install the servers for your stack. Common ones:

| Language | Server | Install |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |

Full list of 30 supported languages in [docs/reference/language-support.md](./docs/reference/language-support.md).

### Step 3: Verify setup

```bash
agent-lsp doctor
```

Probes each configured language server and reports capabilities. Fix any failures before proceeding. See [language support](./docs/reference/language-support.md) for install commands and server-specific notes.

### Step 4: Configure your AI tool

```bash
agent-lsp init
```

Detects language servers on your PATH, asks which AI tool you use, writes the correct MCP config, and installs skill awareness rules for your AI provider (CLAUDE.md for Claude Code, `.cursor/rules/` for Cursor, `.clinerules` for Cline, `.windsurfrules` for Windsurf, `GEMINI.md` for Gemini CLI). For CI or scripted use: `agent-lsp init --non-interactive`.

The generated config looks like:

```json
{
  "mcpServers": {
    "lsp": {
      "type": "stdio",
      "command": "agent-lsp",
      "args": [
        "go:gopls",
        "typescript:typescript-language-server,--stdio",
        "python:pyright-langserver,--stdio"
      ]
    }
  }
}
```

Each arg is `language:server-binary` (comma-separate server args).

### Step 5: Install skills

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git /tmp/agent-lsp-skills
cd /tmp/agent-lsp-skills/skills && ./install.sh --copy
```

Skills are prompt files copied into your AI tool's configuration. `--copy` means the clone can be safely deleted afterward.

Skills are also available as **MCP prompts**: any MCP client can discover them via `prompts/list` and retrieve full workflow instructions via `prompts/get`, with no manual installation required. The `install.sh` path is for AgentSkills-compatible clients (Claude Code slash commands).

### Step 6: Allow tool permissions (Claude Code)

For Claude Code, add `mcp__lsp__*` to your permissions allow list so all 65 tools are available without per-tool approval prompts:

```json
// ~/.claude/settings.json
{
  "permissions": {
    "allow": ["mcp__lsp__*"]
  }
}
```

Without this, Claude Code will prompt for permission on each tool call. Other MCP clients handle permissions differently; check your client's documentation.

Skills are multi-tool workflows that encode reliable procedures: blast-radius check before edit, speculative preview before write, test run after change. See [docs/guide/skills.md](./docs/guide/skills.md) for the full list.

### Step 7: Start working

Your AI agent calls tools automatically. The first call initializes the workspace:

```
start_lsp(root_dir="/your/project")
```

This is what the agent does, not something you type. Then use any of the 65 tools. The session stays warm; no restart needed when switching files.

## What's unique about agent-lsp

| Capability | Details |
|------------|---------|
| Tools | **65** |
| Languages (CI-verified) | **30**, end-to-end integration tests on every push |
| Agent workflows (skills) | **24**, named multi-step procedures, discoverable via MCP `prompts/list` |
| Speculative execution | **8 tools**, simulate changes before writing to disk |
| Phase enforcement | **4 skills**, runtime blocks out-of-order tool calls with recovery guidance |
| Connection model | **persistent**, warm index across files and projects |
| Call hierarchy | **✓**, single tool, direction param |
| Type hierarchy | **✓**, CI-verified |
| Cross-repo references | **✓**, multi-root workspace |
| Auto-watch | **✓**, always-on, debounced file watching |
| HTTP+SSE transport | **✓**, bearer token auth, non-root Docker |
| Distribution | **single Go binary**, 10 install channels |

## Use Cases

- **Multi-project sessions**: point your AI at `~/code/`, work across any project without reconfiguring
- **Polyglot development**: Go backend + TypeScript frontend + Python scripts in one session
- **Large monorepos**: one server handles all languages, routes by file extension
- **Code migration**: refactor across repos with full cross-repo reference tracking
- **CI pipelines**: validate against real language server behavior
- **Niche language stacks**: Gleam, Elixir, Prisma, Zig, Clojure, Nix, Dart, Scala, MongoDB, all CI-verified

## Multi-Language Support

30 languages, CI-verified end-to-end against real language servers on every CI run. No other MCP-LSP implementation tests a single language in CI.

Go, Python, TypeScript, Rust, Java, C, C++, C#, Ruby, PHP, Kotlin, Swift, Scala, Zig, Lua, Elixir, Gleam, Clojure, Dart, Terraform, Nix, Prisma, SQL, MongoDB, JavaScript, YAML, JSON, Dockerfile, CSS, HTML.

See [docs/reference/language-support.md](./docs/reference/language-support.md) for the full coverage matrix.

## Tools

65 tools covering navigation, analysis, refactoring, symbol editing, composite exploration, safe editing, speculative execution, and session lifecycle. All CI-verified.

See [docs/reference/tools.md](./docs/reference/tools.md) for the full reference with parameters and examples.

## Further reading

### Documentation

- [Tools reference](./docs/reference/tools.md): full tool reference with parameters and examples
- [Skills reference](./docs/guide/skills.md): skill reference, workflows, use cases, and composition
- [Language support](./docs/reference/language-support.md): language coverage matrix
- [Architecture](./docs/architecture/architecture.md): system design and internals
- [Speculative execution](./docs/guide/speculative-execution.md): simulate-before-apply workflows
- [LSP conformance](./docs/reference/lsp-conformance.md): LSP 3.17 spec coverage
- [Docker](./DOCKER.md): Docker tags, compose, and volume caching

### Contributing

- [CI notes](./docs/architecture/ci-notes.md): CI quirks and test harness details
- [Distribution](./docs/architecture/distribution.md): install channels and release pipeline

## Development

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git
cd agent-lsp && go build ./...
go test ./...                   # unit tests
go test ./... -tags integration # integration tests (requires language servers)
```

## Library Usage

The `pkg/lsp`, `pkg/session`, and `pkg/types` packages expose a stable Go API for using agent-lsp's LSP client directly without running the MCP server.

```go
import "github.com/blackwell-systems/agent-lsp/pkg/lsp"

client := lsp.NewLSPClient("gopls", []string{})
client.Initialize(ctx, "/path/to/workspace")
defer client.Shutdown(ctx)

locs, err := client.GetDefinition(ctx, fileURI, lsp.Position{Line: 10, Character: 4})
```

See [docs/architecture/architecture.md](./docs/architecture/architecture.md) for the full package API.

## License

MIT
