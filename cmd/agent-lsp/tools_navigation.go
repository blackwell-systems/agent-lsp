package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Navigation tool arg types.

type GoToDefinitionArgs struct {
	FilePath        string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID      string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line            int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column          int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
	PositionPattern string `json:"position_pattern,omitempty" jsonschema:"Alternative to line/column: use @@pattern@@ syntax to match text near the target position"`
}

type GoToTypeDefinitionArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
}

type GoToImplementationArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
}

type GoToDeclarationArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
}

type RenameSymbolArgs struct {
	FilePath        string   `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID      string   `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line            int      `json:"line,omitempty" jsonschema:"1-indexed line number in the file"`
	Column          int      `json:"column,omitempty" jsonschema:"1-indexed column (character offset) in the line"`
	NewName         string   `json:"new_name" jsonschema:"New name for the symbol"`
	PositionPattern string   `json:"position_pattern,omitempty" jsonschema:"Alternative to line/column: use @@pattern@@ syntax to match text near the target position"`
	DryRun          bool     `json:"dry_run,omitempty" jsonschema:"If true\\, return the edit without applying. Always true for this read-only tool"`
	ExcludeGlobs    []string `json:"exclude_globs,omitempty" jsonschema:"Glob patterns for files to exclude from rename (e.g. vendor/**\\, *_gen.go)"`
}

type PrepareRenameArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
}

type GoToSymbolArgs struct {
	SymbolPath    string `json:"symbol_path" jsonschema:"Dot-notation symbol path (e.g. MyStruct.MyMethod\\, http.Handler)"`
	WorkspaceRoot string `json:"workspace_root,omitempty" jsonschema:"Workspace root directory for symbol search. Optional; uses current workspace"`
	Language      string `json:"language,omitempty" jsonschema:"Language filter for symbol search. Optional"`
}

type GetDocumentHighlightsArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
}

type CallHierarchyArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
	Direction  string `json:"direction,omitempty" jsonschema:"Direction: incoming\\, outgoing\\, or both (default: both)"`
}

type TypeHierarchyArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go\\, typescript\\, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
	Direction  string `json:"direction,omitempty" jsonschema:"Direction: incoming\\, outgoing\\, or both (default: both)"`
}

func registerNavigationTools(d toolDeps) {
	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "go_to_definition",
		Description: "Jump to the definition of a symbol at a specific location in a file via LSP. Returns the file path and position where the symbol is defined. Useful for navigating to type declarations, function implementations, or variable assignments across the codebase.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Go to Definition",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToDefinitionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToDefinition(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "go_to_type_definition",
		Description: "Jump to the definition of the type of a symbol at a specific location in a file via LSP. Unlike go_to_definition (which goes to where the symbol itself is defined), this navigates to the type declaration. Useful for interface types, type aliases, and class definitions when working with instances or variables.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Go to Type Definition",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToTypeDefinitionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToTypeDefinition(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "go_to_implementation",
		Description: "Find all implementations of an interface or abstract method at a specific location in a file via LSP. Returns the file paths and positions of all concrete implementations. Use this to navigate from an interface declaration or abstract method to the concrete classes that implement it.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Go to Implementation",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToImplementationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToImplementation(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "go_to_declaration",
		Description: "Jump to the declaration of a symbol at a specific location in a file via LSP. Completes the 'go to X' family alongside go_to_definition, go_to_type_definition, and go_to_implementation. Most useful for languages with separate declaration and definition (e.g., C/C++ header files). Returns the file path and position where the symbol is declared.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Go to Declaration",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToDeclarationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToDeclaration(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "go_to_symbol",
		Description: "Navigate to a symbol definition by dot-notation name (e.g. \"LSPClient.GetReferences\", \"http.Handler\") without needing file_path or line/column. Uses workspace symbol search to locate the definition. Useful when you know the symbol name but not its location.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Go to Symbol",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToSymbol(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "rename_symbol",
		Description: "Get a WorkspaceEdit for renaming a symbol across the entire workspace via LSP. Returns the edit object — NOT applied automatically. Use dry_run=true to preview what would change (returns workspace_edit + note). Use position_pattern with @@ marker for reliable position targeting instead of line/column. Inspect the returned WorkspaceEdit then call apply_edit to commit. Optional exclude_globs (array of glob patterns, e.g. [\"vendor/**\", \"**/*_gen.go\"]) skips matching files from the rename — useful for generated code, vendored files, and test fixtures.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Rename Symbol",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RenameSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRenameSymbol(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "prepare_rename",
		Description: "Validate that a rename is possible at the given position before committing to rename_symbol. Returns the range that would be renamed and a placeholder name suggestion, or a message indicating rename is not supported at this position. Use this before rename_symbol to avoid attempting invalid renames. Returns null if the server does not support prepareRename.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Prepare Rename",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PrepareRenameArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandlePrepareRename(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_document_highlights",
		Description: "Find all occurrences of the symbol at a position within the same file via LSP (textDocument/documentHighlight). Returns ranges and kinds: 1=Text, 2=Read, 3=Write. File-scoped and instant — does not trigger a workspace-wide reference search. Use this to find all local usages of a variable, parameter, or field without the overhead of get_references.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Document Highlights",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDocumentHighlightsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDocumentHighlights(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "call_hierarchy",
		Description: "Show call hierarchy for a symbol at a position. Returns callers (incoming), callees (outgoing), or both depending on the direction parameter. Direction defaults to \"both\". Use this to understand code flow -- which functions call this function and which functions it calls.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Call Hierarchy",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CallHierarchyArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCallHierarchy(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "type_hierarchy",
		Description: "Show type hierarchy for a type at a position. Returns supertypes (parent classes/interfaces), subtypes (subclasses/implementations), or both depending on the direction parameter. Direction defaults to \"both\". Use this to understand class and interface inheritance relationships.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Type Hierarchy",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args TypeHierarchyArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleTypeHierarchy(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
