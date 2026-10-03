package tools

import (
	"context"
	"io"
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

// maxProbeBatchEntries bounds how many directory entries each File.ReadDir
// call loads, so a single directory with a huge number of entries cannot be
// read and sorted in full before the budget or ctx check runs.
// (CodeRabbit #51 re-review thread 2)
const maxProbeBatchEntries = 128

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
	root = canonicalizeIndexCoveragePath(root)
	pending := []string{root}
	seen := 0
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return coverageUnknown
		}
		dir := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		// Read the directory in bounded batches (os.ReadDir would load and
		// sort every entry before the budget could stop the walk) and re-check
		// ctx and the budget between batches.
		dirHandle, err := os.Open(dir)
		if err != nil {
			// This subtree could not be classified — stay conservative.
			return coverageUnknown
		}
		for {
			if err := ctx.Err(); err != nil {
				dirHandle.Close()
				return coverageUnknown
			}
			entries, rerr := dirHandle.ReadDir(maxProbeBatchEntries)
			for _, entry := range entries {
				seen++
				if seen > maxProbeEntries {
					dirHandle.Close()
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
				// Canonicalize the discovered path too: an opened file may be
				// a symlink, whose opened-set entry resolves to the target —
				// comparing unresolved link paths would report a false
				// coverageUnopened. (CodeRabbit #51 re-review thread 2)
				if !opened[canonicalizeIndexCoveragePath(filepath.Join(dir, name))] {
					dirHandle.Close()
					return coverageUnopened
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				dirHandle.Close()
				return coverageUnknown
			}
		}
		dirHandle.Close()
	}
	return coverageComplete
}

// canonicalizeIndexCoveragePath resolves symlinks so both sides of the
// coverage comparison — the walk's root-prefixed paths and the URI-derived
// opened paths — key on the same canonical form; a symlinked workspace root
// would otherwise key the walk by the symlink path while the opened set uses
// the resolved path, producing a spurious caveat. Falls back to the lexical
// path when the target cannot be resolved (freshly created files).
// (CodeRabbit #51 re-review thread 1)
func canonicalizeIndexCoveragePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
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
			out[canonicalizeIndexCoveragePath(p)] = true
		}
	}
	return out
}

// noteIndexCoverageMulti is the multi-server variant of noteIndexCoverage.
// Each client that will actually be queried — initialized and declaring
// workspaceSymbolProvider — establishes coverage independently: the first
// client's open-document set says nothing about another server's index, so
// checking only the default client could omit the caveat when one server is
// fully opened but another is not. The caveat shows when ANY queried client
// has incomplete coverage. (CodeRabbit #59 re-review thread)
func noteIndexCoverageMulti(ctx context.Context, clients []*lsp.LSPClient) string {
	for _, c := range clients {
		if c == nil || !c.IsInitialized() || !c.HasCapability("workspaceSymbolProvider") {
			continue
		}
		if probeWorkspaceCoverage(ctx, c.RootDir(), openedDocumentPaths(c)) != coverageComplete {
			return unopenedFilesCaveat
		}
	}
	return ""
}
