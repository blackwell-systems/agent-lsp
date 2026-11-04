# agent-lsp Tool Reference

All 66 tools exposed by the agent-lsp MCP server: 60 core tools, 3 intent
aliases (`callers`, `explore`, `safe_edit`), and 3 phase-enforcement tools
(`activate_skill`, `deactivate_skill`, `get_skill_phase`). Coordinates are
**1-based** for both `line` and `column` in every tool call; the server converts
internally to the 0-based values the LSP spec requires.

**Output format.** Structured results in this reference are shown as JSON for
readability. The default wire format is GCF, a compact text encoding that uses
fewer tokens; set `AGENT_LSP_OUTPUT_FORMAT=json` in the server's environment to
get JSON instead. Results that must round-trip exactly are always JSON whatever
the setting: the `rename_symbol` dry-run preview and the `refactor` results.
Plain-text results (status messages such as `Document opened: ...`) are the same
in both formats.

---

## Table of Contents

- [Session tools](#session-tools): `start_lsp`, `restart_lsp_server`, `open_document`, `close_document`, `add_workspace_folder`, `remove_workspace_folder`, `list_workspace_folders`
- [Analysis tools](#analysis-tools): `get_diagnostics`, `inspect_symbol`, `get_completions`, `get_signature_help`, `suggest_fixes`, `list_symbols`, `find_symbol`, `blast_radius`, `get_cross_repo_references`, `detect_changes`
- [Composite exploration tools](#composite-exploration-tools): `explore_symbol`
- [Safe editing tools](#safe-editing-tools): `safe_apply_edit`
- [Intent aliases](#intent-aliases): `callers`, `explore`, `safe_edit`
- [Navigation tools](#navigation-tools): `find_references`, `go_to_definition`, `go_to_type_definition`, `go_to_implementation`, `go_to_declaration`
- [Refactoring tools](#refactoring-tools): `rename_symbol`, `prepare_rename`, `refactor`, `format_document`, `format_range`, `apply_edit`, `execute_command`
- [Symbol editing tools](#symbol-editing-tools): `replace_symbol_body`, `insert_after_symbol`, `insert_before_symbol`, `safe_delete_symbol`
- [Utilities](#utilities): `did_change_watched_files`, `set_log_level`
- [Code Intelligence tools](#code-intelligence-tools): `find_callers`, `type_hierarchy`, `get_inlay_hints`, `get_semantic_tokens`, `get_document_highlights`
- [Build & Test tools](#build--test-tools): `run_build`, `run_tests`, `get_tests_for_file`
- [Server Introspection tools](#server-introspection-tools): `get_server_capabilities`, `detect_lsp_servers`
- [Cache tools](#cache-tools): `export_cache`, `import_cache`
- [Simulation tools](#simulation-tools): `create_simulation_session`, `simulate_edit`, `evaluate_session`, `simulate_chain`, `commit_session`, `discard_session`, `destroy_session`, `preview_edit`
- [Startup and warm-up notes](#startup-and-warm-up-notes)
- [Symbol lookup tools](#symbol-lookup-tools): `go_to_symbol`, `get_symbol_source`, `get_symbol_documentation`
- [Phase enforcement tools](#phase-enforcement-tools): `activate_skill`, `deactivate_skill`, `get_skill_phase`
- [Skills](#skills)

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
| `language_id` | string | no | In multi-server mode, selects a specific configured server (e.g. `"go"` targets gopls, `"typescript"` targets typescript-language-server). Without this, all configured servers are started. Use `get_server_capabilities` to diagnose which server is active. |
| `connect` | string | no | Connect to an already-running language server at this TCP address (e.g. `localhost:9999`) instead of spawning a new process. Reuses the existing server's warm index with zero duplicate memory. Supported by gopls (`gopls -listen=:9999`), clangd, and other servers with TCP listen mode. |
| `ready_timeout_seconds` | number | no | If > 0, blocks until all `$/progress` workspace-indexing tokens complete or this many seconds elapse. Useful for servers like jdtls that index asynchronously after initialize. Fires as soon as indexing completes (does not always wait the full timeout). |
| `scope` | string | no | Limit indexing to specific subdirectories of `root_dir`: one relative path, or a JSON array of paths encoded as a string (e.g. `"[\"services/api\", \"libs/core\"]"`). Generates a temporary language-server config (`pyrightconfig.json`, `tsconfig.json`) that restricts analysis scope. Use on large monorepos to prevent reference-query timeouts. |

**Example call**

```json
{
  "root_dir": "/home/user/projects/agent-lsp/test/ts-project"
}
```

**Actual output**

```
LSP server started successfully
```

The result also carries a second text item with a next-step hint. With
`connect`, the message is `Connected to existing language server at <address>`.

**Notes**

- Shuts down the existing LSP process before starting the new one, so there is no resource
  leak.
- After `start_lsp` returns, the underlying language server is initialized but
  may not have finished indexing the workspace. For `find_references` on large
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
LSP server restarted successfully. Note: in multi-server configurations only the default server was restarted; other configured servers are unaffected. Re-opened 2 document(s) from the previous session on the fresh server.
```

**Notes**

- Requires the LSP client to already be initialized. Returns an error if
  `start_lsp` has never been called.
- Documents that were open before the restart are re-opened on the fresh
  server automatically; the message reports how many. You do not need to call
  `open_document` again for them.
- In multi-server mode only the default server is restarted.

---

### `open_document`

Register a file with the language server for analysis. Most analysis tools
(`inspect_symbol`, `get_completions`, `find_references`, etc.) call this
internally via the `withDocument` helper, so you typically only need to call
it explicitly when you want to pre-warm a file or keep it open across multiple
operations.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier (`typescript`, `javascript`, `go`, `python`, `rust`, etc.). Auto-detected from the file extension when omitted. |
| `text` | string | no | File content to send instead of reading the file from disk. |

**Example call**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
  "language_id": "typescript"
}
```

**Actual output**

```
Document opened: /home/user/projects/agent-lsp/test/ts-project/src/example.ts
```

**Notes**

- Idempotent: opening an already-open file is safe; it re-sends `didOpen` so
  the server refreshes its view of the file content.
- Without `text`, the file must exist on disk; the tool reads its content before sending it to the language server.
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
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/consumer.ts"
}
```

**Actual output**

```
Document closed: /home/user/projects/agent-lsp/test/ts-project/src/consumer.ts
```

**Notes**

- Good practice in long sessions or large codebases to close files you are
  done analyzing.
- `get_diagnostics` (no `file_path`) only returns diagnostics for currently
  open files, so closing a file removes it from those results.

---

### `add_workspace_folder`

Add a directory to the LSP workspace, enabling cross-repo references, definitions,
and diagnostics. After adding a folder the language server re-indexes it, so call
sites and symbol definitions in both repos become visible to each other.

Useful pattern: `start_lsp` on a library repo, then `add_workspace_folder` for a
consumer repo. `find_references` on a library function then returns call sites in
both projects.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | yes | Absolute path to the directory to add |

**Example call**

```json
{ "path": "/home/user/projects/my-app" }
```

**Actual output**

```json
{
  "added": "/home/user/projects/my-app",
  "workspace_folders": [
    { "uri": "file:///home/user/projects/my-library", "name": "/home/user/projects/my-library" },
    { "uri": "file:///home/user/projects/my-app",     "name": "/home/user/projects/my-app" }
  ]
}
```

**Notes**

- Requires `start_lsp` to have been called first.
- Supported by gopls, rust-analyzer, typescript-language-server. Servers that do not support multi-root workspaces silently ignore the notification.
- Idempotent: adding a folder that is already present is a no-op.

---

### `remove_workspace_folder`

Remove a directory from the LSP workspace. The server stops indexing that folder.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | yes | Absolute path to the directory to remove |

---

### `list_workspace_folders`

Return the current list of workspace folders the server is indexing.

**Parameters:** none

**Actual output**

```json
{
  "workspace_folders": [
    { "uri": "file:///home/user/projects/my-library", "name": "/home/user/projects/my-library" }
  ]
}
```

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
| `group_by` | string | no | Set to `"symbol"` to group diagnostics by their owning symbol (function, type) instead of returning a flat list per file. Helps show which symbol is broken. |

**Example call (single file)**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts"
}
```

**Actual output (clean file)**

```json
{
  "file:///home/user/projects/agent-lsp/test/ts-project/src/example.ts": []
}
```

**Actual output (all open files)**

```json
{
  "file:///home/user/projects/agent-lsp/test/ts-project/src/example.ts": [],
  "file:///home/user/projects/agent-lsp/test/ts-project/src/consumer.ts": []
}
```

**Output shape (file with errors)**

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

### `inspect_symbol`

Retrieve hover information for a symbol at a specific position via
`textDocument/hover`. Returns type signatures, JSDoc/godoc comments, and other
contextual detail that the language server provides on hover.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes* | Line number (1-based) |
| `column` | number | yes* | Column position (1-based) |
| `position_pattern` | string | no | Alternative to `line`/`column`: text with `@@` marking the cursor (see [Position-pattern parameter](#position-pattern-parameter-position_pattern)) |

\* Not needed when `position_pattern` is provided.

**Example call**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17
}
```

**Expected output (TypeScript function)**

```
function add(a: number, b: number): number

A simple function that adds two numbers
```

**Expected output (TypeScript class)**

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
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes | Line number (1-based) |
| `column` | number | yes | Column position (1-based), typically just after a `.` or at the start of a partial identifier |

**Example call (after `greeter.`)**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/consumer.ts",
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
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes | Line of the call site (1-based) |
| `column` | number | yes | Column inside the argument list (1-based) |

**Example call** (cursor inside `add(1, ` on line 4 of consumer.ts)

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/consumer.ts",
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

### `suggest_fixes`

Retrieve code actions (quick fixes, refactorings) available for a text range,
via `textDocument/codeAction`. The server receives the range and any
diagnostics that overlap it, then returns a list of applicable actions.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `start_line` | number | yes | Start line of the selection (1-based) |
| `start_column` | number | yes | Start column (1-based) |
| `end_line` | number | yes | End line (1-based) |
| `end_column` | number | yes | End column (1-based) |

The range start must not be after the range end (validated by the schema).

**Example call** (selection over `undefinedVariable` on line 44)

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
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

### `list_symbols`

List all symbols defined in a file (functions, classes, interfaces, variables,
methods, etc.) via `textDocument/documentSymbol`. Returns a hierarchical
`DocumentSymbol` tree when the server supports it, or a flat
`SymbolInformation[]` list otherwise.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `format` | string | no | `"json"` (default) returns the full DocumentSymbol tree. `"outline"` returns a compact Markdown representation (`name [Kind] :line`, indented for children), ~5x fewer tokens; useful for structural surveys before targeted navigation. |

**Example call**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
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
- Coordinates in the output are **1-based**. The tool shifts all `range` and `selectionRange` values by +1 before returning, including in nested `children`. Pass them directly to other tools without adjustment.

---

### `find_symbol`

Search for symbols across the entire workspace via `workspace/symbol`. Provide
an empty query to enumerate all indexed symbols, or a substring to filter by
name. Optionally enrich results with hover documentation for a paginated window.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | no | Search string. Use `""` or omit to list all symbols. |
| `detail_level` | string | no | `"basic"` (default) returns name/kind/location only. `"hover"` returns the full structured response with hover-enriched `enriched[]` for the current pagination window. |
| `limit` | number | no | Number of symbols to enrich when `detail_level=hover`. Default `3`. |
| `offset` | number | no | Pagination offset into results for enrichment. Default `0`. |

**Example call (basic)**

```json
{ "query": "Greeter" }
```

**Expected output (basic)**

```json
[
  {
    "name": "Greeter",
    "kind": 5,
    "location": {
      "uri": "file:///home/user/projects/agent-lsp/test/ts-project/src/example.ts",
      "range": {
        "start": { "line": 19, "character": 0 },
        "end": { "line": 32, "character": 1 }
      }
    }
  }
]
```

**Example call (hover enriched)**

```json
{ "query": "Greeter", "detail_level": "hover", "limit": 2, "offset": 0 }
```

**Expected output (hover enriched)**

```json
{
  "total": 1,
  "symbols": [
    {
      "name": "Greeter",
      "kind": 5,
      "location": {
        "uri": "file:///path/to/example.ts",
        "range": { "start": { "line": 19, "character": 0 }, "end": { "line": 32, "character": 1 } }
      }
    }
  ],
  "enriched": [
    {
      "name": "Greeter",
      "kind": 5,
      "location": { "...": "..." },
      "hover": "class Greeter"
    }
  ],
  "pagination": { "offset": 0, "limit": 2, "more": false }
}
```

**Notes**

- Returns `[]` if the server does not declare `workspaceSymbolProvider`. Some
  servers (e.g., tsserver) require at least one file to be open before workspace
  symbol search is available.
- Unlike `list_symbols`, this tool does not take a `file_path`. It
  queries the whole workspace index.
- Result coordinates are 0-based (LSP native).
- With `detail_level=hover`, `symbols[]` always contains the full result set.
  Use `offset` to page through the `enriched[]` window without re-running the
  workspace search.

---

### `blast_radius`

Enumerate all exported symbols in one or more files, resolve their references
across the workspace, and partition callers into test vs non-test. Returns
enclosing test function names for test references. Use before editing a file to
understand blast radius.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `changed_files` | string[] | yes | Absolute paths to the files whose exported symbols should be analyzed |
| `include_transitive` | boolean | no | Set to `true` to also resolve references for each caller (second-order callers). Default: `false` |
| `scope` | string | no | `"exported"` (default) analyzes exported symbols only. `"all"` also includes unexported symbols, for dead-code detection. |
| `filter` | string | no | `"untested"` keeps only symbols that have production callers but zero test callers. |

**Example call**

```json
{
  "changed_files": ["/abs/path/to/internal/lsp/client.go"],
  "include_transitive": false
}
```

**Returns**

```json
{
  "changed_symbols": [
    { "name": "LSPClient", "file": "internal/lsp/client.go", "line": 14 }
  ],
  "affected_symbols": [
    {
      "name": "LSPClient",
      "file": "internal/lsp/client.go",
      "line": 14,
      "test_callers": [
        { "name": "TestGetReferences", "file": "internal/lsp/client_test.go", "line": 42 }
      ],
      "non_test_callers": [
        { "name": "HandleGetReferences", "file": "internal/tools/analysis.go", "line": 67 }
      ],
      "sync_guarded": true
    }
  ],
  "test_files": [
    "internal/lsp/client_test.go"
  ],
  "test_functions": [
    { "name": "TestGetReferences", "file": "internal/lsp/client_test.go", "line": 42 }
  ],
  "non_test_callers": [
    { "name": "HandleGetReferences", "file": "internal/tools/analysis.go", "line": 67 }
  ],
  "summary": "Found 1 changed symbols with 1 test references across 1 test files.",
  "warnings": []
}
```

In the default GCF format the result is a graph: changed symbols, their
callers, and test functions as nodes, with caller-to-symbol edges.

**Notes**

- By default only exported symbols are analyzed (uppercase identifiers in Go; all public symbols in other languages). Pass `scope: "all"` to include unexported ones.
- `changed_symbols` lists each analyzed symbol with its file and 1-based line number.
- `affected_symbols` gives each symbol its own `test_callers` and `non_test_callers`, so you can see which tests cover which function. `filter: "untested"` applies to this list.
- `test_functions` contains the enclosing test function name for each test file reference.
- `test_files` is the deduplicated set of test files that reference any changed symbol.
- `non_test_callers` (top level) is the flat blast radius for production code across all symbols.
- `sync_guarded`: present and `true` on an `affected_symbols` entry that is a method on a type containing synchronization primitives (Mutex, RWMutex, Lock, atomic). Covers Go, Java, Rust, Python, C/C++ primitives. Helps agents distinguish mutex-guarded code from pure functions when assessing risk.
- `warnings` contains messages for any `GetReferences` calls that failed (non-fatal).

---

### `get_cross_repo_references`

Find all references to a library symbol across one or more consumer
repositories. Adds each consumer root as a workspace folder, waits for
indexing, then calls `find_references` and partitions results by repo root.
Use before changing a shared library API to find all downstream callers.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbol_file` | string | yes | Absolute path to the file containing the symbol definition |
| `line` | integer | yes | 1-based line number of the symbol |
| `column` | integer | yes | 1-based column number of the symbol |
| `consumer_roots` | string[] | yes | Absolute paths to consumer repo roots to search |
| `language_id` | string | no | Language identifier (e.g. `"go"`). Auto-detected from the file extension when omitted. |

**Example call**

```json
{
  "symbol_file": "/repos/config-lib/pkg/config/parser.go",
  "line": 42,
  "column": 6,
  "consumer_roots": ["/repos/api-service", "/repos/worker-service"]
}
```

**Returns**

```json
{
  "symbol": "Parse",
  "references": [
    { "file": "/repos/config-lib/pkg/config/parser_test.go", "line": 18, "column": 3, "repo": "primary" },
    { "file": "/repos/api-service/main.go", "line": 14, "column": 9, "repo": "/repos/api-service" },
    { "file": "/repos/worker-service/runner.go", "line": 8, "column": 5, "repo": "/repos/worker-service" }
  ],
  "summary": "Found 3 references across 3 repos."
}
```

**Notes**

- `references` is one flat list; each entry's `repo` names the consumer root
  that contains the file, or `"primary"` for references in the library
  workspace itself. All positions are 1-based.
- `warnings` is present only when a consumer root could not be added as a
  workspace folder; re-add it manually if so.
- In the default GCF format the result is a graph of the symbol and its
  referencing locations.
- Requires `start_lsp` on the library root first.

---

### `detect_changes`

Run `git diff` to identify changed files, analyze their impact via
`blast_radius`, and classify each affected symbol by risk level. A single
call that answers "what did I break?" without manually listing changed files.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `workspace_root` | string | no | Workspace root directory; defaults to the LSP root if omitted |
| `scope` | string | no | Git diff scope: `"unstaged"` (default), `"staged"`, or `"committed"` |
| `range` | string | no | Git range for `"committed"` scope (e.g., `"v0.7.0..HEAD"`, `"abc123..def456"`, or a single ref like `"main"` which expands to `main~1..main`). Ignored for unstaged/staged scopes. |

**Example call**

```json
{
  "workspace_root": "/home/user/myproject",
  "scope": "staged"
}
```

**Returns** (with `AGENT_LSP_OUTPUT_FORMAT=json`; abbreviated)

```json
{
  "changed_files": ["/home/user/myproject/pkg/handler.go"],
  "changed_symbols": [
    { "name": "ServeHTTP", "file": "pkg/handler.go", "line": 42, "risk": "high" }
  ],
  "non_test_callers": [
    { "name": "ServeHTTP", "file": "cmd/server/main.go", "line": 15 }
  ],
  "scope": "staged"
}
```

The JSON result carries the rest of the `blast_radius` fields
(`affected_symbols`, `test_files`, `test_functions`, `summary`, `warnings`) as
well.

**Notes**

- Risk classification is added to each `changed_symbols` entry in JSON mode only: `"high"` (callers from multiple packages), `"medium"` (callers from the same package only), `"low"` (zero non-test callers). In the default GCF format, `detect_changes` returns the `blast_radius` graph as is, without `risk`, `changed_files`, or `scope`.
- Filters out non-source files (plaintext, deleted) before analysis.
- Delegates to `blast_radius` internally, so results benefit from the persistent reference cache.

---

## Cache tools

### `export_cache`

Export the persistent reference cache as a gzip-compressed artifact. The
cache is compacted with `VACUUM INTO` before compression. Use this to share
a warm cache with teammates: export, commit the `.gz` file, and teammates
import it to skip cold-start indexing.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `dest_path` | string | yes | Absolute path for the output `.gz` file |

**Example call**

```json
{
  "dest_path": "/home/user/myproject/.agent-lsp/cache.db.gz"
}
```

**Returns**

```
Cache exported to /home/user/myproject/.agent-lsp/cache.db.gz (1,247 entries)
```

---

### `import_cache`

Import a gzip-compressed cache artifact, replacing the current cache contents.
The artifact is decompressed, validated with `PRAGMA integrity_check`, and
atomically swapped into the active database.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `src_path` | string | yes | Absolute path to the `.gz` cache artifact |

**Example call**

```json
{
  "src_path": "/home/user/myproject/.agent-lsp/cache.db.gz"
}
```

**Returns**

```
Cache imported from /home/user/myproject/.agent-lsp/cache.db.gz (1,247 entries)
```

**Notes**

- The existing cache is closed and replaced atomically.
- If the artifact fails integrity check, the import is rejected and the existing cache remains unchanged.
- Both `export_cache` and `import_cache` require an active LSP session.

---

## Composite exploration tools

### `explore_symbol`

Deep-dive into a symbol: combines type info, source code, callers (top 10),
references (count + top 5 files), and test caller count in one call. Use when
you need full context about a symbol before editing.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the source file |
| `line` | number | no | 1-indexed line number. Optional when `position_pattern` is provided. |
| `column` | number | no | 1-indexed column. Optional when `position_pattern` is provided. |
| `position_pattern` | string | no | Alternative to line/column: use `@@pattern@@` syntax to match text near the target position |
| `language_id` | string | no | Language identifier (e.g. `"go"`, `"typescript"`). Auto-detected from file extension. |

**Example call**

```json
{
  "file_path": "/home/user/project/pkg/hub.go",
  "line": 42,
  "column": 6
}
```

**Notes**

- Composite tool: internally calls hover, get_symbol_source, find_callers, find_references
- Replaces the 4-5 tool sequence agents previously used to understand a symbol
- The `explore` alias provides the same functionality with a shorter name

---

## Safe editing tools

### `safe_apply_edit`

Preview an edit and apply it only if safe (net diagnostic delta == 0). Combines
`preview_edit` + `apply_edit` into one call. If the edit would introduce errors,
returns the preview result with `applied: false` so you can decide.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file to edit |
| `old_text` | string | yes | Exact text to find and replace |
| `new_text` | string | yes | Replacement text |

**Example call**

```json
{
  "file_path": "/home/user/project/pkg/hub.go",
  "old_text": "func Send(msg string)",
  "new_text": "func Send(ctx context.Context, msg string)"
}
```

**Notes**

- Returns `applied: true` on success, `applied: false` with preview diagnostics when the edit would introduce errors
- Agents skip the manual preview-then-apply two-step
- The `safe_edit` alias provides the same functionality with a shorter name

---

## Intent aliases

Three shorter, intent-oriented tool names for common operations. Same handlers
and parameters as the underlying tools. (`blast_radius` is a core tool, not an
alias; see [Analysis tools](#analysis-tools).)

### `callers`

Find all incoming callers of a function or method. Wraps `find_callers` with
`direction` forced to `"incoming"`. Same parameters as `find_callers`.

### `explore`

Composite symbol exploration (same handler as `explore_symbol`). Same parameters
as `explore_symbol`.

### `safe_edit`

Preview + apply when safe (same handler as `safe_apply_edit`). Same parameters
as `safe_apply_edit`.

---

## Navigation tools

### `find_references`

Find all locations where a symbol is referenced across the workspace, via
`textDocument/references`.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the symbol |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes* | Line of the symbol (1-based) |
| `column` | number | yes* | Column of the symbol (1-based) |
| `include_declaration` | boolean | no | Include the symbol's own declaration. Default `false`. |
| `position_pattern` | string | no | Alternative to `line`/`column`: text with `@@` marking the cursor (see [Position-pattern parameter](#position-pattern-parameter-position_pattern)) |

\* Not needed when `position_pattern` is provided.

**Example call**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17,
  "include_declaration": true
}
```

**Actual output** (empty; tsserver not fully indexed in this session)

```json
[]
```

**Expected output (when workspace is indexed)**

```json
[
  {
    "file": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
    "line": 4,
    "column": 17,
    "end_line": 4,
    "end_column": 20
  },
  {
    "file": "/home/user/projects/agent-lsp/test/ts-project/src/consumer.ts",
    "line": 1,
    "column": 10,
    "end_line": 1,
    "end_column": 13
  },
  {
    "file": "/home/user/projects/agent-lsp/test/ts-project/src/consumer.ts",
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
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes | Line of the symbol (1-based) |
| `column` | number | yes | Column (1-based) |

**Example call** (`add` usage in consumer.ts line 4)

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/consumer.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 13
}
```

**Expected output**

```json
[
  {
    "file": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
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

**Parameters:** identical to `go_to_definition`

**Example call** (`alice` variable in consumer.ts, type is `Person`)

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/consumer.ts",
  "language_id": "typescript",
  "line": 7,
  "column": 9
}
```

**Expected output**

```json
[
  {
    "file": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
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

**Parameters:** identical to `go_to_definition`

**Example call** (on an interface method)

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

**Parameters:** identical to `go_to_definition`

**Example call** (C++ function declared in header)

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

Rename a symbol everywhere it is used in the workspace, via
`textDocument/rename`. The tool computes the `WorkspaceEdit` and **applies it to
disk** itself, then returns a text summary. Pass `dry_run: true` to preview the
edit without writing anything.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the symbol to rename |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes* | Line of the symbol (1-based) |
| `column` | number | yes* | Column (1-based) |
| `position_pattern` | string | no | Alternative to `line`/`column`: text with `@@` marking the cursor (see [Position-pattern parameter](#position-pattern-parameter-position_pattern)) |
| `new_name` | string | yes | The replacement name |
| `dry_run` | boolean | no | If `true`, return the edit preview without writing to disk. Default `false`. See [Dry-run preview](#dry-run-preview-for-rename_symbol). |
| `exclude_globs` | array of strings | no | Glob patterns for files to skip (e.g. ["vendor/**", "**/*_gen.go"]). Matching files are left out of the edit. Uses filepath.Match syntax; also matched against the file's basename. |

\* Not needed when `position_pattern` is provided.

**Example call** (rename `add` to `sum`)

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17,
  "new_name": "sum"
}
```

**Expected output**

```
Renamed to "sum" across 3 location(s) in 2 file(s): example.ts, consumer.ts
```

A second text item reports the post-edit diagnostics, for example
`Next step: errors_after: 0, warnings_after: 0. Run get_diagnostics for details.`
When the rename edits files other than `file_path` and their diagnostics could
not be verified, the hint names them.

**Notes**

- When the server produces no edit (nothing to rename, the position does not
  resolve to a renameable symbol, or the server does not declare
  `renameProvider`), the tool returns
  `No rename edits produced for the symbol at <file>:<line>:<column> ...` and
  writes nothing.
- If the exact position yields no edit, the tool retries by looking the symbol
  up by its hover name before giving up.
- Use `prepare_rename` first to validate the position, and `dry_run: true` to
  see every site before applying.
- Do not pass the dry-run preview to `apply_edit`. To apply, call
  `rename_symbol` again without `dry_run`.
- `exclude_globs` patterns match against both the full file path and the
  basename. Common patterns: "vendor/**" (Go vendor tree),
  "**/*_generated.go" (codegen), "testdata/**" (test fixtures).

---

### `prepare_rename`

Validate that a rename operation is possible at the given position before
committing to it, via `textDocument/prepareRename`. Returns the range that
would be renamed and a suggested placeholder name.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the symbol |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes | Line (1-based) |
| `column` | number | yes | Column (1-based) |

**Example call**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
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

- When the server answers `null` for the position, returns
  `{"supported": true, "message": "The server returned no rename range at this position; a rename here may not be valid."}`.
- When the server does not support `textDocument/prepareRename` (no
  `renameProvider` with `prepareProvider: true`, or the method is not
  implemented), returns `{"supported": false, "message": "prepare_rename is not supported by this language server; rename_symbol may still work."}`.
  The tool checks the flag explicitly and skips the request if it is absent.
- Coordinates in the result are 0-based.

---

### `refactor`

Run a refactoring the language server offers at a position, including the
interactive refactorings that ask for input before computing their edit.
gopls v0.24+ uses these for adding or removing struct tags, implementing an
interface, and moving a declaration (move is experimental in gopls and needs
its `moveDeclaration` setting). The server computes the edit; agent-lsp
captures it before it is written, so `dry_run` can return it as a preview.

The flow has up to three calls:

1. Without `action`: list the refactorings at the position.
2. With `action`: if the refactoring needs input, the result has
   `"status": "needs_input"` and the questions (`fields`), each with an `id`,
   `kind`, `default`, and `options` where the answer is a choice.
3. With `action` and `answers`: run it. A rejected answer comes back as
   `needs_input` again with an `error` on that field.

Answers can be passed on the first call with `action` to skip step 2.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | File containing the code |
| `line` | number | yes* | Line (1-based) of the code, or the start of the selection |
| `column` | number | yes* | Column (1-based) |
| `position_pattern` | string | no | Alternative to `line`/`column` (`@@` syntax) |
| `end_line` | number | no | End line of the selection; some refactorings need one (select struct fields to add tags) |
| `end_column` | number | no | End column of the selection |
| `action` | string | no | Title of the refactoring to run (exact, or a unique part of it). Omit to list |
| `answers` | object | no | `{field id: value}` answers to the questions |
| `option_queries` | object | no | `{field id: search text}` to search a `lazyEnum` field's options (for example the interface to implement) |
| `dry_run` | boolean | no | Return the edit without writing it |
| `language_id` | string | no | Language identifier; auto-detected from the extension |

\* Or `position_pattern`.

**Example: list**

```json
{ "file_path": "/repo/main.go", "line": 3, "column": 6 }
```

```json
{"refactorings":[{"title":"Add methods to Person to implement an interface...","kind":"refactor.rewrite.implementInterface"}]}
```

**Example: questions**

```json
{ "file_path": "/repo/main.go", "line": 4, "column": 2, "end_line": 5, "end_column": 11, "action": "Add struct tags" }
```

```json
{
  "status": "needs_input",
  "action": "Add struct tags",
  "fields": [
    {"id": "addTags", "description": "comma-separated list of tags to add; e.g.. \"json,xml\"", "kind": "string", "required": true, "default": "json"},
    {"id": "transform", "kind": "enum", "required": true, "default": "camelcase",
     "options": [{"value": "camelcase", "description": "camelCase"}, {"value": "snakecase", "description": "snake_case"}, "..."]}
  ]
}
```

**Example: run**

```json
{ "file_path": "/repo/main.go", "line": 4, "column": 2, "end_line": 5, "end_column": 11,
  "action": "Add struct tags", "answers": {"addTags": "json", "transform": "snakecase"} }
```

```
Applied "Add struct tags": <n> location(s) in 1 file(s): main.go
```

**Notes**

- Only actions of kind `refactor*` are listed; quick fixes are in `suggest_fixes`.
- Searchable fields (`lazyEnum`) return up to 50 options; narrow them with
  `option_queries`.
- `file` and `list` fields include their `type` (existence and extension
  constraints, element type).
- Interactive refactorings need a server that implements the protocol
  extension (golang/go#76331, microsoft/language-server-protocol#1164);
  currently gopls v0.24 or later. Other servers' refactorings run without
  questions.
- `execute_command` on a command that needs input returns an error pointing
  here, instead of running it without answers.
- With `dry_run: true` the result is the edit preview, nothing is written. To
  apply, re-run `refactor` with the same `action` and `answers` and without
  `dry_run`; do not pass the preview to `apply_edit`.
- The list, `needs_input`, and dry-run results are always JSON, whatever
  `AGENT_LSP_OUTPUT_FORMAT` says.

---

### `format_document`

Compute formatting edits for an entire file via `textDocument/formatting`.
Returns `TextEdit[]` describing what the formatter would change. Edits are
**not applied automatically**. Pass the result to `apply_edit` if you want to
write the formatted output to disk.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `tab_size` | number | no | Spaces per tab. Default `2`. |
| `insert_spaces` | boolean | no | Use spaces instead of tabs. Default `true`. |

**Example call**

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
  "language_id": "typescript",
  "tab_size": 2,
  "insert_spaces": true
}
```

**Expected output** (already-formatted file returns empty array)

```json
[]
```

**Expected output (file needing formatting)**

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
- Unlike the `rename_symbol` preview, this result follows the output format, so
  it is GCF by default. If you plan to pass the edits to `apply_edit`, run the
  server with `AGENT_LSP_OUTPUT_FORMAT=json` so the edits come back as JSON you
  can copy exactly. The same applies to `format_range`.

---

### `format_range`

Compute formatting edits for a selected range within a file via
`textDocument/rangeFormatting`. Otherwise identical to `format_document` but
scoped to specific lines.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `start_line` | number | yes | Start line (1-based) |
| `start_column` | number | yes | Start column (1-based) |
| `end_line` | number | yes | End line (1-based) |
| `end_column` | number | yes | End column (1-based) |
| `tab_size` | number | no | Default `2` |
| `insert_spaces` | boolean | no | Default `true` |

Range start must not be after range end (schema-validated).

**Example call** (format only the `Greeter` class)

```json
{
  "file_path": "/home/user/projects/agent-lsp/test/ts-project/src/example.ts",
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
Use it for edits returned by `format_document`, `format_range`, or a code
action from `suggest_fixes`. `rename_symbol` and `refactor` apply their own
edits, so do not pass their dry-run previews here.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `workspace_edit` | object | — | A `WorkspaceEdit` with either `changes` (Record&lt;uri, TextEdit[]&gt;) or `documentChanges` (TextDocumentEdit[]). Required when using positional mode. |
| `file_path` | string | — | **Text-match mode:** absolute path to the file to edit. Use with `old_text`+`new_text` instead of `workspace_edit`. |
| `old_text` | string | — | **Text-match mode:** exact text to find and replace. First tries exact byte match; falls back to whitespace-normalised line match (tolerates indentation differences). |
| `new_text` | string | — | **Text-match mode:** replacement text. |

Use either `workspace_edit` (positional mode, for edits returned by `format_document`/`format_range` or a code action) or the `file_path`+`old_text`+`new_text` triple (text-match mode, for AI-generated edits where exact line/column are unknown).

**Example call** (applying a single text edit)

```json
{
  "workspace_edit": {
    "changes": {
      "file:///home/user/projects/agent-lsp/test/ts-project/src/example.ts": [
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
Edit applied successfully
```

In text-match mode the message is the same, followed by a note when `old_text`
and `new_text` differ in whether they end with a line break. When `new_text` equals
the matched text, the tool writes nothing and returns
`No change applied: new_text is identical to the matched text, so the file was not modified.`

**Notes**

- Edits within each file are applied in reverse order (bottom-to-top) so that
  earlier offsets remain valid as later text is replaced.
- After writing files to disk, the tool sends `textDocument/didChange` for each
  modified file to keep the language server in sync.
- `documentChanges` (array of `TextDocumentEdit`) and `changes` (object) forms
  are both supported.
- This tool writes to disk immediately. Make sure the edit looks correct before
  calling it.

---

### `execute_command`

Execute a server-defined command via `workspace/executeCommand`. Commands are
returned in the `command` field of code actions (from `suggest_fixes`) and
may also be listed in the server's `executeCommandProvider.commands` capability.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `command` | string | yes | Command identifier (e.g., `_typescript.applyRefactoring`) |
| `arguments` | array of objects | no | Arguments to pass to the command. Each element must be a JSON object; the schema rejects strings, numbers, and other non-object values. |

**Example call** (running `go mod tidy` through gopls)

```json
{
  "command": "gopls.tidy",
  "arguments": [
    { "URIs": ["file:///home/user/projects/myproject/go.mod"] }
  ]
}
```

**Expected output (command with a result)**

The command's result, encoded in the output format. For example,
`gopls.list_known_packages` with `[{ "URI": "file:///path/to/main.go" }]`
returns (shown as JSON):

```json
{
  "Packages": ["bufio", "bytes", "context", "..."]
}
```

**Expected output (command with no result)**

When the server returns no result (as `gopls.tidy` does), the tool returns the
text `null`:

```
null
```

**Notes**

- The request is sent as is; if the server does not support the command (or
  `workspace/executeCommand` at all), the server's error is returned as a tool
  error.
- Because each argument must be an object, commands whose arguments are plain
  strings or numbers (for example `_typescript.applyRefactoring`) cannot be run
  through this tool.
- A command that needs interactive input (gopls v0.24+ interactive
  refactorings) is not run. The tool returns an error that names the input
  fields and points to [`refactor`](#refactor), which asks the questions and
  accepts answers.
- The available commands and their argument shapes are server-specific. Use
  `suggest_fixes` to discover commands rather than constructing them manually.
- Some commands apply changes server-side and push `workspace/applyEdit`
  requests; others return an edit that you must apply with `apply_edit`.

---

## Symbol editing tools

Four tools for editing, inserting, and deleting symbols by name without needing
exact line/column coordinates. These resolve symbols via `list_symbols`
internally.

### `replace_symbol_body`

Replace the body of a symbol (function, method, type) by name, keeping its
declaration line. Resolves the symbol within the file using document symbols,
then replaces everything from the line after the symbol's name through the end
of the symbol with the provided text.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file containing the symbol |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `symbol_path` | string | yes | Symbol name in dot notation (e.g. `"MyFunc"`, `"MyStruct.Method"`, `"Method[0]"` to pick one overload) |
| `new_body` | string | yes | Replacement body. Do not include the signature line; it is kept. |

**Example call**

```json
{
  "file_path": "/home/user/project/pkg/handler.go",
  "symbol_path": "Handler.ServeHTTP",
  "new_body": "\tw.WriteHeader(http.StatusOK)\n}"
}
```

**Notes**

- The declaration line (the line holding the symbol's name, for example
  `func (h *Handler) ServeHTTP(...) {`) is kept. `new_body` replaces the lines
  after it through the symbol's end, so for brace languages it must end with
  the closing brace, as in the example.
- If a signature spans several lines, only the line holding the name is kept;
  the continuation lines are part of the replaced range.
- Output: `Replaced body of "Handler.ServeHTTP" in /home/user/project/pkg/handler.go`,
  plus a hint with the post-edit error and warning counts.
- For methods, use `Type.Method` dot notation.
- Fails if the symbol cannot be found in the file's document symbol tree.
- After replacement, the language server is notified via `didChange`.

---

### `insert_after_symbol`

Insert code immediately after a named symbol. Useful for adding a new function
after an existing one, or appending related code near a symbol.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `symbol_path` | string | yes | Symbol name in dot notation |
| `code` | string | yes | Code to insert after the symbol |

**Example call**

```json
{
  "file_path": "/home/user/project/pkg/math.go",
  "symbol_path": "Add",
  "code": "\nfunc Subtract(a, b int) int {\n\treturn a - b\n}\n"
}
```

**Notes**

- Inserts immediately after the symbol's closing brace/end line.
- Include leading/trailing newlines in `code` for proper spacing.
- Fails if the symbol cannot be found.

---

### `insert_before_symbol`

Insert code immediately before a named symbol. Useful for adding comments,
decorators, or related code above an existing definition.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `symbol_path` | string | yes | Symbol name in dot notation |
| `code` | string | yes | Code to insert before the symbol |

**Example call**

```json
{
  "file_path": "/home/user/project/pkg/math.go",
  "symbol_path": "Add",
  "code": "// Deprecated: use AddV2 instead.\n"
}
```

**Notes**

- Inserts immediately before the symbol's start line.
- Include trailing newline in `code` so the symbol starts on its own line.
- Fails if the symbol cannot be found.

---

### `safe_delete_symbol`

Delete a symbol only if it has zero references across the workspace. Performs a
`find_references` check before deletion; refuses to delete if any callers exist.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file containing the symbol |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `symbol_path` | string | yes | Symbol name in dot notation |

**Example call**

```json
{
  "file_path": "/home/user/project/pkg/legacy.go",
  "symbol_path": "DeprecatedHelper"
}
```

**Expected output (success)**

```
Deleted symbol "DeprecatedHelper" from /home/user/project/pkg/legacy.go
```

**Expected output (refused, returned as a tool error)**

```
symbol has 3 references; cannot safely delete. Use find_references to see callers, update them first, then retry safe_delete_symbol.
```

**Notes**

- Uses `find_references` with `include_declaration: false` to check for callers.
- Refuses deletion if any reference is found, even in test files.
- After successful deletion, notifies the language server via `didChange`.
- Does not remove related imports or test code; use diagnostics to clean up.

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
      "uri": "file:///home/user/projects/agent-lsp/test/ts-project/src/newfile.ts",
      "type": 1
    },
    {
      "uri": "file:///home/user/projects/agent-lsp/test/ts-project/src/example.ts",
      "type": 2
    }
  ]
}
```

**Actual output**

```
File change notifications sent
```

**Notes**

- Sends a `workspace/didChangeWatchedFiles` notification (fire-and-forget); the
  server does not send a response.
- This is a notification, not a request, so there is no success/failure from
  the server side.
- Follow up with `get_diagnostics` after calling this if you want to verify the
  server picked up the changes.
- **Auto-watch:** The server automatically watches the workspace root for file
  changes using fsnotify and forwards them to the LSP server via
  `workspace/didChangeWatchedFiles` with a 150ms debounce. For normal editing
  workflows, calling this tool manually is not required. Use it only for
  explicit control, e.g. to notify the server of changes made by external
  processes before the auto-watcher's debounce window has closed, or for files
  outside the workspace root.

---

### `set_log_level`

Control the verbosity of logs written by the agent-lsp server process.

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
Log level set to "warning"
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
- Log messages are delivered as MCP `notifications/message` events to the connected
  client (not just stderr), so you will see LSP lifecycle events, tool dispatch errors,
  and indexing state directly in the session. Before session init, messages fall back
  to stderr.

---

## Code Intelligence tools

### `find_callers`

Resolve the call hierarchy for a symbol at a specific position. Returns the
symbol's `CallHierarchyItem` plus callers (incoming) and/or callees (outgoing),
via `textDocument/prepareCallHierarchy`, `callHierarchy/incomingCalls`, and
`callHierarchy/outgoingCalls`.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes | Line of the symbol (1-based) |
| `column` | number | yes | Column of the symbol (1-based) |
| `direction` | string | no | `"incoming"` (callers), `"outgoing"` (callees), or `"both"` (default) |
| `cross_concurrent` | boolean | no | If `true`, also flag callers whose call site crosses a concurrency boundary (goroutine, thread spawn, async task). Adds a `concurrent_callers` list; each entry has `caller`, `pattern` (the detected spawn pattern), `file`, and `line`. |

**Example call**

```json
{
  "file_path": "/path/to/project/pkg/handler.go",
  "language_id": "go",
  "line": 42,
  "column": 6,
  "direction": "incoming"
}
```

**Expected output**

```json
{
  "items": [
    {
      "name": "HandleRequest",
      "kind": 12,
      "uri": "file:///path/to/project/pkg/handler.go",
      "range": { "start": { "line": 41, "character": 0 }, "end": { "line": 55, "character": 1 } },
      "selectionRange": { "start": { "line": 41, "character": 5 }, "end": { "line": 41, "character": 20 } }
    }
  ],
  "incoming": [
    {
      "from": {
        "name": "ServeHTTP",
        "kind": 6,
        "uri": "file:///path/to/project/pkg/server.go",
        "range": { "...": "..." },
        "selectionRange": { "...": "..." }
      },
      "fromRanges": [
        { "start": { "line": 22, "character": 2 }, "end": { "line": 22, "character": 17 } }
      ]
    }
  ]
}
```

**Notes**

- Returns `"No call hierarchy item found at ..."` as a string when no symbol is
  found at the given position.
- Returns an error if the server does not declare `callHierarchyProvider`.
- `direction: "both"` fetches both incoming and outgoing in a single call.

---

### `type_hierarchy`

Resolve the type hierarchy for a symbol at a specific position. Returns the
symbol's `TypeHierarchyItem` plus supertypes (parent classes/interfaces) and/or
subtypes (implementations/subclasses), via `textDocument/prepareTypeHierarchy`,
`typeHierarchy/supertypes`, and `typeHierarchy/subtypes`. Requires LSP 3.17.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier. Auto-detected from the file extension when omitted. |
| `line` | number | yes | Line of the symbol (1-based) |
| `column` | number | yes | Column of the symbol (1-based) |
| `direction` | string | no | `"supertypes"` (parents), `"subtypes"` (implementations), or `"both"` (default) |

**Example call**

```json
{
  "file_path": "/path/to/project/pkg/animal.go",
  "language_id": "go",
  "line": 10,
  "column": 6,
  "direction": "both"
}
```

**Expected output**

```json
{
  "items": [
    {
      "name": "Animal",
      "kind": 11,
      "uri": "file:///path/to/project/pkg/animal.go",
      "range": { "start": { "line": 9, "character": 0 }, "end": { "line": 15, "character": 1 } },
      "selectionRange": { "start": { "line": 9, "character": 5 }, "end": { "line": 9, "character": 11 } }
    }
  ],
  "supertypes": [],
  "subtypes": [
    {
      "name": "Dog",
      "kind": 5,
      "uri": "file:///path/to/project/pkg/dog.go",
      "range": { "...": "..." },
      "selectionRange": { "...": "..." }
    }
  ]
}
```

**Notes**

- Returns `"No type hierarchy item found at ..."` as a string when no symbol is
  found at the given position.
- Requires the server to declare `typeHierarchyProvider` (LSP 3.17).
- Omitted fields (`supertypes`, `subtypes`) are absent rather than empty arrays
  when `direction` limits which are fetched.

---

### `get_inlay_hints`

Return inlay hints for a range within a document via `textDocument/inlayHint`.
Inlay hints show inferred type annotations and parameter name labels inline with
source code, the same annotations IDEs display in TypeScript, Rust, Go, and
other languages with type inference.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier |
| `start_line` | number | yes | Start line of the range (1-based) |
| `start_column` | number | yes | Start column (1-based) |
| `end_line` | number | yes | End line of the range (1-based) |
| `end_column` | number | yes | End column (1-based) |

**Example call**

```json
{
  "file_path": "/path/to/project/src/example.ts",
  "language_id": "typescript",
  "start_line": 1,
  "start_column": 1,
  "end_line": 20,
  "end_column": 1
}
```

**Expected output**

```json
[
  {
    "position": { "line": 3, "character": 7 },
    "label": ": number",
    "kind": 1,
    "paddingLeft": true
  },
  {
    "position": { "line": 7, "character": 12 },
    "label": "name: ",
    "kind": 2
  }
]
```

`kind` values: `1`=Type annotation, `2`=Parameter name. `label` may be either a
plain string or an array of `InlayHintLabelPart` objects with `value`, `tooltip`,
and optional `location`.

**Notes**

- Returns `[]` when the server does not declare `inlayHintProvider` or has no
  hints for the given range.
- Position coordinates in the output are 0-based (LSP native).

---

### `get_semantic_tokens`

Return semantic token classifications for a range within a document via
`textDocument/semanticTokens/range` (falls back to full-file if range is not
supported). Semantic tokens classify each token as a function, parameter,
variable, type, keyword, etc., using the type and modifier legend declared by
the server during initialization.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier |
| `start_line` | number | yes | Start line of the range (1-based) |
| `start_column` | number | yes | Start column (1-based) |
| `end_line` | number | yes | End line of the range (1-based) |
| `end_column` | number | yes | End column (1-based) |

**Example call**

```json
{
  "file_path": "/path/to/project/main.go",
  "language_id": "go",
  "start_line": 1,
  "start_column": 1,
  "end_line": 30,
  "end_column": 1
}
```

**Expected output**

```json
[
  {
    "line": 5,
    "character": 5,
    "length": 11,
    "token_type": "function",
    "token_modifiers": ["definition", "exported"]
  },
  {
    "line": 5,
    "character": 17,
    "length": 1,
    "token_type": "parameter",
    "token_modifiers": []
  }
]
```

Output positions are 1-based. The `token_type` and `token_modifiers` fields are
decoded from the server's legend into human-readable strings.

**Notes**

- Returns `[]` when the server does not declare `semanticTokensProvider`.
- The LSP wire format uses delta-encoded 5-integer tuples; this tool decodes
  them into absolute positions with named type/modifier strings from the
  server's legend captured during `initialize`.
- TypeScript, Go, Python, Rust, C, C++, C#, Kotlin, Ruby, and PHP all
  support semantic tokens.

---

### `get_document_highlights`

Return all occurrences of the symbol at a position within the same file via
`textDocument/documentHighlight`. Highlights are file-scoped and instant; they
do not trigger a workspace-wide reference search.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier |
| `line` | number | yes | Line of the symbol (1-based) |
| `column` | number | yes | Column of the symbol (1-based) |

**Example call**

```json
{
  "file_path": "/path/to/project/src/example.ts",
  "language_id": "typescript",
  "line": 4,
  "column": 17
}
```

**Expected output**

```json
[
  {
    "range": {
      "start": { "line": 3, "character": 16 },
      "end": { "line": 3, "character": 19 }
    },
    "kind": 1
  },
  {
    "range": {
      "start": { "line": 7, "character": 12 },
      "end": { "line": 7, "character": 15 }
    },
    "kind": 2
  }
]
```

`kind` values: `1`=Text, `2`=Read, `3`=Write.

**Notes**

- Returns `[]` if the server does not declare `documentHighlightProvider`.
- Use this instead of `find_references` when you only need occurrences within
  the current file. It is faster and requires no workspace indexing.
- Coordinates in the output are 0-based (LSP native).

---

## Server Introspection tools

### `get_server_capabilities`

Return the language server's capability map and classify every agent-lsp tool
as supported or unsupported based on what the server advertised during
initialization. Useful for discovering which tools will return meaningful results
before making analysis calls.

**Parameters**

None. The tool takes no arguments.

**Example call**

```json
{}
```

**Expected output**

```json
{
  "server_name": "gopls",
  "server_version": "v0.16.2",
  "supported_tools": [
    "apply_edit",
    "find_callers",
    "did_change_watched_files",
    "format_document",
    "suggest_fixes",
    "get_completions",
    "get_diagnostics",
    "list_symbols",
    "inspect_symbol",
    "find_references",
    "get_semantic_tokens",
    "get_signature_help",
    "find_symbol",
    "go_to_definition",
    "go_to_implementation",
    "go_to_type_definition",
    "rename_symbol",
    "prepare_rename",
    "start_lsp",
    "..."
  ],
  "unsupported_tools": [
    "get_inlay_hints",
    "type_hierarchy"
  ],
  "capabilities": {
    "hoverProvider": true,
    "completionProvider": { "triggerCharacters": [".", ":"], "resolveProvider": true },
    "..."
  }
}
```

**Notes**

- Requires `start_lsp` to have been called first.
- Always-available tools (e.g., `start_lsp`, `open_document`, `set_log_level`,
  `detect_lsp_servers`) appear in `supported_tools` regardless of server
  capabilities.
- `server_name` and `server_version` are omitted if the server did not provide
  them in its `initialize` response.

---

### `detect_lsp_servers`

Scan a workspace directory for source languages and check PATH for the
corresponding LSP server binaries. Returns the detected languages, installed
servers with their executable paths, and a `suggested_config` array ready to
paste into the agent-lsp MCP server args.

Does not require `start_lsp` to have been called; it works standalone.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `workspace_dir` | string | yes | Absolute path to the workspace root to scan |

**Example call**

```json
{
  "workspace_dir": "/home/user/projects/myproject"
}
```

**Expected output**

```json
{
  "workspace_dir": "/home/user/projects/myproject",
  "workspace_languages": ["go", "typescript"],
  "installed_servers": [
    {
      "language": "go",
      "server": "gopls",
      "path": "/usr/local/bin/gopls",
      "config_entry": "go:gopls"
    },
    {
      "language": "typescript",
      "server": "typescript-language-server",
      "path": "/usr/local/bin/typescript-language-server",
      "config_entry": "typescript:typescript-language-server,--stdio"
    }
  ],
  "suggested_config": [
    "go:gopls",
    "typescript:typescript-language-server,--stdio"
  ],
  "not_installed": []
}
```

**Notes**

- Language detection is score-based: project root markers (e.g., `go.mod`,
  `package.json`, `Cargo.toml`) score +50; individual source files score +1
  per file. Languages are returned ranked by score.
- Skips `node_modules`, `vendor`, `target`, `build`, `dist`, `.git`, and other
  standard build/cache directories.
- `not_installed` lists languages detected in the workspace whose server binary
  was not found on PATH.
- `suggested_config` entries use the format `language:binary` or
  `language:binary,arg1,arg2` and can be passed directly as agent-lsp MCP
  server args.

---

## Build & Test tools

### `run_build`

Compile the project using the detected workspace language. Language-specific
dispatch: `go build ./...` (Go), `cargo build` (Rust), `tsc --noEmit`
(TypeScript), `mypy .` (Python typecheck proxy), `npm run build` (JavaScript),
`dotnet build` (C#), `swift build` (Swift), `zig build` (Zig),
`gradle build --quiet` (Kotlin). Does not require `start_lsp`.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `workspace_dir` | string | yes | Absolute path to the workspace root |
| `path` | string | no | Narrows the build scope (e.g., a subdirectory or file) |
| `language` | string | no | Override auto-detected language |

**Returns**

```json
{
  "success": true,
  "errors": [
    { "file": "/path/to/file.go", "line": 12, "column": 5, "message": "undefined: Foo" }
  ],
  "raw": "go build ./...\n..."
}
```

**Notes**

- Language is auto-detected from workspace root markers (`go.mod`, `Cargo.toml`,
  `package.json`, `pyproject.toml`, etc.) when `language` is omitted.
- `errors` is always an array; it is empty on a clean build.
- Does not start or require an LSP session.

---

### `run_tests`

Run the test suite for the detected workspace language. Language-specific
dispatch: `go test -json ./...` (Go), `cargo test --message-format=json`
(Rust), `pytest --tb=json` (Python), `npm test` (JavaScript/TypeScript),
`dotnet test` (C#), `swift test` (Swift), `zig build test` (Zig),
`gradle test --quiet` (Kotlin). Test failure `location` fields are
LSP-normalized (file URI + zero-based range). Paste directly into
`go_to_definition`. Does not require `start_lsp`.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `workspace_dir` | string | yes | Absolute path to the workspace root |
| `path` | string | no | Narrows the test scope (e.g., a package path or file) |
| `language` | string | no | Override auto-detected language |

**Returns**

```json
{
  "passed": false,
  "failures": [
    {
      "file": "/path/to/file_test.go",
      "line": 42,
      "test_name": "TestFoo",
      "message": "assertion failed: expected 1, got 2",
      "location": {
        "uri": "file:///path/to/file_test.go",
        "range": { "start": { "line": 41, "character": 0 }, "end": { "line": 41, "character": 0 } }
      }
    }
  ],
  "raw": "..."
}
```

**Notes**

- `passed` is `true` only when all tests pass and the test runner exits 0.
- `failures` is empty when `passed` is `true`.
- `location` is LSP-normalized for direct use with `go_to_definition`.
- Does not start or require an LSP session.

---

### `get_tests_for_file`

Return test files that exercise a given source file. Static lookup, no test
execution. Go: `*_test.go` in the same directory. Python: `test_*.py` /
`*_test.py` in the same directory and a `tests/` sibling. TypeScript/JS:
`*.test.ts`, `*.spec.ts`, `*.test.js`, `*.spec.js`, etc. Rust: returns the
source file itself (tests are inline). Swift: returns the source file itself
(tests are inline via XCTest). Zig: returns the source file itself (inline
`test` blocks). C#: globs `*Test*.cs` / `*Tests.cs` in the project tree.
Kotlin: globs `*Test.kt` / `*Tests.kt` in the project tree.
Does not require `start_lsp`.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the source file |

**Returns**

```json
{
  "source_file": "/path/to/pkg/handler.go",
  "test_files": [
    "/path/to/pkg/handler_test.go"
  ]
}
```

**Notes**

- `test_files` is an empty array when no test files are found.
- For Rust, Swift, and Zig, the source file itself is returned in `test_files`
  because tests live in the same file as the source code (Rust `#[cfg(test)]`
  modules, Swift XCTest methods, Zig inline `test` blocks).
- For C#, test files are discovered by globbing `*Test*.cs` / `*Tests.cs`.
- For Kotlin, test files are discovered by globbing `*Test.kt` / `*Tests.kt`.
- Does not start or require an LSP session.

---

## Simulation tools

Speculative code sessions let you apply hypothetical edits, evaluate their diagnostic impact, then commit or discard, all without touching files on disk. See `docs/speculative-execution.md` for the full design.

**Call `start_lsp` before using any simulation tool.** All simulation tools operate against the currently-running language server.

**Position convention:** all `start_line`, `start_column`, `end_line`, `end_column` parameters are **1-indexed** (same as editor line numbers). The server converts to 0-indexed internally.

---

### `create_simulation_session`

Create an isolated speculative session rooted at the current workspace state. The session accumulates in-memory edits without modifying files on disk.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `workspace_root` | string | yes | Absolute path to the workspace root |
| `language` | string | yes | Language identifier (`go`, `typescript`, etc.) |

**Example call**

```json
{
  "workspace_root": "/home/user/projects/myproject",
  "language": "go"
}
```

**Actual output**

```json
{
  "session_id": "a3f2-4b91-...",
  "status": "created"
}
```

**Notes**

- Call `start_lsp` first; the session is attached to the running language server.
- The session must be destroyed when done. Call `destroy_session` to release resources.
- Multiple sessions may exist simultaneously with independent state.

---

### `simulate_edit`

Apply an in-memory edit to an existing session. Does not modify files on disk. Multiple edits accumulate within the session.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | yes | Session ID from `create_simulation_session` |
| `file_path` | string | yes | Absolute path to the file to edit |
| `start_line` | int | yes | Start line of the edit range (1-indexed) |
| `start_column` | int | yes | Start column (1-indexed) |
| `end_line` | int | yes | End line of the edit range (1-indexed) |
| `end_column` | int | yes | End column (1-indexed) |
| `new_text` | string | yes | Replacement text for the specified range |

**Example call**

```json
{
  "session_id": "a3f2-4b91-...",
  "file_path": "/home/user/projects/myproject/pkg/handler.go",
  "start_line": 42,
  "start_column": 1,
  "end_line": 42,
  "end_column": 20,
  "new_text": "replacement text"
}
```

**Actual output**

```json
{
  "session_id": "a3f2-4b91-...",
  "edit_applied": true,
  "version_after": 1
}
```

**Notes**

- Multiple `simulate_edit` calls may be made before calling `evaluate_session`; evaluation reflects the cumulative state.
- Does not evaluate diagnostics. Call `evaluate_session` separately to observe impact.

---

### `evaluate_session`

Compare the session's current diagnostic state against the baseline. Returns which errors were introduced and which were resolved by the accumulated edits.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | yes | Session ID |
| `scope` | string | no | `"file"` (default) or `"workspace"`. File scope is faster and returns `confidence: "high"`. |
| `timeout_ms` | int | no | Milliseconds to wait for diagnostics to settle. Default: 3000ms (file), 8000ms (workspace). |

**Example call**

```json
{
  "session_id": "a3f2-4b91-...",
  "scope": "file",
  "timeout_ms": 5000
}
```

**Actual output**

```json
{
  "session_id": "a3f2-4b91-...",
  "errors_introduced": null,
  "errors_resolved": null,
  "net_delta": 0,
  "scope": "file",
  "confidence": "high",
  "timeout": false,
  "duration_ms": 412
}
```

**Notes**

- `net_delta: 0` means no new errors were introduced, safe to apply.
- `confidence` values: `"high"` (file scope, settled within timeout), `"partial"` (timed out or snapshot incomplete), `"eventual"` (workspace scope, cross-file propagation may be incomplete).
- Does not mutate session state.

---

### `simulate_chain`

Apply a sequence of edits within a session and evaluate diagnostics after each step. Returns per-step results and identifies the last step that is safe to apply.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | yes | Session ID |
| `edits` | array | yes | Ordered list of edit objects, each with `file_path`, `start_line`, `start_column`, `end_line`, `end_column`, `new_text` |
| `timeout_ms` | int | no | Timeout per evaluation step |

**Example call**

```json
{
  "session_id": "a3f2-4b91-...",
  "edits": [
    {
      "file_path": "/home/user/projects/myproject/pkg/handler.go",
      "start_line": 10, "start_column": 1,
      "end_line": 10,   "end_column": 30,
      "new_text": "first change"
    },
    {
      "file_path": "/home/user/projects/myproject/pkg/handler.go",
      "start_line": 20, "start_column": 1,
      "end_line": 20,   "end_column": 30,
      "new_text": "second change"
    }
  ]
}
```

**Actual output**

```json
{
  "steps": [
    { "step": 1, "net_delta": 0, "errors_introduced": [] },
    { "step": 2, "net_delta": 3, "errors_introduced": ["..."] }
  ],
  "safe_to_apply_through_step": 1,
  "cumulative_delta": 3
}
```

**Notes**

- Each step builds on the previous in-memory state within the session.
- `safe_to_apply_through_step` is the last step index with `net_delta: 0`.

---

### `commit_session`

Materialize the session's accumulated speculative state. By default returns a patch without writing to disk (functional mode). Pass `apply: true` to write files to disk.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | yes | Session ID |
| `target` | string | no | Currently unused: the value is accepted but has no effect. Use `apply` to choose between returning the patch and writing to disk. |
| `apply` | boolean | no | If `true`, write changes to disk. Default `false`. |

**Example call (patch only, default)**

```json
{
  "session_id": "a3f2-4b91-..."
}
```

**Example call (write to disk)**

```json
{
  "session_id": "a3f2-4b91-...",
  "apply": true
}
```

**Output** (with `apply: true`)

```json
{
  "session_id": "a3f2-4b91-...",
  "files_written": 1,
  "patch": {
    "file:///path/to/file.go": "package main\n\n// full file content after the session's edits\n..."
  }
}
```

**Notes**

- Default is functional (patch only, no disk writes). Callers opt into disk writes explicitly with `apply: true`.
- `patch` maps each file URI in the session to that file's **full content**
  after the session's edits. It is not a `WorkspaceEdit` or a diff.
- `files_written` is the number of files written to disk. It is omitted when
  zero (the default, patch-only call).
- Commit is only allowed from a session in `evaluated` or `mutated` state; prohibited on `dirty` or `created` sessions.
- Call `destroy_session` after committing to release resources.

---

### `discard_session`

Revert all in-memory edits accumulated in the session. The session is reset to baseline state. Nothing is written to disk.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | yes | Session ID |

**Example call**

```json
{
  "session_id": "a3f2-4b91-..."
}
```

**Actual output**

```json
{
  "session_id": "a3f2-4b91-...",
  "status": "discarded"
}
```

**Notes**

- Equivalent to rolling back a transaction. No side effects.
- Call `destroy_session` after discarding to release resources.

---

### `destroy_session`

Clean up all resources associated with a session. Must be called after committing or discarding to prevent resource leaks.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | yes | Session ID |

**Example call**

```json
{
  "session_id": "a3f2-4b91-..."
}
```

**Actual output**

```json
{
  "session_id": "a3f2-4b91-...",
  "status": "destroyed"
}
```

**Notes**

- Must be called even on dirty sessions. This is the only valid cleanup path for a session in `dirty` state.
- Sessions are in-memory only; session IDs become invalid on MCP server restart.

---

### `preview_edit`

One-shot convenience wrapper: applies a single edit, evaluates diagnostics, and discards in one call. The file on disk is never modified.

Two modes:
- **Standalone** (`session_id` omitted): creates a temporary session, applies the edit, evaluates, then destroys the session automatically. Requires `workspace_root` and `language`.
- **Existing session** (`session_id` provided): applies the edit into an existing session and evaluates without destroying it. `workspace_root` and `language` are ignored.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | no | Existing session ID. If provided, uses that session instead of creating a temporary one |
| `workspace_root` | string | no* | Absolute path to the workspace root (*required when `session_id` is omitted) |
| `language` | string | no* | Language identifier (*required when `session_id` is omitted) |
| `file_path` | string | yes | Absolute path to the file to edit |
| `start_line` | int | yes | Start line (1-indexed) |
| `start_column` | int | yes | Start column (1-indexed) |
| `end_line` | int | yes | End line (1-indexed) |
| `end_column` | int | yes | End column (1-indexed) |
| `new_text` | string | yes | Replacement text |
| `scope` | string | no | `"file"` (default) or `"workspace"` |
| `timeout_ms` | int | no | Timeout for diagnostic evaluation |

**Example call**

```json
{
  "workspace_root": "/home/user/projects/myproject",
  "language": "go",
  "file_path": "/home/user/projects/myproject/pkg/handler.go",
  "start_line": 42,
  "start_column": 1,
  "end_line": 42,
  "end_column": 20,
  "new_text": "replacement text",
  "scope": "file",
  "timeout_ms": 5000
}
```

**Actual output**

```json
{
  "errors_introduced": null,
  "errors_resolved": null,
  "net_delta": 0,
  "confidence": "high"
}
```

**Notes**

- Easiest way to test a single edit. No session lifecycle management required.
- Call `start_lsp` first.
- `net_delta: 0` means the edit is safe to apply (no new errors introduced).
- Automatically discards and destroys the session; the file on disk is never modified.
- Backed by the same session infrastructure as the full lifecycle tools, not a separate code path.

---

## Startup and warm-up notes

The tsserver (and some other language servers) perform asynchronous workspace
indexing after `initialize`. During this period:

- `inspect_symbol` may return empty string.
- `find_references` may return `[]`.
- `get_diagnostics` may return empty diagnostic arrays.

The server handles three server-initiated requests that must be responded to
before workspace loading completes:

1. `window/workDoneProgress/create`: pre-registers a progress token.
2. `workspace/configuration`: the server returns `null` for each requested
   config item.
3. `client/registerCapability`: acknowledged with `null`.

agent-lsp handles all three automatically. For `find_references`, the client
additionally waits for all `$/progress` end events before returning. tsserver
does not emit `$/progress`, so references may require a brief wait and retry
on first use. Set `set_log_level` to `debug` and look for `Progress end:` log
lines to confirm when the server is ready.

---

## Symbol lookup tools

### `go_to_symbol`

Navigate to a symbol's definition by dot-notation path, with no file path or
line/column required. Useful when you know a symbol name but not its location.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbol_path` | string | yes | Dot-notation symbol path, e.g. `"MyClass.method"` or `"pkg.Function"` |
| `workspace_root` | string | no | Restrict search to a specific workspace root |
| `language` | string | no | Filter candidates by language ID |

**Algorithm**

1. Splits `symbol_path` on `.` to extract the leaf name (last component).
2. Calls `GetWorkspaceSymbols` with the leaf name to get candidates.
3. If the path is dotted, prefers candidates where `ContainerName` matches the parent component; otherwise uses the first result.
4. Opens the candidate file via `WithDocument` and calls `GetDefinition` at the candidate position; if empty, uses the candidate location directly.
5. Returns a 1-indexed `FormattedLocation`.

**Example call**

```json
{
  "symbol_path": "LSPClient.GetDefinition",
  "workspace_root": "/Users/you/code/agent-lsp"
}
```

**Actual output**

```json
[
  {
    "file": "/Users/you/code/agent-lsp/internal/lsp/client.go",
    "line": 142,
    "column": 1,
    "end_line": 142,
    "end_column": 13
  }
]
```

---

### `get_symbol_source`

Return the source code of the innermost symbol (function, method, struct, class,
etc.) whose range contains a given cursor position. Composes
`textDocument/documentSymbol` + file read. No new LSP methods required.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file_path` | string | yes | Absolute path to the file |
| `language_id` | string | no | Language identifier (used for document open) |
| `line` | number | no | 1-based line number of the cursor position |
| `column` | number | no | 1-based column of the cursor. Defaults to `1`. |
| `position_pattern` | string | no | `@@`-syntax pattern for cursor placement (see Position-pattern section) |

**Example call**

```json
{
  "file_path": "/home/user/projects/myproject/main.go",
  "language_id": "go",
  "line": 12,
  "column": 5
}
```

**Actual output**

```json
{
  "symbol_name": "handleRequest",
  "symbol_kind": "Function",
  "start_line": 10,
  "end_line": 18,
  "source": "func handleRequest(w http.ResponseWriter, r *http.Request) {\n\t// ...\n}"
}
```

**Notes**

- `findInnermostSymbol` walks the DocumentSymbol tree recursively and returns the deepest symbol whose range contains the cursor, so clicking inside a method body returns the method, not the enclosing class.
- `start_line` and `end_line` are 1-based.
- Provide `line` (and optionally `column`), or `position_pattern` with `@@`. At least one position input is required. The older name `character` is still accepted as an alias for `column`.
- CI-tested across the language matrix via `testGetSymbolSource`.

---

### `get_symbol_documentation`

Fetch authoritative documentation for a named symbol from local toolchain
sources (go doc, pydoc, cargo doc) without requiring an LSP hover response.
Works on transitive dependencies not indexed by the language server.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbol` | string | yes | Fully-qualified symbol name (e.g. `fmt.Println`, `std::vec::Vec::new`) |
| `language_id` | string | yes | Language identifier: `go`, `rust`, `python` |
| `file_path` | string | no | Absolute path to any file in the module/project, used to infer package context for Go |
| `format` | string | no | `"text"` (default) or `"markdown"`. Wraps signature in a fenced code block |

**Algorithm**

Dispatches to a per-language toolchain command based on `language_id`:

| Language | Command |
|----------|---------|
| `go` | `go doc <symbol>` (run from `file_path` directory if provided) |
| `python` | `pydoc <symbol>` |
| `rust` | `cargo doc --open` (offline cache lookup) |

**Example call**

```json
{
  "symbol": "fmt.Println",
  "language_id": "go"
}
```

**Actual output**

```json
{
  "symbol": "fmt.Println",
  "language": "go",
  "source": "toolchain",
  "doc": "func Println(a ...any) (n int, err error)\n\nPrintln formats using the default formats...",
  "signature": "func Println(a ...any) (n int, err error)",
  "error": ""
}
```

**Notes**

- All dispatchers use a 10-second timeout. Cold module cache (first `go doc` call)
  may approach this limit; subsequent calls are fast.
- Output is ANSI-stripped, safe for MCP transport.
- When the toolchain command fails, `source` is `"error"` and `error` contains the
  stderr. This is a structured result, not an MCP error. Callers can detect it and
  fall back to `inspect_symbol`.
- TypeScript and JavaScript are not supported; returns `source: "error"` with an
  appropriate message.
- `get_symbol_documentation` is used as Tier 2 in the `lsp-docs` skill. Call it
  after hover returns empty, before falling back to source navigation.

---

### Position-pattern parameter (`position_pattern`)

Seven tools accept an optional `position_pattern` field: `inspect_symbol`,
`find_references`, `rename_symbol`, `get_symbol_source`, `explore_symbol`,
`explore`, and `refactor`. When provided, it replaces the `line`/`column` pair.

`go_to_definition` also lists `position_pattern` in its schema, but its handler
currently ignores the field and requires `line` and `column`.

**How it works**

The value is a text snippet that appears in the file, with `@@` marking the
cursor position. `ResolvePositionPattern` searches the file for the text
surrounding `@@`, strips the marker, and returns the 1-indexed line and column
of the character immediately after `@@`.

**Example**

```json
{
  "file_path": "/path/to/file.go",
  "position_pattern": "func (c *LSPClient) Get@@Definition"
}
```

This positions the cursor at the `D` in `GetDefinition`, equivalent to
passing the line/column manually, but resistant to line number drift.

**Error cases**

- Pattern missing `@@`: returns an error.
- Search text not found in file: returns an error.

---

### Dry-run preview for `rename_symbol`

`rename_symbol` applies its edit to disk unless `dry_run` is `true`.

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `dry_run` | boolean | no | If `true`, return the edit preview without writing to disk. Default `false`. |

**Dry-run output** (always JSON, whatever `AGENT_LSP_OUTPUT_FORMAT` says)

```json
{
  "workspace_edit": { "changes": { "file:///project/pkg/types.go": [ { "...": "..." } ] } },
  "preview": {
    "files": 2,
    "locations": 5,
    "file_names": ["types.go", "session.go"],
    "note": "dry_run=true: preview only, nothing written. Re-run rename_symbol without dry_run to apply."
  }
}
```

`workspace_edit` is the raw LSP edit (0-based coordinates), included so you can
inspect every site. `preview.files` and `preview.locations` count the edited
files and edit sites; `preview.file_names` lists the edited files' base names.

To apply, call `rename_symbol` again with the same arguments and without
`dry_run`. Do not reconstruct the preview into an `apply_edit` call.

**Normal mode** (`dry_run` omitted or `false`): the edit is written to disk and
the tool returns a text summary, such as
`Renamed to "SessionToken" across 5 location(s) in 2 file(s): types.go, session.go`.

### Glob exclusions for `rename_symbol`

`rename_symbol` accepts an optional `exclude_globs` array. Files matching
any pattern are left out of the edit (and out of the dry-run preview). The
rename still executes on all other files.

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `exclude_globs` | array | no | Array of glob patterns (filepath.Match syntax). Matched against full path and basename. |

**Example** (exclude generated files and vendor tree):

```json
{
  "file_path": "/project/pkg/types.go",
  "line": 12,
  "column": 6,
  "new_name": "SessionToken",
  "exclude_globs": ["vendor/**", "**/*_gen.go", "**/*_generated.go"]
}
```

---

## Phase enforcement tools

Runtime enforcement of skill phase ordering. When an agent activates a skill,
tool calls are checked against the skill's declared phase permissions. Phases
advance automatically as the agent calls tools from later phases. See
[Phase enforcement](../guide/phase-enforcement.md) for the full design.

### `activate_skill`

Start phase enforcement for a skill workflow. Once active, every tool call is
checked against the current phase's allowed/forbidden lists before executing.

**Parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `skill_name` | string | yes | Skill to activate (`lsp-rename`, `lsp-refactor`, `lsp-safe-edit`, `lsp-verify`) |
| `mode` | string | no | `warn` (log violation, allow call) or `block` (return error with recovery guidance). Default: `warn` |

**Example call**

```json
{ "skill_name": "lsp-refactor", "mode": "block" }
```

**Example output**

```json
{
  "status": "activated",
  "skill": "lsp-refactor",
  "mode": "block",
  "current_phase": "blast_radius",
  "total_phases": 5,
  "allowed_tools": ["blast_radius", "go_to_symbol", "find_references"],
  "forbidden_tools": ["apply_edit", "simulate_*", "Edit", "Write", "rename_symbol"]
}
```

**Notes**

- Only one skill can be active at a time. Call `deactivate_skill` before activating a different skill.
- Phase enforcement applies to all agent-lsp tools. External tools (Edit, Write, Bash) appear in forbidden lists for informational purposes but cannot be enforced at runtime.

---

### `deactivate_skill`

Stop phase enforcement for the currently active skill. Idempotent: safe to call
when no skill is active.

**Parameters:** none

**Example output**

```json
{ "status": "deactivated" }
```

---

### `get_skill_phase`

Query the current state of phase enforcement: active skill, current phase,
allowed and forbidden tools, and the full tool call history since activation.

**Parameters:** none

**Example output (skill active)**

```json
{
  "active": true,
  "skill_name": "lsp-rename",
  "current_phase": "preview",
  "phase_index": 1,
  "total_phases": 3,
  "mode": "warn",
  "allowed_tools": ["go_to_symbol", "prepare_rename", "find_references", "rename_symbol"],
  "forbidden_tools": ["apply_edit", "Edit", "Write", "format_document", "run_tests"],
  "tool_history": ["start_lsp", "go_to_symbol", "prepare_rename"]
}
```

**Example output (no skill active)**

```json
{
  "active": false,
  "available_skills": ["lsp-rename", "lsp-refactor", "lsp-safe-edit", "lsp-verify"]
}
```

---

## Skills

Twenty-four agent-native skills compose agent-lsp tools into single-command
workflows. Install with `cd skills && ./install.sh`.

| Skill | Tools used | Purpose |
|-------|-----------|---------|
| `/lsp-safe-edit` | `preview_edit`, `get_diagnostics`, `suggest_fixes`, `apply_edit` | Speculative preview before disk write (`preview_edit`); before/after diagnostic diff; surfaces code actions on introduced errors; handles multi-file edits |
| `/lsp-edit-export` | `find_references`, `inspect_symbol`, `preview_edit` | Safe editing of exported symbols. Finds all callers first, then validates the edit |
| `/lsp-edit-symbol` | `find_symbol`, `list_symbols`, `apply_edit` | Edit a named symbol without knowing its file or position. Resolves name to definition, retrieves full range, applies edit |
| `/lsp-rename` | `prepare_rename`, `rename_symbol` (`dry_run` preview, then apply), `get_diagnostics` | Two-phase rename: `prepare_rename` validates position first, then preview all sites with `dry_run`, confirm, and re-run `rename_symbol` to apply |
| `/lsp-verify` | `get_diagnostics`, `run_build`, `run_tests` | Full three-layer check: LSP diagnostics + build + tests, summarizes pass/fail |
| `/lsp-simulate` | `create_simulation_session`, `preview_edit`, `simulate_chain`, `evaluate_session` | Speculative editing: test changes without touching the file; supports single edits, sessions, and chained multi-edit sequences |
| `/lsp-impact` | `find_references`, `find_callers`, `type_hierarchy` | Blast-radius analysis before renaming or deleting. Maps all callers, implementors, and subtypes |
| `/lsp-dead-code` | `list_symbols`, `find_references` | Detect zero-reference exports and unreachable symbols across a file or workspace |
| `/lsp-implement` | `go_to_implementation`, `type_hierarchy` | Find all concrete implementations of an interface or abstract type, with capability pre-check and risk assessment (0 = likely unused, >10 = breaking API change) |
| `/lsp-docs` | `inspect_symbol`, `get_symbol_documentation`, `go_to_definition`, `get_symbol_source` | Three-tier documentation lookup: hover → offline toolchain doc → source definition |
| `/lsp-cross-repo` | `add_workspace_folder`, `list_workspace_folders`, `find_references`, `go_to_implementation`, `find_callers` | Multi-root cross-repo analysis. Add a consumer repo and find all callers, references, and implementations of a library symbol across both repos |
| `/lsp-local-symbols` | `list_symbols`, `get_document_highlights`, `inspect_symbol` | File-scoped analysis: list all symbols in a file, find all usages of a symbol within the file (faster than workspace search), get type info |
| `/lsp-test-correlation` | `get_tests_for_file`, `find_symbol`, `run_tests` | Find and run only the tests covering an edited file; multi-file deduplication; fallback to workspace symbol search when mapping is absent |
| `/lsp-format-code` | `format_document`, `format_range`, `apply_edit`, `get_diagnostics` | Format a file or selection via the language server formatter; full-file or range; verifies no diagnostics introduced after applying |
| `/lsp-explore` | `go_to_symbol`, `inspect_symbol`, `go_to_implementation`, `find_callers`, `find_references` | Symbol exploration: hover + implementations + call hierarchy + references in one pass, for navigating unfamiliar code |
| `/lsp-understand` | `inspect_symbol`, `go_to_implementation`, `find_callers`, `find_references`, `get_symbol_source`, `list_symbols`, `go_to_symbol` | Deep-dive exploration. Builds a Code Map showing type info, implementations, call hierarchy, references, and source |
| `/lsp-refactor` | `blast_radius`, `preview_edit`, `simulate_chain`, `get_diagnostics`, `run_build`, `run_tests`, `get_tests_for_file`, `apply_edit`, `format_document` | End-to-end safe refactor: blast-radius analysis, speculative preview, apply, verify build, run affected tests |
| `/lsp-extract-function` | `list_symbols`, `suggest_fixes`, `execute_command`, `apply_edit`, `get_diagnostics`, `format_document` | Extract a code block into a named function; primary path uses LSP code action, falls back to manual extraction |
| `/lsp-fix-all` | `get_diagnostics`, `suggest_fixes`, `apply_edit`, `format_document` | Bulk-apply quick-fix code actions for all diagnostics in a file |
| `/lsp-generate` | `suggest_fixes`, `execute_command`, `apply_edit`, `format_document`, `get_diagnostics` | Trigger LSP code generation: implement interface stubs, generate test skeletons, add missing methods |
| `/lsp-architecture` | `start_lsp`, `detect_lsp_servers`, `list_symbols`, `find_symbol`, `blast_radius` | Structural architecture overview of a codebase: languages, package map, entry points, dependency graph, and hotspots |
| `/lsp-concurrency-audit` | `list_symbols`, `find_references`, `find_callers`, `blast_radius`, `get_symbol_source` | Concurrency safety audit for a type or file: maps fields, traces which are accessed from concurrent contexts (goroutines, threads, async tasks), and flags fields that lack synchronization |
| `/lsp-inspect` | `blast_radius`, `find_references`, `list_symbols`, `inspect_symbol`, `get_diagnostics`, `find_callers`, `go_to_definition`, `get_cross_repo_references` | Full code quality audit for a file, package, or directory (dead symbols, silent failures, error wrapping, coverage gaps, doc drift, concurrency safety), with batch and diff modes and a severity-tiered report |
| `/lsp-onboard` | `start_lsp`, `detect_lsp_servers`, `list_symbols`, `find_symbol`, `blast_radius`, `run_build`, `run_tests`, `get_diagnostics` | First-session project onboarding: detects build system, test runner, entry points, and key architecture patterns, and produces a project profile for the session |

Skills work with any MCP client that supports tool use, not just Claude Code.

---

## See also

- [Skills reference](../guide/skills.md): skill reference with workflows, use cases, and composition patterns
- [Language support](language-support.md): language coverage matrix and per-language tool support
