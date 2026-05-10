package tools

import (
	"encoding/json"
	"os"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// EstimateTokenSavings computes approximate token savings by comparing
// the size of returnedText to the full file size.
// Uses len/4 as a rough token approximation (1 token ~ 4 chars).
func EstimateTokenSavings(returnedText string, filePath string) map[string]int {
	tokensReturned := len(returnedText) / 4
	result := map[string]int{
		"tokens_returned": tokensReturned,
	}
	info, err := os.Stat(filePath)
	if err == nil {
		tokensFullFile := int(info.Size()) / 4
		result["tokens_full_file"] = tokensFullFile
		result["tokens_saved"] = tokensFullFile - tokensReturned
	}
	return result
}

// AppendTokenMeta wraps a ToolResult with token savings metadata.
// Appends a JSON content item with _meta.token_savings.
// Returns unchanged if result.IsError or Content is empty.
func AppendTokenMeta(result types.ToolResult, filePath string) types.ToolResult {
	if result.IsError || len(result.Content) == 0 || result.Content[0].Text == "" {
		return result
	}
	text := result.Content[0].Text
	meta := EstimateTokenSavings(text, filePath)
	metaJSON, err := json.Marshal(map[string]any{"_meta": map[string]any{"token_savings": meta}})
	if err != nil {
		return result
	}
	result.Content = append(result.Content, types.ContentItem{
		Type: "text",
		Text: string(metaJSON),
	})
	return result
}
