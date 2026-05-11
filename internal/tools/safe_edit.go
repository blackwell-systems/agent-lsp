// safe_edit.go implements the safe_apply_edit MCP tool handler.
//
// HandleSafeApplyEdit combines preview_edit (speculative simulation) with
// apply_edit into a single call. It previews the edit first; if net_delta == 0
// (no new diagnostics introduced), it applies the edit to disk and returns
// applied=true. If net_delta > 0, it returns the preview result with
// applied=false so the caller can decide how to proceed.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/session"
	"github.com/blackwell-systems/agent-lsp/pkg/types"
)

// HandleSafeApplyEdit combines preview_edit + apply_edit when net_delta == 0.
// Returns applied=true on success or applied=false with preview diagnostics
// when net_delta > 0.
func HandleSafeApplyEdit(ctx context.Context, client *lsp.LSPClient, sessionMgr *session.SessionManager, args map[string]any) (types.ToolResult, error) {
	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	oldText, ok := args["old_text"].(string)
	if !ok || oldText == "" {
		return types.ErrorResult("old_text is required"), nil
	}

	newText, ok := args["new_text"].(string)
	if !ok {
		return types.ErrorResult("new_text is required"), nil
	}

	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Validate file path.
	filePath, err := ValidateFilePath(filePath, "")
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid file_path: %s", err)), nil
	}

	// Read the file and locate old_text to compute positional range.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("reading %s: %s", filePath, err)), nil
	}
	src := string(content)

	idx := strings.Index(src, oldText)
	if idx < 0 {
		return types.ErrorResult(fmt.Sprintf("old_text not found in %s", filePath)), nil
	}

	// Compute 0-based line/column from byte offset.
	before := src[:idx]
	startLine := strings.Count(before, "\n")
	startCol := len(before) - strings.LastIndex(before, "\n") - 1
	if !strings.Contains(before, "\n") {
		startCol = len(before)
	}

	segment := src[idx : idx+len(oldText)]
	endLine := startLine + strings.Count(segment, "\n")
	var endCol int
	if lastNL := strings.LastIndex(segment, "\n"); lastNL < 0 {
		endCol = startCol + len(segment)
	} else {
		endCol = len(segment) - lastNL - 1
	}

	// Build args for HandleSimulateEditAtomic.
	workspaceRoot := client.RootDir()
	language := lsp.LanguageIDFromPath(filePath)

	simArgs := map[string]any{
		"workspace_root": workspaceRoot,
		"language":       language,
		"file_path":      filePath,
		"start_line":     startLine,
		"start_column":   startCol,
		"end_line":       endLine,
		"end_column":     endCol,
		"new_text":       newText,
	}

	simResult, err := HandleSimulateEditAtomic(ctx, sessionMgr, simArgs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("preview failed: %s", err)), nil
	}
	if simResult.IsError {
		return simResult, nil
	}

	// Parse the preview result to extract net_delta.
	var previewJSON map[string]any
	if len(simResult.Content) > 0 {
		// The content text may have an appended hint line; parse only the JSON portion.
		text := simResult.Content[0].Text
		if err := json.Unmarshal([]byte(text), &previewJSON); err != nil {
			// Try to find the JSON object boundary (ignore trailing hint text).
			if braceIdx := strings.LastIndex(text, "}"); braceIdx >= 0 {
				_ = json.Unmarshal([]byte(text[:braceIdx+1]), &previewJSON)
			}
		}
	}

	if previewJSON == nil {
		return types.ErrorResult("failed to parse preview result"), nil
	}

	netDelta := 0
	if nd, ok := previewJSON["net_delta"].(float64); ok {
		netDelta = int(nd)
	}

	if netDelta > 0 {
		// Unsafe: return preview result with applied=false.
		previewJSON["applied"] = false
		data, mErr := json.Marshal(previewJSON)
		if mErr != nil {
			return types.ErrorResult(fmt.Sprintf("marshaling result: %s", mErr)), nil
		}
		return appendHint(types.TextResult(string(data)), "Edit would introduce errors. Review diagnostics before applying."), nil
	}

	// Safe: apply the edit.
	applyArgs := map[string]any{
		"file_path": filePath,
		"old_text":  oldText,
		"new_text":  newText,
	}
	applyResult, err := HandleApplyEdit(ctx, client, applyArgs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("apply failed: %s", err)), nil
	}
	if applyResult.IsError {
		return applyResult, nil
	}

	// Build a combined result.
	previewJSON["applied"] = true
	data, mErr := json.Marshal(previewJSON)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling result: %s", mErr)), nil
	}
	return appendHint(types.TextResult(string(data)), "Edit applied successfully (net_delta == 0)."), nil
}
