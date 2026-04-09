package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// HandleGoToSymbol resolves a dot-notation symbol path to its definition location
// without requiring a file_path or line/column. It uses workspace symbol search
// to locate candidates and then calls GetDefinition for precision.
//
// args["symbol_path"]: dot-notation string e.g. "MyClass.method", "pkg.Function"
// args["workspace_root"]: optional scope (unused in lookup, reserved for future filtering)
// args["language"]: optional filter (reserved for future filtering)
func HandleGoToSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	symbolPath, _ := args["symbol_path"].(string)
	if symbolPath == "" {
		return types.ErrorResult("symbol_path is required"), nil
	}

	// Extract leaf name: last component after splitting on "."
	parts := strings.Split(symbolPath, ".")
	leafName := parts[len(parts)-1]

	syms, err := client.GetWorkspaceSymbols(ctx, leafName)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("get_workspace_symbols: %s", err)), nil
	}

	if len(syms) == 0 {
		return types.TextResult(fmt.Sprintf("no symbols found for symbol_path: %s", symbolPath)), nil
	}

	best := bestSymbolMatch(syms, symbolPath)
	if best == nil {
		return types.TextResult(fmt.Sprintf("no symbols found for symbol_path: %s", symbolPath)), nil
	}

	// Convert candidate URI to file path for WithDocument
	filePath, err := URIToFilePath(best.Location.URI)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("converting URI: %s", err)), nil
	}

	// Use 0-indexed position from the symbol location (LSP convention)
	candidatePos := types.Position{
		Line:      best.Location.Range.Start.Line,
		Character: best.Location.Range.Start.Character,
	}

	locs, wErr := WithDocument[[]types.Location](ctx, client, filePath, "", func(fileURI string) ([]types.Location, error) {
		return client.GetDefinition(ctx, fileURI, candidatePos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_definition: %s", wErr)), nil
	}

	if len(locs) > 0 {
		return locationsResult(locs)
	}

	// Fall back: format the candidate Location directly as a FormattedLocation (1-indexed)
	fp, convErr := URIToFilePath(best.Location.URI)
	if convErr != nil {
		return types.ErrorResult(fmt.Sprintf("converting URI: %s", convErr)), nil
	}
	fallback := []types.FormattedLocation{
		{
			FilePath:  fp,
			StartLine: best.Location.Range.Start.Line + 1,
			StartCol:  best.Location.Range.Start.Character + 1,
			EndLine:   best.Location.Range.End.Line + 1,
			EndCol:    best.Location.Range.End.Character + 1,
		},
	}
	data, mErr := json.Marshal(fallback)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling location: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// bestSymbolMatch picks the best candidate from a list of workspace symbols
// for the given dotted symbol path.
//
// If symbolPath has no ".": returns &candidates[0] (first match).
// If symbolPath has ".": prefers a candidate where ContainerName matches the
// parent component (everything before the last dot), case-sensitive.
// Falls back to &candidates[0] if no ContainerName match is found.
func bestSymbolMatch(candidates []types.SymbolInformation, symbolPath string) *types.SymbolInformation {
	if len(candidates) == 0 {
		return nil
	}

	if !strings.Contains(symbolPath, ".") {
		return &candidates[0]
	}

	lastDot := strings.LastIndex(symbolPath, ".")
	parent := symbolPath[:lastDot]

	for i := range candidates {
		if candidates[i].ContainerName != nil && *candidates[i].ContainerName == parent {
			return &candidates[i]
		}
	}

	return &candidates[0]
}
