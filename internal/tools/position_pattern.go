// position_pattern.go resolves @@-marker patterns to line:column positions.
//
// AI agents frequently provide imprecise positions. Instead of requiring exact
// line:column numbers, tools can accept a position_pattern like "func @@MyFunc"
// where @@ marks the cursor position. This file resolves such patterns to
// precise 1-indexed line:column coordinates by searching the file content.
//
// The @@ marker splits the pattern into prefix ("func ") and suffix ("MyFunc").
// The full search text is "func MyFunc" (prefix + suffix concatenated). The
// cursor position is at byte offset prefix_length within the matched text.
//
// Line/column coordinates are 1-indexed and column is converted to UTF-16
// code units per LSP spec section 3.4 (surrogate pairs for codepoints >= U+10000).
package tools

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// utf16Offset returns the number of UTF-16 code units that precede
// byteOffset in the UTF-8 string line, per LSP spec §3.4.
// byteOffset must fall on a rune boundary within line.
// byteSpanToLSPRange converts the byte span src[startByte:endByte] into a
// 0-based LSP range. LSP characters are UTF-16 code units, not bytes: every
// tool that turns a byte offset (from strings.Index and friends) into a
// position must go through here or utf16Offset, or non-ASCII text earlier on
// the line shifts the position (issue #52).
func byteSpanToLSPRange(src string, startByte, endByte int) (startLine, startChar, endLine, endChar int) {
	before := src[:startByte]
	startLine = strings.Count(before, "\n")
	startLineBegin := strings.LastIndex(before, "\n") + 1 // 0 when there is no newline
	startChar = utf16Offset(src[startLineBegin:startByte], startByte-startLineBegin)

	segment := src[startByte:endByte]
	endLine = startLine + strings.Count(segment, "\n")
	if lastNL := strings.LastIndex(segment, "\n"); lastNL < 0 {
		endChar = startChar + utf16Offset(segment, len(segment))
	} else {
		tail := segment[lastNL+1:]
		endChar = utf16Offset(tail, len(tail))
	}
	return startLine, startChar, endLine, endChar
}

func utf16Offset(line string, byteOffset int) int {
	units := 0
	i := 0
	for i < byteOffset {
		r, size := utf8.DecodeRuneInString(line[i:])
		if r >= 0x10000 {
			units += 2 // surrogate pair in UTF-16
		} else {
			units++
		}
		i += size
	}
	return units
}

// ResolvePositionPattern resolves a "@@" cursor marker in a text pattern to
// a 1-indexed line and column in the given file.
//
// Two pattern forms are supported:
//   - "prefix@@suffix" (exactly one marker): the text before and after "@@"
//     is joined to form the search text; the cursor position is the character
//     immediately following "@@" in the file.
//   - "@@text@@" (double marker): the text between the markers is the search
//     text; the cursor position is the end of the match.
//
// A leading UTF-8 BOM counts as one UTF-16 unit on line 1: agent-lsp sends the
// file to the server with its BOM, and servers (gopls, typescript-language-
// server, pyright) count it, as byteSpanToLSPRange does.
func ResolvePositionPattern(filePath, pattern string) (line, col int, err error) {
	if !strings.Contains(pattern, "@@") {
		return 0, 0, fmt.Errorf("position_pattern must contain @@ marker")
	}
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return resolveInContent(string(contentBytes), pattern)
}

// resolveInContent finds the @@ marker position within content (already loaded).
// content is the raw file text or a line-sliced subset.
//
// Supported pattern forms:
//   - "prefix@@suffix" (exactly one marker): matches prefix+suffix, cursor at
//     the @@ position (where prefix ends within the match).
//   - "@@text@@" (double marker enclosing the text): matches text, cursor at
//     the end of the match. This is the natural form agents write.
//   - anything else (0 markers or ambiguous marker placement) is rejected
//     with an error naming the valid forms.
func resolveInContent(content, pattern string) (line, col int, err error) {
	markerCount := strings.Count(pattern, "@@")
	if markerCount == 0 {
		return 0, 0, fmt.Errorf("position_pattern must contain @@ marker")
	}

	var searchText string
	var cursorWithinMatch int // byte offset of the cursor within the matched text
	switch {
	case markerCount == 1:
		parts := strings.SplitN(pattern, "@@", 2)
		searchText = parts[0] + parts[1]
		cursorWithinMatch = len(parts[0])
	case markerCount == 2 && strings.HasPrefix(pattern, "@@") && strings.HasSuffix(pattern, "@@") && len(pattern) >= 5:
		// Double-marker form: match the text between the markers.
		searchText = pattern[2 : len(pattern)-2]
		cursorWithinMatch = len(searchText)
	default:
		return 0, 0, fmt.Errorf("position_pattern must contain exactly one @@ marker (\"prefix@@suffix\") or enclose the text (\"@@text@@\"); got %q with %d @@ markers", pattern, markerCount)
	}

	if searchText == "" {
		return 0, 0, fmt.Errorf("position_pattern %q resolves to empty search text", pattern)
	}

	matchStart := strings.Index(content, searchText)
	if matchStart < 0 {
		return 0, 0, fmt.Errorf("position_pattern not found in file: %q", pattern)
	}

	// offset is the byte position of the cursor within content
	offset := matchStart + cursorWithinMatch

	// line is 1-indexed: count newlines before offset
	line = strings.Count(content[:offset], "\n") + 1

	// col is 1-indexed UTF-16 code-unit offset from the start of the line.
	var lineStart int
	lastNL := strings.LastIndex(content[:offset], "\n")
	if lastNL < 0 {
		lineStart = 0
	} else {
		lineStart = lastNL + 1
	}
	lineContent := content[lineStart:offset]
	col = utf16Offset(lineContent, len(lineContent)) + 1

	return line, col, nil
}

// ResolvePositionPatternInRange is like ResolvePositionPattern but restricts
// the search to lines [startLine, endLine] (1-indexed, inclusive).
// When startLine == 0 and endLine == 0, the full file is searched (identical
// to ResolvePositionPattern).
// Returns an error if the pattern is not found within the specified range.
func ResolvePositionPatternInRange(filePath, pattern string, startLine, endLine int) (line, col int, err error) {
	if !strings.Contains(pattern, "@@") {
		return 0, 0, fmt.Errorf("position_pattern must contain @@ marker")
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	fileContent := string(contentBytes)

	// When no range restriction, delegate to existing full-file logic.
	if startLine == 0 && endLine == 0 {
		return resolveInContent(fileContent, pattern)
	}

	// Validate bounds.
	if startLine < 1 {
		return 0, 0, fmt.Errorf("line_scope_start must be >= 1, got %d", startLine)
	}
	if endLine < startLine {
		return 0, 0, fmt.Errorf("line_scope_end (%d) must be >= line_scope_start (%d)", endLine, startLine)
	}

	// Slice file to [startLine, endLine].
	lines := strings.Split(fileContent, "\n")
	if startLine > len(lines) {
		return 0, 0, fmt.Errorf("line_scope_start %d exceeds file length %d", startLine, len(lines))
	}
	end := endLine
	if end > len(lines) {
		end = len(lines)
	}
	// lines is 0-indexed; startLine is 1-indexed.
	sliceLines := lines[startLine-1 : end]
	sliceContent := strings.Join(sliceLines, "\n")

	sliceLine, sliceCol, err := resolveInContent(sliceContent, pattern)
	if err != nil {
		return 0, 0, fmt.Errorf("position_pattern not found in lines %d-%d: %w", startLine, endLine, err)
	}

	// Translate slice-relative line number back to file-absolute.
	return sliceLine + (startLine - 1), sliceCol, nil
}

// ExtractPositionWithPattern returns the cursor position from args.
// If args["position_pattern"] is non-empty, it calls ResolvePositionPatternInRange
// with optional line_scope_start and line_scope_end args (0 means no restriction).
// Otherwise it falls through to extractPosition(args).
func ExtractPositionWithPattern(args map[string]any, filePath string) (line, col int, err error) {
	pp, _ := args["position_pattern"].(string)
	if pp != "" {
		scopeStart, _ := toIntOptional(args, "line_scope_start")
		scopeEnd, _ := toIntOptional(args, "line_scope_end")
		return ResolvePositionPatternInRange(filePath, pp, scopeStart, scopeEnd)
	}
	return extractPosition(args)
}

// toIntOptional reads an integer arg, returning 0 and no error when absent.
func toIntOptional(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, nil
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	}
	return 0, fmt.Errorf("%s must be an integer, got %T", key, v)
}
