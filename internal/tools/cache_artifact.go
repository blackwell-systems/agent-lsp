// cache_artifact.go implements MCP tool handlers for exporting and importing
// the symbol reference cache as compressed artifacts for team sharing.
package tools

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// confineCachePath resolves a cache artifact path inside the workspace root.
// A relative path is taken relative to the root. Export would otherwise create
// directories and write a file at any path the client names, and import would
// read any file, so both are held to the workspace like every other file tool.
func confineCachePath(client *lsp.LSPClient, path string) (string, error) {
	root := client.RootDir()
	if root == "" {
		return "", fmt.Errorf("workspace root not initialized; call start_lsp first")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return ValidateFilePath(path, root)
}

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

	destPath, err := confineCachePath(client, destPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid dest_path: %s", err)), nil
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

	srcPath, err := confineCachePath(client, srcPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid src_path: %s", err)), nil
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
