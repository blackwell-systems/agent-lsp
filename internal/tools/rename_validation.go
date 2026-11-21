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

// isRenameTokenRune reports whether r may appear in a renamed symbol. The set
// is language neutral: letters (including CJK), digits, underscore, dollar,
// hyphen and dot cover Python, Lisp, CSS and qualified names. Whitespace,
// quotes and punctuation do not, so a range that landed in prose fails.
func isRenameTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		r == '_' || r == '$' || r == '-' || r == '.'
}

// isSingleRenameToken reports whether s is exactly one symbol spelling. It
// deliberately carries no language-specific identifier grammar; the server
// owns that grammar. Its only claim is that a symbol has no whitespace or
// quote characters, which distinguishes a reference from surrounding prose.
func isSingleRenameToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isRenameTokenRune(r) {
			return false
		}
	}
	return true
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

// renamedSymbolAt reads the symbol under a 1-indexed (line, col) from filePath.
// col counts UTF-16 code units, matching the LSP position sent to the server.
func renamedSymbolAt(filePath string, line, col int) string {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return renameTokenAt(lines[line-1], col)
}

// validateRenameTargets rejects a WorkspaceEdit whose edits do not all cover
// the symbol being renamed. Each edit must cover one token with no prose or
// quotes, all edits must cover the same text, and that text must be the symbol
// at the cursor when it can be resolved. A range in prose would otherwise
// overwrite docstrings and leave them unterminated (issue #80).
func validateRenameTargets(result any, oldName string, readFile func(uri string) (string, error)) error {
	targets, err := collectRenameEdits(result)
	if err != nil {
		return fmt.Errorf("rename_symbol: cannot validate workspace edit: %w", err)
	}
	var first string
	for i, target := range targets {
		content, err := readFile(target.URI)
		if err != nil {
			return fmt.Errorf("rename_symbol: cannot read %s to validate rename edits: %w", target.URI, err)
		}
		original := uripkg.RangeText(content, target.Range)
		if !isSingleRenameToken(original) {
			return fmt.Errorf("rename_symbol: refusing to apply: edit at %s:%d:%d would overwrite %q, which is not a single identifier token (issue #80)", target.URI, target.Range.Start.Line+1, target.Range.Start.Character+1, original)
		}
		if i == 0 {
			first = original
		} else if original != first {
			return fmt.Errorf("rename_symbol: refusing to apply: edits cover different text %q and %q, expected one renamed symbol (issue #80)", first, original)
		}
	}
	if first != "" && oldName != "" && first != oldName {
		return fmt.Errorf("rename_symbol: refusing to apply: edit covers %q but the symbol at the cursor is %q (issue #80)", first, oldName)
	}
	return nil
}
