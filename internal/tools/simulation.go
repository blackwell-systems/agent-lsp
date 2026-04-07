package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/lsp-mcp-go/internal/session"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// HandleCreateSimulationSession creates a new isolated simulation session.
func HandleCreateSimulationSession(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	workspaceRoot, ok := args["workspace_root"].(string)
	if !ok || workspaceRoot == "" {
		return types.ErrorResult("workspace_root is required"), nil
	}

	language, ok := args["language"].(string)
	if !ok || language == "" {
		return types.ErrorResult("language is required"), nil
	}

	// Validate path safety
	_, err := ValidateFilePath(workspaceRoot, "")
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid workspace_root: %s", err)), nil
	}

	// Create session
	sessionID, err := mgr.CreateSession(ctx, workspaceRoot, language)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("create_session failed: %s", err)), nil
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"status":     "created",
	}
	data, _ := json.Marshal(result)
	return types.TextResult(string(data)), nil
}

// HandleSimulateEdit applies a single edit to a session without evaluating.
func HandleSimulateEdit(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return types.ErrorResult("session_id is required"), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	// Extract and validate range
	rng, err := extractRange(args)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid range: %s", err)), nil
	}

	newText, ok := args["new_text"].(string)
	if !ok {
		return types.ErrorResult("new_text is required"), nil
	}

	// Convert file path to URI
	fileURI := CreateFileURI(filePath)

	// Apply edit
	editResult, err := mgr.ApplyEdit(ctx, sessionID, fileURI, rng, newText)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("simulate_edit failed: %s", err)), nil
	}

	data, _ := json.Marshal(editResult)
	return types.TextResult(string(data)), nil
}

// HandleEvaluateSession evaluates the current state of a session.
func HandleEvaluateSession(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return types.ErrorResult("session_id is required"), nil
	}

	scope := "file"
	if v, ok := args["scope"].(string); ok && v != "" {
		scope = v
	}

	timeoutMs := 0
	if _, ok := args["timeout_ms"]; ok {
		if timeoutInt, err := toInt(args, "timeout_ms"); err == nil {
			timeoutMs = timeoutInt
		}
	}

	// Evaluate
	evalResult, err := mgr.Evaluate(ctx, sessionID, scope, timeoutMs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("evaluate_session failed: %s", err)), nil
	}

	data, _ := json.Marshal(evalResult)
	return types.TextResult(string(data)), nil
}

// HandleSimulateChain applies a sequence of edits and evaluates after each step.
func HandleSimulateChain(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return types.ErrorResult("session_id is required"), nil
	}

	editsRaw, ok := args["edits"].([]interface{})
	if !ok || len(editsRaw) == 0 {
		return types.ErrorResult("edits array is required and must not be empty"), nil
	}

	// Parse edits
	chainEdits := make([]session.ChainEdit, 0, len(editsRaw))
	for i, editRaw := range editsRaw {
		editMap, ok := editRaw.(map[string]interface{})
		if !ok {
			return types.ErrorResult(fmt.Sprintf("edit[%d] must be an object", i)), nil
		}

		filePath, ok := editMap["file_path"].(string)
		if !ok || filePath == "" {
			return types.ErrorResult(fmt.Sprintf("edit[%d]: file_path is required", i)), nil
		}

		// Extract range from edit object
		rng, err := extractRange(editMap)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("edit[%d]: invalid range: %s", i, err)), nil
		}

		newText, ok := editMap["new_text"].(string)
		if !ok {
			return types.ErrorResult(fmt.Sprintf("edit[%d]: new_text is required", i)), nil
		}

		chainEdits = append(chainEdits, session.ChainEdit{
			FileURI: CreateFileURI(filePath),
			Range:   rng,
			NewText: newText,
		})
	}

	timeoutMs := 0
	if _, ok := args["timeout_ms"]; ok {
		if timeoutInt, err := toInt(args, "timeout_ms"); err == nil {
			timeoutMs = timeoutInt
		}
	}

	// Simulate chain
	chainResult, err := mgr.SimulateChain(ctx, sessionID, chainEdits, timeoutMs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("simulate_chain failed: %s", err)), nil
	}

	data, _ := json.Marshal(chainResult)
	return types.TextResult(string(data)), nil
}

// HandleCommitSession commits session changes to disk or returns a patch.
func HandleCommitSession(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return types.ErrorResult("session_id is required"), nil
	}

	target := ""
	if v, ok := args["target"].(string); ok {
		target = v
	}

	apply := false
	if v, ok := args["apply"].(bool); ok {
		apply = v
	}

	// Commit
	commitResult, err := mgr.Commit(ctx, sessionID, target, apply)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("commit_session failed: %s", err)), nil
	}

	data, _ := json.Marshal(commitResult)
	return types.TextResult(string(data)), nil
}

// HandleDiscardSession discards all session changes without committing.
func HandleDiscardSession(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return types.ErrorResult("session_id is required"), nil
	}

	// Discard
	err := mgr.Discard(ctx, sessionID)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("discard_session failed: %s", err)), nil
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"status":     "discarded",
	}
	data, _ := json.Marshal(result)
	return types.TextResult(string(data)), nil
}

// HandleDestroySession destroys a session and releases all resources.
func HandleDestroySession(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return types.ErrorResult("session_id is required"), nil
	}

	// Destroy
	err := mgr.Destroy(ctx, sessionID)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("destroy_session failed: %s", err)), nil
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"status":     "destroyed",
	}
	data, _ := json.Marshal(result)
	return types.TextResult(string(data)), nil
}

// HandleSimulateEditAtomic creates a session, applies an edit, evaluates, and destroys atomically.
func HandleSimulateEditAtomic(ctx context.Context, mgr *session.SessionManager, args map[string]interface{}) (types.ToolResult, error) {
	// Extract workspace_root
	workspaceRoot, ok := args["workspace_root"].(string)
	if !ok || workspaceRoot == "" {
		return types.ErrorResult("workspace_root is required"), nil
	}

	// Extract language
	language, ok := args["language"].(string)
	if !ok || language == "" {
		return types.ErrorResult("language is required"), nil
	}

	// Extract file_path
	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	// Validate file path
	_, err := ValidateFilePath(filePath, "")
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid file_path: %s", err)), nil
	}

	// Extract range
	rng, err := extractRange(args)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid range: %s", err)), nil
	}

	// Extract new_text
	newText, ok := args["new_text"].(string)
	if !ok {
		return types.ErrorResult("new_text is required"), nil
	}

	// Optional scope and timeout
	scope := "file"
	if v, ok := args["scope"].(string); ok && v != "" {
		scope = v
	}

	timeoutMs := 0
	if _, ok := args["timeout_ms"]; ok {
		if timeoutInt, err := toInt(args, "timeout_ms"); err == nil {
			timeoutMs = timeoutInt
		}
	}

	// Create session
	sessionID, err := mgr.CreateSession(ctx, workspaceRoot, language)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("create_session failed: %s", err)), nil
	}
	defer mgr.Destroy(ctx, sessionID)

	// Apply edit
	fileURI := CreateFileURI(filePath)
	_, err = mgr.ApplyEdit(ctx, sessionID, fileURI, rng, newText)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("apply_edit failed: %s", err)), nil
	}

	// Evaluate
	evalResult, err := mgr.Evaluate(ctx, sessionID, scope, timeoutMs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("evaluate failed: %s", err)), nil
	}

	// Discard to revert LSP state before Destroy — ensures gopls sees clean
	// file content for subsequent calls, not the modified in-memory version.
	_ = mgr.Discard(ctx, sessionID)

	data, _ := json.Marshal(evalResult)
	return types.TextResult(string(data)), nil
}
