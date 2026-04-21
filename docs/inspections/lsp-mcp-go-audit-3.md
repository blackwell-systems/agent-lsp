# agent-lsp Code Quality Audit — Inspection 3

## Summary

- **Audited:** `/path/to/agent-lsp` (full repo, post audit-2 fixes)
- **Layer map:**
  ```
  cmd/agent-lsp → internal/tools, internal/resources, internal/session,
                    internal/extensions, internal/lsp, internal/logging, internal/types
  internal/tools  → internal/lsp, internal/types, internal/session
  internal/resources → internal/lsp, internal/types
  internal/session → internal/lsp, internal/types
  internal/extensions → internal/types, internal/logging
  internal/lsp → internal/types, internal/logging
  internal/logging → (stdlib only)
  extensions/haskell → internal/extensions, internal/types
  Boundary: internal/lsp must not import internal/tools or above.
  Boundary: internal/types must not import any internal/* package.
  ```
- **Audit-2 findings status:** All 8 findings from audit-2 confirmed fixed. The
  `SerializedExecutor` now uses a channel semaphore with context cancellation.
  `ResourceTemplates()` is exported and wired into `server.AddResourceTemplate`.
  `Deactivate` is unexported. `RootDir()` doc comment is clean. Both silent unmarshal
  failures now log at debug level. `initWarning` defers stderr output to the first
  `Log()` call.
- **Highest severity:** error
- **Signal:** Four new findings. The most consequential are a permanent session livelock
  when `Evaluate` loses its context mid-flight, and an unprotected read of `session.Status`
  that is a data race under concurrent MCP calls. A `context.Background()` override in
  the `workspace/applyEdit` dispatch path and an incorrect `"plaintext"` language ID on
  untracked file opens are lower-severity but correctness-affecting.

---

## Tooling Note

The `mcp__lsp__start_lsp` tool was invoked as required by the inspector protocol and
returned `No such tool available` — LSP is unavailable in this environment. All
symbol-level findings use Grep as fallback and carry `confidence: reduced`. Grep misses
aliased calls, interface dispatch, and dynamically constructed references; each finding
notes this explicitly.

---

## internal/session/manager.go

**coverage_gap** · error · confidence: reduced
`internal/session/manager.go:199–210`
[LSP unavailable — Grep fallback, reduced confidence]

What: `Evaluate` sets `session.SetStatus(StatusEvaluating)` at line 204 before calling
`m.executor.Acquire(ctx, session)` at line 207. If `Acquire` returns an error (because
the request context was cancelled while waiting for the semaphore), the function returns
the error at line 208 — but the session is now permanently in `StatusEvaluating`. The
guard at the top of `Evaluate` (line 199) accepts only `StatusMutated` and
`StatusEvaluated`; `StatusEvaluating` is not in that list. Any subsequent call to
`Evaluate`, `Commit`, or `Discard` will fail with "cannot be evaluated/committed in state
evaluating" or "already in terminal state", making the session permanently unusable
without `Destroy`. A caller that calls `evaluate_session` with a tight deadline, gets a
timeout, and retries will find the session permanently broken.

Fix: Move `SetStatus(StatusEvaluating)` to after `Acquire` succeeds (line 211 in the
current numbering), so a failed acquisition leaves the session in `StatusMutated` or
`StatusEvaluated` and allows retry.

---

**coverage_gap** · error · confidence: reduced
`internal/session/manager.go:99, 199, 319, 372`
[LSP unavailable — Grep fallback, reduced confidence]

What: `session.Status` is read directly without holding `session.mu` at four locations in
`manager.go`:
- Line 99: `session.Status` in the `ApplyEdit` terminal-state guard
- Line 199–200: `session.Status` in the `Evaluate` state guard and error message
- Lines 319–320: `session.Status` in the `Commit` state guard and error message
- Line 372: `session.Status` in the `Discard` terminal-state guard

The write side consistently uses `SetStatus()` (which acquires `session.mu`) and
`MarkDirty()` (which also acquires `session.mu`). The symmetric read side does not.
`SimulationSession.IsTerminal()` and `IsDirty()` both lock `session.mu` before reading
`s.Status`. The direct reads in `manager.go` are therefore a data race: two concurrent
MCP tool calls (e.g. `evaluate_session` and `commit_session`) operating on the same
session ID can race on `session.Status`. The Go race detector would flag these.

Fix: Replace the four direct `session.Status` reads with calls to the existing
guarded accessor methods where possible (`IsTerminal()`, `IsDirty()`), or add explicit
`session.mu.Lock()`/`Unlock()` around the reads, consistent with `IsTerminal()` and
`IsDirty()`.

---

**coverage_gap** · warning · confidence: reduced
`internal/tools/simulation.go:315, 319–322`
[LSP unavailable — Grep fallback, reduced confidence]

What: `HandleSimulateEditAtomic` registers `defer mgr.Destroy(ctx, sessionID)` at line
315. The explicit `mgr.Discard(ctx, sessionID)` at line 332 reverts the LSP client's
in-memory document state back to the original content. However, when `ApplyEdit`
succeeds but `Evaluate` fails (lines 325–327), the function returns early before reaching
line 332. `Destroy` does not revert LSP state — it only removes the session entry from
the map. The LSP client is therefore left holding the modified file content for the
duration of the process, or until the file is reopened. Subsequent `get_diagnostics`,
`get_info_on_location`, or `go_to_definition` calls on that file will operate on stale
in-memory content until the next `open_document` or `get_diagnostics` call triggers a
`ReopenDocument`.

Fix: Call `mgr.Discard` before returning early on `Evaluate` failure, or restructure so
that `Discard` is also deferred (before `Destroy` in defer order, i.e. registered after
`Destroy`).

---

## internal/lsp/client.go

**context_propagation** · warning · confidence: reduced
`internal/lsp/client.go:290`
[LSP unavailable — Grep fallback, reduced confidence]

What: In the `"workspace/applyEdit"` branch of `dispatch()`, the server-initiated
workspace edit is applied via:
```go
applyErr = c.ApplyWorkspaceEdit(context.Background(), p.Edit)
```
`context.Background()` is a fresh root context that carries no deadline or cancellation.
`ApplyWorkspaceEdit` calls `applyEditsToFile`, which does file I/O
(`os.ReadFile`, `os.WriteFile`) and sends an LSP notification. If the LSP server sends a
`workspace/applyEdit` request for a large workspace edit (many files), this runs
completely uncancellable: no timeout, no propagated deadline, no way to abort.

The architectural constraint is that `dispatch()` is called from `readLoop()` which has
no request-scoped context — this is a design limitation, not a coding oversight. A
per-dispatch timeout would require refactoring `dispatch` to accept a context.

Fix: Construct a `context.WithTimeout(context.Background(), <reasonable deadline>)`
(e.g. 30 seconds, matching `defaultTimeout`) rather than a plain
`context.Background()`. This does not require architectural changes and bounds the
worst-case duration of file I/O in the read loop.

---

**coverage_gap** · warning · confidence: reduced
`internal/lsp/client.go:834`
[LSP unavailable — Grep fallback, reduced confidence]

What: `ReopenDocument` handles the case where the requested URI is not tracked by opening
the file from disk with a hardcoded `"plaintext"` language ID:
```go
return c.OpenDocument(ctx, uri, string(data), "plaintext")
```
This is reached when `HandleGetDiagnostics` (in `internal/tools/analysis.go`) calls
`client.ReopenDocument(ctx, fileURI)` on a file that was never explicitly opened via
`open_document`. The `"plaintext"` language ID causes the LSP server to receive a
`textDocument/didOpen` for the file as if it were plain text rather than Go, TypeScript,
or whatever language the server handles. Some servers (e.g. gopls) ignore files with
unrecognized language IDs entirely, silently returning no diagnostics for a file the
caller expected to be analyzed.

Fix: `ReopenDocument` should accept an optional `languageID` parameter (defaulting to
`""` or `"plaintext"`). Callers that know the language — `HandleGetDiagnostics` infers it
from file extension via the resolver — can pass it through. Alternatively, store the
language ID in `docMeta` and use it in the fallback open, even if the URI was not
previously tracked.

---

## internal/extensions/registry.go

**dead_symbol** · warning · confidence: reduced
`internal/extensions/registry.go:76`
[LSP unavailable — Grep fallback, reduced confidence]

What: `deactivate` (lowercase — unexported since audit-2) has no callers in any
production `.go` file. The test `TestRegistry_Deactivate` in `registry_test.go` calls it
directly via same-package access. No production code path ever deactivates an extension:
extensions are activated once at startup and persist for the process lifetime. The method
is not referenced by any server handler, shutdown path, or configuration change handler.

Fix: Delete `deactivate` and its test if runtime extension deactivation is not a planned
feature. If it is planned, add a comment marking it as reserved for future use so it is
not removed during dead-code cleanup.

---

## All Findings

| Severity | Confidence | Check Type | Finding | Location |
|----------|------------|------------|---------|----------|
| error | reduced | coverage_gap | `Evaluate` sets `StatusEvaluating` before `Acquire`; cancelled acquire permanently breaks session | `internal/session/manager.go:204` |
| error | reduced | coverage_gap | `session.Status` read without lock in 4 locations; data race with concurrent SetStatus writes | `internal/session/manager.go:99,199,319,372` |
| warning | reduced | coverage_gap | `HandleSimulateEditAtomic` early return after `Evaluate` failure skips `Discard`; LSP left with modified content | `internal/tools/simulation.go:315,325–327` |
| warning | reduced | context_propagation | `workspace/applyEdit` dispatch uses `context.Background()` — file I/O unbounded, no timeout | `internal/lsp/client.go:290` |
| warning | reduced | coverage_gap | `ReopenDocument` untracked-URI fallback opens file as `"plaintext"` — wrong language ID sent to LSP | `internal/lsp/client.go:834` |
| warning | reduced | dead_symbol | `deactivate` is unexported with no production callers; test-only | `internal/extensions/registry.go:76` |

---

## Not Checked — Out of Scope

- Test files (`*_test.go`) were read to confirm production call sites but were not
  themselves audited for correctness.
- `internal/lsp/framing.go` — Content-Length framing; clean per audit-2; no new checks
  applied.
- `internal/session/differ.go` — diagnostic diff logic; clean per audit-2; no new checks
  applied.
- `extensions/haskell/` — stub extension with all-nil handlers; no findings applicable.
- `internal/config/parse.go`, `internal/config/parse_test.go`,
  `internal/config/autodetect.go` — configuration parsing; no new check types applied.
- `test/multi_lang_test.go` — integration test harness; excluded from audit scope.
- `interface_saturation` — `ClientResolver` has 4 methods, all used by callers; within
  normal bounds.
- `test_coverage` — not applied; a full exported-symbol coverage sweep requires reliable
  LSP to be meaningful.
- `layer_violation` — all import blocks verified against layer map; no violations found.
- `panic_not_recovered` — `runWithRecovery` in `cmd/agent-lsp/main.go` provides
  top-level recovery; no unrecovered panics in goroutines found.
- `duplicate_semantics` (applyRangeEdit / applyEditsToFile) — acknowledged in audit-2,
  cross-reference comment added per the audit-2 IMPL; no new finding raised.

---

## Not Checked — Tooling Constraints

- **LSP `findReferences` and `hover`:** The `mcp__lsp__start_lsp` tool returned
  `No such tool available` at the start of this session. All symbol-level checks fell
  back to Grep. All findings carry `confidence: reduced`. Grep cannot detect aliased
  calls, interface-dispatch call sites, or dynamically constructed symbol references.
- **`SimulateChain` scope override:** Noted again from audit-2 — `SimulateChain` calls
  `Evaluate` with hardcoded `"file"` scope regardless of caller-supplied `timeoutMs`.
  Not raised as a finding; likely intentional per-step design.
