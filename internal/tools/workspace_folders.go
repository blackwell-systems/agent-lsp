package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleAddWorkspaceFolder adds a directory to the LSP workspace, enabling
// cross-repo references, definitions, and diagnostics for language servers
// that support multi-root workspaces (gopls, rust-analyzer, typescript-language-server).
//
// After adding a folder, the server re-indexes it and references in either
// direction across the workspace boundary become available — useful when
// working across a library and its consumers in the same session.
func HandleAddWorkspaceFolder(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return types.ErrorResult("path is required"), nil
	}

	if err := client.AddWorkspaceFolder(path); err != nil {
		return types.ErrorResult(fmt.Sprintf("add_workspace_folder: %s", err)), nil
	}

	folders := client.GetWorkspaceFolders()
	data, err := json.Marshal(map[string]interface{}{
		"added":             path,
		"workspace_folders": folders,
	})
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshal response: %s", err)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleRemoveWorkspaceFolder removes a directory from the LSP workspace.
func HandleRemoveWorkspaceFolder(_ context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return types.ErrorResult("path is required"), nil
	}

	if err := client.RemoveWorkspaceFolder(path); err != nil {
		return types.ErrorResult(fmt.Sprintf("remove_workspace_folder: %s", err)), nil
	}

	folders := client.GetWorkspaceFolders()
	data, err := json.Marshal(map[string]interface{}{
		"removed":           path,
		"workspace_folders": folders,
	})
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshal response: %s", err)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleListWorkspaceFolders returns the current workspace folder list.
func HandleListWorkspaceFolders(_ context.Context, client *lsp.LSPClient, _ map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	folders := client.GetWorkspaceFolders()
	data, err := json.Marshal(map[string]interface{}{
		"workspace_folders": folders,
	})
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshal response: %s", err)), nil
	}
	return types.TextResult(string(data)), nil
}
