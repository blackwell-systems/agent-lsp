# agent-lsp Skill Coverage Audit

**Note:** This audit was conducted when 14 skills existed. The project now has 21 skills. The 7 additional skills (lsp-explore, lsp-extract-function, lsp-fix-all, lsp-generate, lsp-inspect, lsp-refactor, lsp-understand) are not covered here.

**Date:** 2026-04-09 (updated 2026-04-09 post-tools-sprint)  
**Scope:** All 50 agent-lsp tools vs. 14 skills (10 at audit time, 4 added across sprints)  
**Goal:** Identify coverage gaps and recommend new skill candidates

---

## Executive Summary

- **Total tools:** 50
- **Skills at audit:** 10 → **14 after sprints** (`/lsp-cross-repo`, `/lsp-local-symbols`, `/lsp-test-correlation`, `/lsp-format-code` added)
- **Coverage:** ~29 tools directly surfaced through skills (~58% fully covered; ~72% including partial)
- **Partial coverage:** 7 tools mentioned but not as primary workflow drivers
- **Uncovered:** ~10 tools (down from 14 at audit time)

**Sprint completed (P0):** `/lsp-cross-repo` shipped, `/lsp-local-symbols` shipped, `/lsp-rename` `prepare_rename` gate added, `/lsp-safe-edit` enhanced with `simulate_edit_atomic` pre-flight + `get_code_actions` on errors + multi-file protocol.

**Sprint completed (P1):** `/lsp-verify` `get_tests_for_file` pre-step added, `/lsp-test-correlation` skill shipped, `/lsp-format-code` skill shipped, `format_document` folded into `/lsp-safe-edit` (post-edit) and `/lsp-verify` (post-verification).

**Sprint completed (tools):** `get_change_impact` handler added and wired to `/lsp-impact` (Step 0 file-level entry), `get_cross_repo_references` handler added and wired to `/lsp-cross-repo` (Step 3 primary lookup), `simulate_chain` promoted from partial to fully covered via `/lsp-safe-edit` Step 3b refactor preview gate.

**Remaining key gaps:** `go_to_type_definition` (no type introspection skill), `get_signature_help` (no parameter discovery skill).

---

## 1. Coverage Matrix

### Fully Covered (Primary Tool or Skill)

| Tool | Skill(s) | Role |
|------|----------|------|
| `start_lsp` | `lsp-rename`, `lsp-impact`, `lsp-safe-edit`, `lsp-simulate`, `lsp-implement`, `lsp-docs`, `lsp-edit-export`, `lsp-dead-code`, `lsp-verify` | Session initialization (prerequisite in all skills) |
| `open_document` | `lsp-safe-edit`, `lsp-dead-code`, `lsp-implement` | File registration |
| `get_diagnostics` | `lsp-safe-edit`, `lsp-rename`, `lsp-edit-export`, `lsp-verify` | Safety gate, before/after comparison |
| `get_info_on_location` | `lsp-docs` (Tier 1) | Documentation lookup, hover info |
| `get_references` | `lsp-rename`, `lsp-impact`, `lsp-dead-code`, `lsp-edit-export` | Caller enumeration, reference count |
| `go_to_definition` | `lsp-docs` (Tier 3) | Symbol location |
| `go_to_symbol` | `lsp-rename`, `lsp-impact`, `lsp-edit-export`, `lsp-implement` | Symbol lookup by name |
| `rename_symbol` | `lsp-rename` | Workspace-wide rename |
| `apply_edit` | `lsp-rename`, `lsp-edit-symbol`, `lsp-verify` | Edit application |
| `get_code_actions` | `lsp-verify` | Quick fixes and refactorings |
| `call_hierarchy` | `lsp-impact` | Caller/callee analysis |
| `type_hierarchy` | `lsp-impact`, `lsp-implement` | Type inheritance chains |
| `go_to_implementation` | `lsp-implement` | Interface implementation lookup |
| `get_document_symbols` | `lsp-edit-symbol`, `lsp-dead-code` | Symbol enumeration in a file |
| `create_simulation_session` | `lsp-simulate` | Session creation |
| `simulate_edit` | `lsp-simulate` | In-memory edit application |
| `evaluate_session` | `lsp-simulate` | Diagnostic evaluation |
| `commit_session` | `lsp-simulate` | Patch application |
| `discard_session` | `lsp-simulate` | Session revert |
| `destroy_session` | `lsp-simulate` | Session cleanup |
| `simulate_edit_atomic` | `lsp-simulate` | One-shot simulation |
| `run_build` | `lsp-verify`, `lsp-edit-export` | Build verification |
| `run_tests` | `lsp-verify` | Test execution |
| `get_symbol_source` | `lsp-docs` (Tier 3) | Source text extraction |
| `get_symbol_documentation` | `lsp-docs` (Tier 2) | Offline toolchain doc lookup |
| `simulate_chain` | `lsp-safe-edit` | Refactor/rename preview gate (Step 3b) — chain edits, check `cumulative_delta`, commit or discard |
| `get_change_impact` | `lsp-impact` | File-level blast-radius analysis (Step 0) — exports + test/non-test caller partition in one call |
| `get_cross_repo_references` | `lsp-cross-repo` | Cross-repo reference lookup (Step 3) — adds consumer roots, partitions by repo, returns warnings |
| `get_tests_for_file` | `lsp-test-correlation`, `lsp-verify` | Source-to-test file mapping; used as pre-step in verify and primary lookup in test-correlation |

**Count:** 29 tools fully covered.

---

### Partial Coverage (Mentioned but not Primary Driver)

| Tool | Mentioned in | Context | Gap |
|------|--------------|---------|-----|
| `prepare_rename` | ~~(none)~~ → `lsp-rename` **(shipped)** | Safety gate before rename attempt | **Fixed:** `prepare_rename` is now Step 2 in `/lsp-rename`, before reference enumeration — validates position is renameable, catches built-ins and external packages |
| `get_workspace_symbols` | `lsp-edit-symbol` | Symbol search by name | Mentioned only for Step 1; not the primary skill purpose |
| ~~`format_document`~~ | ~~(none)~~ → `lsp-format-code`, `lsp-safe-edit`, `lsp-verify` **(shipped)** | Document formatting | **Fixed:** standalone `/lsp-format-code` skill + folded into `/lsp-safe-edit` post-edit and `/lsp-verify` post-verification |
| ~~`format_range`~~ | ~~(none)~~ → `lsp-format-code` **(shipped)** | Range formatting | **Fixed:** covered by `/lsp-format-code` standalone skill |
| `get_completions` | (none) | Code completion | No skill drives agent toward this |
| `get_signature_help` | (none) | Function signature info | No skill drives agent toward this |
| `get_server_capabilities` | `lsp-impact`, `lsp-implement` | Capability checking | Used as optional prerequisite, not primary workflow driver |

**Count:** 7 tools partially covered.

---

### Uncovered (No Skill Drives Agent Toward These)

| Tool | Type | Agent Workflow Gap | Notes |
|------|------|-------------------|-------|
| `close_document` | Session lifecycle | Agent doesn't know when to close files | Cleanup primitive, but no skill orchestrates batch closing after analysis |
| ~~`get_document_highlights`~~ | Navigation | **(shipped)** | Covered by `/lsp-local-symbols` — primary workflow driver for file-scoped symbol search |
| `go_to_type_definition` | Navigation | Jump to type (distinct from definition) | No skill demonstrates when/why to prefer over `go_to_definition` |
| `go_to_declaration` | Navigation | C/C++ header lookup (declaration vs. definition) | Rarely needed; no driver skill |
| ~~`add_workspace_folder`~~ | Workspace multi-root | **(shipped)** | Covered by `/lsp-cross-repo` |
| `remove_workspace_folder` | Workspace multi-root | Clean up workspace roots | Primitive; no skill drives agent toward this |
| ~~`list_workspace_folders`~~ | Workspace multi-root | **(shipped)** | Covered by `/lsp-cross-repo` (indexing verification step) |
| `did_change_watched_files` | Utilities | Notify server of external file changes | Only needed in edge cases (external processes); no skill demonstrates this |
| `detect_lsp_servers` | Utilities | Scan workspace and suggest config | Project setup tool; not a runtime workflow |
| `get_inlay_hints` | Analysis | Inline type/parameter hints | No skill demonstrates use case |
| `restart_lsp_server` | Session lifecycle | Restart server after crash or config change | Primitive; no skill demonstrates failure recovery workflow |
| `execute_command` | Utilities | Run server-side command (e.g., from code action) | No skill demonstrates when to call this beyond `lsp-verify` code action flow |
| `set_log_level` | Utilities | Change runtime log verbosity | Diagnostic tool; no workflow skill |
| ~~`get_tests_for_file`~~ | Analysis | **(shipped)** | Covered by `/lsp-test-correlation` (primary lookup) and `/lsp-verify` (pre-step) |

**Count:** 10 tools uncovered.

---

## 2. Uncovered Tool Gaps: Workflow Analysis

### Per-Tool Gap Assessment

#### `close_document`
**Agent workflow?** Maybe — but rare.  
**Gap analysis:** Agents don't automatically know when to close files. In a long session analyzing many files, an agent would benefit from a skill that batches `close_document` calls after analysis complete, freeing server memory. This is more of a resource-management hygiene issue than a creative workflow.  
**Recommendation:** Not a skill candidate (too low-level). Document in `lsp-safe-edit` / `lsp-impact` as optional cleanup step.

#### `get_document_highlights`
**Agent workflow?** Yes — "find all references but fast and local-only."  
**Gap analysis:** `get_document_highlights` returns read/write/text occurrences of a symbol within a single file, and is instant (no cross-file search). This is faster than `get_references` for analyzing local usage. No skill demonstrates when to prefer local analysis. An agent iterating on code in one file might reach for this naturally if primed: *"Show me everywhere this variable is used in this file."*  
**Recommendation:** New skill `/lsp-local-symbols` (see New Skill Candidates, below).

#### `go_to_type_definition`
**Agent workflow?** Yes — "where is the type of this variable defined?"  
**Gap analysis:** Distinct from `go_to_definition`. When hovering over `var x MyType`, `go_to_definition` takes you to the `var x` declaration; `go_to_type_definition` takes you to the `type MyType` definition. In codebases with typedef layers or complex type aliases, this is the right first step. No skill demonstrates this workflow.  
**Recommendation:** Could be part of enhanced `/lsp-docs` (Tier 1 should include type navigation). Or a small focused skill `/lsp-type-info`.

#### `go_to_declaration`
**Agent workflow?** Niche — C/C++ headers mainly.  
**Gap analysis:** Separates declaration (e.g., `int foo();` in `.h`) from definition (e.g., `int foo() { ... }` in `.c`). Other languages treat these as the same. Weak agent workflow.  
**Recommendation:** Not a skill candidate. Document in README as language-specific.

#### `add_workspace_folder` / `remove_workspace_folder` / `list_workspace_folders`
**Agent workflow?** Yes — "I need to analyze code across a library and its consumer."  
**Gap analysis:** `add_workspace_folder` enables cross-repo analysis (library + app), but agents have no skill that sets this up. Typical workflow: start LSP on library, add app folder, then run `get_references` on a library symbol to find all call sites in the app. Currently, no skill orchestrates this.  
**Recommendation:** New skill `/lsp-cross-repo` (see New Skill Candidates, below).

#### `did_change_watched_files`
**Agent workflow?** Edge case only.  
**Gap analysis:** Only needed when external processes modify files and agent-lsp's file watcher hasn't picked up the change yet. Very rare in agent workflows (most edits are made by the agent via LSP).  
**Recommendation:** Not a skill candidate. Document in tools.md as edge case.

#### `detect_lsp_servers`
**Agent workflow?** Yes, but one-time setup.  
**Gap analysis:** Scans a workspace and suggests `agent-lsp.json` config. Useful for project onboarding, but not a recurring runtime workflow. This is a setup/initialization tool.  
**Recommendation:** Document in README as setup helper. Not a skill candidate (doesn't fit the "agent reaches for this during development" pattern).

#### `get_inlay_hints`
**Agent workflow?** Maybe — "show me all inferred type annotations in this range."  
**Gap analysis:** Useful for understanding complex code with implicit types, but not a common agent need. Most agents use `get_info_on_location` (hover) on specific symbols. Inlay hints are an IDE feature for visualization, less useful in agent workflows.  
**Recommendation:** Not a strong skill candidate. Could be part of enhanced `/lsp-docs` for "show me all types in this function."

#### `restart_lsp_server`
**Agent workflow?** Recovery path only.  
**Gap analysis:** Agents don't proactively restart servers. This is a fallback when diagnostics become stale or the server becomes unresponsive. Could be part of a larger "health check and recovery" skill.  
**Recommendation:** Document as troubleshooting step in each skill's "If things go wrong" section. Not a standalone skill.

#### `execute_command`
**Agent workflow?** Yes, but narrow.  
**Gap analysis:** Runs server-side commands returned by `get_code_actions`. `lsp-verify` mentions it for code action application. But `execute_command` is rarely the primary workflow driver — it's always a response to something else (code action).  
**Recommendation:** Covered implicitly by `lsp-verify`. No new skill needed. Document as "how to apply code actions" in `lsp-verify` enhancement.

#### `set_log_level`
**Agent workflow?** Debugging only.  
**Gap analysis:** Useful for diagnostic purposes (turn on debug logging when things go wrong), but not a creative workflow.  
**Recommendation:** Not a skill candidate. Document in troubleshooting guide.

#### `get_tests_for_file` ✅ SHIPPED
**Agent workflow?** Yes — "show me the tests for this file so I can run them after editing."  
**Gap analysis:** Bridges source file → test file mapping. Now covered by `/lsp-test-correlation` (primary lookup step) and `/lsp-verify` (pre-step in Layer 3 output).  
**Recommendation:** No action needed — fully covered.

---

## 3. Existing Skill Enhancement Opportunities

### `/lsp-rename` ✅ SHIPPED

**Current tools:** `go_to_symbol`, `prepare_rename`, `get_references`, `rename_symbol`, `apply_edit`, `get_diagnostics`

**Completed:**
1. **`prepare_rename` added as Step 2** (after symbol locate, before reference enumeration): validates rename is possible, fails fast with actionable error for built-ins/keywords/external packages.
   ```
   prepare_rename(file_path, line, column)
      → returns range and placeholder name, or error
   ```
   This is a safety gate.

2. **Add `get_server_capabilities` as prerequisite**: Warn if `rename_symbol` is not in `supported_tools` before attempting rename. Avoids silent failure.

3. **Document fuzzy position fallback**: README says `rename_symbol` has position-pattern fallback; skill should document this and demonstrate use.

**Safety improvement:** Current workflow renames first, then checks diagnostics. `prepare_rename` adds pre-flight validation.

---

### `/lsp-safe-edit` ✅ SHIPPED

**Current tools:** `start_lsp`, `open_document`, `simulate_edit_atomic`, `get_diagnostics`, `get_code_actions`, `apply_edit`, Edit/Write

**Completed:**
1. **`simulate_edit_atomic` pre-flight (Step 3)** — runs before any disk write; `net_delta > 0` pauses and asks; multi-file: per-file independently, sum deltas; new files (Write) skip.
2. **`get_code_actions` on introduced errors (Step 7)** — surfaces quick fixes at each error location after a bad edit; `y/n/select` to apply via `apply_edit`, then re-diff.
3. **Multi-file workflow section** — explicit protocol: open all, BEFORE for all, simulate each, apply file-by-file (stop on failure), merge AFTER, code actions on any file with new errors.

---

### `/lsp-impact`
**Current tools:** `go_to_symbol`, `call_hierarchy`, `type_hierarchy`, `get_references`, `get_server_capabilities`

**Enhancements:**
1. **Add `get_inlay_hints` for type-heavy changes**: When impact report shows type changes, include inlay hints on affected range so agent understands type signature.

2. **Add `get_document_highlights` as quick local scan**: Before running workspace-wide `get_references`, run local highlights to understand how the symbol is used in its definition file. Gives agent early signal of scope.

3. **Document when `call_hierarchy` results differ from `get_references`**: These can produce different results (e.g., `call_hierarchy` shows semantic flow; `get_references` shows syntactic occurrence). Clarify in report.

**Context improvement:** More complete understanding of symbol usage patterns.

---

### `/lsp-simulate`
**Current tools:** All simulation tools (`create_simulation_session`, `simulate_edit`, etc.)

**Enhancements:**
1. **Add `get_server_capabilities` check**: Verify speculative execution is supported before attempting. Some servers don't support sessions; error gracefully.

2. **Promote `simulate_chain` higher in docs**: Currently buried in "Chained Mutations" section. This is valuable for multi-file refactors; should be Step 2 option.

3. **Add post-commit diagnostic check**: After `commit_session(apply: true)`, automatically call `get_diagnostics` on affected files to confirm no unexpected errors in the now-live code.

**Workflow improvement:** Better error handling and post-commit safety.

---

### `/lsp-edit-symbol`
**Current tools:** `get_workspace_symbols`, `go_to_definition`, `get_document_symbols`, `apply_edit`

**Enhancements:**
1. **Add `prepare_rename` workflow**: When editing a symbol's name, check if it can be renamed first.

2. **Add `get_diagnostics` after edit**: Capture baseline before, then verify after. Current skill doesn't guarantee safety.

3. **Include `get_server_capabilities` check**: Verify symbol resolution is supported before attempting `get_document_symbols`.

**Safety improvement:** Verify symbol is editable before applying changes.

---

### `/lsp-dead-code`
**Current tools:** `get_document_symbols`, `get_references`, `open_document`

**Enhancements:**
1. **Add `get_document_highlights` pre-flight**: For each exported symbol, run highlights in that file to understand local usage before checking workspace references.

2. **Add `call_hierarchy` for functions**: Include incoming callers from `call_hierarchy` alongside `get_references` for more complete picture of usage.

3. **Document test-file filtering**: Current skill mentions test-only references but doesn't orchestrate the filtering. Add explicit step to identify `_test.go` / `.test.*` files and exclude from dead-code count.

**Completeness improvement:** More nuanced dead-code detection.

---

### `/lsp-edit-export`
**Current tools:** `go_to_symbol`, `open_document`, `get_references`, `get_diagnostics`, `run_build`, Edit/Write

**Enhancements:**
1. **Add `prepare_rename` if renaming**: If the edit is a rename, validate first.

2. **Add `call_hierarchy` for functions**: Get semantic call graph, not just syntactic references.

3. **Add `get_server_capabilities` validation**: Check what's supported before attempting high-level operations.

4. **Add `simulate_edit_atomic` option**: For high-risk exports (10+ callers), offer to simulate first.

**Safety improvement:** More conservative approach for high-risk changes.

---

### `/lsp-verify`
**Current tools:** `get_diagnostics`, `run_build`, `run_tests`, `get_code_actions`, `apply_edit`

**Enhancements:**
1. **Add `get_tests_for_file` integration**: After running `run_tests`, highlight which tests are for the changed files. Helps prioritize test failures.

2. **Add `simulate_edit_atomic` dry-run**: Before `get_diagnostics`, optionally simulate first to catch errors pre-emptively.

3. **Batch `get_code_actions` application**: When multiple errors exist, group code actions and apply in waves, re-running diagnostics between waves.

**Efficiency improvement:** Faster error remediation.

---

### `/lsp-implement`
**Current tools:** `start_lsp`, `get_server_capabilities`, `go_to_symbol`, `go_to_implementation`, `type_hierarchy`, `open_document`

**Enhancements:**
1. **Add `call_hierarchy` on implementations**: For each implementation found, run `call_hierarchy(direction: "incoming")` to show callers. Helps understand ripple effect of interface changes.

2. **Add `get_inlay_hints` on interface definition**: Show parameter/return types inline so agent understands the contract before deciding to change it.

**Context improvement:** Better understanding of implementation surface before changes.

---

### `/lsp-docs`
**Current tools:** `get_info_on_location`, `get_symbol_documentation`, `go_to_definition`, `get_symbol_source`

**Enhancements:**
1. **Add `go_to_type_definition` in Tier 1**: When looking up a symbol, also jump to its type definition. Include type info in hover result.

2. **Add `get_inlay_hints` for functions**: Extract and display inferred parameter/return types alongside documentation.

3. **Add `get_document_highlights` in Tier 3**: When showing source, also highlight all usages within that file.

4. **Promote `get_symbol_documentation` usage**: Current skill mentions it but doesn't fully exploit it for agent-friendly output. Add formatting recommendations.

**Context improvement:** Richer documentation with types and usage.

---

## 4. New Skill Candidates

### `/lsp-local-symbols` ✅ SHIPPED

**Trigger scenario:** "Show me all occurrences of this variable/symbol in the current file."

**Tools composed:**
- `go_to_symbol` or `get_info_on_location` → locate symbol
- `get_document_highlights` → all occurrences in file
- `get_document_symbols` → hierarchical context (is it in a function? class?)
- `get_signature_help` (optional) → if it's a function call, show signature

**Why an agent wouldn't discover this:** `get_document_highlights` is faster and more precise than `get_references` for local analysis, but no skill surfaces it. Agents default to `get_references` (workspace-wide) when sometimes they just need file-local results.

**Skill description:**
```
/lsp-local-symbols

Fast local symbol analysis — find all occurrences, reads, and writes of a symbol 
within a single file. Faster than workspace-wide reference search when you only 
need local context. Use when navigating code within a function or class.

Argument: symbol name
Optional: file path (if working in multiple files)
```

---

### `/lsp-cross-repo` ✅ SHIPPED

**Trigger scenario:** "I need to analyze how my library is used in my application. Add the app to the workspace and show me all callers."

**Tools composed:**
- `start_lsp` → initialize on library root
- `get_cross_repo_references` → add consumer repos, wait for indexing, partition results by repo (primary step, replaces manual add_workspace_folder + get_references)
- `go_to_implementation` (cross-repo) → find all type implementations in consumer
- `call_hierarchy` (cross-repo) → show callers in consumer of library functions

**Why an agent wouldn't discover this:** Multi-root workspace setup requires explicit orchestration. No skill demonstrates how to set up cross-repo analysis. Agents don't naturally know to call `add_workspace_folder` or how to verify indexing.

**Skill description:**
```
/lsp-cross-repo

Cross-repository analysis for library and consumer workflows. Sets up a multi-root 
workspace, verifies indexing, then finds all library symbol usages in the consumer 
repository. Use when refactoring a shared library and need to understand impact on 
all consumers.

Arguments: library_root, consumer_root
```

---

### `/lsp-test-correlation`
**Trigger scenario:** "I just edited this file. Which tests do I need to run?"

**Tools composed:**
- `start_lsp` → initialize workspace
- `get_tests_for_file` → find test files for source file
- `get_workspace_symbols` → enumerate tests in test files
- `run_tests` (optional) → run the correlated tests
- `get_diagnostics` (optional) → verify no test compilation errors

**Why an agent wouldn't discover this:** `get_tests_for_file` exists but is not exposed by any skill. Agents don't know to correlate source changes to test execution.

**Skill description:**
```
/lsp-test-correlation

Find and run tests for a source file. After editing a file, automatically discover 
and execute the corresponding tests to verify changes are correct. Supports batching 
multiple source files to identify all affected test suites.

Arguments: source_file_path(s)
Optional: run_tests=true to execute automatically
```

---

### `/lsp-format-code`
**Trigger scenario:** "Format this file / range according to the language server's rules."

**Tools composed:**
- `format_document` → full-file formatting
- `format_range` → selection formatting
- `apply_edit` → apply formatting changes
- `get_diagnostics` (optional) → verify no new errors after formatting

**Why an agent wouldn't discover this:** No skill surfaces formatting workflows. Agents might know about `prettier` or `gofmt` but not the LSP-driven equivalent.

**Skill description:**
```
/lsp-format-code

Format code according to language server rules. Supports full-document or 
range-based formatting. Use before committing to ensure consistent style across 
the codebase.

Arguments: file_path, optional range (start_line, end_line)
```

---

### `/lsp-type-info`
**Trigger scenario:** "What is the type of this variable? Show me the type definition."

**Tools composed:**
- `get_info_on_location` → hover for immediate type
- `go_to_type_definition` → jump to type definition
- `get_document_symbols` → find type in definition file
- `get_semantic_tokens` (optional) → classify tokens in type definition
- `get_inlay_hints` (optional) → show inferred types

**Why an agent wouldn't discover this:** `go_to_type_definition` is distinct from `go_to_definition` but no skill explains when to use it. Agents don't naturally distinguish between navigating to a variable vs. its type.

**Skill description:**
```
/lsp-type-info

Type introspection — jump to the definition of a symbol's type. Useful when 
understanding complex type aliases, generics, or typedef chains. Shows the full 
type definition and all methods/fields for the type.

Arguments: symbol location (file, line, column) or symbol name
```

---

### `/lsp-signature-help`
**Trigger scenario:** "What are the parameters for this function?"

**Tools composed:**
- `get_signature_help` → function signature and active parameter
- `get_info_on_location` (optional) → function documentation
- `get_completions` (optional) → if function is not yet called

**Why an agent wouldn't discover this:** `get_signature_help` is a narrow tool, and no skill frames it as a workflow. Agents might hover with `get_info_on_location` but won't know about dedicated signature help.

**Skill description:**
```
/lsp-signature-help

Show function signature and available overloads at a call site. Highlights the 
active parameter to guide correct argument order. Use when calling unfamiliar 
functions or exploring available overloads.

Arguments: file_path, line, column (cursor inside argument list)
```

---

## 5. Folding Opportunities (Skill Consolidation)

### 1. `/lsp-docs` and `/lsp-type-info` could merge
**Opportunity:** Both are about information lookup. `/lsp-docs` does Tier 1/2/3 doc lookup; `/lsp-type-info` does type navigation. Could be `/lsp-inspect` (broad symbol introspection).

**Downside:** User mental model becomes "introspect symbol" instead of "look up docs" or "understand type". Separate skills are more discoverable.

**Recommendation:** Keep separate; too different in intent.

---

### 2. `/lsp-impact`, `/lsp-dead-code`, and `/lsp-implement` share pattern
**Opportunity:** All use `go_to_symbol` → hierarchical analysis (`call_hierarchy`, `type_hierarchy`, `get_references`). Could abstract to `/lsp-analyze-symbol` (generic symbol analyzer).

**Downside:** Different outputs and decision gates. Merging loses specificity.

**Recommendation:** Keep separate. But consider a shared `/lsp-symbol-analysis-commons` utility (internal) for duplicate Step 1 logic.

---

### 3. `/lsp-safe-edit`, `/lsp-edit-symbol`, and `/lsp-edit-export` all edit
**Opportunity:** All apply edits and check diagnostics. Could consolidate to `/lsp-edit` with sub-modes.

**Downside:** `/lsp-safe-edit` is generic (user provides edit text); `/lsp-edit-symbol` locates symbol by name; `/lsp-edit-export` checks callers. Different workflows.

**Recommendation:** Keep separate. But document how they compose (e.g., "use `/lsp-edit-export` for high-risk, then `/lsp-safe-edit` to verify").

---

### 4. `/lsp-rename` and `/lsp-edit-export` share Phase 1
**Opportunity:** Both do "locate symbol" → "find callers" → "ask user permission". Could share workflow.

**Downside:** `/lsp-rename` is rename-specific (uses `rename_symbol`); `/lsp-edit-export` is general edit. Different tools.

**Recommendation:** `/lsp-rename` should probably call the confirmation gate from `/lsp-edit-export` pattern (both ask before proceeding).

---

## 6. Tool Grouping for New Organization

**Observation:** Tools cluster into agent-oriented workflows better than LSP service layers. Recommended organization for future skills:

### Workflow Groups

1. **Symbol Introspection** — `get_info_on_location`, `go_to_definition`, `go_to_type_definition`, `get_document_symbols`, `get_workspace_symbols`, `go_to_symbol`, `get_symbol_source`, `get_symbol_documentation`, `call_hierarchy`, `type_hierarchy`, `go_to_implementation`
   - Skills: `/lsp-docs`, `/lsp-type-info`, `/lsp-impact`, `/lsp-implement`
   - Gap: No skill for `go_to_declaration` or pure `get_workspace_symbols` search

2. **Safe Editing** — `apply_edit`, `prepare_rename`, `rename_symbol`, `format_document`, `format_range`, `get_code_actions`, `execute_command`
   - Skills: `/lsp-rename`, `/lsp-edit-symbol`, `/lsp-edit-export`, `/lsp-safe-edit`
   - Gap: No `/lsp-format-code`; no dedicated skill for code actions

3. **Analysis & Verification** — `get_diagnostics`, `run_build`, `run_tests`, `get_code_actions`, `get_server_capabilities`
   - Skills: `/lsp-verify`, `/lsp-dead-code`
   - Gap: No `/lsp-test-correlation` to connect source→tests

4. **Speculative Execution** — `create_simulation_session`, `simulate_edit`, `simulate_edit_atomic`, `simulate_chain`, `evaluate_session`, `commit_session`, `discard_session`, `destroy_session`
   - Skills: `/lsp-simulate`
   - Gap: None (fully covered)

5. **Navigation & Reference** — `get_references`, `get_document_highlights`, `open_document`, `close_document`, `get_completions`, `get_signature_help`, `get_inlay_hints`
   - Skills: None dedicated (mentioned in skills but not primary driver)
   - Gap: No `/lsp-local-symbols`, `/lsp-signature-help`

6. **Workspace & Session** — `start_lsp`, `restart_lsp_server`, `add_workspace_folder`, `remove_workspace_folder`, `list_workspace_folders`, `did_change_watched_files`, `detect_lsp_servers`, `set_log_level`, `get_tests_for_file`
   - Skills: None dedicated
   - Gap: No `/lsp-cross-repo`, `/lsp-test-correlation`

---

## 7. Recommended Actions (Priority Order)

### Immediate (P0) — ✅ ALL COMPLETE

1. ✅ **Enhance `/lsp-rename`** — `prepare_rename` gate added as Step 2.
2. ✅ **Create `/lsp-cross-repo` skill** — shipped; orchestrates `add_workspace_folder` → indexing verify → cross-repo `get_references` / `call_hierarchy` / `go_to_implementation`.
3. ✅ **Create `/lsp-local-symbols` skill** — shipped; `get_document_symbols` → `get_document_highlights` (read/write/text classification) → `get_info_on_location`.
4. ✅ **Enhance `/lsp-safe-edit`** — `simulate_edit_atomic` pre-flight + `get_code_actions` on errors + multi-file workflow (originally P2, pulled into P0 sprint).

### High (P1)

5. ✅ **Enhance `/lsp-verify`** — `get_tests_for_file` pre-step added; correlated/unrelated failure tagging in Layer 3 output.
6. ✅ **Create `/lsp-test-correlation` skill** — shipped; source→test mapping, `get_workspace_symbols` fallback, multi-file deduplication, scoped `run_tests`.
7. ✅ **Create `/lsp-format-code` skill** — shipped; `format_document` and `format_range` → `apply_edit`; also folded as optional step into `/lsp-safe-edit` (post-edit) and `/lsp-verify` (post-verification).

### Medium (P2)

8. **Create `/lsp-type-info` skill** or fold into enhanced `/lsp-docs`.
   - Use case: "What is the type of this symbol?"
   - Tools: `go_to_type_definition`, `get_semantic_tokens`, `get_inlay_hints`
   - Estimated effort: 2 hours
   - Impact: Better type introspection

9. **Create `/lsp-signature-help` skill**.
   - Use case: "What are the parameters for this function?"
   - Tools: `get_signature_help`, `get_completions`
   - Estimated effort: 1 hour
   - Impact: Narrow but useful for function discovery

### Low (P3)

10. **Document remaining uncovered tools** (close_document, restart_lsp_server, set_log_level, etc.) in a "Utilities & Lifecycle" section of README.
    - Estimated effort: 1 hour
    - Impact: Clarity on why some tools don't have skills

---

## 8. Gap Summary Table

| Gap Type | Count | Examples | Recommendation |
|----------|-------|----------|-----------------|
| Missing safety gates | 3 | `prepare_rename`, `get_server_capabilities`, simulator dry-run | Enhance existing skills |
| Missing navigation workflows | 2 | `get_document_highlights`, `go_to_type_definition` | New: `/lsp-local-symbols`, `/lsp-type-info` |
| Missing cross-repo workflows | 1 | `add_workspace_folder` + multi-root analysis | New: `/lsp-cross-repo` |
| Missing code formatting | 1 | `format_document`, `format_range` | New: `/lsp-format-code` |
| ~~Missing test correlation~~ | ~~1~~ | ~~`get_tests_for_file` + `run_tests`~~ | **Shipped:** `/lsp-test-correlation` |
| Missing code discovery | 1 | `get_completions`, `get_signature_help` in isolation | New: `/lsp-signature-help` (optional) |
| Low-level utilities (no skill needed) | 6+ | `close_document`, `restart_lsp_server`, `set_log_level`, etc. | Document, don't skill |

---

## Conclusion

**Post-sprint state:** ~72% of tools covered (including partial) by 14 skills. P0+P1 sprints added 4 new skills and enhanced 4 existing ones. Tools sprint added `get_change_impact` and `get_cross_repo_references` (50 total tools), promoted `simulate_chain` and `get_tests_for_file` to fully covered.

**Remaining gaps (P2):**
1. Type introspection — `go_to_type_definition` still uncovered; `/lsp-type-info` skill or `/lsp-docs` enhancement
2. Parameter discovery — `get_signature_help` still uncovered; `/lsp-signature-help` skill (narrow but useful)
3. Document low-level utilities (`close_document`, `restart_lsp_server`, `set_log_level`, etc.) without promoting to skills

**Remaining skills to ship:** 1–2 (P2: type info, optional signature help).

