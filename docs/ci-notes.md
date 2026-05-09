# CI Notes

Implementation details for contributors and maintainers about the language server CI test harness.

## Per-language quirks

**Java (jdtls):** Runs in a dedicated `multi-lang-java` job with `continue-on-error: true` and a 15-minute timeout. Isolated from other language servers to avoid memory contention. The job allocates `-Xmx2G`, runs `mvn compile` to populate `target/classes` before testing, and uses `ready_timeout_seconds: 240` on `start_lsp` to block on `$/progress` completion. jdtls receives `initializationOptions` with Maven/Gradle import settings and `extendedClientCapabilities.progressReportProvider: true` to trigger workspace indexing. **Known limitation:** Tier 2 tools (go_to_definition, references, completions, etc.) currently SKIP because jdtls does not complete workspace indexing within the CI timeout despite correct initialization. Speculative session tests (simulate_edit, evaluate_session) pass. The Tier 1 check (start_lsp, open_document, get_diagnostics) passes. Investigation ongoing; the issue appears to be jdtls-specific project import timing in CI environments.

**Scala (metals):** Runs in a separate CI job with `continue-on-error: true` and a 30-minute timeout. metals requires sbt compilation on first start; results are informational.

**Swift (sourcekit-lsp):** Runs on a `macos-latest` runner since sourcekit-lsp ships with Xcode and is not available on Linux CI runners.

**Prisma:** Runs with `continue-on-error: true`. The language server works standalone after `prisma generate` initializes the client.

**SQL (sqls):** Requires a live PostgreSQL service container. The CI job provisions `postgres:16` automatically.

**Nix (nil):** Runs with `continue-on-error: true`. The Nix installer is slow in CI; nil installs via `nix profile install github:oxalica/nil`.

**MongoDB:** The language server is extracted from the `mongodb-js/vscode` VS Code extension VSIX at `dist/languageServer.js`. The CI job has `continue-on-error: true` since the extracted server may behave differently outside a VS Code extension host context. Requires a live `mongo:7` service container provisioned automatically.

**Zig (zls):** Upgraded from zls 0.13.0 to zls 0.14.0 in CI. 21 verified capabilities: Tier 1 (start_lsp, open_document, get_diagnostics, hover), symbols, definition, references, completions, format, semantic_tokens, signature_help, highlights, code_actions, rename, server_capabilities, workspace_folders, type_definition, format_range, apply_edit, detect_servers, close_document, did_change_watched_files, symbol_source. **Known limitation:** workspace_symbols fails — zls 0.14.0 advertises support but may need a specific query format. **Not supported by zls:** declaration (C-only test), type_hierarchy, call_hierarchy, inlay_hints, prepare_rename, go_to_implementation.

**Gleam:** Requires `gleam build --target javascript` before tests (no Erlang on CI runners). The import path in fixtures uses `person` (not `fixture/person`). The built-in LSP (`gleam lsp`) passes 17 capabilities: Tier 1 (start_lsp, open_document, get_diagnostics, hover), symbols, definition, references, completions, format, code_actions, prepare_rename, rename, server_capabilities, workspace_folders, type_definition, format_range, apply_edit, detect_servers, close_document, did_change_watched_files, and symbol_source. Workspace symbols fails (not implemented upstream, gleam-lang/gleam#5191). Declaration, type_hierarchy, call_hierarchy, semantic_tokens, signature_help, highlights, inlay_hints, and go_to_implementation skip (server does not advertise support).

**Elixir (elixir-ls):** Runs with `continue-on-error: true` using `erlef/setup-beam@v1` (Elixir 1.16 / OTP 26). 16 verified capabilities: Tier 1 (start_lsp, open_document, get_diagnostics, hover), definition, references, completions, workspace_symbols, format, signature_help, call_hierarchy, code_actions, server_capabilities, workspace_folders, format_range, apply_edit, detect_servers, close_document, did_change_watched_files. **Known limitation:** `get_document_symbols` (symbols) fails because ElixirLS needs more compile time than the 20s init wait provides. **Not supported by ElixirLS:** declaration, type_hierarchy, semantic_tokens, highlights, inlay_hints, prepare_rename, rename, type_definition, go_to_implementation, symbol_source.

**Clojure (clojure-lsp), Nix (nil), Dart (dart language-server), MongoDB (mongodb-language-server):** CI-verified as of the `ci-coverage-expansion` IMPL.

## mcp-assert: protocol-level assertions

mcp-assert runs two separate CI jobs against agent-lsp. Together they cover both tool correctness (does each tool return the right response through the MCP transport?) and skill protocol compliance (does an agent follow the correct tool call sequence for a given skill?).

### Job 1: `mcp-assert-trajectory` (fast, no server needed)

Validates that agent skills follow correct tool call sequences using inline traces embedded in YAML files. No live language server is required; all assertions run against pre-recorded traces and complete in 0ms each. Total job runtime is under 60 seconds.

**Assertion files:** `examples/mcp-assert/trajectory/` (21 files, one per skill)

**What trajectory assertions verify:** Each `trajectory_*.yaml` file declares an `inline` trace (a sequence of tool calls with arguments) and a set of trajectory rules:

| Rule type | What it checks |
|-----------|----------------|
| `presence` | Required tools appear in the trace |
| `absence` | Forbidden tools do not appear |
| `order` | Tools appear in the correct sequence |
| `args_contain` | A specific tool call includes expected argument values |

**Skills covered:** All 21 skills have trajectory assertions.

| Skill | Assertion file |
|-------|---------------|
| `/lsp-cross-repo` | `trajectory_cross_repo_protocol.yaml` |
| `/lsp-dead-code` | `trajectory_dead_code_protocol.yaml` |
| `/lsp-docs` | `trajectory_docs_protocol.yaml` |
| `/lsp-edit-export` | `trajectory_edit_export_protocol.yaml` |
| `/lsp-edit-symbol` | `trajectory_edit_symbol_protocol.yaml` |
| `/lsp-explore` | `trajectory_explore_protocol.yaml` |
| `/lsp-extract-function` | `trajectory_extract_function_protocol.yaml` |
| `/lsp-fix-all` | `trajectory_fix_all_protocol.yaml` |
| `/lsp-format-code` | `trajectory_format_code_protocol.yaml` |
| `/lsp-generate` | `trajectory_generate_protocol.yaml` |
| `/lsp-impact` | `trajectory_impact_protocol.yaml` |
| `/lsp-implement` | `trajectory_implement_protocol.yaml` |
| `/lsp-inspect` | `trajectory_inspect_protocol.yaml` |
| `/lsp-local-symbols` | `trajectory_local_symbols_protocol.yaml` |
| `/lsp-refactor` | `trajectory_refactor_protocol.yaml` |
| `/lsp-rename` | `trajectory_rename_protocol.yaml` |
| `/lsp-safe-edit` | `trajectory_safe_edit_protocol.yaml` |
| `/lsp-simulate` | `trajectory_simulate_protocol.yaml` |
| `/lsp-test-correlation` | `trajectory_test_correlation_protocol.yaml` |
| `/lsp-understand` | `trajectory_understand_protocol.yaml` |
| `/lsp-verify` | `trajectory_verify_protocol.yaml` |

**Why trajectory runs separately:** Trajectory assertions use inline traces, so they require no running agent-lsp binary and no gopls. They are purely structural: given a tool call sequence, do the ordering and presence rules hold? This lets them run earlier and faster than the tool correctness job, providing immediate feedback on skill protocol regressions without waiting for language server startup.

**Adding trajectory assertions:** Create a YAML file in `examples/mcp-assert/trajectory/` (and a matching copy in `examples/mcp-assert/go/` for the full suite). Use `trace:` to define the inline sequence and `trajectory:` for the rules.

### Job 2: `mcp-assert` (tool correctness, requires gopls)

Runs deterministic assertions against agent-lsp through the MCP stdio transport layer, testing the same path agents use in production. This complements the Go integration tests which call internal functions directly.

**How it works:** [mcp-assert](https://github.com/blackwell-systems/mcp-assert) starts agent-lsp as an MCP server subprocess, sends tool calls over JSON-RPC, and checks the responses against YAML-defined assertions. No LLM involved; all grading is deterministic.

**Assertion suite:** `examples/mcp-assert/go/` — tool correctness assertions plus multi-step workflow assertions (including trajectory files which are also tested here as part of the full suite).

**Tool correctness assertions include:**

| Assertion | Tool tested | What it verifies |
|---|---|---|
| hover | `get_info_on_location` | Returns type info for `Person` at definition site |
| definition | `go_to_definition` | Resolves `Person` reference in greeter.go to main.go |
| references | `get_references` | Finds cross-file callers (main.go + greeter.go), min 2 results |
| diagnostics | `get_diagnostics` | Clean diagnostics for a valid file |
| symbols | `get_document_symbols` | Lists `Person` type and `Greet` method |
| completions | `get_completions` | Returns non-empty completions at a method call site |
| speculative | `simulate_edit_atomic` | Detects type error (`return 42` in `string` method), returns `net_delta` |

**Warmup pattern:** The `references` and `speculative` assertions include `get_diagnostics` setup steps to give gopls time to index the workspace before the actual assertion. Without this, gopls may not have cross-file relationships indexed and returns incomplete results.

**Timeout:** Each assertion has a 120s timeout. The references assertion typically takes ~27s (gopls indexing time). Total job runtime is ~2 minutes.

**Adding new tool correctness assertions:** Create a YAML file in `examples/mcp-assert/go/` following the existing format. Use `{{fixture}}` for fixture directory substitution. Add warmup steps (`get_diagnostics` or `get_references`) if the assertion depends on cross-file indexing.

## Speculative session test job

`speculative-test` runs `TestSpeculativeSessions` across 8 languages in parallel. Each language subtest gets its own MCP process; subtests within a language run sequentially.

| Language | LSP binary | Error edit target | initWait | Timeout |
|---|---|---|---|---|
| Go | gopls | `return 42` in `string` method | 8s | 120s |
| TypeScript | typescript-language-server | `return "wrong"` in `number` function | 8s | 120s |
| Python | pyright-langserver | `return "wrong"` in `int` function | 8s | 120s |
| Rust | rust-analyzer | `"wrong"` where `i32` expected | 15s | 120s |
| C++ | clangd | `return "wrong"` in `int` function | 10s | 120s |
| C# | csharp-ls | `return 42` in `string` method | 10s | 120s |
| Dart | dart (language-server) | `return 42` in `String` method | 8s | 120s |
| Java | jdtls | `return "wrong"` in `int` method | 120s | 300s |

**Java quirk:** jdtls JVM cold-start requires a 120s `initWait` and a 300s per-language timeout. The CI job timeout is set to 20m. The jdtls workspace data dir (`/tmp/jdtls-workspace-speculative-test`) is separate from the one used by `multi-lang-core` (`/tmp/jdtls-workspace-lsp-mcp-test`) to prevent state collisions if both jobs run on the same runner.

**C++ quirk:** clangd provides single translation-unit (TU) diagnostics only. Cross-file propagation requires a rebuild step not available in the session model. `error_detection` is still reliable for intra-file type errors.

## Test file inventory

| File | Job | What it tests |
|---|---|---|
| `test/multi_lang_test.go` | `multi-lang-core`, `multi-lang-extended`, + per-language jobs | Tier 1 + 34 Tier 2 tools across 30 language servers |
| `test/speculative_test.go` | `speculative-test` | All 8 simulation tools across 8 languages in parallel |
| `test/error_paths_test.go` | `unit-and-smoke` | 11 bad-input subtests (out-of-bounds positions, nonexistent files, invalid session IDs); asserts well-formed errors, not crashes |
| `test/consistency_test.go` | `multi-lang-core` | Structural shape validation for 4 tools across Go, TypeScript, Python, Rust in parallel |
| `test/build_tools_test.go` | `unit-and-smoke` | `run_build`, `run_tests`, `get_tests_for_file` |
| `test/documentation_test.go` | `unit-and-smoke` | `get_symbol_documentation` |
| `test/binary_test.go` | `unit-and-smoke` | Binary smoke tests (startup, missing args, help) |
| `examples/mcp-assert/go/*.yaml` | `mcp-assert` | Protocol-level assertions via MCP stdio: tool correctness (hover, definition, references, diagnostics, symbols, completions, speculative) plus multi-step workflow assertions |
| `examples/mcp-assert/trajectory/*.yaml` | `mcp-assert-trajectory` | 21 trajectory assertions (one per skill); inline traces, no server needed, 0ms each |

## Tool-specific notes

**`type_hierarchy`:** Tested on Java (jdtls) and TypeScript (typescript-language-server). TypeScript skips when the server does not return a hierarchy item at the configured position.

**Completions and workspace symbol search:** Not supported by some servers in the test harness; marked `—` in the conformance table.
