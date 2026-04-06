package tools

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// validLogLevels is the set of allowed values for set_log_level.
var validLogLevels = map[string]bool{
	logging.LevelDebug:     true,
	logging.LevelInfo:      true,
	logging.LevelNotice:    true,
	logging.LevelWarning:   true,
	logging.LevelError:     true,
	logging.LevelCritical:  true,
	logging.LevelAlert:     true,
	logging.LevelEmergency: true,
}

// HandleDidChangeWatchedFiles notifies the LSP server that watched files have changed.
func HandleDidChangeWatchedFiles(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	rawChanges, ok := args["changes"].([]interface{})
	if !ok {
		return types.ErrorResult("changes must be an array"), nil
	}

	changes := make([]types.FileChangeEvent, 0, len(rawChanges))
	for i, raw := range rawChanges {
		m, ok := raw.(map[string]interface{})
		if !ok {
			return types.ErrorResult(fmt.Sprintf("changes[%d] must be an object", i)), nil
		}

		uri, _ := m["uri"].(string)
		if uri == "" {
			return types.ErrorResult(fmt.Sprintf("changes[%d].uri is required", i)), nil
		}

		changeType, err := toInt(m, "type")
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("changes[%d].type: %s", i, err)), nil
		}
		if changeType < 1 || changeType > 3 {
			return types.ErrorResult(fmt.Sprintf("changes[%d].type must be 1 (created), 2 (changed), or 3 (deleted)", i)), nil
		}

		changes = append(changes, types.FileChangeEvent{URI: uri, Type: changeType})
	}

	if err := client.DidChangeWatchedFiles(changes); err != nil {
		return types.ErrorResult(fmt.Sprintf("did_change_watched_files: %s", err)), nil
	}
	return types.TextResult("File change notifications sent"), nil
}

// HandleSetLogLevel sets the minimum log level for the server.
func HandleSetLogLevel(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	level, ok := args["level"].(string)
	if !ok || level == "" {
		return types.ErrorResult("level is required"), nil
	}

	if !validLogLevels[level] {
		return types.ErrorResult(fmt.Sprintf(
			"invalid log level %q; valid values: debug, info, notice, warning, error, critical, alert, emergency",
			level,
		)), nil
	}

	logging.SetLevel(level)
	return types.TextResult(fmt.Sprintf("Log level set to %q", level)), nil
}
