package tools

import (
	"encoding/json"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// AppendIndexedField adds an "indexed" boolean field to the first content
// item's JSON payload in the tool result. The field reflects whether the
// workspace has finished indexing (client.IsWorkspaceLoaded()). If the client
// is nil, the result is an error, or the content is not valid JSON, the result
// is returned unchanged.
func AppendIndexedField(result types.ToolResult, client *lsp.LSPClient) types.ToolResult {
	if client == nil {
		return result
	}
	if result.IsError {
		return result
	}
	if len(result.Content) == 0 {
		return result
	}

	text := result.Content[0].Text
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return result
	}

	obj["indexed"] = client.IsWorkspaceLoaded()

	out, err := json.Marshal(obj)
	if err != nil {
		return result
	}

	result.Content[0].Text = string(out)
	return result
}
