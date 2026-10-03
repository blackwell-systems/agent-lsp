package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// maxProbeEntries bounds the total filesystem entries the coverage probe
// examines: files, directories, and excluded entries all count. Bounding
// total entries — not just counted files — keeps the probe cheap on trees
// with many directories, and exhaustion is reported as unknown coverage
// instead of silently classifying a partial walk as complete. (issue #42;
// CodeRabbit #51 thread 2, #59 threads 5-6)
const maxProbeEntries = 2000

// workspaceCoverage is the probe's verdict on workspace index coverage.
type workspaceCoverage int

const (
	// coverageUnopened: the probe found eligible workspace files the
	// session never opened.
	coverageUnopened workspaceCoverage = iota
	// coverageComplete: every eligible file the probe saw was opened in
	// this session.
	coverageComplete
	// coverageUnknown: the probe exhausted its entry budget, hit a walk
	// error, or was cancelled before classifying the workspace — callers
	// must stay conservative (show the caveat).
	coverageUnknown
)

// probeWorkspaceCoverage classifies whether the workspace root contains
// eligible files (non-hidden files outside dot- and skip-directories) that
// the session never didOpen'ed. Some language servers index only opened
// documents, so workspace/symbol and references queries can return empty
// for symbols that exist in those files; tool handlers use this verdict to
// qualify empty results instead of presenting them as authoritative
// "not found" answers. (issue #42)
//
// Coverage is compared against the opened PATH SET — not an open-document
// count — so opens outside the eligible workspace set (e.g. node_modules)
// cannot mask unopened source files. The walk reads one directory at a
// time (os.ReadDir) under a total-entry budget: directory visits and
// excluded entries consume the budget too, ctx cancellation stops it
// early, and any interruption yields coverageUnknown rather than a
// deceptively complete classification.
func probeWorkspaceCoverage(ctx context.Context, root string, opened map[string]bool) workspaceCoverage {
	if root == "" {
		return coverageComplete
	}
	pending := []string{root}
	seen := 0
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return coverageUnknown
		}
		dir := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		entries, err := os.ReadDir(dir)
		if err != nil {
			// This subtree could not be classified — stay conservative.
			return coverageUnknown
		}
		for _, entry := range entries {
			seen++
			if seen > maxProbeEntries {
				// The workspace could not be classified within the
				// budget — unknown coverage, not a complete one.
				return coverageUnknown
			}
			name := entry.Name()
			if entry.IsDir() {
				if strings.HasPrefix(name, ".") || skipDirs[name] {
					continue
				}
				pending = append(pending, filepath.Join(dir, name))
				continue
			}
			if strings.HasPrefix(name, ".") {
				continue
			}
			if !opened[filepath.Join(dir, name)] {
				return coverageUnopened
			}
		}
	}
	return coverageComplete
}

// unopenedFilesCaveat is the empty-result wording for tools whose servers
// may only index opened documents. It states the limitation and the cheap
// recovery (open the declaring file and retry) instead of asserting a
// "not found" the index cannot support. (issue #42)
const unopenedFilesCaveat = "some servers only index opened documents — symbols or references in files not yet opened may be missing from this result; open the declaring file (open_document) and retry"

// noteIndexCoverage returns the caveat sentence when the workspace may have
// files the session never opened, and an empty string otherwise. Unknown
// coverage (budget exhaustion, walk errors, cancellation) also yields the
// caveat — it is the honest wording when coverage cannot be established.
func noteIndexCoverage(ctx context.Context, client *lsp.LSPClient) string {
	if probeWorkspaceCoverage(ctx, client.RootDir(), openedDocumentPaths(client)) != coverageComplete {
		return unopenedFilesCaveat
	}
	return ""
}

// openedDocumentPaths returns the on-disk paths of the documents opened in
// the current session. Comparing path sets — not an open-document count —
// keeps opens outside the eligible workspace set (e.g. node_modules) from
// masking unopened source files. (CodeRabbit #59 thread 4)
func openedDocumentPaths(client *lsp.LSPClient) map[string]bool {
	out := make(map[string]bool)
	for _, u := range client.GetOpenDocuments() {
		if p, err := URIToFilePath(u); err == nil {
			out[p] = true
		}
	}
	return out
}
