package tools

import (
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// AppendIndexedField tells the agent whether the workspace has finished
// indexing (client.IsWorkspaceLoaded()), so it can tell an incomplete result
// from a complete one.
//
// When the result is a JSON object, an "indexed" field is added to it. Any
// other result (GCF, the default output format, or a JSON array) gets an
// "indexed=..." line as a separate content item, the way hints are attached,
// so the primary output is never re-parsed or altered. Previously the field
// was added only by decoding the text as a JSON object, which silently dropped
// it under GCF (issue #75). Nil clients and error results are returned
// unchanged.
func AppendIndexedField(result types.ToolResult, client *lsp.LSPClient) types.ToolResult {
	if client == nil || result.IsError || len(result.Content) == 0 {
		return result
	}
	indexed := client.IsWorkspaceLoaded()

	var obj map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &obj); err == nil && obj != nil {
		obj["indexed"] = indexed
		if out, err := json.Marshal(obj); err == nil {
			result.Content[0].Text = string(out)
			return result
		}
	}

	line := fmt.Sprintf("indexed=%t", indexed)
	if !indexed {
		line += " (the language server has not finished indexing the workspace, so this result may be incomplete)"
	}
	result.Content = append(result.Content, types.ContentItem{Type: "text", Text: line})
	return result
}
