package main

import (
	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// snapshotDiagnostics captures the current diagnostic state for the given files
// from the LSP client. Returns nil if the client is nil.
func snapshotDiagnostics(client *lsp.LSPClient, files []string) *audit.DiagnosticState {
	if client == nil {
		return nil
	}
	state := &audit.DiagnosticState{
		FilesChecked: files,
	}
	for _, f := range files {
		uri := fileToURI(f)
		diags := client.GetDiagnostics(uri)
		for _, d := range diags {
			switch d.Severity {
			case 1: // error
				state.ErrorCount++
			case 2: // warning
				state.WarningCount++
			}
		}
	}
	return state
}

// snapshotAllDiagnostics captures the current diagnostic state across all open files.
func snapshotAllDiagnostics(client *lsp.LSPClient) *audit.DiagnosticState {
	if client == nil {
		return nil
	}
	allDiags := client.GetAllDiagnostics()
	state := &audit.DiagnosticState{}
	for uri, diags := range allDiags {
		state.FilesChecked = append(state.FilesChecked, uri)
		for _, d := range diags {
			switch d.Severity {
			case 1:
				state.ErrorCount++
			case 2:
				state.WarningCount++
			}
		}
	}
	return state
}

// computeDelta computes the difference between before and after diagnostic states.
func computeDelta(before, after *audit.DiagnosticState) *audit.DiagnosticDelta {
	if before == nil || after == nil {
		return nil
	}
	return &audit.DiagnosticDelta{
		Errors:   after.ErrorCount - before.ErrorCount,
		Warnings: after.WarningCount - before.WarningCount,
	}
}

// fileToURI converts a file path to a file:// URI.
func fileToURI(path string) string {
	return "file://" + path
}

// extractFilesFromWorkspaceEdit pulls affected file paths from a workspace_edit arg.
func extractFilesFromWorkspaceEdit(edit map[string]interface{}) []string {
	changes, ok := edit["changes"].(map[string]interface{})
	if !ok {
		return nil
	}
	files := make([]string, 0, len(changes))
	for uri := range changes {
		files = append(files, uri)
	}
	return files
}

// isToolResultError checks if a ToolResult indicates an error.
func isToolResultError(r types.ToolResult) bool {
	return r.IsError
}

// toolResultErrorMsg extracts the error message from a ToolResult.
func toolResultErrorMsg(r types.ToolResult) string {
	if !r.IsError {
		return ""
	}
	if len(r.Content) > 0 {
		return r.Content[0].Text
	}
	return "unknown error"
}
