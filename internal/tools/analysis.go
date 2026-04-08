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
		if err := lsp.WaitForDiagnostics(ctx, client, []string{fileURI}, 25000); err != nil {
			return types.ErrorResult(fmt.Sprintf("waiting for diagnostics: %s", err)), nil
		}
		diags := client.GetDiagnostics(fileURI)
		diagMap = map[string][]types.LSPDiagnostic{fileURI: diags}
	} else {
		if err := client.ReopenAllDocuments(ctx); err != nil {
			return types.ErrorResult(fmt.Sprintf("failed to reopen documents: %s", err)), nil
		}
		openURIs := client.GetOpenDocuments()
		if err := lsp.WaitForDiagnostics(ctx, client, openURIs, 25000); err != nil {
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

	result, wErr := WithDocument[types.CompletionList](ctx, client, filePath, languageID, func(fileURI string) (types.CompletionList, error) {
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

	result, wErr := WithDocument[[]types.CodeAction](ctx, client, filePath, languageID, func(fileURI string) ([]types.CodeAction, error) {
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

	result, wErr := WithDocument[[]types.DocumentSymbol](ctx, client, filePath, languageID, func(fileURI string) ([]types.DocumentSymbol, error) {
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

// workspaceSymbolEnriched is a SymbolInformation with an optional hover field.
type workspaceSymbolEnriched struct {
	types.SymbolInformation
	Hover string `json:"hover,omitempty"`
}

// workspaceSymbolsResponse is the structured response for get_workspace_symbols.
// symbols contains all matches (name/kind/location only). enriched contains
// the hover-enriched window defined by offset and limit. pagination describes
// the current window position.
type workspaceSymbolsResponse struct {
	Total      int                       `json:"total"`
	Symbols    []types.SymbolInformation `json:"symbols"`
	Enriched   []workspaceSymbolEnriched `json:"enriched,omitempty"`
	Pagination *workspaceSymbolPagination `json:"pagination,omitempty"`
}

type workspaceSymbolPagination struct {
	Offset int  `json:"offset"`
	Limit  int  `json:"limit"`
	More   bool `json:"more"`
}

// HandleGetWorkspaceSymbols searches for symbols across the workspace.
//
// detail_level controls enrichment:
//   - "basic" (or empty): returns all matching symbols with name/kind/location only.
//   - "hover" (default when limit/offset used): returns all symbols in symbols[],
//     plus hover-enriched results for the offset..offset+limit window in enriched[].
//
// limit (default 3) and offset (default 0) control the enrichment window.
// The AI can paginate: read symbols[] to see all results, use offset to step
// through enriched detail windows without re-running the workspace search.
func HandleGetWorkspaceSymbols(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	query, _ := args["query"].(string)
	detailLevel, _ := args["detail_level"].(string)
	limit := 3
	if v, ok := toIntOpt(args, "limit"); ok && v > 0 {
		limit = v
	}
	offset := 0
	if v, ok := toIntOpt(args, "offset"); ok && v >= 0 {
		offset = v
	}

	symbols, err := client.GetWorkspaceSymbols(ctx, query)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("get_workspace_symbols: %s", err)), nil
	}

	if detailLevel == "basic" || detailLevel == "" {
		data, mErr := json.Marshal(symbols)
		if mErr != nil {
			return types.ErrorResult(fmt.Sprintf("marshaling workspace symbols: %s", mErr)), nil
		}
		return types.TextResult(string(data)), nil
	}

	// Enrich the offset..offset+limit window with hover info.
	resp := workspaceSymbolsResponse{
		Total:   len(symbols),
		Symbols: symbols,
	}

	if start, end, pg := symbolPaginationWindow(len(symbols), offset, limit); pg != nil {
		resp.Pagination = pg
		window := symbols[start:end]
		enriched := make([]workspaceSymbolEnriched, len(window))
		for i, sym := range window {
			enriched[i] = workspaceSymbolEnriched{SymbolInformation: sym}
			filePath, pErr := URIToFilePath(sym.Location.URI)
			if pErr == nil {
				pos := types.Position{
					Line:      sym.Location.Range.Start.Line + 1,
					Character: sym.Location.Range.Start.Character + 1,
				}
				hoverText, hErr := WithDocument[string](ctx, client, filePath, "", func(uri string) (string, error) {
					return client.GetInfoOnLocation(ctx, uri, pos)
				})
				if hErr == nil && hoverText != "" {
					enriched[i].Hover = hoverText
				}
			}
		}
		resp.Enriched = enriched
	}

	data, mErr := json.Marshal(resp)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling workspace symbols: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// toIntOpt reads an integer argument without error — returns (value, true) if present and valid.
func toIntOpt(args map[string]interface{}, key string) (int, bool) {
	v, err := toInt(args, key)
	return v, err == nil
}

// symbolPaginationWindow computes the enrichment window [start, end) and pagination
// metadata for a result set of size total. Returns nil pagination when offset is
// out of bounds. Extracted for testing.
func symbolPaginationWindow(total, offset, limit int) (start, end int, p *workspaceSymbolPagination) {
	if offset >= total || total == 0 {
		return 0, 0, nil
	}
	end = offset + limit
	if end > total {
		end = total
	}
	return offset, end, &workspaceSymbolPagination{
		Offset: offset,
		Limit:  limit,
		More:   end < total,
	}
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
