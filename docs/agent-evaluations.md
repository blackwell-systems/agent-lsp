# Agent Self-Evaluations

Three independent AI agents evaluated agent-lsp across 10 coding tasks. Each agent was given the same prompt, worked through the tasks using agent-lsp's MCP tools, and wrote an honest assessment. No instructions were given to be positive; agents were told "if a tool fails or is slower than grep, say so."

## Evaluation Prompt

```
Perform the following 10 tasks on this codebase, using agent-lsp MCP tools
where helpful and your built-in tools where they're a better fit.

1. Find all exported functions in internal/notify/hub.go and list their callers
2. Rename a local variable in a test function and verify no errors were introduced
3. Determine the blast radius of changing HandleStartLsp's signature
4. Find dead code in internal/tools/helpers.go
5. Explore the LSPClient type: what it does, what calls it, what it calls
6. Preview an edit that introduces a type error, without touching the file
7. Run the test suite for the internal/notify package only
8. Find all implementations of the NotificationSender interface
9. Format a file and show the result
10. Get a structural overview of cmd/agent-lsp/server.go

For each task, note which tools you used, whether agent-lsp saved steps, any
tools that didn't work as expected, and any tools you wished existed. Then
write a one-paragraph summary.
```

---

## Evaluation 1: Claude Opus 4.6 (via Claude Code)

**Date:** 2026-05-10

### Per-task results

| Task | Primary Tools | agent-lsp saved steps? | Issues |
|------|--------------|----------------------|--------|
| 1. Exported functions + callers | `get_change_impact` | Yes, significantly (1 call vs 6+ greps) | None |
| 2. Rename local variable | `rename_symbol` (dry_run) + Edit | Yes for planning; `apply_edit` denied | Permission denial forced fallback |
| 3. Blast radius | `get_change_impact` | Yes, significantly | None |
| 4. Dead code | `get_change_impact` + Grep | Partial (exported only) | No single tool for unexported dead code |
| 5. Explore LSPClient | Grep/Read (fallback) | N/A (tools denied) | `find_callers`, `inspect_symbol`, `find_references` all denied |
| 6. Preview type error | simulation session | Yes, unique capability | `destroy_session` denied (minor) |
| 7. Run tests | `run_tests` | Marginal vs bash | Path format issue on first try |
| 8. Interface implementations | `go_to_implementation` | Yes, definitively | None |
| 9. Format file | `format_document` | Equivalent to CLI | None |
| 10. Structural overview | Grep/Read (fallback) | N/A (`list_symbols` denied) | Permission denial |

### Tools wished for
- Combined "dead code" check for both exported and unexported symbols
- "Symbol outline" grouped by receiver type

### Summary

> "As a coding agent, I would recommend agent-lsp for any workflow involving refactoring, impact analysis, or safe editing. The standout tools are `get_change_impact` (blast radius in one call, with test/non-test partitioning that would take 5-10 grep commands to replicate), `go_to_implementation` (type-checked interface satisfaction that grep simply cannot do), `rename_symbol` with dry_run (precise scope-aware rename preview), and the simulation session workflow (speculative type-checking without touching disk, which has no grep/read equivalent at all). For simple text searches, pattern matching, or reading files, the built-in tools are faster and more direct. The sweet spot is clear: use agent-lsp for semantic operations (references, implementations, impact, simulation) and built-in tools for syntactic operations (pattern search, file reading)."

---

## Evaluation 2: Cursor (auto mode)

**Date:** 2026-05-10

### Per-task results

| Task | Primary Tools | agent-lsp saved steps? | Issues |
|------|--------------|----------------------|--------|
| 1. Exported functions + callers | `get_change_impact`, `find_references` | Yes, callers via references beats manual grep | Getting exact (line,column) matters |
| 2. Rename local variable | `rename_symbol` + `apply_edit` + `run_tests` | Big win, safe rename without manual edits | Built-in Read blocked in environment |
| 3. Blast radius | `inspect_symbol`, `find_callers`, `find_references` | Yes, fast and precise | None |
| 4. Dead code | `find_references`, `list_symbols` | Moderate, LSP more reliable than grep | For unexported symbols, grep still needed |
| 5. Explore LSPClient | `get_symbol_source`, `list_symbols`, `find_callers` | Yes, call hierarchy faster than manual reads | Call hierarchy doesn't work on types |
| 6. Preview type error | `preview_edit` | Huge, safe what-if diagnostics | Transient baseline diagnostic in snapshot |
| 7. Run tests | `run_tests` | Yes, structured output | Path needs `./internal/notify` not `internal/notify` |
| 8. Interface implementations | `go_to_implementation` | Very big, painful with grep | None |
| 9. Format file | `format_document` + `apply_edit` | Mixed, finding dirty files still needs shell | None |
| 10. Structural overview | `list_symbols` | Yes, instant outline | None |

### Tools wished for
- "Format these files" across a set (bulk format discovery)
- "Give me callers for all exported symbols in this file" (was manually looping)

### Summary

> "As a coding agent, I would recommend agent-lsp for Go-heavy refactors and code navigation because the rename, references, implementations, call hierarchy, and simulation tools remove a lot of brittle grep/manual-edit work and make changes safer; however, I'd caveat that some workflows still need shell assist (like discovering which files need formatting, or package-path quirks for go test), and call-hierarchy is strictly for callables (types require reference-based reasoning)."

---

## Evaluation 3: GPT-5.5 (via Codex CLI)

**Date:** 2026-05-10

### Per-task results

| Task | Primary Tools | agent-lsp saved steps? | Issues |
|------|--------------|----------------------|--------|
| 1. Exported functions + callers | `get_change_impact`, `find_callers` | Yes, real steps saved | None |
| 2. Rename local variable | `rename_symbol` + `preview_edit` | Better than text replace, exact local edits | None |
| 3. Blast radius | `get_change_impact` | Yes, file-level impact in one call | None |
| 4. Dead code | `get_change_impact`, `find_references` | Mixed, `safe_delete_symbol` failed on unexported | `safe_delete_symbol` returned "no identifier found" for `appendHint` |
| 5. Explore LSPClient | `inspect_symbol`, `list_symbols`, `find_callers` | Yes, broad usage visible quickly | None |
| 6. Preview type error | `preview_edit` | Genuinely useful, 6 compiler errors without disk write | None |
| 7. Run tests | `run_tests` | Convenient, normalized pass/fail | JSON output verbose |
| 8. Interface implementations | `go_to_implementation` | Fast and precise | None |
| 9. Format file | `format_document` | Worked but no edits needed | gofmt clearer for final formatting |
| 10. Structural overview | `list_symbols` | Yes, instant structure | None |

### Tools wished for
- Single "file exported functions and methods with callers" tool (get_change_impact skipped methods)

### Summary

> "As a coding agent, I would recommend agent-lsp for symbol-aware navigation, blast-radius checks, rename previews, implementation lookup, and speculative diagnostics because those tasks collapse from many grep/read/reference loops into one or two precise calls. I would still keep grep, nl, git diff, and native format/test commands close by for non-code text, showing exact file context, and cases where an LSP wrapper is too verbose or misses a method-level detail."

---

## Common findings across all three evaluations

**Universally praised:**
- `get_change_impact` (blast radius in one call)
- `go_to_implementation` (type-checked interface satisfaction)
- `preview_edit` / simulation sessions (speculative diagnostics without disk writes)
- `rename_symbol` with dry_run (safe rename preview)

**Universally noted limitations:**
- Call hierarchy works on functions/methods only, not types
- `run_tests` path format requires `./` prefix for Go
- Formatting discovery (which files need formatting) still needs shell
- Unexported symbol dead code detection not covered by `get_change_impact`

**Friction points addressed after evaluations:**
- `get_change_impact` promoted to IMPORTANT in Instructions
- `get_change_impact` now includes exported methods (receiver prefix fix)
- `get_change_impact` now includes per-symbol test_callers (not just flat list)
- `find_callers` description clarified (functions/methods only)
- `find_references` and `inspect_symbol` support `position_pattern` without line/column
- `preview_edit` net_delta no longer counts hints (severity 3/4 filtered)
- `destroy_session` after `preview_edit` returns success instead of error
- `format_document` description clarified (single-file scope)
- Go test path auto-fix shipped (bare paths now auto-prefixed)

---

## Unprompted project assessment (GPT-5.5, fourth evaluation)

After completing the 10 tasks, GPT-5.5 volunteered this unprompted assessment of the project's positioning:

> "Most 'coding agent' tooling wraps shell commands, grep, tree-sitter, or editor-style LSP access in a loose way. This project is more opinionated: it exposes LSP operations as agent-native tools for blast-radius analysis, safe edits, preview diagnostics, call hierarchy, implementations, formatting, tests, and workspace simulation. The stronger idea is not just 'an agent can call LSP,' but 'an agent gets refactoring-grade workflows as structured MCP tools.'"
>
> "The parts that feel most distinctive are: get_change_impact (one-shot exported symbol and caller partitioning is very agent-oriented), preview_edit (speculative type-checking before touching disk is exactly the kind of guardrail coding agents need), safe_delete_symbol/rename_symbol/format_document/get_diagnostics (these turn common risky edits into structured workflows), and the evaluation angle (the project is clearly testing whether semantic tools actually beat grep/read loops, which is the right question)."
>
> "It's not unique in the sense that LSP, MCP, code intelligence, and refactoring tools all exist separately. But combining them into an agent-facing MCP server with safety-oriented workflows is meaningfully differentiated. The main challenge is polish: summaries, latency, noisy outputs, and trustworthiness need to be excellent for agents to prefer it over fast shell tools."
