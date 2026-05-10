package resources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// HandleInspectResource reads the last inspection results from the workspace.
// URI format: inspect://last
// Returns the contents of .agent-lsp/last-inspection.json as application/json.
func HandleInspectResource(_ context.Context, workspaceRoot string, uri string) (ResourceResult, error) {
	if workspaceRoot == "" {
		return ResourceResult{}, fmt.Errorf("workspace root not initialized; call start_lsp first")
	}
	path := filepath.Join(workspaceRoot, ".agent-lsp", "last-inspection.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ResourceResult{
				URI:      uri,
				MIMEType: "application/json",
				Text:     `{"error":"no inspection results found","hint":"run /lsp-inspect first"}`,
			}, nil
		}
		return ResourceResult{}, fmt.Errorf("reading inspection results: %w", err)
	}
	return ResourceResult{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(data),
	}, nil
}
