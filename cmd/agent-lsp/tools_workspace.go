package main

import (
	"context"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Workspace/lifecycle tool arg types.

type StartLspArgs struct {
	RootDir    string `json:"root_dir"`
	LanguageID string `json:"language_id,omitempty"`
}

type RestartLspArgs struct {
	RootDir string `json:"root_dir,omitempty"`
}

type WorkspaceFolderArgs struct {
	Path string `json:"path"`
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

type ApplyEditArgs struct {
	Edit     map[string]interface{} `json:"workspace_edit,omitempty"`
	FilePath string                 `json:"file_path,omitempty"`
	OldText  string                 `json:"old_text,omitempty"`
	NewText  string                 `json:"new_text,omitempty"`
}

type ExecuteCommandArgs struct {
	Command   string        `json:"command"`
	Arguments []map[string]interface{} `json:"arguments,omitempty"`
}

type DidChangeWatchedFilesArgs struct {
	Changes []map[string]interface{} `json:"changes"`
}

type SetLogLevelArgs struct {
	Level string `json:"level"`
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

func registerWorkspaceTools(d toolDeps) {
	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "start_lsp",
		Description: "Initialize or reinitialize the LSP server with a specific project root directory. Call this before using get_references, get_info_on_location, or get_diagnostics when working in a project different from the one the server was started with. root_dir should be the workspace root (directory containing go.mod, package.json, Cargo.toml, etc.). Optional language_id (e.g. \"go\", \"typescript\", \"rust\") selects a specific configured server in multi-server mode — use this when working in a mixed-language repo to ensure the correct server handles the workspace. If unsure which server is active, call get_server_capabilities first.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args StartLspArgs) (*mcp.CallToolResult, any, error) {
		if sm, ok := d.resolver.(*lsp.ServerManager); ok {
			if args.LanguageID != "" {
				client, err := sm.StartForLanguage(ctx, args.RootDir, args.LanguageID)
				if err != nil {
					return makeCallToolResult(types.ErrorResult(err.Error())), nil, nil
				}
				d.cs.set(client)
				return makeCallToolResult(types.TextResult("LSP server started successfully")), nil, nil
			}
			if err := sm.StartAll(ctx, args.RootDir); err != nil {
				return makeCallToolResult(types.ErrorResult(err.Error())), nil, nil
			}
			if c := d.resolver.DefaultClient(); c != nil {
				d.cs.set(c)
			}
			return makeCallToolResult(types.TextResult("LSP server started successfully")), nil, nil
		}
		r, err := tools.HandleStartLsp(ctx, d.cs.get, d.cs.set, d.serverPath, d.serverArgs, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "restart_lsp_server",
		Description: "Restart the LSP server process. Use this if the LSP server becomes unresponsive or after making significant changes to the project structure. Optionally provide a new root_dir to restart with a different workspace root.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RestartLspArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRestartLspServer(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "add_workspace_folder",
		Description: "Add a directory to the LSP workspace, enabling cross-repo references, definitions, and diagnostics. Useful when working across a library and its consumers — after adding the consumer repo, get_references on a library function returns call sites in both repos. Requires start_lsp to have been called first. Language servers that support multi-root workspaces (gopls, rust-analyzer, typescript-language-server) will re-index the new folder automatically.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args WorkspaceFolderArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleAddWorkspaceFolder(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "remove_workspace_folder",
		Description: "Remove a directory from the LSP workspace. The language server will stop indexing that folder.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args WorkspaceFolderArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRemoveWorkspaceFolder(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "list_workspace_folders",
		Description: "List all currently active workspace folders. Use this to see which roots the language server is indexing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleListWorkspaceFolders(ctx, d.cs.get(), nil)
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "open_document",
		Description: "Open a file in the LSP server for analysis. Use this tool before performing operations like getting diagnostics, hover information, or completions for a file. The file remains open for continued analysis until explicitly closed. The language_id parameter tells the server which language service to use (e.g., 'typescript', 'javascript', 'haskell'). The LSP server starts automatically on MCP launch.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args OpenDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleOpenDocument(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "close_document",
		Description: "Close a file in the LSP server. Use this tool when you're done with a file to free up resources and reduce memory usage. It's good practice to close files that are no longer being actively analyzed, especially in long-running sessions or when working with large codebases.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CloseDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCloseDocument(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_diagnostics",
		Description: "Get diagnostic messages (errors, warnings) for files. Use this tool to identify problems in code files such as syntax errors, type mismatches, or other issues detected by the language server. When used without a file_path, returns diagnostics for all open files.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDiagnosticsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDiagnostics(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_server_capabilities",
		Description: "Return the language server's capability map and classify every agent-lsp tool as supported or unsupported based on what the server advertised during initialization. Use this to determine which tools will return results before calling them — saves round trips on servers that don't support certain LSP features (e.g. not all servers support type_hierarchy or inlay_hints). Requires start_lsp to have been called first.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetServerCapabilities(ctx, d.cs.get(), nil)
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "detect_lsp_servers",
		Description: "Scan a workspace directory for source languages and check PATH for the corresponding LSP server binaries. Returns detected workspace languages (ranked by prevalence), installed servers with their paths, and a suggested_config array ready to paste into the agent-lsp MCP server args. Use this to set up agent-lsp for a new project or verify your configuration.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DetectLspServersArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDetectLspServers(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "run_build",
		Description: "Compile the project at workspace_dir using the detected workspace language. Language-specific dispatch (no arbitrary shell execution): go build ./..., cargo build, tsc --noEmit, mypy . (Python typecheck proxy). Optional path param narrows scope. Returns: { success: bool, errors: [{file, line, column, message}], raw: string }. Does not require start_lsp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RunBuildArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRunBuild(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "run_tests",
		Description: "Run the test suite for the detected workspace language. Language-specific dispatch: go test -json ./..., cargo test --message-format=json, pytest --tb=json, npm test. Optional path param narrows scope. Test failure locations are LSP-normalized — paste directly into go_to_definition. Returns: { passed: bool, failures: [{file, line, test_name, message, location}], raw: string }. Does not require start_lsp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RunTestsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRunTests(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "get_tests_for_file",
		Description: "Given a source file path, return the test files that exercise it. Static lookup — no test execution. Go: *_test.go in same directory. Python: test_*.py / *_test.py in same dir and tests/ sibling. TypeScript/JS: *.test.ts, *.spec.ts etc. Rust: returns source file itself (tests inline). Does not require start_lsp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetTestsForFileArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetTestsForFile(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "set_log_level",
		Description: "Set the server logging level. Use this tool to control the verbosity of logs generated by the LSP MCP server. Available levels from least to most verbose: emergency, alert, critical, error, warning, notice, info, debug. Increasing verbosity can help troubleshoot issues but may generate large amounts of output.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SetLogLevelArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSetLogLevel(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "apply_edit",
		Description: "Apply an edit to a file. Two modes: (1) WorkspaceEdit mode — pass workspace_edit with positional changes returned by rename_symbol or format_document; (2) Text-match mode — pass file_path + old_text + new_text to find and replace text without needing line/column positions. Text-match tries exact match first, then whitespace-normalised line match (handles indentation differences). Use text-match when AI-generated positions would be imprecise.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ApplyEditArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleApplyEdit(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "execute_command",
		Description: "Execute a workspace command via LSP. Commands are server-defined identifiers returned by code actions (in the command field of a CodeAction). Use this after get_code_actions to trigger a server-side operation such as applying a refactoring, generating code, or running a server-specific action. Returns the server-defined result or null.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ExecuteCommandArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleExecuteCommand(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "did_change_watched_files",
		Description: "Notify the language server that files have changed on disk outside the editor (workspace/didChangeWatchedFiles). Use this after writing files directly to disk so the server refreshes its caches. Change types: 1=created, 2=changed, 3=deleted. File URIs must use the file:/// scheme.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DidChangeWatchedFilesArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDidChangeWatchedFiles(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "format_document",
		Description: "Get formatting edits for an entire document via LSP. Returns TextEdit[] describing the changes needed to format the file according to the language server's style rules. The edits are returned for inspection — they are NOT applied automatically. Use this to see what formatting changes a formatter would make.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatDocument(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	mcp.AddTool(d.server, &mcp.Tool{
		Name:        "format_range",
		Description: "Get formatting edits for a specific range within a document via LSP (textDocument/rangeFormatting). Returns TextEdit[] for the selected lines/characters only. Use this when you want to format a function, block, or selection rather than the entire file. The edits are NOT applied automatically.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatRangeArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatRange(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
