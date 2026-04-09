package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// typeHierarchyResult is the JSON shape returned by HandleTypeHierarchy.
type typeHierarchyResult struct {
	Items      []types.TypeHierarchyItem `json:"items"`
	Supertypes []types.TypeHierarchyItem `json:"supertypes,omitempty"`
	Subtypes   []types.TypeHierarchyItem `json:"subtypes,omitempty"`
}

// HandleTypeHierarchy resolves type hierarchy for the symbol at the given position.
// The direction argument controls which relationships are returned:
//   - "supertypes" -- parent classes and interfaces
//   - "subtypes"   -- subclasses and implementations
//   - "both"       -- both supertypes and subtypes (default when omitted or empty)
func HandleTypeHierarchy(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	direction := "both"
	if d, ok := args["direction"].(string); ok && d != "" {
		direction = strings.ToLower(d)
	}
	switch direction {
	case "supertypes", "subtypes", "both":
		// valid
	default:
		return types.ErrorResult(fmt.Sprintf("invalid direction %q; must be \"supertypes\", \"subtypes\", or \"both\"", direction)), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	items, wErr := WithDocument[[]types.TypeHierarchyItem](ctx, client, filePath, languageID, func(fileURI string) ([]types.TypeHierarchyItem, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.PrepareTypeHierarchy(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("type_hierarchy (prepare): %s", wErr)), nil
	}

	if len(items) == 0 {
		return types.TextResult(fmt.Sprintf("No type hierarchy item found at %s:%d:%d", filePath, line, col)), nil
	}

	result := typeHierarchyResult{Items: items}

	for _, item := range items {
		if direction == "supertypes" || direction == "both" {
			supers, superErr := client.GetSupertypes(ctx, item)
			if superErr != nil {
				return types.ErrorResult(fmt.Sprintf("type_hierarchy (supertypes): %s", superErr)), nil
			}
			result.Supertypes = append(result.Supertypes, supers...)
		}
		if direction == "subtypes" || direction == "both" {
			subs, subErr := client.GetSubtypes(ctx, item)
			if subErr != nil {
				return types.ErrorResult(fmt.Sprintf("type_hierarchy (subtypes): %s", subErr)), nil
			}
			result.Subtypes = append(result.Subtypes, subs...)
		}
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling type hierarchy result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}
