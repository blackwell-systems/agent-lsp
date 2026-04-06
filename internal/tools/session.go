package tools

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// HandleStartLsp starts (or restarts) the LSP server. If an existing client is
// non-nil, Shutdown is called before creating the new client.
func HandleStartLsp(
	ctx context.Context,
	getClient func() *lsp.LSPClient,
	setClient func(*lsp.LSPClient),
	serverPath string,
	serverArgs []string,
	args map[string]interface{},
) (types.ToolResult, error) {
	rootDir, ok := args["root_dir"].(string)
	if !ok || rootDir == "" {
		return types.ErrorResult("root_dir is required"), nil
	}

	// Shutdown any existing client.
	if existing := getClient(); existing != nil {
		_ = existing.Shutdown(ctx) // best-effort
	}

	client := lsp.NewLSPClient(serverPath, serverArgs)
	if err := client.Initialize(ctx, rootDir); err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to initialize LSP server: %s", err)), nil
	}

	setClient(client)
	return types.TextResult("LSP server started successfully"), nil
}

// HandleRestartLspServer restarts the LSP server with the given root dir.
// root_dir is required: omitting it would construct a malformed "file://" rootURI.
func HandleRestartLspServer(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	rootDir, _ := args["root_dir"].(string)
	if rootDir == "" {
		return types.ErrorResult("root_dir is required for restart_lsp_server"), nil
	}
	if err := client.Restart(ctx, rootDir); err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to restart LSP server: %s", err)), nil
	}
	return types.TextResult("LSP server restarted successfully"), nil
}

// HandleOpenDocument opens a document in the LSP server.
func HandleOpenDocument(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	// text is an optional Go-specific extension not present in the TypeScript schema.
	// Callers may provide file content directly to avoid a disk read.
	// If omitted or empty, the LSP server will read the file from disk on didOpen.
	text, _ := args["text"].(string)
	fileURI := CreateFileURI(filePath)

	if err := client.OpenDocument(ctx, fileURI, text, languageID); err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to open document: %s", err)), nil
	}
	return types.TextResult(fmt.Sprintf("Document opened: %s", filePath)), nil
}

// HandleCloseDocument closes a document in the LSP server.
func HandleCloseDocument(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	fileURI := CreateFileURI(filePath)
	if err := client.CloseDocument(ctx, fileURI); err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to close document: %s", err)), nil
	}
	return types.TextResult(fmt.Sprintf("Document closed: %s", filePath)), nil
}
