// uri_helpers.go centralizes filesystem-path → file:// URI conversion so
// every LSP-bound URI we emit is constructed the same way.
//
// Background — agent-lsp 0.11.2 had multiple call sites doing
//
//	(&url.URL{Scheme: "file", Path: somePath}).String()
//
// which on Windows produced strings of the form `file://S:/Source/...`.
// Go's url.URL formatter treats anything before the first `/` of Path
// as the URI authority (host) component, so `S:` got promoted to the
// authority slot. Some downstream LSP servers (pyright, rust-analyzer,
// typescript-language-server) percent-encoded the resulting URI back
// into a UNC-style path (`\\S:\Source\...`) and reported the workspace
// root as non-existent. The full LSP session then failed.
//
// PathToFileURI produces an RFC 8089 / VS Code-compatible Windows file
// URI of the form `file:///S:/Source/Personal/Channels/PlotPackets`,
// with the empty authority (`//`) and the drive letter under the path,
// where downstream servers expect it.
package lsp

import (
	"net/url"
	"path/filepath"
	"strings"
)

// PathToFileURI converts an absolute filesystem path into an LSP
// `file://` URI that round-trips correctly on both Windows and POSIX.
//
//   - POSIX `/home/user/proj`      → `file:///home/user/proj`
//   - Windows `S:\Source\Proj`     → `file:///S:/Source/Proj`
//   - Windows `S:/Source/Proj`     → `file:///S:/Source/Proj`
//   - Empty input                  → empty string
//
// Path components are percent-encoded per net/url rules.
func PathToFileURI(p string) string {
	if p == "" {
		return ""
	}
	// LSP URIs always use forward slashes regardless of OS.
	p = filepath.ToSlash(p)

	// Windows absolute paths (with drive letter) need a leading slash
	// inserted before the drive so the URI parses as
	//   file:///s:/path
	// rather than treating the drive letter as the authority.
	// Normalize the drive letter to lowercase: most language servers
	// (csharp-ls, roslyn, pyright) emit lowercase drive letters in URIs.
	// Mismatched casing causes silent lookup failures where diagnostics
	// and symbols are keyed under a different URI than what agent-lsp queries.
	if len(p) >= 2 && p[1] == ':' && isASCIILetter(p[0]) {
		p = "/" + strings.ToLower(p[:1]) + p[1:]
	}

	// `&url.URL{Scheme: "file", Path: ...}.String()` writes
	// `file:` + `//` + authority + path. We want an empty authority
	// (RFC 8089 "local file" form), which url.URL produces when the
	// path itself starts with a leading slash. Belt-and-braces: build
	// it through url.URL so any path components needing
	// percent-encoding (spaces, unicode) are handled correctly.
	u := url.URL{Scheme: "file", Path: p}
	s := u.String()

	// url.URL.String() omits the `//` for empty authority when the
	// path doesn't start with `/`. For paths that DO start with `/`
	// (which is all we produce here), it writes `file://` + path,
	// yielding `file:///`. Verify and force the canonical form.
	if !strings.HasPrefix(s, "file:///") && strings.HasPrefix(s, "file:") {
		// Some Go versions write `file:/...` instead of `file:///...`
		// for absolute paths. Normalize.
		s = "file://" + strings.TrimPrefix(s, "file:")
	}
	return s
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// NormalizeFileURI lowercases the drive letter in a Windows file:// URI
// so that URIs from different sources (agent-lsp vs language server) match.
// Non-Windows URIs and non-file URIs are returned unchanged.
//
//	file:///C:/foo → file:///c:/foo
//	file:///c:/foo → file:///c:/foo (no-op)
//	file:///home/x → file:///home/x (no-op)
func NormalizeFileURI(uri string) string {
	// file:///X:/ where X is the drive letter at index 8
	if len(uri) >= 10 && strings.HasPrefix(uri, "file:///") && uri[9] == ':' && isASCIILetter(uri[8]) {
		return uri[:8] + strings.ToLower(uri[8:9]) + uri[9:]
	}
	return uri
}
