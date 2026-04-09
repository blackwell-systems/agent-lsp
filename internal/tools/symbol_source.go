package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// SymbolSourceResult is the JSON-serializable response returned by HandleGetSymbolSource.
// All line numbers are 1-based (matching MCP convention used by all other tools).
type SymbolSourceResult struct {
	SymbolName string `json:"symbol_name"`
	SymbolKind string `json:"symbol_kind"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Source     string `json:"source"`
}

// containsPosition reports whether the 0-based LSP Range r contains the given
// 0-based (line, character) position. Both start and end are inclusive.
func containsPosition(r types.Range, line, character int) bool {
	// Check lower bound: position must be at or after range start.
	afterStart := line > r.Start.Line || (line == r.Start.Line && character >= r.Start.Character)
	// Check upper bound: position must be at or before range end.
	beforeEnd := line < r.End.Line || (line == r.End.Line && character <= r.End.Character)
	return afterStart && beforeEnd
}

// findInnermostSymbol recursively walks the DocumentSymbol tree and returns
// the deepest symbol whose Range contains the given 0-based (line, character)
// position. Returns nil if no symbol contains the position.
func findInnermostSymbol(symbols []types.DocumentSymbol, line, character int) *types.DocumentSymbol {
	for _, s := range symbols {
		if containsPosition(s.Range, line, character) {
			sym := s // copy, not loop variable reference
			if child := findInnermostSymbol(sym.Children, line, character); child != nil {
				return child
			}
			return &sym
		}
	}
	return nil
}

// HandleGetSymbolSource resolves a file path, reads document symbols via LSP,
// finds the innermost symbol containing the cursor (1-based input), slices file
// content to that symbol's range, and returns structured JSON.
func HandleGetSymbolSource(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}
	cleanPath, err := ValidateFilePath(filePath, client.RootDir())
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// ExtractPositionWithPattern reads args["column"], but this tool's MCP
	// parameter is named "character". Alias before calling.
	if ch, ok := args["character"]; ok {
		if args["column"] == nil {
			args["column"] = ch
		}
	}
	if args["column"] == nil {
		args["column"] = float64(1)
	}
	line1, col1, posErr := ExtractPositionWithPattern(args, cleanPath)
	if posErr != nil {
		return types.ErrorResult(fmt.Sprintf("position: %s", posErr)), nil
	}

	// Convert from 1-based (MCP convention) to 0-based (LSP convention).
	line0 := line1 - 1
	char0 := col1 - 1

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	symbols, wErr := WithDocument[[]types.DocumentSymbol](ctx, client, cleanPath, languageID, func(fileURI string) ([]types.DocumentSymbol, error) {
		return client.GetDocumentSymbols(ctx, fileURI)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_symbol_source: %s", wErr)), nil
	}

	sym := findInnermostSymbol(symbols, line0, char0)
	if sym == nil {
		return types.ErrorResult("no symbol found at the given position"), nil
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("reading file: %s", err)), nil
	}
	lines := strings.Split(string(content), "\n")
	start := sym.Range.Start.Line // 0-based
	end := sym.Range.End.Line    // 0-based, inclusive
	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}
	source := strings.Join(lines[start:end+1], "\n")

	result := SymbolSourceResult{
		SymbolName: sym.Name,
		SymbolKind: symbolKindName(int(sym.Kind)),
		StartLine:  sym.Range.Start.Line + 1,
		EndLine:    sym.Range.End.Line + 1,
		Source:     source,
	}
	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}
