package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blackwell-systems/agent-lsp/internal/types"
	uripkg "github.com/blackwell-systems/agent-lsp/internal/uri"
)

// renameEditTarget is one text edit from a rename WorkspaceEdit, normalized
// across the "changes" and "documentChanges" forms.
type renameEditTarget struct {
	URI     string
	Range   types.Range
	NewText string
}

// collectRenameEdits normalizes both WorkspaceEdit forms into a flat list.
// A malformed edit is an error, not an empty list: treating it as empty would
// let the guard pass while the server still applies a corrupt edit (issue #80).
func collectRenameEdits(result any) ([]renameEditTarget, error) {
	if result == nil {
		return nil, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode workspace edit: %w", err)
	}
	var parsed struct {
		Changes map[string][]struct {
			Range   types.Range `json:"range"`
			NewText string      `json:"newText"`
		} `json:"changes"`
		DocumentChanges []struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []struct {
				Range   types.Range `json:"range"`
				NewText string      `json:"newText"`
			} `json:"edits"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode workspace edit: %w", err)
	}
	var out []renameEditTarget
	for uri, edits := range parsed.Changes {
		for _, e := range edits {
			out = append(out, renameEditTarget{URI: uri, Range: e.Range, NewText: e.NewText})
		}
	}
	for _, dc := range parsed.DocumentChanges {
		for _, e := range dc.Edits {
			out = append(out, renameEditTarget{URI: dc.TextDocument.URI, Range: e.Range, NewText: e.NewText})
		}
	}
	return out, nil
}

// isRenameTokenRune reports whether r may appear in the spelling of the symbol
// under the cursor. The set is language neutral: letters (including CJK),
// digits, underscore, dollar, hyphen and dot cover Python, Lisp, CSS and
// qualified names such as self.prefix.
func isRenameTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		r == '_' || r == '$' || r == '-' || r == '.'
}

// renameTokenAt extracts the symbol token at the 1-indexed UTF-16 code-unit
// column col of a single source line. It returns "" when no token covers the
// position. utf16ColToByteOffset keeps the LSP-to-byte conversion in one place.
func renameTokenAt(line string, col int) string {
	byteOff, ok := utf16ColToByteOffset(line, col)
	if !ok || byteOff >= len(line) {
		return ""
	}
	runes := []rune(line)
	idx := utf8.RuneCountInString(line[:byteOff])
	if idx >= len(runes) || !isRenameTokenRune(runes[idx]) {
		return ""
	}
	start := idx
	for start > 0 && isRenameTokenRune(runes[start-1]) {
		start--
	}
	end := idx + 1
	for end < len(runes) && isRenameTokenRune(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

// renamedSymbolAt reads the symbol under a 1-indexed (line, col) from filePath,
// which must lie inside rootDir. col counts UTF-16 code units, matching the LSP
// position sent to the server. It returns "" when the file cannot be read.
func renamedSymbolAt(filePath, rootDir string, line, col int) string {
	path, err := uripkg.ValidatePath(filePath, rootDir)
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return renameTokenAt(lines[line-1], col)
}

// renameNameParts splits the cursor token into the names a rename edit may
// cover. The token can be qualified (self.prefix, pkg.Name), and the server
// edits only the last part, so any dot-separated part counts.
func renameNameParts(token string) []string {
	var parts []string
	for _, p := range strings.Split(token, ".") {
		if p != "" {
			parts = append(parts, strings.ToLower(p))
		}
	}
	return parts
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// renameSource is one file's text, split into lines once per validation.
type renameSource struct {
	content string
	lines   []string
}

// rangeText returns the text a range covers and whether the range sits inside
// an identifier: the character before its start or after its end is a word
// character, as when an edit covers only "foo" of "foo_bar".
func (f *renameSource) rangeText(rng types.Range) (text string, insideWord bool) {
	if rng.Start.Line == rng.End.Line && rng.Start.Line >= 0 && rng.Start.Line < len(f.lines) {
		line := f.lines[rng.Start.Line]
		from, okFrom := utf16ColToByteOffset(line, rng.Start.Character+1)
		to, okTo := utf16ColToByteOffset(line, rng.End.Character+1)
		if okFrom && okTo && from <= to {
			if from < to {
				if r, _ := utf8.DecodeLastRuneInString(line[:from]); from > 0 && isWordRune(r) {
					insideWord = true
				}
				if r, _ := utf8.DecodeRuneInString(line[to:]); to < len(line) && isWordRune(r) {
					insideWord = true
				}
			}
			return line[from:to], insideWord
		}
	}
	return uripkg.RangeText(f.content, rng), false
}

// validateRenameTargets rejects a WorkspaceEdit that would overwrite text that
// is not the symbol being renamed. Every edit that replaces existing text must
//
//   - cover text containing the old name, case-insensitively, so accessors such
//     as getFoo, qualified names, ~Foo and import paths all pass, and
//   - not start or end in the middle of an identifier, so an edit over the
//     first three characters of foo_bar is refused.
//
// A range that lands in prose or on a quote would otherwise overwrite
// docstrings and leave them unterminated (issue #80). Insertions (empty ranges)
// cannot destroy text and are allowed. When the old name is unknown the check
// is skipped. readFile must confine its reads to the workspace.
func validateRenameTargets(result any, oldName string, readFile func(uri string) (string, error)) error {
	targets, err := collectRenameEdits(result)
	if err != nil {
		return fmt.Errorf("rename_symbol: cannot validate workspace edit: %w", err)
	}
	parts := renameNameParts(oldName)
	if len(parts) == 0 {
		return nil
	}
	sources := map[string]*renameSource{}
	for _, target := range targets {
		src, ok := sources[target.URI]
		if !ok {
			content, err := readFile(target.URI)
			if err != nil {
				return fmt.Errorf("rename_symbol: cannot read %s to validate rename edits: %w", target.URI, err)
			}
			src = &renameSource{content: content, lines: strings.Split(content, "\n")}
			sources[target.URI] = src
		}
		original, insideWord := src.rangeText(target.Range)
		if original == "" {
			continue
		}
		lower := strings.ToLower(original)
		matches := false
		for _, p := range parts {
			if strings.Contains(lower, p) {
				matches = true
				break
			}
		}
		if !matches || insideWord {
			why := "does not contain the symbol"
			if matches {
				why = "cuts through the middle of an identifier"
			}
			return fmt.Errorf("rename_symbol: refusing to apply: the edit at %s:%d:%d would overwrite %q, which %s %q. The language server's view of the file may be out of date; nothing was written (issue #80)", target.URI, target.Range.Start.Line+1, target.Range.Start.Character+1, original, why, oldName)
		}
	}
	return nil
}
