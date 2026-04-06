# lsp-mcp Tool Reference

All 24 tools exposed by the lsp-mcp MCP server. Coordinates are **1-based** for
both `line` and `column` in every tool call; the server converts internally to
the 0-based values the LSP spec requires.

---

## Session tools

### `start_lsp`

Initialize or reinitialize the LSP server with a specific project root. Call
this before any analysis when switching to a different project than the one the
server was started with. The server starts automatically on MCP launch with the
directory configured in `mcp.json`; this tool lets you point it at a different
workspace root at runtime.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `root_dir` | string | yes | Absolute path to the workspace root (directory containing `package.json`, `go.mod`, `go.work`, etc.) |

**Example call**

```json
{
  "root_dir": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project"
}
```

**Actual output**

```
LSP server initialized with root: /Users/dayna.blackwell/code/LSP-MCP/test/ts-project
```

**Notes**

- Shuts down the existing LSP process before starting the new one — no resource
  leak.
- After `start_lsp` returns, the underlying language server is initialized but
  may not have finished indexing the workspace. For `get_references` on large
  projects, the server waits for all `$/progress` end events before returning.
- Call `open_document` after this before running any per-file analysis.

---

### `restart_lsp_server`

Restart the current LSP server process without changing the workspace root.
Useful when the server becomes unresponsive or after major project-structure
changes (adding a new module, moving files).

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `root_dir` | string | no | If provided, restarts with a new workspace root. Omit to restart with the same root. |

**Example call**

```json
{}
```

**Actual output**

```
LSP server restarted successfully
```

**Notes**

- Requires the LSP client to already be initialized. Returns an error if
  `start_lsp` has never been called.
- All open documents are lost after restart; call `open_document` again for
  any files you need.

---

### `open_document`

Register a file with the language server for analysis. Most analysis tools
(`get_info_on_location`, `get_completions`, `get_references`, etc.) call this
internally via the `withDocument` helper, so you typically only need to call
it explicitly when you want to pre-warm a file or keep it open across multiple
operations.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier (`typescript`, `javascript`, `go`, `python`, `haskell`, etc.) |

**Example call**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript"
}
```

**Actual output**

```
File successfully opened: /Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts
```

**Notes**

- Idempotent: opening an already-open file is safe; it re-sends `didOpen` so
  the server refreshes its view of the file content.
- The file must exist on disk; the tool reads it with `fs.readFile`.
- The server tracks `file_path` and `language_id` internally so it can
  `reopenDocument` when `get_diagnostics` is called.

---

### `close_document`

Remove a file from the language server's open-document set. Sends
`textDocument/didClose` and frees the server's in-memory state for that file.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file to close |

**Example call**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts"
}
```

**Actual output**

```
File successfully closed: /Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts
```

**Notes**

- Good practice in long sessions or large codebases to close files you are
  done analyzing.
- `get_diagnostics` (no `file_path`) only returns diagnostics for currently
  open files, so closing a file removes it from those results.

---

## Analysis tools

### `get_diagnostics`

Fetch diagnostic messages (errors, warnings, hints) for one or all open files.
The tool re-opens the file(s) from disk to ensure fresh content, waits for the
language server to publish diagnostics, then returns them.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | no | Absolute path to a specific file. Omit to get diagnostics for all open files. |

**Example call — single file**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts"
}
```

**Actual output — clean file**

```json
{
  "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts": []
}
```

**Actual output — all open files**

```json
{
  "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts": [],
  "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts": []
}
```

**Output shape — file with errors**

```json
{
  "file:///path/to/file.ts": [
    {
      "range": {
        "start": { "line": 43, "character": 6 },
        "end": { "line": 43, "character": 21 }
      },
      "severity": 1,
      "code": 2304,
      "source": "ts",
      "message": "Cannot find name 'undefinedVariable'."
    }
  ]
}
```

Severity codes: `1` = error, `2` = warning, `3` = information, `4` = hint.

**Notes**

- Output keys are `file://` URIs, not file paths.
- The tool waits for `textDocument/publishDiagnostics` notifications before
  returning, so it may take a moment on first call.
- Files must have been opened with `open_document` (or any analysis tool) for
  their URIs to appear in the all-files result.

---

### `get_info_on_location`

Retrieve hover information for a symbol at a specific position via
`textDocument/hover`. Returns type signatures, JSDoc/godoc comments, and other
contextual detail that the language server provides on hover.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier |
| `line` | number | yes | Line number (1-based) |
| `column` | number | yes | Column position (1-based) |

**Example call**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17
}
```

**Expected output — TypeScript function**

```
function add(a: number, b: number): number

A simple function that adds two numbers
```

**Expected output — TypeScript class**

```
class Greeter
```

**Notes**

- Returns an empty string when the server returns `null` (e.g. whitespace,
  punctuation, or a position with no symbol).
- The server must declare `hoverProvider` capability; if it does not, the tool
  returns an empty string immediately without sending a request.
- For markdown-formatted hover content, the server returns
  `MarkupContent { kind: "markdown", value: "..." }` and the tool returns the
  `value` field.
- The tool opens the file internally before requesting hover, so `open_document`
  is not required as a prerequisite.

---

### `get_completions`

Request completion items at a cursor position via `textDocument/completion`.
Useful for discovering available properties on an object, functions exported
from a module, or valid identifiers in scope.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier |
| `line` | number | yes | Line number (1-based) |
| `column` | number | yes | Column position (1-based), typically just after a `.` or at the start of a partial identifier |

**Example call — after `greeter.`**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts",
  "language_id": "typescript",
  "line": 11,
  "column": 9
}
```

**Expected output** (truncated)

```json
[
  {
    "label": "greet",
    "kind": 2,
    "detail": "(method) Greeter.greet(person: Person): string",
    "sortText": "0",
    "insertText": "greet"
  },
  {
    "label": "greeting",
    "kind": 5,
    "detail": "(property) Greeter.greeting: string",
    "sortText": "1",
    "insertText": "greeting"
  }
]
```

Completion item `kind` values follow LSP §3.18: `1`=Text, `2`=Method,
`3`=Function, `4`=Constructor, `5`=Field, `6`=Variable, `7`=Class,
`9`=Module, etc.

**Notes**

- Returns `[]` if the server does not declare `completionProvider` capability.
- The server may return a `CompletionList` with `isIncomplete: true` for large
  result sets; the tool extracts the `items` array in that case.
- Place the column immediately after the trigger character (`.`, `:`, space) for
  best results.

---

### `get_signature_help`

Return function signature information when the cursor is inside an argument
list, via `textDocument/signatureHelp`. Shows available overloads and
highlights the active parameter.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier |
| `line` | number | yes | Line of the call site (1-based) |
| `column` | number | yes | Column inside the argument list (1-based) |

**Example call** — cursor inside `add(1, ` on line 4 of consumer.ts

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 16
}
```

**Expected output**

```json
{
  "signatures": [
    {
      "label": "add(a: number, b: number): number",
      "documentation": {
        "kind": "markdown",
        "value": "A simple function that adds two numbers"
      },
      "parameters": [
        { "label": [4, 13] },
        { "label": [15, 23] }
      ]
    }
  ],
  "activeSignature": 0,
  "activeParameter": 1
}
```

**Notes**

- Returns `"No signature help available at this location"` as a string when the
  server returns `null`.
- Returns `[]` if the server does not declare `signatureHelpProvider`.
- `activeParameter` is 0-based and indicates which parameter the cursor is on.

---

### `get_code_actions`

Retrieve code actions (quick fixes, refactorings) available for a text range,
via `textDocument/codeAction`. The server receives the range and any
diagnostics that overlap it, then returns a list of applicable actions.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier |
| `start_line` | number | yes | Start line of the selection (1-based) |
| `start_column` | number | yes | Start column (1-based) |
| `end_line` | number | yes | End line (1-based) |
| `end_column` | number | yes | End column (1-based) |

The range start must not be after the range end (validated by the schema).

**Example call** — selection over `undefinedVariable` on line 44

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "start_line": 44,
  "start_column": 15,
  "end_line": 44,
  "end_column": 30
}
```

**Expected output**

```json
[
  {
    "title": "Add missing import",
    "kind": "quickfix",
    "diagnostics": [ { "message": "Cannot find name 'undefinedVariable'.", "..." : "..." } ],
    "edit": {
      "changes": {
        "file:///path/to/example.ts": [
          { "range": { "...": "..." }, "newText": "import { undefinedVariable } from './somewhere';\n" }
        ]
      }
    }
  },
  {
    "title": "Declare 'undefinedVariable'",
    "kind": "quickfix",
    "command": {
      "title": "Declare variable",
      "command": "_typescript.applyRefactoring",
      "arguments": [ "..." ]
    }
  }
]
```

**Notes**

- Returns `[]` if the server does not declare `codeActionProvider`.
- The tool passes overlapping diagnostics automatically via
  `getOverlappingDiagnostics`; you do not need to supply them manually.
- Actions with an `edit` field can be applied with `apply_edit`. Actions with a
  `command` field must be triggered with `execute_command`.

---

### `get_document_symbols`

List all symbols defined in a file (functions, classes, interfaces, variables,
methods, etc.) via `textDocument/documentSymbol`. Returns a hierarchical
`DocumentSymbol` tree when the server supports it, or a flat
`SymbolInformation[]` list otherwise.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier |

**Example call**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript"
}
```

**Expected output** (hierarchical form)

```json
[
  {
    "name": "add",
    "kind": 12,
    "range": {
      "start": { "line": 3, "character": 0 },
      "end": { "line": 5, "character": 1 }
    },
    "selectionRange": {
      "start": { "line": 3, "character": 16 },
      "end": { "line": 3, "character": 19 }
    }
  },
  {
    "name": "Person",
    "kind": 11,
    "range": { "start": { "line": 10, "character": 0 }, "end": { "line": 15, "character": 1 } },
    "selectionRange": { "start": { "line": 10, "character": 17 }, "end": { "line": 10, "character": 23 } },
    "children": [
      { "name": "name", "kind": 7, "...": "..." },
      { "name": "age",  "kind": 7, "...": "..." },
      { "name": "email","kind": 7, "...": "..." }
    ]
  },
  {
    "name": "Greeter",
    "kind": 5,
    "children": [
      { "name": "constructor", "kind": 9, "...": "..." },
      { "name": "greet",       "kind": 6, "...": "..." }
    ]
  }
]
```

Symbol `kind` values: `4`=Constructor, `5`=Class, `6`=Method, `7`=Property,
`9`=Enum, `11`=Interface, `12`=Function, `13`=Variable, etc. (LSP §3.16.1).

**Notes**

- Returns `[]` if the server does not declare `documentSymbolProvider`.
- Coordinates in the output are 0-based (LSP native); add 1 when passing them
  to other tools.

---

### `get_workspace_symbols`

Search for symbols across the entire workspace via `workspace/symbol`. Provide
an empty query to enumerate all indexed symbols, or a substring to filter by
name.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | yes | Search string. Use `""` to list all symbols. |

**Example call**

```json
{ "query": "Greeter" }
```

**Expected output**

```json
[
  {
    "name": "Greeter",
    "kind": 5,
    "location": {
      "uri": "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
      "range": {
        "start": { "line": 19, "character": 0 },
        "end": { "line": 32, "character": 1 }
      }
    }
  }
]
```

**Notes**

- Returns `[]` if the server does not declare `workspaceSymbolProvider`. Some
  servers (e.g., tsserver) require at least one file to be open before workspace
  symbol search is available.
- Unlike `get_document_symbols`, this tool does not take a `file_path` — it
  queries the whole workspace index.
- Result coordinates are 0-based (LSP native).

---

## Navigation tools

### `get_references`

Find all locations where a symbol is referenced across the workspace, via
`textDocument/references`.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the symbol |
| `language_id` | string | yes | Language identifier |
| `line` | number | yes | Line of the symbol (1-based) |
| `column` | number | yes | Column of the symbol (1-based) |
| `include_declaration` | boolean | no | Include the symbol's own declaration. Default `false`. |

**Example call**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17,
  "include_declaration": true
}
```

**Actual output** (empty — tsserver not fully indexed in this session)

```json
[]
```

**Expected output — when workspace is indexed**

```json
[
  {
    "file": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
    "line": 4,
    "column": 17,
    "end_line": 4,
    "end_column": 20
  },
  {
    "file": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts",
    "line": 1,
    "column": 10,
    "end_line": 1,
    "end_column": 13
  },
  {
    "file": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts",
    "line": 4,
    "column": 13,
    "end_line": 4,
    "end_column": 16
  }
]
```

Output coordinates are 1-based (converted from LSP 0-based by the tool).

**Notes**

- Returns `[]` if the workspace is still indexing. The tool waits for
  `$/progress` end events from gopls; tsserver does not emit these, so on first
  call you may need to retry after a short delay.
- `include_declaration: true` adds the definition site to the results.
- Each result includes `file` (absolute path, not a URI), plus `line`,
  `column`, `end_line`, `end_column` (all 1-based).

---

### `go_to_definition`

Jump to where a symbol is defined, via `textDocument/definition`.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the usage |
| `language_id` | string | yes | Language identifier |
| `line` | number | yes | Line of the symbol (1-based) |
| `column` | number | yes | Column (1-based) |

**Example call** — `add` usage in consumer.ts line 4

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 13
}
```

**Expected output**

```json
[
  {
    "file": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
    "line": 4,
    "column": 17,
    "end_line": 4,
    "end_column": 20
  }
]
```

**Notes**

- Returns `[]` if the server does not declare `definitionProvider`.
- The tool normalizes `LocationLink[]` (targetUri/targetRange) to the same
  `{ file, line, column, end_line, end_column }` shape as `Location[]`.
- For built-in types (e.g., `string`, `number`), the server may return a
  location inside a bundled `.d.ts` declaration file.

---

### `go_to_type_definition`

Navigate to the declaration of the *type* of a symbol, rather than the symbol
itself, via `textDocument/typeDefinition`.

**Parameters** — identical to `go_to_definition`

**Example call** — `alice` variable in consumer.ts (type is `Person`)

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts",
  "language_id": "typescript",
  "line": 7,
  "column": 9
}
```

**Expected output**

```json
[
  {
    "file": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
    "line": 11,
    "column": 18,
    "end_line": 15,
    "end_column": 2
  }
]
```

**Notes**

- Returns `[]` if the server does not declare `typeDefinitionProvider`.
- Particularly useful with variables, parameters, and return values where
  `go_to_definition` would land on the variable declaration rather than the
  type interface.

---

### `go_to_implementation`

Find all concrete implementations of an interface or abstract method, via
`textDocument/implementation`.

**Parameters** — identical to `go_to_definition`

**Example call** — on an interface method

```json
{
  "file_path": "/path/to/project/src/interfaces.ts",
  "language_id": "typescript",
  "line": 5,
  "column": 3
}
```

**Expected output**

```json
[
  {
    "file": "/path/to/project/src/implementations/FooImpl.ts",
    "line": 12,
    "column": 3,
    "end_line": 16,
    "end_column": 4
  }
]
```

**Notes**

- Returns `[]` if the server does not declare `implementationProvider`.
- For a concrete class with no implementations below it, the server typically
  returns the class definition itself.

---

### `go_to_declaration`

Navigate to the *declaration* of a symbol, as distinct from its definition, via
`textDocument/declaration`. In most languages the declaration and definition are
the same location. This tool is most useful for C/C++ where a function can be
declared in a header and defined in a source file.

**Parameters** — identical to `go_to_definition`

**Example call** — C++ function declared in header

```json
{
  "file_path": "/path/to/project/src/main.cpp",
  "language_id": "cpp",
  "line": 15,
  "column": 5
}
```

**Expected output** (C++ clangd example)

```json
[
  {
    "file": "/path/to/project/include/utils.h",
    "line": 8,
    "column": 5,
    "end_line": 8,
    "end_column": 15
  }
]
```

**Notes**

- Returns `[]` if the server does not declare `declarationProvider`.
- For TypeScript and Go, `go_to_declaration` and `go_to_definition` typically
  return the same location. The tool exists to complete the full LSP navigation
  family and is most valuable with C/C++ servers (clangd) and similar languages
  with header/source splits.

---

## Refactoring tools

### `rename_symbol`

Compute a `WorkspaceEdit` for renaming a symbol everywhere it is used in the
workspace, via `textDocument/rename`. The edit is returned for inspection and
is **not applied automatically**. Pass it to `apply_edit` to commit the changes.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the symbol to rename |
| `language_id` | string | yes | Language identifier |
| `line` | number | yes | Line of the symbol (1-based) |
| `column` | number | yes | Column (1-based) |
| `new_name` | string | yes | The replacement name |

**Example call** — rename `add` to `sum`

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17,
  "new_name": "sum"
}
```

**Expected output**

```json
{
  "changes": {
    "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts": [
      {
        "range": {
          "start": { "line": 3, "character": 16 },
          "end": { "line": 3, "character": 19 }
        },
        "newText": "sum"
      }
    ],
    "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/consumer.ts": [
      {
        "range": {
          "start": { "line": 0, "character": 9 },
          "end": { "line": 0, "character": 12 }
        },
        "newText": "sum"
      },
      {
        "range": {
          "start": { "line": 3, "character": 12 },
          "end": { "line": 3, "character": 15 }
        },
        "newText": "sum"
      }
    ]
  }
}
```

The output is a raw `WorkspaceEdit` object. Coordinates are 0-based (LSP
native).

**Notes**

- Returns `"Rename not supported or symbol cannot be renamed at this location"`
  as a string when the server returns `null`.
- Returns `null` if the server does not declare `renameProvider`.
- Use `prepare_rename` first to validate the rename before calling this.
- Pass the returned object directly to `apply_edit` to write the changes to disk.

---

### `prepare_rename`

Validate that a rename operation is possible at the given position before
committing to it, via `textDocument/prepareRename`. Returns the range that
would be renamed and a suggested placeholder name.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the symbol |
| `language_id` | string | yes | Language identifier |
| `line` | number | yes | Line (1-based) |
| `column` | number | yes | Column (1-based) |

**Example call**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17
}
```

**Expected output**

```json
{
  "range": {
    "start": { "line": 3, "character": 16 },
    "end": { "line": 3, "character": 19 }
  },
  "placeholder": "add"
}
```

**Notes**

- Returns `"Rename not supported at this position"` as a string when the server
  returns `null`.
- Returns `null` if the server does not declare `renameProvider` with
  `prepareProvider: true`. The tool checks this flag explicitly and skips the
  request if it is absent.
- Coordinates in the result are 0-based.

---

### `format_document`

Compute formatting edits for an entire file via `textDocument/formatting`.
Returns `TextEdit[]` describing what the formatter would change. Edits are
**not applied automatically** — pass the result to `apply_edit` if you want to
write the formatted output to disk.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier |
| `tab_size` | number | no | Spaces per tab. Default `2`. |
| `insert_spaces` | boolean | no | Use spaces instead of tabs. Default `true`. |

**Example call**

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "tab_size": 2,
  "insert_spaces": true
}
```

**Expected output** (already-formatted file returns empty array)

```json
[]
```

**Expected output — file needing formatting**

```json
[
  {
    "range": {
      "start": { "line": 5, "character": 0 },
      "end": { "line": 5, "character": 4 }
    },
    "newText": "  "
  }
]
```

Each `TextEdit` has a 0-based `range` and a `newText` replacement string.

**Notes**

- Returns `[]` if the server does not declare `documentFormattingProvider`.
- The returned `TextEdit[]` can be wrapped in a `WorkspaceEdit` for `apply_edit`:
  `{ "changes": { "file:///path/to/file.ts": [ ...edits ] } }`.

---

### `format_range`

Compute formatting edits for a selected range within a file via
`textDocument/rangeFormatting`. Otherwise identical to `format_document` but
scoped to specific lines.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | yes | Language identifier |
| `start_line` | number | yes | Start line (1-based) |
| `start_column` | number | yes | Start column (1-based) |
| `end_line` | number | yes | End line (1-based) |
| `end_column` | number | yes | End column (1-based) |
| `tab_size` | number | no | Default `2` |
| `insert_spaces` | boolean | no | Default `true` |

Range start must not be after range end (schema-validated).

**Example call** — format only the `Greeter` class

```json
{
  "file_path": "/Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "start_line": 20,
  "start_column": 1,
  "end_line": 33,
  "end_column": 2
}
```

**Expected output**

```json
[]
```

(No changes needed for already-formatted source.)

**Notes**

- Returns `[]` if the server does not declare `documentRangeFormattingProvider`.
- Not all language servers support range formatting even if they support document
  formatting. Check the server's capabilities if this returns `[]` unexpectedly.

---

### `apply_edit`

Write a `WorkspaceEdit` to disk and notify the language server of the changes.
Pass the object returned by `rename_symbol`, `format_document`, or
`format_range` directly.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `workspace_edit` | object | yes | A `WorkspaceEdit` with either `changes` (Record&lt;uri, TextEdit[]&gt;) or `documentChanges` (TextDocumentEdit[]) |

**Example call** — applying a rename edit

```json
{
  "workspace_edit": {
    "changes": {
      "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts": [
        {
          "range": {
            "start": { "line": 3, "character": 16 },
            "end": { "line": 3, "character": 19 }
          },
          "newText": "sum"
        }
      ]
    }
  }
}
```

**Actual output**

```
Workspace edit applied successfully
```

**Notes**

- Edits within each file are applied in reverse order (bottom-to-top) so that
  earlier offsets remain valid as later text is replaced.
- After writing files to disk, the tool sends `textDocument/didChange` for each
  modified file to keep the language server in sync.
- `documentChanges` (array of `TextDocumentEdit`) and `changes` (object) forms
  are both supported.
- This tool writes to disk immediately — make sure the edit looks correct before
  calling it.

---

### `execute_command`

Execute a server-defined command via `workspace/executeCommand`. Commands are
returned in the `command` field of code actions (from `get_code_actions`) and
may also be listed in the server's `executeCommandProvider.commands` capability.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `command` | string | yes | Command identifier (e.g., `_typescript.applyRefactoring`) |
| `args` | array | no | Arguments to pass to the command |

**Example call** — triggering a TypeScript refactoring command

```json
{
  "command": "_typescript.applyRefactoring",
  "args": [
    "/path/to/file.ts",
    "refactorRewrite",
    "Add return type",
    { "startLine": 35, "startOffset": 1, "endLine": 37, "endOffset": 2 }
  ]
}
```

**Expected output — command with a result**

```json
{
  "edits": [
    {
      "fileName": "/path/to/file.ts",
      "textChanges": [ "..." ]
    }
  ]
}
```

**Expected output — command with no result**

```
Command executed successfully (no result returned)
```

**Notes**

- Returns `null` if the server does not declare `executeCommandProvider`.
- The available commands and their argument shapes are server-specific. Use
  `get_code_actions` to discover commands rather than constructing them manually.
- Some commands apply changes server-side and push `workspace/applyEdit`
  requests; others return an edit that you must apply with `apply_edit`.

---

## Utilities

### `did_change_watched_files`

Notify the language server that files have changed on disk outside the editor
context, via `workspace/didChangeWatchedFiles`. Use this after writing files
directly to disk (e.g., after code generation or template expansion) so the
server refreshes its caches.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `changes` | array | yes | Array of `{ uri, type }` objects. `uri` must use the `file:///` scheme. `type`: `1`=created, `2`=changed, `3`=deleted. |

**Example call**

```json
{
  "changes": [
    {
      "uri": "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/newfile.ts",
      "type": 1
    },
    {
      "uri": "file:///Users/dayna.blackwell/code/LSP-MCP/test/ts-project/src/example.ts",
      "type": 2
    }
  ]
}
```

**Actual output**

```
Notified server of 2 file change(s)
```

**Notes**

- Sends a `workspace/didChangeWatchedFiles` notification (fire-and-forget); the
  server does not send a response.
- This is a notification, not a request, so there is no success/failure from
  the server side.
- Follow up with `get_diagnostics` after calling this if you want to verify the
  server picked up the changes.

---

### `set_log_level`

Control the verbosity of logs written by the lsp-mcp server process.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `level` | string | yes | One of: `debug`, `info`, `notice`, `warning`, `error`, `critical`, `alert`, `emergency` |

Levels from least to most verbose: `emergency` → `alert` → `critical` →
`error` → `warning` → `notice` → `info` → `debug`.

**Example call**

```json
{ "level": "warning" }
```

**Actual output**

```
Log level set to: warning
```

**Notes**

- The default level is `info`.
- Set to `debug` when troubleshooting: the server logs every LSP message sent
  and received in full JSON, including `$/progress`, `workspace/configuration`,
  and `client/registerCapability` server-initiated requests.
- At `warning` and above, only error conditions and lifecycle events are logged.
  Useful in production to reduce noise.
- This affects the MCP server's own logging only, not the underlying language
  server's verbosity.

---

## Startup and warm-up notes

The tsserver (and some other language servers) perform asynchronous workspace
indexing after `initialize`. During this period:

- `get_info_on_location` may return empty string.
- `get_references` may return `[]`.
- `get_diagnostics` may return empty diagnostic arrays.

The server handles three server-initiated requests that must be responded to
before workspace loading completes:

1. `window/workDoneProgress/create` — pre-registers a progress token.
2. `workspace/configuration` — the server returns `null` for each requested
   config item.
3. `client/registerCapability` — acknowledged with `null`.

lsp-mcp handles all three automatically. For `get_references`, the client
additionally waits for all `$/progress` end events before returning. tsserver
does not emit `$/progress`, so references may require a brief wait and retry
on first use. Set `set_log_level` to `debug` and look for `Progress end:` log
lines to confirm when the server is ready.
