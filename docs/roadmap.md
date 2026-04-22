---
title: Roadmap
---

# Roadmap

## Distribution

| Feature | Status | Description |
|---------|--------|-------------|
| **Nix flake** | Planned | `nix run github:blackwell-systems/agent-lsp` |

## Extensions

Extensions add language-specific tools beyond what LSP exposes. The core 50 tools cover everything the language server protocol provides; extensions run arbitrary toolchain logic for a specific language.

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
| **`agent-lsp update`** | Planned | Self-update to the latest release; fetches from GitHub Releases and replaces the binary in-place |
| **Config file format** | Planned | `~/.agent-lsp.json` or `agent-lsp.json` project file for complex setups with per-server options |
| **Continue.dev config support** | Planned | `agent-lsp init` currently skips Continue.dev; it uses a different config format than `mcpServers` |

## Skills

20 skills shipped. See [skills.md](skills.md) for the full catalog.

### Creation skills

Current skills are oriented around modifying existing code. These skills target greenfield creation workflows where LSP can still add value through completions, diagnostics, and code actions.

| Skill | Description |
|-------|-------------|
| `/lsp-create` | Iterative file creation with diagnostic checks between steps. Create file, open in LSP, write incrementally, verify diagnostics after each addition, format on completion. `/lsp-safe-edit` for files that don't exist yet. |
| `/lsp-implement` (extend) | Given an interface or type definition, generate the full implementation using `get_completions` to discover required methods, verify it compiles via diagnostics, format. |
| `/lsp-discover-api` | Completion-driven API exploration. Open a file, place the cursor after a package qualifier, call `get_completions` to show available methods/fields. Use LSP knowledge instead of training data (which may be outdated). |
| `/lsp-bootstrap` | Project scaffolding with LSP verification. Create build files (go.mod, package.json, Cargo.toml), start LSP, confirm indexing works, verify initial diagnostics are clean before writing application code. |
| `/lsp-wire` | After creating a new package/module, verify it's importable from the intended consumer, check the public API surface via `get_document_symbols`, confirm no dangling imports or missing exports. |

### Skill composition

Skills calling other skills. `/lsp-refactor` is already composed from `/lsp-impact` + `/lsp-safe-edit` + `/lsp-verify` + `/lsp-test-correlation`. Formal runtime support for skill-to-skill invocation would enable arbitrary composition.

## Skill Schema Specification

Skills are currently prose, markdown prompts the agent follows. The inputs and outputs are implicit and unvalidatable. A schema layer would make contracts explicit (what goes in, what comes out), enabling validation and eventual skill composition with typed interfaces.

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

agent-lsp currently launches and manages its own language server processes. In IDE environments, the IDE already has gopls/pyright/rust-analyzer running and indexed. Passive mode would connect to an already-running server instead of spawning a duplicate, eliminating double-indexing and double memory usage.

`agent-lsp --connect go:localhost:9999 typescript:localhost:9998`

Some language servers support multi-client connections over TCP (gopls supports `gopls -listen=:9999`). Passive mode would connect to these sockets and share the IDE's warm index. No IDE plugin required for this path.

| Feature | Status | Description |
|---------|--------|-------------|
| **`--connect` transport** | Planned | Connect to an existing language server TCP socket instead of spawning a new process |
| **Shared index** | Planned | Reuse the IDE's warm language server index; no duplicate indexing or memory overhead |

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
| Speculative execution confidence | `confidence` field from `simulate_edit_atomic` (`high`/`partial`/`eventual`) | New speculative test per language |
| Speculative round-trip time | `simulate_edit_atomic` call to response | New speculative test per language |
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

## Bigger Bets

| Feature | Status | Description |
|---------|--------|-------------|
| **Observability** | Planned | Metrics (requests/sec, latency per tool, error rate) for production deployments, valuable for teams running agent-lsp as shared infrastructure |
