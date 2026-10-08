package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// ---- test helpers ----

// writeMsg writes a Content-Length-framed JSON-RPC message to w.
func writeMsg(w io.Writer, v any) error {
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

	go c.readLoop(c.frameReader)

	t.Cleanup(func() {
		serverToClientW.Close()
		serverToClientR.Close()
		clientToServerW.Close()
		clientToServerR.Close()
	})

	return c, serverToClientW, clientToServerR
}

// readNextMsg reads the next framed message from r with a timeout.
func readNextMsg(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	ch := make(chan map[string]any, 1)
	go func() {
		fr := NewFrameReader(r)
		raw, err := fr.ReadMessage()
		if err != nil {
			ch <- nil
			return
		}
		var v map[string]any
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
	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "window/workDoneProgress/create",
		"params":  map[string]any{"token": "testToken"},
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
	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "$/progress",
		"params": map[string]any{
			"token": "work1",
			"value": map[string]any{"kind": "begin", "title": "Loading"},
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
	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "$/progress",
		"params": map[string]any{
			"token": "work1",
			"value": map[string]any{"kind": "end"},
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

	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]any{
			"uri": "file:///bar.go",
			"diagnostics": []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 5},
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

// TestLSPClient_HasPublishedDiagnostics_FalseWithoutPublish verifies that a
// document that never received a publishDiagnostics notification is reported as
// unverified. (issue #44)
func TestLSPClient_HasPublishedDiagnostics_FalseWithoutPublish(t *testing.T) {
	c, _, _ := newTestClient(t)

	if c.HasPublishedDiagnostics("file:///never.go") {
		t.Error("expected HasPublishedDiagnostics=false before any publish")
	}
}

// TestLSPClient_HasPublishedDiagnostics_TrueAfterPublish verifies that receiving
// a publishDiagnostics notification marks that document as verified while other
// documents remain unverified. (issue #44)
func TestLSPClient_HasPublishedDiagnostics_TrueAfterPublish(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	received := make(chan struct{}, 1)
	cb := types.DiagnosticUpdateCallback(func(string, []types.LSPDiagnostic) {
		select {
		case received <- struct{}{}:
		default:
		}
	})
	c.SubscribeToDiagnostics(cb)
	defer c.UnsubscribeFromDiagnostics(cb)

	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]any{
			"uri": "file:///published.go",
			"diagnostics": []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 1},
					},
					"severity": 1,
					"message":  "boom",
				},
			},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for publishDiagnostics")
	}

	if !c.HasPublishedDiagnostics("file:///published.go") {
		t.Error("expected HasPublishedDiagnostics=true after publish")
	}
	if c.HasPublishedDiagnostics("file:///other.go") {
		t.Error("expected HasPublishedDiagnostics=false for a document with no publish")
	}
}

// TestLSPClient_HasPublishedDiagnostics_EmptyPublishCounts verifies that a
// publish carrying an empty diagnostics array still counts as a delivered
// notification (the live-channel-empty case). (issue #44)
func TestLSPClient_HasPublishedDiagnostics_EmptyPublishCounts(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	received := make(chan struct{}, 1)
	cb := types.DiagnosticUpdateCallback(func(string, []types.LSPDiagnostic) {
		select {
		case received <- struct{}{}:
		default:
		}
	})
	c.SubscribeToDiagnostics(cb)
	defer c.UnsubscribeFromDiagnostics(cb)

	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]any{
			"uri":         "file:///clean.go",
			"diagnostics": []any{},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for publishDiagnostics")
	}

	if !c.HasPublishedDiagnostics("file:///clean.go") {
		t.Error("expected HasPublishedDiagnostics=true for an empty publish")
	}
	if len(c.GetDiagnostics("file:///clean.go")) != 0 {
		t.Error("expected no diagnostics for an empty publish")
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

	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]any{
			"uri":         "file:///baz.go",
			"diagnostics": []any{},
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
		r, err := c.sendRequest(ctx, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": "file:///x.go"},
			"position":     map[string]any{"line": 0, "character": 0},
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
	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  map[string]any{"contents": "hover text"},
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

	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"id":      99,
		"method":  "workspace/configuration",
		"params": map[string]any{
			"items": []any{
				map[string]any{"section": "go"},
				map[string]any{"section": "editor"},
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
	result, ok := resp["result"].([]any)
	if !ok {
		t.Fatalf("expected array result, got %T: %v", resp["result"], resp["result"])
	}
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
	for i, item := range result {
		obj, ok := item.(map[string]any)
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

// TestLSPClient_LanguageIDForFile_KnownExtension tests that LanguageIDForFile
// delegates to languageIDFromPath for known extensions.
func TestLSPClient_LanguageIDForFile_KnownExtension(t *testing.T) {
	c := NewLSPClient("fake", nil)
	tests := []struct {
		path string
		want string
	}{
		{"/project/main.go", "go"},
		{"/project/app.ts", "typescript"},
		{"/project/app.py", "python"},
		{"/project/main.rs", "rust"},
		{"/project/script.luau", "luau"}, // luau is in client.go's built-in map
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := c.LanguageIDForFile(tt.path)
			if got != tt.want {
				t.Errorf("LanguageIDForFile(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
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

// TestPullDiagnostics_FullReport verifies that PullDiagnostics issues a
// textDocument/diagnostic request and extracts the items from a "full" report.
// (issue #43)
func TestPullDiagnostics_FullReport(t *testing.T) {
	c, serverW, clientR := newTestClient(t)

	// A server-declared diagnosticProvider is an object, not a bool; the
	// capability check must accept both shapes.
	c.capsMu.Lock()
	c.capabilities["diagnosticProvider"] = map[string]any{"identifier": "test"}
	c.capsMu.Unlock()

	type reply struct {
		diags []types.LSPDiagnostic
		err   error
	}
	done := make(chan reply, 1)
	go func() {
		diags, err := c.PullDiagnostics(context.Background(), "file:///x.go")
		done <- reply{diags, err}
	}()

	req := readNextMsg(t, clientR)
	if req == nil {
		t.Fatal("expected diagnostic request")
	}
	if req["method"] != "textDocument/diagnostic" {
		t.Errorf("expected textDocument/diagnostic, got %v", req["method"])
	}
	params, _ := req["params"].(map[string]any)
	td, _ := params["textDocument"].(map[string]any)
	if td["uri"] != "file:///x.go" {
		t.Errorf("expected textDocument.uri in params, got %v", params)
	}

	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0",
		"id":      req["id"],
		"result": map[string]any{
			"kind": "full",
			"items": []any{
				map[string]any{
					"range":    map[string]any{"start": map[string]any{"line": 0, "character": 0}, "end": map[string]any{"line": 0, "character": 1}},
					"severity": 1,
					"message":  "boom",
				},
				map[string]any{
					"range":    map[string]any{"start": map[string]any{"line": 1, "character": 0}, "end": map[string]any{"line": 1, "character": 1}},
					"severity": 2,
					"message":  "warn",
				},
			},
		},
	}); err != nil {
		t.Fatalf("write response: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("PullDiagnostics error: %v", r.err)
		}
		if len(r.diags) != 2 {
			t.Fatalf("expected 2 diagnostics, got %d: %+v", len(r.diags), r.diags)
		}
		if r.diags[0].Message != "boom" || r.diags[1].Message != "warn" {
			t.Errorf("unexpected diagnostics: %+v", r.diags)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PullDiagnostics")
	}
}

// TestPullDiagnostics_NonFullReportIsUnverified verifies that an answer without
// a "full" report (an "unchanged" report, an unknown kind, or null) returns
// ErrPullDiagnosticsNoReport instead of an empty list. An empty list reads as
// "verified clean" to get_diagnostics, which would report "No errors. Safe to
// proceed." for a document nothing verified. (issue #43)
func TestPullDiagnostics_NonFullReportIsUnverified(t *testing.T) {
	for name, result := range map[string]any{
		"unchanged":    map[string]any{"kind": "unchanged", "resultId": "1"},
		"unknown kind": map[string]any{"kind": "partial"},
		"null":         nil,
	} {
		t.Run(name, func(t *testing.T) {
			c, serverW, clientR := newTestClient(t)
			c.capsMu.Lock()
			c.capabilities["diagnosticProvider"] = true
			c.capsMu.Unlock()

			type reply struct {
				diags []types.LSPDiagnostic
				err   error
			}
			done := make(chan reply, 1)
			go func() {
				diags, err := c.PullDiagnostics(context.Background(), "file:///x.go")
				done <- reply{diags, err}
			}()

			req := readNextMsg(t, clientR)
			if req == nil {
				t.Fatal("expected diagnostic request")
			}
			if err := writeMsg(serverW, map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result":  result,
			}); err != nil {
				t.Fatalf("write response: %v", err)
			}

			select {
			case r := <-done:
				if !errors.Is(r.err, ErrPullDiagnosticsNoReport) {
					t.Fatalf("err = %v, want ErrPullDiagnosticsNoReport", r.err)
				}
				if r.diags != nil {
					t.Errorf("expected no diagnostics, got %+v", r.diags)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for PullDiagnostics")
			}
		})
	}
}

// TestPullDiagnostics_UnsupportedDoesNotSend verifies that a server without the
// diagnosticProvider capability returns the sentinel error without sending a
// request at all. (issue #43)
func TestPullDiagnostics_UnsupportedDoesNotSend(t *testing.T) {
	c, _, clientR := newTestClient(t)

	_, err := c.PullDiagnostics(context.Background(), "file:///x.go")
	if !errors.Is(err, ErrPullDiagnosticsUnsupported) {
		t.Fatalf("expected ErrPullDiagnosticsUnsupported, got %v", err)
	}

	c.pendingMu.Lock()
	pending := len(c.pending)
	c.pendingMu.Unlock()
	if pending != 0 {
		t.Errorf("expected no pending request, got %d", pending)
	}

	// Confirm nothing was written to the server as well.
	sent := make(chan struct{})
	go func() {
		fr := NewFrameReader(clientR)
		if _, rerr := fr.ReadMessage(); rerr == nil {
			close(sent)
		}
	}()
	select {
	case <-sent:
		t.Error("expected no textDocument/diagnostic request when unsupported")
	case <-time.After(150 * time.Millisecond):
	}
}

// TestPullDiagnostics_ContextDeadline verifies that a hanging server does not
// block the client: the caller's context deadline bounds the request, the
// pending entry is cleaned up, and the error is the context error. A pull must
// never wedge the session. (issue #43)
func TestPullDiagnostics_ContextDeadline(t *testing.T) {
	c, _, clientR := newTestClient(t)
	c.capsMu.Lock()
	c.capabilities["diagnosticProvider"] = true
	c.capsMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := c.PullDiagnostics(ctx, "file:///hang.go")
		errCh <- err
	}()

	// Consume the outgoing request so the synchronous pipe write does not block,
	// then deliberately never answer it.
	if req := readNextMsg(t, clientR); req == nil {
		t.Fatal("expected diagnostic request")
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PullDiagnostics did not honor the caller context deadline")
	}

	// A timed-out pull must not poison the session: the pending entry is removed
	// so a later response (or request) is not misrouted.
	c.pendingMu.Lock()
	pending := len(c.pending)
	c.pendingMu.Unlock()
	if pending != 0 {
		t.Errorf("expected pending requests to be cleaned up after timeout, got %d", pending)
	}
}

// TestWriteRawAfterProcessExit verifies that once the subprocess exits, the
// exit-monitor goroutine nulls stdin and sets exited, so a later request
// returns a clean "LSP process has exited" error rather than the confusing
// low-level "file already closed" pipe write error.
func TestWriteRawAfterProcessExit(t *testing.T) {
	// Spawn a process that exits immediately so cmd.Wait returns quickly and
	// the exit monitor runs. No real LSP handshake is involved.
	c := NewLSPClient("/bin/sh", []string{"-c", "exit 0"})
	if err := c.start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait (bounded) for the exit monitor goroutine to observe the exit and
	// flip the exited flag under c.mu.
	deadline := time.Now().Add(3 * time.Second)
	for {
		c.mu.Lock()
		exited := c.exited
		c.mu.Unlock()
		if exited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for exit monitor to set exited")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// A request routed through writeRaw must now surface a clear error.
	_, err := c.SendRequest(context.Background(), "textDocument/hover", nil)
	if err == nil {
		t.Fatal("expected an error after process exit, got nil")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("error %q should mention that the process exited", err.Error())
	}
	if strings.Contains(err.Error(), "file already closed") {
		t.Errorf("error %q should not leak the low-level closed-pipe error", err.Error())
	}
}

// TestResetDiagnostics verifies that ResetDiagnostics removes the cached
// diagnostics for the URI (including under its normalized form), so post-edit
// verification observes only freshly published notifications. (issue #44
// review follow-up)
func TestResetDiagnostics(t *testing.T) {
	c, _, _ := newTestClient(t)
	uri := "file:///some/file.go"

	c.diagMu.Lock()
	c.diags[uri] = []types.LSPDiagnostic{{Severity: 1}}
	c.diagMu.Unlock()

	if !c.HasPublishedDiagnostics(uri) {
		t.Fatal("expected cached diagnostics before reset")
	}

	c.ResetDiagnostics(uri)
	if c.HasPublishedDiagnostics(uri) {
		t.Error("expected cached diagnostics to be removed after ResetDiagnostics")
	}
	if got := c.GetDiagnostics(uri); len(got) != 0 {
		t.Errorf("expected empty diagnostics after reset, got %d", len(got))
	}

	// The normalized form of the URI must be cleared too.
	c.diagMu.Lock()
	c.diags[NormalizeFileURI(uri)] = []types.LSPDiagnostic{{Severity: 2}}
	c.diagMu.Unlock()
	c.ResetDiagnostics(uri)
	if c.HasPublishedDiagnostics(uri) {
		t.Error("expected normalized cached diagnostics to be removed after ResetDiagnostics")
	}
}

// --- HasCapability exported wrapper (issue #42) ---

func TestHasCapabilityExported(t *testing.T) {
	c, _, _ := newTestClient(t)

	if c.HasCapability("workspaceSymbolProvider") {
		t.Fatal("expected false before any capability is registered")
	}

	// A server-declared provider can be an object, not a bool; HasCapability
	// must accept both shapes, matching hasCapability semantics.
	c.capsMu.Lock()
	c.capabilities["workspaceSymbolProvider"] = map[string]any{}
	c.capabilities["referencesProvider"] = true
	c.capsMu.Unlock()

	if !c.HasCapability("workspaceSymbolProvider") {
		t.Error("expected true for object-valued provider")
	}
	if !c.HasCapability("referencesProvider") {
		t.Error("expected true for bool-valued provider")
	}
	if c.HasCapability("typeHierarchyProvider") {
		t.Error("expected false for undeclared capability")
	}
}
