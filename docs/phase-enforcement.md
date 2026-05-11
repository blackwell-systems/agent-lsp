# Skill Phase Enforcement

**Status:** Shipped. 3 tools, 4 skills with phase configs, 17 unit tests.
**Tools:** `activate_skill`, `deactivate_skill`, `get_skill_phase`
**Skills with enforcement:** `lsp-rename` (3 phases), `lsp-refactor` (5 phases), `lsp-safe-edit` (4 phases), `lsp-verify` (5 phases)

---

## The Problem

Skills encode correct multi-step workflows ("analyze impact before editing", "simulate before writing to disk"). But encoding is not enforcement. An agent following `/lsp-refactor` can call `apply_edit` in the blast-radius phase, bypassing the safety gate that exists to prevent uninformed edits. The skill prose says "do not apply yet"; the runtime does nothing to stop it.

This matters because the most dangerous agent failures are ordering violations: applying edits before understanding blast radius, writing to disk before simulating, running tests before building. These produce correct-looking output that silently skips safety steps.

---

## How It Works

Phase enforcement is a runtime state machine that sits in front of every tool handler. When an agent activates a skill, the tracker monitors incoming tool calls and enforces the phase ordering defined in the skill's `tool_permissions` metadata.

```
Agent calls activate_skill("lsp-rename", "block")
    -> tracker enters phase 1: "prerequisites"

Agent calls start_lsp(...)
    -> allowed (in phase 1 allowed list)

Agent calls go_to_symbol(...)
    -> auto-advance to phase 2: "preview" (go_to_symbol is in phase 2's allowed list)

Agent calls prepare_rename(...)
    -> allowed (in current phase)

Agent calls apply_edit(...)
    -> BLOCKED: "apply_edit is forbidden in the preview phase"
    -> recovery: "Complete the preview phase first. Allowed tools: [go_to_symbol, prepare_rename, find_references, rename_symbol]"

Agent calls get_diagnostics(...)
    -> auto-advance to phase 3: "execute" (get_diagnostics is in phase 3's allowed list)

Agent calls apply_edit(...)
    -> now allowed
```

---

## Quick Start

Activate enforcement at the start of any supported skill workflow:

```
activate_skill(skill_name="lsp-rename", mode="warn")
```

Check your current phase at any time:

```
get_skill_phase()

-> {
     "active": true,
     "skill_name": "lsp-rename",
     "current_phase": "preview",
     "phase_index": 1,
     "total_phases": 3,
     "mode": "warn",
     "allowed_tools": ["go_to_symbol", "prepare_rename", "find_references", "rename_symbol"],
     "forbidden_tools": ["apply_edit", "Edit", "Write", "format_document", "run_tests"],
     "tool_history": ["start_lsp", "go_to_symbol"]
   }
```

Deactivate when the workflow is complete:

```
deactivate_skill()
```

---

## Enforcement Modes

| Mode | Behavior | When to use |
|------|----------|-------------|
| `warn` | Logs the violation, allows the call to proceed | Default. Learning mode; see violations in logs without breaking the workflow. |
| `block` | Returns an error with recovery guidance; tool call does not execute | Production safety. Prevents ordering violations from reaching the tool handler. |

In `block` mode, the error response includes structured JSON with the violation details and recovery guidance:

```json
{
  "error": "phase_violation",
  "tool": "apply_edit",
  "skill": "lsp-rename",
  "current_phase": "preview",
  "reason": "apply_edit is forbidden in the \"preview\" phase",
  "recovery": "Complete the \"preview\" phase first. Allowed tools: [go_to_symbol, prepare_rename, find_references, rename_symbol]"
}
```

---

## Phase Advancement

Phases advance automatically. There are no explicit "next phase" calls.

**How it works:** When a tool call matches a later phase's allowed list, the tracker advances to that phase. Tools in the current phase's allowed list stay allowed. Tools not mentioned in any phase (e.g., `inspect_symbol`, `get_completions`) pass through without restriction.

**Rules:**

1. A tool matching the current phase's **forbidden** list: BLOCKED (or warned).
2. A tool matching the **global_forbidden** list: BLOCKED regardless of phase.
3. A tool matching the current phase's **allowed** list: allowed, no phase change.
4. A tool matching a later phase's **allowed** list: allowed, phase advances to that phase.
5. A tool not in any phase's allowed or forbidden list: allowed (pass-through for tools outside the skill's scope).

**Phase skipping:** If an agent calls a tool from phase 3 while in phase 1, the tracker skips phase 2 and advances directly to phase 3. This handles cases where some phases are optional (e.g., skipping `start_lsp` when the server is already running).

---

## Supported Skills

### lsp-rename (3 phases)

```
prerequisites -> preview -> execute
```

| Phase | Allowed | Forbidden | Purpose |
|-------|---------|-----------|---------|
| prerequisites | start_lsp | (none) | Initialize LSP if needed |
| preview | go_to_symbol, prepare_rename, find_references, rename_symbol | apply_edit, Edit, Write | Locate symbol, validate, enumerate references |
| execute | get_diagnostics, rename_symbol, apply_edit | simulate_*, run_build | Apply the rename and verify |

**Global forbidden:** format_document, run_tests (rename does not format or test)

---

### lsp-refactor (5 phases)

```
blast_radius -> speculative_preview -> apply -> build_verification -> test_execution
```

| Phase | Allowed | Forbidden | Purpose |
|-------|---------|-----------|---------|
| blast_radius | blast_radius, go_to_symbol, find_references | apply_edit, simulate_*, Edit, Write | Analyze impact before any edits |
| speculative_preview | open_document, get_diagnostics, preview_edit, simulate_chain | apply_edit, Edit, Write | Simulate edits in memory |
| apply | apply_edit, format_document, Edit, Write | simulate_*, rename_symbol | Write changes to disk |
| build_verification | get_diagnostics, run_build | apply_edit, Edit, Write | Check the build |
| test_execution | get_tests_for_file, run_tests | apply_edit, Edit, Write | Run affected tests |

**Global forbidden:** rename_symbol (refactor uses direct edits, not rename)

**Key safety property:** `apply_edit` is forbidden in `blast_radius` and `speculative_preview`. The agent cannot write to disk until it has both analyzed impact and simulated the change.

---

### lsp-safe-edit (4 phases)

```
setup -> speculative_preview -> apply -> verify_and_fix
```

| Phase | Allowed | Forbidden | Purpose |
|-------|---------|-----------|---------|
| setup | start_lsp, open_document, get_diagnostics | apply_edit, Edit, Write | Capture baseline diagnostics |
| speculative_preview | preview_edit, simulate_chain | apply_edit, Edit, Write | Simulate before touching disk |
| apply | apply_edit, Edit, Write | simulate_* | Write the change |
| verify_and_fix | get_diagnostics, suggest_fixes, apply_edit, format_document | simulate_*, run_build, run_tests | Post-edit verification and fixes |

**Global forbidden:** rename_symbol, blast_radius

---

### lsp-verify (5 phases)

```
test_correlation -> diagnostics -> build -> tests -> fix_and_format
```

| Phase | Allowed | Forbidden | Purpose |
|-------|---------|-----------|---------|
| test_correlation | get_tests_for_file | apply_edit, Edit, Write | Map source to test files |
| diagnostics | start_lsp, get_diagnostics | apply_edit, Edit, Write | Layer 1: LSP diagnostics |
| build | run_build | apply_edit, Edit, Write | Layer 2: compiler build |
| tests | run_tests, Bash | apply_edit, Edit, Write | Layer 3: test suite |
| fix_and_format | suggest_fixes, apply_edit, format_document, get_diagnostics | simulate_*, run_build, run_tests | Apply fixes and format |

**Global forbidden:** simulate_* (verify is post-edit, not speculative), rename_symbol

---

## External Tool Limitations

Phase configs include external tools like `Edit`, `Write`, and `Bash` in their forbidden lists. These are tools provided by the AI agent runtime (e.g., Claude Code), not by agent-lsp. Since those tools bypass MCP entirely, agent-lsp cannot enforce them at runtime.

These entries serve two purposes:
1. **Agent guidance:** `get_skill_phase()` includes them in the forbidden list, so agents see the full picture of what they should not call.
2. **Documentation:** The SKILL.md YAML and this reference document the complete set of ordering constraints.

For full enforcement of external tools, the agent must self-enforce based on the `get_skill_phase()` output.

---

## Audit Trail

Phase events are logged to the JSONL audit trail (when `--audit-log` is configured):

| Event | Logged when |
|-------|-------------|
| `activate_skill` | Agent activates enforcement |
| `deactivate_skill` | Agent deactivates enforcement |
| `phase_advance` | Tracker advances to a new phase |
| `phase_violation` | Tool call violates phase rules (both warn and block modes) |

Each record includes the skill name, current phase, and the tool that triggered the event.

---

## Architecture

Phase enforcement lives in `internal/phase/`:

| File | Purpose |
|------|---------|
| `types.go` | `EnforcementMode`, `PhaseDefinition`, `SkillPhaseConfig`, `PhaseViolation`, `PhaseStatus` |
| `matcher.go` | Glob matching for tool name patterns (trailing `*` wildcard) |
| `tracker.go` | Thread-safe `Tracker` state machine: activate, deactivate, check+record, status |
| `skills.go` | Built-in phase configs for the 4 supported skills |

The tracker is initialized in `cmd/agent-lsp/server.go` and injected into `toolDeps`. Every tool handler is wrapped via the `addToolWithPhaseCheck` generic function, which checks permissions before delegating to the real handler. This wrapper replaced direct `mcp.AddTool` calls so phase enforcement is automatic for all agent-lsp tools.

---

## Adding Phase Enforcement to a New Skill

1. Add `tool_permissions` to the skill's SKILL.md frontmatter (see existing skills for the YAML format).
2. Add a corresponding `SkillPhaseConfig` in `internal/phase/skills.go`.
3. The new skill is automatically available via `activate_skill`.

The tool names in skills.go use the unprefixed form (`apply_edit`, not `mcp__lsp__apply_edit`) because that is what agent-lsp receives in tool call requests. The SKILL.md YAML uses the prefixed form because that is what agents see.
