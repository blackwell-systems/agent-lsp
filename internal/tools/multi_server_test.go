package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gcf "github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- find_symbol multi-server fan-out (issue #2) ---

// Client sets with no initialized client must produce the standard
// not-initialized error, never a successful-looking "No matches" hint.
// (CodeRabbit #59 thread 2)
func TestHandleGetWorkspaceSymbolsMulti_NoInitializedClient(t *testing.T) {
	cases := []struct {
		name    string
		clients []*lsp.LSPClient
	}{
		{"nil slice", nil},
		{"empty slice", []*lsp.LSPClient{}},
		{"single nil", []*lsp.LSPClient{nil}},
		{"single fresh", []*lsp.LSPClient{lsp.NewLSPClient("fake", nil)}},
		{"mixed nil and fresh", []*lsp.LSPClient{nil, lsp.NewLSPClient("fake", nil), nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := HandleGetWorkspaceSymbolsMulti(context.Background(), tc.clients, map[string]any{"query": "x"})
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !r.IsError || !strings.Contains(contentText(r), "start_lsp") {
				t.Fatalf("expected not-initialized error, got %v", r.Content)
			}
		})
	}
}

// startFakeLSP starts a one-connection fake LSP server on a local port and
// returns a passive client connected to it, marked initialized. declared
// lists methods to register dynamically (so HasCapability passes without a
// real initialize handshake); wsymResult is served for workspace/symbol
// queries; fail answers every request with a JSON-RPC error instead.
func startFakeLSP(t *testing.T, declared []string, wsymResult []any, fail bool) *lsp.LSPClient {
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
				"method":  "client/registerCapability",
				"params":  map[string]any{"registrations": regs},
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
				ID     *float64        `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(raw, &msg); err != nil {
				return
			}
			if msg.ID == nil || msg.Method == "" {
				continue
			}
			var body []byte
			if fail {
				body, _ = json.Marshal(map[string]any{
					"jsonrpc": "2.0", "id": *msg.ID,
					"error": map[string]any{"code": -32603, "message": "boom"},
				})
			} else {
				result := any(nil)
				if msg.Method == "workspace/symbol" {
					result = wsymResult
				}
				body, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
			}
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
		// The registration arrives asynchronously; wait until the client's
		// readLoop has processed it, otherwise the silent capability gate in
		// GetWorkspaceSymbols swallows the query and returns an empty slice
		// without ever contacting the server.
		want := map[string]bool{}
		for _, m := range declared {
			if m == "workspace/symbol" {
				want["workspaceSymbolProvider"] = true
			}
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			ok := true
			for k := range want {
				if !client.HasCapability(k) {
					ok = false
					break
				}
			}
			if ok {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	return client
}

func wsymJSON(name string) map[string]any {
	return map[string]any{
		"name": name, "kind": 12,
		"location": map[string]any{
			"uri":   "file:///ws/src/a.mq4",
			"range": map[string]any{"start": map[string]any{"line": 8, "character": 7}, "end": map[string]any{"line": 13, "character": 3}},
		},
	}
}

// Table-driven fan-out through real (faked) initialized servers: capability
// wording, result ordering and server tags, and partial-failure provenance.
// The previous multi-server tests used never-initialized clients, so the
// handler skipped every query and the tests could not detect regressions in
// ordering, server tags, or partial-failure hints. (CodeRabbit #59 thread 4)
func TestHandleGetWorkspaceSymbolsMulti_FanOut(t *testing.T) {
	cases := []struct {
		name       string
		clients    func(t *testing.T) []*lsp.LSPClient
		wantInHint []string
		wantSyms   []string
		wantTags   []string
	}{
		{
			name: "single initialized server without the capability",
			clients: func(t *testing.T) []*lsp.LSPClient {
				return []*lsp.LSPClient{startFakeLSP(t, nil, []any{}, false)}
			},
			wantInHint: []string{"The server does not declare the workspaceSymbolProvider capability"},
		},
		{
			name: "multi initialized servers without the capability",
			clients: func(t *testing.T) []*lsp.LSPClient {
				return []*lsp.LSPClient{
					startFakeLSP(t, nil, []any{}, false),
					startFakeLSP(t, nil, []any{}, false),
				}
			},
			wantInHint: []string{"None of the connected servers declare"},
		},
		{
			name: "two answering servers: order and server tags",
			clients: func(t *testing.T) []*lsp.LSPClient {
				return []*lsp.LSPClient{
					startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Alpha")}, false),
					startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Beta")}, false),
				}
			},
			wantSyms: []string{"Alpha", "Beta"},
			wantTags: []string{"server-0", "server-1"},
		},
		{
			name: "one failing server among two: partial-failure note",
			clients: func(t *testing.T) []*lsp.LSPClient {
				return []*lsp.LSPClient{
					startFakeLSP(t, []string{"workspace/symbol"}, nil, true),
					startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Beta")}, false),
				}
			},
			wantSyms:   []string{"Beta"},
			wantTags:   []string{"server-1"},
			wantInHint: []string{"1 of 2 servers failed"},
		},
		{
			name: "nil member does not inflate the failure-note denominator",
			clients: func(t *testing.T) []*lsp.LSPClient {
				return []*lsp.LSPClient{
					nil,
					startFakeLSP(t, []string{"workspace/symbol"}, nil, true),
					startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Beta")}, false),
				}
			},
			wantSyms:   []string{"Beta"},
			wantTags:   []string{"server-2"},
			wantInHint: []string{"1 of 2 servers failed"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := HandleGetWorkspaceSymbolsMulti(context.Background(), tc.clients(t), map[string]any{"query": "x"})
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if r.IsError {
				t.Fatalf("unexpected error result: %v", r.Content)
			}
			all := contentText(r)
			for _, want := range tc.wantInHint {
				if !strings.Contains(all, want) {
					t.Errorf("hint missing %q: %q", want, all)
				}
			}
			if tc.wantSyms == nil {
				return
			}
			var syms []map[string]any
			if err := json.Unmarshal([]byte(r.Content[0].Text), &syms); err != nil {
				t.Fatalf("payload is not a symbol array: %v (%q)", err, r.Content[0].Text)
			}
			if len(syms) != len(tc.wantSyms) {
				t.Fatalf("expected %d symbols, got %d: %v", len(tc.wantSyms), len(syms), syms)
			}
			for i, want := range tc.wantSyms {
				if syms[i]["name"] != want {
					t.Errorf("symbols[%d].name = %v, want %q", i, syms[i]["name"], want)
				}
				if tc.wantTags != nil && syms[i]["server"] != tc.wantTags[i] {
					t.Errorf("symbols[%d].server = %v, want %q", i, syms[i]["server"], tc.wantTags[i])
				}
			}
		})
	}
}

// The GCF graph payload must identify the producing server per symbol: the
// wire format has no dedicated field, so the server rides in provenance as
// "lsp_resolved@<server>" while single-server symbols keep the plain token
// (wire bytes unchanged). (CodeRabbit #59 thread 1)
func TestBuildWorkspaceSymbolsPayload_ServerProvenance(t *testing.T) {
	p := buildWorkspaceSymbolsPayload([]types.SymbolInformation{
		{Name: "Solo", Kind: 12, Location: types.Location{URI: "file:///ws/a.mq4"}},
		{Name: "Alpha", Kind: 12, Server: "mql-a", Location: types.Location{URI: "file:///ws/a.mq4"}},
		{Name: "Beta", Kind: 12, Server: "mql-b", Location: types.Location{URI: "file:///ws/b.mq4"}},
	})
	out, err := gcf.EncodeGraph(p)
	if err != nil {
		t.Fatalf("EncodeGraph: %v", err)
	}
	if !strings.Contains(out, "lsp_resolved@mql-a") || !strings.Contains(out, "lsp_resolved@mql-b") {
		t.Errorf("server provenance missing in GCF payload:\n%s", out)
	}
	if strings.Contains(out, "lsp_resolved@\n") || strings.Count(out, "lsp_resolved\n") != 1 {
		t.Errorf("single-server symbol must keep the plain lsp_resolved token:\n%s", out)
	}
}

func TestCountInitialized(t *testing.T) {
	if got := countInitialized([]*lsp.LSPClient{nil, lsp.NewLSPClient("fake", nil)}); got != 0 {
		t.Fatalf("fresh clients are not initialized, got %d", got)
	}
}

// --- get_server_capabilities multi shape (issue #2) ---

// One server → the response IS the single result (shape unchanged).
func TestServerCapabilitiesResponse_Single(t *testing.T) {
	single := ServerCapabilitiesResult{ServerName: "gopls"}
	resp := serverCapabilitiesResponse([]ServerCapabilitiesResult{single})
	out, _ := json.Marshal(resp)
	if strings.Contains(string(out), "all_servers") {
		t.Fatalf("single-server response must not carry all_servers: %s", out)
	}
	if !strings.Contains(string(out), `"server_name":"gopls"`) {
		t.Fatalf("expected flattened single result, got %s", out)
	}
}

// Several servers → default fields stay at top level (backwards compatible)
// plus an all_servers array with every server.
func TestServerCapabilitiesResponse_Multi(t *testing.T) {
	resp := serverCapabilitiesResponse([]ServerCapabilitiesResult{
		{ServerName: "clangd"},
		{ServerName: "mql-lsp-server"},
	})
	out, _ := json.Marshal(resp)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["server_name"] != "clangd" {
		t.Errorf("top-level server_name must remain the default server, got %v", m["server_name"])
	}
	arr, ok := m["all_servers"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("expected all_servers with 2 entries, got %v", m["all_servers"])
	}
	first := arr[0].(map[string]any)
	if first["server_name"] != "clangd" {
		t.Errorf("all_servers[0] should be clangd, got %v", first["server_name"])
	}
}

// The additive server tag on SymbolInformation must serialize only in
// multi-server mode (omitempty).
func TestSymbolInformation_ServerTagOmitEmpty(t *testing.T) {
	single, _ := json.Marshal(types.SymbolInformation{Name: "A"})
	if strings.Contains(string(single), "server") {
		t.Fatalf("server tag must be omitted when empty: %s", single)
	}
	multi, _ := json.Marshal(types.SymbolInformation{Name: "A", Server: "mql-lsp-server"})
	if !strings.Contains(string(multi), `"server":"mql-lsp-server"`) {
		t.Fatalf("server tag missing: %s", multi)
	}
}

// Coverage must be established per queried client: the first server's open
// documents say nothing about another server's index, so a fully-opened
// default client must not suppress the caveat when another capable server
// has unopened files. (CodeRabbit #59 re-review thread)
func TestHandleGetWorkspaceSymbolsMulti_CoverageCheckedPerClient(t *testing.T) {
	// Server A: no workspace root — coverage complete.
	serverA := startFakeLSP(t, []string{"workspace/symbol"}, []any{}, false)
	// Server B: a root with a file the session never opened — incomplete.
	serverB := startFakeLSP(t, []string{"workspace/symbol"}, []any{}, false)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	serverB.SetRootDirForTest(root)

	r, err := HandleGetWorkspaceSymbolsMulti(context.Background(),
		[]*lsp.LSPClient{serverA, serverB}, map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	all := contentText(r)
	if !strings.Contains(all, "only index opened documents") {
		t.Fatalf("expected index-coverage caveat when one capable server has unopened files, got %q", all)
	}
}

// contentText concatenates every text content item of a result.
func contentText(r types.ToolResult) string {
	var s string
	for _, c := range r.Content {
		s += c.Text + "\n"
	}
	return s
}
