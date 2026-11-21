package tools

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	uripkg "github.com/blackwell-systems/agent-lsp/internal/uri"
)

// renameRetryDelay is how long rename_symbol waits before asking the server a
// second time when files that mention the symbol are missing from the edit.
const renameRetryDelay = 2 * time.Second

// Limits for the scan below, so a rename in a huge workspace stays cheap.
const (
	renameScanMaxFiles    = 5000
	renameScanMaxFileSize = 1 << 20
	renameScanMaxReported = 8
)

// unrenamedMentions lists files of the renamed symbol's language that contain
// its name as a whole word but are not part of the rename edit.
//
// Language servers answer from the files they have loaded. A server that is
// still discovering the workspace (pyright answers its first request before it
// has read every file) returns a rename that is correct but incomplete, and
// nothing in the response says so. A file that mentions the name and is absent
// from the edit is the visible symptom: it is either a comment or string that
// was never going to be renamed, or a real reference the server did not see.
func unrenamedMentions(root, targetFile, symbol string, edit any) []string {
	name := lastNamePart(symbol)
	if root == "" || name == "" {
		return nil
	}
	ext := filepath.Ext(targetFile)
	if ext == "" {
		return nil
	}
	edited := map[string]bool{}
	if targets, err := collectRenameEdits(edit); err == nil {
		for _, t := range targets {
			edited[filepath.Clean(uripkg.URIToPath(t.URI))] = true
		}
	}
	word := regexp.MustCompile(`(^|[^\p{L}\p{N}_])` + regexp.QuoteMeta(name) + `([^\p{L}\p{N}_]|$)`)

	var missed []string
	visited := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if path != root && (skipDirs[n] || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ext || edited[filepath.Clean(path)] {
			return nil
		}
		if visited++; visited > renameScanMaxFiles {
			return filepath.SkipAll
		}
		if info, err := d.Info(); err != nil || info.Size() > renameScanMaxFileSize {
			return nil
		}
		if b, err := os.ReadFile(path); err == nil && word.Match(b) {
			missed = append(missed, path)
		}
		return nil
	})
	sort.Strings(missed)
	return missed
}

// lastNamePart returns the final dot-separated part of a cursor token:
// "self.prefix" -> "prefix".
func lastNamePart(token string) string {
	parts := strings.Split(token, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return ""
}

// unrenamedHint words the warning for files unrenamedMentions found.
func unrenamedHint(symbol string, missed []string) string {
	if len(missed) == 0 {
		return ""
	}
	shown := missed
	more := ""
	if len(shown) > renameScanMaxReported {
		more = " and " + itoa(len(missed)-renameScanMaxReported) + " more"
		shown = shown[:renameScanMaxReported]
	}
	return "WARNING: " + itoa(len(missed)) + " other file(s) mention " + lastNamePart(symbol) + " but are not part of this rename: " +
		strings.Join(shown, ", ") + more + ". If those are real references (not comments or strings), the language server had not loaded them yet: wait a moment and run rename_symbol again."
}
