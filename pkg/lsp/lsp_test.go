package lsp_test

import (
	"context"
	"testing"

	internallsp "github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/config"
	publsp "github.com/blackwell-systems/agent-lsp/pkg/lsp"
)

// Compile-time assertion: ServerManager satisfies the ClientResolver interface
// declared in pkg/lsp.
var _ publsp.ClientResolver = (*publsp.ServerManager)(nil)

// TestPkgLSPCompileSmoke verifies that pkg/lsp re-exports are type-compatible
// with their internal/lsp counterparts. No LSP binary is started.
func TestPkgLSPCompileSmoke(t *testing.T) {
	t.Skip("compile smoke only — verifies type aliases compile correctly")

	// NewLSPClient: func(string, []string) *LSPClient
	var newClientFn func(string, []string) *publsp.LSPClient = publsp.NewLSPClient
	_ = newClientFn

	// NewSingleServerManager: func(*LSPClient) *ServerManager
	var newSingleFn func(*publsp.LSPClient) *publsp.ServerManager = publsp.NewSingleServerManager
	_ = newSingleFn

	// NewMultiServerManager: func([]config.ServerEntry) *ServerManager
	var newMultiFn func([]config.ServerEntry) *publsp.ServerManager = publsp.NewMultiServerManager
	_ = newMultiFn

	// Type alias identity: publsp.LSPClient IS internallsp.LSPClient.
	var _ *internallsp.LSPClient = (*publsp.LSPClient)(nil)
	var _ *internallsp.ServerManager = (*publsp.ServerManager)(nil)

	// ClientResolver interface method signatures.
	var cr publsp.ClientResolver
	var ctx context.Context = context.Background()
	_ = cr.ClientForFile("/tmp/test.go")
	_ = cr.DefaultClient()
	_ = cr.AllClients()
	_ = cr.Shutdown(ctx)
}
