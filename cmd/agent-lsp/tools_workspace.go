// tools_workspace.go defines the MCP tool registrations for workspace and
// lifecycle operations: start_lsp, restart_lsp, open/close_document,
// get_diagnostics, apply_edit, format_document, format_range, execute_command,
// did_change_watched_files, detect_lsp_servers, and set_log_level.
//
// Each tool follows the same pattern:
//   1. An Args struct with jsonschema tags (drives MCP schema generation).
//   2. A handler registered via addToolWithPhaseCheck that converts the
//      typed args to the internal map format and delegates to internal/tools.
//
// The args structs use jsonschema tags rather than separate schema definitions.
// The go-sdk reads these tags at startup to generate the MCP tool schema
// that clients use for argument validation and documentation.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Workspace/lifecycle tool arg types.

type StartLspArgs struct {
	RootDir              string  `json:"root_dir" jsonschema:"Workspace root directory containing the project (e.g. directory with go.mod, package.json)"`
	LanguageID           string  `json:"language_id,omitempty" jsonschema:"Language server to start (e.g. go, typescript, rust). Optional; auto-detected"`
	ReadyTimeoutSeconds  float64 `json:"ready_timeout_seconds,omitempty" jsonschema:"If > 0, block until all $/progress workspace-indexing tokens complete or this many seconds elapse. Useful for servers like jdtls that index asynchronously after initialize."`
	Scope                string  `json:"scope,omitempty" jsonschema:"Limit indexing to specific subdirectories. Accepts a path string or array of paths relative to root_dir. Generates a temporary language-server config (pyrightconfig.json, tsconfig.json) that restricts analysis scope. Use on large monorepos to prevent reference query timeouts."`
}

type RestartLspArgs struct {
	RootDir string `json:"root_dir,omitempty" jsonschema:"Optional new workspace root. If omitted, restarts with current root"`
}

type WorkspaceFolderArgs struct {
	Path string `json:"path" jsonschema:"Absolute path to the workspace folder to add/remove"`
}

type OpenDocumentArgs struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path to the file to open in the LSP server"`
	LanguageID string `json:"language_id,omitempty"`
	Text       string `json:"text,omitempty" jsonschema:"Optional file content override. If omitted, reads from disk"`
}

type CloseDocumentArgs struct {
	FilePath string `json:"file_path" jsonschema:"Absolute path to the file to close"`
}

type GetDiagnosticsArgs struct {
	FilePath string `json:"file_path,omitempty" jsonschema:"File path to get diagnostics for. If omitted, returns diagnostics for all open files"`
}

type ApplyEditArgs struct {
	Edit     map[string]interface{} `json:"workspace_edit,omitempty" jsonschema:"WorkspaceEdit object (as returned by rename_symbol or format_document)"`
	FilePath string                 `json:"file_path,omitempty" jsonschema:"File path for text-match mode"`
	OldText  string                 `json:"old_text,omitempty" jsonschema:"Text to find and replace (text-match mode)"`
	NewText  string                 `json:"new_text,omitempty" jsonschema:"Replacement text (text-match mode)"`
}

type ExecuteCommandArgs struct {
	Command   string                   `json:"command" jsonschema:"LSP command identifier (from code action's command field)"`
	Arguments []map[string]interface{} `json:"arguments,omitempty" jsonschema:"Command arguments as array of JSON objects"`
}

type DidChangeWatchedFilesArgs struct {
	Changes []map[string]interface{} `json:"changes" jsonschema:"Array of file change events: [{uri\\, type}] where type is 1=created\\, 2=changed\\, 3=deleted"`
}

type SetLogLevelArgs struct {
	Level string `json:"level" jsonschema:"Log level: emergency\\, alert\\, critical\\, error\\, warning\\, notice\\, info\\, or debug"`
}

type FormatDocumentArgs struct {
	FilePath     string `json:"file_path" jsonschema:"Absolute path to the file to format"`
	LanguageID   string `json:"language_id,omitempty"`
	TabSize      int    `json:"tab_size,omitempty" jsonschema:"Tab size in spaces. Default: 4"`
	InsertSpaces *bool  `json:"insert_spaces,omitempty" jsonschema:"Use spaces instead of tabs. Default: true"`
}

type FormatRangeArgs struct {
	FilePath    string `json:"file_path" jsonschema:"Absolute path to the file to format"`
	LanguageID  string `json:"language_id,omitempty"`
	StartLine   int    `json:"start_line" jsonschema:"1-indexed start line of the range to format"`
	StartColumn int    `json:"start_column" jsonschema:"1-indexed start column of the range to format"`
	EndLine     int    `json:"end_line" jsonschema:"1-indexed end line of the range to format"`
	EndColumn   int    `json:"end_column" jsonschema:"1-indexed end column of the range to format"`
	TabSize     int    `json:"tab_size,omitempty" jsonschema:"Tab size in spaces. Default: 4"`
	InsertSpaces *bool `json:"insert_spaces,omitempty" jsonschema:"Use spaces instead of tabs. Default: true"`
}

type DetectLspServersArgs struct {
	WorkspaceDir string `json:"workspace_dir" jsonschema:"Directory to scan for source languages and LSP server binaries"`
}

type RunBuildArgs struct {
	WorkspaceDir string `json:"workspace_dir" jsonschema:"Workspace directory to build"`
	Path         string `json:"path,omitempty" jsonschema:"Optional sub-path to narrow build scope"`
	Language     string `json:"language,omitempty" jsonschema:"Optional language override (go\\, rust\\, typescript\\, python)"`
}

type RunTestsArgs struct {
	WorkspaceDir string `json:"workspace_dir" jsonschema:"Workspace directory to test"`
	Path         string `json:"path,omitempty" jsonschema:"Optional sub-path to narrow test scope"`
	Language     string `json:"language,omitempty" jsonschema:"Optional language override (go\\, rust\\, typescript\\, python)"`
}

type GetTestsForFileArgs struct {
	FilePath string `json:"file_path" jsonschema:"Source file path to find associated test files for"`
}

type ExportCacheArgs struct {
	DestPath string `json:"dest_path" jsonschema:"Destination path for the compressed cache artifact (e.g. .agent-lsp/cache.db.gz)"`
}

type ImportCacheArgs struct {
	SrcPath string `json:"src_path" jsonschema:"Path to the compressed cache artifact to import"`
}

func registerWorkspaceTools(d toolDeps) {
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "start_lsp",
		Description: "Initialize or reinitialize the LSP server with a specific project root directory. Call this before using get_references, get_info_on_location, or get_diagnostics when working in a project different from the one the server was started with. root_dir should be the workspace root (directory containing go.mod, package.json, Cargo.toml, etc.). Optional language_id (e.g. \"go\", \"typescript\", \"rust\") selects a specific configured server in multi-server mode — use this when working in a mixed-language repo to ensure the correct server handles the workspace. If unsure which server is active, call get_server_capabilities first.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Start LSP Server",
			ReadOnlyHint:    false,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args StartLspArgs) (*mcp.CallToolResult, any, error) {
		if sm, ok := d.resolver.(*lsp.ServerManager); ok {
			if args.LanguageID != "" {
				// Apply workspace scoping before starting the language server.
				if args.Scope != "" {
					scopePaths := tools.ParseScopePaths(args.Scope)
					if len(scopePaths) > 0 {
						sc, err := lsp.GenerateScopeConfig(args.RootDir, args.LanguageID, scopePaths)
						if err != nil {
							return makeCallToolResult(types.ErrorResult(fmt.Sprintf("failed to generate scope config: %s", err))), nil, nil
						}
						// Cleanup deferred to client.Shutdown via SetScopeConfig below.
						defer func() {
							if c := d.cs.get(); c != nil && sc != nil {
								c.SetScopeConfig(sc)
							}
						}()
					}
				}
				client, err := sm.StartForLanguage(ctx, args.RootDir, args.LanguageID)
				if err != nil {
					return makeCallToolResult(types.ErrorResult(err.Error())), nil, nil
				}
				d.cs.set(client)
				// Block until workspace indexing completes if requested.
				if args.ReadyTimeoutSeconds > 0 {
					client.WaitForWorkspaceReadyTimeout(ctx, time.Duration(args.ReadyTimeoutSeconds)*time.Second)
				}
				return makeCallToolResult(types.TextResult("LSP server started successfully")), nil, nil
			}
			if err := sm.StartAll(ctx, args.RootDir); err != nil {
				return makeCallToolResult(types.ErrorResult(err.Error())), nil, nil
			}
			if c := d.resolver.DefaultClient(); c != nil {
				d.cs.set(c)
			}
			// Block until workspace indexing completes if requested.
			// Servers like jdtls emit $/progress tokens during Maven/Gradle
			// import; waiting here lets start_lsp return only when ready.
			if args.ReadyTimeoutSeconds > 0 {
				if c := d.resolver.DefaultClient(); c != nil {
					c.WaitForWorkspaceReadyTimeout(ctx, time.Duration(args.ReadyTimeoutSeconds)*time.Second)
				}
			}
			return makeCallToolResult(types.TextResult("LSP server started successfully")), nil, nil
		}
		r, err := tools.HandleStartLsp(ctx, d.cs.get, d.cs.set, d.serverPath, d.serverArgs, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "restart_lsp_server",
		Description: "Restart the LSP server process. Use this if the LSP server becomes unresponsive or after making significant changes to the project structure. Optionally provide a new root_dir to restart with a different workspace root.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Restart LSP Server",
			ReadOnlyHint:    false,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RestartLspArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRestartLspServer(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "add_workspace_folder",
		Description: "Add a directory to the LSP workspace, enabling cross-repo references, definitions, and diagnostics. Useful when working across a library and its consumers — after adding the consumer repo, get_references on a library function returns call sites in both repos. Requires start_lsp to have been called first. Language servers that support multi-root workspaces (gopls, rust-analyzer, typescript-language-server) will re-index the new folder automatically.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Add Workspace Folder",
			ReadOnlyHint:    false,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args WorkspaceFolderArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleAddWorkspaceFolder(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "remove_workspace_folder",
		Description: "Remove a directory from the LSP workspace. The language server will stop indexing that folder.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Remove Workspace Folder",
			ReadOnlyHint:    false,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args WorkspaceFolderArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRemoveWorkspaceFolder(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "list_workspace_folders",
		Description: "List all currently active workspace folders. Use this to see which roots the language server is indexing.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Workspace Folders",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleListWorkspaceFolders(ctx, d.cs.get(), nil)
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "open_document",
		Description: "Open a file in the LSP server for analysis. Use this tool before performing operations like getting diagnostics, hover information, or completions for a file. The file remains open for continued analysis until explicitly closed. The language_id parameter tells the server which language service to use (e.g., 'typescript', 'javascript', 'haskell'). The LSP server starts automatically on MCP launch.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Open Document",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args OpenDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleOpenDocument(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "close_document",
		Description: "Close a file in the LSP server. Use this tool when you're done with a file to free up resources and reduce memory usage. It's good practice to close files that are no longer being actively analyzed, especially in long-running sessions or when working with large codebases.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Close Document",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CloseDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCloseDocument(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "get_diagnostics",
		Description: "Get diagnostic messages (errors, warnings) for files. Use this tool to identify problems in code files such as syntax errors, type mismatches, or other issues detected by the language server. When used without a file_path, returns diagnostics for all open files.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Diagnostics",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetDiagnosticsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetDiagnostics(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "get_server_capabilities",
		Description: "Return the language server's capability map and classify every agent-lsp tool as supported or unsupported based on what the server advertised during initialization. Use this to determine which tools will return results before calling them — saves round trips on servers that don't support certain LSP features (e.g. not all servers support type_hierarchy or inlay_hints). Requires start_lsp to have been called first.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Server Capabilities",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetServerCapabilities(ctx, d.cs.get(), nil)
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "detect_lsp_servers",
		Description: "Scan a workspace directory for source languages and check PATH for the corresponding LSP server binaries. Returns detected workspace languages (ranked by prevalence), installed servers with their paths, and a suggested_config array ready to paste into the agent-lsp MCP server args. Use this to set up agent-lsp for a new project or verify your configuration.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Detect LSP Servers",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DetectLspServersArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDetectLspServers(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "run_build",
		Description: "Compile the project at workspace_dir using the detected workspace language. Language-specific dispatch (no arbitrary shell execution): go build ./..., cargo build, tsc --noEmit, mypy . (Python typecheck proxy). Optional path param narrows scope. Returns: { success: bool, errors: [{file, line, column, message}], raw: string }. Does not require start_lsp.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Run Build",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RunBuildArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRunBuild(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "run_tests",
		Description: "Run the test suite for the detected workspace language. Language-specific dispatch: go test -json ./..., cargo test --message-format=json, pytest --tb=json, npm test. Optional path param narrows scope. Test failure locations are LSP-normalized — paste directly into go_to_definition. Returns: { passed: bool, failures: [{file, line, test_name, message, location}], raw: string }. Does not require start_lsp.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Run Tests",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RunTestsArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleRunTests(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "get_tests_for_file",
		Description: "Given a source file path, return the test files that exercise it. Static lookup — no test execution. Go: *_test.go in same directory. Python: test_*.py / *_test.py in same dir and tests/ sibling. TypeScript/JS: *.test.ts, *.spec.ts etc. Rust: returns source file itself (tests inline). Does not require start_lsp.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Tests for File",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetTestsForFileArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetTestsForFile(ctx, toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "set_log_level",
		Description: "Set the server logging level. Use this tool to control the verbosity of logs generated by the LSP MCP server. Available levels from least to most verbose: emergency, alert, critical, error, warning, notice, info, debug. Increasing verbosity can help troubleshoot issues but may generate large amounts of output.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Set Log Level",
			ReadOnlyHint:    false,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SetLogLevelArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleSetLogLevel(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "apply_edit",
		Description: "Apply an edit to a file. Two modes: (1) WorkspaceEdit mode — pass workspace_edit with positional changes returned by rename_symbol or format_document; (2) Text-match mode — pass file_path + old_text + new_text to find and replace text without needing line/column positions. Text-match tries exact match first, then whitespace-normalised line match (handles indentation differences). Use text-match when AI-generated positions would be imprecise.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Apply Edit",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ApplyEditArgs) (*mcp.CallToolResult, any, error) {
		startTime := time.Now()
		client := d.cs.get()

		// Determine affected files and build edit summary.
		var files []string
		var summary audit.EditSummary
		if args.FilePath != "" && args.OldText != "" {
			files = []string{args.FilePath}
			summary = audit.EditSummary{
				Mode:           "text-match",
				FilePath:       args.FilePath,
				OldTextPreview: audit.Truncate(args.OldText, 200),
				NewTextPreview: audit.Truncate(args.NewText, 200),
			}
		} else if args.Edit != nil {
			files = extractFilesFromWorkspaceEdit(args.Edit)
			summary = audit.EditSummary{Mode: "workspace-edit"}
		}

		diagsBefore := snapshotDiagnostics(client, files)

		r, err := tools.HandleApplyEdit(ctx, client, toolArgsToMap(args))

		diagsAfter := snapshotDiagnostics(client, files)
		delta := computeDelta(diagsBefore, diagsAfter)

		record := audit.Record{
			Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
			Tool:              "apply_edit",
			Files:             files,
			EditSummary:       &summary,
			DiagnosticsBefore: diagsBefore,
			DiagnosticsAfter:  diagsAfter,
			NetDelta:          delta,
			Success:           !isToolResultError(r),
			DurationMs:        time.Since(startTime).Milliseconds(),
		}
		if isToolResultError(r) {
			record.ErrorMessage = toolResultErrorMsg(r)
		}
		d.auditLogger.Log(record)

		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "execute_command",
		Description: "Execute a workspace command via LSP. Commands are server-defined identifiers returned by code actions (in the command field of a CodeAction). Use this after get_code_actions to trigger a server-side operation such as applying a refactoring, generating code, or running a server-specific action. Returns the server-defined result or null.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Execute Command",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ExecuteCommandArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleExecuteCommand(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "did_change_watched_files",
		Description: "Notify the language server that files have changed on disk outside the editor (workspace/didChangeWatchedFiles). Use this after writing files directly to disk so the server refreshes its caches. Change types: 1=created, 2=changed, 3=deleted. File URIs must use the file:/// scheme.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Notify File Changes",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DidChangeWatchedFilesArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleDidChangeWatchedFiles(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "format_document",
		Description: "Get formatting edits for an entire document via LSP. Returns TextEdit[] describing the changes needed to format the file according to the language server's style rules. The edits are returned for inspection — they are NOT applied automatically. Use this to see what formatting changes a formatter would make.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Format Document",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatDocumentArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatDocument(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "format_range",
		Description: "Get formatting edits for a specific range within a document via LSP (textDocument/rangeFormatting). Returns TextEdit[] for the selected lines/characters only. Use this when you want to format a function, block, or selection rather than the entire file. The edits are NOT applied automatically.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Format Range",
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FormatRangeArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleFormatRange(ctx, d.clientForFileWithAutoInit(args.FilePath), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "export_cache",
		Description: "Export the symbol reference cache as a gzip-compressed artifact for team sharing. The exported file can be committed to the repository (e.g. .agent-lsp/cache.db.gz) so teammates skip cold-start indexing. Requires start_lsp to have been called first.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Export Cache",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ExportCacheArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleExportCache(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})

	addToolWithPhaseCheck(d, &mcp.Tool{
		Name:        "import_cache",
		Description: "Import a gzip-compressed cache artifact, replacing the current symbol reference cache. Use this to load a team-shared cache exported via export_cache. Validates database integrity after import. Requires start_lsp to have been called first.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Import Cache",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ImportCacheArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleImportCache(ctx, d.cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
}
