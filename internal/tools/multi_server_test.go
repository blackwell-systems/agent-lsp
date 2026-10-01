package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- find_symbol multi-server fan-out (issue #2) ---

// Single client, capability undeclared → the exact #42 wording must be
// preserved byte-for-byte (single-server behavior unchanged).
func TestHandleGetWorkspaceSymbolsMulti_SingleClientUndeclared(t *testing.T) {
	client := lsp.NewLSPClient("fake", nil)
	r, err := HandleGetWorkspaceSymbolsMulti(context.Background(), []*lsp.LSPClient{client}, map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	all := contentText(r)
	if !strings.Contains(all, "The server does not declare the workspaceSymbolProvider capability") {
		t.Fatalf("expected single-server wording, got %q", all)
	}
}

// Multiple clients, none declaring the capability → the multi wording, not
// the single-server one.
func TestHandleGetWorkspaceSymbolsMulti_MultiClientUndeclared(t *testing.T) {
	r, err := HandleGetWorkspaceSymbolsMulti(context.Background(),
		[]*lsp.LSPClient{lsp.NewLSPClient("fake", nil), lsp.NewLSPClient("fake2", nil)},
		map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	all := contentText(r)
	if !strings.Contains(all, "None of the connected servers declare") {
		t.Fatalf("expected multi-server wording, got %q", all)
	}
}

// No clients at all → the standard not-initialized error.
func TestHandleGetWorkspaceSymbolsMulti_NoClients(t *testing.T) {
	r, err := HandleGetWorkspaceSymbolsMulti(context.Background(), nil, map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError || !strings.Contains(contentText(r), "start_lsp") {
		t.Fatalf("expected not-initialized error, got %v", r.Content)
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

// contentText concatenates every text content item of a result.
func contentText(r types.ToolResult) string {
	var s string
	for _, c := range r.Content {
		s += c.Text + "\n"
	}
	return s
}
