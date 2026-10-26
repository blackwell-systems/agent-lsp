package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGetServerCapabilitiesHandler_FansOutToAllServers is the cmd-level
// regression test for the wiring gap flagged in the maintainer review of
// upstream PR #72: HandleGetServerCapabilitiesMulti was never exercised by a
// test, so reverting the registered handler to the single-server
// HandleGetServerCapabilities(ctx, d.cs.get(), nil) left the whole suite
// green while the fan-out shipped dead. Only calling the registered MCP tool
// observes the wiring. (PR #72 maintainer review, item 1)
func TestGetServerCapabilitiesHandler_FansOutToAllServers(t *testing.T) {
	ctx := context.Background()
	defaultClient := fakeSymServer(t, "DefaultOnly")
	secondClient := fakeSymServer(t, "SecondOnly")

	cs := &clientState{}
	cs.set(defaultClient)

	server := mcp.NewServer(&mcp.Implementation{Name: "agent-lsp-test", Version: "0.0.0"}, nil)
	deps := toolDeps{
		server:                    server,
		cs:                        cs,
		resolver:                  &stubResolver{all: []*lsp.LSPClient{defaultClient, secondClient}},
		clientForFileWithAutoInit: func(string) *lsp.LSPClient { return defaultClient },
	}
	registerWorkspaceTools(deps)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_server_capabilities",
	})
	if err != nil {
		t.Fatalf("call get_server_capabilities: %v", err)
	}
	if res.IsError {
		t.Fatalf("get_server_capabilities returned an error result: %v", res.Content)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	got := text.String()

	// The response is JSON (an agent hint may follow), so decode the first
	// value. The multi-server shape keeps the default server's fields at the
	// top level and adds all_servers with one entry per connected server.
	var resp struct {
		AllServers []map[string]any `json:"all_servers"`
	}
	if err := json.NewDecoder(strings.NewReader(got)).Decode(&resp); err != nil {
		t.Fatalf("get_server_capabilities result does not start with a JSON object: %v\n%s", err, got)
	}
	if len(resp.AllServers) != 2 {
		t.Fatalf("want all_servers with 2 entries, got %d — is the handler still wired to the single-server path?\n%s", len(resp.AllServers), got)
	}
	for i, entry := range resp.AllServers {
		caps, ok := entry["capabilities"].(map[string]any)
		if !ok {
			t.Fatalf("all_servers[%d] has no capabilities map: %v", i, entry)
		}
		if caps["workspaceSymbolProvider"] != true {
			t.Errorf("all_servers[%d] missing the faked workspaceSymbolProvider capability: %v", i, caps)
		}
	}
}
