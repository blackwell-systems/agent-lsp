package tools

import (
	"fmt"
	"os"
	"strings"
)

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
	fileContent := string(contentBytes)

	parts := strings.SplitN(pattern, "@@", 2)
	prefix := parts[0]
	suffix := parts[1]
	searchText := prefix + suffix

	matchStart := strings.Index(fileContent, searchText)
	if matchStart < 0 {
		return 0, 0, fmt.Errorf("position_pattern not found in file: %q", pattern)
	}

	// offset is the byte position of the character immediately after "@@"
	offset := matchStart + len(prefix)

	// line is 1-indexed: count newlines before offset
	line = strings.Count(fileContent[:offset], "\n") + 1

	// col is 1-indexed: distance from the last newline before offset
	lastNL := strings.LastIndex(fileContent[:offset], "\n")
	if lastNL < 0 {
		// no newline before offset means we're on the first line
		col = offset + 1
	} else {
		col = offset - lastNL
	}

	return line, col, nil
}

// ExtractPositionWithPattern returns the cursor position from args.
// If args["position_pattern"] is non-empty, it calls ResolvePositionPattern
// with the given filePath. Otherwise it falls through to extractPosition(args).
func ExtractPositionWithPattern(args map[string]interface{}, filePath string) (line, col int, err error) {
	pp, _ := args["position_pattern"].(string)
	if pp != "" {
		return ResolvePositionPattern(filePath, pp)
	}
	return extractPosition(args)
}
