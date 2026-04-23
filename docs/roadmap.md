---
title: Roadmap
---

# Roadmap

## Distribution

| Feature | Status | Description |
|---------|--------|-------------|
| **Nix flake** | Planned | `nix run github:blackwell-systems/agent-lsp` |

## Extensions

Extensions add language-specific tools beyond what LSP exposes. The core 50 tools cover 26 of the most agent-relevant LSP 3.17 methods (navigation, analysis, refactoring, diagnostics, formatting) plus 24 tools that go beyond the LSP spec (speculative execution, build/test, change impact analysis, cross-repo references, audit). Three low-value LSP methods are intentionally omitted: `selectionRange`, `foldingRange`, and `codeLens`. Extensions run arbitrary toolchain logic for a specific language.

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
allowed-tools: mcp__lsp__get_references mcp__lsp__call_hierarchy ...
---
```

**Behavior when a required capability is missing:** The agent receives a warning before activation: "This skill requires `referencesProvider` which the current language server does not support. The skill may produce incomplete results." The agent can decide whether to proceed.

**Behavior when an optional capability is missing:** The skill activates normally. Steps that use the optional capability skip cleanly. The agent sees which steps were skipped in the output.

### Capability profiles by language

Based on CI testing across 30 languages, the capability landscape clusters into tiers:

| Tier | Capabilities | Languages |
|------|-------------|-----------|
| Full | All 50 tools viable | Go (gopls), TypeScript, Rust, C/C++ (clangd), C# |
| Strong | Most tools; missing call/type hierarchy | Python, Ruby, PHP, Kotlin, Swift, Dart, Gleam, Elixir |
| Basic | Navigation + diagnostics; limited refactoring | YAML, JSON, Dockerfile, CSS, HTML, Terraform, SQL |

Skills that target the "Strong" tier should avoid hard dependencies on `callHierarchyProvider` and `typeHierarchyProvider`. Skills that require these should declare them so agents know the limitation upfront.

### Implementation plan

| Feature | Status | Description |
|---------|--------|-------------|
| **`required-capabilities` metadata** | Planned | Space-separated list of LSP server capability keys in SKILL.md frontmatter `metadata` field. Checked against `get_server_capabilities` before skill activation. |
| **`optional-capabilities` metadata** | Planned | Same format. Steps using these capabilities skip cleanly when unavailable. No warning on activation. |
| **Capability check tool** | Planned | A new tool or skill (`/lsp-check-capabilities`) that reports which of the 20 skills are fully viable, partially viable, or unavailable for the current language server. One call shows the agent what it can and cannot do. |
| **Degraded-mode skill variants** | Planned | For high-value skills like `/lsp-impact`, define a degraded path in the skill body that uses only `get_references` when call/type hierarchy are unavailable. Explicit in the prose, not a separate skill file. |

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

## Agent Evaluation Framework

### Shipped: deterministic trajectory assertions (skill protocol CI)

All 20 skills now have deterministic trajectory assertions in `examples/mcp-assert/trajectory/`. These run in the `mcp-assert-trajectory` CI job on every push and PR: 20 inline-trace assertions, no server needed, 0ms each, under 60 seconds total. They validate `presence`, `absence`, `order`, and `args_contain` rules for each skill's required tool call sequence. This is the deterministic subset of Layer 2 skill workflow testing — not LLM-driven, but covering the structural protocol requirements that can be verified without a running agent. The LLM-driven pass@k/pass^k regression suite (below) remains planned.

### Why existing eval frameworks don't fit

Two categories of eval frameworks exist, and neither addresses what agent-lsp needs:

**Agent eval frameworks** ([Strands Evals](https://github.com/strands-agents/evals), [Braintrust](https://braintrust.dev), [LangSmith](https://docs.langchain.com/langsmith), [AgentBench](https://github.com/THUDM/AgentBench), [SWE-bench](https://github.com/SWE-bench/SWE-bench), [BFCL/Gorilla](https://github.com/ShishirPatil/gorilla)) evaluate from the **agent/model perspective**: "did the model call the right tool?" They test agents, not tool providers.

**MCP eval frameworks** ([mcp-evals](https://github.com/mclenhard/mcp-evals) 129 stars, [alpic-ai/mcp-eval](https://github.com/alpic-ai/mcp-eval) 21 stars, [lastmile-ai/mcp-eval](https://github.com/lastmile-ai/mcp-eval) 20 stars, [dylibso/mcpx-eval](https://github.com/dylibso/mcpx-eval) 22 stars, [gleanwork/mcp-server-tester](https://github.com/gleanwork/mcp-server-tester) 13 stars) test MCP servers directly, but every one uses **LLM-as-judge** scoring. They send a prompt, get a response, and ask an LLM "was this good?" on a 1-5 rubric. This makes sense for subjective tool outputs (e.g., "summarize this document") but is the wrong approach for deterministic tools.

When `get_references` is called on line 42 of a Go file, the correct answer is a deterministic set of locations. No LLM-as-judge is needed. The tool either returns the right locations or it does not. Paying for GPT-4 API calls to grade a response that can be verified with `assert.Equal` is wasteful and introduces false variance.

**The gap:** No framework combines deterministic tool correctness testing with MCP server evaluation. No framework tests across multiple languages or programming environments. No framework measures tool reliability (`pass^k`) or skill protocol compliance (trajectory matching).

The only framework with native MCP integration is [Inspect AI](https://github.com/UKGovernmentBEIS/inspect_ai) (1,900+ stars, UK AI Safety Institute). It can serve MCP tools to an evaluated model and score the results. This is useful for Layer 2 (skill workflow testing) but unnecessary for Layer 1 (tool correctness), which is 80% of the work.

### mcp-eval: potential sister project

The gap identified above is not specific to agent-lsp. Every MCP server author needs to prove their tools work. A standalone `mcp-eval` framework would serve the entire MCP ecosystem:

**What it would be:** A Go-based, deterministic-first eval framework for MCP servers. Given an MCP server binary, run its tools against fixture inputs and grade the outputs. No LLM required for correctness testing. Fast, CI-native, zero API costs.

**How it differs from existing MCP eval tools:**

| Dimension | Existing MCP evals | mcp-eval |
|---|---|---|
| Grading | LLM-as-judge (subjective, costly) | Deterministic assertions (exact, free) |
| Language | Node.js / Python | Go (single binary, fast CI) |
| Multi-language | Not supported | Test same tool across multiple language server backends |
| Reliability metrics | Not measured | `pass@k` and `pass^k` per tool per language |
| Skill/workflow testing | Not supported | Trajectory matchers for tool call ordering |
| Docker isolation | Not supported | Per-trial container execution via existing Docker images |

**Relationship to agent-lsp:** agent-lsp is the reference implementation that scores highest on mcp-eval. The eval framework validates the tool server. The tool server demonstrates the eval framework. Sister projects that feed each other.

```bash
# Evaluate any MCP server
mcp-eval run --server "agent-lsp go:gopls" --suite evals/go/
mcp-eval run --server "other-mcp-server" --suite evals/basic/

# Cross-language matrix
mcp-eval matrix --server "agent-lsp" --languages go,typescript,python,gleam

# CI integration
mcp-eval ci --server "agent-lsp" --threshold 95 --fail-on-regression
```

This is a separate repo (`blackwell-systems/mcp-eval`), not embedded in agent-lsp. It evaluates any MCP server, not just agent-lsp. The open-source framing positions it as infrastructure for the MCP ecosystem.

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

For each of 50 tools across N languages, maintain test fixtures with expected outputs. Call the MCP tool directly, compare output against expected results. Organized as Go table-driven tests with per-language, per-tool coverage tracking.

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
            tool: "get_references", language: "go",
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
            tool: "get_references", language: "gleam",
            fixture: "test/fixtures/gleam",
            input: map[string]any{
                "file_path": "src/person.gleam", "line": 1, "column": 10,
            },
            assert: func(t *testing.T, result string) {
                assert.Contains(t, result, "greeter.gleam")
            },
        },
        // ... 50 tools x 30 languages
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

`simulate_edit_atomic` is a code-based grader for edit quality. `net_delta == 0` means the edit is safe. `net_delta > 0` means the agent introduced errors. This is unique to agent-lsp: the tool itself is the eval.

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
| **Blast-radius-first** | Was `get_change_impact` or `get_references` called before any `apply_edit` on an exported symbol? | Transcript |
| **Simulate-before-apply** | Was `simulate_edit_atomic` called before `apply_edit` when a skill was active? | Transcript |
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

## Bigger Bets

| Feature | Status | Description |
|---------|--------|-------------|
| **Observability** | Planned | Metrics (requests/sec, latency per tool, error rate) for production deployments, valuable for teams running agent-lsp as shared infrastructure |
