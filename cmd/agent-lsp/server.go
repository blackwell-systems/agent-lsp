package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/internal/extensions"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/resources"
	"github.com/blackwell-systems/agent-lsp/internal/session"
	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/blackwell-systems/agent-lsp/internal/types"
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

// csResolver wraps clientState + the original resolver to implement lsp.ClientResolver.
// DefaultClient falls back to cs.get() so start_lsp updates are visible.
// ClientForFile delegates to the real resolver for correct multi-server routing
// (e.g. gopls for .go files, clangd for .c files).
type csResolver struct {
	cs       *clientState
	delegate lsp.ClientResolver
}

func (r *csResolver) DefaultClient() *lsp.LSPClient {
	if c := r.cs.get(); c != nil {
		return c
	}
	return r.delegate.DefaultClient()
}
func (r *csResolver) ClientForFile(path string) *lsp.LSPClient {
	// Delegate to the real resolver for file-based routing (gopls for .go, etc).
	// After start_lsp calls StartAll, all delegate clients are initialized.
	return r.delegate.ClientForFile(path)
}
func (r *csResolver) AllClients() []*lsp.LSPClient  { return r.delegate.AllClients() }
func (r *csResolver) Shutdown(ctx context.Context) error { return r.delegate.Shutdown(ctx) }

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

// clientForFile returns the LSP client for the given file path.
// Prefers cs.get() when initialized — cs is updated by start_lsp and always
// reflects the most recently initialized client. Falls back to resolver.ClientForFile
// for extension-based routing (multi-server mode after start_lsp has run).
func clientForFile(resolver lsp.ClientResolver, cs *clientState, filePath string) *lsp.LSPClient {
	// cs.get() is the source of truth: start_lsp sets it via cs.set(c) after
	// successful Initialize. In single-server mode this is the only initialized
	// client. In multi-server mode it is set to DefaultClient() after StartAll.
	if c := cs.get(); c != nil && c.IsInitialized() {
		return c
	}
	// cs has no initialized client yet — try extension-based routing for
	// multi-server mode where individual clients may have been started.
	if filePath != "" {
		if c := resolver.ClientForFile(filePath); c != nil && c.IsInitialized() {
			return c
		}
	}
	// Return whatever cs has (may be nil or uninitialized — caller's
	// CheckInitialized will surface the error to the user).
	return cs.get()
}

// autoInitClient attempts to infer a workspace root from filePath and
// initialize the resolver. Safe to call concurrently via initMu.
// Returns nil if filePath is empty, inference returns no root,
// or if the file is already within the current workspace root.
func autoInitClient(
	ctx context.Context,
	resolver lsp.ClientResolver,
	cs *clientState,
	initMu *sync.Mutex,
	filePath string,
) *lsp.LSPClient {
	if filePath == "" {
		return nil
	}

	// Check if client is already initialized and file is within its root.
	if existing := cs.get(); existing != nil {
		rootDir := existing.RootDir()
		if rootDir != "" && strings.HasPrefix(filePath, rootDir+"/") {
			return existing
		}
	}

	root, _, err := config.InferWorkspaceRoot(filePath)
	if err != nil || root == "" {
		return nil
	}

	initMu.Lock()
	defer initMu.Unlock()

	// Re-check after acquiring lock (double-checked locking pattern).
	if existing := cs.get(); existing != nil {
		existingRoot := existing.RootDir()
		if existingRoot != "" && strings.HasPrefix(filePath, existingRoot+"/") {
			return existing
		}
	}

	logging.Log(logging.LevelInfo, fmt.Sprintf(
		"auto-init: inferred workspace root %q for file %q", root, filePath))

	if sm, ok := resolver.(*lsp.ServerManager); ok {
		if err := sm.StartAll(ctx, root); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("auto-init StartAll failed: %v", err))
			return nil
		}
		if c := resolver.DefaultClient(); c != nil {
			cs.set(c)
			return c
		}
		return nil
	}

	// Single-server fallback: not supported via autoInitClient because
	// serverPath/serverArgs are not in scope here. Return nil to fall
	// through to the existing "LSP not initialized" error.
	return nil
}

// Run creates and starts the MCP server.
func Run(ctx context.Context, resolver lsp.ClientResolver, registry *extensions.ExtensionRegistry, serverPath string, serverArgs []string) error {
	cs := &clientState{client: resolver.DefaultClient()}
	var initMu sync.Mutex
	// clientForFileWithAutoInit extends clientForFile with auto-init behavior.
	// If the resolver returns no client for filePath, attempt auto-initialization.
	clientForFileWithAutoInit := func(filePath string) *lsp.LSPClient {
		if c := clientForFile(resolver, cs, filePath); c != nil {
			return c
		}
		return autoInitClient(ctx, resolver, cs, &initMu, filePath)
	}
	sessionMgr := session.NewSessionManager(&csResolver{cs: cs, delegate: resolver})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "agent-lsp",
		Version: "0.1.0",
	}, nil)
	// TODO(W3): logging.SetServer needs a logSender implementation wrapping
	// the MCP server. The *mcp.Server type does not expose LogMessage directly.
	// Wire after confirming the correct session notification API.

	// ------- Tool argument types -------

	type StartLspArgs struct {
		RootDir    string `json:"root_dir"`
		LanguageID string `json:"language_id,omitempty"`
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
		FilePath        string `json:"file_path"`
		LanguageID      string `json:"language_id,omitempty"`
		Line            int    `json:"line"`
		Column          int    `json:"column"`
		PositionPattern string `json:"position_pattern,omitempty"`
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
		Format     string `json:"format,omitempty"` // "outline" for compact markdown; default returns JSON
	}
	type GetWorkspaceSymbolsArgs struct {
		Query       string `json:"query,omitempty"`
		DetailLevel string `json:"detail_level,omitempty"`
		Limit       int    `json:"limit,omitempty"`
		Offset      int    `json:"offset,omitempty"`
	}
	type GetReferencesArgs struct {
		FilePath           string `json:"file_path"`
		LanguageID         string `json:"language_id,omitempty"`
		Line               int    `json:"line"`
		Column             int    `json:"column"`
		IncludeDeclaration bool   `json:"include_declaration,omitempty"`
		PositionPattern    string `json:"position_pattern,omitempty"`
	}
	type GoToDefinitionArgs struct {
		FilePath        string `json:"file_path"`
		LanguageID      string `json:"language_id,omitempty"`
		Line            int    `json:"line"`
		Column          int    `json:"column"`
		PositionPattern string `json:"position_pattern,omitempty"`
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
		FilePath        string `json:"file_path"`
		LanguageID      string `json:"language_id,omitempty"`
		Line            int    `json:"line,omitempty"`
		Column          int    `json:"column,omitempty"`
		NewName         string `json:"new_name"`
		PositionPattern string `json:"position_pattern,omitempty"`
		DryRun          bool   `json:"dry_run,omitempty"`
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
		Edit     map[string]interface{} `json:"workspace_edit,omitempty"`
		FilePath string                 `json:"file_path,omitempty"`
		OldText  string                 `json:"old_text,omitempty"`
		NewText  string                 `json:"new_text,omitempty"`
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
	type TypeHierarchyArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
		Direction  string `json:"direction,omitempty"`
	}
	type GetSemanticTokensArgs struct {
		FilePath    string `json:"file_path"`
		LanguageID  string `json:"language_id,omitempty"`
		StartLine   int    `json:"start_line"`
		StartColumn int    `json:"start_column"`
		EndLine     int    `json:"end_line"`
		EndColumn   int    `json:"end_column"`
	}
	type CreateSimulationSessionArgs struct {
		WorkspaceRoot string `json:"workspace_root"`
		Language      string `json:"language"`
	}
	type SimulateEditArgs struct {
		SessionID   string `json:"session_id"`
		FilePath    string `json:"file_path"`
		StartLine   int    `json:"start_line"`
		StartColumn int    `json:"start_column"`
		EndLine     int    `json:"end_line"`
		EndColumn   int    `json:"end_column"`
		NewText     string `json:"new_text"`
	}
	type EvaluateSessionArgs struct {
		SessionID string `json:"session_id"`
		Scope     string `json:"scope,omitempty"`
		TimeoutMs int    `json:"timeout_ms,omitempty"`
	}
	type SimulateChainArgs struct {
		SessionID string        `json:"session_id"`
		Edits     []interface{} `json:"edits"`
		TimeoutMs int           `json:"timeout_ms,omitempty"`
	}
	type CommitSessionArgs struct {
		SessionID string `json:"session_id"`
		Target    string `json:"target,omitempty"`
		Apply     bool   `json:"apply,omitempty"`
	}
	type DiscardSessionArgs struct {
		SessionID string `json:"session_id"`
	}
	type DestroySessionArgs struct {
		SessionID string `json:"session_id"`
	}
	type SimulateEditAtomicArgs struct {
		SessionID     string `json:"session_id,omitempty"`
		WorkspaceRoot string `json:"workspace_root,omitempty"`
		Language      string `json:"language,omitempty"`
		FilePath      string `json:"file_path"`
		StartLine     int    `json:"start_line"`
		StartColumn   int    `json:"start_column"`
		EndLine       int    `json:"end_line"`
		EndColumn     int    `json:"end_column"`
		NewText       string `json:"new_text"`
		Scope         string `json:"scope,omitempty"`
		TimeoutMs     int    `json:"timeout_ms,omitempty"`
	}

	// ------- Register all 45 tools -------

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_lsp",
		Description: "Initialize or reinitialize the LSP server with a specific project root directory. Call this before using get_references, get_info_on_location, or get_diagnostics when working in a project different from the one the server was started with. root_dir should be the workspace root (directory containing go.mod, package.json, Cargo.toml, etc.). Optional language_id (e.g. \"go\", \"typescript\", \"rust\") selects a specific configured server in multi-server mode — use this when working in a mixed-language repo to ensure the correct server handles the workspace. If unsure which server is active, call get_server_capabilities first.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args StartLspArgs) (*mcp.CallToolResult, any, error) {
		// If resolver is a ServerManager, use StartAll (all servers) or
		// StartForLanguage (targeted) depending on whether language_id was supplied.
		if sm, ok := resolver.(*lsp.ServerManager); ok {
			if args.LanguageID != "" {
				client, err := sm.StartForLanguage(ctx, args.RootDir, args.LanguageID)
				if err != nil {
					return makeCallToolResult(types.ErrorResult(err.Error())), nil, nil
				}
				cs.set(client)
				return makeCallToolResult(types.TextResult("LSP server started successfully")), nil, nil
			}
			if err := sm.StartAll(ctx, args.RootDir); err != nil {
				return makeCallToolResult(types.ErrorResult(err.Error())), nil, nil
			}
			if c := resolver.DefaultClient(); c != nil {
				cs.set(c)
			}
			return makeCallToolResult(types.TextResult("LSP server started successfully")), nil, nil
		}
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

	type WorkspaceFolderArgs struct {
		Path string `json:"path"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_workspace_folder",
		Description: "Add a directory to the LSP workspace, enabling cross-repo references, definitions, and diagnostics. Useful when working across a library and its consumers — after adding the consumer repo, get_references on a library function returns call sites in both repos. Requires start_lsp to have been called first. Language servers that support multi-root workspaces (gopls, rust-analyzer, typescript-language-server) will re-index the new folder automatically.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args WorkspaceFolderArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleAddWorkspaceFolder(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_workspace_folder",
		Description: "Remove a directory from the LSP workspace. The language server will stop indexing that folder.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args WorkspaceFolderArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRemoveWorkspaceFolder(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_workspace_folders",
		Description: "List all currently active workspace folders. Use this to see which roots the language server is indexing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleListWorkspaceFolders(ctx, cs.get(), nil)
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "open_document",
		Description: "Open a file in the LSP server for analysis. Use this tool before performing operations like getting diagnostics, hover information, or completions for a file. The file remains open for continued analysis until explicitly closed. The language_id parameter tells the server which language service to use (e.g., 'typescript', 'javascript', 'haskell'). The LSP server starts automatically on MCP launch.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args OpenDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleOpenDocument(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "close_document",
		Description: "Close a file in the LSP server. Use this tool when you're done with a file to free up resources and reduce memory usage. It's good practice to close files that are no longer being actively analyzed, especially in long-running sessions or when working with large codebases.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CloseDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCloseDocument(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_diagnostics",
		Description: "Get diagnostic messages (errors, warnings) for files. Use this tool to identify problems in code files such as syntax errors, type mismatches, or other issues detected by the language server. When used without a file_path, returns diagnostics for all open files.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDiagnosticsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDiagnostics(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_info_on_location",
		Description: "Get information on a specific location in a file via LSP hover. Use this tool to retrieve detailed type information, documentation, and other contextual details about symbols in your code. Particularly useful for understanding variable types, function signatures, and module documentation at a specific location in the code. Use this whenever you need to get a better idea on what a particular function is doing in that context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetInfoOnLocationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetInfoOnLocation(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_completions",
		Description: "Get completion suggestions at a specific location in a file. Use this tool to retrieve code completion options based on the current context, including variable names, function calls, object properties, and more. Helpful for code assistance and auto-completion at a particular location. Use this when determining which functions you have available in a given package, for example when changing libraries.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetCompletionsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetCompletions(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_signature_help",
		Description: "Get function signature help at a specific location in a file via LSP. Returns available overloads and highlights the active parameter. Use this when the cursor is inside a function call's argument list to understand what parameters the function expects.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSignatureHelpArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSignatureHelp(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_code_actions",
		Description: "Get code actions for a specific range in a file. Use this tool to obtain available refactorings, quick fixes, and other code modifications that can be applied to a selected code range. Examples include adding imports, fixing errors, or implementing interfaces.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetCodeActionsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetCodeActions(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document_symbols",
		Description: "Get all symbols defined in a document via LSP (functions, classes, variables, methods, etc.). Returns a hierarchical DocumentSymbol tree or flat SymbolInformation list depending on server support. Use this to get a structural overview of a file. Pass format: \"outline\" for compact markdown output (name [Kind] :line) optimized for LLM consumption — ~5x fewer tokens than JSON for the same structural information.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDocumentSymbolsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDocumentSymbols(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_workspace_symbols",
		Description: "Search for symbols across the entire workspace via LSP. Returns all matching symbols with name, kind, and location. detail_level controls enrichment: omit or use \"basic\" for names/locations only; use \"hover\" to also return hover info (type signature + docs) for a paginated window of results. limit (default 3) and offset (default 0) control which symbols get enriched — use offset to step through results without re-running the search.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetWorkspaceSymbolsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetWorkspaceSymbols(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_references",
		Description: "Find all references to a symbol at a specific location in a file via LSP. Returns every location in the codebase where the symbol is used. Use this to determine if a symbol is dead (zero references), to understand call sites before refactoring, or to trace data flow. Results include file path and line/column for each reference.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetReferencesArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetReferences(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	type GetDocumentHighlightsArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document_highlights",
		Description: "Find all occurrences of the symbol at a position within the same file via LSP (textDocument/documentHighlight). Returns ranges and kinds: 1=Text, 2=Read, 3=Write. File-scoped and instant — does not trigger a workspace-wide reference search. Use this to find all local usages of a variable, parameter, or field without the overhead of get_references.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDocumentHighlightsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDocumentHighlights(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_definition",
		Description: "Jump to the definition of a symbol at a specific location in a file via LSP. Returns the file path and position where the symbol is defined. Useful for navigating to type declarations, function implementations, or variable assignments across the codebase.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToDefinitionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToDefinition(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_type_definition",
		Description: "Jump to the definition of the type of a symbol at a specific location in a file via LSP. Unlike go_to_definition (which goes to where the symbol itself is defined), this navigates to the type declaration. Useful for interface types, type aliases, and class definitions when working with instances or variables.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToTypeDefinitionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToTypeDefinition(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_implementation",
		Description: "Find all implementations of an interface or abstract method at a specific location in a file via LSP. Returns the file paths and positions of all concrete implementations. Use this to navigate from an interface declaration or abstract method to the concrete classes that implement it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToImplementationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToImplementation(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_declaration",
		Description: "Jump to the declaration of a symbol at a specific location in a file via LSP. Completes the 'go to X' family alongside go_to_definition, go_to_type_definition, and go_to_implementation. Most useful for languages with separate declaration and definition (e.g., C/C++ header files). Returns the file path and position where the symbol is declared.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToDeclarationArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToDeclaration(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_symbol",
		Description: "Get a WorkspaceEdit for renaming a symbol across the entire workspace via LSP. Returns the edit object — NOT applied automatically. Use dry_run=true to preview what would change (returns workspace_edit + note). Use position_pattern with @@ marker for reliable position targeting instead of line/column. Inspect the returned WorkspaceEdit then call apply_edit to commit.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RenameSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRenameSymbol(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "prepare_rename",
		Description: "Validate that a rename is possible at the given position before committing to rename_symbol. Returns the range that would be renamed and a placeholder name suggestion, or a message indicating rename is not supported at this position. Use this before rename_symbol to avoid attempting invalid renames. Returns null if the server does not support prepareRename.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PrepareRenameArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandlePrepareRename(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "format_document",
		Description: "Get formatting edits for an entire document via LSP. Returns TextEdit[] describing the changes needed to format the file according to the language server's style rules. The edits are returned for inspection — they are NOT applied automatically. Use this to see what formatting changes a formatter would make.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatDocument(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "format_range",
		Description: "Get formatting edits for a specific range within a document via LSP (textDocument/rangeFormatting). Returns TextEdit[] for the selected lines/characters only. Use this when you want to format a function, block, or selection rather than the entire file. The edits are NOT applied automatically.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatRangeArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatRange(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	type GetInlayHintsArgs struct {
		FilePath    string `json:"file_path"`
		LanguageID  string `json:"language_id,omitempty"`
		StartLine   int    `json:"start_line"`
		StartColumn int    `json:"start_column"`
		EndLine     int    `json:"end_line"`
		EndColumn   int    `json:"end_column"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_inlay_hints",
		Description: "Get inlay hints for a range within a document via LSP (textDocument/inlayHint). Inlay hints are inline annotations that IDEs display in source code — typically inferred type names (e.g. `: string`) and parameter name labels (e.g. `count:`). Useful in languages with type inference (TypeScript, Rust, Go) to see what the compiler knows without reading every type annotation. Returns an array of InlayHint objects, each with a position, label, and optional kind (1=Type, 2=Parameter). Returns an empty array if the language server does not support inlay hints.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetInlayHintsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetInlayHints(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_edit",
		Description: "Apply an edit to a file. Two modes: (1) WorkspaceEdit mode — pass workspace_edit with positional changes returned by rename_symbol or format_document; (2) Text-match mode — pass file_path + old_text + new_text to find and replace text without needing line/column positions. Text-match tries exact match first, then whitespace-normalised line match (handles indentation differences). Use text-match when AI-generated positions would be imprecise.",
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
		Name:        "get_server_capabilities",
		Description: "Return the language server's capability map and classify every agent-lsp tool as supported or unsupported based on what the server advertised during initialization. Use this to determine which tools will return results before calling them — saves round trips on servers that don't support certain LSP features (e.g. not all servers support type_hierarchy or inlay_hints). Requires start_lsp to have been called first.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetServerCapabilities(ctx, cs.get(), nil)
		return makeCallToolResult(r), nil, err
	})

	type DetectLspServersArgs struct {
		WorkspaceDir string `json:"workspace_dir"`
	}
	type RunBuildArgs struct {
		WorkspaceDir string `json:"workspace_dir"`
		Path         string `json:"path,omitempty"`
		Language     string `json:"language,omitempty"`
	}
	type RunTestsArgs struct {
		WorkspaceDir string `json:"workspace_dir"`
		Path         string `json:"path,omitempty"`
		Language     string `json:"language,omitempty"`
	}
	type GetTestsForFileArgs struct {
		FilePath string `json:"file_path"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "detect_lsp_servers",
		Description: "Scan a workspace directory for source languages and check PATH for the corresponding LSP server binaries. Returns detected workspace languages (ranked by prevalence), installed servers with their paths, and a suggested_config array ready to paste into the agent-lsp MCP server args. Use this to set up agent-lsp for a new project or verify your configuration.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DetectLspServersArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDetectLspServers(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_build",
		Description: "Compile the project at workspace_dir using the detected workspace language. Language-specific dispatch (no arbitrary shell execution): go build ./..., cargo build, tsc --noEmit, mypy . (Python typecheck proxy). Optional path param narrows scope. Returns: { success: bool, errors: [{file, line, column, message}], raw: string }. Does not require start_lsp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RunBuildArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRunBuild(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_tests",
		Description: "Run the test suite for the detected workspace language. Language-specific dispatch: go test -json ./..., cargo test --message-format=json, pytest --tb=json, npm test. Optional path param narrows scope. Test failure locations are LSP-normalized — paste directly into go_to_definition. Returns: { passed: bool, failures: [{file, line, test_name, message, location}], raw: string }. Does not require start_lsp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RunTestsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRunTests(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_tests_for_file",
		Description: "Given a source file path, return the test files that exercise it. Static lookup — no test execution. Go: *_test.go in same directory. Python: test_*.py / *_test.py in same dir and tests/ sibling. TypeScript/JS: *.test.ts, *.spec.ts etc. Rust: returns source file itself (tests inline). Does not require start_lsp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetTestsForFileArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetTestsForFile(ctx, toolArgsToMap(args))
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
		r, err := tools.HandleCallHierarchy(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "type_hierarchy",
		Description: "Show type hierarchy for a type at a position. Returns supertypes (parent classes/interfaces), subtypes (subclasses/implementations), or both depending on the direction parameter. Direction defaults to \"both\". Use this to understand class and interface inheritance relationships.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args TypeHierarchyArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleTypeHierarchy(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_semantic_tokens",
		Description: "Get semantic tokens for a range in a file. Returns each token's type (function, variable, keyword, parameter, type, etc.) and modifiers (readonly, static, deprecated, etc.) with 1-based line/character positions. Use this to understand the syntactic role of code elements — distinct from hover which gives documentation. Only available when the language server supports textDocument/semanticTokens.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSemanticTokensArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSemanticTokens(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_simulation_session",
		Description: "Create a new speculative code session for simulating edits without committing to disk. Returns a session ID. Baseline diagnostics are captured lazily on first edit per file. Use this to explore what-if scenarios before applying changes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateSimulationSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCreateSimulationSession(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "simulate_edit",
		Description: "Apply a range edit to a file within a simulation session. Changes are held in-memory only. The session captures baseline diagnostics on first edit to each file, then tracks versions for subsequent edits. Returns the new version number after the edit. All line/column positions are 1-indexed (matching editor line numbers).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateEditArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateEdit(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "evaluate_session",
		Description: "Evaluate a simulation session by comparing current diagnostics against baselines. Returns errors introduced, errors resolved, net delta, and confidence (high for file scope, eventual for workspace). Use after simulate_edit to assess impact before committing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EvaluateSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleEvaluateSession(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "simulate_chain",
		Description: "Apply a sequence of edits and evaluate after each step. Returns per-step diagnostics and identifies the safe-to-apply-through step (last step with net delta == 0). Use this to find the safest partial application of a multi-step change. All line/column positions in each edit are 1-indexed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateChainArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateChain(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "commit_session",
		Description: "Commit a simulation session. With apply=true, writes changes to disk and notifies LSP servers. With apply=false, returns a unified diff patch. Use after evaluate_session confirms the changes are safe.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CommitSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCommitSession(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "discard_session",
		Description: "Discard a simulation session and revert all in-memory changes by restoring baseline content. Use when simulation results show the changes would introduce errors.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DiscardSessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDiscardSession(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "destroy_session",
		Description: "Destroy a simulation session and release all resources. Call this after commit or discard to clean up. Sessions in terminal states (committed, discarded, destroyed) cannot be reused.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DestroySessionArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDestroySession(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "simulate_edit_atomic",
		Description: "One-shot atomic operation: create session, apply edit, evaluate, and destroy. Returns evaluation result. Use for quick what-if checks without managing session lifecycle manually. Requires start_lsp to be called first. All line/column positions are 1-indexed. net_delta: 0 means the edit is safe to apply.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SimulateEditAtomicArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSimulateEditAtomic(ctx, sessionMgr, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	type GoToSymbolArgs struct {
		SymbolPath    string `json:"symbol_path"`
		WorkspaceRoot string `json:"workspace_root,omitempty"`
		Language      string `json:"language,omitempty"`
	}

	type GetSymbolSourceArgs struct {
		FilePath        string `json:"file_path"`
		LanguageID      string `json:"language_id,omitempty"`
		Line            int    `json:"line,omitempty"`
		Character       int    `json:"character,omitempty"`
		PositionPattern string `json:"position_pattern,omitempty"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "go_to_symbol",
		Description: "Navigate to a symbol definition by dot-notation name (e.g. \"LSPClient.GetReferences\", \"http.Handler\") without needing file_path or line/column. Uses workspace symbol search to locate the definition. Useful when you know the symbol name but not its location.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GoToSymbolArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGoToSymbol(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_symbol_source",
		Description: "Return the source code of the innermost symbol (function, method, class, struct, etc.) whose range contains the given cursor position. Calls textDocument/documentSymbol, walks the symbol tree to find the smallest enclosing symbol, then slices the file at that symbol's range. Returns symbol_name, symbol_kind, start_line (1-based), end_line (1-based), and source text. Use line+character or position_pattern (@@-syntax) to specify the cursor. character defaults to 1.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSymbolSourceArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSymbolSource(ctx, clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
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

	// Register URI templates for dynamic resource discovery.
	for _, tmpl := range resources.ResourceTemplates() {
		t := tmpl // capture loop variable
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			Name:        t.Name,
			URITemplate: t.URITemplate,
			Description: t.Description,
		}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		})
	}

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
	logging.Log(logging.LevelInfo, "agent-lsp server starting")

	transport := &mcp.StdioTransport{}
	return server.Run(ctx, transport)
}
