// tools_aliases.go registers intent-based alias tool names that map to
// existing handlers with preset arguments or composite workflows.
//
// Aliases provide shorter, intent-oriented names for common operations:
//   - callers -> find_callers with direction forced to "incoming"
//   - explore -> composite symbol exploration (type + callers + refs + source)
//   - safe_edit -> preview + apply when safe (net_delta == 0)
package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SafeApplyEditArgs is defined in tools_safe_edit.go (Agent C).

func registerAliasTools(d toolDeps) {
	// callers: wraps find_callers with direction forced to "incoming".
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "callers",
		Description: "Find all incoming callers of a function or method. Shortcut for find_callers with direction='incoming'. Use before deleting or refactoring a function to see who depends on it.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Callers",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CallHierarchyArgs) (*mcp.CallToolResult, any, error) {
		args.Direction = "incoming"
		r, err := tools.HandleCallHierarchy(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	// explore: composite symbol exploration (type info + callers + refs + source).
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "explore",
		Description: "Deep exploration of a symbol: combines type info, source, callers, references, and test callers in one call. Use when navigating unfamiliar code and you need the full picture of what a symbol is and how it is used.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Explore",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetInfoOnLocationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleExploreSymbol(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	// safe_edit: preview + apply when net_delta == 0.
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "safe_edit",
		Description: "Preview an edit and apply it only if safe (net diagnostic delta == 0). Combines preview_edit + apply_edit into one step. Returns applied=true on success or applied=false with preview diagnostics when the edit would introduce errors.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Safe Edit",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SafeApplyEditArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSafeApplyEdit(ctx, d.clientForFileWithAutoInit(args.FilePath), d.sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
