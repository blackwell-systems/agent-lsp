---
title: Roadmap
---

# Roadmap

## Distribution

| Feature | Status | Description |
|---------|--------|-------------|
| **Nix flake** | Planned | `nix run github:blackwell-systems/agent-lsp` |

## Extensions

Extensions add language-specific tools beyond what LSP exposes. The core 60 tools cover 26 of the most agent-relevant LSP 3.17 methods (navigation, analysis, refactoring, diagnostics, formatting) plus 31 tools that go beyond the LSP spec (speculative execution, build/test, change impact analysis, cross-repo references, cache management, git-based change detection, symbol-level editing, audit). Three low-value LSP methods are intentionally omitted: `selectionRange`, `foldingRange`, and `codeLens`. Extensions run arbitrary toolchain logic for a specific language.

### Go extension (Wave 1: test + module intelligence)

| Tool | Description |
|------|-------------|
| `go.test_run` | Run a specific test by name, return full output + pass/fail |
| `go.test_coverage` | Coverage % and uncovered lines for a file or package |
| `go.benchmark_run` | Run a benchmark, return ns/op and allocs/op |
| `go.test_race` | Run with `-race`, return any data races found |
| `go.mod_graph` | Full dependency tree as structured data |
| `go.mod_why` | Why is this package in go.mod? (`go mod why`) |
| `go.mod_outdated` | List deps with available upgrades |
| `go.vulncheck` | `govulncheck` scan, CVEs with affected symbols |

### Go extension (Wave 2: build + quality)

| Tool | Description |
|------|-------------|
| `go.escape_analysis` | `gcflags="-m"` output for a function: what allocates and why |
| `go.cross_compile` | Try cross-compiling for a target OS/arch, return errors |
| `go.lint` | `staticcheck` or `golangci-lint` output for a file |
| `go.deadcode` | Find exported symbols with no callers (`go tool deadcode`) |
| `go.vet_all` | `go vet ./...` with structured output |

### Go extension (Wave 3: generation + docs)

| Tool | Description |
|------|-------------|
| `go.generate` | Run `go generate` on a file, return output |
| `go.generate_status` | Which `//go:generate` directives are stale |
| `go.doc` | `go doc` output for any symbol, richer than hover |
| `go.examples` | Find `Example*` test functions for a symbol |

### TypeScript extension

| Tool | Description |
|------|-------------|
| `typescript.tsconfig_diagnostics` | Errors in `tsconfig.json` beyond what the language server reports |
| `typescript.type_coverage` | Type coverage % for a file (`any` usage, implicit types) |

### Rust extension

| Tool | Description |
|------|-------------|
| `rust.cargo_check` | `cargo check` with structured error output |
| `rust.dep_tree` | Crate dependency tree (`cargo tree`) |
| `rust.clippy` | `cargo clippy` lint output for a file |
| `rust.audit` | `cargo audit` CVE scan on `Cargo.lock` |

### Python extension

Python has the largest gap between what `pyright-langserver` gives an agent and what the toolchain provides directly.

| Tool | Description |
|------|-------------|
| `python.test_run` | Run a specific `pytest` test by name, return output + pass/fail |
| `python.test_coverage` | `coverage.py` branch coverage for a file or module |
| `python.lint` | `ruff` lint output with structured violations |
| `python.type_check` | `mypy` type errors for a file (stricter than pyright diagnostics) |
| `python.audit` | `pip-audit` CVE scan on installed packages |
| `python.security` | `bandit` security scan for a file |
| `python.deadcode` | `vulture` dead code detection |
| `python.imports` | `isort` check for unsorted or missing imports |

### C / C++ extension

The gap between what clangd provides and what the broader toolchain offers is larger than any other language. Sanitizers and profiling are completely outside LSP scope.

| Tool | Description |
|------|-------------|
| `cpp.tidy` | `clang-tidy` diagnostics for a file (beyond clangd's built-in checks) |
| `cpp.static_analysis` | `cppcheck` output with structured findings |
| `cpp.asan_run` | Build and run with AddressSanitizer, return memory error output |
| `cpp.ubsan_run` | Build and run with UndefinedBehaviorSanitizer |
| `cpp.valgrind` | `valgrind --memcheck` output for a test binary |
| `cpp.symbols` | `nm` / `objdump` symbol table for a compiled object |

### Java extension

| Tool | Description |
|------|-------------|
| `java.test_run` | Run a specific JUnit test, return output |
| `java.coverage` | JaCoCo coverage report for a class |
| `java.build` | Maven/Gradle build with structured error output |
| `java.deps` | `jdeps` dependency analysis: what packages does this class use? |
| `java.checkstyle` | Checkstyle violations for a file |
| `java.spotbugs` | SpotBugs static analysis findings |

### Elixir extension

| Tool | Description |
|------|-------------|
| `elixir.test_run` | Run a specific ExUnit test, return output |
| `elixir.dialyzer` | Dialyzer type analysis, unique to Elixir; finds type errors without annotations |
| `elixir.credo` | Credo static analysis findings |
| `elixir.audit` | `mix deps.audit` CVE scan |

### Ruby extension

| Tool | Description |
|------|-------------|
| `ruby.test_run` | Run a specific RSpec or Minitest test, return output |
| `ruby.lint` | RuboCop violations for a file |
| `ruby.security` | Brakeman security scan (Rails) |
| `ruby.audit` | `bundle-audit` CVE scan on `Gemfile.lock` |


## Product

| Feature | Status | Description |
|---------|--------|-------------|
| **`agent-lsp update`** | **Shipped** | Self-update to the latest release; fetches from GitHub Releases and replaces the binary in-place. Flags: `--check` (compare without downloading), `--force` (update even if current). |
| **`agent-lsp uninstall`** | **Shipped** | Clean removal of MCP configs, skill installations, CLAUDE.md managed sections, and cache directories. Supports `--dry-run`. |
| **Config file format** | Planned | `~/.agent-lsp.json` or `agent-lsp.json` project file for complex setups with per-server options |
| **Continue.dev config support** | Planned | `agent-lsp init` currently skips Continue.dev; it uses a different config format than `mcpServers` |
| **Skills as MCP prompts** | **Shipped** | Expose all 24 skills via `prompts/list` and `prompts/get` so any MCP client (Cursor, Windsurf, etc.) can discover and invoke them, not just Claude Code. `prompts/list` returns short descriptions (minimal context cost); full workflow instructions load on demand via `prompts/get`. Skills continue to work as Claude Code slash commands in parallel. |
| **Proactive server notifications** | **Shipped** | Server-initiated MCP notifications across four channels: (1) diagnostic changes (2s debounce), (2) workspace ready (one-shot on indexing complete), (3) process health (crash/recovery), (4) stale references (3s debounce on file changes). Hub coordinator in `internal/notify/`, MCP wiring in `cmd/agent-lsp/notifications.go`. All channels wired automatically on `start_lsp`. |

### Context and efficiency

| Feature | Status | Description |
|---------|--------|-------------|
| **`get_editing_context` (composite tool)** | **Shipped** | Single "give me everything I need before editing this file" call. Returns: all symbols with signatures, callers (test/non-test partitioned), callees, imports. Supports `if_none_match` for conditional responses. Shipped as tool #61 in v0.10.0. |
| **Token savings in responses** | **Shipped** | `list_symbols`, `get_symbol_source`, and `get_editing_context` include `_meta.token_savings` showing tokens returned vs full file size. Shipped in v0.10.0. |
| **ETag/conditional responses** | **Shipped** | File-scoped tools accept `if_none_match` parameter. When content hash matches, returns `not_modified` instead of recomputing. Shipped in v0.10.0. |
| **Untested symbol filter** | **Shipped** | `filter: "untested"` parameter on `get_change_impact`. Returns only exported symbols where `non_test_callers > 0 AND test_callers == 0` (active in production code but no test coverage). Complements `/lsp-dead-code` (zero references) with a coverage gap view (has callers, no tests). |

### Symbol-level editing tools

Symbol-level tools accept a symbol name and perform the operation structurally, eliminating error-prone coordinate resolution. All four share a `ResolveSymbolByNamePath` resolver that locates symbols by dot-notation path (e.g. `"Buffer.Reset"`) across the workspace.

| Feature | Status | Description |
|---------|--------|-------------|
| **`replace_symbol_body`** | **Shipped** | Replace the body of a named function, method, or type. Resolves the symbol via `list_symbols`, extracts its full range, applies the replacement. The agent says "replace function X with this new implementation" without knowing line numbers. |
| **`insert_after_symbol`** | **Shipped** | Insert code after a named symbol definition. Useful for adding a new method after an existing one, or a new function after a related helper. |
| **`insert_before_symbol`** | **Shipped** | Insert code before a named symbol definition. Useful for adding imports, comments, or related types before their first consumer. |
| **`safe_delete_symbol`** | **Shipped** | Delete a symbol only if it has zero references (verified via `find_references` before deletion). Prevents accidental removal of active code. |

These complement the existing `apply_edit` (which remains available for raw text edits) and `/lsp-edit-symbol` skill (which orchestrates a multi-step workflow). The difference: symbol-level tools are single atomic calls, not multi-step skills. Higher-level abstraction, lower error rate.

### Agent memory system

Structured knowledge persistence across sessions. Agents accumulate understanding of a codebase (architecture decisions, naming conventions, known pitfalls) that is lost when the session ends. A memory system would store this knowledge and make it available to future sessions.

| Feature | Status | Description |
|---------|--------|-------------|
| **Session memory** | Planned | Key-value store persisted to `~/.agent-lsp/memory/<workspace-hash>/`. Agents write observations during a session; future sessions read them on connect. |
| **Cross-session context** | Planned | MCP resource at `memory://` exposing stored knowledge. Agents read it at session start to recover prior understanding without re-exploring the codebase. |
| **Memory pruning** | Planned | Automatic expiration and relevance scoring. Stale entries (referencing deleted files, outdated patterns) are pruned on access. |

### Provider-agnostic skill awareness

AI agents using agent-lsp need to know about the 24 skills and when to use them. The current approach (SKILL.md files in `~/.claude/skills/`) only works for Claude Code. The solution is a four-layer reinforcement architecture where skill awareness is seeded at connect time and reinforced on every interaction, regardless of which AI provider or client is used.

| Layer | Mechanism | Status | Scope | Durability |
|-------|-----------|--------|-------|------------|
| **1. Connect-time instructions** | `ServerOptions.Instructions` in MCP `initialize` response | **Shipped** | Every MCP client automatically | Decays over long conversations |
| **2. Per-response hints** | Content[1] "Next step:" in every tool response | **Shipped** (v0.8.1) | Every MCP client | Renewed on every tool call |
| **3. On-demand workflows** | `prompts/get("lsp-refactor")` returns full skill workflow | **Shipped** (v0.7.0) | Any client that calls prompts/list | Loaded when needed |
| **4. Phase enforcement** | Error messages with recovery guidance when agent skips steps | **Shipped** (v0.5.0) | Every MCP client | Fires on violations |

Layer 1 (`Instructions`) is the missing piece. Implementation: set `ServerOptions.Instructions` in `cmd/agent-lsp/server.go` with a condensed string that seeds skill awareness and points to `prompts/get` for full workflows. The string should be short enough to survive context pressure (under 200 tokens). Combined with layers 2-4, the agent receives continuous reinforcement without any provider-specific configuration.

| Feature | Status | Description |
|---------|--------|-------------|
| **`Instructions` on initialize** | **Shipped** | `ServerOptions.Instructions` set with condensed skill overview: tool count, key workflows (blast radius before edit, simulate before apply, verify after change), pointer to `prompts/get` for full details. Under 200 tokens. Every MCP client receives it automatically on connect. |
| **`agent-lsp init` rules files** | **Shipped** | `init` writes a skill awareness rules file alongside the MCP config. Claude Code gets a managed CLAUDE.md section (sentinel comments). Cursor gets `.cursor/rules/agent-lsp.mdc`. Cline gets `.clinerules`. Windsurf gets `~/.windsurfrules`. Gemini CLI gets `GEMINI.md`. All use managed sections for idempotent updates. Content generated from embedded SKILL.md files at runtime. |

**Per-platform rules file mapping:**

| Platform | Choice in `init` | Rules File | Format |
|----------|-----------------|------------|--------|
| Claude Code (project) | 1 | `CLAUDE.md` managed section (between sentinel comments) | Markdown |
| Claude Code (global) | 2 | `~/.claude/CLAUDE.md` managed section | Markdown |
| Claude Desktop | 3 | N/A (uses `Instructions` only) | N/A |
| Cursor | 4 | `.cursor/rules/agent-lsp.mdc` | Cursor rules MDC |
| Cline | 5 | `.clinerules` | Markdown |
| Windsurf | 6 | `.windsurfrules` | Markdown |
| Gemini CLI | 7 | `GEMINI.md` | Markdown |

Content is identical across platforms (skill table, tool usage guidance, LSP-first preferences). Generated from SKILL.md files at build time via `go:embed`, keeping rules in sync with shipped skills automatically. The `init` command already knows which platform was selected; it writes the rules file alongside the MCP config in the same step.

### Agent tool adoption enforcement

Agents default to built-in tools (Read, Grep, Edit) over MCP tools even when the MCP tool is faster and more accurate. The provider-agnostic skill awareness layers above seed knowledge of available tools; the items below actively enforce their use.

| Feature | Status | Description |
|---------|--------|-------------|
| **Disallowed reasoning patterns** | **Shipped** | Claude Code init rules include a "use this, not that" table (e.g., "find all usages: use `find_references`, not Grep"). Provider-agnostic Instructions use softer "prefer these tools" language. |
| **Task-to-tool mapping table** | **Shipped** | 10-entry task-to-tool mapping in the MCP Instructions string. Claude Code rules files include a full comparison table with "Not this" column. |
| **Recovery-oriented error messages** | **Shipped** | Symbol resolution errors suggest `list_symbols`. `safe_delete_symbol` with references suggests `find_references` to see callers. `CheckInitialized` suggests `start_lsp`. |
| **Graceful degradation on large results** | Planned | Return shortened summaries (symbol names and file paths only) instead of failing when output exceeds size limits. Lets the agent refine its query instead of hitting a wall. |
| **"No verification needed" assertions** | **Shipped** | `preview_edit` description states: "If net_delta is 0, the edit is safe to apply without further verification." Reduces unnecessary follow-up tool calls after clean previews. |
| **Claude Code suggestion hook** | Exploring | Optional hook that suggests (not blocks) agent-lsp tools when the agent uses grep/read for tasks that LSP handles better (e.g., finding references, getting type info). Non-blocking: logs a suggestion, does not deny the tool call. Must not interfere with non-code tasks (reading configs, markdown, logs). May not be worth the friction. |
| **Claude Code auto-approval** | **Resolved** | Documented in README Step 6: add `"mcp__lsp__*"` to `permissions.allow` in `~/.claude/settings.json`. Other MCP clients (Cursor, Cline, Windsurf, Gemini CLI) already auto-approve or have one-time allow flows. No hook needed. |
| **Per-client tool description overrides** | Planned | Tune tool descriptions based on which client is connected. In Claude Code, `list_symbols` description says "use this instead of Read for file structure." In Cursor, skip that guidance since Cursor has its own LSP. Requires detecting the client from the MCP initialize handshake. |
| **Cross-referencing in tool descriptions** | **Shipped** | Tools suggest related tools where applicable. `apply_edit` recommends `replace_symbol_body` for full function replacements and `preview_edit` before applying. `find_references` recommends `safe_delete_symbol` for zero-reference symbols and `get_change_impact` for blast-radius analysis. `suggest_fixes` points to `/lsp-fix-all` skill. `rename_symbol` recommends `find_references` before renaming exports. |
| **Onboarding nudge on first use** | Planned | On first `get_change_impact` or `find_references` call in a session, if `/lsp-onboard` has not been run (no `.agent-lsp/onboard-complete` marker), append a hint: "hint: run /lsp-onboard for project context (build system, entry points, test runner)." Non-blocking: the tool still returns results. The hint disappears after onboarding completes or after 3 calls (whichever comes first). Inspired by Serena's enforced onboarding flow but non-intrusive. |

### Inspector evolution (`/lsp-inspect`)

The inspector skill is agent-lsp's most powerful quality tool: it found a nil sender crash path and dead exports that tests missed. These improvements make it stronger for systematic codebase auditing.

**Skill-level improvements (SKILL.md changes):**

| Feature | Status | Description |
|---------|--------|-------------|
| **Severity calibration** | **Shipped** | Weight findings by blast radius using caller counts from `get_change_impact`. A silent failure in a function with 50 callers ranks higher than one with 2. Crash paths rank higher than style nits. Filter out findings that produce noise without actionable value. |
| **Fix suggestions** | **Shipped** | Each finding includes the specific fix: "remove lines 42-58" for dead code, "change `return err` to `fmt.Errorf('context: %w', err)`" for error wrapping. Agents can generate a PR directly from inspector output without additional analysis. |
| **Batch mode** | **Shipped** | Accept a directory with `--top N` flag, walk all packages, produce a ranked report. "Top 10 findings in this repo, sorted by severity and blast radius." |
| **Comparison mode** | **Shipped** | Run on a PR diff with `--diff` flag: "what did this change introduce?" Compare before/after inspection results to surface new dead code, new silent failures, or new coverage gaps introduced by the change. |

**Underlying tool improvements (Go code changes):**

| Feature | Status | Description |
|---------|--------|-------------|
| **Cross-file impact scoring** | **Shipped** | Weight each finding by its blast radius using `get_change_impact` data. A silent failure in a function with 50 callers ranks higher than one with 2. The inspector calls `get_change_impact` per finding and includes `caller_count` in the output. |
| **Confidence tiers** | **Shipped** | Replace "high/medium/low" with actionable labels: "verified" (LSP confirmed, act immediately), "suspected" (pattern match, investigate first), "advisory" (style suggestion, optional). Applied to the inspector output format and the check taxonomy in the SKILL.md. |
| **Unexported dead code detection** | **Shipped** | Extend `get_change_impact` with a new `scope` parameter (`scope: "all"`) to check unexported symbols in addition to exported ones. Uses `collectAllSymbols` to walk all document symbols and check references for each. |
| **Inspector result as MCP resource** | **Shipped** | Expose the last inspector run as an MCP resource at `inspect://last`. Results persisted to `.agent-lsp/last-inspection.json`. Agents can re-read findings without re-running the full analysis. Useful for iterative fix-verify cycles. |

### Concurrency analysis

Proven by finding unrecovered goroutines in mark3labs/mcp-go (#860). These checks detect real crash-path bugs that tests miss because they require specific timing or nil states to trigger. Language-agnostic: the check taxonomy is universal, with per-language heuristics selected by `language_id`.

**Language family mapping (covers 25 of 30 supported languages):**

| Family | Languages | Concurrent entry pattern | Sync primitive | Recovery |
|--------|-----------|------------------------|----------------|----------|
| Goroutine | Go | `go func()` | `sync.Mutex`, `sync.Map` | `recover()` |
| Thread | Java, Kotlin, C#, Scala, C, C++, Rust, Swift, Zig, Ruby, Groovy | `Thread`, `spawn`, `Task.Run`, `pthread_create` | `synchronized`, `Mutex`, `lock`, `pthread_mutex` | `catch`, `catch_unwind`, `UncaughtExceptionHandler` |
| Async | Python, TypeScript, JavaScript, Dart, Kotlin (coroutines) | `asyncio.create_task`, `Promise`, `new Worker()` | N/A (single-threaded event loop; `Worker` uses message passing) | `try/except`, `.catch()`, `error` event handler |
| Actor | Elixir, Erlang, Gleam | `spawn`, `Task.async` | Message passing (no shared state) | Supervisors (let-it-crash; skip checks) |
| None | Lua, Bash, SQL, HTML/CSS, Markdown | N/A (skip) | N/A | N/A |

**Inspector check type (`concurrency`):**

| Feature | Status | Description |
|---------|--------|-------------|
| **Unrecovered concurrent entry** | **Shipped** | Detect concurrent entry points without recovery across 4 language families. Go: `go func()` without `recover()`. Thread family: `new Thread()/spawn` without try-catch or `UncaughtExceptionHandler`. Async family: `new Worker()` without `error` event handler. Weight by library vs application code. Proven on mcp-go (#860). |
| **Unchecked type assertion on shared state** | **Shipped** | Detect bare type assertions on concurrent data structures. Go: `sync.Map` with `.(*Type)` without `, ok`. Java: `ConcurrentHashMap` with unchecked cast. Rust: N/A (type system prevents this). TypeScript: N/A (dynamic typing). |
| **Channel/queue never closed** | **Shipped** | Detect channels or queues created but never closed across 5 languages. Go: `make(chan T)` without `close()`. Python: `queue.Queue` without sentinel. TypeScript: `MessageChannel` without `close()`. Rust: `mpsc::channel` without drop. Java: `BlockingQueue` without poison pill. |
| **Shared field without sync** | **Shipped** | Detect fields accessed from multiple concurrent contexts without synchronization. Composes `get_change_impact` (sync_guarded) + `find_callers` (cross_concurrent) to identify symbols called from concurrent contexts on types without sync primitives. Language-agnostic: LSP provides the data, heuristics classify by write/read pattern. |

**Tool-level support:**

| Feature | Status | Description |
|---------|--------|-------------|
| **Sync-guarded metadata in get_change_impact** | **Shipped** | If a symbol is a method on a type that contains a synchronization primitive (Go: `sync.Mutex`/`sync.RWMutex`, Java: `ReentrantLock`, Rust: `Mutex<T>`, Python: `Lock`, C/C++: `pthread_mutex`/`std::mutex`), include `"sync_guarded": true` in the affected_symbols output. Uses document symbols already fetched during Phase 1; zero additional LSP queries. |
| **Cross-concurrent-boundary caller tracing** | **Shipped** | `find_callers` now accepts `cross_concurrent: true`. Annotates incoming callers that cross concurrent boundaries (goroutines, threads, async tasks). Returns `concurrent_callers` array with the detected pattern and source location. Scans a 5-line window above each call site for concurrent entry patterns across all language families. |

**Skill candidate:**

| Feature | Status | Description |
|---------|--------|-------------|
| **`/lsp-concurrency-audit`** | **Shipped** | 24th skill. Given a type, map all its fields, trace which are accessed from concurrent contexts via `find_callers(cross_concurrent=true)` + `get_change_impact(sync_guarded)`, and flag fields without synchronization. Produces a field-level safety report with SAFE/UNSAFE/WRITE-CONCURRENT/READ-ONLY classifications. Language-agnostic across 4 concurrency families. |

## Skills

24 skills shipped. See [skills.md](skills.md) for the full catalog.

### Creation skills

Current skills are oriented around modifying existing code. These skills target greenfield creation workflows where LSP can still add value through completions, diagnostics, and code actions.

| Skill | Description |
|-------|-------------|
| `/lsp-create` | Iterative file creation with diagnostic checks between steps. Create file, open in LSP, write incrementally, verify diagnostics after each addition, format on completion. `/lsp-safe-edit` for files that don't exist yet. |
| `/lsp-implement` (extend) | Given an interface or type definition, generate the full implementation using `get_completions` to discover required methods, verify it compiles via diagnostics, format. |
| `/lsp-discover-api` | Completion-driven API exploration. Open a file, place the cursor after a package qualifier, call `get_completions` to show available methods/fields. Use LSP knowledge instead of training data (which may be outdated). |
| `/lsp-bootstrap` | Project scaffolding with LSP verification. Create build files (go.mod, package.json, Cargo.toml), start LSP, confirm indexing works, verify initial diagnostics are clean before writing application code. |
| `/lsp-wire` | After creating a new package/module, verify it's importable from the intended consumer, check the public API surface via `list_symbols`, confirm no dangling imports or missing exports. |

### Inspection skill (shipped)

| Skill | Description |
|-------|-------------|
| `/lsp-inspect` | Full code quality audit for a file or package. Composes `get_change_impact` (batch dead symbol + test coverage), `find_references` (per-symbol verification), `get_diagnostics` (error detection), and LLM reasoning checks (silent failures, error wrapping, doc drift, coverage gaps). Produces a severity-tiered findings report. Replaces the external `agentskills-code-inspector` with a first-party skill that has direct access to the warm LSP session. Language-agnostic: works with any configured language server. |

Rationale: The inspector workflow was previously a separate repo that orchestrated agent-lsp's tools via MCP round-trips. Shipping it as a bundled skill eliminated: (1) separate installation, (2) MCP permission setup for background agents, (3) warmup gate complexity, (4) redundant `start_lsp` calls. The mechanical checks (`dead_symbol`, `test_coverage`) use `get_change_impact` directly. The reasoning checks (`silent_failure`, `error_wrapping`, `doc_drift`) are LLM-driven heuristics defined in the skill markdown. `/lsp-inspect` is now the 21st shipped skill.

### Skill composition

Skills calling other skills. `/lsp-refactor` is already composed from `/lsp-impact` + `/lsp-safe-edit` + `/lsp-verify` + `/lsp-test-correlation`. Formal runtime support for skill-to-skill invocation would enable arbitrary composition.

## Capability-Gated Skills

### The problem

Not every language server supports the same capabilities. gopls supports call hierarchy, type hierarchy, and semantic tokens. Gleam's LSP does not. But `/lsp-impact` calls all three. Currently, skills handle this at runtime: if a tool returns `IsError` or empty, the agent skips the step or improvises. This works but is fragile and depends on the agent reading prose instructions correctly.

The 30 CI-verified languages expose different capability profiles. A skill that works perfectly with gopls may produce partial or misleading results with a less capable server, and the agent has no way to know this before activating the skill.

### The solution: capability metadata in SKILL.md frontmatter

Each skill declares which LSP server capabilities it requires and which are optional enhancements. Agents (or a skill runner) check these against `get_server_capabilities` before activation.

```yaml
---
name: lsp-impact
description: Blast-radius analysis for a symbol or file.
license: MIT
compatibility: Requires the agent-lsp MCP server
metadata:
  required-capabilities: referencesProvider documentSymbolProvider
  optional-capabilities: callHierarchyProvider typeHierarchyProvider
allowed-tools: mcp__lsp__find_references mcp__lsp__find_callers ...
---
```

**Behavior when a required capability is missing:** The agent receives a warning before activation: "This skill requires `referencesProvider` which the current language server does not support. The skill may produce incomplete results." The agent can decide whether to proceed.

**Behavior when an optional capability is missing:** The skill activates normally. Steps that use the optional capability skip cleanly. The agent sees which steps were skipped in the output.

### Capability profiles by language

Based on CI testing across 30 languages, the capability landscape clusters into tiers:

| Tier | Capabilities | Languages |
|------|-------------|-----------|
| Full | All 60 tools viable | Go (gopls), TypeScript, Rust, C/C++ (clangd), C# |
| Strong | Most tools; missing call/type hierarchy | Python, Ruby, PHP, Kotlin, Swift, Dart, Gleam, Elixir |
| Basic | Navigation + diagnostics; limited refactoring | YAML, JSON, Dockerfile, CSS, HTML, Terraform, SQL |

Skills that target the "Strong" tier should avoid hard dependencies on `callHierarchyProvider` and `typeHierarchyProvider`. Skills that require these should declare them so agents know the limitation upfront.

### Implementation plan

| Feature | Status | Description |
|---------|--------|-------------|
| **`required-capabilities` metadata** | **Shipped** | Space-separated list of LSP server capability keys in SKILL.md frontmatter `metadata` field. All 24 skills declare required and optional capabilities. |
| **`optional-capabilities` metadata** | **Shipped** | Same format. Steps using these capabilities skip cleanly when unavailable. No warning on activation. |
| **Capability check tool** | **Shipped** | Integrated into `get_server_capabilities`: `skills` array classifies all 24 skills as supported/partial/unsupported based on the current server's capabilities. |
| **Degraded-mode skill variants** | Planned | For high-value skills like `/lsp-impact`, define a degraded path in the skill body that uses only `find_references` when call/type hierarchy are unavailable. Explicit in the prose, not a separate skill file. |

### Fits the AgentSkills spec

The `metadata` field in the AgentSkills specification is an arbitrary key-value mapping. `required-capabilities` and `optional-capabilities` are custom keys that conforming agents can read. Agents that don't understand these keys ignore them, falling back to the current runtime behavior. No spec extension needed.

## Skill Schema Specification

Skills are currently prose: markdown prompts the agent follows. The inputs and outputs are implicit and unvalidatable. A schema layer would make contracts explicit (what goes in, what comes out), enabling validation and eventual skill composition with typed interfaces.

The case for machine-readable skill contracts:
- Tooling can validate that an agent invoked a skill correctly
- Clearer interface between the agent and the skill: what goes in, what comes out
- Enables skill composition with type safety (skill A's output feeds skill B's input)
- Documentation that can be auto-generated and kept in sync

| Feature | Status | Description |
|---------|--------|-------------|
| **Skill input/output schema** | Planned | JSON Schema definitions for each skill's expected inputs and guaranteed outputs, machine-readable contracts alongside the prose skill files |
| **Schema validation tooling** | Planned | Validate agent skill invocations against the schema at runtime or in CI, surfacing misuse before it causes silent failures |

## IDE Integration

agent-lsp already works with any IDE that has an MCP client (VS Code via Continue/Cline, JetBrains via AI Assistant, Cursor, Windsurf, Neovim via mcp.nvim). The items below improve this from "works" to "native."

### Passive mode (connect to existing language servers)

The `connect` parameter on `start_lsp` connects to an already-running language server via TCP instead of spawning a duplicate process. In IDE environments where gopls/pyright/rust-analyzer is already running and indexed, passive mode eliminates double-indexing and double memory usage.

```json
{ "tool": "start_lsp", "args": { "root_dir": "/project", "connect": "localhost:9999" } }
```

Language servers that support multi-client TCP connections (gopls via `gopls -listen=:9999`, clangd, etc.) share their warm index with agent-lsp. No IDE plugin required.

| Feature | Status | Description |
|---------|--------|-------------|
| **`connect` parameter on `start_lsp`** | **Shipped** | Connect to an existing language server via TCP (e.g. `gopls -listen=:9999`) instead of spawning a new process. Reuses the IDE's warm index with zero duplicate memory. |
| **Shared index** | **Shipped** | Passive mode reuses the IDE's warm language server index; no duplicate indexing or memory overhead. |

### IDE extensions

| Feature | Status | Description |
|---------|--------|-------------|
| **VS Code extension** | Planned | Auto-start agent-lsp, command palette for skills, inline diff preview for speculative execution, code lens for blast-radius annotations |
| **JetBrains plugin** | Planned | Single plugin for all JetBrains IDEs (GoLand, IntelliJ, PyCharm, WebStorm, CLion, Rider). Only needs `com.intellij.modules.platform` dependency since agent-lsp manages its own LSP connections. No language-specific module dependencies required. |
| **Neovim plugin** | Planned | Lua plugin using `vim.lsp.buf_get_clients()` to proxy requests through existing LSP connections |

## CI Performance Metrics

Instrument the existing test suite to capture per-language timing data on every CI run, then publish it as a public `docs/metrics.md` table. This turns CI from a pass/fail gate into a performance baseline.

### What to measure

| Metric | How | Where |
|--------|-----|-------|
| Server init time | `start_lsp` to first successful response | Existing multi-lang tests |
| Diagnostic settle time | `open_document` to `get_diagnostics` returning stable results | Existing multi-lang tests |
| Speculative execution confidence | `confidence` field from `preview_edit` (`high`/`partial`/`eventual`) | New speculative test per language |
| Speculative round-trip time | `preview_edit` call to response | New speculative test per language |
| Cross-file propagation time | Edit file A → diagnostics update in file B | New test using multi-file fixtures |
| Tool latency (hover, definition, references, completions) | Per-call `time.Since` wrapping | Existing tier-2 tool tests |

### Output schema

Each CI job writes `metrics/<language>.json`:

```json
{
  "language": "go",
  "server": "gopls",
  "init_ms": 1240,
  "diagnostic_settle_ms": 890,
  "speculative_confidence": "high",
  "speculative_round_trip_ms": 2100,
  "cross_file_propagation_ms": 1800,
  "tool_latency_ms": {
    "hover": 45,
    "definition": 62,
    "references": 310,
    "completions": 120
  },
  "timestamp": "2026-04-21T00:00:00Z",
  "ci_run_id": 12345
}
```

### Files to create/modify

| File | Change |
|------|--------|
| `test/metrics.go` | New: timing harness, JSON serialization, `WriteMetrics(path string)` |
| `test/multi_lang_test.go` | Instrument `TestMultiLanguage`: wrap each tool call with `time.Since`, collect into `LanguageMetrics` struct |
| `test/speculative_test.go` | Expand to all supported languages (currently Go only); record `speculative_confidence` and `speculative_round_trip_ms` per language |
| `.github/workflows/ci.yml` | Add `upload-artifact` step per language job; add `collect-metrics` job that runs after all language jobs, downloads all artifacts, and commits merged `metrics.json` to a `metrics` branch |
| `scripts/generate-metrics.py` | New: reads `metrics/<language>.json` files, computes p50/p95 after 5+ runs from `metrics/history.json`, renders `docs/metrics.md` |
| `docs/metrics.md` | Generated output, markdown table with one row per language |

### Public dashboard format

```markdown
| Language   | Server          | Init  | Diag Settle | Spec Confidence | Spec RT | Cross-file |
|------------|-----------------|-------|-------------|-----------------|---------|------------|
| Go         | gopls           | 1.2s  | 0.9s        | high            | 2.1s    | 1.8s       |
| Rust       | rust-analyzer   | 2.1s  | 1.4s        | high            | 2.8s    | 2.2s       |
| TypeScript | typescript-language-server | 0.8s  | 0.6s        | high            | 1.3s    | 1.1s       |
| Python     | pyright         | 1.5s  | 1.1s        | high            | 2.4s    | —          |
```

### Rolling averages

After 5+ CI runs, `generate-metrics.py` reads `metrics/history.json` on the `metrics` branch and replaces single-run numbers with p50/p95 per metric. The history file is a JSON array of per-run records; the script appends the latest run and trims to the last 50 entries.

### Implementation notes

- The timing harness must not fail the test on timeout. Capture what is available and write `-1` for unresolvable metrics.
- Cross-file propagation requires multi-file test fixtures; Go and TypeScript already have them in `test/testdata`; Python and Rust need new fixtures.
- Speculative confidence for languages without `high` confidence is expected. Record the actual value, not a failure.
- The `collect-metrics` CI job should only run on the `main` branch to avoid polluting the metrics branch with PR data.

## Control Plane

The agent-local pipeline (blast-radius → simulate → apply → verify → test) handles correctness for a single session. The control plane adds organizational primitives for teams running agents at scale.

| Feature | Status | Description |
|---------|--------|-------------|
| **Audit trail** | **Shipped** | JSONL log of every `apply_edit`, `rename_symbol`, and `commit_session` call with timestamp, affected files, edit summary, pre/post diagnostic state, and net_delta. Configure via `--audit-log` flag or `AGENT_LSP_AUDIT_LOG` env var. |
| **Change plan output** | Planned | Materialize `simulate_chain` output as a structured, human-reviewable artifact before apply: files, edits, per-step diagnostic delta, safe-to-apply watermark. Three community members have independently requested this. |
| **Policy gates** | Planned | Configurable rules that block apply based on blast-radius thresholds, public API changes, or path patterns. Evaluate at apply time using the audit record. |
| **Cross-session coordination** | Planned | Shared state between concurrent MCP sessions, specifically a symbol-level lock registry to prevent overlapping renames/refactors. Requires a sidecar daemon or file-based coordination. The hardest piece. |

## Agent Evaluation Framework

### Shipped: deterministic trajectory assertions (skill protocol CI)

All 24 skills now have deterministic trajectory assertions in `examples/mcp-assert/trajectory/`. These run in the `mcp-assert-trajectory` CI job on every push and PR: 24 inline-trace assertions, no server needed, 0ms each, under 60 seconds total. They validate `presence`, `absence`, `order`, and `args_contain` rules for each skill's required tool call sequence. This is the deterministic subset of Layer 2 skill workflow testing — not LLM-driven, but covering the structural protocol requirements that can be verified without a running agent. The LLM-driven pass@k/pass^k regression suite (below) remains planned.

### Why existing eval frameworks don't fit

Two categories of eval frameworks exist, and neither addresses what agent-lsp needs:

**Agent eval frameworks** ([Strands Evals](https://github.com/strands-agents/evals), [Braintrust](https://braintrust.dev), [LangSmith](https://docs.langchain.com/langsmith), [AgentBench](https://github.com/THUDM/AgentBench), [SWE-bench](https://github.com/SWE-bench/SWE-bench), [BFCL/Gorilla](https://github.com/ShishirPatil/gorilla)) evaluate from the **agent/model perspective**: "did the model call the right tool?" They test agents, not tool providers.

**MCP eval frameworks** ([mcp-evals](https://github.com/mclenhard/mcp-evals) 129 stars, [alpic-ai/mcp-eval](https://github.com/alpic-ai/mcp-eval) 21 stars, [lastmile-ai/mcp-eval](https://github.com/lastmile-ai/mcp-eval) 20 stars, [dylibso/mcpx-eval](https://github.com/dylibso/mcpx-eval) 22 stars, [gleanwork/mcp-server-tester](https://github.com/gleanwork/mcp-server-tester) 13 stars) test MCP servers directly, but every one uses **LLM-as-judge** scoring. They send a prompt, get a response, and ask an LLM "was this good?" on a 1-5 rubric. This makes sense for subjective tool outputs (e.g., "summarize this document") but is the wrong approach for deterministic tools.

When `find_references` is called on line 42 of a Go file, the correct answer is a deterministic set of locations. No LLM-as-judge is needed. The tool either returns the right locations or it does not. Paying for GPT-4 API calls to grade a response that can be verified with `assert.Equal` is wasteful and introduces false variance.

**The gap:** At the time of writing, no framework combined deterministic tool correctness testing with MCP server evaluation. No framework tested across multiple languages or programming environments. No framework measured tool reliability or skill protocol compliance. **mcp-assert now fills this gap** (see below).

The only framework with native MCP integration is [Inspect AI](https://github.com/UKGovernmentBEIS/inspect_ai) (1,900+ stars, UK AI Safety Institute). It can serve MCP tools to an evaluated model and score the results. This is useful for Layer 2 (skill workflow testing) but unnecessary for Layer 1 (tool correctness), which is 80% of the work.

### mcp-assert: shipped sister project

The gap identified above is what [mcp-assert](https://github.com/blackwell-systems/mcp-assert) fills. It shipped as a separate repo (`blackwell-systems/mcp-assert`) and is now at **v0.8.0**.

**What it is:** A Go-based, deterministic-first testing framework for MCP servers. Given an MCP server binary or URL, run its tools against fixture inputs and grade the outputs. No LLM required for correctness testing. Single binary, CI-native, zero API costs.

**Shipped commands:**

| Command | Description |
|---------|-------------|
| `run` | Execute YAML assertion suites against a live MCP server |
| `audit` | Connect to a server, discover all tools, call each with schema-generated inputs, report health vs. crashes |
| `fuzz` | Category-based adversarial input generation (empty/null args, wrong types, boundary values, injection payloads) |
| `generate` | Auto-generate stub YAML assertions from a server's tool schema |
| `snapshot` | Capture and compare tool output snapshots for regression detection |
| `inspect` | Static analysis of the MCP server codebase (race conditions, error handling, protocol compliance) |
| `watch` | File-watching mode for local development |

**How it differs from existing MCP eval tools:**

| Dimension | Existing MCP evals | mcp-assert |
|---|---|---|
| Grading | LLM-as-judge (subjective, costly) | Deterministic assertions (exact, free) |
| Language | Node.js / Python | Go (single binary, fast CI) |
| Fuzz testing | Not supported | Category-based adversarial inputs from JSON Schema |
| Docker isolation | Not supported | `--docker` flag for destructive tool isolation |
| Output formats | Varies | JSON, JUnit XML, Markdown |
| Transport | Usually stdio only | stdio, HTTP, SSE |

**Relationship to agent-lsp:** agent-lsp's skill trajectory assertions use mcp-assert in CI. mcp-assert's fuzz and audit commands have found bugs in 5 official MCP SDKs (TypeScript, Python, PHP, Go, mcp-go). The projects feed each other: agent-lsp is the reference MCP server that exercises mcp-assert's testing capabilities.

```bash
# Audit any MCP server
mcp-assert audit --server "npx -y @modelcontextprotocol/server-everything" --output ./assertions

# Fuzz test for crashes
mcp-assert fuzz --server "npx -y @modelcontextprotocol/server-everything" --json

# Run assertion suite in CI
mcp-assert run --suite ./assertions --server "npx -y @modelcontextprotocol/server-everything" --junit results.xml
```

### Two-layer architecture

| Layer | What it tests | Requires LLM? | Grading | Priority |
|---|---|---|---|---|
| **Layer 1: Tool Correctness** | Does each tool return correct results for known inputs? | No | Deterministic (expected output comparison) | High (80% of eval value) |
| **Layer 2: Skill Workflow** | Do agents follow skill protocols correctly? | Yes (agent orchestrates) | Trajectory matching + outcome verification | Medium (20% of eval value) |

**Layer 1** is formalized integration testing: call the MCP tool directly with known inputs against real fixture repos, compare output against expected results. No model variability. No flakiness. This is what the CI test matrix already does, expanded to cover every tool x language combination with richer assertions.

**Layer 2** requires an LLM to orchestrate (skills are agent-driven). Capture the tool call trace, compare against expected sequences. This layer is inherently noisy because model behavior varies. Use it for regression detection, not pass/fail gating.

### Patterns borrowed from existing frameworks

| Source | Pattern | Applied to |
|---|---|---|
| [SWE-bench](https://github.com/SWE-bench/SWE-bench) | Docker-isolated execution, real codebases as fixtures, deterministic grading | Layer 1: Docker eval harness, fixture repos per language |
| [Strands Evals](https://github.com/strands-agents/evals) | Trajectory scorers (`in_order_match_scorer`, `any_order_match_scorer`) | Layer 2: skill step ordering verification |
| [BFCL/Gorilla](https://github.com/ShishirPatil/gorilla) | AST-based tool call argument comparison | Layer 2: verify tool call arguments match expected values |
| [Inspect AI](https://github.com/UKGovernmentBEIS/inspect_ai) | MCP-aware eval harness with custom `@scorer` decorators | Layer 2: end-to-end skill evaluation through a model |
| [mcp-server-evaluations](https://github.com/mcp-com-ai/mcp-server-evaluations-skills) | 5-dimension MCP server quality rubric (discovery, functionality, error handling, accuracy, performance) | Layer 1: quality dimensions for tool evaluation |

### Layer 1: Tool Correctness (deterministic, no LLM)

For each of 60 tools across N languages, maintain test fixtures with expected outputs. Call the MCP tool directly, compare output against expected results. Organized as Go table-driven tests with per-language, per-tool coverage tracking.

**What this looks like in practice:**

```go
// test/evals/tool_correctness_test.go
func TestToolCorrectness(t *testing.T) {
    cases := []struct {
        tool     string
        language string
        fixture  string
        input    map[string]any
        assert   func(t *testing.T, result string)
    }{
        {
            tool: "find_references", language: "go",
            fixture: "test/fixtures/go",
            input: map[string]any{
                "file_path": "greeter.go", "line": 10, "column": 6,
            },
            assert: func(t *testing.T, result string) {
                // Person type should be referenced in main.go and greeter.go
                assert.Contains(t, result, "main.go")
                assert.Contains(t, result, "greeter.go")
                assert.JSONFieldCount(t, result, "locations", 3)
            },
        },
        {
            tool: "find_references", language: "gleam",
            fixture: "test/fixtures/gleam",
            input: map[string]any{
                "file_path": "src/person.gleam", "line": 1, "column": 10,
            },
            assert: func(t *testing.T, result string) {
                assert.Contains(t, result, "greeter.gleam")
            },
        },
        // ... 60 tools x 30 languages
    }
}
```

**Coverage tracking:** A generated `docs/eval-coverage.md` table shows pass/fail per tool per language, replacing the manually-maintained CI coverage matrix.

### Skill evals (Layer 2: regression suite)

Each skill has a deterministic correct sequence. Skill evals verify that agents follow the sequence consistently, not just once. This is the `pass^k` metric from the eval literature: does the agent follow the protocol every time?

**Task format:**

```yaml
# test/evals/lsp-rename/rename_type.yaml
task: "Rename the Person type to Entity in the Go fixture"
language: go
fixture: test/fixtures/go
graders:
  - type: transcript
    assert: tool_called("prepare_rename") before tool_called("rename_symbol")
  - type: transcript
    assert: tool_called("rename_symbol", dry_run=true) before tool_called("apply_edit")
  - type: transcript
    assert: tool_called("get_diagnostics") after tool_called("apply_edit")
  - type: outcome
    assert: file_contains("greeter.go", "Entity")
  - type: outcome
    assert: net_delta == 0
```

**Coverage target:** 3-5 tasks per skill, covering the happy path, the halt-on-error path (e.g., high blast radius should stop `/lsp-refactor`), and edge cases (zero references, already-renamed symbol).

```
test/evals/
  lsp-rename/
    rename_type.yaml           # standard rename across files
    rename_function.yaml       # rename with many callers
    rename_no_prepare.yaml     # server doesn't support prepare_rename
  lsp-refactor/
    refactor_safe.yaml         # full pipeline, net_delta == 0
    refactor_high_blast.yaml   # > 20 callers, should halt at gate
    refactor_breaking.yaml     # net_delta > 0, should discard
  lsp-safe-edit/
    safe_edit_clean.yaml       # edit introduces no errors
    safe_edit_breaking.yaml    # edit introduces errors, should surface code actions
  lsp-impact/
    impact_file.yaml           # file-level blast radius
    impact_symbol.yaml         # symbol-level with call hierarchy
    impact_no_hierarchy.yaml   # server lacks callHierarchyProvider
```

**When to run:** On every new model release (Claude, GPT, Gemini). On every skill change. The eval suite answers: "do agents still use our tools correctly after this update?"

### Speculative execution as a built-in grader

`preview_edit` is a code-based grader for edit quality. `net_delta == 0` means the edit is safe. `net_delta > 0` means the agent introduced errors. This is unique to agent-lsp: the tool itself is the eval.

**Metric to track:** First-attempt success rate. What percentage of agent edits produce `net_delta == 0` on first attempt, without a retry? Track this across:

| Dimension | Why it matters |
|-----------|---------------|
| By model | Does Claude produce cleaner first-attempt edits than GPT? |
| By language | Are Go edits safer than Python edits? (Stronger type system = more diagnostic coverage) |
| By skill vs. freestyle | Do skills improve first-attempt success rate vs. raw tool usage? |
| By edit type | Are renames safer than signature changes? Are comment edits always clean? |

This data comes from the audit trail. No new infrastructure needed, just a script that aggregates `net_delta` values from the JSONL log.

### Capability evals (the CI test matrix)

The existing CI test matrix is a capability eval suite. Each language has a set of tools tested against real fixtures. The eval framework from the Anthropic article provides terminology for what we already do:

| CI concept | Eval terminology |
|---|---|
| Language test passing at < 100% | Capability eval (driving improvement) |
| Language test passing at 100% | Regression eval (protecting against backsliding) |
| Adding a new tool test | Expanding capability coverage |
| Test that starts flaking | Eval degradation (investigate, don't ignore) |

**Graduation rule:** When a language reaches 100% on its capability matrix for 5+ consecutive CI runs, it graduates to a regression eval. Any drop below 100% on a regression eval is a blocking failure, not a flake to be skipped.

### Audit trail graders (production monitoring)

The audit trail (`--audit-log`) is a transcript. Post-session graders analyze the JSONL log for protocol compliance and quality signals:

| Grader | What it checks | Type |
|--------|---------------|------|
| **Blast-radius-first** | Was `get_change_impact` or `find_references` called before any `apply_edit` on an exported symbol? | Transcript |
| **Simulate-before-apply** | Was `preview_edit` called before `apply_edit` when a skill was active? | Transcript |
| **Rename protocol** | Was `prepare_rename` called before `rename_symbol`? | Transcript |
| **Uncaught regression** | Did any `apply_edit` produce `net_delta > 0` without a subsequent `discard_session` or fix? | Outcome |
| **Tool error rate** | What percentage of tool calls returned `IsError: true`? High rates indicate misconfiguration or model confusion. | Metric |
| **Session hygiene** | Was every `create_simulation_session` followed by `destroy_session`? Leaked sessions waste memory. | Transcript |

**Implementation:** A CLI subcommand `agent-lsp eval --audit-log /path/to/audit.jsonl` that runs all graders against a log file and produces a report. Useful for post-incident review ("what did the agent actually do?") and for continuous monitoring in team deployments.

### Negative evals

Eval tasks should cover cases where the skill should correctly refuse to act:

| Skill | Negative eval | Expected behavior |
|---|---|---|
| `/lsp-rename` | User says "rename the file" (file rename, not symbol) | Skill does not activate or asks for clarification |
| `/lsp-refactor` | Blast radius > 20 callers | Halts at gate, reports risk, does not proceed |
| `/lsp-safe-edit` | `net_delta > 0` after simulation | Does not write to disk; surfaces errors and code actions |
| `/lsp-impact` | File path does not exist | Returns clear error, does not crash |
| `/lsp-rename` | Cursor on a keyword or built-in type | `prepare_rename` rejects; skill stops |

Negative evals prevent the most dangerous failure mode: an agent that confidently does the wrong thing. A skill that correctly refuses is more valuable than one that blindly proceeds.

### Docker-isolated eval harness

Each eval task runs in an isolated Docker container using the existing agent-lsp Docker images. This guarantees clean state, prevents cross-trial contamination, and provides reproducible environments with language servers pre-installed.

**Architecture:**

```
agent-lsp eval run \
  --task test/evals/lsp-rename/rename_type.yaml \
  --image ghcr.io/blackwell-systems/agent-lsp:go \
  --agent claude \
  --trials 5
```

```
Per trial:
┌──────────────────────────────────────────────┐
│  Docker container (ghcr.io/.../agent-lsp:go) │
│                                              │
│  1. Copy fixture into /workspace             │
│  2. Place skills in agent's skill directory   │
│  3. Start agent-lsp with --audit-log          │
│  4. Agent receives task prompt                │
│  5. Agent discovers skills organically        │
│  6. Agent executes (tool calls logged)        │
│  7. Container stops                           │
│                                              │
│  Output: audit.jsonl + workspace state        │
└──────────────────────────────────────────────┘
         │
         ▼
  Graders run against audit.jsonl + workspace:
  - Deterministic: file state, net_delta, tool ordering
  - LLM rubric: code quality, architectural choices
  - Negative: skill correctly refused when it should have
         │
         ▼
  Aggregate across trials:
  - pass@k (capability: did it work at least once?)
  - pass^k (reliability: did it work every time?)
  - First-attempt net_delta success rate
  - Token usage, duration, command count
```

**Why Docker:** The existing Docker images (`ghcr.io/blackwell-systems/agent-lsp:go`, `:typescript`, `:python`, etc.) already contain agent-lsp + the language server. The eval harness reuses these images rather than building separate eval infrastructure. Each trial starts from the same base image with the same toolchain version, eliminating "works on my machine" variance.

**Multi-language eval matrix:** Run the same eval task across multiple language images to measure whether skills degrade across languages:

```
agent-lsp eval matrix \
  --task test/evals/lsp-rename/rename_type.yaml \
  --images go,typescript,python,rust,gleam \
  --trials 5
```

This produces a table showing pass rates per language per skill, directly answering: "which skills need capability gating for which languages?"

### Implementation priority

| Phase | What | Effort | Impact |
|---|---|---|---|
| **Phase 1** | Reframe the CI test matrix as capability/regression evals. Add graduation rule. Maximize niche language coverage (Gleam, Zig, Elixir, Clojure, Nix, Dart). | 1-2 weeks | Immediate coverage gains. Community distribution via niche language posts. |
| **Phase 2** | Build 3-5 skill eval YAML tasks per skill including negative evals. Run against real Claude Code sessions. Grade transcripts for step ordering and refusal correctness. | 1-2 days per skill | Proves skills work. Catches regressions on model updates. |
| **Phase 3** | Docker-isolated eval harness using existing images. `agent-lsp eval run` and `agent-lsp eval matrix` CLI subcommands. Multi-trial execution with pass@k/pass^k metrics. | 1 week | Reproducible, containerized evaluation. Cross-language skill reliability data. |
| **Phase 4** | Audit trail aggregation and production graders. Track `net_delta` first-attempt success rates by model/language/skill. `agent-lsp eval --audit-log` for post-session analysis. | 2-3 days | Production monitoring. Data-driven skill improvement. Marketing ammunition. |

## Skill Phase Enforcement

agent-lsp is a persistent runtime with stateful sessions (simulation sessions, workspace state, diagnostics cache). It already enforces ordering within sessions (e.g., you can't `evaluate_session` before `create_simulation_session`). Extending this to skill-level phases is a natural evolution.

### How it works

When an agent activates a skill, agent-lsp tracks which phase the skill is in based on tool call history. The `tool_permissions` metadata (already added to 4 skills) declares which tools are allowed per phase. The runtime infers the current phase from the tools called so far and blocks calls that violate the current phase's permissions.

```
Agent calls prepare_rename -> phase = "preview"
Agent calls find_references -> still "preview"
Agent tries apply_edit     -> BLOCKED: "apply_edit is not allowed in the preview phase. 
                               Complete the preview by calling rename_symbol first."
Agent calls rename_symbol  -> phase = "execute", apply_edit now allowed
```

### Differences from Centian's approach

| Dimension | Centian | agent-lsp |
|-----------|---------|-----------|
| Architecture | Separate proxy process, routes all traffic | Built into the tool provider that already has state |
| Setup | Reroute MCP traffic, register tasks, write templates | Zero setup; permissions are in the skill YAML |
| Phase inference | Explicit registration (`task_start_step`) | Automatic from tool call history |
| Recovery | Generic "call centian.task_resume" | Skill-specific ("call rename_symbol to complete preview") |
| Session state | In-memory only, lost on restart | Could persist via existing audit trail |
| Scope | Any MCP server (proxy is server-agnostic) | agent-lsp tools only (the runtime owns the state) |

### Implementation status

| Item | Status | Description |
|------|--------|-------------|
| **Phase state machine** | **Shipped** | `internal/phase/tracker.go`: thread-safe `Tracker` with `ActivateSkill`, `DeactivateSkill`, `CheckAndRecord`, `Status`. Auto-advances phases based on tool call patterns. |
| **Enforcement modes** | **Shipped** | `warn` (log violation, allow call) and `block` (return isError with recovery guidance). Default: `warn`. |
| **Phase inference rules** | **Shipped** | Tools matching a later phase's allowed list auto-advance. Tools not in any config pass through. Global forbidden checked first. |
| **Structured recovery actions** | **Shipped** | Blocked calls return JSON with `reason`, `recovery` (which tools to call), `current_phase`, `skill`. |
| **Audit trail integration** | **Shipped** | `activate_skill`, `deactivate_skill`, `phase_advance`, `phase_violation` events logged to JSONL audit trail. |
| **MCP tools** | **Shipped** | 3 tools: `activate_skill(skill_name, mode)`, `deactivate_skill()`, `get_skill_phase()`. |
| **Generic wrapper** | **Shipped** | `addToolWithPhaseCheck[T]` wraps all tool handlers; single-line replacement across 4 tool files. |
| **Arg-level enforcement** | Planned | Enforce `dry_run=true` vs `dry_run=false` for `rename_symbol` in preview vs execute phases. |

### Skills with tool_permissions (shipped)

- `/lsp-rename` (3 phases: prerequisites, preview, execute)
- `/lsp-refactor` (5 phases: blast_radius, speculative_preview, apply, build_verification, test_execution)
- `/lsp-safe-edit` (4 phases: setup, speculative_preview, apply, verify_and_fix)
- `/lsp-verify` (5 phases: test_correlation, diagnostics, build, tests, fix_and_format)

See [docs/phase-enforcement.md](phase-enforcement.md) for the full design document.

## Bigger Bets

| Feature | Status | Description |
|---------|--------|-------------|
| **LSP server mode (proxy architecture)** | Planned | Expose agent-lsp as an LSP server that editors connect to directly, not just an MCP server for AI agents. Today agent-lsp is a client of gopls/pyright; in this mode it becomes a transparent proxy that sits between the editor and the language server, intercepting and enriching the LSP stream. Standard LSP requests (hover, go-to-definition, completions) pass through to the real language server. agent-lsp injects its own capabilities on top: (1) speculative execution results surfaced as LSP diagnostics (the editor shows "this edit would introduce 3 errors" inline), (2) blast-radius annotations as code lenses ("42 callers" above each exported function), (3) skill-driven refactors exposed as code actions (right-click a function, select "Safe Refactor" which runs the full /lsp-refactor workflow), (4) dead-symbol detection as dimmed/unused diagnostics. The editor gets agent-lsp's intelligence natively without MCP or an AI agent in the loop. This opens a second market: developers who want agent-lsp's analysis but use traditional editors (VS Code, Neovim, Helix, Emacs) without an AI agent. Implementation: agent-lsp listens on a port or stdio as an LSP server (using Go's `gopls/internal/protocol` types or a lightweight LSP server library), proxies all requests to the real language server, and intercepts responses to enrich them. The persistent cache (Layer 3) serves the enrichment data without re-querying. Inspired by lsp-ai's architecture (3.2K stars, Rust) which proved editors will connect to a proxy language server that adds capabilities. This is a v2.0 architectural shift, not a feature addition. |
| **Observability** | Planned | Metrics (requests/sec, latency per tool, error rate) for production deployments, valuable for teams running agent-lsp as shared infrastructure |
| **Persistent knowledge graph** | **Shipped** (reference cache layer) | Language-agnostic infrastructure layer below the LSP clients. Caches LSP-derived data (symbol references, call hierarchies, type relationships, diagnostic baselines, clone fingerprints) in a persistent SQLite store. Schema is language-agnostic: a Go function calling a Python function via cross-repo lives in the same graph. This layer is the foundation for `detect_changes`, `/lsp-architecture`, near-clone detection, and the team-shared index artifact. Implementation: pure Go SQLite (`modernc.org/sqlite`) initially; CGo acceleration (`mattn/go-sqlite3` + C libraries) if clone detection or semantic search require bulk computation at scale. The server/concurrency/protocol layer stays pure Go; only compute-intensive graph operations would use C under the hood. **Lifecycle:** Storage at `~/.agent-lsp/cache/<workspace-hash>/graph.db`, created on first `start_lsp`. Population is opportunistic: every LSP response (`find_references`, `get_change_impact`, `list_symbols`) is cached as a side effect, no separate index step. Invalidation via file watcher: on file change, evict cached entries for that file plus transitive dependents (if known from cached call graph); next query re-queries the LSP server and re-caches. Staleness guard: on session start, check `max(cached_at)`; if older than configurable threshold, invalidate all and repopulate organically. Corruption handling: SQLite WAL mode for crash safety; on open, `PRAGMA integrity_check`; if corrupted, delete and start fresh (cache is disposable). Cleanup: `agent-lsp cleanup` removes caches for workspaces that no longer exist on disk; daemon auto-scans on startup. Size management: configurable max cache size (default 500MB), LRU eviction by workspace. The cache is not mandatory: agent-lsp works without it (queries LSP directly, like today). Missing, corrupted, or stale cache falls back to existing behavior transparently. |
| **`detect_changes` tool** | **Shipped** | Single-call "what did I break?" workflow. Runs `git diff` to identify changed files, feeds them to `get_change_impact`, returns affected symbols with risk classification. Eliminates the manual step of identifying which files to analyze. Inspired by codebase-memory-mcp's `detect_changes`. |
| **`/lsp-architecture` skill** | **Shipped** | Project-level architecture overview in one call. Runs `list_symbols` across all packages, synthesizes package dependency graph, entry points, layer structure, and hotspots (files with highest fan-out/fan-in). Currently `/lsp-understand` only does per-file analysis. |
| **Near-clone detection in `/lsp-inspect`** | Planned | New check type `duplicate_semantics` that identifies functions with high structural similarity. Uses AST-level comparison (shared statement patterns, parameter shapes) rather than text similarity. Surfaces "these two functions are 90% identical, consider extracting a shared helper." |
| **Team-shared index artifact** | **Shipped** | Persist a warm-index snapshot (reference counts, symbol graph, diagnostic baseline) as a compressed file that can be committed to the repo. New sessions load cached state instead of re-indexing from scratch. Eliminates cold-start cost for teams. |
| **`agent-lsp update`** | **Shipped** | Self-update to the latest release; fetches from GitHub Releases and replaces the binary in-place. |
| **`agent-lsp uninstall`** | **Shipped** | Clean removal of all agent configs, skill installations, instruction file entries, and hooks across all supported agents. |
| **Runtime trace correlation** | Planned | Ingest test execution traces and correlate with static call hierarchy from LSP. Identify "called at runtime but zero test coverage" and "test covers code paths that static analysis says are dead." Bridges the gap between static reference analysis and actual execution paths. |
| **Louvain community detection** | Planned | Cluster symbols by call-edge density to discover functional modules. Enhances `/lsp-architecture` with automatically detected boundaries and module groupings, without requiring explicit package structure. |
| **Cross-service HTTP route linking** | Planned | Match `fetch("/api/users")` call sites to `@app.route("/api/users")` handler definitions across files and repos. Extends `get_cross_repo_references` beyond symbol-level to HTTP route-level cross-service analysis. |
| **Git coupling analysis** | Planned | Identify files that change together frequently (e.g., "payment_handler.py changes alongside user_service.py 80% of the time"). Mine `git log` for co-change patterns. Integrate with `detect_changes`: when a file is modified, surface coupled files the agent should also review. Implementation: `git log --name-only --format=""` parsed into a co-occurrence matrix, filtered by threshold (default 60%). Lightweight addition to `detect_changes` response. Inspired by Axon's coupling heatmap. |
| **Async execution flow tracing** | Planned | Trace call chains across async boundaries (callbacks, goroutine launches, event emitters, promise chains). LSP's call hierarchy is synchronous: `go func() { doWork() }()` doesn't show `doWork` as a caller of the launching function. Static analysis of `go func`, `asyncio.create_task`, `.then()`, `EventEmitter.on()` patterns to build async edges in the call graph. Enhances `/lsp-inspect` (find unrecovered panics in async paths) and `/lsp-impact` (blast radius across async boundaries). |
| **Next-step hints in tool responses** | **Shipped** | Every tool response includes a contextual `hint` field suggesting the logical next tool call. Example: `find_references` returns "use get_change_impact to see the full blast radius." Helps agents chain tools correctly without skills, and helps less capable models navigate the 60-tool surface. Zero-cost addition: one extra field in the JSON response. |
| **Tree-sitter fallback** | Planned | See Massive Codebase Strategy section below. |
| **Selective indexing** | **Shipped** | Auto-detects package boundary for Python/TypeScript on workspaces with 500+ source files. Generates scoped language server config. Scope shifts automatically on `open_document`. See Massive Codebase Strategy section below. |

---

## Massive Codebase Strategy

**Status:** Partially shipped (Layer 2: selective indexing, Layer 3: persistent cache). Layer 1 (tree-sitter) remains planned.

**Problem:** LSP servers resolve the full type/dependency graph of a workspace. On small-to-medium codebases (up to ~50K lines), this works well: gopls indexes in seconds, pyright in tens of seconds, and all tools return accurate results. On massive codebases (langchain at 136K stars, openclaw, Linux kernel at 28M LOC), full LSP indexing is either too slow (minutes), too memory-intensive (gigabytes), or both. Pyright resolves the full import graph regardless of workspace scoping. gopls loads all packages transitively referenced by the workspace root.

**Current state:** agent-lsp works well up to ~50K lines. The `scope` parameter on `start_lsp` helps with monorepos but doesn't solve the fundamental problem: the language server wants to load everything.

### Three-layer architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Skills layer                             │
│  Chooses which query layer to use based on the operation     │
├──────────┬──────────────────┬────────────────────────────────┤
│ Layer 1  │    Layer 2       │         Layer 3                │
│ Tree-    │    Selective     │         Persistent             │
│ sitter   │    LSP           │         Knowledge Graph        │
├──────────┼──────────────────┼────────────────────────────────┤
│ Fast     │ Accurate         │ Instant (cached)               │
│ Broad    │ Narrow           │ Grows over time                │
│ No deps  │ Active pkg only  │ All previously-queried symbols │
│ Any size │ Bounded memory   │ SQLite on disk                 │
└──────────┴──────────────────┴────────────────────────────────┘
```

### Layer 1: Tree-sitter (structural discovery)

Parses each file independently using tree-sitter AST analysis. No dependency resolution, no type inference, no memory accumulation. Viable at any scale because each file is processed in isolation.

**What it provides:**
- Symbol extraction (functions, types, classes, methods, constants)
- Call site detection (which functions appear to call which other functions, by name)
- Function boundaries (start line, end line, parameter count)
- Import/require graph (which files import which modules)
- File outlines (structural summary without reading the full file)

**What it cannot provide:**
- Type resolution (what type does this variable have?)
- Cross-package references (is this the `Close` from package A or package B?)
- Diagnostics (does this code compile?)
- Rename safety (will renaming this break callers?)

**When skills use it:**
- `/lsp-architecture`: needs to scan every file for structure. Tree-sitter does this in seconds on any codebase. Full LSP would take minutes.
- `/lsp-inspect` (structural scan phase): find `go func()` blocks, identify error handling patterns, locate channel operations. Pattern-based, not type-based.
- `/lsp-dead-code` (candidate identification): tree-sitter finds all exported symbols. LSP then verifies the candidates that look dead.

**Implementation:** CGo bindings to the C tree-sitter library with vendored grammars for all 30 CI-verified languages. Falls back to tree-sitter automatically for languages without a configured LSP server (Bash, YAML, Terraform, Dockerfile, Makefile).

### Layer 2: Selective LSP (precision work)

Full LSP, but scoped to the active package and its direct dependencies instead of the full transitive closure.

**How it works:**
1. Agent opens a file. agent-lsp identifies the containing package.
2. Instead of indexing the entire workspace, agent-lsp generates a scoped config that limits the language server to that package and its direct imports.
3. The language server loads quickly (seconds, not minutes) because it only resolves one level of dependencies.
4. When the agent moves to a different package, the scope shifts. The old package's results are cached in Layer 3.

**What it provides:** Full LSP accuracy (types, references, diagnostics, rename, code actions) within the scoped region.

**Limitation:** Cross-package references outside the scope return incomplete results. The knowledge graph (Layer 3) fills this gap with cached data from previous scopes.

**Extends:** The existing `scope` parameter on `start_lsp`. Currently requires the user to specify paths. Selective indexing would auto-detect the scope from the agent's current file.

### Layer 3: Persistent knowledge graph (cumulative cache)

SQLite-backed cache of all LSP results across sessions. Every reference lookup, every symbol list, every call hierarchy result is stored keyed by file content hash. Invalidated by file watcher when source changes.

**What it provides:** Instant results for any symbol the agent (or a previous session) already queried. Grows over time as the agent works across the codebase. After a few sessions, most of the codebase is cached.

**How it interacts with the other layers:**
- Tree-sitter identifies candidates (e.g., "these 50 functions might be dead code").
- The knowledge graph answers immediately for the 30 that were queried before ("these 20 have callers, these 10 don't").
- Selective LSP verifies the remaining 20 that aren't cached yet.

See the Persistent Knowledge Graph entry in Bigger Bets for lifecycle details (creation, invalidation, corruption handling, size management).

### How skills choose layers

| Skill | Layer 1 (tree-sitter) | Layer 2 (LSP) | Layer 3 (cache) |
|-------|----------------------|---------------|-----------------|
| `/lsp-architecture` | Primary (structural scan) | Not used | Supplements with cached type info |
| `/lsp-rename` | Not used | Primary (must be accurate) | Pre-checks blast radius from cache |
| `/lsp-inspect` | Structural patterns | Reference verification | Cached dead-symbol results |
| `/lsp-dead-code` | Candidate identification | Verification of candidates | Cached reference counts |
| `/lsp-impact` | Not used | Primary | Cached caller lists |
| `/lsp-understand` | File outline | Call hierarchy, references | Cached results for known symbols |
| `/lsp-refactor` | Not used | Primary (full workflow) | Pre-populates blast radius |

### What this means for users

- **Small codebases (under 50K lines):** No change. Full LSP works perfectly. Layer 3 (cache) makes repeat operations instant.
- **Medium codebases (50K-200K lines):** Layer 3 eliminates cold-start pain. First session is the same as today. Second session is fast for previously-queried areas.
- **Massive codebases (200K+ lines):** All three layers working together. Tree-sitter for discovery, selective LSP for the active area, cache for everything else. The agent can navigate langchain or the Linux kernel without waiting for a full index.

### Implementation order

1. **Knowledge graph (Layer 3):** **Shipped.** Pure Go SQLite cache (`~/.agent-lsp/cache/<hash>/refs.db`). Accelerates `get_change_impact` and reference queries. Export/import via `export_cache`/`import_cache` tools for team sharing.
2. **Selective indexing (Layer 2):** **Shipped.** Auto-detects package boundary for Python and TypeScript on workspaces with 500+ source files. Generates scoped `pyrightconfig.json`/`tsconfig.json`. Scope shifts automatically on `open_document`.
3. **Tree-sitter fallback (Layer 1):** Planned. Adds structural queries for languages without LSP and for broad-scan operations. CGo dependency.
