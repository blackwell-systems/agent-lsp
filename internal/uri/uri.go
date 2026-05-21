package uri

import (
	"net/url"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// URIToPath converts a file:// URI to a local filesystem path,
// correctly decoding percent-encoded characters per RFC 3986.
//
// Windows handling — RFC 8089 says a Windows file URI looks like
// `file:///C:/foo/bar` (note the `///` and the drive letter UNDER the
// path). Go's url.URL parses this with `u.Path == "/C:/foo/bar"` — the
// drive letter sits under a leading slash that is *not* part of the
// path on disk. Returning u.Path verbatim produces invalid Windows
// paths that fail os.Stat and break every downstream string-compare
// against canonical paths (e.g. find_references aggregating callers
// across the workspace silently dropped any caller whose URI came
// back in this form).
//
// We also accept and recover the agent-lsp 0.11.2 malformed form
// `file://X:\foo\bar` that pyright sometimes emits on Windows — there
// the drive ends up under u.Host and the backslash-separated path
// under u.Path; we re-stitch them.
//
// Canonical implementation shared by internal/lsp and internal/session.
func URIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		// Fallback: strip the scheme prefix manually.
		if strings.HasPrefix(uri, "file://") {
			return uri[len("file://"):]
		}
		return uri
	}

	p := u.Path

	// Recover from the agent-lsp 0.11.2 malformed Windows URI form
	// where the drive letter landed under the authority (Host) field.
	// url.Parse("file://S:\\foo\\bar") yields u.Host="S:" and
	// u.Path="\\foo\\bar". Re-stitch into "S:\\foo\\bar".
	if u.Host != "" && len(u.Host) == 2 && u.Host[1] == ':' && isASCIILetter(u.Host[0]) {
		// Trim a trailing slash that pyright sometimes appends.
		p = strings.TrimSuffix(p, "/")
		return u.Host + p
	}

	if p == "" {
		// Edge case: well-formed URI with no path component (e.g.
		// bare "file://"). Fall back to prefix-stripping to match the
		// historical contract (the pre-patch implementation returned
		// "" for this case).
		if strings.HasPrefix(uri, "file://") {
			return uri[len("file://"):]
		}
		return uri
	}

	// Canonical RFC 8089 Windows form: u.Path == "/X:/foo/bar".
	// Strip the leading slash so the drive letter sits at the path
	// root, where Windows expects it.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' && isASCIILetter(p[1]) {
		p = p[1:]
	}
	return p
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
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
