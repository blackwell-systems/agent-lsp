# agent-lsp Code Quality Audit — Report 5

**Date:** 2026-04-09
**Auditor:** Inspector Agent (claude-sonnet-4-6)
**Areas:** `internal/tools`, `internal/lsp`, `internal/session`, `cmd/agent-lsp`
**LSP status:** gopls started successfully; `mcp__lsp__find_references` returned "no package metadata" for all internal packages during this run (cross-session LSP state conflict). Symbol-level dead-code findings use Grep fallback and are annotated `[LSP unavailable — Grep fallback, reduced confidence]`.

---

## Layer Map

```
cmd/agent-lsp        → entry point, MCP server registration, signal handling
internal/lsp         → LSP subprocess client, manager/resolver, diagnostics wait
internal/session     → simulation session lifecycle (create/edit/evaluate/commit/discard)
internal/tools       → MCP tool handler functions (thin dispatch layer)
internal/config      → config parsing, workspace inference
internal/types       → shared types (Position, Range, Location, SymbolInformation, …)
internal/extensions  → per-language extension registry
internal/logging     → structured log dispatch
internal/resources   → MCP resource handlers (diagnostics subscription)

Boundaries:
  internal/tools    may import internal/lsp, internal/session, internal/types
  internal/session  may import internal/lsp, internal/types
  internal/lsp      must NOT import internal/tools or internal/session
  cmd/agent-lsp     may import all internal/* packages
```

No layer violations were observed.

---

## Findings

### CRITICAL

#### C1 — `Restart` does not clear `openDocs`: wrong LSP message sent after restart

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, lines 733–752
**Check:** `silent_failure` / `bug`

`Restart` resets `pending`, `progressTokens`, `capabilities`, and `initialized`, but does **not** reset `openDocs`. After restart the spawned LSP server process is brand-new and has no open documents. On the first `OpenDocument` call for any previously-opened URI, `OpenDocument` finds `alreadyOpen == true` in the stale map and sends `textDocument/didChange` instead of `textDocument/didOpen`. The fresh server has never received a `didOpen` for that URI, so it rejects or ignores the `didChange`. The document is then invisible to the server.

`diags` is similarly not cleared — stale diagnostic entries from the previous server session are served to callers until overwritten.

`legendTypes` / `legendModifiers` are also not cleared — if the new server exposes a different semantic token legend the stale values are served.

```go
// Restart (client.go:733–752) — missing resets:
c.mu.Lock()
c.openDocs = make(map[string]docMeta)   // absent
c.mu.Unlock()
c.diagMu.Lock()
c.diags = make(map[string][]types.LSPDiagnostic) // absent
c.diagMu.Unlock()
c.legendMu.Lock()
c.legendTypes = nil   // absent
c.legendModifiers = nil // absent
c.legendMu.Unlock()
```

**Confidence:** High (structural read).

---

#### C2 — `startWatcher` / `stopWatcher` access `watcherStop` without a lock: data race

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, lines 1946–1950, 2040–2044
**Check:** `silent_failure` / data race

`startWatcher` reads and writes `c.watcherStop` unguarded. `stopWatcher` reads and writes it unguarded. Both are called from different call sites that hold no shared lock:

- `Initialize` → `startWatcher` (after releasing `c.mu`)
- `Shutdown` → `stopWatcher` (no lock)
- `AddWorkspaceFolder` → `startWatcher` (no lock)
- `Restart` → `Shutdown` → `stopWatcher`, then `Initialize` → `startWatcher`

Concurrent calls to `Shutdown` and `AddWorkspaceFolder` (which both happen in a live server under signal-handling goroutines and MCP tool handlers running concurrently) produce an unsynchronised write to `c.watcherStop`. Under `go test -race` this will be flagged.

**Confidence:** High (structural read, no lock guards `watcherStop` in any path).

---

### HIGH

#### H1 — `applyDocumentChanges` silently swallows `create`, `rename`, `delete` filesystem errors

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, lines 1615–1643
**Check:** `silent_failure`

For `create`, `rename`, and `delete` operations in `workspace/applyEdit`, all filesystem errors are discarded with `_ =`:

```go
case "create":
    _ = os.WriteFile(path, []byte{}, 0644)
case "rename":
    _ = os.Rename(uriToPath(op.OldURI), uriToPath(op.NewURI))
case "delete":
    _ = os.Remove(uriToPath(op.URI))
```

The caller (`ApplyWorkspaceEdit`) receives no error. When a language server sends a `workspace/applyEdit` request for a rename, the LSP client responds `{"applied": true}` while the actual filesystem operation may have failed. The workspace on disk diverges from what the server believes.

Only `TextDocumentEdit` errors are propagated (line 1649–1651). The two code paths within the same function are inconsistent.

**Confidence:** High (direct code read).

---

#### H2 — `AddWorkspaceFolder` calls `startWatcher(c.rootDir)` not `startWatcher(path)`: added folders outside `rootDir` are not watched

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, line 1863
**Check:** `bug` / `silent_failure`

`startWatcher` calls `filepath.WalkDir(rootDir, …)` where `rootDir` is the original workspace root. When a second repository is added via `add_workspace_folder`, the watcher is restarted but only walks the original root. Files changed in the newly-added folder generate no `didChangeWatchedFiles` notifications. The LSP index for the second repo goes stale silently after any file edit.

```go
// AddWorkspaceFolder (client.go:1863):
c.startWatcher(c.rootDir)   // should walk `path` too, or c.workspaceFolders
```

The comment says "Extend the auto-watcher to cover the new folder" but the code does not.

**Confidence:** High (direct code read, `filepath.WalkDir` starts at `rootDir` only).

---

#### H3 — `HandleSimulateEditAtomic` silently swallows `Discard` errors; LSP state may remain modified

**File:** `/path/to/agent-lsp/internal/tools/simulation.go`, lines 329, 335
**Check:** `silent_failure`

`HandleSimulateEditAtomic` calls `_ = mgr.Discard(ctx, sessionID)` in both the error path (after Evaluate fails) and the success path. `Discard` may itself fail — for example if the context is cancelled or if `OpenDocument` fails during revert. When `Discard` fails, the LSP server retains the in-memory modified document state. Subsequent calls to `find_references`, `get_diagnostics`, etc. then operate on the mutated buffer. The caller receives no indication that LSP state was not reverted.

The `defer mgr.Destroy(ctx, sessionID)` does not revert LSP document content (noted in the comment), so the Discard call is the only cleanup path.

**Confidence:** High (direct code read).

---

#### H4 — `mcpSessionSender.LogMessage` uses `context.Background()` ignoring shutdown signal

**File:** `/path/to/agent-lsp/cmd/agent-lsp/server.go`, line 28
**Check:** `context_propagation`

```go
func (s *mcpSessionSender) LogMessage(level, logger, message string) error {
    data, _ := json.Marshal(message)
    return s.ss.Log(context.Background(), &mcp.LoggingMessageParams{…})
}
```

`LogMessage` receives no context. `context.Background()` is used unconditionally. If the main `ctx` is cancelled (e.g., SIGTERM received), in-flight log sends still proceed against a shutting-down session. This is low severity in isolation but is a pattern to flag: a fresh context detached from the server's lifecycle means log sends during shutdown cannot be cancelled.

The `json.Marshal(message)` error is also discarded (`data, _ := json.Marshal(message)`). A non-marshallable message produces a nil `data`, which becomes a JSON `null` in the log notification rather than an error or warning.

**Confidence:** High (direct code read).

---

### MEDIUM

#### M1 — `applyDocumentChanges` returns `nil` when `dc` fails to unmarshal as an array: invalid edits silently ignored

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, line 1603–1604
**Check:** `silent_failure`

```go
if err := json.Unmarshal(b, &raw); err != nil {
    return nil // not an array; ignore
}
```

`documentChanges` that is not a JSON array (e.g., a server bug, or a scalar value) causes the entire `documentChanges` branch to be skipped with no logging and no error returned. The workspace edit is applied with zero changes, and `ApplyWorkspaceEdit` returns success to both the server (via `workspace/applyEdit` response) and to the tool caller.

**Confidence:** High (direct code read).

---

#### M2 — Shutdown errors ignored in `StartAll` rollback path via `context.Background()`

**File:** `/path/to/agent-lsp/internal/lsp/manager.go`, lines 91–95, 102–106
**Check:** `error_wrapping` / context_propagation

During `StartAll`, if one server fails to initialize, previously-started servers are shut down for cleanup. These shutdown calls use `context.Background()` rather than the caller's `ctx`. If the caller context is already cancelled (e.g., a 30-second startup deadline), the shutdown calls proceed on a detached context and ignore errors with `_ = c.Shutdown(context.Background())`. The error is not reported or logged. The caller only sees the initialize error for the failing server; prior servers may be left in a partially-initialized state.

**Confidence:** High (direct code read).

---

#### M3 — Duplicate `uriToPath` implementation across `internal/lsp` and `internal/session`

**File:** `/path/to/agent-lsp/internal/session/manager.go` line 487; `/path/to/agent-lsp/internal/lsp/client.go` line 2239
**Check:** `duplicate_semantics`

Both packages define an identical `uriToPath` unexported function with identical logic (url.Parse → fallback strip). The implementations are currently in sync, but any future change to URI handling must be made in two places. The `session` package already imports `internal/lsp`; it could call `lsp.URIToFilePath` (the exported version in `internal/tools/helpers.go`) instead, but that would add a dependency on `internal/tools` which violates the layer boundary. The appropriate fix is to move the canonical implementation into `internal/types` or a new `internal/uri` package.

**Confidence:** High [LSP unavailable — Grep fallback, reduced confidence for cross-package call verification].

---

#### M4 — `HandleRestartLspServer` operates on a single pre-fetched client, misses multi-server mode

**File:** `/path/to/agent-lsp/internal/tools/session.go`, lines 42–54
**Check:** `scope_analysis`

`HandleRestartLspServer` receives a single `*lsp.LSPClient` and restarts only that one client. In multi-server mode (`ServerManager` with multiple entries), calling `restart_lsp_server` restarts only the default client (whichever `cs.get()` returns), not all configured servers. A caller who runs multi-server Go + TypeScript and invokes `restart_lsp_server` would expect both servers to restart, but only one does. The function has no indication in its error return or result text that other servers were unaffected.

**Confidence:** High (structural read of server.go:470 — `cs.get()` is passed directly).

---

#### M5 — `WaitForDiagnostics` timer-based polling at 50ms is susceptible to missed quiet-window

**File:** `/path/to/agent-lsp/internal/lsp/diagnostics.go`, lines 73–93
**Check:** `scope_analysis` / latent timing bug

The quiet-window check (`time.Since(lastEvent) >= quietWindow`) is evaluated only on 50ms ticker ticks, not immediately on notification receipt. A diagnostic notification arriving 499ms after the last one will be seen as "quiet" on the next tick (up to 50ms later), causing `WaitForDiagnostics` to return up to 50ms before the window is truly elapsed. For the 500ms quiet window used in baseline establishment and evaluation, this is a ~10% timing error that could cause premature evaluation against incomplete diagnostics.

This is a pre-existing tradeoff (the notify channel exists to wake the ticker early), but the quiet-window measurement is done in the ticker arm only, not in the notify arm. A diagnostic arriving just before the ticker fires could cause the ticker to report "quiet" while a second diagnostic is still pending.

**Confidence:** Medium (logic analysis, timing-dependent).

---

### LOW

#### L1 — `runWithRecovery` panic recovery does not set a non-zero exit code

**File:** `/path/to/agent-lsp/cmd/agent-lsp/main.go`, lines 98–108
**Check:** `panic_not_recovered` (partial)

The panic is recovered and logged, but `runWithRecovery` returns `nil` (the named return `runErr` is never set in the `recover()` block). The caller at line 91–94 checks `if err != nil` to exit with code 1. After a panic the process returns exit code 0, which signals success to process supervisors (systemd, launchd, Docker). A restart policy that triggers on non-zero exit codes will not restart the process after a panic.

```go
defer func() {
    if r := recover(); r != nil {
        logging.Log(logging.LevelError, fmt.Sprintf("panic recovered: %v", r))
        // runErr is never set — caller sees nil and exits 0
        shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
        defer cancel()
        _ = resolver.Shutdown(shutdownCtx)
    }
}()
```

**Confidence:** High (direct code read).

---

#### L2 — `ValidateFilePath` does not validate that the path actually exists; path traversal via symlink

**File:** `/path/to/agent-lsp/internal/tools/helpers.go`, lines 18–33
**Check:** `scope_analysis`

`ValidateFilePath` resolves the path with `filepath.Abs(filepath.Clean(filePath))` and checks that it has the `rootDir` prefix. This check operates on the lexical path. A symlink inside the workspace pointing to a file outside it (e.g., `/workspace/link -> /etc/passwd`) passes the prefix check. The function does not call `os.Stat` or `filepath.EvalSymlinks` to resolve the real path. This is a low-severity concern for a local tool that runs only under the user's own account, but it is worth noting.

**Confidence:** High (direct code read).

---

#### L3 — `IsDocumentOpen` used only in tests; exported symbol with no production call site

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, line 806
**Check:** `dead_symbol` [LSP unavailable — Grep fallback, reduced confidence]

`IsDocumentOpen` is exported but called only in `client_test.go` (lines 185, 203). No production code in `internal/tools`, `internal/session`, or `cmd/agent-lsp` calls it. If it exists solely for test use, it should be unexported or replaced with a test-only helper. Because gopls references were unavailable, this is Grep-based.

[LSP unavailable — Grep fallback, reduced confidence]

---

#### L4 — `toolArgsToMap` discards `json.Unmarshal` error silently

**File:** `/path/to/agent-lsp/cmd/agent-lsp/server.go`, lines 76–84
**Check:** `silent_failure`

```go
func toolArgsToMap(v interface{}) map[string]interface{} {
    data, err := json.Marshal(v)
    if err != nil {
        return map[string]interface{}{}
    }
    m := map[string]interface{}{}
    _ = json.Unmarshal(data, &m)  // error discarded
    return m
}
```

If `json.Unmarshal` fails (e.g., the marshalled JSON is not a JSON object — only possible if the typed struct marshal produced a non-object, which cannot happen in practice), the function returns an empty map. Tool handlers then receive an empty args map and return "field X is required" errors. This is low risk given the input is always a marshalled struct, but the pattern is inconsistent with the marshal-error guard just above.

**Confidence:** High (direct code read).

---

#### L5 — `applyRangeEdit` in `session/manager.go` and `applyEditsToFile` in `lsp/client.go` share identical line-splice logic with no shared test path

**File:** `/path/to/agent-lsp/internal/session/manager.go` line 432 comment; `/path/to/agent-lsp/internal/lsp/client.go` line 1699
**Check:** `duplicate_semantics`

A code comment on `applyRangeEdit` says "SYNC: if applyEditsToFile changes its line-splice logic, update this function too." This is an acknowledged manual-sync requirement between two independent implementations of the same algorithm. Any divergence in edge-case handling (e.g., empty-file handling, multi-byte characters in character offsets) will produce different results from the session simulation path versus the workspace-edit apply path.

**Confidence:** High (direct code read, comment is explicit about the coupling).

---

## Summary Table

| ID  | Severity | Check             | Location                                              | One-line description                                                  |
|-----|----------|-------------------|-------------------------------------------------------|-----------------------------------------------------------------------|
| C1  | CRITICAL | bug               | `internal/lsp/client.go:733`                         | `Restart` does not clear `openDocs`/`diags`/legend; fresh server gets `didChange` before `didOpen` |
| C2  | CRITICAL | data_race         | `internal/lsp/client.go:1946,2040`                   | `watcherStop` read/write without lock; race between `Shutdown` and `AddWorkspaceFolder` |
| H1  | HIGH     | silent_failure    | `internal/lsp/client.go:1619,1631,1638`              | `create`/`rename`/`delete` filesystem errors swallowed in `applyDocumentChanges` |
| H2  | HIGH     | bug               | `internal/lsp/client.go:1863`                        | `AddWorkspaceFolder` calls `startWatcher(rootDir)` not the new path; added repos not watched |
| H3  | HIGH     | silent_failure    | `internal/tools/simulation.go:329,335`               | `Discard` errors silently dropped in `HandleSimulateEditAtomic`; LSP may stay dirty |
| H4  | HIGH     | context_propagation | `cmd/agent-lsp/server.go:28`                       | `LogMessage` uses `context.Background()`; marshal error discarded with `_` |
| M1  | MEDIUM   | silent_failure    | `internal/lsp/client.go:1603`                        | `applyDocumentChanges` returns `nil` on array-unmarshal failure; edits silently dropped |
| M2  | MEDIUM   | context_propagation | `internal/lsp/manager.go:92,104`                  | `StartAll` rollback uses `context.Background()` for shutdown; errors discarded |
| M3  | MEDIUM   | duplicate_semantics | `internal/session/manager.go:487`, `internal/lsp/client.go:2239` | `uriToPath` duplicated across two packages |
| M4  | MEDIUM   | scope_analysis    | `internal/tools/session.go:42`                       | `HandleRestartLspServer` restarts only `cs.get()` in multi-server mode |
| M5  | MEDIUM   | scope_analysis    | `internal/lsp/diagnostics.go:73`                     | Quiet-window checked only on 50ms ticks, not on notification receipt; up to 50ms premature return |
| L1  | LOW      | panic_not_recovered | `cmd/agent-lsp/main.go:98`                         | Recovered panic returns `runErr=nil`; process exits 0 after panic |
| L2  | LOW      | scope_analysis    | `internal/tools/helpers.go:18`                       | `ValidateFilePath` does not resolve symlinks; in-workspace symlinks to out-of-workspace targets pass |
| L3  | LOW      | dead_symbol       | `internal/lsp/client.go:806`                         | `IsDocumentOpen` exported but called only in tests [Grep fallback, reduced confidence] |
| L4  | LOW      | silent_failure    | `cmd/agent-lsp/server.go:82`                         | `toolArgsToMap` discards `json.Unmarshal` error |
| L5  | LOW      | duplicate_semantics | `internal/session/manager.go:432`, `internal/lsp/client.go:1699` | Line-splice algorithm duplicated; comment-documented manual sync required |

---

## LSP Verification Status

| Finding | Required LSP call | Result |
|---------|-------------------|--------|
| C1      | Structural read   | N/A    |
| C2      | Structural read   | N/A    |
| L3      | `mcp__lsp__find_references` on `IsDocumentOpen` | Failed — "no package metadata"; Grep fallback used |
| All others | Structural / Grep | See annotations |

gopls was unable to serve `find_references` for `internal/lsp` or `internal/tools` packages during this session. The MCP LSP server appears to hold a workspace binding from a prior session pointing at a different module. Structural reads and Grep were used for all symbol-level checks with reduced confidence where noted.
