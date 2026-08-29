package main

import (
	"context"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// mockResolver implements lsp.ClientResolver for testing.
type mockResolver struct {
	clients map[string]*lsp.LSPClient // extension -> client
	def     *lsp.LSPClient
}

func (r *mockResolver) DefaultClient() *lsp.LSPClient {
	return r.def
}

func (r *mockResolver) ClientForFile(filePath string) *lsp.LSPClient {
	if filePath == "" {
		return nil
	}
	ext := ""
	for i := len(filePath) - 1; i >= 0; i-- {
		if filePath[i] == '.' {
			ext = filePath[i+1:]
			break
		}
	}
	if c, ok := r.clients[ext]; ok {
		return c
	}
	return nil
}

func (r *mockResolver) AllClients() []*lsp.LSPClient {
	return nil
}

func (r *mockResolver) Shutdown(_ context.Context) error {
	return nil
}

func TestClientForFile_ExtensionRoutingFirst(t *testing.T) {
	// Setup: two clients - one for luau, one as default (clangd)
	luauClient := lsp.NewLSPClient("luau-lsp", nil)
	luauClient.MarkInitializedForTest()

	clangdClient := lsp.NewLSPClient("clangd", nil)
	clangdClient.MarkInitializedForTest()

	resolver := &mockResolver{
		clients: map[string]*lsp.LSPClient{
			"luau": luauClient,
		},
		def: clangdClient,
	}

	cs := &clientState{}
	cs.set(clangdClient) // default client is clangd

	// Test: .luau file should route to luauClient, not clangdClient
	got := clientForFile(resolver, cs, "/project/script.luau")
	if got != luauClient {
		t.Errorf("clientForFile for .luau = %v, want luauClient", got)
	}
}

func TestClientForFile_FallbackToDefault(t *testing.T) {
	luauClient := lsp.NewLSPClient("luau-lsp", nil)
	luauClient.MarkInitializedForTest()

	clangdClient := lsp.NewLSPClient("clangd", nil)
	clangdClient.MarkInitializedForTest()

	resolver := &mockResolver{
		clients: map[string]*lsp.LSPClient{
			"luau": luauClient,
		},
		def: clangdClient,
	}

	cs := &clientState{}
	cs.set(clangdClient)

	// Test: .go file should fall back to clangdClient
	got := clientForFile(resolver, cs, "/project/main.go")
	if got != clangdClient {
		t.Errorf("clientForFile for .go = %v, want clangdClient", got)
	}
}

func TestClientForFile_EmptyPath(t *testing.T) {
	clangdClient := lsp.NewLSPClient("clangd", nil)
	clangdClient.MarkInitializedForTest()

	resolver := &mockResolver{
		clients: map[string]*lsp.LSPClient{},
		def:     clangdClient,
	}

	cs := &clientState{}
	cs.set(clangdClient)

	// Test: empty path should fall back to default
	got := clientForFile(resolver, cs, "")
	if got != clangdClient {
		t.Errorf("clientForFile for empty path = %v, want clangdClient", got)
	}
}
