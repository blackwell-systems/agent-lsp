# Skills Reference

agent-lsp ships 20 skills — named workflows that encode correct tool sequences so
multi-step operations happen reliably. This doc is a developer reference: what each
skill does, when to reach for it, and what it does that raw tool calls miss.

All 20 skills conform to the [Agent Skills](https://agentskills.io/) open standard — the cross-agent skill format adopted by Claude Code, Cursor, GitHub Copilot, Gemini CLI, OpenAI Codex, JetBrains Junie, and [30+ other tools](https://agentskills.io/clients). Each `SKILL.md` includes the required `name` and `description` frontmatter fields, plus `license`, `compatibility`, and `allowed-tools`.

**agent-lsp skills are not locked to any single AI provider.** Because they follow the AgentSkills open standard, they work with any conforming agent — Claude, Copilot, Cursor, Gemini, Codex, Roo Code, OpenHands, and the rest. The MCP server handles the LSP runtime; the skills are portable workflow definitions that any agent can load and execute.

See the [Setup guide](getting-started/quickstart.md) for installation instructions. For the individual tools that skills compose, see [docs/tools.md](./tools.md). For the full AgentSkills specification, see [agentskills.io/specification](https://agentskills.io/specification).

## Quick example

```
/lsp-impact "MyFunction"     # See what breaks before you change it
/lsp-refactor "MyFunction"   # Full safe refactor: impact → preview → apply → verify → test
/lsp-verify                  # Confirm nothing broke after any edit
```

---

## Before you change anything

Understanding blast radius before touching code prevents broken builds and missed
callers. These three skills are read-only — they never modify files.

### `/lsp-impact`

Blast-radius analysis for a symbol or file — finds all direct references, callers
(via call hierarchy), and type relationships before you touch anything.

**When to reach for it:**
- You are about to rename or change the signature of an exported function and want to know how many files call it.
- You are auditing an entire file before a refactor and need every exported symbol's caller count in one shot.
- You want to know whether a change is low-risk (1–5 files) or high-risk (> 20 files) before committing to it.

**What it does that raw tools miss:**
Raw `get_references` tells you reference count. lsp-impact runs references, call hierarchy, and type hierarchy together, then classifies the result with a risk level — so you get a decision recommendation, not just numbers. The file-level mode (`get_change_impact`) surfaces all exported symbols at once without a symbol-by-symbol loop.

---

### `/lsp-implement`

Find every concrete type that implements an interface or extends an abstract type.

**When to reach for it:**
- You are adding a method to an interface and need the list of every type that must be updated.
- You are removing an interface method and need to confirm no external implementors exist.
- You are exploring an unfamiliar codebase and want to understand the full type hierarchy around a core interface.

**What it does that raw tools miss:**
Cross-references `go_to_implementation` with `type_hierarchy` to produce the union of all implementors — covering both explicit interface satisfaction and subtype relationships. Reports a risk level (0 implementors → likely internal-only; > 10 → breaking API change).

---

### `/lsp-dead-code`

Enumerate exported symbols in a file and surface those with zero references across the workspace.

**When to reach for it:**
- Cleaning up APIs before a release: find exports that are defined but never called.
- Auditing a legacy package to identify what is safe to remove.
- Checking whether a function you are about to delete has any hidden callers.

**What it does that raw tools miss:**
Doesn't just call `get_references` — it verifies indexing is complete before classifying anything (a common failure mode that produces false dead-code candidates), then cross-checks zero-reference results against grep for registration patterns that LSP cannot see (e.g., `router.Handle("/path", myHandler)`). The result is a classified report, not a raw list.

---

## Editing safely

Safe editing means knowing the error impact of a change before it lands on disk,
and having all callers in view before touching any exported symbol. These skills
gate on LSP diagnostics so you catch regressions before the build does.

### `/lsp-safe-edit`

Wrap any edit with a before/after diagnostic comparison. Previews the change
speculatively before writing to disk, then diffs errors introduced vs. resolved.

**When to reach for it:**
- Any non-trivial edit where you want to confirm you haven't broken a type or import.
- A multi-file change (e.g., changing a function signature and updating call sites) where you need to know the cumulative error delta before committing.
- After an edit surfaces new errors — lsp-safe-edit automatically queries code actions at each error location.

**What it does that raw tools miss:**
`simulate_edit_atomic` previews the error delta without touching disk — most agents skip this and apply edits blind. The `simulate_chain` path handles multi-step renames and signature changes: it evaluates each step in sequence and reports exactly how far through the chain is safe to apply.

---

### `/lsp-simulate`

Full speculative editing session — apply changes in memory, evaluate diagnostics,
then commit or discard without touching any files.

**When to reach for it:**
- Exploring a refactor whose safety you are not sure about before any file is touched.
- Planning a sequence of dependent edits (e.g., add a field, update all constructors, update all callers) and wanting to verify the full sequence is clean before starting.
- Recovering a patch across an MCP server restart: `commit_session(apply: false)` returns a portable patch.

**What it does that raw tools miss:**
Unlike `simulate_edit_atomic` (single edit, atomic), lsp-simulate manages a full session lifecycle — create, apply multiple edits, evaluate, commit or discard. The `simulate_chain` tool evaluates diagnostics after every step, reporting exactly which step first introduces an error.

---

### `/lsp-edit-symbol`

Edit a named symbol without knowing its file path or line number.

**When to reach for it:**
- You want to change the signature of `internal/lsp.ParseConfig` but don't have the file open.
- You are modifying a symbol by name from a list and don't want to navigate manually.
- Changing only the signature line (not the body) of a function you can name precisely.

**What it does that raw tools miss:**
Composes `get_workspace_symbols` → `get_document_symbols` → `apply_edit` to resolve a symbol name to its exact range. Supports text-match apply (no position math needed) and positional apply (when you need to replace a full body). This removes the "find the file, find the line" manual step that agents frequently get wrong.

---

### `/lsp-edit-export`

Safe workflow for editing exported symbols — finds all callers first, presents an
impact summary, and requires confirmation before any edit is applied.

**When to reach for it:**
- Changing the signature of a public function used across multiple packages.
- Modifying a public type (adding/removing fields) where downstream callers may break.
- Any change to a symbol that is exported (uppercase Go, `export` TypeScript, `pub` Rust).

**What it does that raw tools miss:**
Enforces a mandatory confirmation gate — callers are listed and the user must say yes before any edit is applied. Also runs the build after the edit to confirm clean compilation. This gate exists even when LSP reports zero callers, because an incomplete index can silently miss usages.

---

### `/lsp-rename`

Two-phase safe rename: validate the rename is possible, preview all affected sites,
confirm, then apply atomically via the language server.

**When to reach for it:**
- Renaming a function, method, type, or variable across an entire codebase.
- Renaming `Handler` in a Go package that is referenced across 12 files.
- Any rename where you want a preview of all changed lines before committing.

**What it does that raw tools miss:**
Uses `prepare_rename` as a safety gate — the language server validates the rename is legal at that position before anything is touched. The dry-run produces the full `workspace_edit` preview (all files and line numbers) before asking for confirmation. Atomically applies all changes in one `apply_edit` call rather than editing file by file.

---

## Understanding unfamiliar code

Before changing code you don't know well, these skills build a complete picture of
a symbol or file's structure, dependencies, and callers.

### `/lsp-explore`

"Tell me about this symbol" — hover, implementations, call hierarchy, and
references in one pass.

**When to reach for it:**
- You encounter an unfamiliar function name in a code review and want its type, docs, callers, and usage sites without opening four tools.
- Navigating a new codebase: start with the primary interface or entry point to map what calls it and what implements it.
- Quick single-symbol triage before deciding whether to dig deeper with lsp-understand.

**What it does that raw tools miss:**
Runs hover, `go_to_implementation`, `call_hierarchy` (incoming), and `get_references` in parallel against a single symbol and formats them into a single Explore Report. Capability-gated: steps that the language server doesn't support are skipped cleanly rather than failing.

---

### `/lsp-understand`

Deep-dive Code Map for a symbol or file: type info, implementations, 2-level call
hierarchy, all references, and source — synthesized into a dependency map.

**When to reach for it:**
- You need to understand how an entire file works as a module (pass a file path, get a Code Map of all its exported symbols).
- Planning a large refactor and needing to know all the internal call chains before touching anything.
- Onboarding to a new codebase: run lsp-understand on the primary handler or service file to build a mental model.

**What it does that raw tools miss:**
Goes beyond lsp-explore in three ways: accepts a file path to analyze all exported symbols as a group; synthesizes cross-symbol relationships (which entry points call each other, share callers, or implement the same interface); enforces a 2-level call hierarchy depth limit to prevent runaway recursion on deeply connected code.

---

### `/lsp-docs`

Three-tier documentation lookup: hover → offline toolchain doc → source definition.

**When to reach for it:**
- Hover returns empty and you need the signature and docs for a standard library function.
- The symbol is in a transitive dependency that the language server doesn't index.
- No LSP session is running but you need documentation for a symbol in the module cache.

**What it does that raw tools miss:**
Falls through tiers automatically: if LSP hover is empty, it calls `get_symbol_documentation` against the installed toolchain (`go doc`, `pydoc`, `cargo doc`) — no LSP session required. If the toolchain call fails, it falls back to `go_to_definition` + `get_symbol_source` to extract the raw source. The result is always the richest documentation available, not "hover returned empty."

---

### `/lsp-cross-repo`

Find all callers of a library symbol across one or more consumer repositories.

**When to reach for it:**
- You maintain a shared library and are about to change a public API: find all consumer call sites before touching anything.
- Auditing how an internal package is used across a set of services.
- Before deleting a symbol: verify no cross-repo dependents exist.

**What it does that raw tools miss:**
`get_cross_repo_references` adds each consumer as a workspace folder, waits for indexing, runs `get_references` across all roots, and returns results partitioned by repo — so you see `api-service: [main.go:14, app.go:31]` and `worker-service: [runner.go:8]` in one call rather than setting up multi-root workspaces manually. Warnings flag any consumer root that failed to index.

---

### `/lsp-local-symbols`

File-scoped symbol analysis: list all symbols in a file, find usages within the
file, and get type info at a position.

**When to reach for it:**
- "What functions and types are defined in this file?" — before reading the whole file.
- Confirming a variable is only used once in a function before inlining it.
- Getting the type signature of a symbol at a specific position without a workspace-wide search.

**What it does that raw tools miss:**
`get_document_highlights` is significantly faster than `get_references` for file-local queries — it doesn't scan the workspace index. This skill routes correctly: use highlights for file-local, escalate to `get_references` (lsp-impact) only when cross-file results are needed. Coordinates from `get_document_symbols` feed directly into highlights and hover without manual position math.

---

## After editing

These skills run after changes are on disk to confirm correctness, apply
remaining fixes, and keep the suite green.

### `/lsp-verify`

Full three-layer verification: LSP diagnostics + compiler build + test suite,
ranked by severity.

**When to reach for it:**
- After completing any edit, refactor, or feature before committing.
- After merging or rebasing to confirm nothing broke.
- When you want a single command that covers "does it type-check, compile, and pass tests."

**What it does that raw tools miss:**
Runs diagnostics first, then build, then tests — ordered by severity so the fastest signal comes first. When `changed_files` is provided, it pre-correlates test files so failures point directly to which tests cover the changed code. Code actions are surfaced for any diagnostic errors so quick fixes are visible immediately.

---

### `/lsp-fix-all`

Apply language-server quick-fix code actions for all current diagnostics in a
file, one at a time with diagnostic re-collection between each fix.

**When to reach for it:**
- A file has accumulated missing imports, unused variables, or other auto-fixable warnings before you start new work.
- You want to systematically resolve all language-server quick-fixes in a file without doing it manually.
- Cleaning up generated code that the server flags with straightforward fixes.

**What it does that raw tools miss:**
The correct fix-all loop re-collects diagnostics after every single `apply_edit` because line numbers shift with each fix. Naive bulk application breaks. This skill enforces the one-fix-per-iteration constraint, filters out structural refactors (only `quickfix` and `source.organizeImports` kinds are applied), and caps at 50 iterations to prevent infinite loops.

---

### `/lsp-test-correlation`

Find and run only the tests that cover a specific source file — without running
the full suite.

**When to reach for it:**
- You edited one file and want fast feedback: which test file covers this code, and do those tests still pass?
- Before committing: run exactly the tests that cover what you touched.
- Debugging a test failure: find which test file corresponds to a broken source file.

**What it does that raw tools miss:**
`get_tests_for_file` maps source files to test files without text search. The skill then uses `get_workspace_symbols` to enumerate specific test function names, so `run_tests` can be scoped to a filter rather than a package — faster than running `./...`. Falls back to symbol search for test function names when the mapping returns no results.

---

### `/lsp-format-code`

Format a file or selection using the language server's formatter — gofmt, prettier,
rustfmt, black — without requiring those tools on PATH separately.

**When to reach for it:**
- Before committing: apply consistent style to all edited files.
- After generating code: clean up AI-generated indentation and spacing.
- After a refactor that shifted indentation levels by adding or removing blocks.

**What it does that raw tools miss:**
Uses the language server's `format_document` and `format_range` tools rather than shelling out to a formatter binary. Supports range-based formatting (format only a selected block) in addition to full-file. Verifies diagnostics after formatting — formatting should never introduce errors, and the skill reports immediately if it does.

---

## Generating code

These skills use the language server's code generation capabilities to produce
new code rather than modifying existing code.

### `/lsp-generate`

Trigger server-side code generation: implement interface stubs, generate test
skeletons, add missing methods, generate mock types.

**When to reach for it:**
- You have a type that needs to implement an interface: generate all required method stubs automatically.
- You want a test skeleton for a new source file.
- Rust: implement all trait members for a `impl Trait for Type {}` block.

**What it does that raw tools miss:**
Routes to the language server's native generator actions via `get_code_actions` and `execute_command`, which produce server-correct output (proper types, proper signatures) rather than templated boilerplate. When no code action is available, falls back with language-specific manual guidance rather than failing silently. Requires confirmation when multiple generator actions match the intent.

---

### `/lsp-extract-function`

Extract a selected code block into a named function — using the language server's
extract-function code action, with manual fallback when no action is available.

**When to reach for it:**
- A function has grown too long and a block of logic should be its own function.
- You want to name and isolate a section of code without manual copy-paste and signature construction.
- Refactoring before adding a test: extract the logic under test into a named function first.

**What it does that raw tools miss:**
Uses the language server's `refactor.extract` code action when available (gopls, tsserver) — the server correctly identifies captured variables, return values, and scope boundaries. When no code action exists (common in Python), falls back to a structured manual extraction that requires user confirmation on the proposed signature before applying. Validates with diagnostics after extraction and formats the result.

---

## Full workflow

### `/lsp-refactor`

End-to-end safe refactor: blast-radius analysis → speculative preview → apply to
disk → build verification → targeted test execution. Composes lsp-impact,
lsp-safe-edit, lsp-verify, and lsp-test-correlation in one coordinated sequence.

**When to reach for it:**
- You know your target and intent up front (e.g., "rename `ParseConfig` to `ParseConfigV2`") and want the complete workflow without switching skills.
- A refactor that touches an exported symbol with multiple callers and requires a clean build and green tests before it's done.
- Any change where you want blast radius, simulation, apply, build, and test in one command.

**What it does that raw tools miss:**
Enforces gate conditions at each phase — Phase 1 halts on high blast radius (> 20 callers) unless confirmed; Phase 2 halts if simulation introduces errors; Phase 4 halts if the build fails. No phase executes if its predecessor fails. Individual skills can be used independently, but lsp-refactor is the correct choice when you want the entire sequence enforced without manual orchestration.

---

## Skill composition

Common sequences for typical developer workflows:

**Refactoring an exported symbol**

```
/lsp-impact "codec.Encode"        # Check blast radius: how many callers?
/lsp-edit-export "codec.Encode"   # Edit with caller confirmation gate
/lsp-verify                       # Diagnostics + build + tests
```

Use lsp-impact first when you want to decide whether to proceed before being
committed to a workflow. lsp-edit-export then handles the confirmation gate and
build check. lsp-verify confirms nothing broke.

**Renaming across the codebase**

```
/lsp-rename "OldName" "NewName"   # Preview all sites, confirm, apply atomically
/lsp-test-correlation <file>      # Run only the tests that cover the changed file
```

lsp-rename handles the language-server atomic rename. lsp-test-correlation gives
fast feedback without running the full suite.

**Understanding unfamiliar code before editing**

```
/lsp-explore "Handler.ServeHTTP"  # Quick: type info, callers, implementations
/lsp-understand "internal/server/handler.go"  # Deeper: Code Map of full file
/lsp-safe-edit <file>             # Edit with before/after diagnostic comparison
```

Start with lsp-explore for a single symbol triage. Escalate to lsp-understand
when you need the full module picture before making changes. Then use lsp-safe-edit
to gate the edit on diagnostic impact.

---

## See also

- [docs/tools.md](./tools.md) — full tool reference with parameters and examples
- [docs/language-support.md](./language-support.md) — language coverage matrix and per-language tool support
