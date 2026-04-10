package lsp

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	internallsp "github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// LSPClient is the core LSP subprocess client. See [internallsp.LSPClient]
// for full documentation.
type LSPClient = internallsp.LSPClient

// ServerManager manages one or more LSP server instances and implements
// [ClientResolver]. See [internallsp.ServerManager] for full documentation.
type ServerManager = internallsp.ServerManager

// ClientResolver resolves the appropriate [LSPClient] for a given file path.
// Implemented by [ServerManager].
type ClientResolver interface {
	ClientForFile(filePath string) *LSPClient
	DefaultClient() *LSPClient
	AllClients() []*LSPClient
	Shutdown(ctx context.Context) error
}

// NewLSPClient creates a new, unstarted LSP client.
// Call [LSPClient.Initialize] to start the subprocess and complete the handshake.
var NewLSPClient = internallsp.NewLSPClient

// NewSingleServerManager wraps a single [LSPClient] to satisfy [ClientResolver].
var NewSingleServerManager = internallsp.NewSingleServerManager

// NewMultiServerManager creates a [ServerManager] from multiple ServerEntry configs.
// Does NOT start servers — call [ServerManager.StartAll] separately.
var NewMultiServerManager = internallsp.NewMultiServerManager

// ServerEntry is the configuration type for a single language server entry.
// Re-exported from internal/config for callers who use NewMultiServerManager.
type ServerEntry = config.ServerEntry
