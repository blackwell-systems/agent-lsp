---
name: lsp-cross-repo
description: Cross-repository analysis — add a second repo to the workspace and run references, implementations, and call hierarchy across both. Use when refactoring a shared library and need to understand how consumers use it.
argument-hint: "[symbol-name] in [library-root] used by [consumer-root]"
allowed-tools: mcp__lsp__start_lsp mcp__lsp__add_workspace_folder mcp__lsp__list_workspace_folders mcp__lsp__get_workspace_symbols mcp__lsp__get_references mcp__lsp__go_to_implementation mcp__lsp__call_hierarchy mcp__lsp__get_info_on_location
---

> Requires the agent-lsp MCP server.

# lsp-cross-repo

Multi-root workspace analysis for library + consumer workflows. Sets up both
repositories in a single LSP session, verifies cross-repo indexing, then finds
all usages of a library symbol in the consumer codebase.

Read-only — does not modify any files.

## When to use

- Before changing a library API: find all callers in the consumer
- Before deleting a symbol: verify it has no cross-repo dependents
- When a change in repo A might break repo B
- Auditing how internal packages are used across services

Use `/lsp-impact` instead for single-repo blast-radius analysis.

## Workflow

### Step 1 — Initialize the primary workspace

Start the language server on the library root if not already running:

```
mcp__lsp__start_lsp({ "root_dir": "/path/to/library" })
```

### Step 2 — Add the consumer repo

```
mcp__lsp__add_workspace_folder({ "path": "/path/to/consumer" })
```

This registers the consumer directory as a second workspace root. The language
server indexes it alongside the primary root, enabling cross-repo symbol resolution.

### Step 3 — Verify both roots are indexed

```
mcp__lsp__list_workspace_folders()
```

Both paths should appear. If the consumer is missing, the `add_workspace_folder`
call did not succeed — retry before continuing.

**Indexing warm-up:** After adding a new folder, the language server may take a
few seconds to index it. Test with a known symbol from the consumer:

```
mcp__lsp__get_workspace_symbols({ "query": "<known-consumer-symbol>" })
```

If it returns results, indexing is complete. If empty, wait 5 seconds and retry.

### Step 4 — Locate the library symbol

Find the symbol's definition position:

```
mcp__lsp__get_workspace_symbols({ "query": "<symbol-name>" })
```

Pick the result in the library repo (not a test file). Note the `file_path`,
`line`, and `column`.

Open the file to ensure it's tracked:

```
mcp__lsp__open_document({ "file_path": "<library-file>", "language_id": "<lang>" })
```

### Step 5 — Find all cross-repo references

```
mcp__lsp__get_references({
  "file_path": "<library-file>",
  "line": <line>,
  "column": <column>,
  "include_declaration": false
})
```

Results from both repos appear in the same response. Filter by path prefix to
separate library-internal references from consumer references.

### Step 6 — Callers and implementations (optional)

For functions — callers in both repos:

```
mcp__lsp__call_hierarchy({
  "file_path": "<library-file>",
  "line": <line>,
  "column": <column>,
  "direction": "incoming"
})
```

For interfaces — all implementations including consumer-side:

```
mcp__lsp__go_to_implementation({
  "file_path": "<library-file>",
  "line": <line>,
  "column": <column>
})
```

## Output format

Report results in two sections:

```
## Library-internal references
- file:line — brief context

## Consumer references  
- file:line — brief context
```

If consumer references are found, summarize the blast radius before proceeding
with any changes.

## Decision guide

| Situation | Action |
|-----------|--------|
| No consumer refs found | Safe to change — but verify indexing first (Step 3) |
| Consumer refs found | Run `/lsp-impact` on each call site before editing |
| `get_references` returns `[]` for a known-used symbol | Indexing incomplete — wait and retry |
| Consumer uses interface, not concrete type | Use `go_to_implementation` to find all implementors |

## Example

```
# Refactoring ParseConfig in a shared config library used by 3 services

start_lsp(root_dir="/repos/config-lib")
add_workspace_folder(path="/repos/api-service")
add_workspace_folder(path="/repos/worker-service")
list_workspace_folders()                           # verify 3 roots
get_workspace_symbols(query="ParseConfig")         # find definition
get_references(file_path=..., line=..., column=...) # all callers across all 3 repos
call_hierarchy(direction="incoming")               # callers with full context
```
