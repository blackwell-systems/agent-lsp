// tools_symbol_edit.go defines MCP tool registrations for symbol-level editing:
// replace_symbol_body, insert_after_symbol, insert_before_symbol, and
// safe_delete_symbol.
//
// These tools resolve symbols by dot-notation name path (e.g. "MyStruct.Method")
// rather than requiring line/column positions, then delegate to handler functions
// in internal/tools for the actual editing logic.
package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Symbol editing tool arg types.

type ReplaceSymbolBodyArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier. Optional; auto-detected"`
	SymbolPath string `json:"symbol_path" jsonschema:"Dot-notation symbol path (e.g. MyStruct.Method, Function, Method[0] for overload disambiguation)"`
	NewBody    string `json:"new_body" jsonschema:"New body text to replace the symbol's body (signature/declaration preserved)"`
}

type InsertAfterSymbolArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier. Optional; auto-detected"`
	SymbolPath string `json:"symbol_path" jsonschema:"Dot-notation symbol path to insert after"`
	Code       string `json:"code" jsonschema:"Code to insert after the symbol definition"`
}

type InsertBeforeSymbolArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier. Optional; auto-detected"`
	SymbolPath string `json:"symbol_path" jsonschema:"Dot-notation symbol path to insert before"`
	Code       string `json:"code" jsonschema:"Code to insert before the symbol definition"`
}

type SafeDeleteSymbolArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier. Optional; auto-detected"`
	SymbolPath string `json:"symbol_path" jsonschema:"Dot-notation symbol path of the symbol to delete"`
}

func registerSymbolEditTools(d toolDeps) {
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "replace_symbol_body",
		Description: "Replace the body of a named symbol (function, method, class) by dot-notation path, preserving the declaration/signature line. Resolves the symbol via document symbols without requiring line/column positions. Use symbol_path like 'MyStruct.Method' or 'Function'. For overload disambiguation, append [N] index (e.g. 'Handle[1]').",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Replace Symbol Body",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ReplaceSymbolBodyArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleReplaceSymbolBody(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "insert_after_symbol",
		Description: "Insert code immediately after a named symbol definition. Resolves the symbol's end position via document symbols. Use for adding new methods after existing ones, appending related functions, etc.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Insert After Symbol",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args InsertAfterSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleInsertAfterSymbol(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "insert_before_symbol",
		Description: "Insert code immediately before a named symbol definition. Resolves the symbol's start position via document symbols. Use for adding imports, comments, decorators, or type definitions before their first consumer.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Insert Before Symbol",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args InsertBeforeSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleInsertBeforeSymbol(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "safe_delete_symbol",
		Description: "Delete a named symbol only if it has zero references across the workspace (verified via LSP references before deletion). Returns an error with the caller count if the symbol is still in use. Prevents accidental removal of active code.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Safe Delete Symbol",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SafeDeleteSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSafeDeleteSymbol(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
