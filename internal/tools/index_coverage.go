package tools

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// maxUnopenedProbeFiles bounds the workspace walk in
// hasUnopenedWorkspaceFiles. The helper only needs to answer "are there
// workspace files the session never opened?" — a bounded walk keeps the
// check cheap on large trees; hitting the cap still answers the question
// (the workspace has more files than the session has opened).
const maxUnopenedProbeFiles = 2000

// hasUnopenedWorkspaceFiles reports whether the workspace contains source
// files the session never didOpen'ed. Some language servers index only
// opened documents, so workspace/symbol and references queries can return
// empty for symbols that exist in those files; tool handlers use this to
// qualify empty results instead of presenting them as authoritative
// "not found" answers. (issue #42)
//
// The walk is bounded by maxUnopenedProbeFiles and skips the same
// non-source directories (dot-dirs, skipDirs from detect.go) the server
// detection walk skips. On any walk error it returns true: the caveat is
// the honest wording when coverage cannot be established.
func hasUnopenedWorkspaceFiles(client *lsp.LSPClient) bool {
	return hasUnopenedFiles(client.RootDir(), client.OpenDocumentCount())
}

// hasUnopenedFiles is the testable core of hasUnopenedWorkspaceFiles: given
// the workspace root and the count of documents opened in the session, it
// reports whether the workspace contains more indexable files than the
// session has opened. (issue #42)
func hasUnopenedFiles(root string, opened int) bool {
	if root == "" {
		return false
	}
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || skipDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		seen++
		if seen > maxUnopenedProbeFiles || seen > opened {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		// Could not establish coverage — stay conservative.
		return true
	}
	return seen > opened
}

// unopenedFilesCaveat is the empty-result wording for tools whose servers
// may only index opened documents. It states the limitation and the cheap
// recovery (open the declaring file and retry) instead of asserting a
// "not found" the index cannot support. (issue #42)
const unopenedFilesCaveat = "some servers only index opened documents — symbols or references in files not yet opened may be missing from this result; open the declaring file (open_document) and retry"

// noteIndexCoverage returns the caveat sentence when the workspace has
// files the session never opened, and an empty string otherwise, so
// callers can append it conditionally without double punctuation logic.
func noteIndexCoverage(client *lsp.LSPClient) string {
	if hasUnopenedWorkspaceFiles(client) {
		return unopenedFilesCaveat
	}
	return ""
}
