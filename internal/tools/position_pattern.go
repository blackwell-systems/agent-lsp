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

	// col is 1-indexed UTF-16 code-unit offset from the start of the line.
	var lineStart int
	lastNL := strings.LastIndex(fileContent[:offset], "\n")
	if lastNL < 0 {
		lineStart = 0
	} else {
		lineStart = lastNL + 1
	}
	lineContent := fileContent[lineStart:offset]
	col = utf16Offset(lineContent, len(lineContent)) + 1

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
