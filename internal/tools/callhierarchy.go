package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// callHierarchyResult is the JSON shape returned by HandleCallHierarchy.
type callHierarchyResult struct {
	Items    []types.CallHierarchyItem         `json:"items"`
	Incoming []types.CallHierarchyIncomingCall `json:"incoming,omitempty"`
	Outgoing []types.CallHierarchyOutgoingCall `json:"outgoing,omitempty"`
}

// HandleCallHierarchy resolves call hierarchy for the symbol at the given position.
// The direction argument controls which calls are returned:
//   - "incoming" -- callers of the function
//   - "outgoing" -- callees of the function
//   - "both"     -- both callers and callees (default when omitted or empty)
func HandleCallHierarchy(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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
	case "incoming", "outgoing", "both":
		// valid
	default:
		return types.ErrorResult(fmt.Sprintf("invalid direction %q; must be \"incoming\", \"outgoing\", or \"both\"", direction)), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	items, wErr := WithDocument[[]types.CallHierarchyItem](ctx, client, filePath, languageID, func(fileURI string) ([]types.CallHierarchyItem, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.PrepareCallHierarchy(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("call_hierarchy (prepare): %s", wErr)), nil
	}

	if len(items) == 0 {
		return types.TextResult(fmt.Sprintf("No call hierarchy item found at %s:%d:%d", filePath, line, col)), nil
	}

	result := callHierarchyResult{Items: items}

	for _, item := range items {
		if direction == "incoming" || direction == "both" {
			calls, callErr := client.GetIncomingCalls(ctx, item)
			if callErr != nil {
				return types.ErrorResult(fmt.Sprintf("call_hierarchy (incoming): %s", callErr)), nil
			}
			result.Incoming = append(result.Incoming, calls...)
		}
		if direction == "outgoing" || direction == "both" {
			calls, callErr := client.GetOutgoingCalls(ctx, item)
			if callErr != nil {
				return types.ErrorResult(fmt.Sprintf("call_hierarchy (outgoing): %s", callErr)), nil
			}
			result.Outgoing = append(result.Outgoing, calls...)
		}
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling call hierarchy result: %s", mErr)), nil
	}
	return appendHint(types.TextResult(string(data)), "Use get_change_impact for a full blast-radius analysis."), nil
}
