package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Simulation/session tool arg types.

type CreateSimulationSessionArgs struct {
	WorkspaceRoot string `json:"workspace_root"`
	Language      string `json:"language"`
}

type SimulateEditArgs struct {
	SessionID   string `json:"session_id"`
	FilePath    string `json:"file_path"`
	StartLine   int    `json:"start_line"`
	StartColumn int    `json:"start_column"`
	EndLine     int    `json:"end_line"`
	EndColumn   int    `json:"end_column"`
	NewText     string `json:"new_text"`
}

type EvaluateSessionArgs struct {
	SessionID string `json:"session_id"`
	Scope     string `json:"scope,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

type SimulateChainArgs struct {
	SessionID string        `json:"session_id"`
	Edits     []interface{} `json:"edits"`
	TimeoutMs int           `json:"timeout_ms,omitempty"`
}

type CommitSessionArgs struct {
	SessionID string `json:"session_id"`
	Target    string `json:"target,omitempty"`
	Apply     bool   `json:"apply,omitempty"`
}

type DiscardSessionArgs struct {
	SessionID string `json:"session_id"`
}

type DestroySessionArgs struct {
	SessionID string `json:"session_id"`
}

type SimulateEditAtomicArgs struct {
	SessionID     string `json:"session_id,omitempty"`
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	Language      string `json:"language,omitempty"`
	FilePath      string `json:"file_path"`
	StartLine     int    `json:"start_line"`
	StartColumn   int    `json:"start_column"`
	EndLine       int    `json:"end_line"`
	EndColumn     int    `json:"end_column"`
	NewText       string `json:"new_text"`
	Scope         string `json:"scope,omitempty"`
	TimeoutMs     int    `json:"timeout_ms,omitempty"`
}

func registerSessionTools(d toolDeps) {
	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "create_simulation_session",
		Description: "Create a new speculative code session for simulating edits without committing to disk. Returns a session ID. Baseline diagnostics are captured lazily on first edit per file. Use this to explore what-if scenarios before applying changes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateSimulationSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCreateSimulationSession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "simulate_edit",
		Description: "Apply a range edit to a file within a simulation session. Changes are held in-memory only. The session captures baseline diagnostics on first edit to each file, then tracks versions for subsequent edits. Returns the new version number after the edit. All line/column positions are 1-indexed (matching editor line numbers).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateEditArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateEdit(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "evaluate_session",
		Description: "Evaluate a simulation session by comparing current diagnostics against baselines. Returns errors introduced, errors resolved, net delta, and confidence (high for file scope, eventual for workspace). Use after simulate_edit to assess impact before committing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EvaluateSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleEvaluateSession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "simulate_chain",
		Description: "Apply a sequence of edits and evaluate after each step. Returns per-step diagnostics and identifies the safe-to-apply-through step (last step with net delta == 0). Use this to find the safest partial application of a multi-step change. All line/column positions in each edit are 1-indexed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateChainArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateChain(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "commit_session",
		Description: "Commit a simulation session. With apply=true, writes changes to disk and notifies LSP servers. With apply=false, returns a unified diff patch. Use after evaluate_session confirms the changes are safe.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CommitSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCommitSession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "discard_session",
		Description: "Discard a simulation session and revert all in-memory changes by restoring baseline content. Use when simulation results show the changes would introduce errors.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DiscardSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDiscardSession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "destroy_session",
		Description: "Destroy a simulation session and release all resources. Call this after commit or discard to clean up. Sessions in terminal states (committed, discarded, destroyed) cannot be reused.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DestroySessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDestroySession(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "simulate_edit_atomic",
		Description: "One-shot atomic operation: create session, apply edit, evaluate, and destroy. Returns evaluation result. Use for quick what-if checks without managing session lifecycle manually. Requires start_lsp to be called first. All line/column positions are 1-indexed. net_delta: 0 means the edit is safe to apply.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateEditAtomicArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateEditAtomic(ctx, d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
