# Refactor Preview with simulate_chain

`simulate_chain` chains in-memory edits and evaluates diagnostics after each step, making it ideal for safe refactor preview before committing. The key signal is `cumulative_delta == 0` at the final step: this means the refactor is safe to commit. If `cumulative_delta` is non-zero at the end, the chain has unresolved errors — investigate which step first showed `net_delta > 0` before discarding.

---

## Prerequisites

Call `start_lsp` with `root_dir` set to your workspace before using any simulation tools.

```json
{ "root_dir": "/your/workspace" }
```

---

## Safe rename preview

Rename a function definition and all its call sites in a single chain. Because `simulate_chain` evaluates after each step, you know exactly which step (if any) introduces errors.

**Step 1: Create a session**

```
create_simulation_session(
  workspace_root="/your/workspace",
  language="go"
)
→ { "session_id": "<session_id>" }
```

**Step 2: Chain definition rename + call-site rename**

```
simulate_chain(
  session_id="<session_id>",
  steps=[
    {
      "file_path": "/your/workspace/pkg/handler.go",
      "start_line": <line>, "start_column": <column>,
      "end_line": <line>,   "end_column": <column>,
      "new_text": "HandleRequestV2"
    },
    {
      "file_path": "/your/workspace/pkg/router.go",
      "start_line": <line>, "start_column": <column>,
      "end_line": <line>,   "end_column": <column>,
      "new_text": "HandleRequestV2"
    }
  ]
)
→ {
    "steps": [
      { "step": 1, "net_delta": 1, "cumulative_delta": 1, "errors_introduced": [...] },
      { "step": 2, "net_delta": -1, "cumulative_delta": 0, "errors_introduced": [] }
    ],
    "safe_through_step": 2,
    "cumulative_delta": 0
  }
```

**Step 3: Check the final cumulative_delta**

If `steps[-1].cumulative_delta == 0`, the rename is clean — all introduced errors were resolved by subsequent steps.

**Step 4: Commit or discard**

```
# cumulative_delta == 0 → safe to commit
commit_session(session_id="<session_id>", apply=true)

# cumulative_delta != 0 → discard and investigate
discard_session(session_id="<session_id>")
# Inspect steps to find the first step where net_delta > 0 — that is where errors remain.
```

---

## Change impact preview

Apply a speculative interface change and observe which steps introduce errors, without ever writing to disk. This is a read-only workflow — `commit_session` is never called.

**Step 1: Create a session**

```
create_simulation_session(
  workspace_root="/your/workspace",
  language="go"
)
→ { "session_id": "<session_id>" }
```

**Step 2: Apply the speculative interface change in step 1**

```
simulate_chain(
  session_id="<session_id>",
  steps=[
    {
      "file_path": "/your/workspace/pkg/types.go",
      "start_line": <line>, "start_column": <column>,
      "end_line": <line>,   "end_column": <column>,
      "new_text": "Process(ctx context.Context, req *NewRequest) (*Response, error)"
    }
  ]
)
→ {
    "steps": [
      { "step": 1, "net_delta": 4, "cumulative_delta": 4, "errors_introduced": [
          { "line": 88, "col": 3, "message": "cannot use *OldRequest as *NewRequest", "severity": "error" },
          ...
        ]
      }
    ],
    "safe_through_step": 0,
    "cumulative_delta": 4
  }
```

**Step 3: Identify call sites that need updating**

Read `steps[0].errors_introduced` to enumerate every call site that needs updating. Each entry includes the file, line, column, and diagnostic message — enough to target follow-up edits.

**Step 4: Discard (never commit)**

```
discard_session(session_id="<session_id>")
```

The workspace is untouched. Use the error list as a refactoring plan.

---

## Multi-file refactor with checkpoint

Chain edits across three files and use `safe_through_step` to commit only the clean prefix if the full chain does not resolve.

**Step 1: Create a session**

```
create_simulation_session(
  workspace_root="/your/workspace",
  language="go"
)
→ { "session_id": "<session_id>" }
```

**Step 2: Chain 3 edits across 3 files**

```
simulate_chain(
  session_id="<session_id>",
  steps=[
    {
      "file_path": "/your/workspace/pkg/service.go",
      "start_line": <line>, "start_column": <column>,
      "end_line": <line>,   "end_column": <column>,
      "new_text": "func (s *Service) Run(ctx context.Context) error {"
    },
    {
      "file_path": "/your/workspace/pkg/worker.go",
      "start_line": <line>, "start_column": <column>,
      "end_line": <line>,   "end_column": <column>,
      "new_text": "if err := s.service.Run(ctx); err != nil {"
    },
    {
      "file_path": "/your/workspace/cmd/main.go",
      "start_line": <line>, "start_column": <column>,
      "end_line": <line>,   "end_column": <column>,
      "new_text": "if err := svc.Run(ctx); err != nil {"
    }
  ]
)
→ {
    "steps": [
      { "step": 1, "net_delta": 0, "cumulative_delta": 0, "errors_introduced": [] },
      { "step": 2, "net_delta": 0, "cumulative_delta": 0, "errors_introduced": [] },
      { "step": 3, "net_delta": 2, "cumulative_delta": 2, "errors_introduced": [...] }
    ],
    "safe_through_step": 2,
    "cumulative_delta": 2
  }
```

**Step 3: Use safe_through_step as a checkpoint**

`safe_through_step: 2` means steps 1 and 2 are clean. Step 3 introduced unresolved errors.

If you want to commit only the clean prefix, discard this session, create a new one, and chain only steps 1–2:

```
discard_session(session_id="<session_id>")

create_simulation_session(workspace_root="/your/workspace", language="go")
→ { "session_id": "<session_id_2>" }

simulate_chain(session_id="<session_id_2>", steps=[<step_1>, <step_2>])

commit_session(session_id="<session_id_2>", apply=true)
```

Then fix step 3 in a follow-up session once the partial commit is in place.

---

## Key response fields

| Field | Meaning |
|-------|---------|
| `steps[i].net_delta` | Error count change after step i. Negative means errors were resolved; positive means errors were introduced. |
| `steps[i].cumulative_delta` | Total error delta from baseline through step i. Zero means the chain is clean up to and including this step. |
| `safe_through_step` | Index of the last step where `cumulative_delta == 0`. Steps up to and including this index are safe to commit. |
| `commit_session(apply=true)` | Writes all edits to disk atomically. |
| `commit_session(apply=false)` | Returns a diff (WorkspaceEdit patch) without writing to disk — useful for inspection before apply. |
