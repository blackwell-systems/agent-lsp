package types_test

import (
	"testing"

	internaltypes "github.com/blackwell-systems/agent-lsp/internal/types"
	pubtypes "github.com/blackwell-systems/agent-lsp/pkg/types"
)

// TestPkgTypesAliasAssignability verifies that pkg/types re-exports are true type
// aliases of internal/types — values are directly assignable without conversion.
func TestPkgTypesAliasAssignability(t *testing.T) {
	t.Skip("compile smoke only — verifies type alias assignability")

	// Position: must be directly assignable (type alias, not distinct type).
	internalPos := internaltypes.Position{Line: 1, Character: 5}
	var pubPos pubtypes.Position = internalPos
	_ = pubPos

	// Range.
	internalRange := internaltypes.Range{
		Start: internaltypes.Position{Line: 0, Character: 0},
		End:   internaltypes.Position{Line: 0, Character: 10},
	}
	var pubRange pubtypes.Range = internalRange
	_ = pubRange

	// LSPDiagnostic.
	internalDiag := internaltypes.LSPDiagnostic{
		Range:    internalRange,
		Severity: 1,
		Message:  "test error",
	}
	var pubDiag pubtypes.LSPDiagnostic = internalDiag
	_ = pubDiag

	// TextEdit.
	internalEdit := internaltypes.TextEdit{
		Range:   internalRange,
		NewText: "hello",
	}
	var pubEdit pubtypes.TextEdit = internalEdit
	_ = pubEdit

	// ToolResult.
	internalResult := internaltypes.ToolResult{
		Content: []internaltypes.ContentItem{{Type: "text", Text: "ok"}},
		IsError: false,
	}
	var pubResult pubtypes.ToolResult = internalResult
	_ = pubResult

	// Constant aliases.
	var _ pubtypes.InlayHintKind = pubtypes.InlayHintKindType
	var _ pubtypes.DocumentHighlightKind = pubtypes.DocumentHighlightText
}
