# agent-lsp Features Dump

Machine-readable feature inventory for AI analysis. Dense structured lists for tool discovery and capability queries. All 66 tools have `ToolAnnotations` (Title, ReadOnlyHint, DestructiveHint, IdempotentHint, OpenWorldHint) and 171+ `jsonschema` struct tags providing parameter semantics in the schema itself.

---

## Tools (66 total, 66 CI-verified)

### Session & Lifecycle (8 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `start_lsp` | Initialize LSP server with workspace root | `root_dir` (string, req), `language_id` (string, opt), `connect` (string, opt), `ready_timeout_seconds` (int, opt) |
| `restart_lsp_server` | Restart current LSP server process | `root_dir` (string, opt) |
| `open_document` | Register file with language server | `file_path` (string, req), `language_id` (string, opt), `text` (string, opt) |
| `close_document` | Unregister file from language server | `file_path` (string, req) |
| `add_workspace_folder` | Add directory to multi-root workspace | `path` (string, req) |
| `remove_workspace_folder` | Remove directory from workspace | `path` (string, req) |
| `list_workspace_folders` | List all workspace folders | none |
| `get_server_capabilities` | Get LSP server capability map | none |

**`start_lsp` notes:**
- Shuts down existing LSP process before starting new one, no resource leak
- Language server initialized but may not have finished indexing on return
- `connect` parameter enables passive mode: connect to an already-running language server via TCP (e.g. `gopls -listen=:9999`) instead of spawning a new process. Reuses the IDE's warm index with zero duplicate memory. Supported by gopls, clangd, and other servers with TCP listen mode.
- `ready_timeout_seconds`: blocks until all `$/progress` workspace-indexing tokens complete before returning, up to the specified timeout; fires as soon as indexing completes (does not always wait the full timeout); grace period for late-emitting servers; also exports `WaitForWorkspaceReadyTimeout` on `LSPClient` for programmatic use beyond the default 60s cap
- `find_references` waits for all `$/progress end` events before returning on large projects
- `language_id` selects specific server in multi-server mode; omit to start all

**`restart_lsp_server` notes:**
- Requires prior `start_lsp`; returns error if never called
- All open documents lost after restart; must call `open_document` again

**`open_document` notes:**
- Most analysis tools call this internally via `WithDocument` helper
- Explicit call needed only to pre-warm files or keep open across multiple operations
- Defaults to `"plaintext"` language_id if omitted

### Navigation (10 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `go_to_definition` | Jump to symbol definition | `file_path` (string, req), `line` (int, req), `column` (int, req), `position_pattern` (string, opt), `line_scope_start` (int, opt), `line_scope_end` (int, opt) |
| `go_to_type_definition` | Jump to type declaration | `file_path` (string, req), `line` (int, req), `column` (int, req) |
| `go_to_implementation` | Find all concrete implementations | `file_path` (string, req), `line` (int, req), `column` (int, req) |
| `go_to_declaration` | Jump to symbol declaration | `file_path` (string, req), `line` (int, req), `column` (int, req) |
| `go_to_symbol` | Navigate by dot-notation symbol name | `symbol_path` (string, req), `workspace_root` (string, req), `language` (string, opt) |
| `rename_symbol` | Rename symbol across workspace | `file_path` (string, req), `line` (int, req), `column` (int, req), `new_name` (string, req), `dry_run` (bool, opt), `exclude_globs` ([]string, opt), `position_pattern` (string, opt), `line_scope_start` (int, opt), `line_scope_end` (int, opt) |
| `prepare_rename` | Validate rename at position | `file_path` (string, req), `line` (int, req), `column` (int, req) |
| `get_document_highlights` | Find all local occurrences (file-scoped) | `file_path` (string, req), `line` (int, req), `column` (int, req) |
| `find_callers` | Show incoming/outgoing calls | `file_path` (string, req), `line` (int, req), `column` (int, req), `direction` (string, opt: "both", "incoming", "outgoing") |
| `type_hierarchy` | Show supertypes/subtypes | `file_path` (string, req), `line` (int, req), `column` (int, req), `direction` (string, opt: "both", "supertypes", "subtypes") |

**`rename_symbol` notes:**
- `dry_run: true` returns `workspace_edit` preview without applying changes
- `exclude_globs`: array of glob patterns; matched against both full path and basename using `filepath.Match` syntax; useful for `**/*_gen.go`, `vendor/**`, `testdata/**`
- Returns `workspace_edit` on both dry-run and live runs; caller passes to `apply_edit` to commit

**`go_to_symbol` notes:**
- `symbol_path` uses dot notation: `"codec.Encode"`, `"Buffer.Reset"`, `"Package.OldName"`
- Returns file, line, column (1-indexed)

**`find_callers` notes:**
- Single tool handles `textDocument/prepareCallHierarchy` + `callHierarchy/incomingCalls` + `callHierarchy/outgoingCalls`
- `direction: "both"` runs all three

**`type_hierarchy` notes:**
- Single tool handles `textDocument/prepareTypeHierarchy` + `typeHierarchy/supertypes` + `typeHierarchy/subtypes`
- Tested on Java (jdtls) and TypeScript; TypeScript skips when server does not return hierarchy item

### Analysis (15 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `inspect_symbol` | Hover information at position | `file_path` (string, req), `line` (int, opt), `column` (int, opt), `position_pattern` (string, opt), `line_scope_start` (int, opt), `line_scope_end` (int, opt) |
| `get_completions` | Code completions at position | `file_path` (string, req), `line` (int, req), `column` (int, req) |
| `get_signature_help` | Function signature at cursor | `file_path` (string, req), `line` (int, req), `column` (int, req) |
| `suggest_fixes` | Available refactorings/fixes | `file_path` (string, req), `start_line` (int, req), `start_column` (int, req), `end_line` (int, req), `end_column` (int, req) |
| `list_symbols` | All symbols in file | `file_path` (string, req), `language_id` (string, opt), `format` (string, opt: "outline") |
| `find_symbol` | Symbols across workspace | `query` (string, req), `detail_level` (string, opt: "basic", "hover"), `limit` (int, opt), `offset` (int, opt) |
| `find_references` | All usages of symbol | `file_path` (string, req), `line` (int, opt), `column` (int, opt), `include_declaration` (bool, opt), `position_pattern` (string, opt), `line_scope_start` (int, opt), `line_scope_end` (int, opt) |
| `get_inlay_hints` | Type annotations/param labels | `file_path` (string, req), `start_line` (int, req), `start_column` (int, req), `end_line` (int, req), `end_column` (int, req) |
| `get_semantic_tokens` | Token type classification | `file_path` (string, req), `start_line` (int, req), `start_column` (int, req), `end_line` (int, req), `end_column` (int, req) |
| `get_symbol_source` | Extract source text for symbol | `file_path` (string, req), `line` (int, req), `column` (int, opt), `position_pattern` (string, opt), `line_scope_start` (int, opt), `line_scope_end` (int, opt) |
| `get_symbol_documentation` | Toolchain docs (go doc, pydoc, cargo doc) | `symbol` (string, req), `language_id` (string, req), `format` (string, opt) |
| `blast_radius` | Blast-radius analysis | `changed_files` (array, req), `include_transitive` (bool, opt) |
| `get_cross_repo_references` | Find usages across consumer repos | `symbol_file` (string, req), `line` (int, req), `column` (int, req), `consumer_roots` (array, req), `language_id` (string, opt) |
| `detect_changes` | Git diff + impact analysis + risk classification | `workspace_root` (string, opt), `scope` (string, opt: "unstaged", "staged", "committed"), `range` (string, opt) |
| `get_editing_context` | Complete pre-edit context in one call | `file_path` (string, req), `language_id` (string, opt), `if_none_match` (string, opt) |

**`detect_changes` notes:**
- Runs `git diff --name-only` for the specified scope (default: unstaged)
- `range` parameter (for "committed" scope only): accepts arbitrary git ranges like `"v0.7.0..HEAD"`, `"abc123..def456"`, or a single ref like `"main"` (expands to `main~1..main`). Ignored for unstaged/staged scopes.
- Filters to recognized source files (skips plaintext, deleted files)
- Feeds filtered files to `blast_radius` for symbol-level analysis
- Enriches each symbol with risk classification: "high" (callers across multiple packages), "medium" (same-package callers only), "low" (zero non-test callers)
- Returns `changed_files`, `affected_symbols` (with risk), and `scope`

**`suggest_fixes` notes:**
- `CodeActionContext.diagnostics` auto-populated with overlapping diagnostics from current diagnostic state; enables diagnostic-specific quick fixes; empty array would suppress fixes tied to visible errors
- Returns `(Command | CodeAction)[]`; normalized to `[]CodeAction`; bare commands wrapped in synthetic CodeAction

**`list_symbols` notes:**
- Returns `DocumentSymbol[] | SymbolInformation[]`; normalized to `[]DocumentSymbol`
- `selectionRange.start.line` and `selectionRange.start.character` are 1-based; pass directly to `find_references`
- `SymbolInformation[]` variant: three-pass tree reconstruction (name→symbol map, attach children by containerName, collect roots); keyed by `name\x00kind` to handle duplicate names across types

**`get_symbol_source` notes:**
- Walks symbol tree with `findInnermostSymbol` to find deepest symbol whose Range contains cursor
- Returns `{SymbolName, SymbolKind, StartLine, EndLine, Source}` with 1-based line numbers

**`get_symbol_documentation` notes:**
- Dispatches to language toolchain, not LSP hover
- Go: `go doc [pkg] Symbol`; walks up from file to locate `go.mod`, constructs fully-qualified package path
- Python: `python3 -m pydoc Symbol`
- Rust: `cargo doc --no-deps --message-format short`
- TypeScript/JavaScript: explicitly unsupported (use LSP hover instead)
- Strips ANSI escape codes; extracts `Signature` from first matching declaration line

**`blast_radius` notes:**
- Enumerates all exported symbols in `changed_files` via `list_symbols`
- Includes exported methods (receiver prefix fix): methods like `(*Hub).SetSender` are now detected correctly; the method name after the last dot is checked for uppercase, not the full name starting with `(`
- Recurses into type children to find methods on types (e.g., `(*Hub).Send`), while filtering out struct fields
- Resolves references for each symbol via `find_references`
- Partitions results: test callers (with enclosing test function names extracted) vs non-test callers
- `affected_symbols` field includes per-symbol `test_callers` and `non_test_callers` lists; agents can see which tests cover each specific method, not just a flat list for the file
- Methods on types containing sync primitives (Mutex, RWMutex, Lock, atomic) include `"sync_guarded": true` in `affected_symbols`. Covers Go, Java, Rust, Python, C/C++ primitives. Agents can distinguish mutex-guarded code from pure functions when assessing blast radius.
- `include_transitive: true` follows one level of transitive callers
- Errors from per-symbol reference lookups surfaced in `warnings` field (not silently discarded)

**`get_editing_context` notes:**
- Composite tool: returns file symbols with signatures, callers partitioned by test/non-test, callees, and imports in one call
- Replaces the 3-5 tool sequence agents previously used to gather pre-edit context (`list_symbols` + `find_references` per symbol + `find_callers`)
- Supports `if_none_match`: pass an ETag from a previous response; if the file content hash matches, returns `not_modified` without recomputing
- Includes `_meta.token_savings` showing tokens returned vs full file size (via `AppendTokenMeta`)
- Handler: `HandleGetEditingContextWithMeta` wraps the core handler with token metadata

**`get_cross_repo_references` notes:**
- Adds each consumer root as workspace folder via `add_workspace_folder`
- Waits for indexing, runs `find_references` across all roots
- Returns: `library_references` (within library), `consumer_references` (map of root → locations), `warnings` (failed roots)
- Results partitioned by repo root prefix

**`find_references` notes:**
- Timeout: 120s (full workspace indexing window)
- Waits for `$/progress end` before sending on gopls (via `waitForWorkspaceReady`)
- `include_declaration: false` excludes definition site from count

### Composite Exploration (1 tool)

| Tool | Description | Parameters |
|------|-------------|------------|
| `explore_symbol` | Deep-dive into a symbol: type info, source, callers (top 10), references (count + top 5 files), test caller count in one call | `file_path` (string, req), `line` (int, opt), `column` (int, opt), `position_pattern` (string, opt), `language_id` (string, opt) |

**`explore_symbol` notes:**
- Composite tool combining hover, get_symbol_source, find_callers, find_references into a single response
- Replaces the 4-5 tool sequence agents previously used to understand a symbol before editing
- Accepts position_pattern as alternative to line/column

### Safe Editing (1 tool)

| Tool | Description | Parameters |
|------|-------------|------------|
| `safe_apply_edit` | Preview + apply in one call; applies only if net_delta == 0 | `file_path` (string, req), `old_text` (string, req), `new_text` (string, req) |

**`safe_apply_edit` notes:**
- Internally calls `preview_edit` then `apply_edit` when safe
- Returns `applied: true` on success, `applied: false` with preview diagnostics when the edit would introduce errors
- Agents skip the manual preview-then-apply two-step

### Intent Aliases (4 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `blast_radius` | Alias for `blast_radius` | Same as `blast_radius` |
| `callers` | Find incoming callers (shortcut for `find_callers` with direction=incoming) | Same as `find_callers` |
| `explore` | Composite symbol exploration (same handler as `explore_symbol`) | Same as `explore_symbol` |
| `safe_edit` | Preview + apply when safe (same handler as `safe_apply_edit`) | Same as `safe_apply_edit` |

**Intent alias notes:**
- Shorter, intent-oriented names for common operations
- Same handlers and parameters as the underlying tools
- Registered as separate MCP tools so agents can discover them by intent

### Workspace & Diagnostics (8 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `get_diagnostics` | Errors/warnings for files | `file_path` (string, opt) |
| `format_document` | Format entire file | `file_path` (string, req), `language_id` (string, opt), `insert_spaces` (bool, opt), `tab_size` (int, opt) |
| `format_range` | Format selection | `file_path` (string, req), `start_line` (int, req), `start_column` (int, req), `end_line` (int, req), `end_column` (int, req), `language_id` (string, opt), `tab_size` (int, opt), `insert_spaces` (bool, opt) |
| `apply_edit` | Apply workspace edit | `file_path` (string, req), `old_text` (string, req), `new_text` (string, req) OR `workspace_edit` (object, req) |
| `execute_command` | Run LSP workspace command | `command` (string, req), `arguments` (array, opt) |
| `did_change_watched_files` | Notify of file changes | `changes` (array, req) |
| `export_cache` | Export reference cache as gzip artifact | `dest_path` (string, req) |
| `import_cache` | Import reference cache from gzip artifact | `src_path` (string, req) |

### Symbol-Level Editing (4 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `replace_symbol_body` | Replace a symbol's body by name | `file_path` (string, req), `symbol_path` (string, req), `new_body` (string, req) |
| `insert_after_symbol` | Insert code after a named symbol | `file_path` (string, req), `symbol_path` (string, req), `code` (string, req) |
| `insert_before_symbol` | Insert code before a named symbol | `file_path` (string, req), `symbol_path` (string, req), `code` (string, req) |
| `safe_delete_symbol` | Delete a symbol only if zero references | `file_path` (string, req), `symbol_path` (string, req) |

**`export_cache` notes:**
- Compacts the SQLite reference cache with `VACUUM INTO`, then gzip-compresses to dest_path
- Returns entry count on success
- Requires an active LSP session with reference cache enabled

**`import_cache` notes:**
- Decompresses gzip artifact, validates with `PRAGMA integrity_check`, atomically replaces current cache
- Returns entry count on success
- Enables team-shared cache: commit the artifact, teammates import to skip cold-start indexing

**`get_diagnostics` notes:**
- `file_path` validates via `ValidateFilePath` before `CreateFileURI` (path traversal prevented)
- Calls `WaitForDiagnostics` with 500ms stabilization window and configurable timeout
- Returns errors then warnings ranked by severity
- `group_by: "symbol"` groups diagnostics under their owning function/type/method. Each diagnostic is assigned to the innermost containing symbol via range containment. Helps agents understand "this function is broken" vs "this file has problems."

**`did_change_watched_files` notes:**
- Not required for normal editing; auto-watcher sends these automatically
- Use when caller manages file changes outside the watched directory

**`set_log_level` (tool 50, workspace category):**
- Sets minimum log level: `debug`, `info`, `notice`, `warning`, `error`, `critical`, `alert`, `emergency`
- Also configurable via `LOG_LEVEL` env var
- Parameters: `level` (string, req)
- No LSP required; CI-verified for all 30 languages

### Build & Test (4 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `run_build` | Compile project | `workspace_dir` (string, req), `language` (string, opt), `path` (string, opt) |
| `run_tests` | Run test suite | `workspace_dir` (string, req), `language` (string, opt), `path` (string, opt) |
| `get_tests_for_file` | Find tests covering source file | `file_path` (string, req) |
| `detect_lsp_servers` | Scan PATH for language servers | `workspace_dir` (string, req) |

**`run_build` / `run_tests` notes:**
- Does NOT require `start_lsp`
- Returns `{ "success": bool, "errors": [...] }` / `{ "passed": bool, "failures": [...] }`
- Language auto-detected from workspace if `language` omitted
- `parseBuildErrors`: tested for TypeScript, Rust, Python synthetic compiler output

**`detect_lsp_servers` notes:**
- Scans PATH for known language server binaries
- Used by `agent-lsp init` to auto-discover installed servers

### Speculative Execution (8 tools)

| Tool | Description | Parameters |
|------|-------------|------------|
| `create_simulation_session` | Create isolated edit session | `workspace_root` (string, req), `language` (string, req) |
| `simulate_edit` | Apply hypothetical edit to session | `session_id` (string, req), `file_path` (string, req), `start_line` (int, req), `start_column` (int, req), `end_line` (int, req), `end_column` (int, req), `new_text` (string, req) |
| `evaluate_session` | Compute diagnostic delta | `session_id` (string, req), `scope` (string, opt: "file", "workspace"), `timeout_ms` (int, opt) |
| `simulate_chain` | Apply sequence of edits, evaluate each | `session_id` (string, req), `edits` (array, req), `timeout_ms` (int, opt) |
| `commit_session` | Materialize edits to disk | `session_id` (string, req), `target` (string, opt), `apply` (bool, opt) |
| `discard_session` | Revert all session edits | `session_id` (string, req) |
| `destroy_session` | Cleanup session state | `session_id` (string, req) |
| `preview_edit` | One-shot speculative edit | `file_path` (string, req), `start_line` (int, req), `start_column` (int, req), `end_line` (int, req), `end_column` (int, req), `new_text` (string, req), `workspace_root` (string, opt), `language` (string, opt), `session_id` (string, opt), `scope` (string, opt), `timeout_ms` (int, opt) |

**`simulate_edit` response shape:**
```json
{ "session_id": "...", "edit_applied": true, "version_after": 3 }
```

**`simulate_chain` response shape:**
```json
{
  "steps": [
    { "step": 1, "net_delta": 0, "errors_introduced": [] },
    { "step": 2, "net_delta": 3, "errors_introduced": [...] }
  ],
  "safe_to_apply_through_step": 1,
  "cumulative_delta": 3
}
```

**`commit_session` semantics:**
- Default (`apply: false`): returns `CommitResult{session_id, files_written: 0, patch}` (no disk write); `patch` is `map[string]string` (file URI → full file content)
- `apply: true`: writes changed files to disk, notifies LSP via `didChange`, returns same `CommitResult` shape with `files_written > 0`
- `target: "/path"`: writes to target path + returns patch
- Prohibited on `dirty` or `created` sessions; valid from `mutated` or `evaluated` state

**`preview_edit` notes:**
- Self-contained: requires `file_path` + (optionally) `workspace_root` + `language`; `session_id` is an optional bypass; if provided, uses an existing session instead of creating/destroying one
- Internally: create → apply → evaluate → discard → destroy
- Returns `EvaluationResult` directly
- `net_delta` filters out severity 3 (info) and 4 (hint) diagnostics from DiffDiagnostics; only errors and warnings count toward the delta

**`destroy_session` notes:**
- Returns success with `status: "already_destroyed"` instead of an error when the session does not exist or was already cleaned up
- Agents calling `destroy_session` after `preview_edit` (which auto-cleans up) no longer see a confusing error

**Total: 66 tools** (63 core + 3 phase enforcement)
- **CI-verified: 66** (including `set_log_level` verified separately across all 30 languages, and 3 phase enforcement tools verified via mcp-assert)
- **ToolAnnotations:** All 66 tools declare `Title`, `ReadOnlyHint`, `DestructiveHint`, `IdempotentHint`, `OpenWorldHint`; MCP clients can auto-approve ~30 read-only tools without human confirmation
- **jsonschema struct tags:** 171+ tags across all Args structs; 100% parameter description coverage
- **1-indexed coordinates:** All line/column parameters are 1-based (editor convention)
- **0-based conversion:** `extractRange` helper converts to 0-based for LSP protocol internally
- **Next-step hints:** Every tool response includes a contextual `hint` field suggesting the logical next tool call. For example, `find_references` hints "use blast_radius to see the full blast radius"; `detect_changes` hints "use blast_radius on specific files for detailed analysis." Helps agents chain tools correctly without skills, and helps less capable models navigate the 66-tool surface. Zero-cost addition: one extra field in the JSON response.
- **Token savings metadata:** `list_symbols`, `get_symbol_source`, and `get_editing_context` include `_meta.token_savings` in responses, showing tokens returned vs full file size. Makes the efficiency story visible on every call.
- **ETag/conditional responses:** `get_editing_context`, `list_symbols`, and `get_symbol_source` accept an `if_none_match` parameter. When the file's content hash matches, returns `not_modified` instead of recomputing. Eliminates redundant computation for unchanged files.
- **Position pattern without line/column:** `find_references` and `inspect_symbol` accept `position_pattern` without requiring `line`/`column` (fields are `*int` pointers, omittable). Agents can locate symbols by text pattern alone.
- **Indexed indicator:** `blast_radius`, `find_references`, and `find_symbol` responses include `indexed: true/false` via `AppendIndexedField`, indicating whether the workspace was fully indexed when results were computed. Agents can decide whether to retry after indexing completes.
- **Auto-diagnostics after symbol edits:** `replace_symbol_body`, `insert_after_symbol`, `insert_before_symbol`, and `safe_delete_symbol` responses include `errors_after` and `warnings_after` fields with post-edit diagnostic counts. Agents see whether an edit introduced problems without a separate `get_diagnostics` call.
- **Proactive diagnostic regression notifications:** `DiagChangeTracker` monitors diagnostic state across edits and pushes notifications when error/warning counts increase. Agents are alerted to regressions without polling.

---

## Skills (23 total)

| Skill | Invocation | Allowed Tools | Description |
|-------|-----------|---------------|-------------|
| `/lsp-rename` | `[old-name] [new-name]` | go_to_symbol, prepare_rename, find_references, rename_symbol, apply_edit, get_diagnostics | Two-phase safe rename: prepare_rename safety gate → preview all sites → hard stop for user confirmation → apply atomically |
| `/lsp-safe-edit` | target file(s) + intent | start_lsp, open_document, get_diagnostics, preview_edit, simulate_chain, suggest_fixes, format_document, apply_edit, Edit, Write, Bash | Speculative before/after diagnostic comparison; surfaces code actions on errors; multi-file aware; Step 3b uses simulate_chain for refactor preview |
| `/lsp-simulate` | workspace + intent | start_lsp, create_simulation_session, simulate_edit, simulate_chain, evaluate_session, commit_session, discard_session, destroy_session, preview_edit | Full session lifecycle management; decision guide on net_delta; cleanup rule enforced |
| `/lsp-impact` | `[symbol-name | file-path]` | go_to_symbol, find_callers, type_hierarchy, find_references, get_server_capabilities, blast_radius | Blast-radius analysis; file-level shortcut via blast_radius; symbol-level via Steps 1–5 |
| `/lsp-verify` | workspace_dir + changed_files | get_diagnostics, run_build, run_tests, get_tests_for_file, suggest_fixes, format_document, apply_edit | Three-layer verification: LSP diagnostics + build + tests; test correlation pre-step; code actions on errors |
| `/lsp-dead-code` | `[file-path]` | list_symbols, find_references, open_document, safe_delete_symbol | Enumerate exported symbols, check each for zero references; Step 0 warm-up sanity check required; cross-check with grep for registration patterns; optional cleanup via safe_delete_symbol |
| `/lsp-implement` | interface name | go_to_symbol, go_to_implementation, find_references | Find all concrete implementations of an interface before changing it |
| `/lsp-edit-export` | symbol name | go_to_symbol, find_references, get_diagnostics, run_build, replace_symbol_body | Safe editing of public APIs; finds all callers first; supports replace_symbol_body for full-body edits |
| `/lsp-edit-symbol` | symbol name + intent | find_symbol, list_symbols, apply_edit, replace_symbol_body | Edit named symbol without knowing file or position; primary path via replace_symbol_body |
| `/lsp-docs` | symbol name | go_to_symbol, inspect_symbol, get_symbol_documentation, get_symbol_source | Three-tier documentation: hover → offline toolchain (go doc/pydoc/cargo doc) → source |
| `/lsp-cross-repo` | symbol + consumer-roots | start_lsp, find_symbol, get_cross_repo_references, add_workspace_folder, list_workspace_folders, go_to_implementation, find_callers, inspect_symbol | Multi-root cross-repo caller analysis; results partitioned by repo |
| `/lsp-explore` | `[symbol-name]` | start_lsp, go_to_symbol, inspect_symbol, go_to_implementation, find_callers, find_references, open_document, get_server_capabilities | hover + implementations + call hierarchy + references in one pass; capability-gated steps; produces Explore Report |
| `/lsp-local-symbols` | `[file-path]` | list_symbols, find_references, inspect_symbol | File-scoped symbol list, usages within file, type info; faster than workspace search |
| `/lsp-test-correlation` | `[source-file]` | get_tests_for_file, run_tests | Find and run only tests covering an edited file |
| `/lsp-format-code` | `[file-path]` | format_document, format_range, apply_edit | Format file or selection via language server formatter; applies edits to disk |
| `/lsp-fix-all` | `[file-path]` | get_diagnostics, suggest_fixes, apply_edit, open_document, format_document | Sequential quick-fix loop: collect diagnostics → apply one fix → re-collect; quick-fix kind only; never batches |
| `/lsp-refactor` | `[symbol-or-file] [intent]` | blast_radius, preview_edit, simulate_chain, get_diagnostics, run_build, run_tests, get_tests_for_file, apply_edit, replace_symbol_body, format_document | End-to-end refactor: blast-radius, speculative preview, apply, build verify, affected tests; supports replace_symbol_body for full-body edits |
| `/lsp-extract-function` | `[file-path] [start-line] [end-line] [name]` | list_symbols, suggest_fixes, execute_command, apply_edit, get_diagnostics, format_document | Extract code block into named function; LSP code action primary, manual fallback with captured-variable analysis |
| `/lsp-generate` | `[file-path:line:col] [intent]` | suggest_fixes, execute_command, apply_edit, format_document, get_diagnostics, go_to_symbol | Language server code generation: interface stubs, test skeletons, missing methods, mocks |
| `/lsp-understand` | `[symbol-name \| file-path]` | inspect_symbol, go_to_implementation, find_callers, find_references, get_symbol_source, list_symbols, go_to_symbol | Deep Code Map: type info + implementations + call hierarchy (2-level) + references + source; synthesizes cross-symbol relationships |
| `/lsp-inspect` | `<file-or-directory> [--checks <types>] [--json] [--top N] [--diff]` | blast_radius, find_references, list_symbols, inspect_symbol, get_diagnostics, find_callers, go_to_definition, get_server_capabilities | Full code quality audit (12 check types): dead symbols, test coverage, silent failures, error wrapping, doc drift, panics, context propagation, unrecovered concurrent entry, unchecked shared state, channel never closed, shared field without sync; batch mode with --top ranking; comparison mode with --diff; blast-radius severity calibration; fix suggestions; confidence tiers (verified/suspected/advisory); result persistence via inspect://last resource |
| `/lsp-architecture` | `[workspace-root-path]` | start_lsp, list_symbols, blast_radius, detect_lsp_servers, find_symbol | Project-level architecture overview: language distribution, package map (capped at 30), entry points, hotspots (top 10 by reference count), dependency flow. Read-only. |
| `/lsp-onboard` | `[workspace-root-path]` | start_lsp, detect_lsp_servers, list_symbols, find_symbol, blast_radius, run_build, run_tests, get_diagnostics, get_editing_context | First-session project onboarding: detect languages, build system, entry points, package map, hotspots, diagnostics baseline. Produces a structured project profile. |
| `/lsp-concurrency-audit` | `[type-name]` | find_callers, blast_radius, list_symbols, inspect_symbol, find_references | Field-level concurrency safety audit: maps all fields on a type, traces concurrent access via find_callers(cross_concurrent) + blast_radius(sync_guarded), classifies each field as SAFE/UNSAFE/WRITE-CONCURRENT/READ-ONLY. Language-agnostic across 4 concurrency families. |

**User-facing reference:** `docs/skills.md` (one-page skill catalog with usage examples and trigger conditions)

**Discovery:** Skills are available through two channels:
- **MCP prompts:** Any MCP client discovers skills via `prompts/list` (short descriptions) and retrieves full instructions via `prompts/get`. No manual installation required. Skill definitions are embedded in the binary.
- **AgentSkills install:** `cd skills && ./install.sh` copies SKILL.md files to `~/.claude/skills/` for Claude Code slash command access. Flags: `--copy`, `--force`, `--dry-run`. Scans for `SKILL.md` files up to two levels deep.

**CLAUDE.md sync:** `install.sh` maintains managed skills table in `~/.claude/CLAUDE.md` between sentinel comments (`<!-- agent-lsp:skills:start/end -->`). Auto-discovers skills from SKILL.md frontmatter; re-running keeps CLAUDE.md in sync without touching surrounding content.

**SKILL.md format ([AgentSkills](https://agentskills.io/specification) conformant):**
```markdown
---
name: lsp-verify
description: <one-line description for skill discovery>
argument-hint: "[optional-args]"    # optional
license: MIT
compatibility: Requires the agent-lsp MCP server
allowed-tools: mcp__lsp__get_diagnostics mcp__lsp__run_build ...
metadata:
  required-capabilities: referencesProvider
  optional-capabilities: callHierarchyProvider typeHierarchyProvider
---
# skill body (prompt for agent)
```

**Capability metadata:** All 24 skills declare `required-capabilities` and `optional-capabilities` in frontmatter. Maps directly to LSP server capability keys from `get_server_capabilities`. Agents can check before activation whether the current language server supports the skill's requirements. Skills with zero required capabilities (lsp-safe-edit, lsp-simulate, lsp-verify, lsp-test-correlation) work with any language server.

**Runtime skill classification:** `get_server_capabilities` now includes a `skills` array in its response, classifying every skill as `supported` (all required capabilities present), `partial` (required present, some optional missing), or `unsupported` (missing required capabilities). One call at session start tells the agent exactly which skills to use and which to skip.

| Capability | Skills that require it | Skills that optionally use it |
|---|---|---|
| `referencesProvider` | lsp-rename, lsp-impact, lsp-dead-code, lsp-edit-export, lsp-cross-repo, lsp-refactor | lsp-explore, lsp-understand |
| `codeActionProvider` | lsp-fix-all, lsp-extract-function, lsp-generate | lsp-safe-edit, lsp-verify |
| `workspaceSymbolProvider` | lsp-rename, lsp-edit-symbol | lsp-impact, lsp-implement, lsp-cross-repo, lsp-generate, lsp-understand, lsp-test-correlation, lsp-edit-export |
| `hoverProvider` | lsp-docs, lsp-explore, lsp-understand | lsp-local-symbols |
| `documentSymbolProvider` | lsp-dead-code, lsp-local-symbols | lsp-extract-function, lsp-understand |
| `implementationProvider` | lsp-implement | lsp-cross-repo, lsp-explore, lsp-understand |
| `documentFormattingProvider` | lsp-format-code | lsp-safe-edit, lsp-verify, lsp-fix-all, lsp-refactor, lsp-extract-function, lsp-generate |
| `renameProvider` | lsp-rename | (none) |
| `callHierarchyProvider` | (none) | lsp-impact, lsp-cross-repo, lsp-explore, lsp-understand |
| `typeHierarchyProvider` | (none) | lsp-impact, lsp-implement |

**Provider-agnostic:** Skills conform to the AgentSkills open standard and work with any conforming agent (Claude Code, Cursor, GitHub Copilot, Gemini CLI, OpenAI Codex, JetBrains Junie, and 30+ others). The `--dest` flag on `install.sh` installs to any agent's skill directory. The installer updates CLAUDE.md, AGENTS.md (Codex), and GEMINI.md instruction files when present.

**MCP prompts:** All 24 skills are also exposed via `prompts/list` and `prompts/get`. Any MCP client discovers them on connection without manual installation. `prompts/list` returns short descriptions (minimal context cost); full workflow instructions load on demand via `prompts/get`. Skill SKILL.md files are embedded in the binary at build time.

---

## Reliability

**Process lifecycle cleanup:** `Shutdown()` sends `shutdown`/`exit` to the language server, waits up to 3 seconds, then force-kills. `resolver.Shutdown()` runs on every exit path (signal, panic, stdin EOF). `StartForLanguage` shuts down the previous client before starting a new one. Prevents orphaned gopls/pyright processes across sessions.

**Write mutex separation:** `writeRaw` uses a dedicated `writeMu` instead of the shared `c.mu`. Prevents stdin pipe backpressure from deadlocking the client when concurrent reference queries fill the OS pipe buffer (64KB on macOS).

**Per-symbol timeout:** `blast_radius` caps each reference query at 15 seconds. Prevents one slow symbol from blocking the entire batch. Timed-out symbols are skipped with a warning.

**Diagnostic logging:** Every tool call logs latency via the central `addToolWithPhaseCheck` wrapper. Calls exceeding 5 seconds log at WARNING level. Process start/exit events log PID and uptime.

**Daemon broker panic recovery:** All goroutines in `RunBroker` have `defer recover()`. Forwarded requests use the broker's lifecycle context (cancellable on shutdown).

**Persistent reference cache:** Symbol reference results are cached in a per-workspace SQLite database (`~/.agent-lsp/cache/<hash>/refs.db`), keyed by file content hash. Subsequent sessions serve cached results instantly; the language server is only re-queried for files that changed. File watcher invalidates entries on source changes. Cache is opportunistic: missing or corrupted databases fall back to direct LSP queries transparently. Pure Go SQLite (`modernc.org/sqlite`), no CGo.

**Selective indexing:** On workspaces with 500+ source files (Python, TypeScript), auto-detects the active package boundary and generates scoped language server config. The scope shifts automatically as the agent navigates between packages. Combined with the persistent cache, previously-visited packages serve cached results while the current package gets full LSP precision. Go and Rust bypass this entirely (native module boundaries).

### Skill Workflow Details

**`/lsp-rename` phase structure:**
1. Phase 1 (preview): go_to_symbol → prepare_rename → find_references → rename_symbol(dry_run=true) → hard stop (must confirm)
2. Edge case: 0 references → warning + confirmation required
3. Phase 2 (execute): capture pre-rename diagnostics → rename_symbol → apply_edit → post-rename diagnostics diff

**`/lsp-safe-edit` step structure:**
1. open_document for each target file
2. Capture BEFORE diagnostics
3. preview_edit (step 3): decision on net_delta ≤ 0 vs > 0
4. (Step 3b) simulate_chain for renames/signature changes: check cumulative_delta + safe_to_apply_through_step
5. Apply edit to disk (Edit/Write tool)
6. Capture AFTER diagnostics
7. Compute diff: introduced = AFTER not in BEFORE; resolved = BEFORE not in AFTER
8. Surface code actions for introduced errors
9. Optional format_document on clean diff

**`/lsp-simulate` decision guide:**

| net_delta | confidence | Action |
|-----------|------------|--------|
| 0 | high | Safe. Commit or apply. |
| 0 | eventual | Likely safe. Workspace scope; re-evaluate if risk matters. |
| > 0 | any | Do NOT apply. Inspect errors_introduced. Discard. |
| > 0 | partial | Timeout. Results incomplete. Discard and retry smaller scope. |

**`/lsp-dead-code` caveats (false zero-reference cases):**
- Registration patterns: `server.AddTool(HandleFoo)` (handler passed as value, no static call site)
- Reflection/dynamic dispatch
- `//go:linkname` and assembly references in Go
- External package consumers not in workspace
- Incomplete indexing (Step 0 warm-up check mitigates)
- Fix: grep wiring files for zero-reference symbols before classifying dead

**`/lsp-impact` file-level entry (Step 0):**
- Accepts file path → `blast_radius` → `affected_symbols`, `test_callers`, `non_test_callers`
- Decision: 0 non-test callers = low risk; many callers = staged rollout consideration

**`/lsp-explore` phases:**
1. Phase 1: go_to_symbol → open_document
2. Phase 2: inspect_symbol (hover, always)
3. Phase 3: get_server_capabilities → go_to_implementation (if supported)
4. Phase 4 (parallel): find_callers(incoming) + find_references
5. Output: Explore Report with definition, implementations, callers, references, summary

**`/lsp-cross-repo` output structure:**
```
library_references: [file:line ...]
consumer_references: { "/path/to/consumer-a": [file:line ...], ... }
warnings: [roots that failed indexing]
```

**`/lsp-inspect` capabilities (12 check types):**
- **Batch mode:** Directory-level inspection with `--top N` ranked output. Walks all `.go`, `.ts`, `.py` files recursively and produces findings sorted by severity then blast radius.
- **Comparison mode:** `--diff` flag for branch-only issue detection. Filters findings to lines within git diff ranges against main. Output prefixed with "New issues introduced by this branch."
- **Unexported dead code detection:** Pass `scope='all'` to `blast_radius` to include unexported/lowercase symbols in dead code analysis, not just exported symbols.
- **MCP resource `inspect://last`:** Programmatic access to the last inspection result. Results persisted to `.agent-lsp/last-inspection.json` in workspace root.
- **Confidence tiers:** `verified` (LSP-confirmed, act immediately), `suspected` (pattern match, investigate first), `advisory` (style, optional). Replaces the previous high/medium/low labels.
- **Fix suggestions:** Every finding includes exact fix text (e.g., "Remove lines N-M", "Change `return err` to `return fmt.Errorf(...)`").
- **Blast-radius severity calibration:** Severity escalates based on `non_test_callers` count from `blast_radius`. Functions with 10+ callers have findings escalated by one tier (info->warning, warning->error).
- **Concurrency safety checks (4 check types):**
  - `unrecovered_concurrent_entry`: detects goroutines, threads, and async tasks without recovery across 10 languages. Covers 4 concurrency families (goroutine, thread, async, actor).
  - `unchecked_shared_state`: detects bare type assertions on sync.Map, ConcurrentHashMap, and similar concurrent collections.
  - `channel_never_closed`: detects channels or queues created but never closed within the same scope.
  - `shared_field_without_sync`: detects fields accessed from multiple concurrent contexts without synchronization. Composes `blast_radius(sync_guarded)` + `find_callers(cross_concurrent)` to identify unprotected shared state.

---

## Languages (30 CI-verified)

| Language | Server Binary | CI Status | Notes |
|----------|---------------|-----------|-------|
| TypeScript | `typescript-language-server` | passing | `npm i -g typescript-language-server typescript` |
| Python | `pyright-langserver` | passing | `npm i -g pyright`; **daemon mode**: persistent broker for large repos (validated on FastAPI, 1,119 files) |
| Go | `gopls` | passing | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | passing | `rustup component add rust-analyzer` |
| Java | `jdtls` | flaky | cold-start indexing; Tier 2 skipped on timeout; eclipse.jdt.ls snapshots |
| C | `clangd` | passing | `apt install clangd` / `brew install llvm` |
| PHP | `intelephense` | passing | `npm i -g intelephense` |
| C++ | `clangd` | passing | shared binary with C |
| JavaScript | `typescript-language-server` | passing | shared binary with TypeScript |
| Ruby | `solargraph` | passing | `gem install solargraph` |
| YAML | `yaml-language-server` | passing | `npm i -g yaml-language-server` |
| JSON | `vscode-json-language-server` | passing | `npm i -g vscode-langservers-extracted` |
| Dockerfile | `docker-langserver` | passing | `npm i -g dockerfile-language-server-nodejs` |
| C# | `csharp-ls` | passing | `dotnet tool install -g csharp-ls` |
| Kotlin | `kotlin-language-server` | passing | GitHub releases |
| Lua | `lua-language-server` | passing | GitHub releases |
| Swift | `sourcekit-lsp` | passing | macos-latest runner only; ships with Xcode |
| Zig | `zls` | passing | must match Zig version exactly |
| CSS | `vscode-css-language-server` | passing | `npm i -g vscode-langservers-extracted` |
| HTML | `vscode-html-language-server` | passing | `npm i -g vscode-langservers-extracted` |
| Terraform | `terraform-ls` | passing | releases.hashicorp.com |
| Scala | `metals` | best-effort | cold-start; continue-on-error; `cs install metals` via Coursier |
| Gleam | `gleam` | passing | built-in LSP (`serverArgs: ["lsp"]`) |
| Elixir | `elixir-ls` | best-effort | continue-on-error; `language_server.sh` symlinked as `elixir-ls`; 16 verified capabilities |
| Prisma | `prisma-language-server` | investigating | requires VS Code extension host; `npm i -g @prisma/language-server` |
| SQL | `sqls` | passing | postgres:16 service container; `go install github.com/sqls-server/sqls@latest` |
| Clojure | `clojure-lsp` | passing | native binary from GitHub releases |
| Nix | `nil` | passing | `nix profile install github:oxalica/nil`; DeterminateSystems/nix-installer-action required in CI |
| Dart | `dart` | passing | Ships with Dart SDK; `brew install dart` |
| MongoDB | `mongodb-language-server` | investigating | extracted from vscode VSIX at `dist/languageServer.js`; mongo:7 service container |

**Tier 1 (Core 4 tools):** `start_lsp`, `open_document`, `get_diagnostics`, `inspect_symbol`, verified for all 30 languages
**Tier 2 (Extended 34 tools):** verified per-language; coverage varies by server capabilities

### CI Tool Coverage Matrix (Tier 2)

| Language | symbols | definition | references | completions | workspace | format | declaration | type_hier | hover | call_hier | sem_tok | sig_help |
|----------|---------|------------|------------|-------------|-----------|--------|-------------|-----------|-------|-----------|---------|----------|
| TypeScript | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | pass |
| Python | pass | pass | pass | pass | pass | — | — | — | pass | pass | pass | — |
| Go | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Rust | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | — |
| Java | — | — | — | — | — | — | — | pass | pass | pass | — | — |
| C | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| PHP | pass | pass | pass | pass | pass | — | — | — | pass | pass | pass | pass |
| C++ | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| JavaScript | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| Ruby | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| YAML | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| JSON | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| Dockerfile | — | — | — | pass | pass | — | — | — | pass | — | — | — |
| C# | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Kotlin | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Lua | pass | — | — | pass | pass | pass | — | — | pass | pass | pass | pass |
| Swift | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Zig | pass | pass | pass | pass | fail | pass | — | — | pass | — | pass | pass |
| CSS | pass | — | — | pass | pass | pass | — | — | pass | — | — | — |
| HTML | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| Terraform | pass | pass | — | pass | pass | pass | — | — | pass | — | — | — |
| Scala | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Gleam | pass | pass | pass | pass | fail | pass | — | — | pass | — | — | — |
| Elixir | fail | pass | pass | pass | pass | pass | — | — | pass | pass | — | pass |
| Prisma | pass | pass | pass | — | — | pass | — | — | pass | — | — | — |
| SQL | pass | pass | pass | pass | pass | — | — | — | pass | — | — | — |
| Clojure | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| Nix | pass | — | — | pass | pass | — | — | — | pass | — | — | — |
| Dart | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| MongoDB | — | — | — | pass | pass | — | — | — | pass | — | — | — |

### Language Expansion Tiers

| Tier | Languages | Count | Notes |
|------|-----------|-------|-------|
| Current | all 30 above | 30 | |
| Tier 3 candidates | Bash (bash-language-server) | 1 | good hover and completions; definition/references limited |
| Tier 4 — skip for now | Haskell (ghcup slow), OCaml (opam nontrivial), Elm (niche), R (niche) | 4 | CI complexity blockers |

### Adding a New Language: Required Steps

1. **`langConfig` entry** in `test/multi_lang_test.go` `buildLanguageConfigs()`:
   - Fields: `binary`, `serverArgs`, `fixture`, `file`, `hoverLine/hoverColumn`, `definitionLine/definitionColumn`, `referenceLine/referenceColumn`, `completionLine/completionColumn`, `workspaceSymbol`, `secondFile`, `supportsFormatting`, `declarationLine/declarationColumn`, `highlightLine/highlightColumn`, `inlayHintEndLine`, `renameSymbolLine/renameSymbolColumn/renameSymbolName`, `codeActionLine/codeActionEndLine`
2. **Fixture files** in `test/fixtures/<lang>/`: primary file (Person class/struct), greeter cross-file, build/project file if required
3. **CI install step** in `.github/workflows/ci.yml`: job selection based on weight (JVM → multi-lang-core; lightweight npm → multi-lang-extended; macOS-only → dedicated macos-latest; heavy/slow → dedicated + continue-on-error)

---

## LSP 3.17 Conformance

### LSP Method → MCP Tool Mapping

| LSP Method | Spec § | MCP Tool | Status |
|-----------|--------|----------|--------|
| `textDocument/didOpen` | §3.15.7 | `open_document` | ✓ |
| `textDocument/didClose` | §3.15.9 | `close_document` | ✓ |
| `textDocument/publishDiagnostics` | §3.17.1 | `get_diagnostics` | ✓ |
| `textDocument/hover` | §3.15.11 | `inspect_symbol` | ✓ |
| `textDocument/completion` | §3.15.13 | `get_completions` | ✓ |
| `textDocument/signatureHelp` | §3.15.14 | `get_signature_help` | ✓ |
| `textDocument/definition` | §3.15.2 | `go_to_definition` | ✓ |
| `textDocument/references` | §3.15.8 | `find_references` | ✓ |
| `textDocument/documentSymbol` | §3.15.20 | `list_symbols` | ✓ |
| `textDocument/codeAction` | §3.15.22 | `suggest_fixes` | ✓ |
| `textDocument/formatting` | §3.15.16 | `format_document` | ✓ |
| `textDocument/rangeFormatting` | §3.15.17 | `format_range` | ✓ |
| `textDocument/rename` | §3.15.19 | `rename_symbol` | ✓ |
| `textDocument/prepareRename` | §3.15.19 | `prepare_rename` | ✓ |
| `textDocument/typeDefinition` | §3.15.3 | `go_to_type_definition` | ✓ |
| `textDocument/implementation` | §3.15.4 | `go_to_implementation` | ✓ |
| `textDocument/declaration` | §3.15.5 | `go_to_declaration` | ✓ |
| `textDocument/documentHighlight` | §3.15.10 | `get_document_highlights` | ✓ |
| `textDocument/inlayHint` | §3.17.11 | `get_inlay_hints` | ✓ |
| `textDocument/semanticTokens/full` | §3.16.12 | `get_semantic_tokens` | ✓ |
| `textDocument/prepareCallHierarchy` + `callHierarchy/incomingCalls` + `callHierarchy/outgoingCalls` | §3.16.5 | `find_callers` | ✓ |
| `textDocument/prepareTypeHierarchy` + `typeHierarchy/supertypes` + `typeHierarchy/subtypes` | §3.17.12 | `type_hierarchy` | ✓ |
| `textDocument/selectionRange` | §3.15.29 | — | ✗ not implemented |
| `textDocument/foldingRange` | §3.15.28 | — | ✗ not implemented |
| `textDocument/codeLens` | §3.15.21 | — | ✗ not implemented |
| `workspace/symbol` | §3.15.21 | `find_symbol` | ✓ |
| `workspace/configuration` | §3.16.14 | — | ✓ protocol only (server-initiated; responds null×items.length) |
| `workspace/executeCommand` | §3.16.13 | `execute_command` | ✓ |
| `workspace/didChangeWatchedFiles` | §3.16.8 | `did_change_watched_files` (+ auto-watch) | ✓ |
| `workspace/didChangeWorkspaceFolders` | §3.16.5 | `add_workspace_folder`, `remove_workspace_folder` | ✓ |

### Protocol Compliance

- **Lifecycle:** `initialize` → `initialized` → `shutdown` fully implemented; graceful async shutdown via SIGINT/SIGTERM; subprocess never orphaned
- **Initialize timeout:** 300s to accommodate JVM servers (jdtls cold-start 60–90s)
- **Progress:** `$/progress` begin/report/end + `window/workDoneProgress/create`; token pre-registered before response; `waitForWorkspaceReady` blocks references until all progress tokens complete
- **Server-initiated:** `workspace/configuration` (null×items), `client/registerCapability` (null), `window/workDoneProgress/create` (null) all handled; unrecognized requests get null to unblock server
- **Capability check:** server capabilities checked before sending requests; unsupported features skipped rather than sent to fail silently
- **Message framing:** Content-Length with UTF-8 byte counts (not character counts), `\r\n\r\n` delimiter; buffer overflow >10MB discards entire buffer
- **JSON-RPC 2.0:** Full compliance; IDs monotonically incrementing integers; string IDs also supported (Prisma compatibility)
- **Error codes:** `-32601` (MethodNotFound) → warning; `-32002` (ServerNotInitialized) → warning; others → debug
- **Process crash:** exit-monitor goroutine calls `rejectPending`, sets `initialized=false`; callers fail fast
- **Capabilities declared:** hover, completion, references, definition, implementation, typeDefinition, declaration, codeAction, publishDiagnostics, window.workDoneProgress, workspace.configuration, workspace.didChangeWatchedFiles

### Previously Non-Conformant (Fixed)

| Issue | Fix |
|-------|-----|
| `notifications/resources/update` wrong method name | Corrected to `notifications/resources/updated` |
| `UnsubscribeRequest.params.context` field doesn't exist in MCP schema | Subscription contexts tracked server-side in `Map<uri, context>` |
| `process.on('exit', async)` — await never completes | Replaced with SIGINT/SIGTERM handlers |
| `workspace/configuration` not responded to | Added handler; was blocking gopls workspace loading |
| `window/workDoneProgress/create` response in wrong code path | Moved to server-initiated request handler block |
| `rootPath` sent in `initialize` params | Removed (deprecated; `rootUri` and `workspaceFolders` sent instead) |
| Empty `diagnostics: []` in `codeAction` context | Replaced with overlapping diagnostics filter |
| `MarkupContent.kind` ignored in hover response | `kind` now checked before accessing `value` |

### Response Shape Normalization

| Response | Shapes handled |
|----------|----------------|
| `textDocument/hover` | MarkupContent (`{kind, value}`), MarkedString[] (deprecated), plain string (deprecated) |
| `textDocument/completion` | `CompletionItem[]`, `CompletionList ({isIncomplete, items})` |
| `textDocument/codeAction` | `(Command | CodeAction)[]`; discriminated by checking if `command` field is a bare string |
| `textDocument/documentSymbol` | `DocumentSymbol[]`, `SymbolInformation[]`; three-pass tree reconstruction for SymbolInformation |

---

## Persistent Daemon Mode

Language servers like pyright (Python) and tsserver (TypeScript) need sustained background indexing before `textDocument/references` works on large codebases. agent-lsp solves this with a persistent daemon broker that survives between agent sessions.

**How it works:**

1. `start_lsp` with `language_id="python"` or `"typescript"` auto-detects that a daemon is needed
2. A broker subprocess (`agent-lsp daemon-broker`) spawns, starts pyright, indexes the workspace
3. Agent-lsp connects via Unix socket, queries are proxied to pyright through the broker
4. Subsequent sessions connect to the same warm daemon instantly (no re-indexing)
5. Daemon exits after 30 minutes of inactivity; state lives in `~/.cache/agent-lsp/daemons/`

**Languages that use daemon mode:** Python, TypeScript, JavaScript
**Languages that bypass daemon mode:** Go, Rust, C, C++, Java, Ruby, PHP, and all others (zero overhead; these servers index fast enough to serve references within seconds)

**CLI management:**

```bash
agent-lsp daemon-status                    # List all active daemons
agent-lsp daemon-stop --all                # Stop all daemons
agent-lsp daemon-stop --root-dir=X --language=python  # Stop specific daemon
```

**Validated on:** FastAPI (1,119 Python files, 80K stars): daemon indexes in ~10 seconds, `find_references` returns 1,214 results across 556 files instantly. Without daemon mode, pyright timed out at 5 minutes on every attempt.

---

## Speculative Execution

### Session States

`created` → `mutated` → `evaluating` → `evaluated` → `committed` | `discarded` → `destroyed`
`dirty` (terminal, on revert failure or connection failure during mutation)

### Isolation Model

- Single LSP server handles all sessions; concurrent sessions **serialized** (V1)
- `SerializedExecutor`: per-session `chan struct{}` (not global — `map[string]chan struct{}`); preserves cancellation via `select`
- Baseline immutable at session creation; lazy per-file settle on first `simulate_edit` for that file
- Session-local in-memory document overlays
- No cross-session visibility
- Per-document version counters (monotonically increasing; revert is new version N+1, not rollback)
- `SessionExecutor` interface is upgrade seam for future per-session LSP instances

### Session State Model Fields

```go
type SimulationSession struct {
    ID               string
    Status           SessionStatus
    Client           *lsp.LSPClient
    Edits            []AppliedEdit
    Baselines        map[string]DiagnosticsSnapshot // per-file, lazily populated on first simulate_edit
    Versions         map[string]int                 // per-file document version counter
    Contents         map[string]string              // per-file current in-memory content
    OriginalContents map[string]string              // per-file content at baseline (for Discard)
    Workspace        string
    Language         string
    DirtyErr         error                          // accessible only via DirtyError() when Status==dirty
    mu               sync.Mutex
}
```

### Evaluation Result Shape

```json
{
  "session_id": "a3f2-...",
  "errors_introduced": [{ "line": 42, "col": 5, "message": "...", "severity": "error" }],
  "errors_resolved": [],
  "net_delta": 1,
  "scope": "file",
  "confidence": "high",
  "timeout": false,
  "duration_ms": 412
}
```

**`confidence` values:**
- `"high"` — single-file, diagnostics settled within timeout
- `"partial"` — timed out, returned snapshot may be incomplete
- `"eventual"` — workspace scope, cross-file propagation may be incomplete

**Not shipped:** `affected_symbols` and `edit_risk_score` (planned, never implemented)

**`net_delta` semantics:**
- `0` → safe to apply
- `> 0` → introduces errors
- `< 0` → resolves errors

### Timeout Behavior

| Scope | Default timeout |
|-------|----------------|
| file | 3000ms |
| workspace | 8000ms |

- Configurable via `timeout_ms` parameter
- On timeout: returns current snapshot with `confidence: "partial"`, `timeout: true`
- Revert still executes on timeout — cleanup unconditional

### Cross-File Propagation by Server

| Server | Cross-file reliability | Typical time |
|--------|----------------------|--------------|
| gopls | High | 2–5s |
| tsserver | Good | 1–3s |
| rust-analyzer | High | 2–4s |
| Others | Inconsistent | unknown |

### Diagnostic Diffing

Two diagnostics identical if all match: `range.start`, `range.end`, `message`, `severity`, `source` (optional)
- Diff: introduced (in post, not baseline), resolved (in baseline, not post), unchanged (not returned — reduces noise)
- Complexity: O(n+m) with fingerprint-keyed counter map

### Failure Semantics

| Operation | Failure | Behavior |
|-----------|---------|----------|
| `create_simulation_session` | Server unavailable | Return error; no session created |
| `simulate_edit` | Server rejects `didChange` | Abort; session state unchanged; return error |
| `evaluate_session` timeout | Diagnostics did not settle | Return snapshot with `confidence: "partial"`, `timeout: true`; session remains usable |
| `evaluate_session` connection failure | After mutation | Attempt internal revert; mark session `dirty` if revert fails |
| `commit_session` | Write failure | Return error; session state preserved; retry allowed |
| `discard_session` | Revert failure | Mark session `dirty`; error returned; call `destroy_session` to force cleanup |
| Concurrent mutation detected | During evaluation | Mark result `confidence: "partial"`; session remains usable |

### Session Observability Events

| Event | Fields |
|-------|--------|
| `session.created` | session_id, workspace_root, language |
| `session.edit_applied` | session_id, file, range, version_after |
| `session.evaluation_start` | session_id, edit_count, scope |
| `session.evaluation_complete` | session_id, duration_ms, net_delta, confidence |
| `session.committed` | session_id, files_written, duration_ms |
| `session.discarded` | session_id, edit_count |
| `session.dirty` | session_id, step, error |
| `session.destroyed` | session_id |

Events flow through `logging` package at `LevelDebug` (lifecycle) and `LevelError` (dirty/failure).

### Deferred by Design

| Feature | Upgrade seam |
|---------|-------------|
| Physical isolation (per-session LSP) | Swap `SerializedExecutor` for `IsolatedExecutor` via `SessionExecutor` interface; no API changes |
| Session persistence | `commit_session` returns portable `WorkspaceEdit`; callers persist independently |
| Deterministic workspace evaluation | `confidence: "eventual"` flag; re-validate after commit |

---

## Phase Enforcement (3 tools)

Runtime enforcement of skill phase ordering. Prevents agents from calling tools out of order during multi-step workflows (e.g., `apply_edit` before blast-radius analysis in `/lsp-refactor`).

### Phase Enforcement Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `activate_skill` | Start phase enforcement for a skill workflow | `skill_name` (string, req), `mode` (string, opt: "warn", "block") |
| `deactivate_skill` | Stop phase enforcement | none |
| `get_skill_phase` | Query current phase, allowed/forbidden tools, tool history | none |

### Enforcement Modes

| Mode | Behavior |
|------|----------|
| `warn` | Log violation, allow call to proceed (default) |
| `block` | Return isError with structured recovery guidance |

### Skills with Phase Configs

| Skill | Phases | Key Safety Property |
|-------|--------|---------------------|
| `lsp-rename` | 3: prerequisites, preview, execute | `apply_edit` blocked until preview complete |
| `lsp-refactor` | 5: blast_radius, speculative_preview, apply, build_verification, test_execution | `apply_edit` and `simulate_*` blocked until blast-radius complete |
| `lsp-safe-edit` | 4: setup, speculative_preview, apply, verify_and_fix | `apply_edit` blocked until simulation complete |
| `lsp-verify` | 5: test_correlation, diagnostics, build, tests, fix_and_format | `simulate_*` globally forbidden (verify is post-edit) |

### Phase Advancement

- Automatic: calling a tool from a later phase's allowed list advances to that phase
- Pass-through: tools not in any phase config (e.g., `inspect_symbol`) are always allowed
- Global forbidden: some tools are blocked regardless of phase (skill-specific)
- External tools (Edit, Write, Bash) appear in forbidden lists for agent guidance but cannot be enforced at runtime

### Architecture

- `internal/phase/types.go` — `EnforcementMode`, `PhaseDefinition`, `SkillPhaseConfig`, `PhaseViolation`, `PhaseStatus`
- `internal/phase/matcher.go` — glob matching for tool name patterns (trailing `*` wildcard)
- `internal/phase/tracker.go` — thread-safe `Tracker` state machine (activate, deactivate, check+record, status)
- `internal/phase/skills.go` — built-in phase configs for 4 skills
- `cmd/agent-lsp/tools_phase.go` — MCP tool registration for 3 phase enforcement tools
- `cmd/agent-lsp/server.go` — `addToolWithPhaseCheck[T]` generic wrapper; all tool handlers wrapped automatically

### Audit Trail Integration

Phase events logged to JSONL audit trail: `activate_skill`, `deactivate_skill`, `phase_advance`, `phase_violation`.

See [docs/phase-enforcement.md](./phase-enforcement.md) for the full design document.

---

## Proactive Notifications

Server-initiated MCP notifications that inform the agent about state changes without requiring a tool call. Four channels, each with independent debouncing to avoid flooding the agent during high-activity periods (e.g., initial indexing).

### Notification Channels

| Channel | What it reports | MCP primitive | Debounce |
|---------|----------------|---------------|----------|
| Diagnostic changes | New errors/warnings from the language server, or resolved diagnostics | `notifications/resources/updated` | 2 seconds |
| Workspace ready | Language server indexing complete (all `$/progress` tokens done) | `logging/message` | None (one-shot) |
| Process health | Language server crash or recovery | `logging/message` | None (immediate) |
| Stale references | Watched files changed on disk; cached references may be outdated | `notifications/resources/updated` + `logging/message` | 3 seconds |

### Architecture

- `internal/notify/hub.go`: Central `Hub` coordinator with `NotificationSender` interface, `SetSender`, `Send`, `SendResourceUpdate`, `AddStopFunc`, `Close`. Thread-safe via `sync.RWMutex`.
- `internal/notify/diagnostics.go`: `DiagUpdate` struct, `diagDebouncer` with configurable interval. Coalesces rapid `publishDiagnostics` updates so the agent receives one notification per stabilization window, not hundreds during indexing.
- `internal/notify/workspace.go`: `SubscribeWorkspaceReady` polls `IsWorkspaceLoaded`, emits a JSON notification when indexing completes. 5-minute timeout.
- `internal/notify/health.go`: `SubscribeHealth` polls `IsAlive`, emits crash/recovery notifications on state transitions.
- `internal/notify/stale.go`: `StaleNotifier` with 3-second debounce, emits `ResourceUpdated` + log notification when files change on disk.
- `internal/lsp/client_notify.go`: `SubscribeToFileChanges`, `IsAlive`, `IsWorkspaceLoaded` methods on `LSPClient`.

### Value

- Diagnostic changes: the agent knows "the file I just edited now has 3 errors" immediately, without calling `get_diagnostics`.
- Workspace ready: replaces the current poll/block pattern for indexing completion.
- Process health: the agent learns about a language server crash immediately, instead of discovering it on the next tool call.
- Stale references: signals that cached `blast_radius` / `find_references` results may be outdated after external edits.

### Status

Shipped. Both waves complete: notification infrastructure (`internal/notify/`) and MCP server wiring (`cmd/agent-lsp/notifications.go`). All four channels are wired automatically on `start_lsp`.

---

## Provider-Agnostic Skill Awareness

Four-layer reinforcement architecture ensures agents know about the 24 skills regardless of which AI provider or MCP client is used.

| Layer | Mechanism | Durability |
|-------|-----------|------------|
| Connect-time | `ServerOptions.Instructions` in MCP `initialize` response; condensed skill overview, tool count, key workflows | Decays over long conversations |
| Per-response | Content[1] "Next step:" hint in every tool response | Renewed on every tool call |
| On-demand | `prompts/get("lsp-refactor")` returns full workflow instructions | Loaded when needed |
| Phase enforcement | Error messages with recovery guidance when agent skips steps | Fires on violations |

**Init rules files:** `agent-lsp init` writes a provider-specific rules file alongside the MCP config. Content generated from embedded SKILL.md files at runtime. All files use managed sections (sentinel comments) for idempotent updates.

| Platform | Rules File |
|----------|------------|
| Claude Code (project) | `CLAUDE.md` managed section |
| Claude Code (global) | `~/.claude/CLAUDE.md` managed section |
| Cursor | `.cursor/rules/agent-lsp.mdc` |
| Cline | `.clinerules` |
| Windsurf | `~/.windsurfrules` |
| Gemini CLI | `GEMINI.md` |

---

## Agent Tool Adoption Enforcement

Features that actively guide agents toward using the correct MCP tools instead of defaulting to built-in tools (Grep, Read, Edit).

| Feature | Description |
|---------|-------------|
| **Disallowed reasoning patterns** | Claude Code init rules include a "use this, not that" table (e.g., "find all usages: use `find_references`, not Grep"). Provider-agnostic Instructions use softer "prefer these tools" language. |
| **Task-to-tool mapping table** | 10-entry task-to-tool mapping in the MCP Instructions string. Claude Code rules files include a full comparison table with "Not this" column. |
| **Recovery-oriented error messages** | Symbol resolution errors suggest `list_symbols`. `safe_delete_symbol` with references suggests `find_references` to see callers. `CheckInitialized` suggests `start_lsp`. |
| **Cross-referencing in tool descriptions** | Tools suggest related tools where applicable. `apply_edit` recommends `replace_symbol_body` for full function replacements and `preview_edit` before applying. `find_references` recommends `safe_delete_symbol` for zero-reference symbols and `blast_radius` for blast-radius analysis. `suggest_fixes` points to `/lsp-fix-all` skill. `rename_symbol` recommends `find_references` before renaming exports. |
| **"No verification needed" assertions** | `preview_edit` description states: "If net_delta is 0, the edit is safe to apply without further verification." Reduces unnecessary follow-up tool calls after clean previews. |

---

## Distribution Channels

| Channel | Status | Command/URL |
|---------|--------|-------------|
| GitHub Releases | done (v0.1.0) | https://github.com/blackwell-systems/agent-lsp/releases |
| `curl \| sh` | done (v0.1.1) | `curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh \| sh` |
| Homebrew | done (v0.1.2) | `brew install blackwell-systems/tap/agent-lsp` |
| npm | done (v0.1.2) | `npm install -g @blackwell-systems/agent-lsp` |
| Docker GHCR | done (v0.1.2) | `docker pull ghcr.io/blackwell-systems/agent-lsp:latest` |
| Docker Hub | done (v0.1.2) | `docker pull blackwellsystems/agent-lsp:latest` |
| MCP Registry | done (v0.1.2) | `io.github.blackwell-systems/agent-lsp` — verified at `registry.modelcontextprotocol.io` |
| Smithery/Glama | done (v0.1.2) | auto-indexed via `smithery.yaml` |
| mcpservers.org | done (v0.1.2) | manual listing |
| PulseMCP | done (v0.1.2) | ingests from official registry weekly |
| Windows `install.ps1` | done (v0.2.0) | `irm https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.ps1 \| iex` — installs to `%LOCALAPPDATA%\agent-lsp`, adds to user PATH; no admin required |
| Scoop | done (v0.2.0) | `scoop bucket add blackwell-systems https://github.com/blackwell-systems/agent-lsp && scoop install agent-lsp` — manifest at `bucket/agent-lsp.json` |
| Winget | done (v0.2.0) | `winget install BlackwellSystems.agent-lsp` — manifests at `winget/manifests/` |
| PyPI | done (v0.5.2) | `pip install agent-lsp` — platform-specific wheels, no Go toolchain required |
| `go install` | done | `go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest` |
| Nix flake | planned | `nix run github:blackwell-systems/agent-lsp` |
| Awesome MCP Servers | done (v0.4.0) | Listed in [punkpeye/awesome-mcp-servers](https://github.com/punkpeye/awesome-mcp-servers) |
| VS Code extension | planned | zero-CLI-setup for Copilot/Continue/Cline |

### Licensing

- **MIT LICENSE** — copyright Blackwell Systems and Dayna Blackwell; `LICENSE` file at repo root

### Platforms (GitHub Releases binaries)

| Platform | Architectures |
|----------|--------------|
| macOS | arm64, amd64 |
| Linux | arm64, amd64 |
| Windows | arm64, amd64 |

### npm Packages (7 total)

- `@blackwell-systems/agent-lsp` — root (optionalDependencies pattern; JS shim + platform binary selection)
- `@blackwell-systems/agent-lsp-darwin-arm64`
- `@blackwell-systems/agent-lsp-darwin-x64`
- `@blackwell-systems/agent-lsp-linux-arm64`
- `@blackwell-systems/agent-lsp-linux-x64`
- `@blackwell-systems/agent-lsp-win32-x64`
- `@blackwell-systems/agent-lsp-win32-arm64`

### Release Pipeline

```
git tag v* push
    ↓
release (GoReleaser) → binaries + GitHub Release + Homebrew formula auto-update
    ↓
npm-publish → downloads binaries from GitHub Release, publishes 7 npm packages
    ↓
mcp-registry-publish → publishes metadata to official MCP Registry (GitHub OIDC; no secrets)

GoReleaser (inside release job):
    v* tag → 11 image stanzas pushed to both GHCR + Docker Hub:
    base/latest/semver, go, typescript, python, ruby, cpp, php, web, backend, fullstack, full
    Uses docker/Dockerfile.release (pre-compiled binary from GoReleaser build context)
```

---

## Docker Images

| Tag | Contents | Approx. Size |
|-----|----------|--------------|
| `latest` / `base` | Binary only (same image, two aliases) | ~50 MB |
| `go` | Go + gopls | ~200 MB |
| `typescript` | Node.js + typescript-language-server | ~300 MB |
| `python` | Node.js + pyright-langserver | ~300 MB |
| `ruby` | Ruby + solargraph | ~400 MB |
| `cpp` | clangd | ~150 MB |
| `php` | Node.js + intelephense | ~300 MB |
| `web` | TypeScript + Python | ~400 MB |
| `backend` | Go + Python | ~500 MB |
| `fullstack` | Go + TypeScript + Python | ~600 MB |
| `full` | Go, TypeScript, Python, Ruby, C/C++, PHP | ~1–2 GB |

**Registries:** `ghcr.io/blackwell-systems/agent-lsp` (primary), `blackwellsystems/agent-lsp` (mirror)
**Architecture:** All 11 image tags publish as multi-arch manifest lists (`linux/amd64` + `linux/arm64`). Native performance on Apple Silicon and AWS Graviton without Rosetta/QEMU emulation.
**Tags:** `latest` and `base` are the same image; semver tags (`0.1.2`, `0.1`) also pushed for the base image
**Trigger:** Release tags (`v*`) only
**Build:** `docker/Dockerfile` (base/latest, multi-stage), `docker/Dockerfile.release` (GoReleaser, pre-compiled binary), `docker/Dockerfile.lang` (per-language), `docker/Dockerfile.combo` (web/backend/fullstack), `docker/Dockerfile.full` (full); source-build Dockerfiles use two-stage — Go builder + `debian:bookworm-slim`; static binary; no Go runtime in final image
**Security:** Runs as uid/gid 65532 (`nonroot`); `EXPOSE 8080`; `HOME=/tmp` (writable by nonroot); no root shell; auth token read from `AGENT_LSP_TOKEN` env var (never CLI arg); HTTP server enforces `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout`; entrypoint uses package-manager whitelist (no eval)
**USER root fix:** `Dockerfile.lang`, `Dockerfile.combo`, `Dockerfile.full` switch to `USER root` for package installation, then back to `USER nonroot` before entrypoint
**HEALTHCHECK:** `docker-compose.yml` wires `HEALTHCHECK CMD curl -sf http://localhost:8080/health` for the `agent-lsp-http` service
**Memory limit (docker-compose default):** 4 GB; CPU limit: 2 cores
**Workspace mount:** read-write (code actions may modify files); mount `:ro` for read-only analysis

**docker-compose.yml HTTP service:** `agent-lsp-http` service exposes port `${AGENT_LSP_HTTP_PORT:-8080}:8080` with token read from `AGENT_LSP_TOKEN` env var.

**HTTP mode (docker run):**
```bash
docker run --rm -p 8080:8080 -v /your/project:/workspace \
  -e AGENT_LSP_TOKEN=secret \
  ghcr.io/blackwell-systems/agent-lsp:go \
  --http --port 8080 go:gopls
```

**Languages not in pre-built tags (use `LSP_SERVERS` or custom image):**
Rust, Java, C#, Kotlin, Dart, Scala, Lua, Elixir, Clojure, Zig, Haskell, Swift

**Runtime install via `LSP_SERVERS` env var:**
`gopls`, `typescript-language-server`, `pyright-langserver`, `rust-analyzer`, `clangd`, `solargraph`, `intelephense`, `csharp-ls`, `lua-language-server`, `zls`, `kotlin-language-server`, `metals`, `elixir-ls`, `clojure-lsp`, `haskell-language-server-wrapper`, `sourcekit-lsp`, `jdtls`, `dart`

**Volume caching:** Mount named volume at `/var/cache/lsp-servers` to persist `LSP_SERVERS` installs across container restarts

**MCP client config (docker run):**
```json
{
  "mcpServers": {
    "lsp": {
      "type": "stdio",
      "command": "docker",
      "args": ["run", "--rm", "-i", "-v", "/your/project:/workspace",
               "ghcr.io/blackwell-systems/agent-lsp:go", "go:gopls"]
    }
  }
}
```

---

## Planned Features

### Extensions (language-specific toolchain tools beyond LSP)

**Go — Wave 1 (test + module intelligence)**
- `go.test_run` — run specific test by name, return full output + pass/fail
- `go.test_coverage` — coverage % and uncovered lines for file or package
- `go.benchmark_run` — run benchmark, return ns/op and allocs/op
- `go.test_race` — run with `-race`, return data races found
- `go.mod_graph` — full dependency tree as structured data
- `go.mod_why` — why is this package in go.mod?
- `go.mod_outdated` — list deps with available upgrades
- `go.vulncheck` — `govulncheck` scan — CVEs with affected symbols

**Go — Wave 2 (build + quality)**
- `go.escape_analysis` — `gcflags="-m"` output for a function
- `go.cross_compile` — try cross-compiling for target OS/arch, return errors
- `go.lint` — `staticcheck` or `golangci-lint` output for a file
- `go.deadcode` — find exported symbols with no callers (`go tool deadcode`)
- `go.vet_all` — `go vet ./...` with structured output

**Go — Wave 3 (generation + docs)**
- `go.generate` — run `go generate` on a file, return output
- `go.generate_status` — which `//go:generate` directives are stale
- `go.doc` — `go doc` output for any symbol — richer than hover
- `go.examples` — find `Example*` test functions for a symbol

**TypeScript**
- `typescript.tsconfig_diagnostics` — errors in `tsconfig.json` beyond LSP
- `typescript.type_coverage` — type coverage % for a file (any usage, implicit types)

**Rust**
- `rust.cargo_check` — `cargo check` with structured error output
- `rust.dep_tree` — crate dependency tree (`cargo tree`)
- `rust.clippy` — `cargo clippy` lint output for a file
- `rust.audit` — `cargo audit` CVE scan on `Cargo.lock`

**Python**
- `python.test_run` — run specific `pytest` test by name, return output + pass/fail
- `python.test_coverage` — `coverage.py` branch coverage for file or module
- `python.lint` — `ruff` lint output with structured violations
- `python.type_check` — `mypy` type errors for a file (stricter than pyright)
- `python.audit` — `pip-audit` CVE scan on installed packages
- `python.security` — `bandit` security scan for a file
- `python.deadcode` — `vulture` dead code detection
- `python.imports` — `isort` check — unsorted or missing imports

**C/C++**
- `cpp.tidy` — `clang-tidy` diagnostics for a file
- `cpp.static_analysis` — `cppcheck` output with structured findings
- `cpp.asan_run` — build and run with AddressSanitizer, return memory error output
- `cpp.ubsan_run` — build and run with UndefinedBehaviorSanitizer
- `cpp.valgrind` — `valgrind --memcheck` output for a test binary
- `cpp.symbols` — `nm`/`objdump` symbol table for a compiled object

**Java**
- `java.test_run` — run specific JUnit test, return output
- `java.coverage` — JaCoCo coverage report for a class
- `java.build` — Maven/Gradle build with structured error output
- `java.deps` — `jdeps` dependency analysis
- `java.checkstyle` — Checkstyle violations for a file
- `java.spotbugs` — SpotBugs static analysis findings

**Elixir**
- `elixir.test_run` — run specific ExUnit test, return output
- `elixir.dialyzer` — Dialyzer type analysis
- `elixir.credo` — Credo static analysis findings
- `elixir.audit` — `mix deps.audit` CVE scan

**Ruby**
- `ruby.test_run` — run specific RSpec or Minitest test, return output
- `ruby.lint` — RuboCop violations for a file
- `ruby.security` — Brakeman security scan (Rails)
- `ruby.audit` — `bundle-audit` CVE scan on `Gemfile.lock`

### Skill Schema Specification (planned)

- JSON Schema definitions for each skill's expected inputs and guaranteed outputs — machine-readable contracts alongside prose SKILL.md files
- Schema validation tooling for CI — validates agent skill invocations against schema

### Product (planned)

- **Config file format** — `~/.agent-lsp.json` or `agent-lsp.json` project file for complex setups with per-server options
- **Continue.dev config support** — `agent-lsp init` currently skips Continue.dev (different config format than `mcpServers`)

### Bigger Bets (planned)

- **VS Code extension** — zero-CLI setup for Copilot, Continue, Cline users
- **Observability** — metrics (requests/sec, latency per tool, error rate) for production deployments

---

## Architecture

### Package Structure

**cmd/agent-lsp:**
- `main.go` — CLI entrypoint; argument parsing; signal handling; panic recovery via `runWithRecovery`; `--version` flag; `LOG_LEVEL` env; `--http`/`--port` flags for HTTP+SSE transport
- `version.go` — `var Version = "dev"`; set at build time via `-ldflags="-X main.Version=x.y.z"` by GoReleaser
- `server.go` — MCP server construction; `toolDeps` struct; `mcpSessionSender`; `InitializedHandler` wires logging bridge; `csResolver` wrapper; HTTP server setup with `/health` endpoint
- `doctor.go` — `agent-lsp doctor` subcommand; probes each configured language server, reports version + supported capabilities, exits 1 on failure
- `tools_navigation.go` — 10 navigation tools
- `tools_analysis.go` — 14 analysis tools (includes `detect_changes`)
- `tools_workspace.go` — 21 workspace/lifecycle tools (includes `set_log_level`, `export_cache`, `import_cache`)
- `tools_session.go` — 8 simulation/session tools
- `tools_phase.go` — 3 phase enforcement tools; `checkPhasePermission` helper

**internal/phase:**
- `types.go` — `EnforcementMode`, `PhaseDefinition`, `SkillPhaseConfig`, `PhaseViolation`, `PhaseStatus`
- `matcher.go` — `MatchToolPattern`, `MatchesAny` (glob matching for tool name patterns)
- `tracker.go` — `Tracker` state machine: `ActivateSkill`, `DeactivateSkill`, `CheckAndRecord`, `Status`
- `skills.go` — built-in phase configs for lsp-rename, lsp-refactor, lsp-safe-edit, lsp-verify

**internal/config:**
- `config.go` — `ServerEntry`, `Config` types for multi-server JSON config
- `parse.go` — argument parsing (single-server, multi-server `lang:binary,--arg`, `--config`, auto-detect)
- `infer.go` — `InferWorkspaceRoot`: walks up from file to find `go.mod`/`package.json`/`Cargo.toml`/etc.
- `autodetect.go` — `AutodetectServers`: scans PATH for known language server binaries

**internal/lsp:**
- `client.go` — `LSPClient`: subprocess lifecycle, JSON-RPC framing, request/response correlation, server-initiated requests, file watcher
- `manager.go` — `ServerManager`: multi-server registry, `ClientForFile` routing by extension (linear scan, first match wins, fallback to `entries[0]`)
- `resolver.go` — `ClientResolver` interface: `ClientForFile`, `DefaultClient`, `AllClients`, `Shutdown`
- `framing.go` — Content-Length framing (`FrameReader`/`FrameWriter`)
- `diagnostics.go` — `WaitForDiagnostics`: 500ms stabilization window; empty URIs slice resolves immediately
- `normalize.go` — `NormalizeDocumentSymbols`, `NormalizeCompletion`, `NormalizeCodeActions`

**internal/session:**
- `manager.go` — `SessionManager`: create/apply/evaluate/commit/discard/destroy sessions
- `types.go` — `SimulationSession`, `SessionStatus`, `EvaluationResult`, `ChainResult`; `DirtyError()` accessor
- `executor.go` — `SerializedExecutor`: per-session `chan struct{}` in `map[string]chan struct{}`; `SessionExecutor` interface
- `differ.go` — `DiffDiagnostics`: O(n+m) fingerprint-keyed counter map

**internal/tools (27 files):**
`helpers.go`, `analysis.go`, `navigation.go`, `callhierarchy.go`, `typehierarchy.go`, `inlayhints.go`, `highlights.go`, `semantic_tokens.go`, `capabilities.go`, `detect.go`, `documentation.go`, `symbol_source.go`, `symbol_path.go`, `simulation.go`, `build.go`, `change_impact.go`, `cross_repo.go`, `detect_changes.go`, `cache_artifact.go`, `workspace_folders.go`, `utilities.go`, `fuzzy.go`, `position_pattern.go`, `runner.go`, `workspace.go` (rename_symbol, prepare_rename, format_document, format_range, apply_edit, execute_command), `session.go`, `doc.go`

**internal/resources:**
- `resources.go` — `HandleDiagnosticsResource`, `HandleHoverResource`, `HandleCompletionsResource`; three resource templates
- `subscriptions.go` — `HandleSubscribeDiagnostics`, `HandleUnsubscribeDiagnostics`

**internal/types:**
- `types.go` — 29 shared concrete types: `Position`, `Range`, `Location`, `LSPDiagnostic`, `DocumentSymbol`, `CompletionList`, `CodeAction`, `CallHierarchyItem`, `TypeHierarchyItem`, `InlayHint`, `DocumentHighlight`, `SemanticToken`, `ToolResult`, `Extension` interface

**internal/uri:**
- `uri.go` — `URIToPath` (RFC 3986, `url.Parse`-based, percent-decoded); `ApplyRangeEdit` (shared by lsp + session)

**internal/logging:**
- `logging.go` — `Log`, `SetServer`, `SetLevel`, `SetLevelFromEnv` (called explicitly from `main()`; `init()` is no-op); `MarkServerInitialized`; MCP notification bridge; 8 log levels per MCP spec
- Pre-MCP-session: writes to stderr; post-MCP-session: routes through `logging/message` notifications

**internal/httpauth:**
- `auth.go` — `BearerTokenMiddleware(token string, next http.Handler) http.Handler`; constant-time Bearer token validation via `crypto/subtle.ConstantTimeCompare`; RFC 7235-compliant 401 with `WWW-Authenticate: Bearer` header and `{"error":"unauthorized"}` JSON body; no-op passthrough when token is empty
- `auth_test.go` — unit tests for middleware

**internal/extensions:**
- `registry.go` — `ExtensionRegistry`; `Activate`, `RegisterFactory`, `GetToolHandlers`; registered via `init()` functions at compile time; extensions take precedence over core handlers

**pkg/ (public stable Go API, pkg.go.dev indexed):**
- `pkg/lsp` — type aliases re-exporting `internal/lsp` types (`LSPClient`, `ServerManager`, `ClientResolver`)
- `pkg/session` — type aliases re-exporting `internal/session` types (`SessionManager`, `SessionExecutor`, all speculative execution types)
- `pkg/types` — all 29 type aliases + 5 constants + 2 constructor vars from `internal/types`
- All aliases are `type X = internal.X` — values interchangeable without conversion
- Each package has smoke tests verifying alias targets are non-nil at compile time

**skills/:**
- 24 skill directories; each contains `SKILL.md` with frontmatter + prompt body
- `install.sh` — symlinks/copies skill dirs to `~/.claude/skills/`; maintains CLAUDE.md managed block

### Key Architectural Facts

- **Persistent session:** LSP subprocess stays warm across all requests
- **Multi-server routing:** single process routes by file extension/language ID; `ClientForFile` linear scan, first match wins
- **Auto-init:** `clientForFileWithAutoInit` — if no `start_lsp` called, walks up from file path to find workspace root and starts automatically
- **Auto-watch:** fsnotify, always-on, 150ms debounce; exclusions: `.git`, `node_modules`, `target`, `build`, `dist`, `vendor`, `__pycache__`, `.venv`, `venv`, dot-prefixed dirs; `addWatcherRoot` for `add_workspace_folder` (adds to live watcher, does not restart)
- **`stopWatcher`:** closes stop channel, triggers final flush before goroutine exits; called during `Shutdown` and at start of each `startWatcher` on reinit
- **Speculative execution:** isolated in-memory session layer on top of LSP
- **Serialized concurrency:** sessions logically isolated, physically serialized per-server via per-session `chan struct{}`
- **Progress protocol:** `waitForWorkspaceReady` uses `sync.Cond` (not polling); `handleProgress` broadcasts when `progressTokens` becomes empty; 60s deadline timer goroutine prevents indefinite block
- **Server-initiated requests:** all three types gopls sends handled
- **Normalization layer:** `normalize.go` centralizes polymorphic response handling
- **Fuzzy matching:** workspace symbol lookup with `position_pattern` fallback
- **LineScope:** `line_scope_start`/`line_scope_end` parameters restrict `position_pattern` matching to a line range; eliminates false matches when the same token appears multiple times in a file
- **1-based coordinates:** all line/column inputs 1-indexed; `WithDocument` converts to 0-based for LSP
- **Static binary:** `CGO_ENABLED=0`, no runtime dependency
- **GOWORK stripping:** subprocess environment has `GOWORK` stripped via `removeEnv` to prevent gopls from loading wrong workspace
- **UTF-16 character offsets:** `position_pattern.go` uses `utf16Offset` helper (walks UTF-8 runes, counts surrogate pairs for U+10000+); LSP §3.4 requires UTF-16 code unit offsets
- **`DiffDiagnostics` O(n+m):** fingerprint-keyed counter map; counts handle duplicate diagnostics correctly
- **Panic recovery:** `readLoop` and `startWatcher` goroutines have `defer recover()` — panics logged + stack trace, server stays alive

### Request Lifecycle

```
MCP client → JSON-RPC over stdio
    ↓
server.go: mcp.Server dispatches to registered tool handler
    ↓
clientForFileWithAutoInit(filePath)
    ↓ resolves correct *LSPClient; auto-inits if needed
    ↓
tools.HandleXxx(ctx, client, args)
    ↓
tools.WithDocument[T](ctx, client, filePath, languageID, cb)
    ↓ ValidateFilePath → read file → textDocument/didOpen or didChange → URI
    ↓
client.GetXxx(ctx, fileURI, position)
    ↓ JSON-RPC request with Content-Length framing to LSP subprocess stdin
    ↓ blocks on pendingRequest channel
    ↓
LSP subprocess responds → readLoop() → dispatch() → unblocks pending channel
    ↓ normalize.go handles polymorphic response shapes
    ↓
types.ToolResult{Content: [{type:"text", text: JSON}]}
    ↓
server.go: makeCallToolResult converts to *mcp.CallToolResult
    ↓
MCP client receives JSON-RPC response
```

### Resource Subscription System

| URI Template | Description |
|---|---|
| `lsp-diagnostics:///{filePath}` | Diagnostics for file (or all open files if path empty) |
| `lsp-hover:///{filePath}?line={line}&column={column}&language_id={language_id}` | Hover info at position |
| `lsp-completions:///{filePath}?line={line}&column={column}&language_id={language_id}` | Completions at position |

**Subscription scopes:**
- Specific file: fires only when `updatedURI == fileURI`
- All files: fires for any `updatedURI` starting with `file://`

**Flow:** `resources/subscribe` → `client.SubscribeToDiagnostics(callback)` → LSP publishes `textDocument/publishDiagnostics` → callback fires → `ss.Notify("notifications/resources/updated")` → client reads `resources/read`

### WaitForDiagnostics

Resolves when:
1. All target URIs received ≥1 diagnostic notification *after* initial snapshot
2. No further notifications for 500ms (stabilization window)
3. OR `timeoutMs` exceeded

Empty `targetURIs` slice → resolves immediately.

### Extension System

```go
// Registration at compile time via init()
extensions.RegisterFactory("haskell", func() extensions.Extension {
    return &HaskellExtension{}
})

// Extension interface
type Extension interface {
    ToolHandlers() map[string]ToolHandler
    ResourceHandlers() map[string]ResourceHandler
    SubscriptionHandlers() map[string]ResourceHandler
    PromptHandlers() map[string]any
}
```

- Extensions take precedence over core handlers on name conflicts
- Unused extensions: zero runtime cost (no filesystem scan, no `dlopen`)
- `cmd/agent-lsp/main.go` calls `registry.Activate(languageID)` for each configured server

### Layer Rules

- `cmd/agent-lsp/` owns MCP server lifecycle; routes via five tool registration files
- `internal/tools/` + `internal/resources/` import from `internal/lsp/`, `internal/session/`, `internal/types/` — not from each other
- `internal/lsp/` imports: `internal/types/`, `internal/logging/`, `internal/uri/` — no upward deps
- `internal/session/` imports: `internal/lsp/`, `internal/types/`, `internal/logging/`, `internal/uri/`
- `internal/uri/` imports: `internal/types/` only — canonical URI/path conversion layer
- `internal/extensions/` imports: `internal/types/` only
- `extensions/<language>/` imports from `internal/tools/` for re-exported utilities

---

## CLI

| Command | Purpose |
|---------|---------|
| `agent-lsp <lang:server[,args]...>` | Start MCP server (multi-server mode, stdio) |
| `agent-lsp <lang> <server>` | Start MCP server (legacy single-server mode, stdio) |
| `agent-lsp --config /path/to/agent-lsp.json` | Start MCP server from JSON config |
| `agent-lsp` | Start MCP server with auto-detected language servers |
| `agent-lsp --http [--port N] <lang:server...>` | Start MCP server over HTTP+SSE |
| `agent-lsp doctor` | Probe each configured language server; report version + capabilities; exit 1 on failure |
| `agent-lsp init` | Interactive setup wizard |
| `agent-lsp init --non-interactive` | CI/scripted setup |
| `agent-lsp update` | Self-update to latest GitHub Release; `--check` to compare only, `--force` to update even if current |
| `agent-lsp uninstall` | Clean removal of MCP configs, skill installations, CLAUDE.md sections, caches; `--dry-run` supported |
| `agent-lsp --help` / `-h` / `help` | Print usage summary with all modes and subcommands |
| `agent-lsp --version` | Print version and exit |

**Argument format:** `language:server-binary[,--arg1][,--arg2]`

**HTTP flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--http` | off | Enable HTTP+SSE transport instead of stdio |
| `--port N` | `8080` | TCP port to listen on (1–65535) |
| `AGENT_LSP_TOKEN` (env) | — | Bearer token for auth; empty = unauthenticated (warns on start) |

| `--audit-log PATH` | off | JSONL audit log path (or `AGENT_LSP_AUDIT_LOG` env var) |

Auth token must be set via environment variable — not `--token` flag — to avoid credential exposure in the process list.

**`/health` endpoint:** unauthenticated `GET /health` returns `{"status":"ok"}` (200). Bypasses Bearer token auth so container orchestrators and Docker healthchecks can probe liveness without credentials.

**Auth middleware:** `internal/httpauth.BearerTokenMiddleware(token, next)` — constant-time Bearer token validation via `crypto/subtle.ConstantTimeCompare`; RFC 7235-compliant 401 with `WWW-Authenticate: Bearer` header; no-op passthrough when token is empty.

**Example:** `agent-lsp go:gopls typescript:typescript-language-server,--stdio python:pyright-langserver,--stdio`

**MCP config example:**
```json
{
  "mcpServers": {
    "lsp": {
      "type": "stdio",
      "command": "agent-lsp",
      "args": [
        "go:gopls",
        "typescript:typescript-language-server,--stdio",
        "python:pyright-langserver,--stdio"
      ]
    }
  }
}
```

**Library usage (without MCP server):**
```go
import "github.com/blackwell-systems/agent-lsp/pkg/lsp"

client := lsp.NewLSPClient("gopls", []string{})
client.Initialize(ctx, "/path/to/workspace")
defer client.Shutdown(ctx)

locs, err := client.GetDefinition(ctx, fileURI, lsp.Position{Line: 10, Character: 4})
```

---

## CI

| Job | Languages | Runner | Notes |
|-----|-----------|--------|-------|
| `unit-and-smoke` | (all unit tests) | ubuntu-latest | renamed from `test` |
| `multi-lang-core` | Go, TypeScript, Python, Rust, Java, Kotlin | ubuntu-latest | |
| `multi-lang-extended` | C, C++, JavaScript, PHP, Ruby, YAML, JSON, Dockerfile, C#, CSS, HTML | ubuntu-latest | |
| `multi-lang-zig` | Zig | ubuntu-latest | |
| `multi-lang-terraform` | Terraform | ubuntu-latest | |
| `multi-lang-lua` | Lua | ubuntu-latest | |
| `multi-lang-swift` | Swift | macos-latest | sourcekit-lsp macOS only |
| `multi-lang-scala` | Scala | ubuntu-latest | continue-on-error; 30min timeout |
| `multi-lang-gleam` | Gleam | ubuntu-latest | |
| `multi-lang-elixir` | Elixir | ubuntu-latest | continue-on-error; erlef/setup-beam@v1 (Elixir 1.16/OTP 26) |
| `multi-lang-prisma` | Prisma | ubuntu-latest | continue-on-error |
| `multi-lang-sql` | SQL | ubuntu-latest | postgres:16 service container; pg_isready health check |
| `multi-lang-clojure` | Clojure | ubuntu-latest | |
| `multi-lang-nix` | Nix | ubuntu-latest | DeterminateSystems/nix-installer-action@v16 required |
| `multi-lang-dart` | Dart | ubuntu-latest | |
| `multi-lang-java` | Java | ubuntu-latest | continue-on-error; `-Xmx2G`; 15min timeout; isolated from `multi-lang-core` to avoid OOM |
| `multi-lang-mongodb` | MongoDB | ubuntu-latest | continue-on-error; mongo:7 service container; mongosh health check |
| `speculative-test` | session lifecycle (8 languages: Go, TypeScript, Python, Rust, C++, C#, Dart, Java) | ubuntu-latest | `TestSpeculativeSessions` table-driven in `test/speculative_test.go`; 20min timeout; Java 300s extended timeout for JVM startup |
| `mcp-assert-trajectory` | (skill protocols, all 24 skills) | ubuntu-latest | inline traces, no server needed, 0ms per assertion; total under 60s |
| `mcp-assert` | Go (tool correctness via gopls) | ubuntu-latest | full MCP stdio transport; 120s per assertion; ~2min total |

**Test files:**
- `test/multi_lang_test.go` — `TestMultiLanguage` (1573 lines after extraction)
- `test/lang_configs_test.go` — `buildLanguageConfigs()` (840 lines; extracted from multi_lang_test.go)
- `test/speculative_test.go` — `TestSpeculativeSessions` (table-driven, 8 languages)
- `test/error_paths_test.go` — 11 subtests covering bad input across `go_to_definition`, `get_diagnostics`, `simulate_edit`, `preview_edit`, `find_references`, `rename_symbol`; asserts well-formed error responses, never nil/crashes
- `test/consistency_test.go` — parallel structural shape validation across Go, TypeScript, Python, Rust for `list_symbols`, `go_to_definition`, `get_diagnostics`, `inspect_symbol`; verifies response shape contracts hold across language servers
- `test/fixtures/<lang>/` — per-language fixture files

---

## Audit Trail

- **JSONL logging** for mutating operations: `apply_edit`, `rename_symbol`, `commit_session`
- **Configurable:** `--audit-log /path/to/file.jsonl` flag or `AGENT_LSP_AUDIT_LOG` env var
- **Non-blocking:** channel-buffered writer; tool handlers never block on I/O
- **Record fields:** timestamp, files touched, edit summary, pre/post diagnostic state, `net_delta`
- **Implementation:** `internal/audit/audit.go`

---

---

## Protocol-Level Testing (mcp-assert)

agent-lsp is tested through the MCP protocol layer using [mcp-assert](https://github.com/blackwell-systems/mcp-assert), a deterministic correctness testing framework for MCP servers. No LLM-as-judge; all grading is assertion-based.

**Two CI jobs run mcp-assert on every push and PR:**

**`mcp-assert-trajectory`** — validates that all 24 skills follow correct tool call sequences. Uses inline traces embedded in YAML files; no live language server needed. Each assertion completes in 0ms. Total job runtime under 60 seconds. Assertion files: `examples/mcp-assert/trajectory/` (24 files, one per skill). Trajectory assertions check `presence` (required tools appear), `absence` (forbidden tools do not appear), `order` (correct sequence), and `args_contain` (specific argument values).

**`mcp-assert`** — tests tool correctness through the full MCP stdio transport against real gopls. Assertion files: `examples/mcp-assert/go/*.yaml`. 120s per-assertion timeout; total runtime ~2 minutes.

**What mcp-assert tests that Go integration tests don't:**
- MCP JSON-RPC serialization/deserialization
- JSON Schema parameter validation
- MCP protocol negotiation (initialize/initialized handshake)
- Tool response format through the transport layer
- The exact path agents use in production
- Skill protocol compliance: correct ordering, presence, and absence of tool calls

---

**Speculative test coverage:**
- `discard_path` — applies edit via `simulate_edit`, discards session
- `evaluate_session` — asserts `net_delta == 0` for comment-only edits
- `simulate_chain` — asserts `cumulative_delta == 0` and `safe_to_apply_through_step == 2`
- `commit_path` — applies comment edit before committing
- `preview_edit_standalone` — asserts `net_delta == 0` for comment edit
- `error_detection` — applies `return 42` in `func ... string` body; asserts `net_delta > 0` and `errors_introduced` non-empty

---

## Agent Self-Evaluations

Four independent AI agents evaluated agent-lsp across 10 coding tasks (find callers, rename safely, preview edits, detect dead code). Each model wrote an honest assessment comparing LSP tools against grep/read equivalents. Results documented in [docs/agent-evaluations.md](./agent-evaluations.md).

| Model | Verdict | Top-rated tools |
|-------|---------|----------------|
| Claude (Opus 4.6) | Recommend | `blast_radius`, `go_to_implementation`, simulation sessions |
| Cursor (auto) | Recommend | rename, references, implementations, simulation |
| GPT-5.5 (Codex) | Recommend | references, implementations, rename previews, diagnostics |
| Gemini 2.5 Pro | Highly recommend | rename, implementations, diagnostic preview |
