package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleGetInlayHints returns inlay hints for a range within a document.
// Inlay hints display inferred type annotations and parameter name labels
// inline with source code — the same annotations IDEs show in TypeScript,
// Rust, Go, and other languages with type inference.
//
// Required args: file_path, start_line, start_column, end_line, end_column (1-based).
// Optional arg: language_id.
//
// Returns an empty array when the connected language server does not support
// inlayHintProvider or has no hints for the given range.
func HandleGetInlayHints(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	languageID, _ := args["language_id"].(string)

	rng, err := extractRange(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	hints, wErr := WithDocument[[]types.InlayHint](ctx, client, filePath, languageID, func(fileURI string) ([]types.InlayHint, error) {
		return client.GetInlayHints(ctx, fileURI, rng)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_inlay_hints: %s", wErr)), nil
	}

	data, mErr := json.Marshal(hints)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling inlay hints: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}
