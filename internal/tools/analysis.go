package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// HandleGetDiagnostics retrieves LSP diagnostics for a file or all open documents.
func HandleGetDiagnostics(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, _ := args["file_path"].(string)

	var diagMap map[string][]types.LSPDiagnostic

	if filePath != "" {
		fileURI := CreateFileURI(filePath)
		if err := client.ReopenDocument(ctx, fileURI); err != nil {
			return types.ErrorResult(fmt.Sprintf("failed to reopen document: %s", err)), nil
		}
		if err := lsp.WaitForDiagnostics(ctx, client, []string{fileURI}, 10000); err != nil {
			return types.ErrorResult(fmt.Sprintf("waiting for diagnostics: %s", err)), nil
		}
		diags := client.GetDiagnostics(fileURI)
		diagMap = map[string][]types.LSPDiagnostic{fileURI: diags}
	} else {
		if err := client.ReopenAllDocuments(ctx); err != nil {
			return types.ErrorResult(fmt.Sprintf("failed to reopen documents: %s", err)), nil
		}
		openURIs := client.GetOpenDocuments()
		if err := lsp.WaitForDiagnostics(ctx, client, openURIs, 10000); err != nil {
			return types.ErrorResult(fmt.Sprintf("waiting for diagnostics: %s", err)), nil
		}
		all := client.GetAllDiagnostics()
		// Filter to only open documents.
		openSet := make(map[string]bool, len(openURIs))
		for _, u := range openURIs {
			openSet[u] = true
		}
		diagMap = make(map[string][]types.LSPDiagnostic)
		for uri, diags := range all {
			if openSet[uri] {
				diagMap[uri] = diags
			}
		}
	}

	data, err := json.Marshal(diagMap)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling diagnostics: %s", err)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleGetInfoOnLocation retrieves hover information at a source location.
func HandleGetInfoOnLocation(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	line, col, err := extractPosition(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[string](ctx, client, filePath, languageID, func(fileURI string) (string, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetInfoOnLocation(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_info_on_location: %s", wErr)), nil
	}
	return types.TextResult(result), nil
}

// HandleGetCompletions retrieves completion suggestions at a source location.
func HandleGetCompletions(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	line, col, err := extractPosition(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[[]interface{}](ctx, client, filePath, languageID, func(fileURI string) ([]interface{}, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetCompletion(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_completions: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling completions: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleGetSignatureHelp retrieves signature help at a source location.
func HandleGetSignatureHelp(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	line, col, err := extractPosition(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[interface{}](ctx, client, filePath, languageID, func(fileURI string) (interface{}, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetSignatureHelp(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_signature_help: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling signature help: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleGetCodeActions retrieves code actions for a range in a document.
func HandleGetCodeActions(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	rng, err := extractRange(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[[]interface{}](ctx, client, filePath, languageID, func(fileURI string) ([]interface{}, error) {
		return client.GetCodeActions(ctx, fileURI, rng)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_code_actions: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling code actions: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleGetDocumentSymbols retrieves the symbols defined in a document.
func HandleGetDocumentSymbols(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[[]interface{}](ctx, client, filePath, languageID, func(fileURI string) ([]interface{}, error) {
		return client.GetDocumentSymbols(ctx, fileURI)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_document_symbols: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling document symbols: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleGetWorkspaceSymbols searches for symbols across the workspace.
func HandleGetWorkspaceSymbols(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	query, _ := args["query"].(string)
	// query may be empty (returns all symbols)

	result, err := client.GetWorkspaceSymbols(ctx, query)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("get_workspace_symbols: %s", err)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling workspace symbols: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// extractPosition reads line and column from args, validates 1-indexed.
func extractPosition(args map[string]interface{}) (line, col int, err error) {
	line, err = toInt(args, "line")
	if err != nil {
		return 0, 0, fmt.Errorf("line: %w", err)
	}
	if line < 1 {
		return 0, 0, fmt.Errorf("line must be >= 1, got %d", line)
	}

	col, err = toInt(args, "column")
	if err != nil {
		return 0, 0, fmt.Errorf("column: %w", err)
	}
	if col < 1 {
		return 0, 0, fmt.Errorf("column must be >= 1, got %d", col)
	}

	return line, col, nil
}

// extractRange reads start/end line and column from args, validates 1-indexed and ordering.
func extractRange(args map[string]interface{}) (types.Range, error) {
	startLine, err := toInt(args, "start_line")
	if err != nil {
		return types.Range{}, fmt.Errorf("start_line: %w", err)
	}
	if startLine < 1 {
		return types.Range{}, fmt.Errorf("start_line must be >= 1, got %d", startLine)
	}

	startCol, err := toInt(args, "start_column")
	if err != nil {
		return types.Range{}, fmt.Errorf("start_column: %w", err)
	}
	if startCol < 1 {
		return types.Range{}, fmt.Errorf("start_column must be >= 1, got %d", startCol)
	}

	endLine, err := toInt(args, "end_line")
	if err != nil {
		return types.Range{}, fmt.Errorf("end_line: %w", err)
	}
	if endLine < 1 {
		return types.Range{}, fmt.Errorf("end_line must be >= 1, got %d", endLine)
	}

	endCol, err := toInt(args, "end_column")
	if err != nil {
		return types.Range{}, fmt.Errorf("end_column: %w", err)
	}
	if endCol < 1 {
		return types.Range{}, fmt.Errorf("end_column must be >= 1, got %d", endCol)
	}

	// start must not be after end
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		return types.Range{}, fmt.Errorf("start position (%d:%d) must not be after end position (%d:%d)",
			startLine, startCol, endLine, endCol)
	}

	return types.Range{
		Start: types.Position{Line: startLine - 1, Character: startCol - 1},
		End:   types.Position{Line: endLine - 1, Character: endCol - 1},
	}, nil
}

// toInt extracts an integer from args[key]. Handles float64 (JSON default) and int.
func toInt(args map[string]interface{}, key string) (int, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("missing required argument %q", key)
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("argument %q must be a number, got %T", key, v)
	}
}
