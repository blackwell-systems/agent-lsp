# Speculative Execution for Code

**Status:** Planning
**Feature:** `simulate_edit` tool

---

## The Idea

LSP today is a query engine — agents ask what exists and react to what they find. This makes AI-assisted editing inherently trial-and-error: edit, discover breakage, fix, repeat.

`simulate_edit` turns LSP into a simulation engine. Push a change to the language server's in-memory buffer, evaluate the resulting diagnostic state, revert, return the impact — without touching disk.

```
Current workflow:    edit → discover breakage → fix → repeat
With simulate_edit:  simulate → see full impact → decide → apply once
```

This is the first agent-native primitive in lsp-mcp-go. Everything else (navigation, diagnostics, hover) is LSP exposed. This is new capability: **state mutation → evaluation → rollback**.

---

## Why This Works

LSP already supports `textDocument/didChange` for in-memory document state. The language server typechecks the buffer and publishes diagnostics — without any disk writes. Reverting is another `didChange` with the original content.

The infrastructure already exists in this codebase:
- `WithDocument` — document lifecycle management
- `WaitForDiagnostics` — diagnostic settling with timeout
- `did_change_watched_files` — change notification plumbing

Implementation cost is a new tool handler that sequences: **push → wait → diff → revert → return**.

---

## What Diagnostics Give You

Diagnostics accurately detect:
- Type errors (broken signatures, wrong argument types)
- Syntax errors
- Unresolved symbols
- Interface implementation failures

Diagnostics do **not** directly give:
- "K call sites will break" — use `get_references` + simulation together for this
- Behavioral/runtime changes
- Cross-language impact in multi-server mode

**Do not promise:** "K call sites break"
**Do promise:** "diagnostic impact — N errors introduced, M resolved"

---

## Delivery Tiers

### V1 — Single-file impact preview (ship first)

```
simulate_edit(
  file_path:  string,
  start_line: int,
  start_col:  int,
  end_line:   int,
  end_col:    int,
  new_text:   string
) → {
  errors_introduced: [{ line, col, message, severity }],
  errors_resolved:   [{ line, col, message, severity }],
  net_delta:         int,      // positive = more errors, negative = fewer
  scope:             "file",
  confidence:        "high"
}
```

**Implementation:** get diagnostics → `didChange` → `WaitForDiagnostics` → diff → `didChange` revert → return delta.

Single-file response is instant (~500ms) and reliable. Confidence is high because the language server fully retypechecks the in-memory buffer.

**What agents can do with V1:**
- "Does this rename introduce any errors in this file?"
- "Is it safe to delete this function body?"
- "Does changing this type signature break anything visible?"

---

### V1.5 — Enriched impact signal

Extend the response with inferred signals:

```json
{
  "errors_introduced": [...],
  "errors_resolved":   [...],
  "net_delta":         2,
  "affected_symbols":  ["HandleRequest", "ServeHTTP"],
  "edit_risk_score":   0.73,   // 0.0 = safe, 1.0 = high risk
  "scope":             "file",
  "confidence":        "high"
}
```

**`edit_risk_score` heuristics:**
- Errors introduced in exported symbols → higher risk
- Errors in test files only → lower risk
- Net delta 0 (no diagnostic change) → 0.0
- Multiple errors in call sites → higher risk

Agents can now **compare plans** — simulate option A, simulate option B, pick the one with lower risk score.

---

### V1.5 — Workspace-scope (opt-in, slower)

```
simulate_edit(..., scope: "workspace", timeout_ms: 8000) → {
  errors_introduced:  [...],   // may include errors in other files
  errors_resolved:    [...],
  net_delta:          int,
  scope:              "workspace",
  confidence:         "eventual"  // cross-file propagation may be incomplete
}
```

Cross-file diagnostic propagation behavior by server:
| Server | Cross-file reliability | Typical propagation time |
|--------|----------------------|--------------------------|
| gopls | High (re-typechecks importing packages) | 2-5s |
| tsserver | Good (project-wide) | 1-3s |
| rust-analyzer | High | 2-4s |
| Others | Inconsistent | unknown |

Workspace scope is honest about confidence: cross-file propagation may be incomplete within the timeout window.

---

### V2 — Chained simulations (refactor planning)

```
simulate_chain([
  { file, edit_1 },
  { file, edit_2 },
  { file, edit_3 }
]) → {
  steps: [
    { step: 1, net_delta: 0, errors_introduced: [] },
    { step: 2, net_delta: 3, errors_introduced: [...] },
    { step: 3, net_delta: 0, errors_introduced: [] }
  ],
  safe_to_apply_through_step: 1,  // last step with net_delta 0
  cumulative_delta: 0
}
```

Each step builds on the previous in-memory state. Agent can simulate an entire multi-step refactor before applying any of it. The server sees a sequence of incremental edits; diagnostics reflect the cumulative state after each.

This requires sequential didChange + WaitForDiagnostics calls with state accumulation, reverted in reverse at the end.

---

## The Architectural Problem: Concurrency

This is the only real blocker. Between `didChange` and revert, the document is in a mutated in-memory state. Another MCP tool call arriving during this window would run against simulated state — returning results based on a change that was never applied to disk.

Three viable designs:

### Option A — Global simulation lock (V1)

Acquire a per-client mutex before pushing the edit, release after revert. Other tool calls block until the simulation completes (~500ms for single-file).

```
Pros:  Simple to implement. Completely safe.
Cons:  Serializes all tool calls during simulation window.
       Acceptable for V1 given short duration (~500ms).
```

### Option B — Document version guard (V1.5)

Tag the simulation with a document version number. Detect if any concurrent `didChange` arrived during the simulation window; if so, mark result as stale and retry once.

```
Pros:  Non-blocking for unrelated files.
Cons:  Adds retry logic. Version tracking per document.
```

### Option C — Shadow buffer (V2, if servers support it)

Maintain separate real and simulated document state. Only the simulation uses the shadow. Requires server cooperation (most LSP servers don't support multiple views cleanly — one document URI, one state).

**Verdict:** Ship V1 with Option A (global lock). The simulation window is short enough that serialization isn't a user-facing problem. Upgrade to Option B for V1.5 if lock contention is observed.

---

## Cross-Language Limits

In multi-server mode, `simulate_edit` operates on one language server at a time. A TypeScript change that breaks a Go caller (via a shared JSON contract) will not surface in the simulation — the Go server has no knowledge of the TypeScript edit.

This is an honest constraint, not a flaw. Single-language impact is the right scope for V1.

---

## Positioning

When shipping:

> "Simulate code changes before applying them. See exactly what breaks — without touching your files."

This is the correct message. It describes the behavior precisely and makes the agent-native value immediate.

Do not frame it as a testing tool or a linting tool. Frame it as **planning infrastructure**.

---

## Invariants

These must hold for every execution of `simulate_edit`:

1. **No external observation of simulated state** — no other tool call may read or mutate LSP document state during a simulation window
2. **Exact revert** — the document must be restored to the byte-identical state prior to simulation; version numbers do not roll back (they increment forward)
3. **Strict monotonic versioning** — each `didChange` increments the document version by exactly 1; the revert uses the next version number, not the pre-simulation version
4. **Complete or invalid** — a simulation either completes with a valid result, or fails and marks the session dirty; there is no silent partial success

---

## Failure Semantics

Each step has a defined failure behavior:

| Step | Failure | Behavior |
|------|---------|----------|
| `didChange` (apply) | Server rejects or connection fails | Abort immediately; no mutation occurred; return error |
| `WaitForDiagnostics` timeout | Diagnostics did not settle | Return current snapshot with `confidence: "partial"`, `timeout: true` |
| `WaitForDiagnostics` error | Connection failure after mutation | Attempt revert; mark session dirty if revert also fails |
| Revert `didChange` | Server rejects or connection fails | Mark session corrupted; block all further operations; surface error immediately |
| Concurrent mutation detected | Another `didChange` arrived during window | Mark result stale; return `confidence: "stale"`; do not retry automatically |

**Guarantee:** the system will not silently continue in a corrupted state. Any unrecoverable failure surfaces immediately.

---

## Session Integrity

The LSP session is assumed valid only when:

- Document versions are consistent (monotonically increasing with no gaps)
- Revert completed successfully
- No concurrent mutation was detected during the simulation window

If any of these are violated, the session is marked **dirty**:

- All subsequent tool calls fail fast with `"session dirty: reinitialize required"`
- The client must call `restart_lsp_server` to recover
- The dirty flag is cleared on successful reinitialization

This prevents ghost bugs — subtle misbehavior caused by a language server holding stale in-memory state while the rest of the system assumes it is clean.

---

## Isolation Model

`simulate_edit` executes under a **global simulation lock** (V1).

**Guarantee:** no other tool call may read or mutate LSP state between the `didChange` (apply) and `didChange` (revert) calls. From the perspective of all other tools, the simulation is atomic.

This is the transaction boundary. The lock is per `LSPClient` instance. In multi-server mode, each server has its own lock; simulations on different servers may execute concurrently.

---

## Document Versioning

- Each `textDocument/didChange` call increments the document version by 1
- Version is tracked per open document on `LSPClient` (not currently implemented — prerequisite for V1)
- The revert `didChange` uses the next version number (pre-simulation version + 2), not the original version
- A version mismatch between expected and actual (detectable if the server echoes version) invalidates the simulation result

```
pre-simulation version:  N
apply didChange:          N+1
revert didChange:         N+2
post-simulation version:  N+2  (not N — versions never roll back)
```

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

Position matching uses the post-edit line/character coordinates. If the edit shifts lines, the baseline diagnostics are not adjusted — they reflect the pre-edit positions, which is intentional (the delta communicates what changed, not where things moved to).

---

## Timeout Behavior

If diagnostics do not settle within the timeout window:

- Return the current diagnostic snapshot (whatever the server has published so far)
- Set `confidence: "partial"` and `timeout: true` in the response
- The revert still executes — timeout applies only to diagnostic collection, not to session cleanup
- No automatic retry

Default timeout: 3000ms (single-file), 8000ms (workspace scope). Configurable via `timeout_ms` argument.

---

## Revert Guarantee

Revert is unconditional. It executes via `defer` before any return path, including panics.

```go
defer func() {
    if err := client.RevertSimulation(ctx, uri, originalContent, nextVersion); err != nil {
        client.MarkSessionDirty(fmt.Errorf("revert failed: %w", err))
    }
}()
```

If revert fails:
- Session is marked dirty
- Error is returned to the caller (not silenced)
- No further operations on this client will succeed until reinitialization

---

## Observability

Emit structured log events at each phase:

| Event | Fields |
|-------|--------|
| `simulate_edit.start` | `simulation_id`, `file`, `range`, `new_text_length` |
| `simulate_edit.apply` | `simulation_id`, `version_before`, `version_after` |
| `simulate_edit.diagnostics_ready` | `simulation_id`, `duration_ms`, `diagnostic_count`, `timed_out` |
| `simulate_edit.revert` | `simulation_id`, `version` |
| `simulate_edit.complete` | `simulation_id`, `total_duration_ms`, `net_delta`, `confidence` |
| `simulate_edit.failure` | `simulation_id`, `step`, `error`, `session_dirty` |

These events flow through the existing `logging` package at `LevelDebug` (complete/start) and `LevelError` (failure). No new infrastructure required.

---

## Full Response Contract

```json
{
  "errors_introduced": [
    { "line": 42, "col": 5, "message": "cannot use string as int", "severity": "error" }
  ],
  "errors_resolved": [],
  "net_delta": 1,
  "scope": "file",
  "confidence": "high",
  "timeout": false,
  "duration_ms": 412,
  "simulation_id": "a3f2-..."
}
```

`simulation_id` is a UUID generated at handler entry. Aids tracing across log events and agent reasoning ("simulation a3f2 showed +1 error at line 42").

`confidence` values:
- `"high"` — single-file, diagnostics settled within timeout
- `"partial"` — timed out, returned snapshot may be incomplete
- `"stale"` — concurrent mutation detected during simulation window
- `"eventual"` — workspace scope, cross-file propagation may be incomplete

---

## Implementation Notes

**Tool handler structure:**

```go
func HandleSimulateEdit(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
    // 1. Validate args (file_path, range, new_text)
    // 2. ValidateFilePath
    // 3. Get current diagnostics (baseline)
    // 4. Acquire simulation lock
    // 5. Apply textDocument/didChange (in-memory only, not disk)
    // 6. WaitForDiagnostics (with timeout)
    // 7. Get new diagnostics
    // 8. Diff old vs new
    // 9. Push revert didChange (restore original content)
    // 10. WaitForDiagnostics (confirm revert settled)
    // 11. Release lock
    // 12. Return SimulateEditResult
}
```

**Key: the revert in step 9 must always run, even if step 6-8 fail.** Use defer.

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

Version must increment on each change. Track version per open document on `LSPClient`.

**Revert:** Apply the inverse range edit (original text for the same range).

---

## Open Questions Before Scouting

1. **Document version tracking** — `LSPClient` currently doesn't track `textDocument/didChange` version numbers. Need to add version counter per open document, incremented on each mutation.

2. **Baseline diagnostic timing** — should we call `WaitForDiagnostics` before pushing the edit to ensure we have a stable baseline? Or trust whatever diagnostics are currently cached? Recommend: use cached diagnostics as baseline (avoids additional wait), document the assumption.

3. **Revert reliability** — if the client crashes between push and revert, the language server holds mutated state until it's restarted. Acceptable for V1; V2 should add a cleanup hook on `LSPClient.Shutdown`.

4. **Scope parameter** — default to `"file"` in V1, add `"workspace"` as opt-in in V1.5. Don't expose workspace scope until cross-server timing is validated.
