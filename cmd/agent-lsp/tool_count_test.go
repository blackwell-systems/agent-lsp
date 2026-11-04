package main

import (
	"context"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registeredTools lists the tool names registerAllTools registers, as an MCP
// client sees them.
func registeredTools(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "agent-lsp-test", Version: "0.0.0"}, nil)
	registerAllTools(toolDeps{
		server:                    server,
		cs:                        &clientState{},
		resolver:                  &stubResolver{},
		clientForFileWithAutoInit: func(string) *lsp.LSPClient { return nil },
	})
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// The tool count is written into the server instructions and the rules
// `agent-lsp init` generates, and quoted across the docs. It drifted (docs
// said 67 while 66 were registered); this pins the constant to the registry.
func TestRegisteredToolCount(t *testing.T) {
	if got := len(registeredTools(t)); got != registeredToolCount {
		t.Fatalf("registerAllTools registers %d tools, but registeredToolCount = %d; update the constant (and the docs that quote it)", got, registeredToolCount)
	}
}
