package tools

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// SymbolLocation holds the resolved position of a symbol.
type SymbolLocation struct {
	FilePath       string      // absolute file path
	StartLine      int         // 0-indexed start line
	StartCol       int         // 0-indexed start column
	EndLine        int         // 0-indexed end line
	EndCol         int         // 0-indexed end column
	Range          types.Range // full symbol range (for body replacement)
	SelectionRange types.Range // name/signature range
}

// overloadPattern matches a trailing "[N]" index suffix on a name segment.
var overloadPattern = regexp.MustCompile(`^(.+)\[(\d+)\]$`)

// ResolveSymbolByNamePath resolves a symbol name path to its location.
// If filePath is empty, workspace symbols are searched first to find the file.
// If filePath is provided, document symbols are used directly.
//
// Name path resolution:
//   - "Foo" matches top-level symbol named Foo
//   - "Foo.Bar" matches child Bar under parent Foo
//   - "Foo.Bar.Baz" matches arbitrarily nested paths
//   - "Bar[0]", "Bar[1]" disambiguates when multiple symbols share a name
//
// Returns error if symbol not found, index out of range, or LSP calls fail.
func ResolveSymbolByNamePath(ctx context.Context, client *lsp.LSPClient, filePath string, namePath string) (*SymbolLocation, error) {
	if client == nil {
		return nil, fmt.Errorf("LSP client not initialized; call start_lsp first")
	}
	if namePath == "" {
		return nil, fmt.Errorf("symbol name path is required")
	}

	// If no filePath, resolve via workspace symbols.
	if filePath == "" {
		// Extract leaf name (last segment, strip any overload index).
		parts := strings.Split(namePath, ".")
		leafName := parts[len(parts)-1]
		if m := overloadPattern.FindStringSubmatch(leafName); m != nil {
			leafName = m[1]
		}

		syms, err := client.GetWorkspaceSymbols(ctx, leafName)
		if err != nil {
			return nil, fmt.Errorf("workspace symbol search: %w", err)
		}
		if len(syms) == 0 {
			return nil, fmt.Errorf("no workspace symbols found for %q", namePath)
		}

		best := bestSymbolMatch(syms, namePath)
		if best == nil {
			return nil, fmt.Errorf("no matching symbol for %q", namePath)
		}

		resolved, err := URIToFilePath(best.Location.URI)
		if err != nil {
			return nil, fmt.Errorf("converting symbol URI: %w", err)
		}
		filePath = resolved
	}

	// Use document symbols for precise range resolution.
	docSyms, wErr := WithDocument[[]types.DocumentSymbol](ctx, client, filePath, "", func(fileURI string) ([]types.DocumentSymbol, error) {
		return client.GetDocumentSymbols(ctx, fileURI)
	})
	if wErr != nil {
		return nil, fmt.Errorf("getting document symbols: %w", wErr)
	}

	matched, err := resolveInDocumentSymbols(docSyms, namePath)
	if err != nil {
		return nil, err
	}

	return &SymbolLocation{
		FilePath:       filePath,
		StartLine:      matched.Range.Start.Line,
		StartCol:       matched.Range.Start.Character,
		EndLine:        matched.Range.End.Line,
		EndCol:         matched.Range.End.Character,
		Range:          matched.Range,
		SelectionRange: matched.SelectionRange,
	}, nil
}

// resolveInDocumentSymbols walks the document symbol tree to find the
// symbol matching namePath. Returns the matched DocumentSymbol or error.
func resolveInDocumentSymbols(symbols []types.DocumentSymbol, namePath string) (*types.DocumentSymbol, error) {
	segments := strings.Split(namePath, ".")
	if len(segments) == 0 || (len(segments) == 1 && segments[0] == "") {
		return nil, fmt.Errorf("empty name path")
	}

	current := symbols
	var result *types.DocumentSymbol

	for _, seg := range segments {
		name := seg
		index := -1 // -1 means "take first match"

		if m := overloadPattern.FindStringSubmatch(seg); m != nil {
			name = m[1]
			idx, err := strconv.Atoi(m[2])
			if err != nil {
				return nil, fmt.Errorf("invalid overload index in %q: %w", seg, err)
			}
			index = idx
		}

		// Collect all matches at this level.
		var matches []*types.DocumentSymbol
		for i := range current {
			if current[i].Name == name {
				matches = append(matches, &current[i])
			}
		}

		if len(matches) == 0 {
			return nil, fmt.Errorf("symbol %q not found at current level", name)
		}

		var picked *types.DocumentSymbol
		if index < 0 {
			picked = matches[0]
		} else {
			if index >= len(matches) {
				return nil, fmt.Errorf("overload index [%d] out of bounds for %q (found %d matches)", index, name, len(matches))
			}
			picked = matches[index]
		}

		result = picked
		current = picked.Children
	}

	return result, nil
}

// HandleReplaceSymbolBody replaces the body of a named symbol, preserving the
// declaration/signature line. The body is considered to start at the line after
// SelectionRange.End.Line.
func HandleReplaceSymbolBody(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, _ := args["file_path"].(string)
	symbolPath, _ := args["symbol_path"].(string)
	newBody, _ := args["new_body"].(string)

	if symbolPath == "" {
		return types.ErrorResult("symbol_path is required"), nil
	}
	if newBody == "" {
		return types.ErrorResult("new_body is required"), nil
	}

	loc, err := ResolveSymbolByNamePath(ctx, client, filePath, symbolPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("resolve symbol: %s. Use list_symbols to see available symbols in the file.", err)), nil
	}

	// Body starts at line after SelectionRange end.
	bodyStartLine := loc.SelectionRange.End.Line + 1
	bodyStartPos := types.Position{Line: bodyStartLine, Character: 0}
	bodyEndPos := loc.Range.End

	fileURI := CreateFileURI(loc.FilePath)
	edit := map[string]any{
		"changes": map[string]any{
			fileURI: []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": bodyStartPos.Line, "character": bodyStartPos.Character},
						"end":   map[string]any{"line": bodyEndPos.Line, "character": bodyEndPos.Character},
					},
					"newText": newBody,
				},
			},
		},
	}

	if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
		return types.ErrorResult(fmt.Sprintf("apply edit: %s", err)), nil
	}

	hint := "Use get_diagnostics to verify the edit didn't introduce errors."
	return appendHint(types.TextResult(fmt.Sprintf("Replaced body of %q in %s", symbolPath, loc.FilePath)), hint), nil
}

// HandleInsertAfterSymbol inserts code immediately after a named symbol definition.
func HandleInsertAfterSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, _ := args["file_path"].(string)
	symbolPath, _ := args["symbol_path"].(string)
	code, _ := args["code"].(string)

	if symbolPath == "" {
		return types.ErrorResult("symbol_path is required"), nil
	}
	if code == "" {
		return types.ErrorResult("code is required"), nil
	}

	loc, err := ResolveSymbolByNamePath(ctx, client, filePath, symbolPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("resolve symbol: %s. Use list_symbols to see available symbols in the file.", err)), nil
	}

	// Insert at the end of the symbol range.
	insertPos := loc.Range.End
	fileURI := CreateFileURI(loc.FilePath)
	edit := map[string]any{
		"changes": map[string]any{
			fileURI: []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": insertPos.Line, "character": insertPos.Character},
						"end":   map[string]any{"line": insertPos.Line, "character": insertPos.Character},
					},
					"newText": "\n" + code,
				},
			},
		},
	}

	if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
		return types.ErrorResult(fmt.Sprintf("apply edit: %s", err)), nil
	}

	hint := "Use get_diagnostics to verify the insertion didn't introduce errors."
	return appendHint(types.TextResult(fmt.Sprintf("Inserted code after %q in %s", symbolPath, loc.FilePath)), hint), nil
}

// HandleInsertBeforeSymbol inserts code immediately before a named symbol definition.
func HandleInsertBeforeSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, _ := args["file_path"].(string)
	symbolPath, _ := args["symbol_path"].(string)
	code, _ := args["code"].(string)

	if symbolPath == "" {
		return types.ErrorResult("symbol_path is required"), nil
	}
	if code == "" {
		return types.ErrorResult("code is required"), nil
	}

	loc, err := ResolveSymbolByNamePath(ctx, client, filePath, symbolPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("resolve symbol: %s. Use list_symbols to see available symbols in the file.", err)), nil
	}

	// Insert at the start of the symbol range (column 0 of its start line).
	insertPos := types.Position{Line: loc.Range.Start.Line, Character: 0}
	fileURI := CreateFileURI(loc.FilePath)
	edit := map[string]any{
		"changes": map[string]any{
			fileURI: []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": insertPos.Line, "character": insertPos.Character},
						"end":   map[string]any{"line": insertPos.Line, "character": insertPos.Character},
					},
					"newText": code + "\n",
				},
			},
		},
	}

	if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
		return types.ErrorResult(fmt.Sprintf("apply edit: %s", err)), nil
	}

	hint := "Use get_diagnostics to verify the insertion didn't introduce errors."
	return appendHint(types.TextResult(fmt.Sprintf("Inserted code before %q in %s", symbolPath, loc.FilePath)), hint), nil
}

// HandleSafeDeleteSymbol deletes a symbol only if it has zero references.
// Returns an error if the symbol has any references, preventing accidental breakage.
func HandleSafeDeleteSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, _ := args["file_path"].(string)
	symbolPath, _ := args["symbol_path"].(string)

	if symbolPath == "" {
		return types.ErrorResult("symbol_path is required"), nil
	}
	if filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	loc, err := ResolveSymbolByNamePath(ctx, client, filePath, symbolPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("resolve symbol: %s. Use list_symbols to see available symbols in the file.", err)), nil
	}

	// Check references (excluding the declaration itself).
	fileURI := CreateFileURI(loc.FilePath)
	refs, err := client.GetReferences(ctx, fileURI, loc.SelectionRange.Start, false)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("get references: %s", err)), nil
	}

	if len(refs) > 0 {
		return types.ErrorResult(fmt.Sprintf("symbol has %d references; cannot safely delete. Use find_references to see callers, update them first, then retry safe_delete_symbol.", len(refs))), nil
	}

	// Delete the entire symbol range.
	edit := map[string]any{
		"changes": map[string]any{
			fileURI: []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": loc.Range.Start.Line, "character": loc.Range.Start.Character},
						"end":   map[string]any{"line": loc.Range.End.Line, "character": loc.Range.End.Character},
					},
					"newText": "",
				},
			},
		},
	}

	if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
		return types.ErrorResult(fmt.Sprintf("apply edit: %s", err)), nil
	}

	return types.TextResult(fmt.Sprintf("Deleted symbol %q from %s", symbolPath, loc.FilePath)), nil
}
