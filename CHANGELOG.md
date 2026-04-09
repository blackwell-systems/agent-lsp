# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, Semantic Versioning.

## [Unreleased]

### Changed (2026-04-09)
- **`lsp-verify` skill corrected and hardened** — three fixes from dogfooding: (1) `get_diagnostics` parameter corrected from `workspace_dir` (invalid) to `file_path` — call once per changed file; (2) large test output warning added — `run_tests` on large repos can return 300k+ chars and overflow context; recovery options: grep saved output file for `FAIL` lines, or scope tests to the changed package directly; (3) all three layers now explicitly instructed to run in parallel since they are fully independent.
- **`lsp-dead-code` skill hardened against false positives** — four improvements from dogfooding a full-repo dead-code audit: (1) mandatory Step 0 indexing warm-up — verify a known-active symbol returns ≥1 reference before trusting any results; explicit retry/restart protocol if indexing stalls; (2) `"no identifier found"` recovery note — methods on receivers shift the name column rightward, added grep-for-column technique to recover without blind retrying; (3) zero-reference cross-check — before classifying any handler/constructor/type as dead, grep wiring files (`main.go`, `server.go`, `cmd/`) for the symbol name to catch registration patterns (`server.AddTool(HandleFoo)`) that are invisible to LSP; (4) new caveat #2 documenting why registration-pattern references produce zero LSP hits; Step 3 classification table adds "Zero LSP, found by grep → ACTIVE" as a distinct outcome.

### Fixed (2026-04-09)
- **`get_document_symbols` coordinates are now 1-based** — `range` and `selectionRange` positions in the output were previously 0-based (raw LSP passthrough), inconsistent with every other coordinate-accepting tool (`get_references`, `get_info_on_location`, etc.) which all use 1-based input. The handler now shifts all line/character values by +1 before returning, including in nested `children` symbols. The `lsp-dead-code` skill instruction to "add 1 to selectionRange before passing to get_references" is now unnecessary — coordinates flow directly between tools. **Breaking:** any hardcoded line offsets captured from previous `get_document_symbols` output will be off by one.

### Added (2026-04-09)
- **`get_document_symbols` `format: "outline"` parameter** — when `format: "outline"`, returns the symbol tree as compact markdown (`name [Kind] :line`, indented for children) instead of JSON; reduces token volume ~5x for large files; useful for quick structural surveys before targeted navigation. Default behavior (JSON) unchanged.
- **`start_lsp` `language_id` parameter** — optional field selects a specific configured server in multi-server mode (e.g. `language_id: "go"` targets gopls, `language_id: "typescript"` targets tsserver); routes via new `ServerManager.StartForLanguage` which matches by `language_id` field or extension set; without `language_id`, behavior is unchanged (StartAll). Fixes an agent usability gap where the wrong language server could be active in a mixed-language repo with no in-session override. Description updated to recommend `get_server_capabilities` for diagnosing active-server mismatches.
- **`apply_edit` text-match mode** — new `file_path` + `old_text` + `new_text` parameter mode; finds `old_text` in the file (exact byte match first, then whitespace-normalised line match that tolerates indentation differences) and applies the replacement without requiring line/column positions; positional `workspace_edit` mode unchanged
- **`lsp-edit-symbol` skill** — edit a named symbol without knowing its file or position; composes `get_workspace_symbols` → `get_document_symbols` → `apply_edit` to resolve the symbol name to its definition range and apply the edit; decision guide covers signature-only edits, full-body replacements, and ambiguous symbol disambiguation

### Changed (2026-04-09)
- **Skill descriptions updated with trigger conditions** — all four skill `description` fields now include explicit "use when" clauses per the Claude Code skills spec, enabling automatic invocation when relevant. Descriptions trimmed to ≤250 chars (spec cap). Non-spec `compatibility` field moved to markdown body. `argument-hint` added to `lsp-rename` and `lsp-edit-export` for autocomplete UX.
- **Skills migrated to Agent Skills directory format** — each skill is now a self-contained directory (`lsp-rename/SKILL.md`, `lsp-safe-edit/SKILL.md`, `lsp-edit-export/SKILL.md`, `lsp-verify/SKILL.md`) conforming to the [Agent Skills open spec](https://agentskills.io/specification). Flat `.md` files and shared `PATTERNS.md` removed. `patterns.md` duplicated into each skill's `references/` directory (spec requires self-contained skills). Frontmatter updated: `user-invocable` removed (not in spec), `allowed-tools` fixed to space-delimited, `compatibility` field added. `install.sh` updated to symlink skill directories to `~/.claude/skills/` instead of flat files.

### Added (2026-04-08) — LSP Skills wave

- **`go_to_symbol` MCP tool** — navigate to any symbol by dot-notation path (e.g. `"MyClass.method"`, `"pkg.Function"`) without needing a file path or line/column; uses `GetWorkspaceSymbols` to find candidates and resolves to the definition location; supports optional `workspace_root` and `language` filters
- **Position-pattern parameter (`position_pattern`)** — `@@` cursor marker syntax for position-based tools; `ResolvePositionPattern` searches file content for the pattern and returns the 1-indexed line/col of the character immediately after `@@`; `ExtractPositionWithPattern` integrates with existing `extractPosition` fallback; field added to `GetInfoOnLocationArgs`, `GetReferencesArgs`, `GoToDefinitionArgs`, and `RenameSymbolArgs`
- **Dry-run preview mode for `rename_symbol`** — `dry_run: true` returns a preview envelope `{ "workspace_edit": {...}, "preview": { "note": "..." } }` without writing to disk; existing behavior unchanged when `dry_run` is omitted or false
- **Four agent-native skills** — `lsp-safe-edit`, `lsp-edit-export`, `lsp-rename`, `lsp-verify`; compose lsp-mcp-go tools into single-command workflows for safe editing, exported-symbol refactoring, two-phase rename, and full diagnostic+build+test verification
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
  - `cmd/lsp-mcp-go/server.go` — all 17 per-file tool handlers wrapped with `clientForFileWithAutoInit`; double-checked locking ensures thread-safe single initialization per workspace root


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
- Multi-server routing — single `lsp-mcp-go` process manages multiple language servers; routes tool calls to the correct server by file extension. Supports inline arg-pairs (`go:gopls typescript:tsserver,--stdio`) and `--config lsp-mcp.json`; backward-compatible with existing single-server invocation
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
- Single binary distribution via `go install github.com/blackwell-systems/lsp-mcp-go@latest`
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
