package lsp

import (
	"testing"
)

// --- removeEnv ---

func TestRemoveEnv(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		key  string
		want []string
	}{
		{
			"removes matching key",
			[]string{"PATH=/usr/bin", "GOWORK=/tmp/go.work", "HOME=/home/user"},
			"GOWORK",
			[]string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			"no match leaves env intact",
			[]string{"PATH=/usr/bin", "HOME=/home/user"},
			"GOWORK",
			[]string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			"removes multiple entries for same key",
			[]string{"FOO=1", "FOO=2", "BAR=3"},
			"FOO",
			[]string{"BAR=3"},
		},
		{
			"empty env",
			[]string{},
			"PATH",
			[]string{},
		},
		{
			"does not remove partial key match",
			[]string{"GOWORKSPACE=/tmp", "GOWORK=/tmp/go.work"},
			"GOWORK",
			[]string{"GOWORKSPACE=/tmp"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := removeEnv(tc.env, tc.key)
			if len(got) != len(tc.want) {
				t.Fatalf("len: want %d, got %d (%v)", len(tc.want), len(got), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: want %q, got %q", i, tc.want[i], got[i])
				}
			}
		})
	}
}

// --- languageIDFromURI ---

func TestLanguageIDFromURI_Extended(t *testing.T) {
	cases := []struct {
		uri  string
		want string
	}{
		{"file:///project/main.go", "go"},
		{"file:///project/app.ts", "typescript"},
		{"file:///project/app.tsx", "typescript"},
		{"file:///project/app.js", "javascript"},
		{"file:///project/app.jsx", "javascript"},
		{"file:///project/app.py", "python"},
		{"file:///project/lib.rs", "rust"},
		{"file:///project/Main.java", "java"},
		{"file:///project/util.cs", "csharp"},
		{"file:///project/Greeter.kt", "kotlin"},
		{"file:///project/Greeter.kts", "kotlin"},
		{"file:///project/lib.hs", "haskell"},
		{"file:///project/lib.lhs", "haskell"},
		{"file:///project/lib.rb", "ruby"},
		{"file:///project/lib.ml", "ocaml"},
		{"file:///project/lib.mli", "ocaml"},
		{"file:///project/main.c", "c"},
		{"file:///project/main.cpp", "cpp"},
		{"file:///project/main.cc", "cpp"},
		{"file:///project/main.cxx", "cpp"},
		{"file:///project/Makefile", "plaintext"},
		{"file:///project/README", "plaintext"},
		// URI with query/fragment
		{"file:///project/app.py?version=2", "python"},
		{"file:///project/app.go#L42", "go"},
	}
	for _, tc := range cases {
		t.Run(tc.uri, func(t *testing.T) {
			got := languageIDFromURI(tc.uri)
			if got != tc.want {
				t.Errorf("languageIDFromURI(%q) = %q, want %q", tc.uri, got, tc.want)
			}
		})
	}
}

// --- hasCapability ---

func TestHasCapability(t *testing.T) {
	c := NewLSPClient("fake", nil)
	// Set capabilities directly.
	c.capabilities = map[string]interface{}{
		"hoverProvider":      true,
		"completionProvider": map[string]interface{}{"triggerCharacters": []string{"."}},
		"renameProvider":     false,
		"nilValue":           nil,
	}

	cases := []struct {
		key  string
		want bool
	}{
		{"hoverProvider", true},
		{"completionProvider", true},   // non-nil map
		{"renameProvider", false},      // bool false
		{"nilValue", false},            // nil value
		{"missingKey", false},          // absent
	}
	for _, tc := range cases {
		got := c.hasCapability(tc.key)
		if got != tc.want {
			t.Errorf("hasCapability(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// --- GetCapabilities shallow copy ---

func TestGetCapabilities_ShallowCopy(t *testing.T) {
	c := NewLSPClient("fake", nil)
	c.capabilities["hoverProvider"] = true
	c.capabilities["definitionProvider"] = true

	caps := c.GetCapabilities()
	if len(caps) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(caps))
	}

	// Modifying the copy should not affect the original.
	caps["injected"] = true
	if _, found := c.capabilities["injected"]; found {
		t.Error("modifying GetCapabilities result should not affect internal state")
	}
}

// --- GetServerInfo ---

func TestGetServerInfo(t *testing.T) {
	c := NewLSPClient("fake", nil)
	// Before initialization, both should be empty.
	name, version := c.GetServerInfo()
	if name != "" || version != "" {
		t.Errorf("expected empty server info, got name=%q version=%q", name, version)
	}

	// Set server info.
	c.serverName = "gopls"
	c.serverVersion = "v0.15.0"
	name, version = c.GetServerInfo()
	if name != "gopls" {
		t.Errorf("expected name=gopls, got %q", name)
	}
	if version != "v0.15.0" {
		t.Errorf("expected version=v0.15.0, got %q", version)
	}
}

// --- RootDir ---

func TestRootDir(t *testing.T) {
	c := NewLSPClient("fake", nil)
	if c.RootDir() != "" {
		t.Errorf("expected empty RootDir, got %q", c.RootDir())
	}
	c.rootDir = "/workspace/project"
	if c.RootDir() != "/workspace/project" {
		t.Errorf("expected /workspace/project, got %q", c.RootDir())
	}
}

// --- GetOpenDocuments ---

func TestGetOpenDocuments_Empty(t *testing.T) {
	c := NewLSPClient("fake", nil)
	docs := c.GetOpenDocuments()
	if len(docs) != 0 {
		t.Errorf("expected empty docs, got %d", len(docs))
	}
}

func TestGetOpenDocuments_WithDocs(t *testing.T) {
	c := NewLSPClient("fake", nil)
	c.openDocs["file:///a.go"] = docMeta{filePath: "/a.go", languageID: "go"}
	c.openDocs["file:///b.py"] = docMeta{filePath: "/b.py", languageID: "python"}

	docs := c.GetOpenDocuments()
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	// Check both URIs are present (order not guaranteed).
	found := map[string]bool{}
	for _, d := range docs {
		found[d] = true
	}
	if !found["file:///a.go"] || !found["file:///b.py"] {
		t.Errorf("expected both URIs, got %v", docs)
	}
}

// --- warmup: diagnosticReceived before EnsureReady ---

func TestWarmup_DiagnosticReceivedBeforeEnsureReady(t *testing.T) {
	ws := newWarmupState()
	// Simulate diagnostic arriving before EnsureReady is called.
	ws.NotifyDiagnostic()
	if !ws.diagnosticReceived.Load() {
		t.Error("diagnosticReceived should be true")
	}
	// FirstRefTimeout should still return extended timeout (diagnostic alone
	// doesn't complete warmup; only EnsureReady or MarkReady does).
	if ws.FirstRefTimeout() == 0 {
		t.Error("FirstRefTimeout should be non-zero before EnsureReady completes")
	}
}

func TestWarmup_FirstRefDoneIndependent(t *testing.T) {
	ws := newWarmupState()
	// Set completed but not firstRefDone.
	ws.completed.Store(true)
	if ws.firstRefDone.Load() {
		t.Error("firstRefDone should not be set by completed alone")
	}
	// FirstRefTimeout returns 0 because completed is true.
	if ws.FirstRefTimeout() != 0 {
		t.Error("FirstRefTimeout should be 0 when completed is true")
	}
}

// --- getCapabilityRaw ---

func TestGetCapabilityRaw(t *testing.T) {
	c := NewLSPClient("fake", nil)
	c.capabilities["testKey"] = "testValue"
	c.capabilities["nilKey"] = nil

	got := c.getCapabilityRaw("testKey")
	if got != "testValue" {
		t.Errorf("expected testValue, got %v", got)
	}

	got = c.getCapabilityRaw("nilKey")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	got = c.getCapabilityRaw("missing")
	if got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

// --- isDocumentOpen ---

func TestIsDocumentOpen(t *testing.T) {
	c := NewLSPClient("fake", nil)
	if c.isDocumentOpen("file:///a.go") {
		t.Error("expected false for unopened document")
	}
	c.openDocs["file:///a.go"] = docMeta{filePath: "/a.go"}
	if !c.isDocumentOpen("file:///a.go") {
		t.Error("expected true for opened document")
	}
}
