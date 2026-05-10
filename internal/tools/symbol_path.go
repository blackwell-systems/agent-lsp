package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleGoToSymbol resolves a dot-notation symbol path to its definition location
// without requiring a file_path or line/column. It uses workspace symbol search
// to locate candidates and then calls GetDefinition for precision.
//
// args["symbol_path"]: dot-notation string e.g. "MyClass.method", "pkg.Function"
// args["workspace_root"]: optional scope (unused in lookup, reserved for future filtering)
// args["language"]: optional filter (reserved for future filtering)
func HandleGoToSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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
		return types.ErrorResult(fmt.Sprintf("find_symbol: %s", err)), nil
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

	goToSymHint := "Use inspect_symbol for type info, or find_references for all usages."
	if len(locs) > 0 {
		res, err := locationsResult(locs)
		if err != nil {
			return res, err
		}
		return appendHint(res, goToSymHint), nil
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
	return appendHint(types.TextResult(string(data)), goToSymHint), nil
}

// bestSymbolMatch picks the best candidate from a list of workspace symbols
// for the given dotted symbol path.
//
// For dotted paths (e.g. "MyClass.method"), priority is:
//  1. Exact name + ContainerName match in a non-test file.
//  2. Exact name + ContainerName match in any file.
//  3. ContainerName match in a non-test file.
//  4. ContainerName match in any file.
//  5. Exact name in a non-test file.
//  6. Exact name in any file.
//  7. First candidate.
//
// For non-dotted paths (e.g. "HandleGoToSymbol"), priority is:
//  1. Exact name in a non-test file.
//  2. Exact name in any file.
//  3. First candidate.
func bestSymbolMatch(candidates []types.SymbolInformation, symbolPath string) *types.SymbolInformation {
	if len(candidates) == 0 {
		return nil
	}

	leaf := symbolPath
	var parent string
	dotted := strings.Contains(symbolPath, ".")
	if dotted {
		lastDot := strings.LastIndex(symbolPath, ".")
		leaf = symbolPath[lastDot+1:]
		parent = symbolPath[:lastDot]
	}

	isTest := func(uri string) bool { return strings.HasSuffix(uri, "_test.go") }
	nameMatch := func(s types.SymbolInformation) bool { return s.Name == leaf }
	containerMatch := func(s types.SymbolInformation) bool {
		return s.ContainerName != nil && *s.ContainerName == parent
	}

	if dotted {
		// 1. Exact name + container, non-test.
		for i := range candidates {
			if nameMatch(candidates[i]) && containerMatch(candidates[i]) && !isTest(candidates[i].Location.URI) {
				return &candidates[i]
			}
		}
		// 2. Exact name + container, any file.
		for i := range candidates {
			if nameMatch(candidates[i]) && containerMatch(candidates[i]) {
				return &candidates[i]
			}
		}
		// 3. Container only, non-test.
		for i := range candidates {
			if containerMatch(candidates[i]) && !isTest(candidates[i].Location.URI) {
				return &candidates[i]
			}
		}
		// 4. Container only, any file.
		for i := range candidates {
			if containerMatch(candidates[i]) {
				return &candidates[i]
			}
		}
	}

	// 5 / 1. Exact name, non-test.
	for i := range candidates {
		if nameMatch(candidates[i]) && !isTest(candidates[i].Location.URI) {
			return &candidates[i]
		}
	}
	// 6 / 2. Exact name, any file.
	for i := range candidates {
		if nameMatch(candidates[i]) {
			return &candidates[i]
		}
	}

	return &candidates[0]
}
