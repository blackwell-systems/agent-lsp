// analysis.go implements MCP tool handlers for code analysis queries:
// get_diagnostics, inspect_symbol (hover), get_completions,
// get_signature_help, suggest_fixes, list_symbols, and
// find_symbol.
//
// get_diagnostics has special behavior: it reopens the document from disk
// before collecting diagnostics, ensuring results reflect the latest saved
// state rather than stale LSP cache. It waits up to 25 seconds for
// diagnostics to settle (cross-package analysis in Go can be slow).
//
// suggest_fixes filters the returned actions to a concise summary:
// title, kind, and whether a command or workspace edit is attached.
// Full workspace edits are not inlined to keep responses compact.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleGetDiagnostics retrieves LSP diagnostics for a file or all open documents.
func HandleGetDiagnostics(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, _ := args["file_path"].(string)

	var diagMap map[string][]types.LSPDiagnostic

	if filePath != "" {
		cleanPath, err := ValidateFilePath(filePath, client.RootDir())
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("invalid file path: %s", err)), nil
		}
		fileURI := CreateFileURI(cleanPath)
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

	// group_by=symbol: group diagnostics under their owning symbol.
	groupBy, _ := args["group_by"].(string)
	if groupBy == "symbol" && filePath != "" {
		result, gErr := groupDiagnosticsBySymbol(ctx, client, filePath, diagMap)
		if gErr == nil {
			data, err := json.Marshal(result)
			if err != nil {
				return types.ErrorResult(fmt.Sprintf("marshaling diagnostics: %s", err)), nil
			}
			hint := "No errors. Safe to proceed."
			if len(result.Symbols) > 0 || len(result.Ungrouped) > 0 {
				hint = "Use suggest_fixes at each error location for quick fixes."
			}
			return appendHint(types.TextResult(string(data)), hint), nil
		}
		// Fall through to ungrouped if symbol grouping fails.
	}

	data, err := json.Marshal(diagMap)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling diagnostics: %s", err)), nil
	}

	hasErrors := false
	for _, diags := range diagMap {
		if len(diags) > 0 {
			hasErrors = true
			break
		}
	}
	hint := "No errors. Safe to proceed."
	if hasErrors {
		hint = "Use suggest_fixes at each error location for quick fixes."
	}
	return appendHint(types.TextResult(string(data)), hint), nil
}

// symbolDiagGroup groups diagnostics under a named symbol.
type symbolDiagGroup struct {
	Name        string               `json:"name"`
	Kind        string               `json:"kind"`
	Line        int                  `json:"line"`
	Diagnostics []types.LSPDiagnostic `json:"diagnostics"`
}

// groupedDiagnostics is the response format for group_by=symbol.
type groupedDiagnostics struct {
	Symbols   []symbolDiagGroup     `json:"symbols"`
	Ungrouped []types.LSPDiagnostic `json:"ungrouped"`
}

// groupDiagnosticsBySymbol assigns each diagnostic to its owning symbol
// based on range containment. Diagnostics outside any symbol range go
// into the ungrouped list.
func groupDiagnosticsBySymbol(ctx context.Context, client *lsp.LSPClient, filePath string, diagMap map[string][]types.LSPDiagnostic) (*groupedDiagnostics, error) {
	fileURI := CreateFileURI(filePath)
	symbols, err := client.GetDocumentSymbols(ctx, fileURI)
	if err != nil {
		return nil, err
	}

	diags := diagMap[fileURI]
	if len(diags) == 0 {
		return &groupedDiagnostics{}, nil
	}

	// Build a flat list of symbols with their ranges for containment checks.
	type flatSymbol struct {
		name      string
		kind      string
		startLine int
		endLine   int
	}
	var flat []flatSymbol
	var flatten func(syms []types.DocumentSymbol, prefix string)
	flatten = func(syms []types.DocumentSymbol, prefix string) {
		for _, s := range syms {
			name := s.Name
			if prefix != "" {
				name = prefix + "." + name
			}
			flat = append(flat, flatSymbol{
				name:      name,
				kind:      symbolKindName(int(s.Kind)),
				startLine: s.Range.Start.Line,
				endLine:   s.Range.End.Line,
			})
			if len(s.Children) > 0 {
				flatten(s.Children, name)
			}
		}
	}
	flatten(symbols, "")

	// Assign each diagnostic to the innermost containing symbol.
	grouped := make(map[string]*symbolDiagGroup)
	var ungrouped []types.LSPDiagnostic

	for _, d := range diags {
		line := d.Range.Start.Line
		var bestMatch *flatSymbol
		for i := range flat {
			s := &flat[i]
			if line >= s.startLine && line <= s.endLine {
				if bestMatch == nil || (s.endLine-s.startLine) < (bestMatch.endLine-bestMatch.startLine) {
					bestMatch = s
				}
			}
		}
		if bestMatch != nil {
			g, ok := grouped[bestMatch.name]
			if !ok {
				g = &symbolDiagGroup{
					Name: bestMatch.name,
					Kind: bestMatch.kind,
					Line: bestMatch.startLine + 1,
				}
				grouped[bestMatch.name] = g
			}
			g.Diagnostics = append(g.Diagnostics, d)
		} else {
			ungrouped = append(ungrouped, d)
		}
	}

	result := &groupedDiagnostics{Ungrouped: ungrouped}
	for _, g := range grouped {
		result.Symbols = append(result.Symbols, *g)
	}
	return result, nil
}

// HandleGetInfoOnLocation retrieves hover information at a source location.
func HandleGetInfoOnLocation(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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
		return types.ErrorResult(fmt.Sprintf("inspect_symbol: %s", wErr)), nil
	}
	return appendHint(types.TextResult(result), "Use find_references to find all usages of this symbol."), nil
}

// HandleGetCompletions retrieves completion suggestions at a source location.
func HandleGetCompletions(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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
func HandleGetSignatureHelp(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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

	result, wErr := WithDocument[any](ctx, client, filePath, languageID, func(fileURI string) (any, error) {
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
func HandleGetCodeActions(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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
		return types.ErrorResult(fmt.Sprintf("suggest_fixes: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling code actions: %s", mErr)), nil
	}
	return appendHint(types.TextResult(string(data)), "Use execute_command to apply a code action."), nil
}

// HandleGetDocumentSymbols retrieves the symbols defined in a document.
func HandleGetDocumentSymbols(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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

	format, _ := args["format"].(string)

	result, wErr := WithDocument[[]types.DocumentSymbol](ctx, client, filePath, languageID, func(fileURI string) ([]types.DocumentSymbol, error) {
		return client.GetDocumentSymbols(ctx, fileURI)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("list_symbols: %s", wErr)), nil
	}

	shifted := make([]types.DocumentSymbol, len(result))
	for i, s := range result {
		shifted[i] = shiftDocumentSymbol(s)
	}

	docSymbolHint := "Use get_change_impact with this file to analyze blast radius."
	if format == "outline" {
		return appendHint(types.TextResult(renderOutline(shifted, 0)), docSymbolHint), nil
	}

	data, mErr := json.Marshal(shifted)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling document symbols: %s", mErr)), nil
	}
	return appendHint(types.TextResult(string(data)), docSymbolHint), nil
}

// workspaceSymbolEnriched is a SymbolInformation with an optional hover field.
type workspaceSymbolEnriched struct {
	types.SymbolInformation
	Hover string `json:"hover,omitempty"`
}

// workspaceSymbolsResponse is the structured response for find_symbol.
// symbols contains all matches (name/kind/location only). enriched contains
// the hover-enriched window defined by offset and limit. pagination describes
// the current window position.
type workspaceSymbolsResponse struct {
	Total      int                        `json:"total"`
	Symbols    []types.SymbolInformation  `json:"symbols"`
	Enriched   []workspaceSymbolEnriched  `json:"enriched,omitempty"`
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
func HandleGetWorkspaceSymbols(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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
		return types.ErrorResult(fmt.Sprintf("find_symbol: %s", err)), nil
	}

	wsSymHint := "Use inspect_symbol on a symbol for type details."
	if detailLevel == "basic" || detailLevel == "" {
		data, mErr := json.Marshal(symbols)
		if mErr != nil {
			return types.ErrorResult(fmt.Sprintf("marshaling workspace symbols: %s", mErr)), nil
		}
		return appendHint(types.TextResult(string(data)), wsSymHint), nil
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
	return appendHint(types.TextResult(string(data)), wsSymHint), nil
}

// toIntOpt reads an integer argument without error — returns (value, true) if present and valid.
func toIntOpt(args map[string]any, key string) (int, bool) {
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
func extractPosition(args map[string]any) (line, col int, err error) {
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
func extractRange(args map[string]any) (types.Range, error) {
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

// shiftDocumentSymbol converts all positions in a DocumentSymbol (and its children)
// from 0-based LSP convention to 1-based for MCP tool output.
func shiftDocumentSymbol(s types.DocumentSymbol) types.DocumentSymbol {
	s.Range = shiftRange(s.Range)
	s.SelectionRange = shiftRange(s.SelectionRange)
	for i, c := range s.Children {
		s.Children[i] = shiftDocumentSymbol(c)
	}
	return s
}

func shiftRange(r types.Range) types.Range {
	return types.Range{
		Start: types.Position{Line: r.Start.Line + 1, Character: r.Start.Character + 1},
		End:   types.Position{Line: r.End.Line + 1, Character: r.End.Character + 1},
	}
}

// renderOutline renders a DocumentSymbol tree as compact markdown for LLM consumption.
// Each symbol appears as "name [Kind] :line", indented two spaces per depth level.
// Children are rendered recursively beneath their parent.
func renderOutline(symbols []types.DocumentSymbol, depth int) string {
	var b strings.Builder
	indent := strings.Repeat("  ", depth)
	for _, s := range symbols {
		fmt.Fprintf(&b, "%s%s [%s] :%d\n", indent, s.Name, symbolKindName(int(s.Kind)), s.Range.Start.Line)
		if len(s.Children) > 0 {
			b.WriteString(renderOutline(s.Children, depth+1))
		}
	}
	return b.String()
}

// symbolKindName maps LSP SymbolKind integers to readable names.
func symbolKindName(kind int) string {
	names := map[int]string{
		1: "File", 2: "Module", 3: "Namespace", 4: "Package", 5: "Class",
		6: "Method", 7: "Property", 8: "Field", 9: "Constructor", 10: "Enum",
		11: "Interface", 12: "Function", 13: "Variable", 14: "Constant",
		22: "EnumMember", 23: "Struct", 26: "TypeParameter",
	}
	if n, ok := names[kind]; ok {
		return n
	}
	return fmt.Sprintf("Kind%d", kind)
}

// toInt extracts an integer from args[key]. Handles float64 (JSON default) and int.
func toInt(args map[string]any, key string) (int, error) {
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
