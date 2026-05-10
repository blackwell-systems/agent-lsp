# agent-lsp Code Quality Audit — Report 6

**Date:** 2026-04-09
**Auditor:** Inspector Agent (claude-sonnet-4-6)
**Areas:** `internal/tools`, `internal/lsp`, `internal/session`, `cmd/agent-lsp`, `internal/uri`, `internal/logging`, `internal/resources`
**LSP status:** gopls started but returned "no package metadata" for all `find_references` calls (go.work conflict from parent workspace). Symbol-level dead-code checks use Grep fallback annotated accordingly. All structural findings are direct code reads.

---

## Regression Note: Prior Findings

All 15 findings from audit #5 were verified against the current codebase:

| Audit #5 ID | Status |
|-------------|--------|
| C1 — `Restart` stale `openDocs` | **Fixed** — `Restart` now clears `openDocs`, `diags`, `legendTypes`, `legendModifiers` |
| C2 — `watcherStop` data race | **Fixed** — `watcherMu sync.Mutex` guards all `watcherStop` access |
| H1 — `applyDocumentChanges` swallows fs errors | **Fixed** — `create`/`rename`/`delete` errors now returned |
| H2 — `AddWorkspaceFolder` watches wrong path | **Partially fixed** — passes `path` now, but introduces new bug (see C1 below) |
| H3 — `Discard` errors dropped in `HandleSimulateEditAtomic` | **Fixed** — error surfaced to caller |
| H4 — `LogMessage` marshal error discarded | **Fixed** — marshal failure encoded as error text |
| M1 — `applyDocumentChanges` ignores array-unmarshal failure | **Fixed** — now returns error |
| M2 — `StartAll` rollback uses `context.Background()` | **Fixed** — errors logged with caller ctx |
| M3 — `uriToPath` duplicated | **Fixed** — moved to `internal/uri.URIToPath` |
| M4 — `HandleRestartLspServer` single-client in multi-server | **Acknowledged** — comment added to result text |
| M5 — quiet-window checked only on 50ms tick | **Fixed** — also checked on notify channel receive |
| L1 — recovered panic exits 0 | **Fixed** — `runErr` set in recover block |
| L2 — `ValidateFilePath` doesn't resolve symlinks | **Fixed** — `filepath.EvalSymlinks` added |
| L3 — `IsDocumentOpen` exported but test-only | **Fixed** — renamed to `isDocumentOpen` (unexported) |
| L4 — `toolArgsToMap` discards unmarshal error | **Fixed** — error logged before returning empty map |
| L5 — line-splice algorithm duplicated | **Fixed** — canonical `internal/uri.ApplyRangeEdit` shared |

---

## Layer Map

```
cmd/agent-lsp        → entry point, MCP server registration, signal handling
internal/lsp         → LSP subprocess client, manager/resolver, diagnostics wait
internal/session     → simulation session lifecycle (create/edit/evaluate/commit/discard)
internal/tools       → MCP tool handler functions (thin dispatch layer)
internal/config      → config parsing, workspace inference
internal/types       → shared types (Position, Range, Location, ...)
internal/extensions  → per-language extension registry
internal/logging     → structured log dispatch
internal/resources   → MCP resource handlers (diagnostics subscription)
internal/uri         → shared URI and range-edit utilities

Boundaries:
  internal/tools    may import internal/lsp, internal/session, internal/types, internal/uri
  internal/session  may import internal/lsp, internal/types, internal/uri
  internal/lsp      must NOT import internal/tools or internal/session
  cmd/agent-lsp     may import all internal/* packages
```

No layer violations were observed.

---

## Findings

### CRITICAL

#### C1 — `AddWorkspaceFolder` replaces the rootDir watcher with the new-folder watcher: original workspace goes dark

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, line 1848
**Check:** `bug` / `silent_failure`

The H2 fix from audit #5 changed `startWatcher(c.rootDir)` to `startWatcher(path)` to ensure the added folder is watched. However, `startWatcher` always calls `stopWatcherLocked()` first, which closes the channel of the currently-running goroutine watching `c.rootDir`. The result: after `AddWorkspaceFolder("/new/repo")`, the original workspace root stops receiving `didChangeWatchedFiles` notifications. Edits to files under `c.rootDir` are no longer propagated to the LSP server. The LSP index for the original workspace goes stale silently.

`startWatcher` is designed to watch a single root (it calls `filepath.WalkDir(rootDir, ...)` over exactly one tree). Making it support multiple roots requires either accumulating all roots and walking each, or using the `watcher.Add` API for each new root within the existing goroutine.

```go
// AddWorkspaceFolder (client.go:1846–1848) — current:
c.startWatcher(path)   // stops rootDir watcher; only path is watched after this

// Required behavior: both c.rootDir AND path must be watched
```

**Confidence:** High (direct code read, structural analysis of `startWatcher`).

---

#### C2 — `initialized` not cleared on unplanned LSP process exit: `CheckInitialized` passes on a dead server

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, lines 193–203
**Check:** `bug` / `silent_failure`

The exit-monitor goroutine in `start()` calls `rejectPending(exitErr)` to unblock waiting requests, but does **not** set `c.initialized = false`. After an unplanned LSP subprocess crash (OOM, segfault, etc.):

1. `IsInitialized()` still returns `true`
2. `CheckInitialized(client)` in every tool handler passes without error
3. Tool handlers proceed to call `SendRequest` / `sendNotification`
4. `writeRaw` finds `c.stdin` is closed (or nil after the crash detection race) and returns `"LSP client not started"`
5. The tool handler returns an internal RPC-style error rather than the clear `"LSP not initialized; call start_lsp first"` message

Callers receive opaque errors and cannot distinguish a crashed server from a transient RPC timeout. The fix requires adding `c.mu.Lock(); c.initialized = false; c.mu.Unlock()` inside the exit monitor goroutine, alongside the existing `rejectPending` call.

**Confidence:** High (direct code read; `Shutdown()` at line 718 does not set initialized false either, but that is intentional since Restart re-initializes).

---

### HIGH

#### H1 — `NormalizeDocumentSymbols` name map uses last-write-wins for duplicate names: child wiring silently wrong

**File:** `/path/to/agent-lsp/internal/lsp/normalize.go`, line 61
**Check:** `silent_failure` / `scope_analysis`

When normalizing `SymbolInformation[]` responses (Pass 1), the name map is keyed by `info.Name`:

```go
nameMap[info.Name] = ds // last write wins on duplicates
```

In any real codebase, multiple symbols share names across types (e.g., multiple structs each have a method named `String()`, or multiple `Error()` methods). All but the last `String()` are overwritten. In Pass 2, when a child's `ContainerName == "SomeType"` is looked up, only the last-written `SomeType` node is found. Children are attached to the wrong parent. The resulting symbol tree is silently incorrect for any file with overloaded method names.

This is structural: the name map must be keyed by a compound key such as `containerName + "/" + name` or the full qualified path, not the bare name.

**Confidence:** High (direct code read, comment "last write wins on duplicates" acknowledges the limitation).

---

#### H2 — `SerializedExecutor` serializes all sessions globally: independent sessions block each other

**File:** `/path/to/agent-lsp/internal/session/executor.go`, lines 12–35
**Check:** `scope_analysis`

`SessionManager` holds a single `SerializedExecutor` shared across all sessions. The `sem chan struct{}` is a capacity-1 channel. When session A is in `Evaluate` (waiting 3–8 seconds for diagnostics to stabilize), any concurrent call to session B's `ApplyEdit` or `Evaluate` blocks on `e.sem <- struct{}{}` until A releases.

The architecture doc describes this as serializing "concurrent LSP access within a session", but the implementation serializes across all sessions. Two callers using separate simulation sessions (e.g., one evaluating a refactoring while another runs a chain edit) are forced sequential.

The `Acquire` call takes a `*SimulationSession` parameter (for future per-session locking) but the current body ignores it:

```go
func (e *SerializedExecutor) Acquire(ctx context.Context, s *SimulationSession) error {
    select {
    case e.sem <- struct{}{}:   // single global semaphore, s is unused
        return nil
    ...
}
```

Per-session locking can be achieved by adding a `sync.Mutex` to `SimulationSession` and having `Acquire` lock `s.mu` instead of the global channel.

**Confidence:** High (direct code read).

---

#### H3 — `ResolvePositionPattern` uses byte column offsets, not UTF-16 code unit offsets: wrong positions on non-ASCII files

**File:** `/path/to/agent-lsp/internal/tools/position_pattern.go`, lines 40–49
**Check:** `bug`

LSP positions use UTF-16 code unit offsets for the `character` field (LSP spec §3.4). `ResolvePositionPattern` computes `col` as:

```go
lastNL := strings.LastIndex(fileContent[:offset], "\n")
if lastNL < 0 {
    col = offset + 1          // byte offset from file start
} else {
    col = offset - lastNL     // byte offset from last newline
}
```

`offset` is a byte offset in a UTF-8 string. For a line containing characters outside the Basic Multilingual Plane (e.g., emoji `U+1F600` encodes to 4 UTF-8 bytes but 2 UTF-16 code units), the computed `col` will exceed the correct UTF-16 column. gopls/rust-analyzer interpret this as a position past the line end, returning empty results with no error.

The same issue affects `textMatchApply` in `workspace.go` lines 323–325, which computes `startChar` and `endChar` from byte offsets.

**Confidence:** High (specification-based analysis; identical to how gopls counts columns per LSP spec §3.4).

---

### MEDIUM

#### M1 — `start()` exit-monitor goroutine: `cmd.Wait()` called on process started from `exec.Command`; stderr drain goroutine may race `cmd.Wait`

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, lines 176–203
**Check:** `scope_analysis` / potential data race

`cmd.StderrPipe()` returns a pipe whose read end is passed to `drainStderr`. `cmd.Wait()` in the exit monitor goroutine closes all `cmd.StdoutPipe()` and `cmd.StderrPipe()` connections internally (per `os/exec` contract). This can cause the `drainStderr` goroutine's `r.Read(buf)` to receive an error simultaneously with `Wait()` completing, which is the expected behavior.

However: `drainStderr` appends to `c.stderrBuf` under `stderrMu`, and the exit-monitor goroutine reads `c.stderrBuf` also under `stderrMu` to log the error message. No race here. But `c.stdin` is set to `nil` inside `Shutdown()` under `c.mu` (line 726–729), while the exit monitor goroutine runs without holding any lock. If `Shutdown()` is racing with the exit monitor (e.g., Ctrl+C while the subprocess is crashing), `stdin.Close()` in Shutdown and the nil assignment can interleave with `writeRaw` which reads `c.stdin` under `c.mu`. This is safe since both `writeRaw` and `Shutdown` hold `c.mu`. The subprocess exit and pipe cleanup do not race through `c.stdin`. Noting for completeness: no actionable race found here.

**Lowering to MEDIUM for documentation only — no bug.**

Actually, promoting this to a documentation note rather than a finding. Let me note a different M finding.

#### M1 — `logging.MarkServerInitialized()` called before MCP session is established: log routing flag set prematurely

**File:** `/path/to/agent-lsp/cmd/agent-lsp/server.go`, line 1016
**Check:** `scope_analysis` / `silent_failure`

`MarkServerInitialized()` is called at line 1016, before `server.Run(ctx, transport)`. At this point, `logging.mcpServer` is still `nil` (no MCP client has connected yet). `Log` checks `if initialized && sender != nil` before routing to MCP, so the nil sender prevents actual MCP sends — this is safe.

However, the flag is also set correctly inside `InitializedHandler` (line 237), which fires when the MCP client actually connects. The premature call at line 1016 is redundant and misleading: it sets `serverInitialized = true` with no sender, then the handler sets it again (no-op the second time). If `SetServer` is ever called before line 1016 (e.g., if initialization order changes), log messages that should go to stderr (pre-session) would be routed to a partially-ready MCP session.

The call at line 1016 should be removed; `InitializedHandler` is the canonical and correct place.

**Confidence:** High (direct code read).

---

#### M2 — `DiffDiagnostics` is O(n*m): quadratic scan on large diagnostic sets

**File:** `/path/to/agent-lsp/internal/session/differ.go`, lines 42–83
**Check:** `scope_analysis`

`DiffDiagnostics` uses a nested loop comparing every diagnostic in `current` against every diagnostic in `baseline`:

```go
for _, curr := range current {
    for _, base := range baseline {
        if DiagnosticsEqual(curr, base) { found = true; break }
    }
    ...
}
```

For a file with N baseline diagnostics and M current diagnostics, this is O(N*M). For a file with 200 diagnostics on each side (common in large codebases after a mass refactor), this is 40,000 comparisons per evaluation call, called once per URI per evaluation. With many files in a workspace-scope evaluation, this compounds.

A fingerprint-keyed map (`map[string]int` counting occurrences, where fingerprint = `fmt.Sprintf("%d:%d:%s:%d", r.Start.Line, r.Start.Character, message, severity)`) reduces this to O(N+M).

**Confidence:** High (direct code read).

---

#### M3 — `textMatchApply` uses string concatenation for file URI: special characters in file paths not percent-encoded

**File:** `/path/to/agent-lsp/internal/tools/workspace.go`, line 338
**Check:** `scope_analysis`

```go
fileURI := "file://" + filePath
```

`CreateFileURI` (in `helpers.go`) uses `url.URL{Scheme: "file", Path: filePath}.String()` which percent-encodes special characters. `textMatchApply` uses string concatenation instead. For paths with spaces (e.g., `/home/user/my project/file.go`), the URI produced is `file:///home/user/my project/file.go` (unencoded space), while `CreateFileURI` would produce `file:///home/user/my%20project/file.go`. When this URI is used as a key in the `changes` map, `applyEditsToFile` calls `uripkg.URIToPath(uri)` which uses `url.Parse` and may interpret the space as a query separator, producing a wrong path.

**Confidence:** High (direct code read; `CreateFileURI` is the established pattern in this codebase, this is an inconsistent use).

---

#### M4 — `MarkServerInitialized()` called at line 1016 with no sender set, then again in `InitializedHandler`: redundant double call

*(Folded into M1 above.)*

---

### LOW

#### L1 — `NormalizeDocumentSymbols` SymbolInformation tree reconstruction uses pointer-dereferenced copies in Pass 3: children added in Pass 2 after the copy are included correctly, but the comment is misleading

**File:** `/path/to/agent-lsp/internal/lsp/normalize.go`, lines 76–83
**Check:** `scope_analysis`

The comment on Pass 3 says "Value-copying roots before Pass 2 completes would miss children added later." The implementation correctly defers the value copy to after Pass 2 completes, via `*symPtrs[i]`. This is correct. However, children appended to a parent's `Children []types.DocumentSymbol` slice are value-type elements (structs, not pointers). When Pass 3 dereferences `*symPtrs[i]`, it gets a copy of the parent including its `Children` slice header (pointer + len + cap) — which points to the same backing array populated in Pass 2. This works for the top-level copy, but grandchildren added to a child's `Children` after that child was appended to the parent will be in the child's backing array, not the grandchild's parent copy in the parent's `Children` slice.

In practice, `SymbolInformation` has no grandparent depth (it is a flat list), so this is not a practical bug — but the logic would fail for deeply-nested `SymbolInformation` hierarchies if such a server existed.

**Confidence:** Medium (structural analysis; not triggered by any real LSP server returning multi-level SymbolInformation).

---

#### L2 — `waitForWorkspaceReady` uses a 100ms polling loop with no notification channel: up to 100ms latency after workspace indexing completes

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, lines 396–414
**Check:** `scope_analysis`

`waitForWorkspaceReady` polls `progressTokens` every 100ms. When the last progress token is removed (the workspace is ready), the method may sleep up to 100ms before detecting it. For fast language servers (e.g., gopls on a small module), this is a gratuitous wait imposed on every `Initialize` call. A condition variable or a notification channel triggered from `handleProgress` when `progressTokens` becomes empty would eliminate the latency.

**Confidence:** High (direct code read).

---

#### L3 — `AddWorkspaceFolder` context parameter dropped: no context propagation to `sendNotification`

**File:** `/path/to/agent-lsp/internal/lsp/client.go`, line 1823
**Check:** `context_propagation`

`AddWorkspaceFolder(path string)` has no `ctx context.Context` parameter, but calls `c.sendNotification(...)` which internally calls `c.writeRaw(body)`. If the caller's context is cancelled (e.g., a tool timeout), there is no way to cancel the notification send. `sendNotification` always completes (or fails on pipe error) regardless of external cancellation. Compare with `sendRequest` which accepts and honors a context.

For a notification (fire-and-forget), this is lower severity than for a request. But it is inconsistent with `RemoveWorkspaceFolder` (which also drops context) and makes timeout enforcement impossible for folder changes.

**Confidence:** High (direct code read; `RemoveWorkspaceFolder` has the same pattern).

---

#### L4 — `HandleAddWorkspaceFolder` / `HandleRemoveWorkspaceFolder` / `HandleListWorkspaceFolders`: json.Marshal error discarded with `_`

**File:** `/path/to/agent-lsp/internal/tools/workspace_folders.go`, lines 34, 57, 71
**Check:** `silent_failure`

```go
data, _ := json.Marshal(map[string]interface{}{...})
return types.TextResult(string(data)), nil
```

If `json.Marshal` fails (theoretically impossible for this input, but the pattern is inconsistent), `data` is nil and `string(nil)` is `""`, returning an empty result to the caller with no error indication. Consistent error handling would use:

```go
data, err := json.Marshal(...)
if err != nil {
    return types.ErrorResult(fmt.Sprintf("marshaling response: %s", err)), nil
}
```

This matches the pattern used in `simulation.go`, `analysis.go`, and all other handlers.

**Confidence:** High (direct code read).

---

## Summary Table

| ID  | Severity | Check                | Location                                                    | One-line description                                                                           |
|-----|----------|----------------------|-------------------------------------------------------------|-----------------------------------------------------------------------------------------------|
| C1  | CRITICAL | bug / silent_failure | `internal/lsp/client.go:1848`                              | `AddWorkspaceFolder` calls `startWatcher(path)` which stops `rootDir` watcher; original workspace goes dark |
| C2  | CRITICAL | bug / silent_failure | `internal/lsp/client.go:193`                               | Exit-monitor goroutine does not clear `initialized`; `CheckInitialized` passes on crashed server |
| H1  | HIGH     | silent_failure       | `internal/lsp/normalize.go:61`                             | `nameMap[info.Name]` last-write-wins on duplicate names; children attached to wrong parent    |
| H2  | HIGH     | scope_analysis       | `internal/session/executor.go:12`                          | Single global semaphore serializes all sessions; independent sessions block each other        |
| H3  | HIGH     | bug                  | `internal/tools/position_pattern.go:40`, `workspace.go:323` | Byte column offsets passed as LSP UTF-16 character offsets; wrong positions on non-ASCII files |
| M1  | MEDIUM   | scope_analysis       | `cmd/agent-lsp/server.go:1016`                             | `MarkServerInitialized()` called before MCP session established; redundant, misleading flag   |
| M2  | MEDIUM   | scope_analysis       | `internal/session/differ.go:42`                            | `DiffDiagnostics` is O(n*m); quadratic scan on large diagnostic sets                         |
| M3  | MEDIUM   | scope_analysis       | `internal/tools/workspace.go:338`                          | `textMatchApply` uses string concat for URI instead of `url.URL`; spaces in paths not encoded |
| L1  | LOW      | scope_analysis       | `internal/lsp/normalize.go:76`                             | Pass 3 value-copy of roots includes children correctly for 1-level depth but not multi-level SymbolInformation |
| L2  | LOW      | scope_analysis       | `internal/lsp/client.go:396`                               | `waitForWorkspaceReady` polls at 100ms intervals; up to 100ms gratuitous latency after indexing |
| L3  | LOW      | context_propagation  | `internal/lsp/client.go:1823,1854`                         | `AddWorkspaceFolder`/`RemoveWorkspaceFolder` drop context; notification sends cannot be cancelled |
| L4  | LOW      | silent_failure       | `internal/tools/workspace_folders.go:34,57,71`             | `json.Marshal` error discarded with `_` in three workspace folder handlers                    |

---

## LSP Verification Status

| Finding | Required LSP call | Result |
|---------|-------------------|--------|
| C1      | Structural read   | N/A    |
| C2      | Structural read   | N/A    |
| H1      | Structural read   | N/A    |
| H2      | Structural read   | N/A    |
| H3      | Specification analysis + code read | N/A |
| All others | Structural / code read | N/A |

gopls returned "no package metadata" for all `find_references` calls during this session due to a `go.work` conflict in the parent directory. No `dead_symbol` checks requiring `findReferences` were attempted. All findings are based on direct code reads with high structural confidence.

---

## Notes on Prior Findings

The codebase shows systematic improvement between audit #5 and this audit: 15 out of 15 prior findings were addressed. The H2 fix introduced a new regression (C1 above) — a common pattern when a partial fix to a watcher architecture leaves the broader multi-root case unhandled. The most impactful new findings are C1 (watcher regression from H2 fix), C2 (initialized flag not cleared on crash), and H3 (UTF-16 column offset mismatch).
