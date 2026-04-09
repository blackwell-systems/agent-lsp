---
name: lsp-dead-code
description: Enumerate exported symbols in a file and surface those with zero references across the workspace. Use when auditing for dead code, cleaning up APIs, or checking which exports are safe to remove.
argument-hint: "[file-path]"
allowed-tools: mcp__lsp__get_document_symbols mcp__lsp__get_references mcp__lsp__open_document
---

> Requires the lsp-mcp-go MCP server.

# lsp-dead-code

Audit an exported symbol list for zero-reference candidates. Calls
`get_document_symbols` to enumerate symbols, then checks each exported
symbol with `get_references` to find callers. Produces a classified report.

## When to Use

Use this skill when you want to identify dead code in a file — exported
symbols that are defined but never called anywhere in the workspace. Common
use cases:

- Cleaning up APIs before a release
- Identifying legacy exports that can be safely removed
- Auditing a package for unused public surface area

**Important:** This skill surfaces *candidates*. Always review results
manually before deleting anything. See the Caveats section below.

## What counts as "exported"

| Language   | Exported means...                                                      |
|------------|------------------------------------------------------------------------|
| Go         | Identifier starts with an uppercase letter (e.g. `MyFunc`, `MyType`)  |
| TypeScript | Has `export` keyword; or is a public class member (no `private`)       |
| Python     | Not prefixed with `_`; or explicitly listed in `__all__`               |
| Java/C#    | Has `public` or `protected` visibility modifier                        |
| Rust       | Has `pub` keyword                                                      |

## Prerequisites

If LSP is not yet initialized, call `mcp__lsp__start_lsp` with the
workspace root first:

```
mcp__lsp__start_lsp({ "root_dir": "/your/workspace" })
```

lsp-mcp-go supports auto-inference from file paths, so explicit start is
only required when switching workspaces or on a cold session.

**Indexing note:** `get_references` waits for workspace indexing to complete.
If it returns `[]` for ALL symbols, the workspace may not be fully indexed
yet — wait 2–3 seconds and retry the first symbol before concluding there
are no references.

## Step 1 — Open the file and enumerate symbols

Open the file so the language server tracks it, then fetch all symbols:

```
mcp__lsp__open_document({ "file_path": "/abs/path/to/file.go" })

mcp__lsp__get_document_symbols({ "file_path": "/abs/path/to/file.go" })
```

Collect the full symbol list. Filter to **exported symbols only** using the
language-appropriate rule from the table above.

**Coordinate note:** `get_document_symbols` returns 0-based coordinates
(LSP native). You must add 1 to both line and character before passing them
to `get_references`:

```
line_for_refs = symbol.selectionRange.start.line + 1
col_for_refs  = symbol.selectionRange.start.character + 1
```

This conversion is required. Omitting it causes off-by-one errors that
produce incorrect reference results.

## Step 2 — Check references for each exported symbol

For each exported symbol, call `get_references` with
`include_declaration: false` so the definition site itself is excluded
from the count. A count of 0 means no callers, not no occurrences.

```
mcp__lsp__get_references({
  "file_path": "/abs/path/to/file.go",
  "line": <selectionRange.start.line + 1>,      // convert 0-based to 1-based
  "column": <selectionRange.start.character + 1>,
  "include_declaration": false
})
```

Record the result for each symbol:
```
{ symbol_name, kind, line, reference_count, locations[] }
```

**Batching note:** For files with many exported symbols (>20), process in
batches of 5–10 to avoid overwhelming the LSP server. The workspace
indexing is shared state — rapid-fire calls may return stale results.
Between batches, check if any `get_references` returned `[]` unexpectedly.
If the first symbol returns `[]` but is clearly used, wait 2–3 seconds and
retry before trusting the result.

## Step 3 — Classify and report

Classify each exported symbol by reference count:

- **Zero references** — candidate for removal. Flag with WARNING.
- **1–2 references** — review manually. May be test-only usage.
- **3+ references** — active symbol. Not dead code.

For test-only references: if all locations are in `_test.go` files (Go) or
files named `*.test.*` / `*.spec.*`, mark the symbol as "test-only" in
the report rather than "zero-reference".

Produce the Dead Code Report using the format in
[references/patterns.md](references/patterns.md).

## Caveats

The following cases produce zero LSP references even though the symbol IS
used at runtime. Do not delete any zero-reference candidate without
manual review:

1. **Incomplete indexing.** `get_references` only searches files open or
   indexed by the language server. If the workspace is partially indexed,
   results may be incomplete.

2. **Reflection and dynamic dispatch.** Symbols used via reflection
   (`reflect.TypeOf` in Go, `Class.forName` in Java) or dynamic dispatch
   have no static call sites visible to the LSP.

3. **`//go:linkname` and assembly.** Go symbols linked via `//go:linkname`
   or referenced from assembly files will show zero LSP references.

4. **Library public API.** Exported symbols called from external packages
   not present in the workspace will show zero references even if
   consumers exist.

5. **Declaration excluded from count.** The definition site is not counted
   (`include_declaration: false`). A count of 0 means no callers found,
   not that the symbol never appears in the source tree.

6. **Always review before deleting.** Zero LSP references is a signal to
   investigate, not a guarantee the symbol is unused.

## Step 4 — Next steps

After generating the report:

- **For each zero-reference symbol:** Run `lsp-impact` on the symbol to
  confirm. If `lsp-impact` also finds zero references, it is safe to
  consider for removal. Still check the Caveats section above.

- **For symbols with only test-file references:** Mark as "test-only" in
  the report. These may be candidates for removal if the tests themselves
  are redundant, but should not be deleted without reviewing whether the
  tests serve a documentation or contract purpose.

- **For symbols with 1–2 references in production code:** These are likely
  active but lightly used. Do not remove without checking whether they are
  part of a committed public API.
