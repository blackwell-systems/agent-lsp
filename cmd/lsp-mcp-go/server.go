package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/blackwell-systems/lsp-mcp-go/internal/extensions"
	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"
	"github.com/blackwell-systems/lsp-mcp-go/internal/resources"
	"github.com/blackwell-systems/lsp-mcp-go/internal/tools"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// clientState holds the current LSP client reference, guarded by a mutex.
type clientState struct {
	mu     sync.RWMutex
	client *lsp.LSPClient
}

func (s *clientState) get() *lsp.LSPClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *clientState) set(c *lsp.LSPClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = c
}

// toolArgsToMap converts a typed args struct to map[string]interface{} via JSON round-trip.
func toolArgsToMap(v interface{}) map[string]interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		return map[string]interface{}{}
	}
	m := map[string]interface{}{}
	_ = json.Unmarshal(data, &m)
	return m
}

// makeCallToolResult converts a types.ToolResult to *mcp.CallToolResult.
func makeCallToolResult(r interface{}) *mcp.CallToolResult {
	data, err := json.Marshal(r)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "internal error: " + err.Error()}},
			IsError: true,
		}
	}

	var tr struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(data, &tr); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "internal error: " + err.Error()}},
			IsError: true,
		}
	}

	content := make([]mcp.Content, 0, len(tr.Content))
	for _, c := range tr.Content {
		content = append(content, &mcp.TextContent{Text: c.Text})
	}
	return &mcp.CallToolResult{
		Content: content,
		IsError: tr.IsError,
	}
}

// clientForFile returns the LSP client for the given file path using the resolver.
// Falls back to cs.get() (default client) if filePath is empty or no server
// is configured for the file's extension.
func clientForFile(resolver lsp.ClientResolver, cs *clientState, filePath string) *lsp.LSPClient {
	if filePath != "" {
		if c := resolver.ClientForFile(filePath); c != nil {
			return c
		}
	}
	return cs.get()
}

// Run creates and starts the MCP server.
func Run(ctx context.Context, resolver lsp.ClientResolver, registry *extensions.ExtensionRegistry, serverPath string, serverArgs []string) error {
	cs := &clientState{client: resolver.DefaultClient()}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "lsp-mcp-go",
		Version: "0.1.0",
	}, nil)
	// TODO(W3): logging.SetServer needs a logSender implementation wrapping
	// the MCP server. The *mcp.Server type does not expose LogMessage directly.
	// Wire after confirming the correct session notification API.

	// ------- Tool argument types -------

	type StartLspArgs struct {
		RootDir string `json:"root_dir"`
	}
	type RestartLspArgs struct {
		RootDir string `json:"root_dir,omitempty"`
	}
	type OpenDocumentArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Text       string `json:"text,omitempty"`
	}
	type CloseDocumentArgs struct {
		FilePath string `json:"file_path"`
	}
	type GetDiagnosticsArgs struct {
		FilePath string `json:"file_path,omitempty"`
	}
	type GetInfoOnLocationArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type GetCompletionsArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type GetSignatureHelpArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type GetCodeActionsArgs struct {
		FilePath    string `json:"file_path"`
		LanguageID  string `json:"language_id,omitempty"`
		StartLine   int    `json:"start_line"`
		StartColumn int    `json:"start_column"`
		EndLine     int    `json:"end_line"`
		EndColumn   int    `json:"end_column"`
	}
	type GetDocumentSymbolsArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
	}
	type GetWorkspaceSymbolsArgs struct {
		Query string `json:"query,omitempty"`
	}
	type GetReferencesArgs struct {
		FilePath           string `json:"file_path"`
		LanguageID         string `json:"language_id,omitempty"`
		Line               int    `json:"line"`
		Column             int    `json:"column"`
		IncludeDeclaration bool   `json:"include_declaration,omitempty"`
	}
	type GoToDefinitionArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type GoToTypeDefinitionArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type GoToImplementationArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type GoToDeclarationArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type RenameSymbolArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
		NewName    string `json:"new_name"`
	}
	type PrepareRenameArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	type FormatDocumentArgs struct {
		FilePath     string `json:"file_path"`
		LanguageID   string `json:"language_id,omitempty"`
		TabSize      int    `json:"tab_size,omitempty"`
		InsertSpaces *bool  `json:"insert_spaces,omitempty"`
	}
	type FormatRangeArgs struct {
		FilePath    string `json:"file_path"`
		LanguageID  string `json:"language_id,omitempty"`
		StartLine   int    `json:"start_line"`
		StartColumn int    `json:"start_column"`
		EndLine     int    `json:"end_line"`
		EndColumn   int    `json:"end_column"`
		TabSize     int    `json:"tab_size,omitempty"`
		InsertSpaces *bool `json:"insert_spaces,omitempty"`
	}
	type ApplyEditArgs struct {
		Edit interface{} `json:"workspace_edit"`
	}
	type ExecuteCommandArgs struct {
		Command   string        `json:"command"`
		Arguments []interface{} `json:"arguments,omitempty"`
	}
	type DidChangeWatchedFilesArgs struct {
		Changes []interface{} `json:"changes"`
	}
	type SetLogLevelArgs struct {
		Level string `json:"level"`
	}
	type CallHierarchyArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
		Direction  string `json:"direction,omitempty"`
	}

	// ------- Register all 25 tools -------

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_lsp",
		Description: "Initialize or reinitialize the LSP server with a specific project root directory. Call this before using get_references, get_info_on_location, or get_diagnostics when working in a project different from the one the server was started with. The root_dir should be the workspace root (directory containing go.work, go.mod, package.json, etc.).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args StartLspArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleStartLsp(ctx, cs.get, cs.set, serverPath, serverArgs, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "restart_lsp_server",
		Description: "Restart the LSP server process. Use this if the LSP server becomes unresponsive or after making significant changes to the project structure. Optionally provide a new root_dir to restart with a different workspace root.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RestartLspArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRestartLspServer(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "open_document",
		Description: "Open a file in the LSP server for analysis. Use this tool before performing operations like getting diagnostics, hover information, or completions for a file. The file remains open for continued analysis until explicitly closed. The language_id parameter tells the server which language service to use (e.g., 'typescript', 'javascript', 'haskell'). The LSP server starts automatically on MCP launch.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args OpenDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleOpenDocument(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "close_document",
		Description: "Close a file in the LSP server. Use this tool when you're done with a file to free up resources and reduce memory usage. It's good practice to close files that are no longer being actively analyzed, especially in long-running sessions or when working with large codebases.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CloseDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCloseDocument(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_diagnostics",
		Description: "Get diagnostic messages (errors, warnings) for files. Use this tool to identify problems in code files such as syntax errors, type mismatches, or other issues detected by the language server. When used without a file_path, returns diagnostics for all open files.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDiagnosticsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDiagnostics(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_info_on_location",
		Description: "Get information on a specific location in a file via LSP hover. Use this tool to retrieve detailed type information, documentation, and other contextual details about symbols in your code. Particularly useful for understanding variable types, function signatures, and module documentation at a specific location in the code. Use this whenever you need to get a better idea on what a particular function is doing in that context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetInfoOnLocationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetInfoOnLocation(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_completions",
		Description: "Get completion suggestions at a specific location in a file. Use this tool to retrieve code completion options based on the current context, including variable names, function calls, object properties, and more. Helpful for code assistance and auto-completion at a particular location. Use this when determining which functions you have available in a given package, for example when changing libraries.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetCompletionsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetCompletions(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_signature_help",
		Description: "Get function signature help at a specific location in a file via LSP. Returns available overloads and highlights the active parameter. Use this when the cursor is inside a function call's argument list to understand what parameters the function expects.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSignatureHelpArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSignatureHelp(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_code_actions",
		Description: "Get code actions for a specific range in a file. Use this tool to obtain available refactorings, quick fixes, and other code modifications that can be applied to a selected code range. Examples include adding imports, fixing errors, or implementing interfaces.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetCodeActionsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetCodeActions(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document_symbols",
		Description: "Get all symbols defined in a document via LSP (functions, classes, variables, methods, etc.). Returns a hierarchical DocumentSymbol tree or flat SymbolInformation list depending on server support. Use this to get a structural overview of a file.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDocumentSymbolsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDocumentSymbols(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_workspace_symbols",
		Description: "Search for symbols across the entire workspace via LSP. Use an empty query string to list all indexed symbols, or provide a query to filter by name. Returns matching symbol names, kinds, and locations.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetWorkspaceSymbolsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetWorkspaceSymbols(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_references",
		Description: "Find all references to a symbol at a specific location in a file via LSP. Returns every location in the codebase where the symbol is used. Use this to determine if a symbol is dead (zero references), to understand call sites before refactoring, or to trace data flow. Results include file path and line/column for each reference.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetReferencesArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetReferences(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_definition",
		Description: "Jump to the definition of a symbol at a specific location in a file via LSP. Returns the file path and position where the symbol is defined. Useful for navigating to type declarations, function implementations, or variable assignments across the codebase.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToDefinitionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToDefinition(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_type_definition",
		Description: "Jump to the definition of the type of a symbol at a specific location in a file via LSP. Unlike go_to_definition (which goes to where the symbol itself is defined), this navigates to the type declaration. Useful for interface types, type aliases, and class definitions when working with instances or variables.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToTypeDefinitionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToTypeDefinition(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_implementation",
		Description: "Find all implementations of an interface or abstract method at a specific location in a file via LSP. Returns the file paths and positions of all concrete implementations. Use this to navigate from an interface declaration or abstract method to the concrete classes that implement it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToImplementationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToImplementation(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_declaration",
		Description: "Jump to the declaration of a symbol at a specific location in a file via LSP. Completes the 'go to X' family alongside go_to_definition, go_to_type_definition, and go_to_implementation. Most useful for languages with separate declaration and definition (e.g., C/C++ header files). Returns the file path and position where the symbol is declared.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToDeclarationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToDeclaration(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_symbol",
		Description: "Get a WorkspaceEdit for renaming a symbol across the entire workspace via LSP. Returns the edit object describing all files and positions that need to change — it is NOT applied automatically. Inspect the returned WorkspaceEdit to understand the full scope of a rename before applying it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RenameSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRenameSymbol(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "prepare_rename",
		Description: "Validate that a rename is possible at the given position before committing to rename_symbol. Returns the range that would be renamed and a placeholder name suggestion, or a message indicating rename is not supported at this position. Use this before rename_symbol to avoid attempting invalid renames. Returns null if the server does not support prepareRename.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PrepareRenameArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandlePrepareRename(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "format_document",
		Description: "Get formatting edits for an entire document via LSP. Returns TextEdit[] describing the changes needed to format the file according to the language server's style rules. The edits are returned for inspection — they are NOT applied automatically. Use this to see what formatting changes a formatter would make.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatDocument(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "format_range",
		Description: "Get formatting edits for a specific range within a document via LSP (textDocument/rangeFormatting). Returns TextEdit[] for the selected lines/characters only. Use this when you want to format a function, block, or selection rather than the entire file. The edits are NOT applied automatically.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatRangeArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatRange(ctx, clientForFile(resolver, cs, args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_edit",
		Description: "Apply a WorkspaceEdit to the workspace by writing file changes to disk. Pass the WorkspaceEdit object returned by rename_symbol or format_document. Edits are applied in reverse order to preserve offsets, then the LSP server is notified of each changed file via didChange. Use this after inspecting the edit returned by rename_symbol or format_document to commit the changes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ApplyEditArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleApplyEdit(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "execute_command",
		Description: "Execute a workspace command via LSP. Commands are server-defined identifiers returned by code actions (in the command field of a CodeAction). Use this after get_code_actions to trigger a server-side operation such as applying a refactoring, generating code, or running a server-specific action. Returns the server-defined result or null.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ExecuteCommandArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleExecuteCommand(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "did_change_watched_files",
		Description: "Notify the language server that files have changed on disk outside the editor (workspace/didChangeWatchedFiles). Use this after writing files directly to disk so the server refreshes its caches. Change types: 1=created, 2=changed, 3=deleted. File URIs must use the file:/// scheme.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DidChangeWatchedFilesArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDidChangeWatchedFiles(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_log_level",
		Description: "Set the server logging level. Use this tool to control the verbosity of logs generated by the LSP MCP server. Available levels from least to most verbose: emergency, alert, critical, error, warning, notice, info, debug. Increasing verbosity can help troubleshoot issues but may generate large amounts of output.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SetLogLevelArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSetLogLevel(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "call_hierarchy",
		Description: "Show call hierarchy for a symbol at a position. Returns callers (incoming), callees (outgoing), or both depending on the direction parameter. Direction defaults to \"both\". Use this to understand code flow -- which functions call this function and which functions it calls.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CallHierarchyArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCallHierarchy(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	// ------- Register resources -------

	server.AddResource(&mcp.Resource{
		URI:         "lsp-diagnostics://",
		Name:        "All Diagnostics",
		Description: "LSP diagnostics for all open documents",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := cs.get()
		if client == nil {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      req.Params.URI,
					MIMEType: "application/json",
					Text:     "{}",
				}},
			}, nil
		}
		result, err := resources.HandleDiagnosticsResource(ctx, client, req.Params.URI)
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

	server.AddResource(&mcp.Resource{
		URI:         "lsp-hover://",
		Name:        "LSP Hover",
		Description: "LSP hover information. URI format: lsp-hover:///path/to/file?line=N&column=N&language_id=X",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := cs.get()
		if client == nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		uri := req.Params.URI
		if !strings.HasPrefix(uri, "lsp-hover://") {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		result, err := resources.HandleHoverResource(ctx, client, uri)
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

	server.AddResource(&mcp.Resource{
		URI:         "lsp-completions://",
		Name:        "LSP Completions",
		Description: "LSP completions. URI format: lsp-completions:///path/to/file?line=N&column=N&language_id=X",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := cs.get()
		if client == nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		uri := req.Params.URI
		if !strings.HasPrefix(uri, "lsp-completions://") {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		result, err := resources.HandleCompletionsResource(ctx, client, uri)
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

	// Subscribe to diagnostic updates for logging purposes (all managed clients).
	for _, c := range resolver.AllClients() {
		if c != nil {
			c.SubscribeToDiagnostics(func(uri string, _ []types.LSPDiagnostic) {
				logging.Log(logging.LevelDebug, "diagnostics updated for: "+uri)
			})
		}
	}

	// Mark server as initialized and start on stdio transport.
	logging.MarkServerInitialized()
	logging.Log(logging.LevelInfo, "lsp-mcp-go server starting")

	transport := &mcp.StdioTransport{}
	return server.Run(ctx, transport)
}
