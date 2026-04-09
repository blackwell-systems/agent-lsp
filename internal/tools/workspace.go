package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// HandleRenameSymbol renames the symbol at the given location across the workspace.
// When the direct position lookup returns an empty WorkspaceEdit, it falls back to
// workspace symbol search by hover name and retries — handling imprecise AI positions.
func HandleRenameSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	newName, ok := args["new_name"].(string)
	if !ok || newName == "" {
		return types.ErrorResult("new_name is required"), nil
	}

	line, col, err := ExtractPositionWithPattern(args, filePath)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[interface{}](ctx, client, filePath, languageID, func(fileURI string) (interface{}, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		res, rErr := client.RenameSymbol(ctx, fileURI, pos, newName)
		if rErr != nil {
			return nil, rErr
		}
		if isEmptyWorkspaceEdit(res) {
			logging.Log(logging.LevelDebug, "rename_symbol: empty result at exact position, trying fuzzy fallback")
			res = renameWithFuzzyFallback(ctx, client, fileURI, line, col, newName)
		}
		return res, nil
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("rename_symbol: %s", wErr)), nil
	}

	dryRun, _ := args["dry_run"].(bool)
	if dryRun {
		type previewEnvelope struct {
			WorkspaceEdit interface{} `json:"workspace_edit"`
			Preview       struct {
				Note string `json:"note"`
			} `json:"preview"`
		}
		env := previewEnvelope{WorkspaceEdit: result}
		env.Preview.Note = "dry_run=true: inspect workspace_edit and call apply_edit to commit"
		data, mErr := json.Marshal(env)
		if mErr != nil {
			return types.ErrorResult(fmt.Sprintf("marshaling dry_run result: %s", mErr)), nil
		}
		return types.TextResult(string(data)), nil
	}
	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling rename result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// renameWithFuzzyFallback retries rename using workspace symbol candidates when the
// direct position lookup returned an empty WorkspaceEdit. Mirrors the pattern used
// by go_to_definition and get_references for position-imprecise AI callers.
func renameWithFuzzyFallback(ctx context.Context, client *lsp.LSPClient, fileURI string, line, col int, newName string) interface{} {
	hoverPos := types.Position{Line: line - 1, Character: col - 1}
	hoverText, err := client.GetInfoOnLocation(ctx, fileURI, hoverPos)
	if err != nil || hoverText == "" {
		return nil
	}

	symbolName := extractSymbolName(hoverText)
	if symbolName == "" {
		return nil
	}

	logging.Log(logging.LevelDebug, "rename fuzzyFallback: searching workspace symbols for "+symbolName)

	syms, symErr := client.GetWorkspaceSymbols(ctx, symbolName)
	if symErr != nil || len(syms) == 0 {
		return nil
	}

	for _, sym := range syms {
		if sym.Location.URI == "" {
			continue
		}
		candidatePos := types.Position{
			Line:      sym.Location.Range.Start.Line,
			Character: sym.Location.Range.Start.Character,
		}
		res, rErr := client.RenameSymbol(ctx, sym.Location.URI, candidatePos, newName)
		if rErr == nil && !isEmptyWorkspaceEdit(res) {
			logging.Log(logging.LevelDebug, "rename fuzzyFallback: found result via workspace symbol candidate")
			return res
		}
	}
	return nil
}

// isEmptyWorkspaceEdit reports whether a RenameSymbol result contains no edits.
// A nil result or one that marshals to "null" or "{}" is considered empty —
// indicating the server found no symbol at the requested position.
func isEmptyWorkspaceEdit(result interface{}) bool {
	if result == nil {
		return true
	}
	data, err := json.Marshal(result)
	if err != nil {
		return true
	}
	s := string(data)
	return s == "null" || s == "{}"
}

// HandlePrepareRename checks whether a rename is valid at the given location.
func HandlePrepareRename(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	result, wErr := WithDocument[interface{}](ctx, client, filePath, languageID, func(fileURI string) (interface{}, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.PrepareRename(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("prepare_rename: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling prepare_rename result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleFormatDocument formats an entire document.
func HandleFormatDocument(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	tabSize := 2
	if v, err := toInt(args, "tab_size"); err != nil && args["tab_size"] != nil {
		return types.ErrorResult(fmt.Sprintf("tab_size: %s", err)), nil
	} else if err == nil {
		tabSize = v
	}

	insertSpaces := true
	if v, ok := args["insert_spaces"].(bool); ok {
		insertSpaces = v
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[[]types.TextEdit](ctx, client, filePath, languageID, func(fileURI string) ([]types.TextEdit, error) {
		return client.FormatDocument(ctx, fileURI, tabSize, insertSpaces)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("format_document: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling format result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleFormatRange formats a range within a document.
func HandleFormatRange(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
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

	tabSize := 2
	if v, tErr := toInt(args, "tab_size"); tErr != nil && args["tab_size"] != nil {
		return types.ErrorResult(fmt.Sprintf("tab_size: %s", tErr)), nil
	} else if tErr == nil {
		tabSize = v
	}

	insertSpaces := true
	if v, ok := args["insert_spaces"].(bool); ok {
		insertSpaces = v
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[[]types.TextEdit](ctx, client, filePath, languageID, func(fileURI string) ([]types.TextEdit, error) {
		return client.FormatRange(ctx, fileURI, rng, tabSize, insertSpaces)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("format_range: %s", wErr)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling format_range result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleApplyEdit applies a workspace edit.
//
// Two modes:
//   - WorkspaceEdit mode (default): supply workspace_edit with positional changes.
//   - Text-match mode: supply file_path + old_text + new_text. The tool finds
//     old_text in the file (exact match first, then whitespace-normalized line
//     match) and replaces it with new_text without requiring line/column positions.
func HandleApplyEdit(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Text-match mode: file_path + old_text + new_text.
	oldText, hasOld := args["old_text"].(string)
	filePath, hasPath := args["file_path"].(string)
	if hasOld && oldText != "" && hasPath && filePath != "" {
		newText, _ := args["new_text"].(string)
		edit, err := textMatchApply(filePath, oldText, newText)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("apply_edit (text-match): %s", err)), nil
		}
		if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
			return types.ErrorResult(fmt.Sprintf("apply_edit: %s", err)), nil
		}
		return types.TextResult("Edit applied successfully"), nil
	}

	// WorkspaceEdit mode.
	edit, ok := args["workspace_edit"]
	if !ok || edit == nil {
		return types.ErrorResult("workspace_edit is required (or supply file_path + old_text + new_text for text-match mode)"), nil
	}

	if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
		return types.ErrorResult(fmt.Sprintf("apply_edit: %s", err)), nil
	}
	return types.TextResult("Edit applied successfully"), nil
}

// textMatchApply constructs a WorkspaceEdit by locating oldText in the file at
// filePath and replacing it with newText. It tries an exact byte match first;
// if that fails it falls back to a line-by-line whitespace-normalised match
// (each line compared after strings.TrimSpace). The normalised match replaces
// full lines, so newText should include the desired indentation for those lines.
func textMatchApply(filePath, oldText, newText string) (interface{}, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}
	src := string(content)

	startByte, endByte, ok := findText(src, oldText)
	if !ok {
		return nil, fmt.Errorf("old_text not found in %s (tried exact and whitespace-normalised line match)", filePath)
	}

	// Compute LSP range (0-based line/character) from byte offsets.
	before := src[:startByte]
	startLine := strings.Count(before, "\n")
	lastNL := strings.LastIndex(before, "\n")
	var startChar int
	if lastNL < 0 {
		startChar = startByte
	} else {
		startChar = startByte - lastNL - 1
	}

	segment := src[startByte:endByte]
	endLine := startLine + strings.Count(segment, "\n")
	lastNLInSeg := strings.LastIndex(segment, "\n")
	var endChar int
	if lastNLInSeg < 0 {
		endChar = startChar + len(segment)
	} else {
		endChar = len(segment) - lastNLInSeg - 1
	}

	fileURI := "file://" + filePath
	edit := map[string]interface{}{
		"changes": map[string]interface{}{
			fileURI: []interface{}{
				map[string]interface{}{
					"range": map[string]interface{}{
						"start": map[string]interface{}{"line": startLine, "character": startChar},
						"end":   map[string]interface{}{"line": endLine, "character": endChar},
					},
					"newText": newText,
				},
			},
		},
	}
	return edit, nil
}

// findText returns the byte range [start, end) of oldText within src.
// It first tries an exact bytes match; on failure it attempts a line-by-line
// comparison where each line is stripped of leading/trailing whitespace before
// comparison. The normalised match returns the byte range of the full matched
// lines (including their original indentation) so callers replace complete lines.
func findText(src, oldText string) (start, end int, found bool) {
	// Exact match.
	if idx := strings.Index(src, oldText); idx >= 0 {
		return idx, idx + len(oldText), true
	}

	// Whitespace-normalised line match.
	srcLines := strings.Split(src, "\n")
	patLines := strings.Split(oldText, "\n")
	// Drop a trailing empty element caused by a trailing newline in oldText.
	if len(patLines) > 0 && patLines[len(patLines)-1] == "" {
		patLines = patLines[:len(patLines)-1]
	}
	if len(patLines) == 0 || len(patLines) > len(srcLines) {
		return 0, 0, false
	}

	normPat := make([]string, len(patLines))
	for i, l := range patLines {
		normPat[i] = strings.TrimSpace(l)
	}

	for i := 0; i <= len(srcLines)-len(patLines); i++ {
		match := true
		for j, pat := range normPat {
			if strings.TrimSpace(srcLines[i+j]) != pat {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// Compute byte offsets: start at beginning of line i, end after last
		// character of line i+len(patLines)-1 (before its newline, if any).
		byteStart := 0
		for k := 0; k < i; k++ {
			byteStart += len(srcLines[k]) + 1 // +1 for '\n'
		}
		byteEnd := byteStart
		lastIdx := i + len(patLines) - 1
		for k := i; k <= lastIdx; k++ {
			byteEnd += len(srcLines[k])
			if k < len(srcLines)-1 {
				byteEnd++ // '\n'
			}
		}
		return byteStart, byteEnd, true
	}

	return 0, 0, false
}

// HandleExecuteCommand executes a workspace command.
func HandleExecuteCommand(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	command, ok := args["command"].(string)
	if !ok || command == "" {
		return types.ErrorResult("command is required"), nil
	}

	var cmdArgs []interface{}
	if v, ok := args["arguments"].([]interface{}); ok {
		cmdArgs = v
	}

	result, err := client.ExecuteCommand(ctx, command, cmdArgs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("execute_command: %s", err)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling execute_command result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}
