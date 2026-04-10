# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, Semantic Versioning.

## [Unreleased]

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
Answers "what breaks if I change this file?" without running tests. Given a list of changed files, it enumerates all exported symbols in those files via `get_document_symbols`, resolves every reference via `get_references`, and partitions the results into test callers (with enclosing test function names extracted) and non-test callers. Supports optional one-level transitive following to surface second-order impact. Useful before any refactor to understand blast radius and which tests will need updating.

#### `get_cross_repo_references`
First-class cross-repo caller analysis. Given a symbol (file + position) and a list of consumer repo roots, adds each consumer as a workspace folder and calls `get_references` across all of them. Results are partitioned by repo root prefix so callers in each consumer are reported separately. Designed for library authors who need to know which downstream consumers reference a symbol before changing its signature.

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
- **`discard_path` bug fix** — test was calling `simulate_edit_atomic` with a `session_id`, but `simulate_edit_atomic` is a self-contained tool (creates its own session internally, requires `workspace_root` + `language`); the call was silently returning `IsError: true` and logging it as "may be expected"; fixed to call `simulate_edit` which is the correct tool for applying edits to an existing session
- **`evaluate_session` response assertions** — existing subtests were only logging the response; now parse the JSON and assert `net_delta == 0` for comment-only edits (with `confidence != "low"` guard for CI timing); `simulate_edit` response now asserts `edit_applied == true`
- **`simulate_chain` response assertions** — parse `ChainResult` JSON; assert `cumulative_delta == 0` for two-comment chain; assert `safe_to_apply_through_step == 2`
- **`commit_path` improved** — now applies a comment edit via `simulate_edit` before committing, making the test more meaningful than committing a clean session
- **`simulate_edit_atomic_standalone` subtest** — proper standalone usage of `simulate_edit_atomic` with `workspace_root` + `language` parameters; asserts response is an `EvaluationResult` with `net_delta == 0` for a comment edit
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

### Added (2026-04-10) — Language expansion (26 languages)
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
- **`/lsp-test-correlation` skill** — find and run only the tests covering an edited source file; `get_tests_for_file` maps source → test files, `get_workspace_symbols` enumerates specific test functions within those files, `run_tests` executes the scoped set; fallback to workspace symbol search when `get_tests_for_file` returns no mapping; multi-file workflow deduplicates test files across all changed sources; `[correlated / unrelated]` classification guides where to investigate failures first
- **`/lsp-verify` `get_tests_for_file` pre-step** — when `changed_files` is known, `get_tests_for_file` runs before the three parallel layers to build a source→test map; Layer 3 failure report now tags each failing test as correlated (covers changed code) or unrelated (collateral failure) to narrow debugging scope

### Added (2026-04-09) — Skills expansion
- **`/lsp-cross-repo` skill** — multi-root workspace analysis for library + consumer workflows; orchestrates `add_workspace_folder` → `list_workspace_folders` (verify indexing) → `get_workspace_symbols` → `get_references` / `call_hierarchy` / `go_to_implementation` across both repos; solves the "agent doesn't know to add a second workspace folder" discoverability gap; output separates library-internal from consumer references
- **`/lsp-local-symbols` skill** — file-scoped symbol analysis without workspace-wide search; composes `get_document_symbols` (symbol tree for the file) → `get_document_highlights` (all usages within the file, classified as read/write/text) → `get_info_on_location` (type signature); faster than `get_references` for local-scope questions; explicit "when NOT to use" guidance prevents misuse as a cross-file search
- **`/lsp-rename` `prepare_rename` safety gate** — `prepare_rename` now runs as Step 2 (after symbol location, before reference enumeration); validates that the language server can rename at the given position before doing any further work; catches built-ins, keywords, and imported external package names that cannot be renamed across module boundaries; fail-fast with actionable error message
- **`/lsp-safe-edit` `simulate_edit_atomic` pre-flight** — `simulate_edit_atomic` now runs before any disk write (Step 3); returns `net_delta` (errors introduced minus resolved) without touching disk; `net_delta > 0` pauses and asks before proceeding; multi-file: run per-file independently and sum deltas
- **`/lsp-safe-edit` code actions on introduced errors** — if post-edit diagnostics introduce new errors, `get_code_actions` is called at each error location and available quick fixes are surfaced to the user with `y/n/select`; accepted actions applied via `apply_edit`, then re-diff
- **`/lsp-safe-edit` multi-file workflow** — explicit protocol for edits spanning multiple files: open all, collect BEFORE for all, simulate each file independently, apply file-by-file (stop on first failure), merge AFTER diagnostics, check code actions on any file with new errors

### Changed (2026-04-09)
- **`lsp-verify` skill corrected and hardened** — three fixes from dogfooding: (1) `get_diagnostics` parameter corrected from `workspace_dir` (invalid) to `file_path` — call once per changed file; (2) large test output warning added — `run_tests` on large repos can return 300k+ chars and overflow context; recovery options: grep saved output file for `FAIL` lines, or scope tests to the changed package directly; (3) all three layers now explicitly instructed to run in parallel since they are fully independent.
- **`lsp-dead-code` skill hardened against false positives** — four improvements from dogfooding a full-repo dead-code audit: (1) mandatory Step 0 indexing warm-up — verify a known-active symbol returns ≥1 reference before trusting any results; explicit retry/restart protocol if indexing stalls; (2) `"no identifier found"` recovery note — methods on receivers shift the name column rightward, added grep-for-column technique to recover without blind retrying; (3) zero-reference cross-check — before classifying any handler/constructor/type as dead, grep wiring files (`main.go`, `server.go`, `cmd/`) for the symbol name to catch registration patterns (`server.AddTool(HandleFoo)`) that are invisible to LSP; (4) new caveat #2 documenting why registration-pattern references produce zero LSP hits; Step 3 classification table adds "Zero LSP, found by grep → ACTIVE" as a distinct outcome.

### Fixed (2026-04-09)
- **`get_document_symbols` coordinates are now 1-based** — `range` and `selectionRange` positions in the output were previously 0-based (raw LSP passthrough), inconsistent with every other coordinate-accepting tool (`get_references`, `get_info_on_location`, etc.) which all use 1-based input. The handler now shifts all line/character values by +1 before returning, including in nested `children` symbols. The `lsp-dead-code` skill instruction to "add 1 to selectionRange before passing to get_references" is now unnecessary — coordinates flow directly between tools. **Breaking:** any hardcoded line offsets captured from previous `get_document_symbols` output will be off by one.

### Added (2026-04-09)
- **`lsp-implement` skill** — find all concrete implementations of an interface or abstract type; composes `go_to_implementation` + `type_hierarchy`; includes capability pre-check, risk assessment table (0 implementors → likely unused, >10 → breaking API change), and language notes for Go/TypeScript/Java/Rust/C#
- **`lsp-verify` code action fix section** — when Layer 1 diagnostics return errors, call `get_code_actions` at the error location to surface available quick fixes, apply with `apply_edit`, then re-verify; `get_code_actions` and `apply_edit` added to skill `allowed-tools`
- **`get_document_symbols` `format: "outline"` parameter** — when `format: "outline"`, returns the symbol tree as compact markdown (`name [Kind] :line`, indented for children) instead of JSON; reduces token volume ~5x for large files; useful for quick structural surveys before targeted navigation. Default behavior (JSON) unchanged.
- **`start_lsp` `language_id` parameter** — optional field selects a specific configured server in multi-server mode (e.g. `language_id: "go"` targets gopls, `language_id: "typescript"` targets tsserver); routes via new `ServerManager.StartForLanguage` which matches by `language_id` field or extension set; without `language_id`, behavior is unchanged (StartAll). Fixes an agent usability gap where the wrong language server could be active in a mixed-language repo with no in-session override. Description updated to recommend `get_server_capabilities` for diagnosing active-server mismatches.
- **`apply_edit` text-match mode** — new `file_path` + `old_text` + `new_text` parameter mode; finds `old_text` in the file (exact byte match first, then whitespace-normalised line match that tolerates indentation differences) and applies the replacement without requiring line/column positions; positional `workspace_edit` mode unchanged
- **`lsp-edit-symbol` skill** — edit a named symbol without knowing its file or position; composes `get_workspace_symbols` → `get_document_symbols` → `apply_edit` to resolve the symbol name to its definition range and apply the edit; decision guide covers signature-only edits, full-body replacements, and ambiguous symbol disambiguation
- **`get_symbol_source` tool** — returns the source code of the innermost symbol (function, method, struct, class, etc.) whose range contains a given cursor position; composes `textDocument/documentSymbol` + file read; `findInnermostSymbol` walks the symbol tree recursively to find the deepest enclosing symbol; accepts `line`+`character` (1-based) or `position_pattern` (@@-syntax); `character` aliased to `column` for consistency with other tools; CI-verified in `testGetSymbolSource` across all 22 languages
- **MCP log notifications** — internal log messages (LSP server start, tool dispatch errors, indexing events) now route as `notifications/message` to the connected MCP client via `mcpSessionSender`; wired through `InitializedHandler` in `ServerOptions` so the live `*ServerSession` is captured per-connection; before session init and on send failure, falls back to stderr; level threshold controlled by `set_log_level`
- **`get_symbol_documentation` tool** — fetch authoritative documentation for a named
  symbol from local toolchain sources (go doc, pydoc, cargo doc) without requiring an
  LSP hover response. Works on transitive dependencies not indexed by the language
  server. Returns `{ symbol, language, source, doc, signature }`. Dispatches to
  per-language toolchain commands with a 10-second timeout; strips ANSI escape codes;
  returns a structured error (not MCP error) when the toolchain fails so callers can
  fall back to LSP hover.
- **`lsp-docs` skill** — three-tier documentation lookup: (1) `get_info_on_location`
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

## [Unreleased]

### Fixed (2026-04-08)
- **`run_build` and `run_tests` in Go workspaces** — both tools now unconditionally set `GOWORK=off` when running `go build` and `go test`; Go searches upward through parent directories for `go.work` files, and when found, `./...` patterns only match modules listed in the workspace file; setting `GOWORK=off` forces Go to build/test all modules in the directory, matching the tool's intent

### Added (2026-04-08)
- **`run_build`, `run_tests`, and `get_tests_for_file` MCP tools** — three new
  build-tool integration tools that do not require `start_lsp`; language-specific
  dispatch: `go build ./...` / `cargo build` / `tsc --noEmit` / `mypy .` (run_build),
  `go test -json ./...` / `cargo test --message-format=json` / `pytest --tb=json` /
  `npm test` (run_tests); test failure `location` fields are LSP-normalized (file URI
  + zero-based range) — paste directly into `go_to_definition` or `get_references`;
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
- **Integration test coverage expanded to 26 tools** — multi-language Tier 2 matrix grown from 12 → 26 tools per language: added `testGetDocumentHighlights`, `testGetInlayHints`, `testGetCodeActions`, `testPrepareRename`, `testRenameSymbol`, `testGetServerCapabilities`, `testWorkspaceFolders`, `testGoToTypeDefinition`, `testGoToImplementation`, `testFormatRange`, `testApplyEdit`, `testDetectLspServers`, `testCloseDocument`, `testDidChangeWatchedFiles`; `TestSpeculativeSessions` in `test/speculative_test.go` covers full lifecycle: create, `simulate_edit` (non-atomic), `simulate_edit_atomic`, `simulate_chain`, evaluate, discard, commit, destroy
- **`rename_symbol` fuzzy position fallback** — when the direct position lookup returns an empty `WorkspaceEdit`, falls back to workspace symbol search by hover name and retries at each candidate position; mirrors the fuzzy fallback already in `go_to_definition` and `get_references`; handles AI position imprecision without correctness regression
- **Multi-root workspace support** — `add_workspace_folder`, `remove_workspace_folder`, `list_workspace_folders` tools; `workspace/didChangeWorkspaceFolders` notifications; enables cross-repo references, definitions, and diagnostics across library + consumer repos in one session; workspace folder list persisted on client and initialized from `start_lsp` root
- **`get_document_highlights`** — file-scoped symbol occurrence search (`textDocument/documentHighlight`); returns ranges with read/write/text kinds; instant, no workspace scan; `DocumentHighlight` and `DocumentHighlightKind` types added to `internal/types`
- **Auto-watch workspace** — `fsnotify` watcher starts automatically after `start_lsp`; forwards file changes to the LSP server via `workspace/didChangeWatchedFiles`; debounced 150ms; skips `.git/`, `node_modules/`, etc.; `did_change_watched_files` tool no longer required for normal editing workflows
- **`get_server_capabilities`** — returns server identity (`name`, `version` from `serverInfo`), full LSP capability map, and classified tool lists (`supported_tools` / `unsupported_tools`) based on what the server advertised at initialization; lets AI pre-filter capability-gated tools before calling them; `GetCapabilities()` and `GetServerInfo()` methods added to `LSPClient`; `serverName`/`serverVersion` now captured from initialize response
- **`get_inlay_hints`** — new MCP tool (`textDocument/inlayHint`); returns inline type annotations and parameter name labels for a range; capability-guarded (returns empty array when server does not support `inlayHintProvider`); `InlayHint`, `InlayHintLabelPart`, `InlayHintKind` types added to `internal/types`
- **`detect_lsp_servers`** — new MCP tool; scans workspace for source languages (file extensions + root markers, scored by prevalence), checks PATH for corresponding LSP server binaries, returns `suggested_config` entries ready to paste into MCP config; deduplicates shared binaries (c+cpp → one clangd entry)
- **`get_workspace_symbols` enrichment** — new `detail_level`, `limit`, `offset` params; `detail_level=hover` enriches a paginated window of results with hover info (type signature + docs); `symbols[]` always returns full result set; `enriched[]` + `pagination` returned for the window; mirrors mcp-lsp-bridge's ToC + detail-window pattern
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
- `simulate_edit_atomic` now calls `Discard` before `Destroy` — without Discard, gopls retained the modified document between atomic calls; the next call's baseline captured stale (modified) diagnostics, producing incorrect `net_delta` values
- `start_lsp` in multi-server/auto-detect mode now calls `ServerManager.StartAll` — previously only restarted the first detected server (clangd), leaving gopls and other language servers uninitialized; simulation sessions for Go files now correctly use gopls
- `csResolver` wrapper added to `server.go` so `SessionManager` sees clients set by `start_lsp` at runtime; previously the original resolver held a nil client until `start_lsp` was called, causing "no LSP client available" errors
- `SessionManager.CreateSession` routes by language extension via `ClientForFile` — in multi-server mode `DefaultClient()` returned clangd; routing by `.go`/`.py`/`.ts` extension now picks the correct language server per session
- `languageToExtension` helper added to `internal/session/manager.go` — maps language IDs (`go`, `python`, `typescript`, `javascript`, `rust`, `c`, `cpp`, `java`, `ruby`) to file extensions for client routing

### Added
- **Speculative code sessions** — simulate edits without committing to disk; create sessions with baseline diagnostics, apply edits in-memory, evaluate diagnostic changes (errors introduced/resolved), and commit or discard atomically; implemented via `internal/session` package with SessionManager (lifecycle), SerializedExecutor (LSP access serialization), and diagnostic differ (baseline vs current comparison); 8 new MCP tools: `create_simulation_session`, `simulate_edit`, `evaluate_session`, `simulate_chain`, `commit_session`, `discard_session`, `destroy_session`, `simulate_edit_atomic`; tool count 26 → 34; enables safe what-if analysis and multi-step edit planning before execution; useful for AI assistants to verify edits won't introduce errors before applying
- Tier 2 language expansion — CI-verified language count 7 → 13: C++ (clangd), JavaScript (typescript-language-server), Ruby (solargraph), YAML (yaml-language-server), JSON (vscode-json-language-server), Dockerfile (dockerfile-language-server-nodejs); C++ and JavaScript reuse existing CI binaries (zero new install cost); Ruby/YAML/JSON/Dockerfile each add one install line
- Integration test harness updated to 13 langConfig entries with correct fixture positions, cross-file coverage, and per-language capability flags (`supportsFormatting`, `supportsDeclaration`)
- GitHub Actions `multi-lang-test` job extended with 4 new language server install steps

### Fixed
- `clientForFile` now uses `cs.get()` as the authoritative client after `start_lsp` — multi-server routing changes caused `start_lsp` to update `cs` but leave `resolver`'s stale client reference in place, causing all tools to return "LSP client not started" after a successful `start_lsp`; `cs.get()` is now always used for single-server mode
- Test error logging for `open_document` and `get_diagnostics` now extracts text from `Content[0]` instead of printing the raw slice address

### Added
- Multi-server routing — single `agent-lsp` process manages multiple language servers; routes tool calls to the correct server by file extension. Supports inline arg-pairs (`go:gopls typescript:tsserver,--stdio`) and `--config agent-lsp.json`; backward-compatible with existing single-server invocation
- `call_hierarchy` tool — single tool with `direction: "incoming" | "outgoing" | "both"` (default: both); hides the two-step LSP prepare/query protocol behind one call; returns typed JSON with `items`, `incoming`, `outgoing`
- Fuzzy position fallback for `go_to_definition` and `get_references` — when a direct position lookup returns empty, falls back to workspace symbol search by hover name and retries at each candidate; handles AI assistant position imprecision without correctness regression
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
- Tier 1 tests (start_lsp, open_document, get_diagnostics, get_info_on_location) for all 7 languages: TypeScript, Python, Go, Rust, Java, C, PHP
- Tier 2 tests (get_document_symbols, go_to_definition, get_references, get_completions, get_workspace_symbols, format_document, go_to_declaration) for all 7 languages
- Test fixtures for all 7 languages with cross-file greeter files for `get_references` coverage
- GitHub Actions CI: `test` job (unit tests, every PR) and `multi-lang-test` job (full 7-language matrix)
- `WaitForDiagnostics` initial-snapshot skip — matches TypeScript `sawInitialSnapshot` behavior; prevents early exit when URIs are already cached
- `Initialize` now sends `clientInfo`, `workspace.didChangeConfiguration`, and `workspace.didChangeWatchedFiles` capabilities to match TypeScript reference
- Initial Go port of LSP-MCP — full 1:1 implementation with TypeScript reference
- All 24 tools: session (4), analysis (7), navigation (5), refactoring (6), utilities (2)
- `WithDocument[T]` generic helper — Go equivalent of the TypeScript `withDocument` pattern
- Single binary distribution via `go install github.com/blackwell-systems/agent-lsp@latest`
- Buffer-based LSP message framing with byte-accurate `Content-Length` parsing (no UTF-8/UTF-16 mismatch)
- `WaitForDiagnostics` with 500ms stabilisation window
- `WaitForFileIndexed` with 1500ms stability window — lets gopls finish cross-package indexing before issuing `get_references`
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
- `get_references` `include_declaration` defaults to `false` (matches TypeScript schema)
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
