package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// formatLocations converts a slice of LSP Location values to FormattedLocation,
// converting URIs to file paths and converting to 1-indexed positions.
func formatLocations(locs []types.Location) ([]types.FormattedLocation, error) {
	result := make([]types.FormattedLocation, 0, len(locs))
	for _, loc := range locs {
		fp, err := URIToFilePath(loc.URI)
		if err != nil {
			return nil, fmt.Errorf("converting URI %s: %w", loc.URI, err)
		}
		result = append(result, types.FormattedLocation{
			FilePath:  fp,
			StartLine: loc.Range.Start.Line + 1,
			StartCol:  loc.Range.Start.Character + 1,
			EndLine:   loc.Range.End.Line + 1,
			EndCol:    loc.Range.End.Character + 1,
		})
	}
	return result, nil
}

// locationsResult marshals formatted locations into a ToolResult.
func locationsResult(locs []types.Location) (types.ToolResult, error) {
	formatted, err := formatLocations(locs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("formatting locations: %s", err)), nil
	}
	data, mErr := json.Marshal(formatted)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling locations: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleGetReferences retrieves all references to the symbol at the given location.
func HandleGetReferences(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	includeDecl := false
	if v, ok := args["include_declaration"].(bool); ok {
		includeDecl = v
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	fileURI := CreateFileURI(filePath)
	locs, wErr := WithDocument[[]types.Location](ctx, client, filePath, languageID, func(fURI string) ([]types.Location, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetReferences(ctx, fURI, pos, includeDecl)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_references: %s", wErr)), nil
	}
	if len(locs) == 0 {
		locs, wErr = fuzzyPositionFallback(ctx, client, fileURI, line, col, func(pos types.Position) ([]types.Location, error) {
			return client.GetReferences(ctx, fileURI, pos, includeDecl)
		})
		if wErr != nil {
			return types.ErrorResult(fmt.Sprintf("get_references (fuzzy): %s", wErr)), nil
		}
	}
	return locationsResult(locs)
}

// HandleGoToDefinition finds the definition of the symbol at the given location.
func HandleGoToDefinition(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	fileURI := CreateFileURI(filePath)
	locs, wErr := WithDocument[[]types.Location](ctx, client, filePath, languageID, func(fURI string) ([]types.Location, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetDefinition(ctx, fURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("go_to_definition: %s", wErr)), nil
	}
	if len(locs) == 0 {
		locs, wErr = fuzzyPositionFallback(ctx, client, fileURI, line, col, func(pos types.Position) ([]types.Location, error) {
			return client.GetDefinition(ctx, fileURI, pos)
		})
		if wErr != nil {
			return types.ErrorResult(fmt.Sprintf("go_to_definition (fuzzy): %s", wErr)), nil
		}
	}
	return locationsResult(locs)
}

// HandleGoToTypeDefinition finds the type definition of the symbol at the given location.
func HandleGoToTypeDefinition(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	locs, wErr := WithDocument[[]types.Location](ctx, client, filePath, languageID, func(fileURI string) ([]types.Location, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetTypeDefinition(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("go_to_type_definition: %s", wErr)), nil
	}
	return locationsResult(locs)
}

// HandleGoToImplementation finds implementations of the symbol at the given location.
func HandleGoToImplementation(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	locs, wErr := WithDocument[[]types.Location](ctx, client, filePath, languageID, func(fileURI string) ([]types.Location, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetImplementation(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("go_to_implementation: %s", wErr)), nil
	}
	return locationsResult(locs)
}

// HandleGoToDeclaration finds the declaration of the symbol at the given location.
func HandleGoToDeclaration(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	locs, wErr := WithDocument[[]types.Location](ctx, client, filePath, languageID, func(fileURI string) ([]types.Location, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetDeclaration(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("go_to_declaration: %s", wErr)), nil
	}
	return locationsResult(locs)
}
