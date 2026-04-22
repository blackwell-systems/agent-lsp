package lsp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// ---- test helpers ----

// writeMsg writes a Content-Length-framed JSON-RPC message to w.
func writeMsg(w io.Writer, v interface{}) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(EncodeMessage(body))
	return err
}

// newTestClient creates a minimal LSPClient connected to in-memory pipes.
// Returns the client, a writer to simulate server->client messages,
// and a reader to observe client->server output.
func newTestClient(t *testing.T) (*LSPClient, io.WriteCloser, io.ReadCloser) {
	t.Helper()
	// serverToClient: server writes here, client reads
	serverToClientR, serverToClientW := io.Pipe()
	// clientToServer: client writes here, we read to observe
	clientToServerR, clientToServerW := io.Pipe()

	c := NewLSPClient("", nil)
	c.stdin = clientToServerW
	c.frameReader = NewFrameReader(serverToClientR)

	go c.readLoop()

	t.Cleanup(func() {
		serverToClientW.Close()
		serverToClientR.Close()
		clientToServerW.Close()
		clientToServerR.Close()
	})

	return c, serverToClientW, clientToServerR
}

// readNextMsg reads the next framed message from r with a timeout.
func readNextMsg(t *testing.T, r io.Reader) map[string]interface{} {
	t.Helper()
	ch := make(chan map[string]interface{}, 1)
	go func() {
		fr := NewFrameReader(r)
		raw, err := fr.ReadMessage()
		if err != nil {
			ch <- nil
			return
		}
		var v map[string]interface{}
		json.Unmarshal(raw, &v)
		ch <- v
	}()
	select {
	case v := <-ch:
		return v
	case <-time.After(1 * time.Second):
		t.Error("readNextMsg: timeout")
		return nil
	}
}

// ---- tests ----

// TestLSPClient_ServerRequestHandling verifies that when the server sends
// window/workDoneProgress/create, the client pre-registers the token in
// progressTokens and responds null.
func TestLSPClient_ServerRequestHandling(t *testing.T) {
	c, serverW, clientR := newTestClient(t)

	id := 42
	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "window/workDoneProgress/create",
		"params":  map[string]interface{}{"token": "testToken"},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read the client's null response.
	resp := readNextMsg(t, clientR)
	if resp == nil {
		t.Fatal("expected response from client")
	}
	if resp["id"] != float64(id) {
		t.Errorf("expected id=%d, got %v", id, resp["id"])
	}
	// result should be null (nil in JSON)
	if _, hasResult := resp["result"]; !hasResult {
		t.Error("expected result field in response")
	}

	// Verify token was pre-registered.
	time.Sleep(20 * time.Millisecond)
	c.progressMu.Lock()
	_, ok := c.progressTokens["testToken"]
	c.progressMu.Unlock()
	if !ok {
		t.Error("expected testToken to be pre-registered in progressTokens")
	}
}

// TestLSPClient_ProgressTracking verifies that $/progress begin/end tokens
// update progressTokens correctly.
func TestLSPClient_ProgressTracking(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	// Send $/progress begin.
	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "$/progress",
		"params": map[string]interface{}{
			"token": "work1",
			"value": map[string]interface{}{"kind": "begin", "title": "Loading"},
		},
	}); err != nil {
		t.Fatalf("write begin: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	c.progressMu.Lock()
	if _, ok := c.progressTokens["work1"]; !ok {
		t.Error("expected work1 token after begin")
	}
	c.progressMu.Unlock()

	// Send $/progress end.
	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "$/progress",
		"params": map[string]interface{}{
			"token": "work1",
			"value": map[string]interface{}{"kind": "end"},
		},
	}); err != nil {
		t.Fatalf("write end: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	c.progressMu.Lock()
	if _, ok := c.progressTokens["work1"]; ok {
		t.Error("expected work1 token removed after end")
	}
	c.progressMu.Unlock()
}

// TestLSPClient_DocumentTracking verifies open/close tracking.
func TestLSPClient_DocumentTracking(t *testing.T) {
	c, serverW, clientR := newTestClient(t)
	_ = serverW

	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- c.OpenDocument(ctx, "file:///foo.go", "package main", "go")
	}()

	msg := readNextMsg(t, clientR)
	if msg == nil {
		t.Fatal("expected didOpen message")
	}
	if msg["method"] != "textDocument/didOpen" {
		t.Errorf("expected didOpen, got %v", msg["method"])
	}
	<-done

	if !c.isDocumentOpen("file:///foo.go") {
		t.Error("expected document to be open")
	}

	done2 := make(chan error, 1)
	go func() {
		done2 <- c.CloseDocument(ctx, "file:///foo.go")
	}()

	msg2 := readNextMsg(t, clientR)
	if msg2 == nil {
		t.Fatal("expected didClose message")
	}
	if msg2["method"] != "textDocument/didClose" {
		t.Errorf("expected didClose, got %v", msg2["method"])
	}
	<-done2

	if c.isDocumentOpen("file:///foo.go") {
		t.Error("expected document to be closed")
	}
}

// TestLSPClient_PublishDiagnostics verifies diagnostic storage and subscription.
func TestLSPClient_PublishDiagnostics(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	var mu sync.Mutex
	var gotURI string
	var gotCount int
	received := make(chan struct{}, 1)

	cb := types.DiagnosticUpdateCallback(func(uri string, diags []types.LSPDiagnostic) {
		mu.Lock()
		gotURI = uri
		gotCount = len(diags)
		mu.Unlock()
		select {
		case received <- struct{}{}:
		default:
		}
	})
	c.SubscribeToDiagnostics(cb)

	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]interface{}{
			"uri": "file:///bar.go",
			"diagnostics": []interface{}{
				map[string]interface{}{
					"range": map[string]interface{}{
						"start": map[string]interface{}{"line": 0, "character": 0},
						"end":   map[string]interface{}{"line": 0, "character": 5},
					},
					"severity": 1,
					"message":  "undefined: foo",
				},
			},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-received:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for diagnostic callback")
	}

	mu.Lock()
	u := gotURI
	cnt := gotCount
	mu.Unlock()

	if u != "file:///bar.go" {
		t.Errorf("expected uri file:///bar.go, got %s", u)
	}
	if cnt != 1 {
		t.Errorf("expected 1 diagnostic, got %d", cnt)
	}

	diags := c.GetDiagnostics("file:///bar.go")
	if len(diags) != 1 {
		t.Errorf("GetDiagnostics: expected 1, got %d", len(diags))
	}
	if diags[0].Message != "undefined: foo" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

// TestLSPClient_UnsubscribeFromDiagnostics verifies that callbacks can be removed.
func TestLSPClient_UnsubscribeFromDiagnostics(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	var mu sync.Mutex
	count := 0
	cb := types.DiagnosticUpdateCallback(func(uri string, diags []types.LSPDiagnostic) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	c.SubscribeToDiagnostics(cb)
	c.UnsubscribeFromDiagnostics(cb)

	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]interface{}{
			"uri":         "file:///baz.go",
			"diagnostics": []interface{}{},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	c2 := count
	mu.Unlock()
	if c2 != 0 {
		t.Errorf("expected callback not called after unsubscribe, got count=%d", c2)
	}
}

// TestLSPClient_RequestResponse verifies basic request/response correlation.
func TestLSPClient_RequestResponse(t *testing.T) {
	c, serverW, clientR := newTestClient(t)

	ctx := context.Background()

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		// Temporarily add hover capability.
		c.capsMu.Lock()
		c.capabilities["hoverProvider"] = true
		c.capsMu.Unlock()
		r, err := c.sendRequest(ctx, "textDocument/hover", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": "file:///x.go"},
			"position":     map[string]interface{}{"line": 0, "character": 0},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- r
	}()

	// Read the outgoing request.
	reqMsg := readNextMsg(t, clientR)
	if reqMsg == nil {
		t.Fatal("expected request from client")
	}
	id := reqMsg["id"]

	// Server responds.
	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  map[string]interface{}{"contents": "hover text"},
	}); err != nil {
		t.Fatalf("write response: %v", err)
	}

	select {
	case result := <-resultCh:
		if !strings.Contains(string(result), "hover text") {
			t.Errorf("expected hover text in result, got %s", result)
		}
	case err := <-errCh:
		t.Fatalf("request error: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for response")
	}
}

// TestLSPClient_WorkspaceConfiguration verifies that workspace/configuration
// requests are answered with an array of empty objects (one per item).
// Empty objects ({}) instead of null are critical for servers like jdtls
// that interpret null as "no configuration" and skip project import.
func TestLSPClient_WorkspaceConfiguration(t *testing.T) {
	c, serverW, clientR := newTestClient(t)
	_ = c

	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      99,
		"method":  "workspace/configuration",
		"params": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"section": "go"},
				map[string]interface{}{"section": "editor"},
			},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp := readNextMsg(t, clientR)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp["id"] != float64(99) {
		t.Errorf("expected id=99, got %v", resp["id"])
	}
	result, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected array result, got %T: %v", resp["result"], resp["result"])
	}
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
	for i, item := range result {
		obj, ok := item.(map[string]interface{})
		if !ok {
			t.Errorf("result[%d]: expected empty object, got %T: %v", i, item, item)
			continue
		}
		if len(obj) != 0 {
			t.Errorf("result[%d]: expected empty object, got %v", i, obj)
		}
	}
}

// TestLSPClient_GetOpenDocuments verifies the open document list.
func TestLSPClient_GetOpenDocuments(t *testing.T) {
	c, serverW, clientR := newTestClient(t)
	_ = serverW

	ctx := context.Background()
	uris := []string{"file:///a.go", "file:///b.go", "file:///c.go"}
	for _, uri := range uris {
		done := make(chan struct{})
		go func(u string) {
			defer close(done)
			c.OpenDocument(ctx, u, "package main", "go")
		}(uri)
		readNextMsg(t, clientR) // consume didOpen
		<-done
	}

	open := c.GetOpenDocuments()
	if len(open) != len(uris) {
		t.Errorf("expected %d open docs, got %d", len(uris), len(open))
	}
}

func TestLanguageIDFromURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///foo/bar.go", "go"},
		{"file:///foo/bar.ts", "typescript"},
		{"file:///foo/bar.py", "python"},
		{"file:///foo/bar.unknown", "plaintext"},
		{"file:///foo/Makefile", "plaintext"},
	}
	for _, tt := range tests {
		got := languageIDFromURI(tt.uri)
		if got != tt.want {
			t.Errorf("languageIDFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
