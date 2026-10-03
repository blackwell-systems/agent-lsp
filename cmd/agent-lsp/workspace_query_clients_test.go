package main

import (
	"context"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// stubResolver implements lsp.ClientResolver with a fixed client set.
type stubResolver struct {
	defaultClient *lsp.LSPClient
	forFile       map[string]*lsp.LSPClient
	all           []*lsp.LSPClient
}

func (s *stubResolver) ClientForFile(path string) *lsp.LSPClient { return s.forFile[path] }
func (s *stubResolver) DefaultClient() *lsp.LSPClient            { return s.defaultClient }
func (s *stubResolver) AllClients() []*lsp.LSPClient             { return s.all }
func (s *stubResolver) Shutdown(ctx context.Context) error       { return nil }

// --- workspaceQueryClients (issue #2) ---

// Server-less workspace queries use the default client first plus every
// other connected client, deduplicated. In single-server mode this is just
// the old default.
func TestWorkspaceQueryClients_SingleServer(t *testing.T) {
	cs := &clientState{}
	c1 := lsp.NewLSPClient("mql-lsp-server", nil)
	cs.set(c1)
	got := workspaceQueryClients(cs, &stubResolver{})
	if len(got) != 1 || got[0] != c1 {
		t.Fatalf("expected [c1], got %v", got)
	}
}

// Multi-server: the default (cs) leads; resolver clients are appended
// without duplicates — the same client reached through both paths appears
// once.
func TestWorkspaceQueryClients_MultiServerDedup(t *testing.T) {
	cs := &clientState{}
	clangd := lsp.NewLSPClient("clangd", nil)
	mql := lsp.NewLSPClient("mql-lsp-server", nil)
	cs.set(clangd)
	got := workspaceQueryClients(cs, &stubResolver{all: []*lsp.LSPClient{clangd, mql}})
	if len(got) != 2 {
		t.Fatalf("expected 2 clients after dedup, got %d", len(got))
	}
	if got[0] != clangd || got[1] != mql {
		t.Fatalf("expected [clangd, mql], got %v", got)
	}
}

// No clients anywhere → empty set (the handler reports not-initialized).
func TestWorkspaceQueryClients_Empty(t *testing.T) {
	got := workspaceQueryClients(&clientState{}, &stubResolver{})
	if len(got) != 0 {
		t.Fatalf("expected empty set, got %v", got)
	}
}
