<!-- agent-lsp:rules:start -->
## agent-lsp Skills

agent-lsp provides 56 code intelligence tools and 22 workflow skills.
Prefer LSP tools over Grep/Glob/Read for code navigation.

**Before editing code:** call `get_change_impact` for blast-radius analysis.
**Before applying edits:** call `simulate_edit_atomic` to preview the diagnostic delta.
**After any change:** call `get_diagnostics`, then `run_build` and `run_tests`.

| Skill | Description |
|-------|-------------|
| `/lsp-architecture` | Generate a structural architecture overview of a codebase: languages, package map, entry points, dependency graph, an... |
| `/lsp-cross-repo` | Cross-repository analysis — find all callers of a library symbol in one or more consumer repos. Use when refactorin... |
| `/lsp-dead-code` | Enumerate exported symbols in a file and surface those with zero references across the workspace. Use when auditing f... |
| `/lsp-docs` | Three-tier documentation lookup for any symbol — hover → offline toolchain doc → source definition. Use when ho... |
| `/lsp-edit-export` | Safe workflow for editing exported symbols or public APIs. Use when changing a function signature, modifying a public... |
| `/lsp-edit-symbol` | Edit a named symbol without knowing its file or position. Use when you want to change a function, type, or variable b... |
| `/lsp-explore` | Tell me about this symbol": hover + implementations + call hierarchy + references in one pass — for navigating unfa... |
| `/lsp-extract-function` | Extract a selected code block into a named function. Primary path uses the language server's extract-function code ac... |
| `/lsp-fix-all` | Apply available quick-fix code actions for all current diagnostics in a file, one at a time with re-collection betwee... |
| `/lsp-format-code` | Format a file or selection using the language server's formatter. Use before committing to apply consistent style, or... |
| `/lsp-generate` | Trigger language server code generation — implement interface stubs, generate test skeletons, add missing methods, ... |
| `/lsp-impact` | Blast-radius analysis for a symbol or file — shows all callers, type supertypes/subtypes, and reference count befor... |
| `/lsp-implement` | Find all concrete implementations of an interface or abstract type. Use when you need to know what types satisfy an i... |
| `/lsp-inspect` | Full code quality audit for a file or package. Applies a check taxonomy (dead symbols, silent failures, error wrappin... |
| `/lsp-local-symbols` | Fast file-scoped symbol analysis — find all usages of a symbol within the current file, list all symbols defined in... |
| `/lsp-refactor` | End-to-end safe refactor workflow — blast-radius analysis, speculative preview, apply to disk, verify build, run af... |
| `/lsp-rename` | Two-phase safe rename across the entire workspace. Use when renaming any symbol, function, method, variable, type, or... |
| `/lsp-safe-edit` | Wrap any code edit with before/after diagnostic comparison. Speculatively previews the change first (simulate_edit_at... |
| `/lsp-simulate` | Speculative code editing session — simulate changes in memory before touching disk. Use when planning edits that mi... |
| `/lsp-test-correlation` | Find and run the tests that cover a source file. Use after editing a file to discover exactly which test files and te... |
| `/lsp-understand` | Deep-dive exploration of unfamiliar code — given a symbol or file, builds a complete Code Map showing type info, im... |
| `/lsp-verify` | Full three-layer verification after any change — LSP diagnostics + compiler build + test suite, ranked by severity.... |

Call `prompts/get` with any skill name for full workflow instructions.
<!-- agent-lsp:rules:end -->
