# Roadmap

## Distribution

| Feature | Status | Description |
|---------|--------|-------------|
| **Prebuilt binaries** | Done (v0.1.0) | GoReleaser publishing `.tar.gz`/`.zip` binaries for Linux, macOS, and Windows to GitHub Releases — eliminates the `go install` requirement for non-Go developers |
| **`agent-lsp init`** | Done (v0.1.0) | Interactive setup wizard: detects installed language servers, asks which AI tool you use, writes the correct MCP config — turns manual setup into one command |
| **Homebrew tap** | Done (v0.1.2) | `brew install blackwell-systems/tap/agent-lsp` — formula auto-updated by GoReleaser on every release via `brews:` config |
| **`curl \| sh` installer** | Done (v0.1.1) | `curl -fsSL .../install.sh \| sh` — detects OS/arch, finds the right asset from GitHub Releases API, installs to `/usr/local/bin` |
| **npm wrapper** | Done (v0.1.2) | `npm install -g @blackwell-systems/agent-lsp` — optionalDependencies pattern; platform binary auto-selected at install time |
| **MCP registry listings** | Done (v0.1.2) | Published to official MCP Registry (`io.github.blackwell-systems/agent-lsp`); auto-published via GitHub OIDC in CI; smithery.yaml for Glama/Smithery indexing; mcpservers.org listed |
| **Docker Hub mirroring** | Done (v0.1.2) | All images mirrored to Docker Hub automatically on every release alongside GHCR |
| **Windows install script** | Planned | PowerShell install script + Scoop/Chocolatey package — GoReleaser ships Windows binaries but there's no Windows install story |
| **Nix flake** | Planned | `nix run github:blackwell-systems/agent-lsp` — Nix users expect it; large overlap with the developer audience |

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

## Tools

| Feature | Status | Description |
|---------|--------|-------------|
| **Rename with glob exclusions** | Planned | `rename_symbol` accepts glob patterns to exclude files from the rename — useful for generated code, vendored files, and test fixtures that should not be updated |
| **LineScope for position_pattern** | Planned | Restrict a `position_pattern` match to a specific line range — eliminates false matches when the same token appears multiple times in a file |

## Transport

| Feature | Status | Description |
|---------|--------|-------------|
| **HTTP/SSE transport** | Planned (v0.2) | Run agent-lsp as a persistent HTTP server; enables remote deployments, Docker without `-i`, and multi-client sessions sharing one warm index |
| **Language server health endpoint** | Planned | HTTP `/health` endpoint returning language server status — required for container orchestration and Docker-based production deployments |

## Product

| Feature | Status | Description |
|---------|--------|-------------|
| **`agent-lsp doctor`** | Planned | Diagnostic command: checks each configured language server starts correctly, reports version, lists supported capabilities — reduces "why isn't this working" setup friction |
| **`agent-lsp update`** | Planned | Self-update to the latest release — standard for CLI tools; fetches from GitHub Releases and replaces the binary in-place |
| **Config file format** | Planned | `~/.agent-lsp.json` or `agent-lsp.json` project file for complex setups with per-server options — currently arg-based only |
| **Continue.dev config support** | Planned | `agent-lsp init` currently skips Continue.dev; it uses a different config format than `mcpServers` and is a popular AI coding extension |

## Skills

Skills encode correct tool sequences so workflows actually happen. The current 14 skills cover navigation, safety checking, and analysis. Three gaps remain.

### Code action skills (gap: `get_code_actions` + `execute_command` are underserved)

| Skill | Description |
|-------|-------------|
| `/lsp-extract-function` | Extract a selected code block into a named function using LSP code actions |
| `/lsp-fix-all` | Get diagnostics → apply available code action fixes for each error → verify |
| `/lsp-generate` | Trigger server-side code generation (implement interface, generate test stubs, add missing methods) |

### Full edit lifecycle skill

| Skill | Description |
|-------|-------------|
| `/lsp-refactor` | Meta-skill: impact check → speculative preview → apply → verify → run affected tests. Takes an intent and sequences the full workflow end-to-end |

### Understanding skill

| Skill | Description |
|-------|-------------|
| `/lsp-explore` | "Tell me about this symbol": hover + implementations + call hierarchy + references in one pass — for navigating unfamiliar code |

### Skill composition

Skills calling other skills — `/lsp-refactor` composed from `/lsp-impact` + `/lsp-safe-edit` + `/lsp-verify` + `/lsp-test-correlation`. Requires runtime support for skill invocation from within a skill.

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
