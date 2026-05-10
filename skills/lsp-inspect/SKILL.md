---
name: lsp-inspect
description: Full code quality audit for a file or package. Applies a check taxonomy (dead symbols, silent failures, error wrapping, coverage gaps, test coverage, doc drift, unrecovered panics, context propagation) using LSP-first strategies. Produces a severity-tiered findings report. Language-agnostic.
argument-hint: "<file-or-directory> [--checks <type1>,<type2>] [--json]"
user-invocable: true
allowed-tools: mcp__lsp__start_lsp mcp__lsp__open_document mcp__lsp__get_change_impact mcp__lsp__find_references mcp__lsp__list_symbols mcp__lsp__inspect_symbol mcp__lsp__get_diagnostics mcp__lsp__find_callers mcp__lsp__go_to_definition mcp__lsp__get_server_capabilities mcp__lsp__get_cross_repo_references
license: MIT
compatibility: Requires the agent-lsp MCP server (github.com/blackwell-systems/agent-lsp)
metadata:
  required-capabilities: documentSymbolProvider referencesProvider
  optional-capabilities: callHierarchyProvider
---

> Requires the agent-lsp MCP server.

# lsp-inspect

Full code quality audit for a file, package, or directory. Combines LSP batch
analysis (`get_change_impact`) with targeted per-symbol checks and LLM-driven
heuristic analysis. Produces a severity-tiered findings report with confidence
levels.

## When to Use

- Auditing a package before a release or major refactor
- Finding dead code, untested exports, and error handling gaps in unfamiliar code
- Reviewing code quality of an external codebase for contribution opportunities
- Pre-merge quality gate on a set of changed files

## Input

```
/lsp-inspect <target> [--checks <type1>,<type2>] [--json]
```

**Target** can be:
- A file path: `/lsp-inspect src/handlers/auth.go`
- A directory/package: `/lsp-inspect internal/runnables/`
- Multiple targets: `/lsp-inspect pkg/a.go pkg/b.go`

**Flags:**
- `--checks <type1>,<type2>`: only run listed check types (default: all applicable)
- `--json`: emit structured JSON instead of markdown

## Check Taxonomy

| Check | What it finds | LSP strategy |
|-------|--------------|--------------|
| `dead_symbol` | Exported symbol with zero references | Tier 1A: `get_change_impact` batch; Tier 1B: `find_references` per-symbol |
| `test_coverage` | Exported symbol with no test callers | Tier 1A: `get_change_impact` test_callers field |
| `silent_failure` | Error/exception suppressed without re-raise or logging | Read code, identify bare `except:`, empty `if err != nil {}`, swallowed returns |
| `error_wrapping` | Error returned/raised without context | Read code, identify `return err` without `fmt.Errorf` wrapping or `raise` without `from` |
| `coverage_gap` | Unhandled input, error path, or code branch | Read code, identify switch/match without default, unchecked type assertions |
| `doc_drift` | Docstring/comment that doesn't match the actual signature | Compare `inspect_symbol` hover text against source |
| `panic_not_recovered` | Unhandled crash in a goroutine or async context | Read code, identify `go func()` without recover, unguarded `.unwrap()` |
| `context_propagation` | Function receives context but creates a fresh root for callees | Read code, identify `context.Background()` in functions with `ctx` parameter |

## Execution

### Step 0: Initialize and verify workspace

```
mcp__lsp__start_lsp(root_dir="<repo_root>")
```

Open one file per package being audited:

```
mcp__lsp__open_document(file_path="<target_file>", language_id="<lang>")
```

**Warm-up check (mandatory):** Pick one symbol you know is actively used.
Call `find_references` on it. If it returns `[]`, wait 3-5 seconds and retry.
Do not proceed until a known-active symbol returns >= 1 reference.

### Step 1: Batch analysis (Tier 1A)

Call `get_change_impact` once per file in the target:

```
mcp__lsp__get_change_impact(changed_files=["/abs/path/file.go"], include_transitive=false)
```

This returns all exported symbols with:
- `non_test_callers`: count of production code references
- `test_callers`: count of test file references

Classify immediately:
- `non_test_callers == 0 AND test_callers == 0` -> dead symbol candidate
- `non_test_callers == 0 AND test_callers > 0` -> test-only (may be dead)
- `non_test_callers > 0 AND test_callers == 0` -> untested export

If `get_change_impact` fails or is unavailable, fall back to Tier 1B
(`find_references` per-symbol) for `dead_symbol` checks.

### Step 2: Heuristic checks (LLM-driven)

Read the source code of each file (use offset/limit for files over 500 lines).
Apply the following checks by reading and reasoning about the code:

**silent_failure:** Look for:
- Go: `if err != nil { return }` (no error returned), bare `_ = fn()`
- Python: bare `except:` or `except Exception: pass`
- TypeScript: empty `.catch(() => {})`, `try {} catch(e) {}`
- Rust: `.unwrap_or_default()` on fallible ops that should propagate

**error_wrapping:** Look for:
- Go: `return err` without `fmt.Errorf("context: %w", err)`
- Python: `raise ValueError(str(e))` without `from e`
- TypeScript: `throw e` without wrapping in a contextual error

**coverage_gap:** Look for:
- Switch/match without exhaustive cases or default branch
- Unchecked type assertions (`v := x.(Type)` vs `v, ok := x.(Type)`)
- Missing nil/null checks before dereference after fallible calls

**doc_drift:** For exported functions, compare:
- Parameter names in docstring vs actual signature
- Return type described in doc vs actual return
- Use `inspect_symbol` hover text to cross-reference

**panic_not_recovered:** Look for:
- Go: `go func() { ... }()` without `defer recover()`
- Rust: `.unwrap()` or `.expect()` in non-test, non-main code
- Python: bare thread creation without exception handling

**context_propagation:** Look for:
- Functions that accept `ctx context.Context` but call `context.Background()` or `context.TODO()` internally

### Step 3: Cross-check and classify

For each finding, assign:

**Severity:**
- `error`: Will cause runtime failure, data loss, or resource leak
- `warning`: May cause confusion, maintenance burden, or subtle bugs
- `info`: Style issue or improvement opportunity

**Confidence:**
- `high`: LSP-verified (Tier 1A/1B) or unambiguous code pattern
- `medium`: Heuristic match with possible false positive
- `low`: Grep-based or uncertain pattern match

### Step 4: Output

Produce the findings report:

```markdown
## Inspection Report: <target>

**Files analyzed:** N
**Checks applied:** [list]
**Findings:** E errors, W warnings, I info

### Errors

| # | Check | File:Line | Finding | Confidence |
|---|-------|-----------|---------|------------|
| 1 | dead_symbol | pkg/foo.go:42 | `UnusedHelper` has 0 references | high (LSP) |

### Warnings

| # | Check | File:Line | Finding | Confidence |
|---|-------|-----------|---------|------------|
| 1 | error_wrapping | pkg/bar.go:88 | `return err` without context wrapping | high |
| 2 | test_coverage | pkg/foo.go:15 | `ProcessInput` has 0 test callers | high (LSP) |

### Info

| # | Check | File:Line | Finding | Confidence |
|---|-------|-----------|---------|------------|
| 1 | doc_drift | pkg/foo.go:20 | Docstring mentions `timeout` param, signature has `deadline` | medium |
```

When `--json` is passed, emit structured JSON with the same fields.

## Caveats

1. **Re-exports:** Symbols in `__init__.py` (Python), `index.ts` (TypeScript),
   or public API surface files may appear dead locally but are consumed externally.
   Check `__all__`, barrel exports, and package-level re-exports before classifying.

2. **Registration patterns:** Symbols passed as values to framework registrars
   (HTTP handlers, plugin hooks) show zero LSP references. Grep wiring files
   before confirming dead.

3. **Library public API:** If the target is a library consumed by external repos,
   zero internal references doesn't mean dead. Use `--consumer-repos` or note
   as "library export, verify externally."

4. **Heuristic checks are advisory.** Silent failure and error wrapping checks
   depend on LLM reasoning about intent. False positives are expected; always
   review before acting on findings.

5. **Large files:** For files over 500 lines, read targeted sections (use
   offset/limit). Do not read entire large files into context.
