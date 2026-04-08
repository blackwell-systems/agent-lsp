package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// HandleGetDocumentHighlights returns all occurrences of the symbol at a
// position within the same file. Highlights are file-scoped and instant —
// they do not trigger a workspace-wide reference search. Each result includes
// a range and an optional kind: 1=Text, 2=Read, 3=Write.
//
// Use this to find all local usages of a variable, parameter, or field
// without the overhead of get_references. Returns an empty array when the
// server does not support documentHighlightProvider.
func HandleGetDocumentHighlights(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	languageID, _ := args["language_id"].(string)

	line, col, err := extractPosition(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	highlights, wErr := WithDocument[[]types.DocumentHighlight](ctx, client, filePath, languageID, func(fileURI string) ([]types.DocumentHighlight, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetDocumentHighlights(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_document_highlights: %s", wErr)), nil
	}

	data, mErr := json.Marshal(highlights)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling highlights: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}
