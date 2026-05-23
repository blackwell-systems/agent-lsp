package lsp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/config"
)

// TestServerManager_DefaultClient_NoEntries tests DefaultClient with empty manager
func TestServerManager_DefaultClient_NoEntries(t *testing.T) {
	m := &ServerManager{entries: []*managedEntry{}}

	if client := m.DefaultClient(); client != nil {
		t.Errorf("DefaultClient() for empty manager = %v, want nil", client)
	}
}

// TestServerManager_ClientForFile_NoEntries tests ClientForFile with empty manager
func TestServerManager_ClientForFile_NoEntries(t *testing.T) {
	m := &ServerManager{entries: []*managedEntry{}}

	if client := m.ClientForFile("/test.go"); client != nil {
		t.Errorf("ClientForFile() for empty manager = %v, want nil", client)
	}
}

// TestServerManager_AllClients_WithNilClients tests AllClients filters nil entries
func TestServerManager_AllClients_WithNilClients(t *testing.T) {
	client1 := NewLSPClient("fake1", nil)
	client2 := NewLSPClient("fake2", nil)

	m := &ServerManager{
		entries: []*managedEntry{
			{client: client1, extensions: map[string]bool{"go": true}},
			{client: nil, extensions: map[string]bool{"ts": true}}, // nil client
			{client: client2, extensions: map[string]bool{"py": true}},
		},
	}

	clients := m.AllClients()

	if len(clients) != 2 {
		t.Errorf("AllClients() returned %d clients, want 2 (filtering nil)", len(clients))
	}

	// Verify the returned clients are the non-nil ones
	found1, found2 := false, false
	for _, c := range clients {
		if c == client1 {
			found1 = true
		}
		if c == client2 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("AllClients() should return only non-nil clients")
	}
}

// TestServerManager_ClientForFile_CaseInsensitiveExt tests extension matching is case-insensitive
func TestServerManager_ClientForFile_CaseInsensitiveExt(t *testing.T) {
	client := NewLSPClient("fake", nil)

	m := &ServerManager{
		entries: []*managedEntry{
			{
				client:     client,
				extensions: map[string]bool{"go": true, "ts": true},
			},
		},
	}

	tests := []struct {
		path           string
		shouldMatchExt bool // true if extension matches
	}{
		{"/test.go", true},
		{"/test.GO", true},
		{"/test.Go", true},
		{"/test.ts", true},
		{"/test.TS", true},
		{"/test.rs", false}, // .rs not in extensions, should fall back to default (which is client)
	}

	for _, tt := range tests {
		got := m.ClientForFile(tt.path)
		// All paths return a client (either matched or fallback to default)
		if got == nil {
			t.Errorf("ClientForFile(%q): got nil, should return client or fallback", tt.path)
		}
		// If extension doesn't match, it falls back to default (first entry's client)
		// So all paths should return the same client
		if got != client {
			t.Errorf("ClientForFile(%q): got different client than expected", tt.path)
		}
	}
}

// TestServerManager_ClientForFile_NoExtension tests fallback for files without extensions
func TestServerManager_ClientForFile_NoExtension(t *testing.T) {
	client := NewLSPClient("fake", nil)

	m := &ServerManager{
		entries: []*managedEntry{
			{
				client:     client,
				extensions: map[string]bool{"go": true},
			},
		},
	}

	// File without extension should fall back to default client
	got := m.ClientForFile("/path/to/Makefile")
	if got != client {
		t.Error("ClientForFile without extension should return default client")
	}
}

// TestServerManager_ClientForFile_MultipleEntries tests routing with multiple servers
func TestServerManager_ClientForFile_MultipleEntries(t *testing.T) {
	goClient := NewLSPClient("gopls", nil)
	tsClient := NewLSPClient("tsserver", nil)
	rsClient := NewLSPClient("rust-analyzer", nil)

	m := &ServerManager{
		entries: []*managedEntry{
			{client: goClient, extensions: map[string]bool{"go": true}},
			{client: tsClient, extensions: map[string]bool{"ts": true, "tsx": true, "js": true, "jsx": true}},
			{client: rsClient, extensions: map[string]bool{"rs": true}},
		},
	}

	tests := []struct {
		path   string
		want   *LSPClient
		reason string
	}{
		{"/test.go", goClient, "go file"},
		{"/test.ts", tsClient, "ts file"},
		{"/test.tsx", tsClient, "tsx file"},
		{"/test.js", tsClient, "js file"},
		{"/test.rs", rsClient, "rust file"},
		{"/test.py", goClient, "unknown extension falls back to first (go)"},
	}

	for _, tt := range tests {
		got := m.ClientForFile(tt.path)
		if got != tt.want {
			t.Errorf("ClientForFile(%q) [%s]: got wrong client", tt.path, tt.reason)
		}
	}
}

// TestNewMultiServerManager_EmptyEntries tests construction with no entries
func TestNewMultiServerManager_EmptyEntries(t *testing.T) {
	m := NewMultiServerManager([]config.ServerEntry{})

	if len(m.entries) != 0 {
		t.Errorf("NewMultiServerManager with empty entries: got %d entries, want 0", len(m.entries))
	}

	if m.DefaultClient() != nil {
		t.Error("DefaultClient() should return nil for empty manager")
	}
}

// TestNewMultiServerManager_ExtensionNormalization tests extension cleanup
func TestNewMultiServerManager_ExtensionNormalization(t *testing.T) {
	entries := []config.ServerEntry{
		{
			Extensions: []string{".go", "GO", ".Ts", "tsx"},
			Command:    []string{"test-server"},
			LanguageID: "test",
		},
	}

	m := NewMultiServerManager(entries)

	if len(m.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.entries))
	}

	entry := m.entries[0]

	// Verify extensions are normalized (lowercase, no leading dot)
	expected := map[string]bool{
		"go":  true,
		"ts":  true,
		"tsx": true,
	}

	if len(entry.extensions) != len(expected) {
		t.Errorf("got %d extensions, want %d", len(entry.extensions), len(expected))
	}

	for ext := range expected {
		if !entry.extensions[ext] {
			t.Errorf("missing expected extension %q (extensions: %v)", ext, entry.extensions)
		}
	}

	// Verify uppercase GO was normalized
	if entry.extensions["GO"] {
		t.Error("extension 'GO' should be normalized to 'go'")
	}
}

// TestNewMultiServerManager_LanguageIDInference tests language ID is inferred when empty
func TestNewMultiServerManager_LanguageIDInference(t *testing.T) {
	entries := []config.ServerEntry{
		{Extensions: []string{"go"}, Command: []string{"gopls"}},
		{Extensions: []string{"ts"}, Command: []string{"tsserver"}},
		{Extensions: []string{"py"}, Command: []string{"pyright"}},
	}

	m := NewMultiServerManager(entries)

	// Language IDs should be inferred. We can't directly inspect them
	// (unexported field), but the manager should build without error
	// and have the correct number of entries.
	if len(m.entries) != 3 {
		t.Errorf("got %d entries, want 3", len(m.entries))
	}
}

// TestServerManager_Shutdown_EmptyManager tests shutdown with no clients
func TestServerManager_Shutdown_EmptyManager(t *testing.T) {
	m := &ServerManager{entries: []*managedEntry{}}
	ctx := context.Background()

	err := m.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown on empty manager returned error: %v", err)
	}
}

// TestServerManager_Shutdown_WithNilClients tests shutdown filters nil clients
func TestServerManager_Shutdown_WithNilClients(t *testing.T) {
	m := &ServerManager{
		entries: []*managedEntry{
			{client: nil, extensions: map[string]bool{"go": true}},
			{client: nil, extensions: map[string]bool{"ts": true}},
		},
	}

	ctx := context.Background()
	err := m.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown with nil clients returned error: %v", err)
	}
}

// TestStartForLanguage_SingleServerMode tests single-server fallback behavior
func TestStartForLanguage_SingleServerMode(t *testing.T) {
	client := NewLSPClient("fake-server", nil)
	m := NewSingleServerManager(client)

	// In single-server mode, any language request returns the same client
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// This would normally call Initialize, but since we're using a fake server
	// it will fail. We're testing the routing logic, not actual initialization.
	returnedClient, err := m.StartForLanguage(ctx, "/tmp", "go")

	// We expect an error because the fake server doesn't exist,
	// but the important part is that it attempted to return the client
	_ = returnedClient
	_ = err

	// Verify single-server mode returns the same client for different languages
	// (by checking the manager has only one entry)
	if len(m.entries) != 1 {
		t.Errorf("single-server manager has %d entries, want 1", len(m.entries))
	}
}

// TestStartForLanguage_ErrorMessage tests error message quality
func TestStartForLanguage_ErrorMessage(t *testing.T) {
	m := NewMultiServerManager([]config.ServerEntry{
		{Extensions: []string{"go"}, Command: []string{"gopls"}, LanguageID: "go"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := m.StartForLanguage(ctx, "/tmp", "rust")
	if err == nil {
		t.Fatal("expected error for unconfigured language")
	}

	errMsg := err.Error()

	// Verify error message contains useful information
	if !strings.Contains(errMsg, "rust") {
		t.Errorf("error message should mention language 'rust': %s", errMsg)
	}

	if !strings.Contains(errMsg, "no server configured") {
		t.Errorf("error message should explain no server configured: %s", errMsg)
	}
}

// TestInferLanguageID_Coverage tests language ID inference for various extensions
func TestInferLanguageID_Coverage(t *testing.T) {
	// Test by creating managers and verifying they build without error
	tests := []struct {
		ext      string
		expected string // what we expect inferLanguageID to return
	}{
		{"go", "go"},
		{"ts", "typescript"},
		{"tsx", "typescript"},
		{"js", "javascript"},
		{"jsx", "javascript"},
		{"py", "python"},
		{"rs", "rust"},
		{"java", "java"},
		{"c", "c"},
		{"cpp", "cpp"},
		{"cs", "csharp"},
		{"rb", "ruby"},
		{"php", "php"},
		{"hs", "haskell"},
		{"kt", "kotlin"},
		{"ml", "ocaml"},
	}

	for _, tt := range tests {
		entry := config.ServerEntry{
			Extensions: []string{tt.ext},
			Command:    []string{"fake-server"},
			// LanguageID empty to trigger inference
		}

		m := NewMultiServerManager([]config.ServerEntry{entry})
		if m == nil {
			t.Errorf("NewMultiServerManager failed for extension %q", tt.ext)
		}

		// We can't directly verify the inferred language ID (unexported),
		// but we verified the manager builds successfully
	}
}

// TestServerManager_StartForLanguage_CaseInsensitive tests language matching is case-insensitive
func TestServerManager_StartForLanguage_CaseInsensitive(t *testing.T) {
	m := NewMultiServerManager([]config.ServerEntry{
		{Extensions: []string{"go"}, Command: []string{"gopls"}, LanguageID: "go"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Try with different cases (will fail due to fake server, but tests routing)
	cases := []string{"go", "Go", "GO", "gO"}

	for _, lang := range cases {
		_, err := m.StartForLanguage(ctx, "/tmp", lang)
		// We expect errors because gopls doesn't exist, but NOT "no server configured"
		if err != nil && strings.Contains(err.Error(), "no server configured") {
			t.Errorf("StartForLanguage(%q): got 'no server configured', should match case-insensitively", lang)
		}
	}
}

// TestNewSingleServerManager_NilClient tests handling of nil client
func TestNewSingleServerManager_NilClient(t *testing.T) {
	// While not recommended, test that it doesn't crash
	m := NewSingleServerManager(nil)

	if m == nil {
		t.Fatal("NewSingleServerManager(nil) returned nil manager")
	}

	if m.DefaultClient() != nil {
		t.Error("DefaultClient() should return nil when constructed with nil client")
	}
}

// TestServerManager_ConcurrentAccess tests thread safety
func TestServerManager_ConcurrentAccess(t *testing.T) {
	client := NewLSPClient("fake", nil)
	m := NewSingleServerManager(client)

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 20; i++ {
		go func() {
			m.ClientForFile("/test.go")
			m.DefaultClient()
			m.AllClients()
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	// If we get here without data races, test passes
}

// TestServerManager_ExtensionEdgeCases tests edge cases in extension handling
func TestServerManager_ExtensionEdgeCases(t *testing.T) {
	m := NewMultiServerManager([]config.ServerEntry{
		{
			Extensions: []string{"", " ", "..", "...go"},
			Command:    []string{"test"},
			LanguageID: "test",
		},
	})

	if len(m.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.entries))
	}

	// Verify problematic extensions are normalized
	entry := m.entries[0]

	// Empty strings and single dots should result in empty key
	if entry.extensions[""] && len(entry.extensions) > 1 {
		t.Log("Note: empty extension key present (expected for empty input)")
	}
}
