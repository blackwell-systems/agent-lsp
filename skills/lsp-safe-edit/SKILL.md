---
name: lsp-safe-edit
description: Wrap any code edit with before/after diagnostic comparison. Captures baseline errors, makes the edit, then reports errors introduced vs. resolved.
compatibility: Requires lsp-mcp-go MCP server
allowed-tools: mcp__lsp__start_lsp mcp__lsp__open_document mcp__lsp__get_diagnostics Edit Write Bash
---

# lsp-safe-edit

Wrap any code edit with a before/after diagnostic comparison. Captures baseline
errors, makes the edit, then reports what errors were introduced vs. resolved.

## Prerequisites

LSP must be running for the target workspace. If not yet initialized, call
`mcp__lsp__start_lsp` with the workspace root before proceeding.

Auto-init note: lsp-mcp-go supports workspace auto-inference from file paths.
Explicit `start_lsp` is only needed when switching workspace roots.

## Input

- **target file(s):** One or more files to be edited (absolute paths).
- **description of change:** What you intend to edit and why.

## Workflow

**Step 1 — Open target file(s)**

Call `mcp__lsp__open_document` for each file that will be edited. This
registers the file with the LSP server so diagnostics reflect the current
on-disk state.

```
mcp__lsp__open_document(file_path: "/abs/path/to/file.go")
```

**Step 2 — Capture baseline diagnostics (BEFORE)**

Call `mcp__lsp__get_diagnostics` for each target file. Store the full result
as BEFORE. If multiple files are involved, collect diagnostics for all of them.

```
BEFORE = mcp__lsp__get_diagnostics(file_path: "/abs/path/to/file.go")
```

Wait for diagnostics to stabilize. If the server returns an empty list
immediately after open, wait briefly and call again — LSP analysis is async.

**Step 3 — Make the edit**

Apply the change using the Edit or Write tool as appropriate:

- Use `Edit` for targeted replacements in an existing file.
- Use `Write` only when creating a new file or doing a full rewrite.

```
Edit(file_path: "/abs/path/to/file.go", old_string: "...", new_string: "...")
```

**Step 4 — Capture post-edit diagnostics (AFTER)**

Call `mcp__lsp__get_diagnostics` again for each edited file. Store the result
as AFTER.

```
AFTER = mcp__lsp__get_diagnostics(file_path: "/abs/path/to/file.go")
```

**Step 5 — Compute the diagnostic diff**

Compare BEFORE and AFTER:

- **Introduced** = diagnostics in AFTER that were not in BEFORE (new problems).
- **Resolved** = diagnostics in BEFORE that are not in AFTER (fixed problems).

Match diagnostics by `(file, line, message)` tuple to avoid false matches from
line-number shifts. Severity: treat `error` and `warning` separately.

**Step 6 — Report using DiagnosticDiffFormat**

Output the summary using the format in [references/patterns.md](references/patterns.md).

## Decision Guide

| Net change | Action |
|------------|--------|
| 0          | Safe to proceed. No new errors introduced. |
| Negative   | Net improvement — errors resolved. Safe to proceed. |
| Positive   | **Do NOT commit.** New errors introduced. |

When net change > 0:

1. Show the full list of introduced errors.
2. Offer to revert the edit using the original `old_string` in an `Edit` call.
3. Wait for user decision before proceeding.

Do not commit or stage files when net change > 0.
