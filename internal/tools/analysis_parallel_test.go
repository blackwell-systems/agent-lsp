package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// startHangingLSP starts a one-connection fake LSP server on a local port and
// returns a passive client connected to it, marked initialized. declared
// lists methods to register dynamically (so HasCapability passes without a
// real initialize handshake); workspace/symbol requests are accepted but
// never answered — the "slow server" of the fan-out cap test.
func startHangingLSP(t *testing.T, declared []string) *lsp.LSPClient {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if len(declared) > 0 {
			regs := make([]map[string]any, 0, len(declared))
			for i, m := range declared {
				regs = append(regs, map[string]any{"id": fmt.Sprintf("r%d", i), "method": m})
			}
			regBody, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"method": "client/registerCapability",
				"params": map[string]any{"registrations": regs},
			})
			if _, err := conn.Write(lsp.EncodeMessage(regBody)); err != nil {
				return
			}
		}
		r := lsp.NewFrameReader(conn)
		for {
			raw, err := r.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				ID     *float64 `json:"id"`
				Method string   `json:"method"`
			}
			if err := json.Unmarshal(raw, &msg); err != nil {
				return
			}
			if msg.ID == nil || msg.Method == "" {
				continue
			}
			if msg.Method == "workspace/symbol" {
				// The hang under test: accept the request, never answer.
				continue
			}
			body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
			if _, err := conn.Write(lsp.EncodeMessage(body)); err != nil {
				return
			}
		}
	}()
	client, err := lsp.NewPassiveClient(ln.Addr().String())
	if err != nil {
		t.Fatalf("NewPassiveClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	client.MarkInitializedForTest()
	if len(declared) > 0 {
		// Same registration wait as startFakeLSP: the registration arrives
		// asynchronously and the silent capability gate in
		// GetWorkspaceSymbols would otherwise swallow the query.
		deadline := time.Now().Add(2 * time.Second)
		for !client.HasCapability("workspaceSymbolProvider") {
			if time.Now().After(deadline) {
				t.Fatal("capability registration never arrived; the gate would return an empty result for the wrong reason")
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	return client
}

// The multi-server fan-out must query servers concurrently under a
// per-server timeout cap: one hanging server must degrade to a "did not
// answer" provenance note naming it, not stall the aggregate for its full
// (60s workspace-ready + 30s request timeout) worst case.
// (CodeRabbit #59 review thread 5)
func TestHandleGetWorkspaceSymbolsMulti_ParallelFanOutBoundedByPerServerCap(t *testing.T) {
	oldCap := perServerQueryTimeout
	perServerQueryTimeout = 300 * time.Millisecond
	defer func() { perServerQueryTimeout = oldCap }()

	fast := startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Alpha")}, false)
	hanging := startHangingLSP(t, []string{"workspace/symbol"})

	start := time.Now()
	r, err := HandleGetWorkspaceSymbolsMulti(context.Background(),
		[]*lsp.LSPClient{fast, hanging}, map[string]any{"query": "x"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error result: %v", r.Content)
	}
	// The cap bounds the whole fan-out because the servers run
	// concurrently: the run must finish near the 300ms cap, nowhere near the
	// hanging server's 30s internal request timeout (or the sum of both
	// waits under sequencing).
	if elapsed > 5*time.Second {
		t.Fatalf("fan-out took %s; the per-server cap (%s) must bound the aggregate", elapsed, perServerQueryTimeout)
	}
	all := contentText(r)
	if !strings.Contains(all, "did not answer") {
		t.Errorf("expected did-not-answer provenance note, got %q", all)
	}
	if !strings.Contains(all, "server-1") {
		t.Errorf("did-not-answer note must name the hanging server, got %q", all)
	}
	var syms []map[string]any
	if err := json.Unmarshal([]byte(r.Content[0].Text), &syms); err != nil {
		t.Fatalf("payload is not a symbol array: %v (%q)", err, r.Content[0].Text)
	}
	if len(syms) != 1 || syms[0]["name"] != "Alpha" || syms[0]["server"] != "server-0" {
		t.Fatalf("expected the fast (default) server's Alpha tagged server-0, got %v", syms)
	}
}
