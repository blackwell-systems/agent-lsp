// tools_session.go defines MCP tool registrations for speculative execution:
// create_simulation_session, simulate_edit, simulate_edit_atomic,
// simulate_chain, evaluate_session, commit_session, discard_session,
// and destroy_session.
//
// Speculative execution lets agents preview edits in memory before writing
// to disk. A session snapshots the LSP state, applies edits virtually,
// collects diagnostics, and reports the net error delta. If net_delta == 0,
// the edit is safe to commit; otherwise, the agent can discard and retry.
//
// Sessions are managed by internal/session.SessionManager and are isolated
// from each other and from the live workspace state.
package main

import (
	"context"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Simulation/session tool arg types.

type CreateSimulationSessionArgs struct {
	WorkspaceRoot string `json:"workspace_root" jsonschema:"Workspace root directory for the simulation session"`
	Language      string `json:"language" jsonschema:"Language identifier for the session (e.g. go, typescript)"`
}

type SimulateEditArgs struct {
	SessionID   string `json:"session_id" jsonschema:"Session identifier returned by create_simulation_session"`
	FilePath    string `json:"file_path" jsonschema:"Absolute path to the file to edit within the session"`
	StartLine   int    `json:"start_line" jsonschema:"1-indexed start line of the range to replace"`
	StartColumn int    `json:"start_column" jsonschema:"1-indexed start column of the range to replace"`
	EndLine     int    `json:"end_line" jsonschema:"1-indexed end line of the range to replace"`
	EndColumn   int    `json:"end_column" jsonschema:"1-indexed end column of the range to replace"`
	NewText     string `json:"new_text" jsonschema:"Replacement text for the specified range"`
}

type EvaluateSessionArgs struct {
	SessionID string `json:"session_id" jsonschema:"Session identifier returned by create_simulation_session"`
	Scope     string `json:"scope,omitempty" jsonschema:"Evaluation scope: file (fast, single file) or workspace (full, all files). Default: file"`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"Timeout in milliseconds for LSP diagnostics collection. Default: 5000"`
}

type SimulateChainArgs struct {
	SessionID string                   `json:"session_id" jsonschema:"Session identifier returned by create_simulation_session"`
	Edits     []map[string]interface{} `json:"edits" jsonschema:"Array of edit objects, each with file_path, start_line, start_column, end_line, end_column, new_text"`
	TimeoutMs int                      `json:"timeout_ms,omitempty" jsonschema:"Timeout in milliseconds for LSP diagnostics collection. Default: 5000"`
}

type CommitSessionArgs struct {
	SessionID string `json:"session_id" jsonschema:"Session identifier returned by create_simulation_session"`
	Target    string `json:"target,omitempty" jsonschema:"Commit target: disk (write files) or patch (return unified diff). Default: patch"`
	Apply     bool   `json:"apply,omitempty" jsonschema:"If true, write changes to disk and notify LSP. If false, return diff only"`
}

type DiscardSessionArgs struct {
	SessionID string `json:"session_id" jsonschema:"Session identifier returned by create_simulation_session"`
}

type DestroySessionArgs struct {
	SessionID string `json:"session_id" jsonschema:"Session identifier returned by create_simulation_session"`
}

type SimulateEditAtomicArgs struct {
	SessionID     string `json:"session_id,omitempty" jsonschema:"Session identifier returned by create_simulation_session"`
	WorkspaceRoot string `json:"workspace_root,omitempty" jsonschema:"Workspace root directory for the simulation session"`
	Language      string `json:"language,omitempty" jsonschema:"Language identifier for the session (e.g. go, typescript)"`
	FilePath      string `json:"file_path" jsonschema:"Absolute path to the file to edit within the session"`
	StartLine     int    `json:"start_line" jsonschema:"1-indexed start line of the range to replace"`
	StartColumn   int    `json:"start_column" jsonschema:"1-indexed start column of the range to replace"`
	EndLine       int    `json:"end_line" jsonschema:"1-indexed end line of the range to replace"`
	EndColumn     int    `json:"end_column" jsonschema:"1-indexed end column of the range to replace"`
	NewText       string `json:"new_text" jsonschema:"Replacement text for the specified range"`
	Scope         string `json:"scope,omitempty" jsonschema:"Evaluation scope: file (fast, single file) or workspace (full, all files). Default: file"`
	TimeoutMs     int    `json:"timeout_ms,omitempty" jsonschema:"Timeout in milliseconds for LSP diagnostics collection. Default: 5000"`
}

func registerSessionTools(d toolDeps) {
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "create_simulation_session",
		Description: "Create a new speculative code session for simulating edits without committing to disk. Returns a session ID. Baseline diagnostics are captured lazily on first edit per file. Use this to explore what-if scenarios before applying changes.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Simulation Session",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateSimulationSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCreateSimulationSession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "simulate_edit",
		Description: "Apply a range edit to a file within a simulation session. Changes are held in-memory only. The session captures baseline diagnostics on first edit to each file, then tracks versions for subsequent edits. Returns the new version number after the edit. All line/column positions are 1-indexed (matching editor line numbers).",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Simulate Edit",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateEditArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateEdit(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "evaluate_session",
		Description: "Evaluate a simulation session by comparing current diagnostics against baselines. Returns errors introduced, errors resolved, net delta, and confidence (high for file scope, eventual for workspace). Use after simulate_edit to assess impact before committing.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Evaluate Session",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EvaluateSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleEvaluateSession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "simulate_chain",
		Description: "Apply a sequence of edits and evaluate after each step. Returns per-step diagnostics and identifies the safe-to-apply-through step (last step with net delta == 0). Use this to find the safest partial application of a multi-step change. All line/column positions in each edit are 1-indexed.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Simulate Chain",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateChainArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateChain(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "commit_session",
		Description: "Commit a simulation session. With apply=true, writes changes to disk and notifies LSP servers. With apply=false, returns a unified diff patch. Use after evaluate_session confirms the changes are safe.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Commit Session",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CommitSessionArgs) (*mcp.CallToolResult, any, error) {
		startTime := time.Now()
		client := d.cs.get()

		diagsBefore := snapshotAllDiagnostics(client)

		r, err := tools.HandleCommitSession(ctx, d.sessionMgr, toolArgsToMap(args))

		diagsAfter := snapshotAllDiagnostics(client)
		delta := computeDelta(diagsBefore, diagsAfter)

		var filesChecked []string
		if diagsAfter != nil {
			filesChecked = diagsAfter.FilesChecked
		}

		record := audit.Record{
			Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
			Tool:              "commit_session",
			SessionID:         args.SessionID,
			Files:             filesChecked,
			EditSummary:       &audit.EditSummary{
				Mode:   "commit",
				Target: args.Target,
				Apply:  args.Apply,
			},
			DiagnosticsBefore: diagsBefore,
			DiagnosticsAfter:  diagsAfter,
			NetDelta:          delta,
			Success:           !isToolResultError(r),
			DurationMs:        time.Since(startTime).Milliseconds(),
		}
		if isToolResultError(r) {
			record.ErrorMessage = toolResultErrorMsg(r)
		}
		d.auditLogger.Log(record)

		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "discard_session",
		Description: "Discard a simulation session and revert all in-memory changes by restoring baseline content. Use when simulation results show the changes would introduce errors.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Discard Session",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DiscardSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDiscardSession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "destroy_session",
		Description: "Destroy a simulation session and release all resources. Call this after commit or discard to clean up. Sessions in terminal states (committed, discarded, destroyed) cannot be reused.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Destroy Session",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DestroySessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDestroySession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "simulate_edit_atomic",
		Description: "One-shot atomic operation: create session, apply edit, evaluate, and destroy. Returns evaluation result. Use for quick what-if checks without managing session lifecycle manually. Requires start_lsp to be called first. All line/column positions are 1-indexed. net_delta: 0 means the edit is safe to apply.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Simulate Edit (Atomic)",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateEditAtomicArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateEditAtomic(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
