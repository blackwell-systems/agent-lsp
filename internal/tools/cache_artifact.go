// cache_artifact.go implements MCP tool handlers for exporting and importing
// the symbol reference cache as compressed artifacts for team sharing.
package tools

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleExportCache exports the symbol reference cache as a gzip-compressed
// artifact to the specified destination path.
func HandleExportCache(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	destPath, ok := args["dest_path"].(string)
	if !ok || destPath == "" {
		return types.ErrorResult("dest_path is required"), nil
	}

	cache := client.RefCache()
	if cache == nil {
		return types.ErrorResult("reference cache is not available"), nil
	}

	if err := cache.ExportArtifact(destPath); err != nil {
		return types.ErrorResult(fmt.Sprintf("export failed: %s", err)), nil
	}

	entries, _ := cache.Stats()
	return appendHint(types.TextResult(fmt.Sprintf("Cache exported to %s (%d entries)", destPath, entries)), "Commit .agent-lsp/cache.db.gz to share with teammates."), nil
}

// HandleImportCache imports a gzip-compressed cache artifact from the
// specified source path, replacing the current cache contents.
func HandleImportCache(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	srcPath, ok := args["src_path"].(string)
	if !ok || srcPath == "" {
		return types.ErrorResult("src_path is required"), nil
	}

	cache := client.RefCache()
	if cache == nil {
		return types.ErrorResult("reference cache is not available"), nil
	}

	if err := cache.ImportArtifact(srcPath); err != nil {
		return types.ErrorResult(fmt.Sprintf("import failed: %s", err)), nil
	}

	entries, _ := cache.Stats()
	return appendHint(types.TextResult(fmt.Sprintf("Cache imported from %s (%d entries)", srcPath, entries)), "Cache imported. Reference queries will use cached results."), nil
}
