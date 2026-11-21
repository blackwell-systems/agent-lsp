package tools

import (
	"encoding/json"
	"fmt"
	"unicode"

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
func collectRenameEdits(result any) []renameEditTarget {
	if result == nil {
		return nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil
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
		return nil
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
	return out
}

// isIdentifierToken reports whether s is exactly one language-neutral
// identifier: a letter, underscore, or dollar sign followed by letters,
// digits, underscores, or dollar signs. Unicode letters are allowed so real
// CJK identifiers remain renameable.
func isIdentifierToken(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r == '$':
		case unicode.IsLetter(r):
		case unicode.IsDigit(r) && i > 0:
		default:
			return false
		}
	}
	return true
}

// validateRenameTargets rejects a WorkspaceEdit whose ranges do not each cover
// exactly one identifier in the current file. A server returning offsets that
// land in prose would otherwise overwrite docstrings and leave unterminated
// strings behind (issue #80).
func validateRenameTargets(result any, readFile func(uri string) (string, error)) error {
	for _, target := range collectRenameEdits(result) {
		content, err := readFile(target.URI)
		if err != nil {
			return fmt.Errorf("rename_symbol: cannot read %s to validate rename edits: %w", target.URI, err)
		}
		original := uripkg.RangeText(content, target.Range)
		if !isIdentifierToken(original) {
			return fmt.Errorf("rename_symbol: refusing to apply: edit at %s:%d:%d would overwrite %q, which is not a single identifier token (issue #80)", target.URI, target.Range.Start.Line+1, target.Range.Start.Character+1, original)
		}
	}
	return nil
}
