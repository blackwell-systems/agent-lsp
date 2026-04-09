# Speculative Execution for Code

**Status:** Shipped — all 8 tools implemented and CI-tested (`TestSpeculativeSessions` in `test/speculative_test.go`)
**Tools:** `create_simulation_session`, `simulate_edit`, `evaluate_session`, `simulate_chain`, `commit_session`, `discard_session`, `destroy_session`, `simulate_edit_atomic`

---

## Prerequisites

Call `start_lsp` with `root_dir` set to your workspace before using any simulation tools. The language server must be initialized and pointing at the correct workspace root for diagnostics to be meaningful.

```json
{ "root_dir": "/your/workspace" }
```

Simulation tools create sessions on the currently-running language server. If `start_lsp` has not been called (or was called with a different workspace), session results will be empty or incorrect.

---

## Position convention

All `start_line`, `start_column`, `end_line`, and `end_column` parameters are **1-indexed** — the same as line numbers shown by `cat -n` and most editors. The `extractRange` helper in the codebase converts these to the 0-indexed values the LSP protocol requires. Do not subtract 1 before passing positions to simulation tools.

---

## Quick start

The simplest path: use `simulate_edit_atomic` for a single speculative edit. It handles the full session lifecycle internally — no session ID to track, file on disk is never modified.

```
start_lsp(root_dir="/your/workspace")

simulate_edit_atomic(
  workspace_root="/your/workspace",
  language="go",
  file_path="/your/workspace/pkg/handler.go",
  start_line=42, start_column=1,
  end_line=42,   end_column=20,
  new_text="replacement text",
  scope="file",
  timeout_ms=5000
)

→ {"errors_introduced": null, "errors_resolved": null, "net_delta": 0, "confidence": "high"}
```

`net_delta: 0` means no new errors were introduced — safe to apply. A positive `net_delta` means the edit would break things; inspect `errors_introduced` for details.

---

## The Idea

LSP today is a query engine — agents ask what exists and react to what they find. This makes AI-assisted editing inherently trial-and-error: edit, discover breakage, fix, repeat.

Speculative code sessions turn LSP into a simulation engine. Create an isolated semantic workspace, apply hypothetical changes, evaluate the resulting diagnostic state, then commit or discard — without ever touching disk.

```
Current workflow:    edit → discover breakage → fix → repeat
With sessions:       create session → mutate → evaluate → decide → commit once (or discard)
```

The valuable primitive is not "preview one edit." It is:

> create an isolated semantic future of the codebase

This is the first agent-native primitive in agent-lsp. Everything else (navigation, diagnostics, hover) is LSP exposed. This is new capability: **isolated state → mutation → evaluation → commit or discard**.

---

## Session Lifecycle

A speculative code session is an isolated semantic workspace rooted in a baseline code state.

```
create_simulation_session(workspace_root, language)
    → session_id

simulate_edit(session_id, file_path, range, new_text)
    → edit_result

evaluate_session(session_id)
    → evaluation_result

[optional: additional simulate_edit calls]

commit_session(session_id, target?)   OR   discard_session(session_id)
    → commit_result                            → ok

destroy_session(session_id)
    → ok
```

Session state persists across operations. A session accumulates speculative edits and maintains its own diagnostic snapshot. Multiple sessions may exist in parallel, each with independent state.

### Session Phases

| Phase | Entered by | Exits to |
|-------|-----------|----------|
| `created` | `create_simulation_session` | `mutated`, `evaluating` |
| `mutated` | `simulate_edit` | `mutated`, `evaluating` |
| `evaluating` | `evaluate_session` | `evaluated`, `timed_out` |
| `evaluated` | evaluation completes | `mutated`, `committed`, `discarded` |
| `committed` | `commit_session` | `destroyed` |
| `discarded` | `discard_session` | `destroyed` |
| `dirty` | revert failure or version mismatch | — (terminal, requires destroy) |

A `dirty` session must not be committed. Call `destroy_session` to clean up.

---

## Session State Model

A session holds:

- **baseline_ref** — the workspace state at session creation (read-only within session)
- **isolated LSP semantic state** — in-memory document buffers managed by the session
- **document versions** — per-document version counter, monotonically increasing
- **accumulated speculative edits** — ordered list of edits applied within the session
- **diagnostics snapshot** — latest diagnostic state after most recent evaluation
- **session status** — one of: `created`, `mutated`, `evaluated`, `committed`, `discarded`, `dirty`
- **session_id** — UUID assigned at creation, used for tracing

The baseline is the code state at the moment `create_simulation_session` is called. It is immutable from the session's perspective — the session can only mutate its own overlay.

---

## Isolation Model

**Session isolation is per-session, not per-call.**

- One session must not observe another session's speculative state
- The baseline is conceptually shared (read-only); speculative overlays are session-local
- Commit materializes one session's state; discard removes it without side effects
- No cross-session visibility at any point

### Logical isolation vs physical isolation

This is the primary unresolved architectural tension.

**Logical isolation (current design):** a single LSP server instance handles all sessions. Concurrent sessions on the same server are serialized — only one session may hold mutated in-memory state at a time. The mutex enforces ordering; sessions do not run truly in parallel against the same server.

```
session_a and session_b on same server:
  → session_a acquires lock, mutates, evaluates, reverts, releases
  → session_b acquires lock (was blocked), mutates, evaluates, reverts, releases
```

This provides **correct results** and **no state leakage**, but sessions are sequential, not parallel.

**Physical isolation (future path):** each session gets its own LSP server instance. Sessions run truly in parallel with no serialization. Cost: LSP startup per session (~1-3s, memory per server), which makes it impractical for short-lived sessions.

**Current choice: logical isolation.**

Reasoning: for the primary use cases (single-agent planning, sequential comparison), serialization is not a bottleneck. The ~500ms per simulation is fast enough that the queue rarely matters. Physical isolation is the right upgrade path if parallel multi-agent simulation becomes a real workload.

This is explicitly documented, not hidden:

> Speculative code sessions use serialized access to a shared language server to guarantee isolation. This provides deterministic behavior without the overhead of per-session LSP instances. True parallel execution with per-session language servers may be introduced in future versions if workload characteristics justify it.

---

## Concurrent Session Semantics

Multiple sessions may exist simultaneously:

- Each session has independent semantic state
- Evaluation results are comparable across sessions (different strategies, same baseline)
- No cross-session visibility
- Sessions on different language servers may evaluate concurrently
- Sessions on the same language server are serialized within that server's scope (V1)

This enables consumers (like Scout-and-Wave) to run strategy comparison:

```
session_a = create_simulation_session(...)    # strategy A
session_b = create_simulation_session(...)    # strategy B

simulate_edit(session_a, edit_1a)
simulate_edit(session_b, edit_1b)

result_a = evaluate_session(session_a)
result_b = evaluate_session(session_b)

# compare result_a.net_delta vs result_b.net_delta
# pick the winner, commit that session
```

---

## Evaluation Model

Mutation and observation are separate operations.

**`simulate_edit(session_id, edit)` → edit_result**

Mutates session state. Pushes `textDocument/didChange` to the language server's in-memory buffer. Does not evaluate diagnostics — returns only whether the edit was applied.

```json
{
  "session_id": "a3f2-...",
  "edit_applied": true,
  "version_after": 3
}
```

**`evaluate_session(session_id)` → evaluation_result**

Observes current session state. Calls `WaitForDiagnostics`, diffs against baseline, returns impact summary. Does not mutate state.

```json
{
  "session_id": "a3f2-...",
  "errors_introduced": [{ "line": 42, "col": 5, "message": "cannot use string as int", "severity": "error" }],
  "errors_resolved": [],
  "net_delta": 1,
  "affected_symbols": ["HandleRequest"],
  "edit_risk_score": 0.73,
  "scope": "file",
  "confidence": "high",
  "timeout": false,
  "duration_ms": 412
}
```

A caller may call `simulate_edit` multiple times before calling `evaluate_session`. The evaluation reflects the cumulative state.

**Atomic convenience wrapper:** `simulate_edit_atomic` is internally a create → apply → evaluate → discard cycle. Accepts an optional `session_id` to reuse an existing session; if omitted, creates and destroys a temporary session automatically. Useful for single-edit what-if checks without managing session lifecycle.

---

## Commit Semantics

`commit_session(session_id, target?)` materializes the accumulated speculative state.

### Functional vs imperative commit

Two models, both supported:

**Functional (default):** `commit_session` returns a `WorkspaceEdit`-compatible patch. The caller decides whether and how to apply it. No disk writes. Safe for CI, multi-agent orchestration, and any caller that wants to inspect the patch before applying.

**Imperative (opt-in):** pass `apply: true` (or a `target` path) to write files to disk directly. Equivalent to calling `apply_edit` on the returned patch, but in one step.

Default is **functional** — return patch only, no side effects. Callers opt into disk writes explicitly.

```
commit_session(session_id)                  # → WorkspaceEdit patch, no disk write
commit_session(session_id, apply: true)     # → writes to disk + returns patch
commit_session(session_id, target: "/path") # → writes to target path + returns patch
```

This matters for:
- **CI** — inspect patch, validate, then decide whether to apply
- **Multi-agent** — one agent commits a patch, orchestrator applies after comparing
- **Safety** — patch-only commit cannot corrupt workspace state

**Commit constraints:**

- Commit is only allowed from a session in `evaluated` or `mutated` state
- Commit is prohibited on `dirty` sessions — the state may be corrupt
- Commit is prohibited on `created` sessions — no edits have been applied
- A timed-out evaluation does not block commit, but the session carries `confidence: "partial"`

**After commit:**

- Session transitions to `committed`
- Session may not be mutated further
- Call `destroy_session` to release resources

**Discard:**

`discard_session(session_id)` reverts all accumulated in-memory state and releases the session. Nothing is written to disk. Equivalent to rolling back a transaction.

---

## Failure and Corruption Semantics

### Per-operation failure behavior

| Operation | Failure | Behavior |
|-----------|---------|----------|
| `create_simulation_session` | Server unavailable | Return error; no session created |
| `simulate_edit` | Server rejects `didChange` | Abort; session state unchanged; return error |
| `evaluate_session` timeout | Diagnostics did not settle | Return snapshot with `confidence: "partial"`, `timeout: true`; session remains usable |
| `evaluate_session` connection failure | After mutation | Attempt internal revert; mark session `dirty` if revert fails |
| `commit_session` | Write failure | Return error; session state preserved; retry allowed |
| `discard_session` | Revert failure | Mark session `dirty`; error returned; call `destroy_session` to force cleanup |
| Concurrent mutation detected | Another `didChange` arrived during evaluation | Mark result `confidence: "stale"`; session remains usable; do not retry automatically |

### Session dirty state

A session becomes `dirty` when:

- An internal revert fails during `discard_session`
- A connection failure occurs while the session holds mutated state
- Document version tracking detects a gap (concurrent external mutation)

A dirty session:

- Must not be committed — state may not reflect intended mutations
- Must be destroyed via `destroy_session` (forced cleanup)
- Reports `session_dirty: true` on all subsequent operation calls

**Guarantee:** the system will not silently continue in a corrupted state. Any unrecoverable failure surfaces immediately.

---

## Session Invariants

These must hold for every session, for every operation:

1. **Isolation** — no other session may read or mutate this session's speculative state
2. **Baseline immutability** — the baseline is read-only from the session's perspective; only the session's overlay is mutable
3. **Monotonic versioning** — document versions are strictly increasing within a session; `N → N+1 → N+2 → ...`; version never rolls back
4. **No silent corruption** — a session either holds valid state or is marked `dirty`; there is no in-between
5. **Evaluation reflects session state only** — `evaluate_session` returns diagnostics caused by edits in this session, not external mutations
6. **Commit requires valid state** — `dirty` sessions must not be committed under any circumstances

---

## Implementation Scope

Build the full session model in one pass. There are no delivery tiers — the session API is the foundation from day one.

### Core API (full implementation)

```
create_simulation_session(workspace_root, language) → session_id
simulate_edit(session_id, file_path, range, new_text) → edit_result
evaluate_session(session_id, scope?, timeout_ms?) → evaluation_result
simulate_chain(session_id, edits[]) → chain_result
commit_session(session_id, target?) → commit_result
discard_session(session_id) → ok
destroy_session(session_id) → ok
```

### Convenience alias

`simulate_edit_atomic` is a thin wrapper — not a separate API, just a helper for callers that don't need session persistence:

```go
func SimulateEditAtomic(ctx, mgr, args) (ToolResult, error) {
    sid := mgr.Create(ctx, ...)
    defer mgr.Destroy(ctx, sid)
    mgr.ApplyEdit(ctx, sid, ...)
    return mgr.Evaluate(ctx, sid, ...)
}
```

Exposed as an MCP tool for single-edit use cases. Backed by the same session infrastructure — no separate code path.

### Scope support

Both single-file (`scope: "file"`) and workspace (`scope: "workspace"`) are implemented together. Workspace scope carries `confidence: "eventual"` to be honest about cross-file propagation timing.

Cross-file diagnostic propagation behavior by server:
| Server | Cross-file reliability | Typical propagation time |
|--------|----------------------|-----------------------------|
| gopls | High (re-typechecks importing packages) | 2-5s |
| tsserver | Good (project-wide) | 1-3s |
| rust-analyzer | High | 2-4s |
| Others | Inconsistent | unknown |

### Chained mutations

`simulate_chain` applies a sequence of edits within a session and evaluates after each step:

```
simulate_chain(session_id, [edit_1, edit_2, edit_3]) → {
  steps: [
    { step: 1, net_delta: 0, errors_introduced: [] },
    { step: 2, net_delta: 3, errors_introduced: [...] },
    { step: 3, net_delta: 0, errors_introduced: [] }
  ],
  safe_to_apply_through_step: 1,
  cumulative_delta: 0
}
```

Each step builds on the previous in-memory state. `safe_to_apply_through_step` is the last step with `net_delta: 0`.

---

## Document Versioning

Version numbers are per-session, per-document:

```
session created, document opened: version N (baseline)
simulate_edit call 1:             N+1
simulate_edit call 2:             N+2
discard / revert:                 N+3  (revert is itself a new version, not a rollback)
```

Versions never roll back. The revert `didChange` sends the original content with the next monotonically increasing version number.

Version is tracked per open document on the session's `LSPClient`. Mismatch between expected and tracked version invalidates the session (marks `dirty`).

---

## Diagnostic Diffing

Two diagnostics are considered identical if all of the following match:

- `range.start` (line + character)
- `range.end` (line + character)
- `message` (exact string)
- `severity` (error / warning / info / hint)
- `source` (optional — ignored if absent in either)

The diff is computed as:

- **introduced:** present in post-simulation diagnostics, not in baseline
- **resolved:** present in baseline, not in post-simulation diagnostics
- **unchanged:** present in both (not returned — reduces noise)

Position matching uses post-edit coordinates. Baseline diagnostics reflect pre-edit positions by design — the delta communicates what changed, not where things moved to.

---

## Evaluation Response Contract

```json
{
  "session_id": "a3f2-...",
  "errors_introduced": [
    { "line": 42, "col": 5, "message": "cannot use string as int", "severity": "error" }
  ],
  "errors_resolved": [],
  "net_delta": 1,
  "affected_symbols": ["HandleRequest"],
  "edit_risk_score": 0.73,
  "scope": "file",
  "confidence": "high",
  "timeout": false,
  "duration_ms": 412
}
```

`confidence` values:
- `"high"` — single-file, diagnostics settled within timeout
- `"partial"` — timed out, returned snapshot may be incomplete
- `"stale"` — concurrent mutation detected during evaluation window
- `"eventual"` — workspace scope, cross-file propagation may be incomplete

`edit_risk_score` heuristics:
- Errors introduced in exported symbols → higher risk
- Errors in test files only → lower risk
- Net delta 0 (no diagnostic change) → 0.0
- Multiple errors in call sites → higher risk

---

## Baseline Stability

The diagnostic diff is only as trustworthy as the baseline. If the baseline is incomplete, errors that already exist appear in `errors_introduced` — false positives that corrupt the diff.

**The problem:** LSP diagnostic publication is asynchronous. After a document opens, the language server processes it and publishes via `textDocument/publishDiagnostics` over a window of milliseconds to seconds. Snapshotting before this window closes produces an incomplete baseline.

### Strategy: lazy per-file settle

On first `simulate_edit` for a given file, wait for that file's diagnostics to settle before recording its per-file baseline. Do not pay for files the session never touches.

```
simulate_edit(session_id, file_path, edit)
  → if file not in session.baselines:
      WaitForDiagnostics(file_path)
      session.baselines[file_path] = snapshot
  → apply edit
  → return edit_result
```

This is the correct strategy for all cases: pay per touched file, not per session. A session that touches one file in a large workspace does not pay settle cost for the rest.

### What "settled" means

Diagnostics are considered settled when no new `textDocument/publishDiagnostics` notification has arrived for the target file within a quiet window (default: 300ms). The existing `WaitForDiagnostics` implementation handles this.

A settle timeout (default: 3000ms) caps the wait. If the server has not published anything within the timeout, use whatever is cached — and mark the baseline with `baseline_confidence: "partial"` to flag that the diff may contain false positives.

---

## Timeout Behavior

If diagnostics do not settle within the timeout window:

- Return the current diagnostic snapshot (whatever the server has published so far)
- Set `confidence: "partial"` and `timeout: true` in the response
- Internal revert still executes — timeout applies only to diagnostic collection, not session cleanup
- No automatic retry

Default timeout: 3000ms (single-file), 8000ms (workspace scope). Configurable via `timeout_ms` argument.

---

## Revert Guarantee (Internal)

Revert is unconditional within the session. When a session is discarded or an evaluation produces partial results, the in-memory state is always restored via `defer`:

```go
defer func() {
    if err := session.Revert(ctx); err != nil {
        session.MarkDirty(fmt.Errorf("revert failed: %w", err))
    }
}()
```

If revert fails:
- Session is marked `dirty`
- Error is returned to the caller (not silenced)
- No further operations on this session will succeed

This is an internal implementation detail, not a user-visible contract. Users see session state (`dirty` or clean); they do not manage revert explicitly.

---

## Observability

Emit structured log events at each phase:

| Event | Fields |
|-------|--------|
| `session.created` | `session_id`, `workspace_root`, `language` |
| `session.edit_applied` | `session_id`, `file`, `range`, `version_after` |
| `session.evaluation_start` | `session_id`, `edit_count`, `scope` |
| `session.evaluation_complete` | `session_id`, `duration_ms`, `net_delta`, `confidence` |
| `session.committed` | `session_id`, `files_written`, `duration_ms` |
| `session.discarded` | `session_id`, `edit_count` |
| `session.dirty` | `session_id`, `step`, `error` |
| `session.destroyed` | `session_id` |

These events flow through the existing `logging` package at `LevelDebug` (lifecycle events) and `LevelError` (dirty/failure). No new infrastructure required.

---

## Cross-Language Limits

In multi-server mode, a session operates on one language server at a time. A TypeScript change that breaks a Go caller (via a shared JSON contract) will not surface in the session — the Go server has no knowledge of the TypeScript edit.

This is an honest constraint, not a flaw. Single-language impact is the right scope.

---

## Positioning

When shipping:

> "Simulate code changes before applying them. See exactly what breaks — without touching your files."

This is the correct message. It describes the behavior precisely and makes the agent-native value immediate.

Do not frame it as a testing tool or a linting tool. Frame it as **planning infrastructure**.

---

## Implementation Notes

**V1 tool handler (atomic wrapper):**

```go
func HandleSimulateEditAtomic(ctx context.Context, mgr *SessionManager, args map[string]interface{}) (types.ToolResult, error) {
    // 1. Validate args (file_path, range, new_text)
    // 2. ValidateFilePath
    // 3. session = mgr.Create(ctx, workspaceRoot, language)
    // 4. defer session.Destroy(ctx)
    // 5. baseline = session.GetDiagnostics(ctx, uri)
    // 6. session.ApplyEdit(ctx, uri, range, newText)
    // 7. result = session.Evaluate(ctx, timeout)
    // 8. return SimulateEditResult from result.Diff(baseline)
}
```

**Session executor interface (pluggable isolation):**

```go
// SessionExecutor abstracts how a session acquires and releases LSP access.
// V1 serializes; future versions may provide per-session LSP instances.
type SessionExecutor interface {
    Acquire(ctx context.Context, session *SimulationSession) error
    Release(session *SimulationSession)
}

// SerializedExecutor — V1 implementation. Per-server mutex.
type SerializedExecutor struct {
    mu sync.Mutex
}

func (e *SerializedExecutor) Acquire(ctx context.Context, s *SimulationSession) error {
    e.mu.Lock()
    return nil
}

func (e *SerializedExecutor) Release(s *SimulationSession) {
    e.mu.Unlock()
}
```

Session IDs and the full API remain unchanged regardless of executor. Swapping `SerializedExecutor` for an `IsolatedExecutor` (per-session LSP) requires no API changes.

**Session manager structure:**

```go
type SessionManager struct {
    sessions map[string]*SimulationSession
    executor SessionExecutor
    mu       sync.RWMutex
}

type SimulationSession struct {
    ID        string
    client    *lsp.LSPClient
    status    SessionStatus
    edits     []AppliedEdit
    baselines map[string]DiagnosticsSnapshot // per-file, populated lazily on first simulate_edit
    mu        sync.Mutex
}
```

**`textDocument/didChange` format:**
```json
{
  "textDocument": { "uri": "file:///path", "version": N },
  "contentChanges": [
    {
      "range": { "start": {"line": L, "character": C}, "end": {...} },
      "text": "new content"
    }
  ]
}
```

Version must increment on each change. Tracked per open document on `SimulationSession`.

---

## Open Questions

### Resolved
- ✅ **Baseline diagnostic timing** — lazy per-file settle. See **Baseline Stability**.
- ✅ **Session lifecycle** — create / mutate / evaluate / commit / discard / destroy.
- ✅ **Mutation vs evaluation separation** — `simulate_edit` mutates, `evaluate_session` observes.
- ✅ **Diagnostic diff model** — range + message + severity + source equality. See **Diagnostic Diffing**.
- ✅ **Versioning model** — monotonic, never rolls back. See **Document Versioning**.
- ✅ **Commit semantics** — functional by default (patch only), imperative opt-in. See **Commit Semantics**.
- ✅ **Failure surfacing** — dirty state, no silent corruption. See **Failure and Corruption Semantics**.

### Open (ranked)

**1. Isolation model: logical vs physical** ✅ Resolved

Shipped as logical isolation (serialized shared LSP). Constraint is documented. `SessionExecutor` interface is the upgrade seam for per-session LSP instances if workload justifies it.

**2. Workspace evaluation: best-effort or deterministic?** ✅ Resolved

Shipped as best-effort with `confidence: "eventual"` for workspace scope. Acceptable for planning use cases. Revisit if CI-grade guarantees become required.

**3. Session resource cost** ✅ Resolved by implementation

No session cap enforced. In-memory document buffers per touched file. Acceptable for current workloads; monitor if session counts scale significantly.

**4. Session storage** ✅ Resolved

Sessions are in-memory only — IDs become invalid on MCP server restart. This is the correct design. `commit_session` returns a portable `WorkspaceEdit` that callers can persist independently.

**5. Dirty state recovery** ✅ Resolved

Dirty is terminal — destroy and reinitialize. No recovery path. Dirty means LSP state is unknown; replaying edits against uncertain base is worse than reinitializing.

---

## Deferred by Design

These are intentional deferrals with designed seams for future upgrade — not missing features.

### Physical isolation (per-session LSP instances)

**Deferred.** Serialized execution provides correctness. The `SessionExecutor` interface is the upgrade seam — swap `SerializedExecutor` for `IsolatedExecutor` without API changes.

**Revisit triggers:**
- p95 queue wait > 1s
- Sustained concurrent sessions > 5–10
- Users reporting blocked workflows from serialization

### Session persistence

**Deferred.** Sessions are ephemeral compute artifacts. Durability is provided via returned patches — `commit_session` returns a portable `WorkspaceEdit` that callers can persist, store, or replay independently.

> Sessions are ephemeral; artifacts are durable.

**Revisit triggers:**
- Long-running planning sessions that span MCP restarts
- Human-in-the-loop workflows that require resume

### Deterministic workspace evaluation

**Deferred.** Best-effort with explicit `confidence` flags. Agents can re-evaluate or fall back to file scope when results carry `confidence: "eventual"`. Final correctness comes from re-validation after commit, not from the simulation itself.

**Revisit triggers:**
- CI-grade guarantees required at workspace scope
- Addition of a final validation pass (fresh session post-merge)
