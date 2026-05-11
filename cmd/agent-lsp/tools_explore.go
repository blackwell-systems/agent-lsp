// tools_explore.go defines MCP tool registrations for composite exploration tools.
// These tools combine multiple LSP queries into a single call for deep-dive
// symbol analysis.
package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ExploreSymbolArgs mirrors GetInfoOnLocationArgs for the explore_symbol tool.
type ExploreSymbolArgs struct {
	FilePath        string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID      string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line            *int   `json:"line,omitempty" jsonschema:"1-indexed line number in the file. Optional when position_pattern is provided."`
	Column          *int   `json:"column,omitempty" jsonschema:"1-indexed column (character offset) in the line. Optional when position_pattern is provided."`
	PositionPattern string `json:"position_pattern,omitempty" jsonschema:"Alternative to line/column: use @@pattern@@ syntax to match text near the target position"`
}

func registerExploreTools(d toolDeps) {
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "explore_symbol",
		Description: "Deep-dive into a symbol: type info, source code, callers (top 10), references (count + top 5 files), and test caller count in one call. Use when you need full context about a symbol before editing. Accepts file_path + line/column or position_pattern.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Explore Symbol",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ExploreSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleExploreSymbol(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
