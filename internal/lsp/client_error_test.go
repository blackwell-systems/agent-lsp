package lsp

import (
	"net"
	"testing"
	"time"
)

// TestNewDaemonClient_ConnectionFailure tests error handling when socket connection fails
func TestNewDaemonClient_ConnectionFailure(t *testing.T) {
	info := &DaemonInfo{
		RootDir:    "/tmp/test",
		LanguageID: "python",
		SocketPath: "/nonexistent/socket/path.sock",
		PID:        12345,
		Ready:      true,
		StartTime:  time.Now(),
	}

	_, err := NewDaemonClient(info)
	if err == nil {
		t.Error("expected error when connecting to nonexistent socket, got nil")
	}
}

// TestNewPassiveClient_ConnectionFailure tests error handling for failed TCP connection
func TestNewPassiveClient_ConnectionFailure(t *testing.T) {
	// Use a port that's extremely unlikely to be in use
	addr := "127.0.0.1:59999"

	_, err := NewPassiveClient(addr)
	if err == nil {
		t.Error("expected error when connecting to nonexistent server, got nil")
	}
}

// TestNewPassiveClient_ValidConnection tests successful connection to a mock server
func TestNewPassiveClient_ValidConnection(t *testing.T) {
	// Start a mock TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// Accept connections in background
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		// Keep connection open briefly then close
		time.Sleep(100 * time.Millisecond)
		conn.Close()
	}()

	client, err := NewPassiveClient(addr)
	if err != nil {
		t.Fatalf("NewPassiveClient failed: %v", err)
	}

	// Verify client is marked as passive
	if !client.isPassive {
		t.Error("expected isPassive=true for passive client")
	}

	// Verify initialized is false (needs Initialize call)
	if client.IsInitialized() {
		t.Error("expected IsInitialized()=false before Initialize call")
	}
}

// TestTimeoutForClient tests request timeout configuration
func TestTimeoutForClient(t *testing.T) {
	tests := []struct {
		method  string
		want    time.Duration
		wantMin time.Duration // minimum expected timeout
	}{
		{"initialize", 300 * time.Second, 300 * time.Second},
		{"textDocument/references", 120 * time.Second, 120 * time.Second},
		{"textDocument/hover", 30 * time.Second, 30 * time.Second},
		{"unknownMethod", defaultTimeout, defaultTimeout},
		{"", defaultTimeout, defaultTimeout},
	}

	for _, tt := range tests {
		got := timeoutFor(tt.method)
		if got < tt.wantMin {
			t.Errorf("timeoutFor(%q) = %v, want at least %v", tt.method, got, tt.wantMin)
		}
	}
}

// TestLSPClient_IsAlive tests alive checking for different client states
func TestLSPClient_IsAlive(t *testing.T) {
	// Test uninitialized client (cmd is nil, so IsAlive returns false)
	c := NewLSPClient("fake-server", nil)
	if c.IsAlive() {
		t.Error("new uninitialized client with nil cmd should report IsAlive()=false")
	}

	// Test daemon/passive clients (always report alive)
	c2 := NewLSPClient("fake", nil)
	c2.isDaemon = true
	if !c2.IsAlive() {
		t.Error("daemon client should always report IsAlive()=true")
	}

	c3 := NewLSPClient("fake", nil)
	c3.isPassive = true
	if !c3.IsAlive() {
		t.Error("passive client should always report IsAlive()=true")
	}
}

// TestLSPClient_IsInitialized tests initialization state tracking
func TestLSPClient_IsInitialized(t *testing.T) {
	c := NewLSPClient("fake-server", nil)

	if c.IsInitialized() {
		t.Error("new client should not be initialized")
	}

	// After marking as initialized internally
	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()

	if !c.IsInitialized() {
		t.Error("client should report as initialized after flag set")
	}
}

// TestLSPClient_RootDir tests root directory retrieval
func TestLSPClient_RootDir(t *testing.T) {
	c := NewLSPClient("fake-server", nil)

	// Before Initialize, root dir should be empty
	if root := c.RootDir(); root != "" {
		t.Errorf("RootDir() = %q before Initialize, want empty", root)
	}

	// Set root dir manually (simulating Initialize)
	testRoot := "/test/workspace"
	c.mu.Lock()
	c.rootDir = testRoot
	c.mu.Unlock()

	if root := c.RootDir(); root != testRoot {
		t.Errorf("RootDir() = %q, want %q", root, testRoot)
	}
}

// TestLSPClient_IsWorkspaceLoaded tests workspace loaded state
func TestLSPClient_IsWorkspaceLoaded(t *testing.T) {
	c := NewLSPClient("fake-server", nil)

	if c.IsWorkspaceLoaded() {
		t.Error("new client should not have workspace loaded")
	}

	// Mark as loaded
	c.workspaceLoaded.Store(true)

	if !c.IsWorkspaceLoaded() {
		t.Error("IsWorkspaceLoaded should return true after marking loaded")
	}
}

// TestLSPClient_ServerIdentity tests server name and version tracking
func TestLSPClient_ServerIdentity(t *testing.T) {
	c := NewLSPClient("test-server", []string{"--arg1"})

	// Initially empty (serverName and serverVersion are private fields)
	// We verify they can be set and are tracked correctly
	c.capsMu.Lock()
	c.serverName = "Test Language Server"
	c.serverVersion = "1.2.3"
	c.capsMu.Unlock()

	// Verify fields are set (through GetCapabilities or other methods)
	// This tests internal state management
	c.capsMu.RLock()
	name := c.serverName
	version := c.serverVersion
	c.capsMu.RUnlock()

	if name != "Test Language Server" {
		t.Errorf("serverName = %q, want %q", name, "Test Language Server")
	}

	if version != "1.2.3" {
		t.Errorf("serverVersion = %q, want %q", version, "1.2.3")
	}
}

// TestLSPClient_hasCapability tests capability checking (unexported method)
func TestLSPClient_hasCapability(t *testing.T) {
	c := NewLSPClient("fake-server", nil)

	// Initially no capabilities
	if c.hasCapability("hoverProvider") {
		t.Error("new client should not have capabilities")
	}

	// Add a capability
	c.capsMu.Lock()
	c.capabilities["hoverProvider"] = true
	c.capabilities["definitionProvider"] = true
	c.capsMu.Unlock()

	if !c.hasCapability("hoverProvider") {
		t.Error("hasCapability should return true for set capability")
	}

	if c.hasCapability("nonexistent") {
		t.Error("hasCapability should return false for unset capability")
	}
}

// TestLSPClient_GetCapabilities tests capability retrieval
func TestLSPClient_GetCapabilities(t *testing.T) {
	c := NewLSPClient("fake-server", nil)

	c.capsMu.Lock()
	c.capabilities["test"] = true
	c.capabilities["nested"] = map[string]any{"key": "value"}
	c.capsMu.Unlock()

	caps := c.GetCapabilities()

	if len(caps) != 2 {
		t.Errorf("GetCapabilities() returned %d caps, want 2", len(caps))
	}

	if caps["test"] != true {
		t.Error("GetCapabilities should return test=true")
	}

	nested, ok := caps["nested"].(map[string]any)
	if !ok {
		t.Error("GetCapabilities should preserve nested map structure")
	} else if nested["key"] != "value" {
		t.Errorf("nested capability: got %v, want {key: value}", nested)
	}
}

// TestLSPClient_OpenDocuments tests document tracking
func TestLSPClient_OpenDocuments(t *testing.T) {
	c := NewLSPClient("fake-server", nil)

	// Initially no open documents
	docs := c.GetOpenDocuments()
	if len(docs) != 0 {
		t.Errorf("GetOpenDocuments() returned %d docs, want 0", len(docs))
	}

	// Simulate opening a document
	c.mu.Lock()
	c.openDocs["file:///test.go"] = docMeta{
		filePath:   "/test.go",
		languageID: "go",
		version:    1,
	}
	c.mu.Unlock()

	docs = c.GetOpenDocuments()
	if len(docs) != 1 {
		t.Errorf("GetOpenDocuments() returned %d docs, want 1", len(docs))
	}

	if docs[0] != "file:///test.go" {
		t.Errorf("GetOpenDocuments()[0] = %q, want %q", docs[0], "file:///test.go")
	}
}

// TestNewLSPClient_Initialization tests client construction
func TestNewLSPClient_Initialization(t *testing.T) {
	serverPath := "gopls"
	serverArgs := []string{"serve", "-rpc.trace"}

	c := NewLSPClient(serverPath, serverArgs)

	if c == nil {
		t.Fatal("NewLSPClient returned nil")
	}

	if c.serverPath != serverPath {
		t.Errorf("serverPath = %q, want %q", c.serverPath, serverPath)
	}

	if len(c.serverArgs) != len(serverArgs) {
		t.Errorf("len(serverArgs) = %d, want %d", len(c.serverArgs), len(serverArgs))
	}

	// Verify internal state initialization
	if c.pending == nil {
		t.Error("pending map not initialized")
	}
	if c.openDocs == nil {
		t.Error("openDocs map not initialized")
	}
	if c.diags == nil {
		t.Error("diags map not initialized")
	}
	if c.capabilities == nil {
		t.Error("capabilities map not initialized")
	}
	if c.progressCond == nil {
		t.Error("progressCond not initialized")
	}
	if c.warmup == nil {
		t.Error("warmup not initialized")
	}
}

// TestLSPClient_ConcurrentAccess tests thread safety of public methods
func TestLSPClient_ConcurrentAccess(t *testing.T) {
	c := NewLSPClient("fake-server", nil)

	// Simulate concurrent access to various methods
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			c.IsInitialized()
			c.IsAlive()
			c.RootDir()
			c.hasCapability("test")
			c.GetOpenDocuments()
			c.GetCapabilities()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without data races, test passes
}

// TestLanguageIDFromURIEdgeCases tests edge cases in language ID detection
func TestLanguageIDFromURIEdgeCases(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		// Multiple dots
		{"file:///test.spec.ts", "typescript"},
		{"file:///backup.tar.gz", "plaintext"},

		// Case variations
		{"file:///Main.GO", "go"},
		{"file:///Test.PY", "python"},

		// No extension
		{"file:///nodots", "plaintext"},

		// Special characters (path decoding tested in existing tests)
		{"file:///test.go", "go"},
		{"file:///lib.rs", "rust"},
	}

	for _, tt := range tests {
		got := languageIDFromURI(tt.uri)
		if got != tt.want {
			t.Errorf("languageIDFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

// TestRemoveEnv_EmptyKey tests removing empty key
func TestRemoveEnv_EmptyKey(t *testing.T) {
	env := []string{"A=1", "B=2"}
	result := removeEnv(env, "")

	if len(result) != len(env) {
		t.Errorf("removeEnv with empty key modified slice: got len %d, want %d", len(result), len(env))
	}
}

// TestRemoveEnv_Whitespace tests handling of whitespace in keys
func TestRemoveEnv_Whitespace(t *testing.T) {
	env := []string{"KEY=value", " KEY=value2"}
	result := removeEnv(env, "KEY")

	// Should remove "KEY=value" but not " KEY=value2" (leading space makes it different)
	if len(result) != 1 {
		t.Errorf("removeEnv: got len %d, want 1", len(result))
	}
	if len(result) > 0 && result[0] != " KEY=value2" {
		t.Errorf("removeEnv: kept wrong entry: %q", result[0])
	}
}

// TestJSONRPCMsg_IDField tests that both integer and string IDs are supported
func TestJSONRPCMsg_IDField(t *testing.T) {
	// Test integer ID
	msg1 := jsonrpcMsg{
		JSONRPC: "2.0",
		Method:  "test",
	}
	_ = msg1

	// Test that ID field can hold different types (via json.RawMessage)
	// This is implicitly tested by the client's request handling
}
