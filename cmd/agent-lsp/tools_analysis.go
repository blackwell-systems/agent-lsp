package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Analysis tool arg types.

type GetInfoOnLocationArgs struct {
	FilePath        string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID      string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	Line            int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column          int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
	PositionPattern string `json:"position_pattern,omitempty" jsonschema:"Alternative to line/column: use @@pattern@@ syntax to match text near the target position"`
}

type GetCompletionsArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
}

type GetSignatureHelpArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	Line       int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column     int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
}

type GetCodeActionsArgs struct {
	FilePath    string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID  string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	StartLine   int    `json:"start_line" jsonschema:"1-indexed start line of the range"`
	StartColumn int    `json:"start_column" jsonschema:"1-indexed start column of the range"`
	EndLine     int    `json:"end_line" jsonschema:"1-indexed end line of the range"`
	EndColumn   int    `json:"end_column" jsonschema:"1-indexed end column of the range"`
}

type GetDocumentSymbolsArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	Format     string `json:"format,omitempty" jsonschema:"Output format: 'outline' for compact markdown, default returns JSON"`
}

type GetWorkspaceSymbolsArgs struct {
	Query       string `json:"query,omitempty" jsonschema:"Symbol name or pattern to search for across the workspace"`
	DetailLevel string `json:"detail_level,omitempty" jsonschema:"Enrichment level: 'basic' for names/locations only, 'hover' to include type signatures and docs"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum number of symbols to enrich with hover info (default 3)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"Number of symbols to skip before enriching (default 0), for pagination"`
}

type GetReferencesArgs struct {
	FilePath           string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID         string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	Line               int    `json:"line" jsonschema:"1-indexed line number in the file"`
	Column             int    `json:"column" jsonschema:"1-indexed column (character offset) in the line"`
	IncludeDeclaration bool   `json:"include_declaration,omitempty" jsonschema:"Whether to include the declaration site in the results"`
	PositionPattern    string `json:"position_pattern,omitempty" jsonschema:"Alternative to line/column: use @@pattern@@ syntax to match text near the target position"`
}

type GetInlayHintsArgs struct {
	FilePath    string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID  string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	StartLine   int    `json:"start_line" jsonschema:"1-indexed start line of the range"`
	StartColumn int    `json:"start_column" jsonschema:"1-indexed start column of the range"`
	EndLine     int    `json:"end_line" jsonschema:"1-indexed end line of the range"`
	EndColumn   int    `json:"end_column" jsonschema:"1-indexed end column of the range"`
}

type GetSemanticTokensArgs struct {
	FilePath    string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID  string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	StartLine   int    `json:"start_line" jsonschema:"1-indexed start line of the range"`
	StartColumn int    `json:"start_column" jsonschema:"1-indexed start column of the range"`
	EndLine     int    `json:"end_line" jsonschema:"1-indexed end line of the range"`
	EndColumn   int    `json:"end_column" jsonschema:"1-indexed end column of the range"`
}

type GetSymbolSourceArgs struct {
	FilePath        string `json:"file_path" jsonschema:"Absolute path to the source file"`
	LanguageID      string `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
	Line            int    `json:"line,omitempty" jsonschema:"1-indexed line number of the cursor position"`
	Column          int    `json:"column,omitempty" jsonschema:"1-indexed column (character offset) in the line (defaults to 1)"`
	PositionPattern string `json:"position_pattern,omitempty" jsonschema:"Alternative to line/column: use @@pattern@@ syntax to match text near the target position"`
}

type GetSymbolDocumentationArgs struct {
	Symbol     string `json:"symbol" jsonschema:"Fully qualified symbol name to look up (e.g. 'fmt.Println', 'os.File.Read')"`
	LanguageID string `json:"language_id" jsonschema:"Language identifier (e.g. go, python, rust) to select the correct toolchain doc command"`
	FilePath   string `json:"file_path,omitempty" jsonschema:"Optional file path to establish workspace context for the documentation lookup"`
	Format     string `json:"format,omitempty" jsonschema:"Output format for the documentation (e.g. 'markdown', 'plain')"`
}

type GetChangeImpactArgs struct {
	ChangedFiles      []string `json:"changed_files" jsonschema:"List of absolute file paths to analyze for exported symbol impact"`
	IncludeTransitive bool     `json:"include_transitive,omitempty" jsonschema:"If true, include second-order callers (callers of callers) in the results"`
}

type GetCrossRepoReferencesArgs struct {
	SymbolFile    string   `json:"symbol_file" jsonschema:"Absolute path to the file containing the symbol to search for"`
	Line          int      `json:"line" jsonschema:"1-indexed line number of the symbol in the file"`
	Column        int      `json:"column" jsonschema:"1-indexed column (character offset) of the symbol in the line"`
	ConsumerRoots []string `json:"consumer_roots" jsonschema:"List of absolute paths to consumer repository roots to search for references"`
	LanguageID    string   `json:"language_id,omitempty" jsonschema:"Language identifier (e.g. go, typescript, python). Optional; auto-detected from file extension"`
}

func registerAnalysisTools(d toolDeps) {
	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_info_on_location",
		Description: "Get information on a specific location in a file via LSP hover. Use this tool to retrieve detailed type information, documentation, and other contextual details about symbols in your code. Particularly useful for understanding variable types, function signatures, and module documentation at a specific location in the code. Use this whenever you need to get a better idea on what a particular function is doing in that context.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Hover Info",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetInfoOnLocationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetInfoOnLocation(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_completions",
		Description: "Get completion suggestions at a specific location in a file. Use this tool to retrieve code completion options based on the current context, including variable names, function calls, object properties, and more. Helpful for code assistance and auto-completion at a particular location. Use this when determining which functions you have available in a given package, for example when changing libraries.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Completions",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetCompletionsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetCompletions(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_signature_help",
		Description: "Get function signature help at a specific location in a file via LSP. Returns available overloads and highlights the active parameter. Use this when the cursor is inside a function call's argument list to understand what parameters the function expects.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Signature Help",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSignatureHelpArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSignatureHelp(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_code_actions",
		Description: "Get code actions for a specific range in a file. Use this tool to obtain available refactorings, quick fixes, and other code modifications that can be applied to a selected code range. Examples include adding imports, fixing errors, or implementing interfaces.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Code Actions",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetCodeActionsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetCodeActions(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_document_symbols",
		Description: "Get all symbols defined in a document via LSP (functions, classes, variables, methods, etc.). Returns a hierarchical DocumentSymbol tree or flat SymbolInformation list depending on server support. Use this to get a structural overview of a file. Pass format: \"outline\" for compact markdown output (name [Kind] :line) optimized for LLM consumption — ~5x fewer tokens than JSON for the same structural information.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Document Symbols",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDocumentSymbolsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDocumentSymbols(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_workspace_symbols",
		Description: "Search for symbols across the entire workspace via LSP. Returns all matching symbols with name, kind, and location. detail_level controls enrichment: omit or use \"basic\" for names/locations only; use \"hover\" to also return hover info (type signature + docs) for a paginated window of results. limit (default 3) and offset (default 0) control which symbols get enriched — use offset to step through results without re-running the search.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Workspace Symbols",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetWorkspaceSymbolsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetWorkspaceSymbols(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_references",
		Description: "Find all references to a symbol at a specific location in a file via LSP. Returns every location in the codebase where the symbol is used. Use this to determine if a symbol is dead (zero references), to understand call sites before refactoring, or to trace data flow. Results include file path and line/column for each reference.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Find References",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetReferencesArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetReferences(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_inlay_hints",
		Description: "Get inlay hints for a range within a document via LSP (textDocument/inlayHint). Inlay hints are inline annotations that IDEs display in source code — typically inferred type names (e.g. `: string`) and parameter name labels (e.g. `count:`). Useful in languages with type inference (TypeScript, Rust, Go) to see what the compiler knows without reading every type annotation. Returns an array of InlayHint objects, each with a position, label, and optional kind (1=Type, 2=Parameter). Returns an empty array if the language server does not support inlay hints.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Inlay Hints",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetInlayHintsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetInlayHints(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_semantic_tokens",
		Description: "Get semantic tokens for a range in a file. Returns each token's type (function, variable, keyword, parameter, type, etc.) and modifiers (readonly, static, deprecated, etc.) with 1-based line/character positions. Use this to understand the syntactic role of code elements — distinct from hover which gives documentation. Only available when the language server supports textDocument/semanticTokens.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Semantic Tokens",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSemanticTokensArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSemanticTokens(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_symbol_source",
		Description: "Return the source code of the innermost symbol (function, method, class, struct, etc.) whose range contains the given cursor position. Calls textDocument/documentSymbol, walks the symbol tree to find the smallest enclosing symbol, then slices the file at that symbol's range. Returns symbol_name, symbol_kind, start_line (1-based), end_line (1-based), and source text. Use line+character or position_pattern (@@-syntax) to specify the cursor. character defaults to 1.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Symbol Source",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSymbolSourceArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSymbolSource(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_symbol_documentation",
		Description: "Fetch authoritative documentation for a named symbol from local toolchain sources (go doc, pydoc, cargo doc) without requiring an LSP hover response. Works on transitive dependencies not indexed by the language server. Returns the full doc text, extracted signature, and source tag. Falls back gracefully when the toolchain command fails or the language is unsupported.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Symbol Documentation",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSymbolDocumentationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSymbolDocumentation(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_change_impact",
		Description: "Enumerate all exported symbols in the specified files, resolve their references across the workspace, and partition callers into test vs non-test. Returns affected_symbols (name, file, line), test_callers (with enclosing test function names), and non_test_callers. Use before editing a file to understand blast radius. Set include_transitive=true to surface second-order callers (callers of callers).",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Change Impact",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetChangeImpactArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetChangeImpact(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_cross_repo_references",
		Description: "Find all references to a library symbol across one or more consumer repositories. Adds each consumer_root as a workspace folder, waits for indexing, then calls get_references and partitions results by repo. Returns library_references (within the primary repo), consumer_references (map of root → locations), and warnings (roots that could not be indexed). Use before changing a shared library API to find all downstream callers.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Cross-Repo References",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetCrossRepoReferencesArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetCrossRepoReferences(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
