package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleGetSemanticTokens retrieves semantic tokens for a range in a document.
// Uses textDocument/semanticTokens/range (or full as fallback).
// Returns an array of SemanticToken objects with 1-based positions and
// human-readable token type and modifier names.
func HandleGetSemanticTokens(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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

	tokens, wErr := WithDocument[[]types.SemanticToken](ctx, client, filePath, languageID, func(fileURI string) ([]types.SemanticToken, error) {
		return client.GetSemanticTokens(ctx, fileURI, rng)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_semantic_tokens: %s", wErr)), nil
	}

	if len(tokens) == 0 {
		return types.TextResult("No semantic tokens found in the specified range. The language server may not support semantic tokens, or there are no tokens in this range."), nil
	}

	data, mErr := json.Marshal(tokens)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling semantic tokens: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}
