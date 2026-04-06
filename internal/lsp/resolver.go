package lsp

import "context"

// ClientResolver resolves the appropriate LSPClient for a given file path.
// In single-server mode the manager returns the same client regardless of path.
// In multi-server mode it routes by file extension.
type ClientResolver interface {
	// ClientForFile returns the LSPClient that should handle operations for
	// the given absolute file path. Falls back to DefaultClient if the extension
	// is not mapped. Returns nil only if the manager holds zero clients.
	ClientForFile(filePath string) *LSPClient

	// DefaultClient returns the primary (or only) LSPClient.
	// Used for tools that are not file-specific (e.g. get_workspace_symbols).
	DefaultClient() *LSPClient

	// AllClients returns all managed clients, including nil entries filtered out.
	AllClients() []*LSPClient

	// Shutdown gracefully shuts down all managed LSP clients.
	Shutdown(ctx context.Context) error
}
