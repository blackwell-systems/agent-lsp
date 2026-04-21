---
name: lsp-edit-symbol
description: "Edit a named symbol without knowing its file or position. Use when you want to change a function, type, or variable by name and don't have exact coordinates. Resolves the symbol to its definition, retrieves its full range, and applies the edit."
argument-hint: "[symbol-name] [new-body-or-signature]"
allowed-tools:
  - mcp__lsp__get_workspace_symbols
  - mcp__lsp__get_document_symbols
  - mcp__lsp__apply_edit
---

# lsp-edit-symbol

Edit a named symbol (function, type, variable) without needing its exact file path
or line/column. Composes `go_to_symbol` → `get_document_symbols` → `apply_edit`.

## Workflow

### Step 1 — Locate the symbol

```json
{ "tool": "get_workspace_symbols", "query": "MyFunc" }
```

Returns a list of matching symbols with file URI and position. Pick the definition
(not a test file, not a stub). If multiple matches, use the container name or file
path to disambiguate.

### Step 2 — Get the full range

```json
{
  "tool": "get_document_symbols",
  "file_path": "/path/to/file.go",
  "language_id": "go"
}
```

Find `MyFunc` in the returned tree. The `range` field covers the entire symbol
including its body; `selectionRange` covers only the name. Use `range` when
replacing the full definition, `selectionRange` when renaming only.

### Step 3 — Apply the edit

**Option A — text-match (recommended when you have the old text):**
```json
{
  "tool": "apply_edit",
  "file_path": "/path/to/file.go",
  "old_text": "func MyFunc() {",
  "new_text": "func MyFunc() error {"
}
```
No position needed. Tolerates indentation differences.

**Option B — positional (when you have the exact range from Step 2):**
```json
{
  "tool": "apply_edit",
  "workspace_edit": {
    "changes": {
      "file:///path/to/file.go": [{
        "range": { "start": {"line": 12, "character": 0}, "end": {"line": 18, "character": 1} },
        "newText": "func MyFunc() error {\n\treturn nil\n}"
      }]
    }
  }
}
```

## Decision guide

| Situation | Approach |
|-----------|----------|
| Changing signature only | Step 1 → Step 3A with one-line old_text |
| Replacing full body | Step 1 → Step 2 → Step 3B with full range |
| Symbol name ambiguous | Use `get_workspace_symbols` query + container name filter |
| After edit | Run `get_diagnostics` to verify no errors introduced |

## Notes

- `get_workspace_symbols` returns declaration sites, not all references. The
  first non-test result is usually the definition.
- Positions in `get_document_symbols` are **1-based** (shifted from LSP convention).
  `apply_edit` `workspace_edit` expects **0-based** — subtract 1 when using positional
  mode (Option B). Text-match mode (Option A) requires no position math.
- For renames (not edits), use `/lsp-rename` instead — it updates all call sites.
