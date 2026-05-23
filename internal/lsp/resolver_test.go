package lsp

import (
	"context"
	"testing"
)

// TestClientResolver_InterfaceCompliance tests that ServerManager implements ClientResolver
func TestClientResolver_InterfaceCompliance(t *testing.T) {
	var _ ClientResolver = (*ServerManager)(nil)

	// This test will fail at compile time if ServerManager doesn't implement ClientResolver
}

// TestClientResolver_AllMethods tests all resolver methods on a real manager
func TestClientResolver_AllMethods(t *testing.T) {
	client := NewLSPClient("fake-server", nil)
	var resolver ClientResolver = NewSingleServerManager(client)

	// Test ClientForFile
	c1 := resolver.ClientForFile("/test.go")
	if c1 != client {
		t.Error("ClientForFile should return the client")
	}

	// Test DefaultClient
	c2 := resolver.DefaultClient()
	if c2 != client {
		t.Error("DefaultClient should return the client")
	}

	// Test AllClients
	all := resolver.AllClients()
	if len(all) != 1 || all[0] != client {
		t.Errorf("AllClients() = %v, want [client]", all)
	}

	// Test Shutdown (may return error for unstarted client)
	ctx := context.Background()
	_ = resolver.Shutdown(ctx) // Shutdown on uninitialized client is allowed to error
}

// TestClientResolver_NilReturns tests resolver with no clients
func TestClientResolver_NilReturns(t *testing.T) {
	var resolver ClientResolver = &ServerManager{entries: []*managedEntry{}}

	if c := resolver.ClientForFile("/test.go"); c != nil {
		t.Error("empty resolver should return nil client")
	}

	if c := resolver.DefaultClient(); c != nil {
		t.Error("empty resolver should return nil default client")
	}

	if all := resolver.AllClients(); len(all) != 0 {
		t.Errorf("empty resolver AllClients() = %d clients, want 0", len(all))
	}
}

// TestClientResolver_EmptyFilePath tests handling of empty file path
func TestClientResolver_EmptyFilePath(t *testing.T) {
	client := NewLSPClient("fake", nil)
	var resolver ClientResolver = NewSingleServerManager(client)

	// Empty path should still work (falls back to default)
	c := resolver.ClientForFile("")
	if c != client {
		t.Error("empty path should return default client")
	}
}

// TestClientResolver_MultipleShutdowns tests calling Shutdown multiple times
func TestClientResolver_MultipleShutdowns(t *testing.T) {
	client := NewLSPClient("fake", nil)
	var resolver ClientResolver = NewSingleServerManager(client)

	ctx := context.Background()

	// First shutdown (may error for unstarted client, but shouldn't crash)
	_ = resolver.Shutdown(ctx)

	// Second shutdown (should not crash)
	_ = resolver.Shutdown(ctx)

	// Test passes if no panic occurs
}
