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
// The pattern must contain exactly one "@@" marker. The text before and after
// "@@" is joined to form the search text. The cursor position is the character
// immediately following "@@" in the file.
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
func resolveInContent(content, pattern string) (line, col int, err error) {
	parts := strings.SplitN(pattern, "@@", 2)
	prefix := parts[0]
	suffix := parts[1]
	searchText := prefix + suffix

	matchStart := strings.Index(content, searchText)
	if matchStart < 0 {
		return 0, 0, fmt.Errorf("position_pattern not found in file: %q", pattern)
	}

	// offset is the byte position of the character immediately after "@@"
	offset := matchStart + len(prefix)

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
func ExtractPositionWithPattern(args map[string]interface{}, filePath string) (line, col int, err error) {
	pp, _ := args["position_pattern"].(string)
	if pp != "" {
		scopeStart, _ := toIntOptional(args, "line_scope_start")
		scopeEnd, _ := toIntOptional(args, "line_scope_end")
		return ResolvePositionPatternInRange(filePath, pp, scopeStart, scopeEnd)
	}
	return extractPosition(args)
}

// toIntOptional reads an integer arg, returning 0 and no error when absent.
func toIntOptional(args map[string]interface{}, key string) (int, error) {
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
