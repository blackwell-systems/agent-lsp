package lsp

import (
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// TestGetAllDiagnostics verifies the deep copy behavior.
func TestGetAllDiagnostics(t *testing.T) {
	c := NewLSPClient("fake", nil)

	// Empty: should return empty map, not nil.
	all := c.GetAllDiagnostics()
	if all == nil {
		t.Fatal("expected non-nil map")
	}
	if len(all) != 0 {
		t.Errorf("expected empty map, got %d entries", len(all))
	}

	// Add diagnostics and verify they're returned.
	c.diagMu.Lock()
	c.diags["file:///a.go"] = []types.LSPDiagnostic{
		{Message: "error1", Severity: 1},
		{Message: "warning1", Severity: 2},
	}
	c.diags["file:///b.go"] = []types.LSPDiagnostic{
		{Message: "error2", Severity: 1},
	}
	c.diagMu.Unlock()

	all = c.GetAllDiagnostics()
	if len(all) != 2 {
		t.Fatalf("expected 2 URIs, got %d", len(all))
	}
	if len(all["file:///a.go"]) != 2 {
		t.Errorf("expected 2 diags for a.go, got %d", len(all["file:///a.go"]))
	}
	if len(all["file:///b.go"]) != 1 {
		t.Errorf("expected 1 diag for b.go, got %d", len(all["file:///b.go"]))
	}

	// Verify it's a deep copy: modifying returned slice shouldn't affect internal state.
	all["file:///a.go"][0].Message = "mutated"
	c.diagMu.RLock()
	if c.diags["file:///a.go"][0].Message == "mutated" {
		t.Error("GetAllDiagnostics returned a shallow copy; expected deep copy")
	}
	c.diagMu.RUnlock()
}

// TestSetScopeConfig verifies scope config is stored.
func TestSetScopeConfig(t *testing.T) {
	c := NewLSPClient("fake", nil)
	if c.scopeConfig != nil {
		t.Error("expected nil scopeConfig on new client")
	}
	sc := &ScopeConfig{GeneratedFiles: []string{"/tmp/test"}}
	c.SetScopeConfig(sc)
	if c.scopeConfig != sc {
		t.Error("expected scopeConfig to be set")
	}
}

// TestLanguageIDFromPath covers the extension-to-language mapping.
func TestLanguageIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/foo/bar.go", "go"},
		{"/foo/bar.ts", "typescript"},
		{"/foo/bar.tsx", "typescript"},
		{"/foo/bar.js", "javascript"},
		{"/foo/bar.jsx", "javascript"},
		{"/foo/bar.py", "python"},
		{"/foo/bar.rs", "rust"},
		{"/foo/bar.cs", "csharp"},
		{"/foo/bar.hs", "haskell"},
		{"/foo/bar.lhs", "haskell"},
		{"/foo/bar.rb", "ruby"},
		{"/foo/bar.kt", "kotlin"},
		{"/foo/bar.kts", "kotlin"},
		{"/foo/bar.ml", "ocaml"},
		{"/foo/bar.mli", "ocaml"},
		{"/foo/bar.c", "c"},
		{"/foo/bar.cpp", "cpp"},
		{"/foo/bar.cc", "cpp"},
		{"/foo/bar.cxx", "cpp"},
		{"/foo/bar.java", "java"},
		{"/foo/bar.unknown", "plaintext"},
		{"/foo/Makefile", "plaintext"},
	}
	for _, tt := range tests {
		got := LanguageIDFromPath(tt.path)
		if got != tt.want {
			t.Errorf("LanguageIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestDefaultClient verifies DefaultClient returns the first entry.
func TestDefaultClient(t *testing.T) {
	c := NewLSPClient("fake", nil)
	m := NewSingleServerManager(c)
	got := m.DefaultClient()
	if got != c {
		t.Error("expected DefaultClient to return the single client")
	}
}

// TestDefaultClient_EmptyManager verifies DefaultClient returns nil for empty.
func TestDefaultClient_EmptyManager(t *testing.T) {
	m := &ServerManager{}
	got := m.DefaultClient()
	if got != nil {
		t.Error("expected nil for empty manager")
	}
}

// TestIsDaemon verifies the daemon flag accessor.
func TestIsDaemon(t *testing.T) {
	c := NewLSPClient("fake", nil)
	if c.IsDaemon() {
		t.Error("expected IsDaemon false for direct client")
	}
	c.isDaemon = true
	if !c.IsDaemon() {
		t.Error("expected IsDaemon true after setting flag")
	}
}

// TestGetDaemonInfo verifies the daemon info accessor.
func TestGetDaemonInfo(t *testing.T) {
	c := NewLSPClient("fake", nil)
	if c.GetDaemonInfo() != nil {
		t.Error("expected nil daemon info for direct client")
	}
	info := &DaemonInfo{PID: 42}
	c.daemonInfo = info
	if c.GetDaemonInfo() != info {
		t.Error("expected daemon info to match")
	}
}

// TestLanguageIDFromPath_CaseInsensitive verifies uppercase extensions are handled.
func TestLanguageIDFromPath_CaseInsensitive(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/foo/bar.GO", "go"},
		{"/foo/bar.Py", "python"},
		{"/foo/bar.RS", "rust"},
		{"/foo/bar.TS", "typescript"},
		{"/foo/bar.TSX", "typescript"},
		{"/foo/bar.JS", "javascript"},
		{"/foo/bar.JSX", "javascript"},
		{"/foo/bar.CPP", "cpp"},
		{"/foo/bar.JAVA", "java"},
	}
	for _, tt := range tests {
		got := LanguageIDFromPath(tt.path)
		if got != tt.want {
			t.Errorf("LanguageIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestLanguageIDFromPath_NoExtension verifies files without extensions return plaintext.
func TestLanguageIDFromPath_NoExtension(t *testing.T) {
	tests := []string{
		"/foo/Makefile",
		"/foo/Dockerfile",
		"/foo/README",
		"noext",
		"",
	}
	for _, path := range tests {
		got := LanguageIDFromPath(path)
		if got != "plaintext" {
			t.Errorf("LanguageIDFromPath(%q) = %q, want plaintext", path, got)
		}
	}
}

// TestNewLSPClient_Initialized verifies a new client is not initialized.
func TestNewLSPClient_Initialized(t *testing.T) {
	c := NewLSPClient("fake", nil)
	if c.IsInitialized() {
		t.Error("expected IsInitialized false for new client")
	}
}

// TestGetAllDiagnostics_DeepCopyIsolation verifies mutations to returned map
// keys do not affect the internal state.
func TestGetAllDiagnostics_DeepCopyIsolation(t *testing.T) {
	c := NewLSPClient("fake", nil)

	c.diagMu.Lock()
	c.diags["file:///x.go"] = []types.LSPDiagnostic{
		{Message: "err", Severity: 1},
	}
	c.diagMu.Unlock()

	all := c.GetAllDiagnostics()
	// Add a new key to the returned map.
	all["file:///injected.go"] = []types.LSPDiagnostic{{Message: "injected"}}

	// Internal state should not contain the injected key.
	c.diagMu.RLock()
	_, found := c.diags["file:///injected.go"]
	c.diagMu.RUnlock()
	if found {
		t.Error("modifying returned map should not affect internal state")
	}
}

// TestTimeoutFor verifies known and unknown method timeouts.
func TestTimeoutFor(t *testing.T) {
	tests := []struct {
		method string
		want   time.Duration
	}{
		{"initialize", 300 * time.Second},
		{"textDocument/references", 120 * time.Second},
		{"textDocument/hover", 30 * time.Second},
		{"textDocument/documentHighlight", 10 * time.Second},
		{"callHierarchy/incomingCalls", 60 * time.Second},
		{"unknown/method", defaultTimeout},
		{"", defaultTimeout},
	}
	for _, tt := range tests {
		got := timeoutFor(tt.method)
		if got != tt.want {
			t.Errorf("timeoutFor(%q) = %v, want %v", tt.method, got, tt.want)
		}
	}
}

// TestReopenAllDocuments_EmptyOpenDocs verifies no error when no documents are open.
func TestReopenAllDocuments_EmptyOpenDocs(t *testing.T) {
	c := NewLSPClient("fake", nil)
	ctx := t.Context()
	// ReopenAllDocuments with no open docs should return nil (no-op).
	// Note: this won't actually send notifications since stdin is nil,
	// but with zero docs it should just return nil without touching stdin.
	err := c.ReopenAllDocuments(ctx)
	if err != nil {
		t.Errorf("expected nil error for empty openDocs, got: %v", err)
	}
}
