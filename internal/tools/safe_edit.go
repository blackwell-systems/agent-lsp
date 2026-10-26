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

	// Locate old_text through the SAME plan builder apply_edit uses (exact
	// match first, then the whitespace-normalised full-line fallback, with the
	// same EOL balancing): the preview previously ran a raw strings.Index on
	// the unbalanced text, so on the normalised path it previewed a span the
	// apply would never produce. (PR #68 maintainer review)
	plan, err := buildTextMatchPlan(filePath, oldText, newText)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("old_text not found in %s (tried exact and whitespace-normalised line match)", filePath)), nil
	}
	src := plan.src

	// 0-based line and UTF-16 column over the plan's matched span. Columns
	// were byte counts here, which only agreed with the simulation's edit by
	// accident before issue #52 made that path UTF-16 aware; on a line with
	// non-ASCII text before the match the preview edited the wrong span.
	startLine, startCol, endLine, endCol := byteSpanToLSPRange(src, plan.startByte, plan.startByte+len(plan.spanText))

	// Build args for HandleSimulateEditAtomic.
	workspaceRoot := client.RootDir()
	language := client.LanguageIDForFile(filePath)

	simArgs := map[string]any{
		"workspace_root": workspaceRoot,
		"language":       language,
		"file_path":      filePath,
		"start_line":     startLine + 1, // convert 0-based to 1-based
		"start_column":   startCol + 1,
		"end_line":       endLine + 1,
		"end_column":     endCol + 1,
		"new_text":       newText,
	}

	evalResult, err := simulateEditAtomicCore(ctx, sessionMgr, simArgs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("preview failed: %s", err)), nil
	}

	// In-memory JSON round-trip of the evaluation struct — format-independent.
	// Previously this parsed the format-aware tool output of
	// HandleSimulateEditAtomic, which is GCF (not JSON) under the default
	// output format, so the parse always failed with "failed to parse preview
	// result".
	previewJSON := map[string]any{}
	if raw, marshalErr := json.Marshal(evalResult); marshalErr == nil {
		_ = json.Unmarshal(raw, &previewJSON)
	}
	if previewJSON == nil {
		previewJSON = map[string]any{}
	}

	netDelta := 0
	if nd, ok := previewJSON["net_delta"].(float64); ok {
		netDelta = int(nd)
	}

	if blocked, hint := previewBlocksApply(netDelta, previewJSON); blocked {
		previewJSON["applied"] = false
		encoded, _ := EncodeResult(ctx, previewJSON)
		return appendHint(encoded, hint), nil
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
	encoded, _ := EncodeResult(ctx, previewJSON)
	return appendHint(encoded, "Edit applied successfully (net_delta == 0)."), nil
}

// previewBlocksApply decides whether a safe_apply_edit preview permits writing
// the edit. Only a verified result does: net_delta must not be positive, and the
// diagnostics must have settled. A timed-out wait (confidence "partial") or a
// server that was still indexing (confidence "low") reports net_delta 0 because
// nothing was checked, and applying then would claim a safety the preview never
// established. A cold gopls was observed to apply a type error this way.
func previewBlocksApply(netDelta int, preview map[string]any) (bool, string) {
	if netDelta > 0 {
		return true, "Edit would introduce errors. Review diagnostics before applying."
	}
	confidence, _ := preview["confidence"].(string)
	timedOut, _ := preview["timeout"].(bool)
	if timedOut || confidence == string(session.ConfidencePartial) || confidence == string(session.ConfidenceLow) {
		return true, "Not applied: the diagnostics did not settle (the language server timed out or is still indexing), so the edit could not be verified as safe. Retry once the server is ready, or use apply_edit to write it without verification."
	}
	return false, ""
}
