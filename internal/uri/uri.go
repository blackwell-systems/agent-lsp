package uri

import (
	"net/url"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// URIToPath converts a file:// URI to a local filesystem path,
// correctly decoding percent-encoded characters per RFC 3986.
// Canonical implementation shared by internal/lsp and internal/session (M3).
func URIToPath(uri string) string {
	if u, err := url.Parse(uri); err == nil && u.Path != "" {
		return u.Path
	}
	if strings.HasPrefix(uri, "file://") {
		return uri[len("file://"):]
	}
	return uri
}

// ApplyRangeEdit applies a single range edit to content in-memory and
// returns the new content string. Canonical implementation shared by
// internal/lsp and internal/session (L5 deduplication).
func ApplyRangeEdit(content string, rng types.Range, newText string) string {
	lines := strings.Split(content, "\n")

	startLine := rng.Start.Line
	startChar := rng.Start.Character
	endLine := rng.End.Line
	endChar := rng.End.Character

	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	before := ""
	if startLine >= 0 && startLine < len(lines) {
		l := lines[startLine]
		if startChar > len(l) {
			startChar = len(l)
		}
		before = l[:startChar]
	}

	after := ""
	if endLine >= 0 && endLine < len(lines) {
		l := lines[endLine]
		if endChar > len(l) {
			endChar = len(l)
		}
		after = l[endChar:]
	}

	newLines := strings.Split(newText, "\n")
	newLines[0] = before + newLines[0]
	newLines[len(newLines)-1] += after

	result := make([]string, 0, len(lines)-(endLine-startLine)+len(newLines))
	result = append(result, lines[:startLine]...)
	result = append(result, newLines...)
	result = append(result, lines[endLine+1:]...)

	return strings.Join(result, "\n")
}
