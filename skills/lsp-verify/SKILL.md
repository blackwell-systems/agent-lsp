---
name: lsp-verify
description: Full three-layer verification after any change — LSP diagnostics + compiler build + test suite, ranked by severity. Use after completing any edit, refactor, or feature to confirm nothing is broken before committing.
allowed-tools: mcp__lsp__get_diagnostics mcp__lsp__run_build mcp__lsp__run_tests
---

> Requires the lsp-mcp-go MCP server.

# lsp-verify: Three-Layer Verification

## When to Use

Run this skill after any significant change to verify correctness at every level:

- After editing source files (logic changes, refactors, new functions)
- After merging or rebasing branches
- After dependency updates or configuration changes
- Before committing or pushing code

## Input

- `workspace_dir` (required): absolute path to the workspace root (e.g. `/Users/you/code/myproject`)
- `changed_files` (optional): list of files you edited — used for targeted diagnostics

## Execution

**Run all three layers in parallel** — they are independent and do not need to
be sequenced. Issue all three calls in the same message to minimize wall time.

### Layer 1: LSP Diagnostics

Call `mcp__lsp__get_diagnostics` with `file_path` set to each changed file.
`get_diagnostics` takes a file path, not a workspace directory.

Note: requires LSP to be initialized. If not yet running, call `start_lsp`
with the workspace root first.

```
mcp__lsp__get_diagnostics({ "file_path": "<path/to/changed/file>" })
```

Call once per changed file. If you don't know which files changed, call it on
the primary files touched in this session. Rank results by severity: errors
first, then warnings.

### Layer 2: Build

```
mcp__lsp__run_build({ "workspace_dir": "<workspace_dir>" })
```

Returns `{ "success": bool, "errors": [...] }`. A failed build means the code
does not compile. Build errors are blocking — must be resolved before shipping.

### Layer 3: Tests

```
mcp__lsp__run_tests({ "workspace_dir": "<workspace_dir>" })
```

Does NOT require `start_lsp`. Returns `{ "passed": bool, "failures": [...] }`.

**Large output warning:** `run_tests` on large repos can return hundreds of
thousands of characters and exceed the context window. If the result is saved
to a file rather than returned inline, do NOT attempt to read the whole file.
Instead, search it for failures:

```bash
grep -E "^(FAIL|--- FAIL)" <output_file>
```

Or run tests directly on just the changed package to avoid the size issue:

```bash
GOWORK=off go test -count=1 -short ./internal/mypackage/... 2>&1 | grep -E "FAIL|ok"
```

Test failures are blocking — they indicate regressions or unmet contracts.

## Output Format

After running all three layers, produce a structured report:

```
## Verification Report

### Layer 1: LSP Diagnostics
[CLEAN / N errors, M warnings]

<details if N > 0 or M > 0>
Errors:
- file:line - message

Warnings:
- file:line - message
</details>

### Layer 2: Build
[PASSED / FAILED - N errors]

<details if FAILED>
- error message (file:line)
</details>

### Layer 3: Tests
[PASSED / FAILED - N failures]

<details if FAILED>
- test name: message (file:line)
</details>

### Summary
Overall: CLEAN / NEEDS ATTENTION
Blocking issues: [errors that must be fixed before shipping]
```

- **CLEAN**: no errors in any layer (warnings are advisory only)
- **NEEDS ATTENTION**: one or more blocking issues found

## Blocking vs Advisory

| Layer | Errors | Warnings |
|-------|--------|----------|
| LSP Diagnostics | Blocking | Advisory |
| Build | All blocking | N/A |
| Tests | All blocking | N/A |

Build errors and test failures block shipping. LSP warnings and style
suggestions are advisory — document them but do not treat as blockers unless
they indicate logical errors.
