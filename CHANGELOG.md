# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, Semantic Versioning.

## [Unreleased]

### Added

- **`/lsp-onboard` skill (23rd skill).** First-session project onboarding. Explores the project structure via LSP tools: detects languages and build system, identifies entry points, maps package structure, finds hotspots (most-referenced files), and checks for pre-existing diagnostics. Produces a structured project profile for the agent's reference throughout the session.

- **`get_editing_context` composite tool (tool #61).** Single call returns file symbols with signatures, callers partitioned by test/non-test, callees, and imports. Supports `if_none_match` for conditional responses. Replaces the 3-5 tool sequence agents previously used to gather pre-edit context.

- **Token savings metadata.** `list_symbols`, `get_symbol_source`, and `get_editing_context` now include `_meta.token_savings` in responses showing tokens returned vs full file size. Makes the efficiency story visible on every call.

- **ETag/conditional responses.** File-scoped tools accept `if_none_match` parameter. When the file's content hash matches, returns `not_modified` instead of recomputing.

### Fixed

- **`get_change_impact` now includes exported methods.** Methods like `(*Hub).SetSender` were missed because the name starts with `(`, failing the uppercase export check. Now extracts the method name after the last dot before checking case. Previously only top-level functions and types were analyzed; methods on types (e.g., `(*Hub).Send`) were skipped because `collectExportedSymbols` didn't recurse into children. Now recurses into type children while filtering out struct fields. Found by GPT-5.5 agent evaluation.

- **`safe_delete_symbol` column resolution.** Same `SelectionRange.Start` bug as the v0.8.1 symbol position fix: gopls returns positions pointing to the `func` keyword, not the identifier name. Unexported symbols like `appendHint` returned "no identifier found" when checking references. Fixed by resolving the actual identifier column from the source line. Found by GPT-5.5 agent evaluation.

- **Token savings wiring.** `AppendTokenMeta` was implemented but not wired into `list_symbols` or `get_symbol_source` handlers (Agent D's merge didn't land). Manually wired.

- **Flaky `TestSubscribeHealth_Stop`.** Timing race on CI: health poller could fire one message before the stop channel was read. Fixed by comparing message count before/after stop instead of asserting absolute zero.

- **`get_change_impact` per-symbol test callers.** Response now includes `affected_symbols` with per-symbol `test_callers` and `non_test_callers` lists. Agents can see which tests cover each specific method, not just a flat list for the file. Backward compatible: existing top-level fields remain.

- **`destroy_session` no longer returns an error on missing sessions.** Returns success with `status: "already_destroyed"` instead of `isError: true`. Agents calling `destroy_session` after `preview_edit` (which auto-cleans up) no longer see a confusing error.

- **`preview_edit` net_delta no longer counts hints.** `DiffDiagnostics` now filters out severity 3 (info) and 4 (hint) before computing the delta. Previously, hints like "interface{} can be replaced by any" counted toward net_delta, making preview_edit report confusing deltas unrelated to the actual edit. Found by GPT-5.5 agent evaluation.

- **`destroy_session` error message improved.** Now explains that `preview_edit` creates and destroys sessions automatically, so a separate `destroy_session` call is not needed. Addresses confusion from agent evaluations where models called destroy_session after preview_edit and got errors.

- **`position_pattern` now works without line/column.** `find_references` and `inspect_symbol` handlers called `extractPosition` (requires line/column) instead of `ExtractPositionWithPattern` (supports position_pattern fallback). Also changed line/column fields to `*int` pointers so the JSON Schema generator marks them as truly optional. Previously, agents using position_pattern got "line: missing required argument." Found in all three GPT-5.5 agent evaluations.

- **`find_references` and `inspect_symbol` schema fix (superseded).** `line` and `column` were required in the JSON schema even when `position_pattern` was provided as an alternative. Made them optional so agents can use `position_pattern` alone without validation errors.

- **`get_change_impact` discoverability.** Promoted to IMPORTANT in MCP Instructions with "replaces manual loops over find_references." Agent evaluations showed agents manually looping over exports instead of calling it.
- **`find_callers` type confusion.** Description now clarifies it works on functions/methods only; for types, use `find_references`. Both agent evaluations showed confusion when call hierarchy returned nothing for types.
- **`format_document` scope clarity.** Description now clarifies single-file scope and suggests shell (e.g. `gofmt -l`) for discovering which files need formatting.

## [0.9.0] - 2026-05-10

### Changed

- **Breaking: 7 tools renamed for intent-based naming.** get_info_on_location -> inspect_symbol, get_document_symbols -> list_symbols, get_workspace_symbols -> find_symbol, get_code_actions -> suggest_fixes, get_references -> find_references, call_hierarchy -> find_callers, simulate_edit_atomic -> preview_edit. Function signatures unchanged; only the MCP tool name strings are affected.

### Added

- **Symbol-level editing tools (4 new tools).** `replace_symbol_body` replaces a function/method/type body by name without needing line/column coordinates. `insert_after_symbol` and `insert_before_symbol` add code adjacent to a named symbol. `safe_delete_symbol` removes a symbol only after confirming zero references via `find_references`. All four resolve symbols internally via `list_symbols`. Skills updated: `lsp-edit-symbol` now uses `replace_symbol_body` as the primary edit path, `lsp-dead-code` offers optional cleanup via `safe_delete_symbol`, and `lsp-refactor`/`lsp-edit-export` support `replace_symbol_body` as an alternative to positional edits. Tool count: 56 to 60.

- **Provider-specific rules files during `init`.** `agent-lsp init` now writes a skill awareness rules file alongside the MCP config, giving every AI provider immediate context about the 22 skills and when to use them. Claude Code gets a managed CLAUDE.md section (between sentinel comments, safe to run repeatedly). Cursor gets `.cursor/rules/agent-lsp.mdc`. Cline gets `.clinerules`. Windsurf gets `~/.windsurfrules`. Gemini CLI gets `GEMINI.md`. All use managed sections to preserve existing user content. Content generated from embedded SKILL.md files at runtime, staying in sync with shipped skills automatically.

- **Fix: skill description parsing.** `parseSkillMD` now skips indented keys in SKILL.md frontmatter, preventing nested `description` fields (from `tool_permissions` phase definitions) from overwriting the top-level skill description. Four skills (lsp-refactor, lsp-rename, lsp-safe-edit, lsp-verify) had wrong descriptions in `prompts/list` responses.

- **Server instructions on initialize.** The MCP `initialize` response now includes an `Instructions` field with a condensed skill overview: tool count, key workflows (blast radius before edit, simulate before apply, verify after change), and pointer to `prompts/get` for full skill workflows. Every MCP client receives this automatically on connect, providing provider-agnostic skill awareness without any configuration files.

- **Proactive server notifications.** Four server-initiated MCP notification channels push state changes to agents without requiring a tool call: (1) diagnostic changes with 2-second debouncing to coalesce rapid `publishDiagnostics` updates during indexing, (2) workspace ready (one-shot notification when all `$/progress` indexing tokens complete, 5-minute timeout), (3) process health (crash/recovery notifications on language server state transitions), (4) stale references (3-second debounce, signals when watched files change on disk). Architecture: `internal/notify/` package with Hub coordinator and per-channel subscribers, `internal/lsp/client_notify.go` with LSPClient hooks (`SubscribeToFileChanges`, `IsAlive`, `IsWorkspaceLoaded`), `cmd/agent-lsp/notifications.go` with MCP session wiring (`mcpNotifySender`, `wireNotificationsToClient`). Notifications are best-effort; send errors are silently dropped. Wired automatically on `start_lsp`.

- **Passive mode (`connect` parameter on `start_lsp`).** Connect to an already-running language server via TCP instead of spawning a new process. Pass `connect: "localhost:9999"` to reuse the IDE's warm index with zero duplicate memory or indexing. Supported by gopls (`gopls -listen=:9999`), clangd, and other servers with TCP listen mode. On shutdown, agent-lsp closes the TCP connection without killing the server process.

- **`group_by=symbol` parameter on `get_diagnostics`.** Diagnostics can now be grouped by their owning symbol instead of returned as a flat list per file. Each diagnostic is assigned to the innermost containing symbol via range containment. Helps agents understand "this function is broken" vs "this file has problems." Usage: `get_diagnostics(file_path: "...", group_by: "symbol")`.

- **Intent-based tool descriptions and titles.** All 7 renamed tools now have descriptions focused on agent intent rather than LSP protocol details. Titles updated to match (e.g., "Inspect Symbol" instead of "Get Hover Info", "Preview Edit" instead of "Simulate Edit (Atomic)").

- **Cross-referencing in tool descriptions.** Tools now suggest related tools: `apply_edit` recommends `replace_symbol_body` for full function replacements and `preview_edit` before applying. `find_references` recommends `safe_delete_symbol` for zero-reference symbols and `get_change_impact` for blast-radius analysis. `suggest_fixes` points to `/lsp-fix-all` skill. `rename_symbol` recommends `find_references` before renaming exports.

- **"No verification needed" assertions.** `preview_edit` description now states: "If net_delta is 0, the edit is safe to apply without further verification." Reduces unnecessary follow-up tool calls after clean previews.

### Fixed

- **Go test path format.** `run_tests` with bare paths like `internal/notify` were interpreted as stdlib packages by `go test`. Now auto-prefixes `./` and appends `/...` for Go paths that don't start with `.` or `/`.

- **Nil sender crash in notification channels.** workspace.go, health.go, and diagnostics.go called `hub.sender.SendLog()` directly, bypassing Hub.Send()'s nil-sender and closed-state guards. This would panic during the window between `start_lsp` and MCP session initialization when sender is nil. Fixed to route through `hub.Send()`. Found by `/lsp-inspect`.

- **Dead code removal (`AdaptFileChangeEvents`).** Exported but never called; the conversion was done inline in notifications.go. Removed along with its test. Found by `/lsp-dead-code`.

- **`CleanupStaleDaemons` wiring.** Exported and tested but never called from production code. Wired into `startOrConnectDaemon` before `FindRunningDaemon` so stale daemon state directories are purged before lookup. Found by `/lsp-dead-code`.

### Refactored

- **`interface{}` to `any` across codebase.** Go 1.18+ alias applied via `gofmt -r`. 85 files, ~930 replacements. No behavioral change.

## [0.8.1] - 2026-05-09

### Added

- **Next-step hints in tool responses.** Every tool response now includes a contextual `hint` field suggesting the logical next tool call. For example, `find_references` returns "use get_change_impact to see the full blast radius"; `preview_edit` returns "call get_diagnostics to check for remaining issues." Helps agents chain tools correctly without skills and helps less capable models navigate the 60-tool surface.

- **`detect_changes` range parameter.** The `committed` scope now accepts a `range` parameter for arbitrary git ranges: `"v0.7.0..HEAD"`, `"abc123..def456"`, or a single ref like `"main"` (expands to `main~1..main`). Ignored for unstaged/staged scopes. Previously only compared `HEAD~1..HEAD`.

- **Fix: symbol position resolution in `get_change_impact`.** `collectExportedSymbols` used `DocumentSymbol.SelectionRange.Start` directly for reference lookups, but gopls returns positions pointing to the `func` keyword for functions and methods, not the identifier name. This caused `GetReferencesRaw` to produce "no identifier found" for every function and method. Fixed by searching the source line for the actual symbol name and using that column. Before: 130 warnings per scan. After: zero warnings, full reference data for all symbols.

- **Selective indexing (Layer 2).** Auto-detects the package boundary for the agent's current file and generates scoped language server config (pyrightconfig.json, tsconfig.json) limited to that package and its direct local dependencies. Activates automatically when the workspace has 500+ source files for Python or TypeScript and no manual scope was specified. On `open_document`, if the file is in a different package, the config is regenerated automatically. Pyright and tsserver watch their config files and reload without a server restart. Combined with the persistent cache (Layer 3), previously-visited packages serve cached results while the current package gets full LSP precision.

- **`detect_changes` MCP tool.** Single-call "what did I break?" workflow. Runs `git diff --name-only` (scopes: unstaged, staged, committed), filters to recognized language files, feeds them to `get_change_impact`, and enriches each symbol with risk classification: "high" (callers across multiple packages), "medium" (same-package callers only), "low" (zero non-test callers).

- **`agent-lsp update` subcommand.** Self-update to the latest GitHub Release. Fetches the release API, compares versions, downloads the correct binary for the current OS/arch, and atomically replaces the running binary. Flags: `--check` (compare without downloading), `--force` (update even if already current).

- **`/lsp-architecture` skill (22nd skill).** Project-level architecture overview in one call. Composes `list_symbols`, `find_symbol`, and `get_change_impact` to produce: language distribution, package map (capped at 30), entry points, hotspots (top 10 files by reference count), and dependency flow. SKILL.md only, no Go code.

- **Cache artifact export/import.** `export_cache` compresses the SQLite reference cache with `VACUUM INTO` + gzip. `import_cache` decompresses and validates with `PRAGMA integrity_check`. On `start_lsp`, if the local cache is empty and `.agent-lsp/cache.db.gz` exists in the workspace root, it auto-imports the artifact. Enables team-shared cache: commit the artifact, teammates skip the cold start.

- **`agent-lsp uninstall` subcommand.** Clean removal of MCP server entries (from `.mcp.json`, `.cursor/mcp.json`, etc.), skill installations (`~/.claude/skills/lsp-*`), CLAUDE.md managed sections (between sentinel comments), and cache directories. Supports `--dry-run`. Preserves other MCP server configurations.

- **Persistent reference cache (knowledge graph Layer 3).** `get_change_impact` results are now cached in a per-workspace SQLite database (`~/.agent-lsp/cache/<hash>/refs.db`). First call queries the language server and stores results keyed by file content hash. Subsequent calls for the same symbols return instantly from cache. File watcher automatically invalidates entries when source files change on disk. Cache is opportunistic: agent-lsp works without it, and missing or corrupted databases fall back to direct LSP queries transparently. Pure Go SQLite via `modernc.org/sqlite`, no CGo.

- **Fix: progressMu deadlock in WaitForWorkspaceReadyTimeout.** The function locked `progressMu` at entry but did not unlock on the timeout or normal exit paths. When `get_change_impact` opened multiple files, gopls emitted `$/progress` notifications that required `progressMu` in `readLoop`. The held lock deadlocked the entire read pipeline: no LSP responses could be dispatched, blocking all pending requests indefinitely. Root cause of every multi-file `get_change_impact` hang. Fixed with `defer progressMu.Unlock()`.

- **Panic recovery in daemon broker connection handler.** `handleBrokerConnection` goroutines now have `defer recover()`, matching the other two goroutines in `RunBroker`. Previously a panic from a malformed message would kill the entire daemon broker process.

- **Context propagation in daemon broker.** Forwarded requests now use the broker's lifecycle context instead of `context.Background()`. Requests are cancelled when the daemon shuts down instead of running indefinitely.

- **Per-symbol timeout in `get_change_impact`.** Each reference query in the parallel batch is capped at 15 seconds. Previously, a single slow symbol (e.g., a widely-referenced type in a large file) could block the entire operation for minutes. Timed-out symbols are skipped with a warning instead of stalling the batch.

- **Write mutex separation fixes stdin pipe deadlock.** `writeRaw` now uses a dedicated `writeMu` instead of the shared `c.mu`. When gopls's stdin pipe buffer fills under heavy concurrent writes (e.g., 100+ parallel reference queries), the `Write()` call blocks. Previously this held `c.mu`, deadlocking all subsequent operations including reads and state checks. The separated mutex allows reads and state transitions to proceed while a write is blocked on pipe backpressure.

- **Diagnostic logging for all tool calls and process lifecycle.** Every tool call now logs its latency via the central `addToolWithPhaseCheck` wrapper. Calls exceeding 5 seconds are logged at WARNING level (e.g., "tool get_change_impact: 8.2s (slow)"); all others log at DEBUG. Process lifecycle events are also logged: "LSP server started: gopls (PID 12345)" on spawn, "LSP server gopls (PID 12345) exited cleanly after 45s" on exit, with uptime tracking. These diagnostics make performance bottlenecks and process leaks visible in the audit trail without requiring manual investigation.

- **Test coverage expansion.** 174+ new tests across 9 packages covering process lifecycle, broker framing, scope config, warmup state, prompt parsing, normalization edge cases, session types, config inference, resource parsing, skill classification, symbol extraction, and more. Coverage improved: internal/tools 31% to 39.6%, internal/lsp 29.8% to 35.7%, cmd/agent-lsp 17.3% to 22.2%, internal/resources 17.5% to 35%, internal/session to 63.6%, internal/config to 69.2%.

- **`get_change_impact` concurrency bump.** Worker pool increased from 8 to 16 parallel reference queries. Reduces wall time on large files (100+ exports) by keeping the gopls request queue saturated.

- **Process lifecycle cleanup.** Fixed orphaned language server processes accumulating across sessions. Three changes:
  1. `Shutdown()` now waits up to 3 seconds for the language server to exit, then force-kills it. Previously it sent `shutdown`/`exit` via stdin and hoped the process would die.
  2. `StartForLanguage` shuts down the previous client before starting a new one, preventing leaks when switching workspaces.
  3. `resolver.Shutdown()` is now called on every exit path (stdin EOF, context cancelled), not just on signals and panics. This was the primary cause of process accumulation: normal session ends never cleaned up child processes.

- **Skills as MCP prompts.** All 22 skills are now discoverable via `prompts/list` and retrievable via `prompts/get`, making them available to any MCP client (Cursor, Windsurf, etc.), not just Claude Code. The listing returns only short descriptions to minimize context cost; full workflow instructions load on demand when a specific prompt is requested. Skill SKILL.md files are embedded into the binary at build time for portable, self-contained distribution. Skills continue to work as AgentSkills slash commands in parallel.

- **`/lsp-inspect` skill (21st skill).** Full code quality audit for a file or package. Combines LSP batch analysis (`get_change_impact`) with LLM-driven heuristic checks. Check taxonomy: `dead_symbol`, `test_coverage`, `silent_failure`, `error_wrapping`, `coverage_gap`, `doc_drift`, `panic_not_recovered`, `context_propagation`. Runs inline (no background agent, no permission gates). Language-agnostic: works with any configured language server. Replaces the external `agentskills-code-inspector` with a first-party skill that uses the already-warm LSP session directly.

- **Skill capability check in `get_server_capabilities`.** The response now includes a `skills` array classifying all 22 skills as `supported`, `partial`, or `unsupported` based on the current language server's capabilities. Each entry lists missing required and optional capabilities. Agents can check once at startup which skills will work instead of attempting skills that fail.

- **Workspace scoping via `scope` parameter on `start_lsp`.** Generates a temporary language-server config (`pyrightconfig.json` for Python, `tsconfig.json` for TypeScript) that restricts analysis to specified subdirectories. Enables agent-lsp to work on large monorepos without full-workspace indexing. Accepts a string or array of paths relative to `root_dir`. Config is automatically removed on server shutdown, with backup/restore of any pre-existing config file. No-op for languages with native module boundaries (Go, Rust).

- **Multi-signal warmup gate for `find_references`.** Servers that don't emit `$/progress` tokens (pyright, jedi-language-server) previously caused reference queries to time out because the workspace wasn't confirmed ready. New three-signal readiness detection:
  1. `$/progress` tokens (existing path, gopls/rust-analyzer/jdtls)
  2. Diagnostic arrival: waits up to 30s for first `publishDiagnostics` notification
  3. Hover canary: issues a hover request to confirm file analysis is complete

  First reference query on a cold workspace gets a 5-minute timeout (vs 2 minutes for subsequent queries). On success, workspace is marked warm and future queries use normal timeouts. On timeout, returns a guidance message recommending the `scope` parameter or longer warmup. The warmup gate is only active for daemon-connected clients; direct-mode clients (gopls, rust-analyzer) use the existing fast path with zero overhead.

- **Persistent LSP daemon mode for Python and TypeScript.** Language servers that need sustained background indexing (pyright, tsserver) now run as persistent daemon brokers that survive between agent sessions. Architecture:
  - `start_lsp` with `language_id="python"` or `"typescript"` automatically spawns a daemon broker subprocess
  - The broker owns the language server, listens on a Unix socket, and proxies JSON-RPC
  - Agent-lsp connects to the daemon via socket (no subprocess spawn on subsequent sessions)
  - Daemon auto-exits after 30 minutes of inactivity
  - `find_references` on daemon clients returns clear "indexing in progress" status if workspace isn't ready
  - New CLI commands: `agent-lsp daemon-status`, `agent-lsp daemon-stop [--all]`
  - Go, Rust, C, and other languages with fast-indexing servers bypass daemon mode entirely (zero overhead)

  Validated on FastAPI (1,119 Python files, 80K stars): daemon indexes in ~10 seconds, `find_references` on the `FastAPI` class returns 1,214 references across 556 files instantly. Previously timed out at 5 minutes on every attempt.

### Fixed

- **Windows build fix.** Extracted `Setsid` syscall attribute into platform-specific files (`procattr_unix.go`, `procattr_windows.go`). The daemon broker used `syscall.SysProcAttr{Setsid: true}` which is Unix-only and prevented compilation on Windows. The Windows variant uses `CREATE_NEW_PROCESS_GROUP` instead.
- **Daemon broker panic recovery.** Added `defer recover()` to warmup and socket accept goroutines in the daemon broker. Previously, a panic in either goroutine would crash the broker silently with no error reporting.
- **Daemon ready flag write failure now logged.** `WriteDaemonInfo` in the warmup goroutine previously discarded errors with `_ =`. If writing the ready flag fails, the daemon would appear permanently stuck in "indexing" state. Now logs a warning.
- **Content-Length parse error handling in broker.** Malformed `Content-Length` headers from socket clients now return an error instead of silently producing `contentLength=0`.
- **Error wrapping in `StopDaemon`.** `os.FindProcess` errors now include the PID and operation context.

- **Concurrency fixes in daemon broker and warmup state.** Fixed 4 data races found by internal concurrency audit:
  - DaemonInfo struct writes synchronized with mutex between warmup goroutine and main event loop
  - `lastDisconn` read protected by `connMu` lock (was racing with write path)
  - `firstRefDone` moved from package-level to per-warmupState field (prevents cross-client state leakage in multi-server mode)
  - `socketConn` nil-write in daemon Shutdown protected by `c.mu`

### Testing

- **17 new unit tests for daemon, warmup, and scope packages.** Covers `NeedsDaemon`, `DaemonDir`, `WriteDaemonInfo`/`RefreshDaemonInfo` round-trip, `CleanupStaleDaemons`, `GenerateScopeConfig` (Python, TypeScript, Go no-op), backup/restore of existing configs, `warmupState` lifecycle (`FirstRefTimeout`, `MarkReady`, `NotifyDiagnostic`).

### Performance

- **get_change_impact: 100x faster on large files.** Rewrote the batch reference query system:
  - Parallel worker pool (8 concurrent goroutines) replaces sequential loop
  - `GetReferencesRaw` skips per-file `WaitForFileIndexed` for batch callers
  - Single warmup query absorbs cold-start cost; result reused (not discarded)
  - `workspaceLoaded` atomic flag: once all `$/progress` tokens complete, `WaitForFileIndexed` becomes a no-op for all subsequent calls
  - Struct fields excluded from export collection (avoids 50%+ unnecessary queries on Go codebases)
  - Context cancellation check in workers for early exit
  - Transitive references parallelized (was sequential)
  - Test function deduplication in output

  Before: `get_change_impact` on a 2,295-line file (80+ exports) hung for 20+ minutes.
  After: completes in under 30 seconds on a cold workspace, under 5 seconds on a warm one.

  This unblocked the `/inspect` skill on external repositories.

## [0.5.3] - 2026-05-04

### Fixed

- Nullable array schemas (`"type": ["null","array"]`) collapsed to `"type": "array"` for Gemini 2.5 Flash compatibility. Fixes #2.

## [0.5.2] - 2026-05-03

### Added

- PyPI distribution: `pip install agent-lsp`. Platform-specific wheels published automatically on release.
- Download stats script (`scripts/download-stats.sh`).

### Changed

- PyPI publish job added to release workflow (automated on every tag).

## [0.5.1] - 2026-05-03

### Added

- **Token savings experiment** (`experiments/token-savings/`): reproducible Go script that measures input token cost of LSP vs grep/read approaches across Go, Python, and TypeScript codebases. 13 tasks covering 7 agent skills. Auto-discovers target symbols. Results: 5-34x savings across 5 codebases (agent-lsp, Hono, FastAPI, Next.js, HashiCorp Consul).
- **`SendRequest` public method** on `LSPClient`: exposes the low-level JSON-RPC request path for batch/measurement scenarios where the workspace is already indexed.
- **PyPI distribution** (`pypi/`): platform-specific wheels containing the Go binary. `pip install agent-lsp` works on macOS, Linux, and Windows without a Go toolchain.
- **Download stats script** (`scripts/download-stats.sh`): queries npm, PyPI, GitHub Releases, and Docker Hub. Generates SVG badge.

### Changed

- File-level comments added to 21 core source files across `cmd/agent-lsp/`, `internal/lsp/`, `internal/tools/`, and `internal/session/` explaining architecture, design decisions, and data flow.
- Roadmap updated: "mcp-eval" section replaced with "mcp-assert: shipped sister project" reflecting v0.8.0 reality.
- README: added token savings section with headline numbers (5-34x, link to full experiment).
- Distribution docs: added PyPI channel and marketing/discovery tracking section.

## [0.5.0] - 2026-04-25

### Added

- **Skill phase enforcement**: runtime state machine that enforces tool call ordering during skill workflows. Three new tools: `activate_skill`, `deactivate_skill`, `get_skill_phase`. Four skills have phase configs: `lsp-rename` (3 phases), `lsp-refactor` (5 phases), `lsp-safe-edit` (4 phases), `lsp-verify` (5 phases). Two enforcement modes: `warn` (log and allow) and `block` (return error with structured recovery guidance). Phases advance automatically based on tool call patterns. Global forbidden lists prevent tools that don't belong in a skill's workflow. All agent-lsp tool handlers wrapped via generic `addToolWithPhaseCheck` function. Phase events logged to JSONL audit trail. 17 unit tests covering matching, phase advancement, warn/block modes, and full workflow integration for lsp-rename and lsp-refactor. New `internal/phase/` package. See [docs/phase-enforcement.md](docs/phase-enforcement.md).
- **Phase enforcement documentation**: standalone deep-dive doc (`docs/phase-enforcement.md`), tool reference entries in `docs/tools.md`, architecture and features docs updated, mcp-assert assertions for all 3 new tools.
- **Prerequisites section** in installation guide: states what you need (a language server, an MCP client) before installing.
- **"What to try first" guidance** in quickstart: concrete examples of what to ask your AI agent after setup.

### Changed

- Tool count updated from 50 to 53 across all documentation (README, index, architecture, tools, features, quickstart).
- `ROADMAP.md` and `docs/changelog.md` replaced with symlinks to their `docs/` and root counterparts respectively, eliminating duplicate files that had drifted out of sync.
- `required-capabilities` and `optional-capabilities` metadata marked as shipped in roadmap (were listed as planned but shipped in v0.4.0).
- Awesome MCP Servers marked as shipped in FEATURES.md distribution table.
- `ready_timeout_seconds` parameter added to `start_lsp` documentation in tools.md (was documented in FEATURES.md but missing from the primary tool reference).
- Architecture doc updated: nine `doc.go` packages (adds `phase`), five tool registration files (adds `tools_phase.go`), `addToolWithPhaseCheck` wrapper documented.
- Cross-reference to phase enforcement added to speculative-execution.md "See also" section.

## [0.4.0] - 2026-04-24

### Added

- **Elixir: 16 verified capabilities** (up from 13). Fixed definition, find_callers, and apply_edit. Symbols (list_symbols) now correctly marked as failing due to ElixirLS needing more compile time than the 20s init wait provides.
- **AgentSkills spec conformity** — all 20 skills now include `license` and `compatibility` frontmatter fields per the [AgentSkills specification](https://agentskills.io/specification). Skills work with any conforming agent: Claude Code, Cursor, GitHub Copilot, Gemini CLI, OpenAI Codex, JetBrains Junie, and 30+ others.
- **Provider-agnostic skill installer** — `install.sh --dest DIR` installs skills to any agent's skill directory, not just Claude Code. Updates CLAUDE.md, AGENTS.md (Codex), and GEMINI.md instruction files when present.
- **Architecture documentation** — concurrency model section (goroutine architecture, four channel patterns, crash recovery), speculative execution sequence diagram, error handling section (three-layer propagation), Key Terms glossary, HTTP transport mode details, audit trail section, config file example.
- **LSP Conformance page** added to docs site navigation.
- **Gleam: 17 verified capabilities** (up from 6 skipping). Added `gleam build --target javascript` pre-build step, fixed module import path (`person` not `fixture/person`), enriched fixture with Result type and pattern matching. New dedicated `typeDefLine`/`signatureHelpLine` test config fields for independent positioning.
- **Documentation site** — agent-lsp.com live on GitHub Pages with Cloudflare DNS.
- **mcp-assert** — sister project ([github.com/blackwell-systems/mcp-assert](https://github.com/blackwell-systems/mcp-assert)). Deterministic correctness testing for MCP servers. 103 assertions across 7 servers in 3 languages: agent-lsp (60, 100% tool coverage), filesystem (14, 92%), memory (5), SQLite (6), mcp-go SDK (18, 100%). 14 assertion types, 100 unit tests, 8 CLI commands (run/ci/matrix/coverage/generate/snapshot/watch/init), setup output capture for chained workflows, mkdocs site. [GitHub Action](https://github.com/blackwell-systems/mcp-assert-action). Found 2 upstream bugs: [modelcontextprotocol/servers#4029](https://github.com/modelcontextprotocol/servers/issues/4029), [mark3labs/mcp-go#826](https://github.com/mark3labs/mcp-go/issues/826).
- **Agent evaluation framework** on roadmap — two-layer architecture (deterministic tool correctness + skill workflow trajectory matching), Docker-isolated eval harness, negative evals, capability-gated skills.
- **Capability metadata in skills** — all 20 SKILL.md files now declare `required-capabilities` and `optional-capabilities` in frontmatter metadata. Agents can check `get_server_capabilities` against a skill's requirements before activation. 5 skills have zero required capabilities (work with any LSP); `referencesProvider` is the most common requirement (8 skills); `callHierarchyProvider` and `typeHierarchyProvider` are always optional, never required.
- **Zig coverage maximization** — upgraded zls from 0.13.0 to 0.14.0 in CI; 21 verified capabilities (up from 18). signature_help now passes (call site position in main.zig), apply_edit now passes (trailing whitespace in fixture), symbol_source now passes (likely zls 0.14 improvement). workspace_symbols fails (zls 0.14.0 advertises support but may need specific query format).
- **`user-invocable` on all skills** — all 20 SKILL.md files now declare `user-invocable: true` in frontmatter, making them available as `/lsp-*` slash commands in Claude Code and other AgentSkills-conforming agents.
- **mcp-assert CI job** — 51 deterministic protocol-level assertions covering all 50 agent-lsp tools against real gopls on every push and PR. 100% tool coverage through the MCP transport layer. Outputs JUnit XML artifact and shields.io badge JSON.

### Fixed

- **change-impact-test CI flake** — replaced fixed `time.Sleep` with `ready_timeout_seconds` and warmup probe that polls `find_references` until gopls returns cross-file results. Skips on persistent timeout instead of failing.
- **Docker release pipeline** — inlined base layer into all Dockerfiles to eliminate build race; split language/combo/full images into parallel matrix job (10 runners) to avoid 60m GoReleaser timeout; fixed hardcoded `linux-amd64` Go download URL for ARM64 builds.
- **Completions test** — handles both raw array and CompletionList object response shapes (fixes Gleam and other servers that return the full CompletionList).
- **apply_edit test** — detects whole-file replacement formatters (Gleam always returns a full-file TextEdit) by comparing edit content against current file content.
- **Parameter naming consistency** — `get_symbol_source` renamed `character` to `column` in JSON Schema to match all other position-taking tools. Implementation accepts both for backward compatibility. `format_range` schema descriptions corrected from "0-indexed" to "1-indexed" (the implementation already validated `>= 1`). Both found by dogfooding mcp-assert.

## [0.3.0] - 2026-04-22

### Added

- **`ready_timeout_seconds` on `start_lsp`** — optional parameter that blocks until all `$/progress` workspace-indexing tokens complete before returning, up to the specified timeout. Replaces fixed post-initialize sleeps for servers like jdtls that index asynchronously after `initialize`. Fires as soon as indexing completes rather than always waiting the full timeout. Also exports `WaitForWorkspaceReadyTimeout` on `LSPClient` for callers needing a configurable timeout beyond the default 60s cap.
- **Error path integration tests** (`test/error_paths_test.go`) — 11 subtests covering deliberately bad input across `go_to_definition`, `get_diagnostics`, `simulate_edit`, `preview_edit`, `find_references`, and `rename_symbol`. Asserts well-formed error responses, never nil results or crashes, without asserting specific message text.
- **Cross-language consistency tests** (`test/consistency_test.go`) — parallel structural shape validation across Go, TypeScript, Python, and Rust for `list_symbols`, `go_to_definition`, `get_diagnostics`, and `inspect_symbol`. Verifies response shape contracts hold across all language servers.
- **Dedicated `multi-lang-java` CI job** — jdtls isolated to its own runner to avoid OOM-induced SIGTERM when sharing memory with other language servers. Runs with `continue-on-error: true`, `-Xmx2G`, and a 15-minute timeout. `multi-lang-core` no longer installs jdtls and drops from 45m to 30m timeout.
- **ARM64 Docker images** — all 11 Docker image tags now publish as multi-arch manifest lists (`linux/amd64` + `linux/arm64`). Native performance on Apple Silicon and AWS Graviton without Rosetta/QEMU emulation.

- **MCP tool annotations** — all 50 tools now declare `ToolAnnotations` with `Title`, `ReadOnlyHint`, `DestructiveHint`, `IdempotentHint`, and `OpenWorldHint`. MCP clients can auto-approve read-only tools (~30 of 50) without human confirmation.
- **JSON Schema parameter descriptions** — 171 `jsonschema` struct tags across all Args structs. Schema description coverage goes from 0% to 100%. Agents see parameter semantics (1-indexed positions, valid values, defaults) in the tool schema itself.
- **Speculative session tests expanded to 8 languages** — `TestSpeculativeSessions` is now table-driven and covers Go (gopls), TypeScript (typescript-language-server), Python (pyright), Rust (rust-analyzer), C++ (clangd), C# (csharp-ls), Dart (dart analysis server), and Java (jdtls). Each language runs as a parallel subtest with its own MCP process. The `error_detection` subtest verifies `net_delta > 0` for a per-language type-breaking edit. Java uses a 300s extended timeout to accommodate jdtls JVM startup. CI `speculative-test` job updated to install all required LSP servers; timeout bumped to 20m.
- **`--help` flag** — `agent-lsp --help` (or `-h` or `help`) prints usage summary with all modes and subcommands.
- **`docs/skills.md`** — user-facing skill reference organized by workflow category with concrete use cases and composition examples.
- **`glama.json`** — Glama MCP registry profile for server discovery and quality scoring.

### Changed

- **Graceful startup with no language servers** — auto-detect mode now starts the MCP server with all 50 tools registered even when no language servers are found on PATH. Previously exited with an error. Enables introspection, container health checks, and deferred server configuration via `start_lsp`.

### Fixed

- **jdtls `JAVA_HOME` on Linux CI** — `javaHome` in the Java `langConfig` was hardcoded to a macOS Homebrew path, causing jdtls to exit immediately on Linux runners. Now reads `JAVA_HOME` from the environment, resolving correctly on both platforms.
- **TypeScript speculative test `discard_path` net_delta** — inserting a comment at line 1 of `example.ts` shifted 3 pre-existing error positions, producing a false-positive `net_delta=3`. Switched `safeEditFile` to `consumer.ts` (no pre-existing errors) and added a `get_diagnostics` flush after opening the file to ensure baseline is captured against steady-state diagnostics.
- **Python speculative chain test** — chain test hardcoded `// chain step N` but `//` is floor division in Python. Now uses `lang.safeEditText` (language-appropriate comment syntax).
- **BSD awk in `install.sh`** — fixed CLAUDE.md managed block update failing silently on macOS due to embedded newlines in awk `-v` variable. Uses temp file with `getline` instead.
- **Docker `USER nonroot` inheritance** — `Dockerfile.lang`, `Dockerfile.combo`, and `Dockerfile.full` now switch to `USER root` before `apt-get install` and back to `nonroot` after. Previously failed with exit code 100 because the base image's `USER nonroot` was inherited.
- **`Dockerfile.release` for GoReleaser** — GoReleaser Docker builds now use a dedicated Dockerfile that copies the pre-built binary instead of compiling from source. Fixes build context issues where source files were unavailable.
- **Docker build ordering** — release workflow pre-builds and pushes the base image before GoReleaser starts, fixing parallel build race where language images couldn't find the base in the registry.
- **Leaked agent constraint in `/lsp-generate`** — removed SAW agent brief instruction that leaked into the published SKILL.md.
- **Install script archive extraction** — `install.sh` and `install.ps1` now handle GoReleaser's nested archive directory structure instead of assuming a flat layout.
- **`agent-lsp init` Claude Code global path** — option 2 now writes to `~/.claude/.mcp.json` (Claude Code) instead of `claude_desktop_config.json` (Claude Desktop). Menu label updated to match.
- **`go install` path** — documented command was missing `/cmd/agent-lsp` suffix, causing "not a main package" error.
- **jdtls CI exit status 15** — `sudo mkdir` created the `-data` directory owned by root, preventing jdtls from writing workspace metadata. Removed hardcoded `-data` from wrapper scripts; tests now control workspace directory via `serverArgs`.

## [0.2.1] - 2026-04-20

### Fixed

- **Exit code on no-args** — `agent-lsp` invoked with no arguments and no language servers on PATH now exits 0 with usage help instead of exit 1. Fixes Winget validation failure.

## [0.2.0] - 2026-04-19

### Added

- **Windows install support** — `install.ps1` PowerShell script (no admin required; installs to `%LOCALAPPDATA%\agent-lsp` and adds to user PATH), Scoop bucket manifest (`bucket/agent-lsp.json`; `scoop bucket add blackwell-systems https://github.com/blackwell-systems/agent-lsp`), and Winget manifests (`winget/manifests/`; `winget install BlackwellSystems.agent-lsp`).
- **HTTP+SSE transport** — agent-lsp can now serve MCP over HTTP using `--http [--port N]`. Enables persistent remote service deployment: Docker containers on remote hosts, shared CI servers, and multi-client setups without cold-start cost. Auth via `AGENT_LSP_TOKEN` environment variable enforces Bearer token authentication using `crypto/subtle.ConstantTimeCompare`.
- **`internal/httpauth` package** — `BearerTokenMiddleware(token, next http.Handler)` wraps any HTTP handler with constant-time Bearer token validation. Returns RFC 7235-compliant 401 with `WWW-Authenticate: Bearer` header and `{"error":"unauthorized"}` JSON body. No-op passthrough when token is empty.
- **`/health` endpoint** — unauthenticated `GET /health` returns `{"status":"ok"}` (200). Bypasses Bearer token auth so container orchestrators and Docker healthchecks can probe liveness without credentials. `docker-compose.yml` wires `HEALTHCHECK` for the `agent-lsp-http` service.
- **Docker security hardening** — images now run as uid/gid 65532 (`nonroot`); `EXPOSE 8080` added; `HOME` set to `/tmp` (writable by nonroot); `docker-compose.yml` adds `agent-lsp-http` service for HTTP mode with `AGENT_LSP_TOKEN` wiring.
- **`docker-compose.yml` HTTP service** — `agent-lsp-http` service exposes port `${AGENT_LSP_HTTP_PORT:-8080}:8080` with token read from `AGENT_LSP_TOKEN` env var (not CLI arg).
- **`/lsp-explore` skill** — composes hover, go_to_implementation, find_callers, and find_references into a single "tell me about this symbol" workflow for navigating unfamiliar code.
- **`/lsp-fix-all` skill** — apply available quick-fix code actions for all current diagnostics in a file, one at a time with re-collection after each fix. Enforces a sequential fix loop to handle line number shifts after each apply_edit.
- **`/lsp-refactor` skill** — end-to-end safe refactor: blast-radius analysis → speculative preview → apply → build verify → targeted tests. Inlines tool sequences from lsp-impact, lsp-safe-edit, lsp-verify, and lsp-test-correlation.
- **`/lsp-extract-function` skill** — extract a selected code block into a named function. Primary path uses the language server's extract-function code action; manual fallback identifies captured variables and constructs the function signature.
- **`/lsp-generate` skill** — trigger language server code generation (interface stubs, test skeletons, missing method stubs, mock types) via `suggest_fixes` + `execute_command`. Documents per-language generator patterns for Go, TypeScript, Python, and Rust.
- **`/lsp-understand` skill** — deep-dive exploration of unfamiliar code by symbol name or file path. Synthesizes hover, implementations, call hierarchy (2-level depth limit), references, and source into a structured Code Map. Broader than `/lsp-explore`: operates on files as a unit and surfaces inter-symbol relationships.
- **`agent-lsp doctor` subcommand** — probes each configured language server, reports version and supported capabilities, exits 1 if any server fails. Useful for CI health checks and debugging setup issues.
- **LineScope for `position_pattern`** — `line_scope_start` / `line_scope_end` args restrict pattern matching to a line range, eliminating false matches when the same token appears multiple times in a file.
- **`rename_symbol` glob exclusions** — new optional `exclude_globs` parameter (array of glob strings) excludes matching files from the returned WorkspaceEdit. Useful for generated code (`**/*_gen.go`), vendored files (`vendor/**`), and test fixtures (`testdata/**`).
- **MIT LICENSE file** — added explicit license; copyright Blackwell Systems and Dayna Blackwell.

### Changed

- **Auth token reads from env var** — `AGENT_LSP_TOKEN` environment variable takes precedence over `--token` CLI flag, keeping credentials out of the process list. `--token` still accepted for local dev but env var always wins; using `--token` without the env var prints a warning to stderr.
- **HTTP server timeouts** — `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 60s`, and `IdleTimeout: 120s` added to prevent Slowloris-style resource exhaustion and stalled response writers.
- **`--listen-addr` IP validation** — rejects hostnames and invalid values; only valid IP addresses accepted (`net.ParseIP`).
- **`--no-auth` loopback enforcement** — `--no-auth` is rejected when `--listen-addr` is a non-loopback address.
- **`entrypoint.sh` security** — replaced `eval` with a POSIX `case` whitelist; `awk` uses `-v name=` variable binding; `apt-get` arm validates package name; all expansions quoted.
- **Port range validation** — `--port` rejects values outside 1–65535.
- **Accurate HTTP bind log** — reports actual bound address from `ln.Addr().String()`.
- **`install.sh` CLAUDE.md sync** — maintains a managed skills table in `~/.claude/CLAUDE.md` between sentinel comments; auto-discovers skills from SKILL.md frontmatter.
- Docker builds now trigger on release tags only; removed `:edge` tag.
- Moved `Dockerfile`, `Dockerfile.full`, `Dockerfile.lang`, and `docker-compose.yml` into `docker/` directory.
- Removed `:base` as a user-facing tag (still used internally between CI jobs).
- Surfaced quick install snippet at top of README after value proposition.

## [0.1.2] - 2026-04-10

### Added (2026-04-10) — Public pkg/ API

Exposed a stable importable Go API so other programs can use agent-lsp's LSP client and speculative execution engine without running the MCP server:

- **`pkg/types`** — all 29 LSP data types, 5 constants, and 2 constructor vars re-exported as type aliases from `internal/types`
- **`pkg/lsp`** — `LSPClient`, `ServerManager`, `ClientResolver` interface, and all constructors; `ServerEntry` re-exported from `internal/config`
- **`pkg/session`** — `SessionManager`, `SessionExecutor` interface, all speculative execution types and constants

All `pkg/` types are aliases (`type X = internal.X`) — values are interchangeable with internal types without conversion. `pkg.go.dev` now indexes and renders the full public API surface.

Added package-level doc comments to all 9 previously undocumented internal packages (`internal/lsp`, `internal/session`, `internal/types`, `internal/logging`, `internal/uri`, `internal/extensions`, `internal/tools`, `internal/resources`, `cmd/agent-lsp`).

Added **Library Usage** section to `README.md` with import examples for `pkg/lsp`, `pkg/session`, and `pkg/types`. Updated `docs/architecture.md` to document the new `pkg/` layer.

### Added (2026-04-10) — `--version` flag

`agent-lsp --version` prints the version and exits. Defaults to `dev` for local builds; GoReleaser injects the release tag at build time via `-ldflags="-X main.Version=x.y.z"`. The MCP server's `Implementation.Version` field now reads from the same variable.

### Fixed (2026-04-10) — Docker image build failures

- **`go`/gopls** — `apt golang-go` installs Go 1.19, too old for gopls. Switched to fetching the latest Go tarball from `go.dev/VERSION` at build time.
- **`ruby`/solargraph** — added `build-essential` for native C extension compilation (`prism`).
- **`csharp`** — `csharp-ls` NuGet package lacks `DotnetToolSettings.xml`; moved to `LSP_SERVERS` runtime-only with a clear error message.
- **`dart`** — not in standard Debian bookworm repos; moved to `LSP_SERVERS` runtime-only.
- **combo images** — inline Dockerfile assumed `npm` and `go` were in the base image; fixed to install nodejs/npm and Go from `go.dev` when needed.
- Per-language tag table in `DOCKER.md` corrected: removed 12 tags that were never published; split into published tags and `LSP_SERVERS`-only languages with install notes.

### Added (2026-04-10) — Docker image distribution (ghcr.io)

Tiered Docker image distribution published to `ghcr.io/blackwell-systems/agent-lsp`:

- **`:latest` (base)** — binary only, no language servers, ~50MB. Supports `LSP_SERVERS=gopls,pyright,...` env var for runtime install with `/var/cache/lsp-servers` volume caching.
- **Per-language tags** (`:go`, `:typescript`, `:python`, `:ruby`, `:cpp`, `:php`) — extend base, one language server pre-installed.
- **Combo tags** (`:web`, `:backend`, `:fullstack`) — curated multi-language images for common stacks.
- **`:full`** — all package-manager-installable language servers (~2–3GB).
- `Dockerfile`, `Dockerfile.lang`, `Dockerfile.full` — multi-stage builds on `debian:bookworm-slim`.
- `docker/entrypoint.sh` — POSIX sh runtime installer; `docker/lsp-servers.yaml` — registry of all 18 supported servers.
- `.github/workflows/docker.yml` — separate workflow (not release.yml) building all tiers in parallel, pushing to ghcr.io on `main` push (`:edge`) and version tags.
- `docker-compose.yml` + `.env.example` for local development.
- `DOCKER.md` rewritten with per-language one-liners, `LSP_SERVERS` usage, volume caching, MCP client config.
- `README.md` gains a `## Docker` section with the four most common one-liners.

### Added (2026-04-10) — Architecture diagram

- `docs/architecture.drawio` — draw.io diagram of the full system: MCP client → server.go (toolDeps) → 4 tool registration files → internal/tools handlers → internal/lsp client layer → gopls subprocess. Includes internal/session, leaf packages, and layer rule annotation.

### Fixed (2026-04-10) — Inspector audit-7: 11 bugs and quality improvements

#### Security
- **Path traversal in `HandleGetDiagnostics`** — `HandleGetDiagnostics` accepted a caller-supplied `file_path` and passed it directly to `CreateFileURI` without validation. Every other handler validates with `ValidateFilePath` first. A caller could supply `../../etc/passwd` and the handler would read it via `ReopenDocument`. Fixed by adding a `ValidateFilePath(filePath, client.RootDir())` call before `CreateFileURI`; the sanitized path is used throughout the handler.

#### Fixed
- **Context dropped in `StartForLanguage` shutdown** — `StartForLanguage(ctx, ...)` called `e.client.Shutdown(context.Background())` when replacing an existing client, discarding the caller's cancellation and deadline. Fixed to pass `ctx`.
- **`LanguageIDFromPath` missing C/C++/Java extensions** — The exported `LanguageIDFromPath` function (used by `HandleGetChangeImpact`) lacked `.c`, `.cpp`, `.cc`, `.cxx`, and `.java` entries. Those file types were mapped to `"plaintext"`, producing incorrect language IDs in impact reports. Added the missing cases.
- **`GetReferences` errors silently discarded in `HandleGetChangeImpact`** — Per-symbol reference lookup errors were swallowed (`locs, _ := ...`), causing affected symbols to appear with zero callers instead of surfacing a diagnostic. Errors now appear as a `warnings` field in the tool response.
- **`writeRaw` error missing context** — Returned the raw `stdin.Write` error with no indication of which operation triggered it. Wrapped as `fmt.Errorf("writeRaw: %w", err)`.
- **`sendNotification` marshal error missing method name** — Both `json.Marshal` error paths in `sendNotification` returned without the method name, making debug traces opaque. Wrapped as `fmt.Errorf("sendNotification %s: marshal ...: %w", method, err)`.
- **`init()` side effect in `internal/logging`** — `init()` read `LOG_LEVEL` from the environment and mutated package-level state, coupling test setup to import order. Extracted to `SetLevelFromEnv()`, called explicitly from `main()`; `init()` is now a no-op.
- **`DirtyErr` accessible on non-dirty sessions** — `SimulationSession.DirtyErr` was a public field readable in any state, giving `nil` with no signal on non-dirty sessions. Added `DirtyError() error` accessor that returns `DirtyErr` only when `Status == StatusDirty`; updated the one internal call site in `session/manager.go`.

#### Test coverage
- **`WaitForFileIndexed` timeout, cancellation, and stability-window-reset paths untested** — Added three tests matching the `WaitForDiagnostics` pattern: `TestWaitForFileIndexed_Timeout`, `TestWaitForFileIndexed_ContextCancelled`, and `TestWaitForFileIndexed_StabilityWindowReset`.
- **`parseBuildErrors` missing tests for TypeScript, Rust, and Python** — Added `TestParseBuildErrors_TypeScript`, `TestParseBuildErrors_Rust`, and `TestParseBuildErrors_Python` with synthetic compiler output strings.

### Fixed (2026-04-10) — Inspector-surfaced bugs and quality fixes

#### Errors fixed
- **Panic recovery in long-lived goroutines** — `readLoop` and `startWatcher` goroutines had no `recover()`. A panic in `dispatch()` or `fsnotify` would terminate the entire process; `runWithRecovery` in main cannot catch goroutine panics. Both goroutines now have a deferred recovery that logs the panic and stack trace at error level and returns, keeping the server alive.
- **`Run()` decomposed from 832 to 379 lines** — The monolithic `Run()` function in `cmd/agent-lsp/server.go` held ~50 tool registrations, inline arg struct definitions, resource handlers, diagnostic subscription, and transport startup as a single untestable unit. Extracted into four themed registration files: `tools_navigation.go` (10 tools), `tools_analysis.go` (13 tools), `tools_workspace.go` (19 tools), `tools_session.go` (8 tools), each taking a `toolDeps` struct.
- **`normalize_test.go` was asserting broken behavior** — `TestNormalizeDocumentSymbols_SymbolInformationVariant` used `_ = root.Children` to silence a failing assertion, masking the bug and preventing regression detection. Updated to assert `len(root.Children) == 1` and `root.Children[0].Name == "MyField"`.

#### Warnings fixed
- **Duplicate extension→languageID mapping** — `langIDFromPath` in `change_impact.go` and `inferLanguageID` in `manager.go` both mapped file extensions to LSP language IDs with different coverage (`.cs`, `.hs`, `.rb` were silently labeled `"plaintext"` in impact reports). Replaced with a single exported `lsp.LanguageIDFromPath` function covering all extensions; `langIDFromPath` removed.
- **Duplicate URI-to-path conversion** — `tools.URIToFilePath` duplicated the logic in `uri.URIToPath` with different error behavior. `URIToFilePath` now delegates to `uri.URIToPath`, preserving the `(string, error)` signature.
- **Bare error returns in session manager** — `Discard` and `Destroy` returned bare `err` from `GetSession`, losing call-site context. Wrapped as `fmt.Errorf("discard: %w", err)` and `fmt.Errorf("destroy: %w", err)`.
- **`waitForWorkspaceReady` could block indefinitely** — The cond var refactor (audit-6 L2) introduced a bug: the 60s deadline was only checked after `cond.Wait()` returned, but if gopls dropped a progress token without emitting the corresponding `end` notification, `Wait()` never returned. Added a timer goroutine that broadcasts at the deadline, guaranteeing the wait unblocks.
- **gopls inherited shell `GOWORK` env var** — `exec.Command` inherits the full parent environment; a `GOWORK` value pointing at a different workspace caused gopls to fail package metadata loading for the target repo. The subprocess environment now has `GOWORK` stripped via `removeEnv`, letting gopls discover the correct go.work naturally from `root_dir`.

### Added (2026-04-10) — Three new MCP tools for code-impact analysis

#### `get_change_impact`
Answers "what breaks if I change this file?" without running tests. Given a list of changed files, it enumerates all exported symbols in those files via `list_symbols`, resolves every reference via `find_references`, and partitions the results into test callers (with enclosing test function names extracted) and non-test callers. Supports optional one-level transitive following to surface second-order impact. Useful before any refactor to understand blast radius and which tests will need updating.

#### `get_cross_repo_references`
First-class cross-repo caller analysis. Given a symbol (file + position) and a list of consumer repo roots, adds each consumer as a workspace folder and calls `find_references` across all of them. Results are partitioned by repo root prefix so callers in each consumer are reported separately. Designed for library authors who need to know which downstream consumers reference a symbol before changing its signature.

#### `simulate_chain` — refactor preview framing
`simulate_chain` is now documented and surfaced as a "refactor preview" tool: apply a rename/signature change speculatively, walk the chain of dependent edits, and read `cumulative_delta` + `safe_to_apply_through_step` before writing a single byte to disk. Added `docs/refactor-preview.md` with four worked examples (safe rename preview, change impact preview, multi-file refactor with checkpoint, key response fields reference). README updated with refactor-preview framing in the tools table.

### Fixed (2026-04-09) — Audit-6 batch: 12 bugs and quality fixes

#### Critical
- **C1 — `AddWorkspaceFolder` watcher regression** — The audit-5 H2 fix (passing `path` instead of `c.rootDir` to `startWatcher`) made `AddWorkspaceFolder` call `startWatcher(path)`, which internally stopped the existing watcher goroutine before starting a new one watching only the new path. After adding a second workspace folder, file changes under the original root were no longer delivered to the LSP server; the index went stale silently. Fixed by adding a `watcher *fsnotify.Watcher` field to `LSPClient` and a new `addWatcherRoot` method that calls `watcher.Add(path)` on the live watcher goroutine rather than restarting it. `AddWorkspaceFolder` now calls `addWatcherRoot` instead of `startWatcher`.
- **C2 — Exit-monitor goroutine did not clear `initialized` on crash** — After an unplanned LSP subprocess exit (OOM, segfault), `rejectPending` was called to unblock pending requests, but `c.initialized` was left `true`. All subsequent tool calls passed `CheckInitialized` and received opaque RPC errors instead of the clear "call start_lsp first" message. Fixed by adding `c.mu.Lock(); c.initialized = false; c.mu.Unlock()` in the exit-monitor goroutine immediately after `rejectPending`.

#### High
- **H1 — `NormalizeDocumentSymbols` name map was last-write-wins on duplicate names** — `nameMap[info.Name]` overwrote earlier entries for symbols sharing a name (e.g., multiple `String()` or `Error()` methods across types). Children were attached to the wrong parent node. Fixed by keying the name map with `nameKey(name, kind)` using `\x00` as separator; a separate `nameByBare` map handles `ContainerName` lookups.
- **H2 — `SerializedExecutor` global semaphore serialized all sessions** — A single `chan struct{}` blocked all concurrent session operations regardless of which sessions were involved. Two independent speculative sessions were forced sequential. Fixed by replacing the global channel with `map[string]chan struct{}` — one buffered channel per session ID — created on first access under a guard mutex. The per-session channel preserves the original cancellation semantics via `select`.
- **H3 — Column offsets were byte offsets, not UTF-16 code unit offsets** — `ResolvePositionPattern` and `textMatchApply` computed the `character` field using raw byte subtraction. LSP spec §3.4 requires UTF-16 code unit offsets; gopls silently returns empty results when given positions past the line end. Fixed by adding a `utf16Offset(line string, byteOffset int) int` helper in `position_pattern.go` (walks UTF-8 runes, counts surrogate pairs for U+10000+) and using it in both locations.

#### Medium
- **M1 — `MarkServerInitialized()` called before MCP session established** — A premature call at `server.go:1016` set `serverInitialized = true` before any MCP client had connected, making the initialization flag misleading and fragile to ordering changes. Removed; the canonical call inside `InitializedHandler` (which fires on MCP client connection) is the only remaining call site.
- **M2 — `DiffDiagnostics` was O(n×m)** — Nested loop compared every current diagnostic against every baseline diagnostic. For files with hundreds of diagnostics, this compounded across URIs per evaluation. Fixed with a fingerprint-keyed counter map (`map[string]int`) for O(n+m) complexity; fingerprint uses Range, Message, and Severity (matching `DiagnosticsEqual` semantics); counts handle duplicate diagnostics correctly.
- **M3 — `textMatchApply` built file URIs via string concatenation** — `"file://" + filePath` does not percent-encode spaces or special characters; `CreateFileURI` (using `url.URL`) was already the established pattern elsewhere. Fixed by replacing the concat with a `CreateFileURI(filePath)` call.

#### Low
- **L1 — `NormalizeDocumentSymbols` Pass 3 comment was misleading** — Comment incorrectly implied the value-copy logic handled multi-level SymbolInformation hierarchies. Updated to accurately describe deferred pointer dereferencing, why it is correct for the 1-level depth that LSP SymbolInformation always produces, and the spec constraint.
- **L2 — `waitForWorkspaceReady` polled at 100ms intervals** — Unnecessary latency of up to 100ms after workspace indexing completed. Replaced busy-poll with `sync.Cond`; `handleProgress` now broadcasts when `progressTokens` becomes empty; `waitForWorkspaceReady` blocks on `Wait()` with a context-deadline fallback.
- **L3 — `AddWorkspaceFolder`/`RemoveWorkspaceFolder` dropped context** — Methods had no `ctx context.Context` parameter; notification sends could not be cancelled. Added `ctx` as first parameter to both methods and updated the call sites in `workspace_folders.go`.
- **L4 — `json.Marshal` errors discarded in three workspace folder handlers** — `HandleAddWorkspaceFolder`, `HandleRemoveWorkspaceFolder`, and `HandleListWorkspaceFolders` used `data, _ := json.Marshal(...)`. Fixed by capturing the error and returning `types.ErrorResult` on failure, consistent with all other handlers.

### Fixed (2026-04-09) — Audit-5 batch: 16 bugs and quality fixes

#### Critical
- **C1 — `Restart` did not clear per-session state** — `openDocs`, `diags`, `legendTypes`, and `legendModifiers` were not reset on restart; after reconnecting to a fresh LSP server, stale open-document records caused the server to receive `didChange` instead of `didOpen` for already-open files, and stale diagnostics were served from the previous session. Fixed by adding explicit zeroing of all four maps/slices inside `Restart`, guarded by their respective mutexes, before calling `Initialize`.
- **C2 — `watcherStop` data race in `startWatcher`/`stopWatcher`** — the `watcherStop` channel was read and written without synchronization, causing a race detectable by `go test -race`. Fixed by adding `watcherMu sync.Mutex` to `LSPClient`; `startWatcher` and `stopWatcher` now hold the mutex around all reads and writes of `watcherStop`.

#### High
- **H1 — `applyDocumentChanges` swallowed filesystem errors** — create, rename, and delete operations used `_ = os.WriteFile(...)` / `_ = os.Rename(...)` / `_ = os.Remove(...)`; errors were silently discarded. Fixed by capturing and returning errors from all three cases.
- **H2 — `AddWorkspaceFolder` started watcher on root dir instead of new path** — called `c.startWatcher(c.rootDir)` instead of `c.startWatcher(path)`; adding a second workspace folder would restart the watcher on the original root. Fixed by passing `path`.
- **H3 — `HandleSimulateEditAtomic` discarded `Discard` errors** — cleanup calls used `_ = mgr.Discard(...)`; if the session cleanup failed the error was lost. Fixed by capturing both errors and returning a combined message when both the evaluate-path and discard-path error.
- **H4 — `LogMessage` used `context.Background()` and discarded marshal error** — the function created a detached context rather than using the caller's context, and `json.Marshal` errors were silently dropped, resulting in JSON null being sent to the client. Fixed by adding explicit error handling with a fallback encoded-error string; added comment explaining the intentional `context.Background()` for the notification send path.

#### Medium
- **M1 — `applyDocumentChanges` returned nil on array-unmarshal failure** — when the changes JSON couldn't be unmarshalled into `[]types.TextEdit`, the function returned nil instead of an error, silently applying no edits. Fixed by returning the unmarshal error.
- **M2 — `StartAll` rollback used `context.Background()` for shutdown** — rollback loops in `StartAll` called `c.Shutdown(context.Background())`, ignoring the caller's context and discarding shutdown errors. Fixed by passing `ctx` and logging shutdown errors at debug level.
- **M3 — `uriToPath` duplicated across `internal/lsp` and `internal/session`** — two near-identical implementations with a manual-sync comment. Extracted to new `internal/uri` package as `uri.URIToPath`; both packages now import and call the shared version.
- **M4 — `HandleRestartLspServer` only restarted the default client in multi-server mode** — the handler restarted `c.lspManager.GetClient(c.language)` but did not address other configured servers. Fixed by adding a note to the success message indicating that only the default server for the current language is restarted in multi-server configurations.
- **M5 — `WaitForDiagnostics` quiet-window checked on 50 ms ticks only** — when a `notify` event arrived just after a tick, the quiet-window exit condition wasn't evaluated until the next tick (up to 50 ms delay). Fixed by adding the same quiet-window check to the `case <-notify:` arm so it's evaluated immediately on each notification.

#### Low
- **L1 — Recovered panic exited 0** — `runWithRecovery`'s recover block logged the panic but did not set the named return error, so the process exited 0 instead of 1. Fixed by setting `runErr = fmt.Errorf("panic: %v", r)`.
- **L2 — `ValidateFilePath` did not resolve symlinks** — the prefix check used the lexical path, so a symlink pointing outside the workspace root would pass validation. Fixed by calling `filepath.EvalSymlinks` on both the file path and the root dir before the prefix check; non-existent paths fall back to lexical path.
- **L3 — `IsDocumentOpen` exported but only used in tests** — renamed to `isDocumentOpen`; `client_test.go` is in `package lsp` (same package) so the unexported name remains accessible.
- **L4 — `toolArgsToMap` discarded `Unmarshal` error** — used `_ = json.Unmarshal(...)`; failures were silent. Fixed by capturing the error, logging at debug level, and returning an empty map.
- **L5 — Line-splice algorithm duplicated with manual-sync comment** — `applyRangeEdit` in `internal/session/manager.go` and the inline loop in `applyEditsToFile` in `internal/lsp/client.go` implemented the same line-splice logic independently. Extracted to `uri.ApplyRangeEdit` in the new `internal/uri` package; both sites now delegate to the shared implementation.

### Fixed + Added (2026-04-09) — Speculative session test hardening
- **`discard_path` bug fix** — test was calling `preview_edit` with a `session_id`, but `preview_edit` is a self-contained tool (creates its own session internally, requires `workspace_root` + `language`); the call was silently returning `IsError: true` and logging it as "may be expected"; fixed to call `simulate_edit` which is the correct tool for applying edits to an existing session
- **`evaluate_session` response assertions** — existing subtests were only logging the response; now parse the JSON and assert `net_delta == 0` for comment-only edits (with `confidence != "low"` guard for CI timing); `simulate_edit` response now asserts `edit_applied == true`
- **`simulate_chain` response assertions** — parse `ChainResult` JSON; assert `cumulative_delta == 0` for two-comment chain; assert `safe_to_apply_through_step == 2`
- **`commit_path` improved** — now applies a comment edit via `simulate_edit` before committing, making the test more meaningful than committing a clean session
- **`preview_edit_standalone` subtest** — proper standalone usage of `preview_edit` with `workspace_root` + `language` parameters; asserts response is an `EvaluationResult` with `net_delta == 0` for a comment edit
- **`error_detection` subtest** — validates the core speculative session value proposition: apply `return 42` in a `func ... string` body (type error), evaluate, assert `net_delta > 0` and `errors_introduced` is non-empty; CI-safe: accepts skip when `confidence == "low"` or `timeout == true` (gopls indexing window)

### Added (2026-04-09) — Full tool coverage (47/47 at time; total now 50)
- **`testSetLogLevel`** — integration test for `set_log_level`; sets level to `"debug"`, verifies confirmation message contains "debug", resets to `"info"`; no LSP required, runs for all 30 languages
- **`testExecuteCommand`** — integration test for `execute_command`; queries `get_server_capabilities` for `executeCommandProvider.commands`, skips if server advertises none, calls `commands[0]` with a file URI argument; server-level errors treated as skip (dispatch path still exercised); Go-level transport errors are failures; tool coverage 32 → 34 (multi-language harness); 47/47 tools covered across all test suites (3 tools added later: `get_change_impact`, `get_cross_repo_references`, promoted `simulate_chain`; see Unreleased entry above)

### Added (2026-04-09) — Test coverage + CI cleanup
- **`testGoToSymbol` and `testRestartLspServer` test functions** — two previously untested tools now covered in `TestMultiLanguage`; `testGoToSymbol` calls `go_to_symbol` with `lang.workspaceSymbol` and verifies at least one result is returned; `testRestartLspServer` restarts the server, waits 5 s for re-indexing, reopens the document, and confirms hover still works; both wired into `tier2Results` with skip guards; tool coverage 28 → 32 (accounting for `go_to_symbol`, `restart_lsp_server`, and two tools added in prior waves)
- **`test/lang_configs_test.go`** — `buildLanguageConfigs()` extracted from `test/multi_lang_test.go` into its own file (840 lines); `multi_lang_test.go` reduced from 2340 → 1573 lines; only additional import needed was `path/filepath`; no behavior changes
- **`unit-and-smoke` GHA job** — renamed from `test` for clarity, distinguishing it from the `multi-lang-*` integration jobs

### Fixed (2026-04-09) — Nix CI
- **`multi-lang-nix` install** — `nil` build script queries `nix` at compile time to generate builtin completions; previous `cargo install --git ... nil` failed with `"Is nix accessible?: NotFound"`; fix: install Nix via `DeterminateSystems/nix-installer-action@v16` before installing nil, then use `nix profile install github:oxalica/nil` to pull from binary cache instead of compiling

### Added (2026-04-09) — Language expansion (30 languages)
- **MongoDB integration test** — `mongodb-language-server` (`npm i -g @mongodb-js/mongodb-language-server`); fixture at `test/fixtures/mongodb/` with `query.mongodb` (14-line playground file, `find` at line 9 col 12, `aggregate` at line 11 col 12) and `schema.mongodb` (15-line `createCollection` with `$jsonSchema` validator for `name`/`age` fields); dedicated `multi-lang-mongodb` CI job with `mongo:7` service container on port 27017, `mongosh` health check, and `TestMultiLanguage/^MongoDB$` test; `supportsFormatting: false`; language count updated 29 → 30

### Added (2026-04-09) — Language expansion (29 languages)
- **Clojure integration test** — `clojure-lsp`; fixture at `test/fixtures/clojure/` with `deps.edn` (empty map for project recognition) and `src/fixture/core.clj` (7-line file with `greet` function at line 3 col 7, call site at line 7 col 13); dedicated `multi-lang-clojure` CI job installing clojure-lsp native binary
- **Nix integration test** — `nil` (Nix language server); fixture at `test/fixtures/nix/flake.nix` (9-line flake with `helper` binding at line 5 col 5, call site at line 7 col 21); `supportsFormatting: false`; dedicated `multi-lang-nix` CI job installing nil binary
- **Dart integration test** — `dart language-server`; fixture at `test/fixtures/dart/` with `pubspec.yaml` (SDK `>=3.0.0 <4.0.0`), `lib/fixture.dart` (`Greeter` class at line 1 col 7, `greet` method at line 2 col 10), `lib/caller.dart` (imports and calls `Greeter`; `Greeter` at col 13, `greet` at col 11); dedicated `multi-lang-dart` CI job installing Dart SDK via apt; language count updated 26 → 29; see also MongoDB entry below

### Added (2026-04-09) — Language expansion (26 languages)
- **SQL integration test** — `sqls` (`go install github.com/sqls-server/sqls@latest`); fixture at `test/fixtures/sql/` with `schema.sql` (CREATE TABLE person + post), `query.sql` (two SELECT statements, 18 lines, calibrated hover/completion/reference positions), `.sqls.yml` (postgresql DSN); `serverArgs: []string{"--config", filepath.Join(fixtureBase, "sql", ".sqls.yml")}` — config path is resolved at test time, not hardcoded; dedicated `multi-lang-sql` CI job with `postgres:16` service container, `pg_isready` health check, `psql` schema load step, and `PGPASSWORD` env for the load command; supportsFormatting/rename/inlayHints all false (sqls does not implement them); language count updated 25 → 26
- **JSON-RPC string ID support** — `jsonrpcMsg.ID` changed from `*int` to `json.RawMessage`; dispatch now handles both integer and string IDs per JSON-RPC 2.0 spec; `sendResponse` echoes the raw ID bytes verbatim; `sendRequest` marshals integer IDs into RawMessage; fixes compatibility with servers that use string IDs (e.g. `prisma-language-server`)

### Added (2026-04-09) — Language expansion (25 languages)
- **Gleam integration test** — `gleam lsp` (built-in to the Gleam binary, `serverArgs: ["lsp"]`); fixture at `test/fixtures/gleam/` with `gleam.toml`, `src/person.gleam`, `src/greeter.gleam`; full Tier 2 coverage including rename, highlights, code actions, and inlay hints; dedicated `multi-lang-gleam` CI job (downloads binary from GitHub releases)
- **Elixir integration test** — `elixir-ls` (`language_server.sh` symlinked as `elixir-ls`); fixture at `test/fixtures/elixir/` with `mix.exs`, `lib/person.ex`, `lib/greeter.ex`; rename and inlay hints skipped (`renameSymbolLine: 0`, `inlayHintEndLine: 0` — ElixirLS does not implement those); dedicated `multi-lang-elixir` CI job using `erlef/setup-beam@v1` (Elixir 1.16 / OTP 26), `continue-on-error: true` due to ElixirLS cold-start variability
- **Prisma integration test** — `prisma-language-server --stdio` (`npm i -g @prisma/language-server`); fixture at `test/fixtures/prisma/schema.prisma` — two-model schema (`Person`, `Post`) with a relation; call site and definition both in the same file (schema is a single-file language); inlay hints skipped; dedicated `multi-lang-prisma` CI job
- **Language count updated 22 → 25** — README badge, prose, Tier 2 table, Language IDs list, comparison table, `docs/language-support.md`, `docs/tools.md`

### Added (2026-04-09) — Skills expansion (continued)
- **`format_document` step folded into `/lsp-safe-edit` and `/lsp-verify`** — `format_document` → `apply_edit` is now an optional final step in both skills; in `/lsp-safe-edit` it fires after diagnostics are clean (Step 8, before the report); in `/lsp-verify` it fires after all three layers pass as a pre-commit cleanup; skipped when there are unresolved errors or the user did not request formatting; `format_document` added to `allowed-tools` in both skills
- **`/lsp-format-code` skill** — format a file or selection via the language server's formatter (`gofmt` via gopls, `prettier` via tsserver, `rustfmt` via rust-analyzer, etc.); `format_document` for full file, `format_range` for selection; both return `TextEdit[]` applied via `apply_edit`; optional `get_server_capabilities` pre-check for `documentFormattingProvider`; post-apply `get_diagnostics` guard; multi-file protocol runs format calls in parallel then applies per-file sequentially; language notes table covers Go/TypeScript/Rust/Python/C

### Added (2026-04-09) — Skills expansion (continued)
- **`/lsp-test-correlation` skill** — find and run only the tests covering an edited source file; `get_tests_for_file` maps source → test files, `find_symbol` enumerates specific test functions within those files, `run_tests` executes the scoped set; fallback to workspace symbol search when `get_tests_for_file` returns no mapping; multi-file workflow deduplicates test files across all changed sources; `[correlated / unrelated]` classification guides where to investigate failures first
- **`/lsp-verify` `get_tests_for_file` pre-step** — when `changed_files` is known, `get_tests_for_file` runs before the three parallel layers to build a source→test map; Layer 3 failure report now tags each failing test as correlated (covers changed code) or unrelated (collateral failure) to narrow debugging scope

### Added (2026-04-09) — Skills expansion
- **`/lsp-cross-repo` skill** — multi-root workspace analysis for library + consumer workflows; orchestrates `add_workspace_folder` → `list_workspace_folders` (verify indexing) → `find_symbol` → `find_references` / `find_callers` / `go_to_implementation` across both repos; solves the "agent doesn't know to add a second workspace folder" discoverability gap; output separates library-internal from consumer references
- **`/lsp-local-symbols` skill** — file-scoped symbol analysis without workspace-wide search; composes `list_symbols` (symbol tree for the file) → `get_document_highlights` (all usages within the file, classified as read/write/text) → `inspect_symbol` (type signature); faster than `find_references` for local-scope questions; explicit "when NOT to use" guidance prevents misuse as a cross-file search
- **`/lsp-rename` `prepare_rename` safety gate** — `prepare_rename` now runs as Step 2 (after symbol location, before reference enumeration); validates that the language server can rename at the given position before doing any further work; catches built-ins, keywords, and imported external package names that cannot be renamed across module boundaries; fail-fast with actionable error message
- **`/lsp-safe-edit` `preview_edit` pre-flight** — `preview_edit` now runs before any disk write (Step 3); returns `net_delta` (errors introduced minus resolved) without touching disk; `net_delta > 0` pauses and asks before proceeding; multi-file: run per-file independently and sum deltas
- **`/lsp-safe-edit` code actions on introduced errors** — if post-edit diagnostics introduce new errors, `suggest_fixes` is called at each error location and available quick fixes are surfaced to the user with `y/n/select`; accepted actions applied via `apply_edit`, then re-diff
- **`/lsp-safe-edit` multi-file workflow** — explicit protocol for edits spanning multiple files: open all, collect BEFORE for all, simulate each file independently, apply file-by-file (stop on first failure), merge AFTER diagnostics, check code actions on any file with new errors

### Changed (2026-04-09)
- **`lsp-verify` skill corrected and hardened** — three fixes from dogfooding: (1) `get_diagnostics` parameter corrected from `workspace_dir` (invalid) to `file_path` — call once per changed file; (2) large test output warning added — `run_tests` on large repos can return 300k+ chars and overflow context; recovery options: grep saved output file for `FAIL` lines, or scope tests to the changed package directly; (3) all three layers now explicitly instructed to run in parallel since they are fully independent.
- **`lsp-dead-code` skill hardened against false positives** — four improvements from dogfooding a full-repo dead-code audit: (1) mandatory Step 0 indexing warm-up — verify a known-active symbol returns ≥1 reference before trusting any results; explicit retry/restart protocol if indexing stalls; (2) `"no identifier found"` recovery note — methods on receivers shift the name column rightward, added grep-for-column technique to recover without blind retrying; (3) zero-reference cross-check — before classifying any handler/constructor/type as dead, grep wiring files (`main.go`, `server.go`, `cmd/`) for the symbol name to catch registration patterns (`server.AddTool(HandleFoo)`) that are invisible to LSP; (4) new caveat #2 documenting why registration-pattern references produce zero LSP hits; Step 3 classification table adds "Zero LSP, found by grep → ACTIVE" as a distinct outcome.

### Fixed (2026-04-09)
- **`list_symbols` coordinates are now 1-based** — `range` and `selectionRange` positions in the output were previously 0-based (raw LSP passthrough), inconsistent with every other coordinate-accepting tool (`find_references`, `inspect_symbol`, etc.) which all use 1-based input. The handler now shifts all line/character values by +1 before returning, including in nested `children` symbols. The `lsp-dead-code` skill instruction to "add 1 to selectionRange before passing to find_references" is now unnecessary — coordinates flow directly between tools. **Breaking:** any hardcoded line offsets captured from previous `list_symbols` output will be off by one.

### Added (2026-04-09)
- **`lsp-implement` skill** — find all concrete implementations of an interface or abstract type; composes `go_to_implementation` + `type_hierarchy`; includes capability pre-check, risk assessment table (0 implementors → likely unused, >10 → breaking API change), and language notes for Go/TypeScript/Java/Rust/C#
- **`lsp-verify` code action fix section** — when Layer 1 diagnostics return errors, call `suggest_fixes` at the error location to surface available quick fixes, apply with `apply_edit`, then re-verify; `suggest_fixes` and `apply_edit` added to skill `allowed-tools`
- **`list_symbols` `format: "outline"` parameter** — when `format: "outline"`, returns the symbol tree as compact markdown (`name [Kind] :line`, indented for children) instead of JSON; reduces token volume ~5x for large files; useful for quick structural surveys before targeted navigation. Default behavior (JSON) unchanged.
- **`start_lsp` `language_id` parameter** — optional field selects a specific configured server in multi-server mode (e.g. `language_id: "go"` targets gopls, `language_id: "typescript"` targets tsserver); routes via new `ServerManager.StartForLanguage` which matches by `language_id` field or extension set; without `language_id`, behavior is unchanged (StartAll). Fixes an agent usability gap where the wrong language server could be active in a mixed-language repo with no in-session override. Description updated to recommend `get_server_capabilities` for diagnosing active-server mismatches.
- **`apply_edit` text-match mode** — new `file_path` + `old_text` + `new_text` parameter mode; finds `old_text` in the file (exact byte match first, then whitespace-normalised line match that tolerates indentation differences) and applies the replacement without requiring line/column positions; positional `workspace_edit` mode unchanged
- **`lsp-edit-symbol` skill** — edit a named symbol without knowing its file or position; composes `find_symbol` → `list_symbols` → `apply_edit` to resolve the symbol name to its definition range and apply the edit; decision guide covers signature-only edits, full-body replacements, and ambiguous symbol disambiguation
- **`get_symbol_source` tool** — returns the source code of the innermost symbol (function, method, struct, class, etc.) whose range contains a given cursor position; composes `textDocument/documentSymbol` + file read; `findInnermostSymbol` walks the symbol tree recursively to find the deepest enclosing symbol; accepts `line`+`character` (1-based) or `position_pattern` (@@-syntax); `character` aliased to `column` for consistency with other tools; CI-verified in `testGetSymbolSource` across all 22 languages
- **MCP log notifications** — internal log messages (LSP server start, tool dispatch errors, indexing events) now route as `notifications/message` to the connected MCP client via `mcpSessionSender`; wired through `InitializedHandler` in `ServerOptions` so the live `*ServerSession` is captured per-connection; before session init and on send failure, falls back to stderr; level threshold controlled by `set_log_level`
- **`get_symbol_documentation` tool** — fetch authoritative documentation for a named
  symbol from local toolchain sources (go doc, pydoc, cargo doc) without requiring an
  LSP hover response. Works on transitive dependencies not indexed by the language
  server. Returns `{ symbol, language, source, doc, signature }`. Dispatches to
  per-language toolchain commands with a 10-second timeout; strips ANSI escape codes;
  returns a structured error (not MCP error) when the toolchain fails so callers can
  fall back to LSP hover.
- **`lsp-docs` skill** — three-tier documentation lookup: (1) `inspect_symbol`
  (hover, fast, live); (2) `get_symbol_documentation` (offline, authoritative, works on
  unindexed deps); (3) `go_to_definition` + `get_symbol_source` (source fallback). Use
  when hover text is absent or the symbol is in a transitive dependency.

### Changed (2026-04-09)
- **Skill descriptions updated with trigger conditions** — all four skill `description` fields now include explicit "use when" clauses per the Claude Code skills spec, enabling automatic invocation when relevant. Descriptions trimmed to ≤250 chars (spec cap). Non-spec `compatibility` field moved to markdown body. `argument-hint` added to `lsp-rename` and `lsp-edit-export` for autocomplete UX.
- **Skills migrated to Agent Skills directory format** — each skill is now a self-contained directory (`lsp-rename/SKILL.md`, `lsp-safe-edit/SKILL.md`, `lsp-edit-export/SKILL.md`, `lsp-verify/SKILL.md`) conforming to the [Agent Skills open spec](https://agentskills.io/specification). Flat `.md` files and shared `PATTERNS.md` removed. `patterns.md` duplicated into each skill's `references/` directory (spec requires self-contained skills). Frontmatter updated: `user-invocable` removed (not in spec), `allowed-tools` fixed to space-delimited, `compatibility` field added. `install.sh` updated to symlink skill directories to `~/.claude/skills/` instead of flat files.

### Added (2026-04-08) — LSP Skills wave

- **`go_to_symbol` MCP tool** — navigate to any symbol by dot-notation path (e.g. `"MyClass.method"`, `"pkg.Function"`) without needing a file path or line/column; uses `GetWorkspaceSymbols` to find candidates and resolves to the definition location; supports optional `workspace_root` and `language` filters
- **Position-pattern parameter (`position_pattern`)** — `@@` cursor marker syntax for position-based tools; `ResolvePositionPattern` searches file content for the pattern and returns the 1-indexed line/col of the character immediately after `@@`; `ExtractPositionWithPattern` integrates with existing `extractPosition` fallback; field added to `GetInfoOnLocationArgs`, `GetReferencesArgs`, `GoToDefinitionArgs`, and `RenameSymbolArgs`
- **Dry-run preview mode for `rename_symbol`** — `dry_run: true` returns a preview envelope `{ "workspace_edit": {...}, "preview": { "note": "..." } }` without writing to disk; existing behavior unchanged when `dry_run` is omitted or false
- **Four agent-native skills** — `lsp-safe-edit`, `lsp-edit-export`, `lsp-rename`, `lsp-verify`; compose agent-lsp tools into single-command workflows for safe editing, exported-symbol refactoring, two-phase rename, and full diagnostic+build+test verification
- **`skills/install.sh`** — executable install script for registering skills with MCP clients

### Fixed (2026-04-08)
- **`run_build` and `run_tests` in Go workspaces** — both tools now unconditionally set `GOWORK=off` when running `go build` and `go test`; Go searches upward through parent directories for `go.work` files, and when found, `./...` patterns only match modules listed in the workspace file; setting `GOWORK=off` forces Go to build/test all modules in the directory, matching the tool's intent

### Added (2026-04-08)
- **`run_build`, `run_tests`, and `get_tests_for_file` MCP tools** — three new
  build-tool integration tools that do not require `start_lsp`; language-specific
  dispatch: `go build ./...` / `cargo build` / `tsc --noEmit` / `mypy .` (run_build),
  `go test -json ./...` / `cargo test --message-format=json` / `pytest --tb=json` /
  `npm test` (run_tests); test failure `location` fields are LSP-normalized (file URI
  + zero-based range) — paste directly into `go_to_definition` or `find_references`;
  `get_tests_for_file` returns test files for a source file via static lookup (no test
  execution); shared runner abstraction in `internal/tools/runner.go`; tool count 42 → 45
- **Build tool dispatch expanded to 9 languages** — `run_build` and `run_tests` now dispatch for csharp (`dotnet build`/`dotnet test`), swift (`swift build`/`swift test`), zig (`zig build`/`zig build test`), kotlin (`gradle build --quiet`/`gradle test --quiet`) in addition to the original 5 (go, typescript, javascript, python, rust); `get_tests_for_file` updated with patterns for all new languages
- **`apply_edit` real file-write test** — replaced no-op empty WorkspaceEdit with a full format→apply→re-format cycle; Go, TypeScript, and Rust fixtures each have a blank line with deliberate trailing whitespace that their formatters strip; second `format_document` call returning empty edits proves the write persisted to disk; skip message when fixture already clean (subsequent runs on same checkout)
- **`detect_lsp_servers` extended to 22 languages** — added `knownServers` entries and file extension mappings for C#, Kotlin, Lua, Swift, Zig, CSS/SCSS/Less, HTML, Terraform, Scala; fixed `.kt`/`.kts` extensions which were incorrectly mapped to `java` instead of `kotlin`
- **Zig language support** — `zls` added as 19th CI-verified language; dedicated `multi-lang-zig` CI job; fixture with `person.zig`, `greeter.zig`, `main.zig`, `build.zig`
- **CSS language support** — `vscode-css-language-server` added as 20th CI-verified language; zero new CI install cost (`vscode-langservers-extracted` already present); fixture: `styles.css`
- **HTML language support** — `vscode-html-language-server` added as 21st CI-verified language; zero new CI install cost; fixture: `index.html`
- **Terraform language support** — `terraform-ls` (HashiCorp) added as 22nd CI-verified language; dedicated `multi-lang-terraform` CI job; fixture: `main.tf`, `variables.tf`
- **Lua language support** — `lua-language-server` added as 17th CI-verified language; fixture with `person.lua`, `greeter.lua`, `main.lua` (EmmyDoc annotations for type-aware hover); dedicated `multi-lang-lua` CI job; binary installed from GitHub releases
- **Swift language support** — `sourcekit-lsp` added as 18th CI-verified language; fixture with `Person.swift`, `Greeter.swift`, `main.swift`, `Package.swift`; dedicated `multi-lang-swift` CI job on `macos-latest` (sourcekit-lsp ships with Xcode, zero install cost)
- **Scala language support** — `metals` added as 16th CI-verified language; fixture with `Person.scala`, `Greeter.scala`, `Main.scala`, `build.sbt`; dedicated `multi-lang-scala` CI job with `continue-on-error: true` and 30-minute timeout (metals requires sbt compilation on cold start)
- **Kotlin language support** — `kotlin-language-server` added as 15th CI-verified language; fixture with `Person.kt`, `Greeter.kt`, `main.kt`, `build.gradle.kts`; added to `multi-lang-core` CI job (reuses Java setup); full Tier 1 + Tier 2 coverage
- **C# language support** — `csharp-ls` added as 14th CI-verified language; fixture with `Person.cs`, `Greeter.cs`, `Program.cs`; full Tier 1 + Tier 2 coverage including hover, definition, references, completions, formatting, rename, highlights
- **CI workflow split into 4 parallel jobs** — `test` (unit + binary smoke), `multi-lang-core` (Go/TypeScript/Python/Rust/Java), `multi-lang-extended` (C/C++/JS/PHP/Ruby/YAML/JSON/Dockerfile/CSharp), `speculative-test` (gopls + `TestSpeculativeSessions`); unit tests now correctly run `./internal/... ./cmd/...` instead of `-run TestBinary`; `TestSpeculativeSessions` now in CI
- **Integration test coverage expanded to 26 tools** — multi-language Tier 2 matrix grown from 12 → 26 tools per language: added `testGetDocumentHighlights`, `testGetInlayHints`, `testGetCodeActions`, `testPrepareRename`, `testRenameSymbol`, `testGetServerCapabilities`, `testWorkspaceFolders`, `testGoToTypeDefinition`, `testGoToImplementation`, `testFormatRange`, `testApplyEdit`, `testDetectLspServers`, `testCloseDocument`, `testDidChangeWatchedFiles`; `TestSpeculativeSessions` in `test/speculative_test.go` covers full lifecycle: create, `simulate_edit` (non-atomic), `preview_edit`, `simulate_chain`, evaluate, discard, commit, destroy
- **`rename_symbol` fuzzy position fallback** — when the direct position lookup returns an empty `WorkspaceEdit`, falls back to workspace symbol search by hover name and retries at each candidate position; mirrors the fuzzy fallback already in `go_to_definition` and `find_references`; handles AI position imprecision without correctness regression
- **Multi-root workspace support** — `add_workspace_folder`, `remove_workspace_folder`, `list_workspace_folders` tools; `workspace/didChangeWorkspaceFolders` notifications; enables cross-repo references, definitions, and diagnostics across library + consumer repos in one session; workspace folder list persisted on client and initialized from `start_lsp` root
- **`get_document_highlights`** — file-scoped symbol occurrence search (`textDocument/documentHighlight`); returns ranges with read/write/text kinds; instant, no workspace scan; `DocumentHighlight` and `DocumentHighlightKind` types added to `internal/types`
- **Auto-watch workspace** — `fsnotify` watcher starts automatically after `start_lsp`; forwards file changes to the LSP server via `workspace/didChangeWatchedFiles`; debounced 150ms; skips `.git/`, `node_modules/`, etc.; `did_change_watched_files` tool no longer required for normal editing workflows
- **`get_server_capabilities`** — returns server identity (`name`, `version` from `serverInfo`), full LSP capability map, and classified tool lists (`supported_tools` / `unsupported_tools`) based on what the server advertised at initialization; lets AI pre-filter capability-gated tools before calling them; `GetCapabilities()` and `GetServerInfo()` methods added to `LSPClient`; `serverName`/`serverVersion` now captured from initialize response
- **`get_inlay_hints`** — new MCP tool (`textDocument/inlayHint`); returns inline type annotations and parameter name labels for a range; capability-guarded (returns empty array when server does not support `inlayHintProvider`); `InlayHint`, `InlayHintLabelPart`, `InlayHintKind` types added to `internal/types`
- **`detect_lsp_servers`** — new MCP tool; scans workspace for source languages (file extensions + root markers, scored by prevalence), checks PATH for corresponding LSP server binaries, returns `suggested_config` entries ready to paste into MCP config; deduplicates shared binaries (c+cpp → one clangd entry)
- **`find_symbol` enrichment** — new `detail_level`, `limit`, `offset` params; `detail_level=hover` enriches a paginated window of results with hover info (type signature + docs); `symbols[]` always returns full result set; `enriched[]` + `pagination` returned for the window; mirrors mcp-lsp-bridge's ToC + detail-window pattern
- **`type_hierarchy`** — MCP tool for `textDocument/typeHierarchy`; `direction: supertypes/subtypes/both`; `TypeHierarchyItem` type (LSP 3.17); CI-verified for Java (jdtls) and TypeScript
- **LSP response normalization** — `GetDocumentSymbols`, `GetCompletion`, `GetCodeActions` now return concrete typed Go structs; `NormalizeDocumentSymbols` (two-pass `SymbolInformation[]` → `DocumentSymbol[]` tree reconstruction), `NormalizeCompletion`, `NormalizeCodeActions` in `internal/lsp/normalize.go`

### Added
- Auto-infer workspace root from file path — all per-file `mcp__lsp__*` tools now automatically walk up from the file path to find a workspace root marker (`go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`, `setup.py`, `.git`) and initialize the correct LSP client if none is active; `start_lsp` is no longer required before first use
  - `internal/config.InferWorkspaceRoot(filePath)` — exported helper, walks directory tree upward checking markers in priority order
  - `cmd/agent-lsp/server.go` — all 17 per-file tool handlers wrapped with `clientForFileWithAutoInit`; double-checked locking ensures thread-safe single initialization per workspace root


- Tests for `Destroy` (session removal + not-found error), `ApplyEdit` terminal and dirty guards, and `languageToExtension` (all 10 named cases + default fallback) — previously only the `"go"` case was exercised

### Changed
- `Commit` uses `maps.Copy` instead of a manual loop to build the workspace edit patch

### Fixed
- `logging.Log` data race on `initWarning` eliminated — read and write now hold `mu.Lock()` before accessing the field; previously two concurrent `Log()` calls could both observe the non-empty warning and race to zero it
- `ServerManager.StartAll` now shuts down all previously-initialized clients before returning on failure — previously leaked LSP subprocesses and open pipes when any server in a multi-server config failed to initialize
- `resources.ResourceEntry` type deleted — had zero production callers
- `mcp__lsp__*` tool routing fixed: `settings.json` now passes explicit `go:gopls` args so gopls is always the default client and entry[0]; previously alphabetical ordering made clangd the default, causing all `.go` file queries to be answered by clangd with invalid AST errors
- `Evaluate` no longer permanently breaks a session when context cancellation races the semaphore acquire — `SetStatus(StatusEvaluating)` is now set only after `Acquire` succeeds, so a cancelled acquire leaves the session in `StatusMutated` and allows retry
- `session.Status` reads in `Evaluate` and `Commit` now hold `session.mu` before comparison, eliminating a data race with concurrent `SetStatus` writes detected by the Go race detector
- `HandleSimulateEditAtomic` now calls `mgr.Discard` before returning early on `Evaluate` failure — previously the LSP client retained stale in-memory document content until the next `open_document` call
- `workspace/applyEdit` dispatch now uses `context.WithTimeout(context.Background(), defaultTimeout)` instead of a plain `context.Background()` — prevents indefinite blocking on large workspace edits in the read loop
- `ReopenDocument` untracked-URI fallback now infers language ID from file extension via `languageIDFromURI` instead of hardcoding `"plaintext"` — gopls previously ignored these files silently, returning zero diagnostics
- `deactivate` method and `TestRegistry_Deactivate` deleted from `internal/extensions` — method had no production callers after being unexported in audit-2
- `SerializedExecutor.Acquire` now respects context cancellation — replaced `sync.Mutex` with a buffered-channel semaphore; callers that pass a cancelled or deadline-exceeded context to `ApplyEdit`, `Evaluate`, or `Discard` now receive `ctx.Err()` instead of blocking indefinitely
- `generateResourceList` dead function removed; `resourceTemplates` exported as `ResourceTemplates` and wired into `server.go` via `AddResourceTemplate` — MCP clients can now discover per-file `lsp-diagnostics://`, `lsp-hover://`, and `lsp-completions://` URIs via `resources/list`
- `ExtensionRegistry.Deactivate` unexported to `deactivate` — method had no external callers; was test-only
- `applyRangeEdit` cross-reference comment updated to point to `LSPClient.applyEditsToFile` to prevent independent bug-fix divergence
- `RootDir()` doc comment corrected — previously carried the `Initialize` doc comment verbatim due to copy-paste
- `workspace/configuration` params unmarshal error now logged at debug level instead of silently discarded with `_ =`; fallback empty-array response preserved
- `applyDocumentChanges` discriminator unmarshal failure now logs at debug level and skips the malformed entry instead of falling through to the `TextDocumentEdit` branch
- `init()` in `internal/logging` no longer writes to stderr at import time — invalid `LOG_LEVEL` value is stored and flushed on the first `Log()` call instead


- `ApplyEditArgs.Edit` type changed from `interface{}` to `map[string]interface{}` — Claude Code's MCP schema validator rejected the empty schema produced by `interface{}` and silently dropped all 34 tools silently; `map[string]interface{}` produces a valid `"type": "object"` schema
- `preview_edit` now calls `Discard` before `Destroy` — without Discard, gopls retained the modified document between atomic calls; the next call's baseline captured stale (modified) diagnostics, producing incorrect `net_delta` values
- `start_lsp` in multi-server/auto-detect mode now calls `ServerManager.StartAll` — previously only restarted the first detected server (clangd), leaving gopls and other language servers uninitialized; simulation sessions for Go files now correctly use gopls
- `csResolver` wrapper added to `server.go` so `SessionManager` sees clients set by `start_lsp` at runtime; previously the original resolver held a nil client until `start_lsp` was called, causing "no LSP client available" errors
- `SessionManager.CreateSession` routes by language extension via `ClientForFile` — in multi-server mode `DefaultClient()` returned clangd; routing by `.go`/`.py`/`.ts` extension now picks the correct language server per session
- `languageToExtension` helper added to `internal/session/manager.go` — maps language IDs (`go`, `python`, `typescript`, `javascript`, `rust`, `c`, `cpp`, `java`, `ruby`) to file extensions for client routing

### Added
- **Speculative code sessions** — simulate edits without committing to disk; create sessions with baseline diagnostics, apply edits in-memory, evaluate diagnostic changes (errors introduced/resolved), and commit or discard atomically; implemented via `internal/session` package with SessionManager (lifecycle), SerializedExecutor (LSP access serialization), and diagnostic differ (baseline vs current comparison); 8 new MCP tools: `create_simulation_session`, `simulate_edit`, `evaluate_session`, `simulate_chain`, `commit_session`, `discard_session`, `destroy_session`, `preview_edit`; tool count 26 → 34; enables safe what-if analysis and multi-step edit planning before execution; useful for AI assistants to verify edits won't introduce errors before applying
- Tier 2 language expansion — CI-verified language count 7 → 13: C++ (clangd), JavaScript (typescript-language-server), Ruby (solargraph), YAML (yaml-language-server), JSON (vscode-json-language-server), Dockerfile (dockerfile-language-server-nodejs); C++ and JavaScript reuse existing CI binaries (zero new install cost); Ruby/YAML/JSON/Dockerfile each add one install line
- Integration test harness updated to 13 langConfig entries with correct fixture positions, cross-file coverage, and per-language capability flags (`supportsFormatting`, `supportsDeclaration`)
- GitHub Actions `multi-lang-test` job extended with 4 new language server install steps

### Fixed
- `clientForFile` now uses `cs.get()` as the authoritative client after `start_lsp` — multi-server routing changes caused `start_lsp` to update `cs` but leave `resolver`'s stale client reference in place, causing all tools to return "LSP client not started" after a successful `start_lsp`; `cs.get()` is now always used for single-server mode
- Test error logging for `open_document` and `get_diagnostics` now extracts text from `Content[0]` instead of printing the raw slice address

### Added
- Multi-server routing — single `agent-lsp` process manages multiple language servers; routes tool calls to the correct server by file extension. Supports inline arg-pairs (`go:gopls typescript:tsserver,--stdio`) and `--config agent-lsp.json`; backward-compatible with existing single-server invocation
- `find_callers` tool — single tool with `direction: "incoming" | "outgoing" | "both"` (default: both); hides the two-step LSP prepare/query protocol behind one call; returns typed JSON with `items`, `incoming`, `outgoing`
- Fuzzy position fallback for `go_to_definition` and `find_references` — when a direct position lookup returns empty, falls back to workspace symbol search by hover name and retries at each candidate; handles AI assistant position imprecision without correctness regression
- Path traversal prevention — `ValidateFilePath` in `WithDocument` resolves all `..` components and verifies the result is within the workspace root; stores `rootDir` on `LSPClient` (set during `Initialize`)
- `types.CallHierarchyItem`, `types.CallHierarchyIncomingCall`, `types.CallHierarchyOutgoingCall` — typed protocol structs for call hierarchy responses
- `types.TextEdit`, `types.SymbolInformation`, `types.SemanticToken` — typed protocol structs; `FormatDocument`/`FormatRange` and `GetWorkspaceSymbols` migrated from `interface{}` to typed returns
- `types.SymbolKind`, `types.SymbolTag` — integer enum types used across call hierarchy and symbol structs
- `get_semantic_tokens` tool — classifies each token in a range as function/parameter/variable/type/keyword/etc using `textDocument/semanticTokens/range` (falls back to full); decodes LSP's delta-encoded 5-integer tuple format into absolute 1-based positions with human-readable type and modifier names from the server's legend; only MCP-LSP server to expose this
- Semantic token legend captured during `initialize` — `legendTypes`/`legendModifiers` stored on `LSPClient` under dedicated mutex; `GetSemanticTokenLegend()` accessor added
- `types.SemanticToken` — typed struct for decoded token output
- Tool count: 24 → 26

### Added (LSP 3.17 spec compliance)
- `workspace/applyEdit` server-initiated request handler — client now responds `ApplyWorkspaceEditResult{applied:true}` instead of null; servers using this for code actions (e.g. file creation/rename) no longer silently fail
- `documentChanges` resource operations: `CreateFile`, `RenameFile`, `DeleteFile` entries now executed (discriminated by `kind` field); previously only `TextDocumentEdit` was processed
- `$/progress report` kind handled — intermediate progress notifications are now logged at debug level instead of silently discarded
- `PrepareRename` `bool` capability case — `renameProvider: true` (no options object) no longer incorrectly sends `textDocument/prepareRename`; correctly returns nil when `prepareProvider` not declared
- `uriToPath` now uses `url.Parse` for RFC 3986-correct percent-decoding — fixes file reads/writes for workspaces with spaces or special characters in path (was using raw string slicing, leaving `%20` literal)
- Removed deprecated `rootPath` from `initialize` params — superseded by `rootUri` and `workspaceFolders`

### Added
- Multi-language integration test harness — Go port of `multi-lang.test.js` using `mcp.CommandTransport` + `ClientSession.CallTool` from the official Go MCP SDK
- Tier 1 tests (start_lsp, open_document, get_diagnostics, inspect_symbol) for all 7 languages: TypeScript, Python, Go, Rust, Java, C, PHP
- Tier 2 tests (list_symbols, go_to_definition, find_references, get_completions, find_symbol, format_document, go_to_declaration) for all 7 languages
- Test fixtures for all 7 languages with cross-file greeter files for `find_references` coverage
- GitHub Actions CI: `test` job (unit tests, every PR) and `multi-lang-test` job (full 7-language matrix)
- `WaitForDiagnostics` initial-snapshot skip — matches TypeScript `sawInitialSnapshot` behavior; prevents early exit when URIs are already cached
- `Initialize` now sends `clientInfo`, `workspace.didChangeConfiguration`, and `workspace.didChangeWatchedFiles` capabilities to match TypeScript reference
- Initial Go port of LSP-MCP — full 1:1 implementation with TypeScript reference
- All 24 tools: session (4), analysis (7), navigation (5), refactoring (6), utilities (2)
- `WithDocument[T]` generic helper — Go equivalent of the TypeScript `withDocument` pattern
- Single binary distribution via `go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest`
- Buffer-based LSP message framing with byte-accurate `Content-Length` parsing (no UTF-8/UTF-16 mismatch)
- `WaitForDiagnostics` with 500ms stabilisation window
- `WaitForFileIndexed` with 1500ms stability window — lets gopls finish cross-package indexing before issuing `find_references`
- Extension registry with compile-time factory registration via `init()`
- `SubscriptionHandlers` and `PromptHandlers` on the `Extension` interface
- Full 14-method LSP request timeout table matching the TypeScript reference
- `$/progress` tracking for workspace-ready detection
- Server-initiated request handling: `window/workDoneProgress/create`, `workspace/configuration`, `client/registerCapability`
- Graceful SIGINT/SIGTERM shutdown with LSP `shutdown` + `exit` sequence
- `GetCodeActions` passes overlapping diagnostics in context per LSP 3.17 §3.16.8
- `SubscribeToDiagnostics` replays current diagnostic snapshot to new subscribers
- `ReopenDocument` fallback to disk read on untracked URI

### Fixed
- `FormattedLocation` JSON field names match TypeScript response shape (`file`, `line`, `column`, `end_line`, `end_column`)
- `apply_edit` argument field is `workspace_edit` in both handler and server registration (was `edit` in `ApplyEditArgs` struct, causing every call to fail silently)
- `execute_command` argument field is `args` (matches TypeScript schema)
- `find_references` `include_declaration` defaults to `false` (matches TypeScript schema)
- `GetInfoOnLocation` hover parsing handles all four LSP `MarkupContent` shapes (string, MarkupContent, MarkedString, MarkedString array)
- `WaitForDiagnostics` timeout 25,000ms (matches TypeScript reference)
- `applyEditsToFile` sends correct incremented version number in `textDocument/didChange`
- `format_document` and `format_range` default `tab_size` is 2 (matches TypeScript schema)
- `format_document` and `format_range` now surface invalid `tab_size` argument errors to callers instead of silently using the default
- `did_change_watched_files` accepts empty `changes` array per LSP spec
- `restart_lsp_server` rejects missing `root_dir` with a clear error instead of sending malformed `rootURI = "file://"` to the LSP server
- `GetSignatureHelp`, `RenameSymbol`, `PrepareRename`, `ExecuteCommand` now propagate JSON unmarshal errors instead of returning `nil, nil` on malformed LSP responses
- `LSPDiagnostic.Code` changed from `string` to `interface{}` — integer codes from rust-analyzer, clangd, etc. are no longer silently dropped
- Removed dead `docVers` field from `LSPClient` (version tracking uses `docMeta.version`)
- `Shutdown` error now wrapped with operation context
- `GenerateResourceList` and `ResourceTemplates` made unexported — they had no external callers and were not wired to the MCP server
- `WaitForDiagnostics` errors in resource handlers now propagate instead of being logged and suppressed
- Removed dead `sep` variable in `framing.go` (`tryParse` allocated `[]byte("\r\n\r\n")` then immediately blanked it)
