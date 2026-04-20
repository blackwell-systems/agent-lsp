# Roadmap

## Distribution

| Feature | Status | Description |
|---------|--------|-------------|
| **Nix flake** | Planned | `nix run github:blackwell-systems/agent-lsp` |

## Extensions

Extensions add language-specific tools beyond what LSP exposes. The core 50 tools cover everything the language server protocol provides; extensions run arbitrary toolchain logic for a specific language.

### Go extension (Wave 1 — test + module intelligence)

| Tool | Description |
|------|-------------|
| `go.test_run` | Run a specific test by name, return full output + pass/fail |
| `go.test_coverage` | Coverage % and uncovered lines for a file or package |
| `go.benchmark_run` | Run a benchmark, return ns/op and allocs/op |
| `go.test_race` | Run with `-race`, return any data races found |
| `go.mod_graph` | Full dependency tree as structured data |
| `go.mod_why` | Why is this package in go.mod? (`go mod why`) |
| `go.mod_outdated` | List deps with available upgrades |
| `go.vulncheck` | `govulncheck` scan — CVEs with affected symbols |

### Go extension (Wave 2 — build + quality)

| Tool | Description |
|------|-------------|
| `go.escape_analysis` | `gcflags="-m"` output for a function — what allocates and why |
| `go.cross_compile` | Try cross-compiling for a target OS/arch, return errors |
| `go.lint` | `staticcheck` or `golangci-lint` output for a file |
| `go.deadcode` | Find exported symbols with no callers (`go tool deadcode`) |
| `go.vet_all` | `go vet ./...` with structured output |

### Go extension (Wave 3 — generation + docs)

| Tool | Description |
|------|-------------|
| `go.generate` | Run `go generate` on a file, return output |
| `go.generate_status` | Which `//go:generate` directives are stale |
| `go.doc` | `go doc` output for any symbol — richer than hover |
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
| `python.imports` | `isort` check — unsorted or missing imports |

### C / C++ extension

The gap between what clangd provides and what the broader toolchain offers is larger than any other language — sanitizers and profiling are completely outside LSP scope.

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
| `java.deps` | `jdeps` dependency analysis — what packages does this class use? |
| `java.checkstyle` | Checkstyle violations for a file |
| `java.spotbugs` | SpotBugs static analysis findings |

### Elixir extension

| Tool | Description |
|------|-------------|
| `elixir.test_run` | Run a specific ExUnit test, return output |
| `elixir.dialyzer` | Dialyzer type analysis — unique to Elixir, finds type errors without annotations |
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

20 skills shipped. See [docs/skills.md](docs/skills.md) for the full catalog.

### Skill composition

Skills calling other skills. `/lsp-refactor` is already composed from `/lsp-impact` + `/lsp-safe-edit` + `/lsp-verify` + `/lsp-test-correlation`. Formal runtime support for skill-to-skill invocation would enable arbitrary composition.

## Skill Schema Specification

Skills are currently prose — markdown prompts the agent follows. The inputs and outputs are implicit and unvalidatable. A schema layer would make contracts explicit — what goes in, what comes out — enabling validation and eventual skill composition with typed interfaces.

The case for machine-readable skill contracts:
- Tooling can validate that an agent invoked a skill correctly
- Clearer interface between the agent and the skill — what goes in, what comes out
- Enables skill composition with type safety (skill A's output feeds skill B's input)
- Documentation that can be auto-generated and kept in sync

| Feature | Status | Description |
|---------|--------|-------------|
| **Skill input/output schema** | Planned | JSON Schema definitions for each skill's expected inputs and guaranteed outputs — machine-readable contracts alongside the prose skill files |
| **Schema validation tooling** | Planned | Validate agent skill invocations against the schema at runtime or in CI — surfaces misuse before it causes silent failures |

## Bigger Bets

| Feature | Status | Description |
|---------|--------|-------------|
| **VS Code extension** | Planned | Largest surface area for developer tools — makes agent-lsp available to Copilot, Continue, and Cline users with zero CLI setup |
| **Observability** | Planned | Metrics (requests/sec, latency per tool, error rate) for production deployments — valuable for teams running agent-lsp as shared infrastructure |
