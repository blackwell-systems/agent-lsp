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
