// tools_phase.go defines MCP tool registrations for skill phase enforcement:
// activate_skill, deactivate_skill, and get_skill_phase.
//
// Phase enforcement tracks which phase a skill workflow is in (e.g. "preview"
// vs "execute" for lsp-rename) and blocks tool calls that violate the current
// phase's permissions. This prevents agents from applying edits before
// completing blast-radius analysis, or committing before verifying diagnostics.
//
// These tools are registered directly on the MCP server (not via
// addToolWithPhaseCheck) because they control the phase tracker itself.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/agent-lsp/internal/phase"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Phase enforcement tool arg types.

type ActivateSkillArgs struct {
	SkillName string `json:"skill_name" jsonschema:"Name of the skill to activate (e.g. lsp-rename, lsp-refactor, lsp-safe-edit, lsp-verify)"`
	Mode      string `json:"mode,omitempty" jsonschema:"Enforcement mode: warn (log violations but allow) or block (return error with recovery guidance). Default: warn"`
}

type DeactivateSkillArgs struct{}

type GetSkillPhaseArgs struct{}

func registerPhaseTools(d toolDeps) {
	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "activate_skill",
		Description: "Activate phase enforcement for a skill workflow. Once active, tool calls are checked against the skill's phase permissions. Phases advance automatically as you call tools from later phases. Use this at the start of a skill workflow to enable safety guardrails that prevent out-of-order operations (e.g., applying edits before completing blast-radius analysis).",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Activate Skill Phase Enforcement",
			ReadOnlyHint:    false,
			IdempotentHint:  false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ActivateSkillArgs) (*mcp.CallToolResult, any, error) {
		if d.phaseTracker == nil {
			return makeCallToolResult(types.ErrorResult("phase enforcement is not initialized")), nil, nil
		}

		mode := phase.ModeWarn
		if args.Mode == "block" {
			mode = phase.ModeBlock
		}

		if err := d.phaseTracker.ActivateSkill(args.SkillName, mode); err != nil {
			return makeCallToolResult(types.ErrorResult(err.Error())), nil, nil
		}

		status := d.phaseTracker.Status()
		data, _ := json.Marshal(map[string]any{
			"status":          "activated",
			"skill":           status.SkillName,
			"mode":            status.Mode,
			"current_phase":   status.CurrentPhase,
			"total_phases":    status.TotalPhases,
			"allowed_tools":   status.AllowedTools,
			"forbidden_tools": status.ForbiddenTools,
		})
		return makeCallToolResult(types.TextResult(string(data))), nil, nil
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "deactivate_skill",
		Description: "Deactivate phase enforcement for the currently active skill. Tool calls will no longer be checked against phase permissions. Call this when the skill workflow is complete or when you need to exit the workflow early.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Deactivate Skill Phase Enforcement",
			ReadOnlyHint:    false,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DeactivateSkillArgs) (*mcp.CallToolResult, any, error) {
		if d.phaseTracker == nil {
			return makeCallToolResult(types.ErrorResult("phase enforcement is not initialized")), nil, nil
		}

		d.phaseTracker.DeactivateSkill()
		return makeCallToolResult(types.TextResult(`{"status":"deactivated"}`)), nil, nil
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_skill_phase",
		Description: "Get the current state of skill phase enforcement: active skill, current phase, allowed and forbidden tools, and tool call history. Use this to understand where you are in a skill workflow and what tools are available.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Skill Phase Status",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSkillPhaseArgs) (*mcp.CallToolResult, any, error) {
		if d.phaseTracker == nil {
			return makeCallToolResult(types.ErrorResult("phase enforcement is not initialized")), nil, nil
		}

		status := d.phaseTracker.Status()
		if !status.Active {
			skills := d.phaseTracker.AvailableSkills()
			data, _ := json.Marshal(map[string]any{
				"active":           false,
				"available_skills": skills,
			})
			return makeCallToolResult(types.TextResult(string(data))), nil, nil
		}

		data, _ := json.Marshal(status)
		return makeCallToolResult(types.TextResult(string(data))), nil, nil
	})
}

// checkPhasePermission checks if a tool call is permitted under the current
// phase enforcement. Returns a non-nil *mcp.CallToolResult if the call should
// be blocked (ModeBlock and violation detected). Returns nil if the call is
// allowed or if phase enforcement is inactive.
func checkPhasePermission(tracker *phase.Tracker, toolName string) *mcp.CallToolResult {
	if tracker == nil {
		return nil
	}

	violation := tracker.CheckAndRecord(toolName)
	if violation == nil {
		return nil
	}

	if !violation.Blocked {
		// ModeWarn: violation is logged by the tracker; let the call proceed.
		return nil
	}

	// ModeBlock: return an error with structured recovery guidance.
	data, _ := json.Marshal(map[string]any{
		"error":         "phase_violation",
		"tool":          violation.ToolName,
		"skill":         violation.SkillName,
		"current_phase": violation.CurrentPhase,
		"reason":        violation.Reason,
		"recovery":      violation.Recovery,
	})
	msg := fmt.Sprintf("Phase violation: %s\n\nRecovery: %s\n\n%s",
		violation.Reason, violation.Recovery, string(data))
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}
