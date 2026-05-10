package main

import (
	"context"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/resources"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerInspectResources registers the inspect://last MCP resource.
// Must be called from Run() in server.go after the server is created.
func registerInspectResources(server *mcp.Server, cs *clientState) {
	server.AddResource(&mcp.Resource{
		URI:         "inspect://last",
		Name:        "Last Inspection",
		Description: "Results from the most recent /lsp-inspect run (.agent-lsp/last-inspection.json)",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		if !strings.HasPrefix(uri, "inspect://") {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		client := cs.get()
		var workspaceRoot string
		if client != nil {
			workspaceRoot = client.RootDir()
		}
		result, err := resources.HandleInspectResource(ctx, workspaceRoot, uri)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      result.URI,
				MIMEType: result.MIMEType,
				Text:     result.Text,
			}},
		}, nil
	})
}
