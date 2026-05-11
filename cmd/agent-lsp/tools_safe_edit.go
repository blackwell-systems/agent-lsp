// tools_safe_edit.go defines MCP tool registration for safe_apply_edit:
// a combined preview_edit + apply_edit that only writes to disk when the
// speculative simulation shows net_delta == 0.
package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SafeApplyEditArgs are the arguments for safe_apply_edit.
type SafeApplyEditArgs struct {
	FilePath string `json:"file_path" jsonschema:"Absolute path to the file to edit"`
	OldText  string `json:"old_text" jsonschema:"Exact text to find and replace"`
	NewText  string `json:"new_text" jsonschema:"Replacement text"`
}

func registerSafeEditTools(d toolDeps) {
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "safe_apply_edit",
		Description: "Preview an edit and apply it only if safe (net_delta == 0). Combines preview_edit + apply_edit into one call. If the edit would introduce errors (net_delta > 0), returns the preview result with applied=false so you can decide.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Safe Apply Edit",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SafeApplyEditArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSafeApplyEdit(ctx, d.clientForFileWithAutoInit(args.FilePath), d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
