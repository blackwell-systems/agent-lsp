---
name: lsp-verify
description: Full three-layer verification after any change — LSP diagnostics + compiler build + test suite, ranked by severity.
compatibility: Requires lsp-mcp-go MCP server
allowed-tools: mcp__lsp__get_diagnostics mcp__lsp__run_build mcp__lsp__run_tests
---

# lsp-verify: Three-Layer Verification

## When to Use

Run this skill after any significant change to verify correctness at every level:

- After editing source files (logic changes, refactors, new functions)
- After merging or rebasing branches
- After dependency updates or configuration changes
- Before committing or pushing code

## Input

- `workspace_dir` (required): absolute path to the workspace root (e.g. `/Users/you/code/myproject`)

## Verification Layers

### Layer 1: LSP Diagnostics

Call `mcp__lsp__get_diagnostics` with the workspace directory.

Note: `get_diagnostics` requires LSP to be initialized. If not yet running, call
`start_lsp` with the workspace root first. Auto-inference is supported when file
paths are provided, but explicit initialization ensures complete workspace coverage.

```
mcp__lsp__get_diagnostics({ "workspace_dir": "<workspace_dir>" })
```

Rank results by severity: list errors first, then warnings.

### Layer 2: Build

Call `mcp__lsp__run_build` with the workspace directory.

```
mcp__lsp__run_build({ "workspace_dir": "<workspace_dir>" })
```

Returns `{ "success": bool, "errors": [...] }`. A failed build means the
code does not compile. Build errors are blocking — they must be resolved before
the change can ship.

### Layer 3: Tests

Call `mcp__lsp__run_tests` with the workspace directory.

```
mcp__lsp__run_tests({ "workspace_dir": "<workspace_dir>" })
```

Note: `run_tests` does NOT require `start_lsp` to be called first. The
`workspace_dir` parameter is required.

Returns `{ "passed": bool, "failures": [...] }`. Each failure includes a
`file:line` location. Test failures are blocking — they indicate regressions
or unmet contracts.

## Output Format

After running all three layers, produce a structured report using progressive
disclosure: summary first, then details.

```
## Verification Report

### Layer 1: LSP Diagnostics
[CLEAN / N errors, M warnings]

<details if N > 0 or M > 0>
Errors:
- file:line - message
...

Warnings:
- file:line - message
...
</details>

### Layer 2: Build
[PASSED / FAILED - N errors]

<details if FAILED>
- error message (file:line)
...
</details>

### Layer 3: Tests
[PASSED / FAILED - N failures]

<details if FAILED>
- test name: message (file:line)
...
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
