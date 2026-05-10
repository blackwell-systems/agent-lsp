package session

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestDiagnosticsEqual_Match(t *testing.T) {
	a := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}
	b := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}

	if !DiagnosticsEqual(a, b) {
		t.Error("expected identical diagnostics to be equal")
	}
}

func TestDiagnosticsEqual_DifferentMessage(t *testing.T) {
	a := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}
	b := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "different error",
		Source:   "gopls",
	}

	if DiagnosticsEqual(a, b) {
		t.Error("expected diagnostics with different messages to be unequal")
	}
}

func TestDiagnosticsEqual_DifferentRange(t *testing.T) {
	a := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}
	b := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 11, Character: 5},
			End:   types.Position{Line: 11, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}

	if DiagnosticsEqual(a, b) {
		t.Error("expected diagnostics with different ranges to be unequal")
	}
}

func TestDiagnosticsEqual_SourceIgnoredWhenAbsent(t *testing.T) {
	// Test case 1: a has source, b is empty
	a := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}
	b := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "",
	}

	if !DiagnosticsEqual(a, b) {
		t.Error("expected diagnostics with one empty source to be equal")
	}

	// Test case 2: a is empty, b has source
	a2 := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "",
	}
	b2 := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}

	if !DiagnosticsEqual(a2, b2) {
		t.Error("expected diagnostics with one empty source to be equal (reversed)")
	}

	// Test case 3: both have different sources (should not match)
	a3 := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "gopls",
	}
	b3 := types.LSPDiagnostic{
		Range: types.Range{
			Start: types.Position{Line: 10, Character: 5},
			End:   types.Position{Line: 10, Character: 15},
		},
		Severity: 1,
		Message:  "undefined variable",
		Source:   "eslint",
	}

	if DiagnosticsEqual(a3, b3) {
		t.Error("expected diagnostics with different sources to be unequal")
	}
}

func TestDiffDiagnostics_NewErrors(t *testing.T) {
	baseline := []types.LSPDiagnostic{}
	current := []types.LSPDiagnostic{
		{
			Range: types.Range{
				Start: types.Position{Line: 5, Character: 10},
				End:   types.Position{Line: 5, Character: 20},
			},
			Severity: 1,
			Message:  "new error",
			Source:   "gopls",
		},
	}

	introduced, resolved := DiffDiagnostics(baseline, current)

	if len(introduced) != 1 {
		t.Errorf("expected 1 introduced error, got %d", len(introduced))
	}
	if len(resolved) != 0 {
		t.Errorf("expected 0 resolved errors, got %d", len(resolved))
	}

	if introduced[0].Line != 6 || introduced[0].Col != 11 {
		t.Errorf("expected 1-indexed position (6,11), got (%d,%d)", introduced[0].Line, introduced[0].Col)
	}
	if introduced[0].Message != "new error" {
		t.Errorf("expected message 'new error', got %q", introduced[0].Message)
	}
	if introduced[0].Severity != "error" {
		t.Errorf("expected severity 'error', got %q", introduced[0].Severity)
	}
}

func TestDiffDiagnostics_ResolvedErrors(t *testing.T) {
	baseline := []types.LSPDiagnostic{
		{
			Range: types.Range{
				Start: types.Position{Line: 5, Character: 10},
				End:   types.Position{Line: 5, Character: 20},
			},
			Severity: 1,
			Message:  "old error",
			Source:   "gopls",
		},
	}
	current := []types.LSPDiagnostic{}

	introduced, resolved := DiffDiagnostics(baseline, current)

	if len(introduced) != 0 {
		t.Errorf("expected 0 introduced errors, got %d", len(introduced))
	}
	if len(resolved) != 1 {
		t.Errorf("expected 1 resolved error, got %d", len(resolved))
	}

	if resolved[0].Line != 6 || resolved[0].Col != 11 {
		t.Errorf("expected 1-indexed position (6,11), got (%d,%d)", resolved[0].Line, resolved[0].Col)
	}
	if resolved[0].Message != "old error" {
		t.Errorf("expected message 'old error', got %q", resolved[0].Message)
	}
}

func TestDiffDiagnostics_NoChange(t *testing.T) {
	baseline := []types.LSPDiagnostic{
		{
			Range: types.Range{
				Start: types.Position{Line: 5, Character: 10},
				End:   types.Position{Line: 5, Character: 20},
			},
			Severity: 1,
			Message:  "same error",
			Source:   "gopls",
		},
	}
	current := []types.LSPDiagnostic{
		{
			Range: types.Range{
				Start: types.Position{Line: 5, Character: 10},
				End:   types.Position{Line: 5, Character: 20},
			},
			Severity: 1,
			Message:  "same error",
			Source:   "gopls",
		},
	}

	introduced, resolved := DiffDiagnostics(baseline, current)

	if len(introduced) != 0 {
		t.Errorf("expected 0 introduced errors, got %d", len(introduced))
	}
	if len(resolved) != 0 {
		t.Errorf("expected 0 resolved errors, got %d", len(resolved))
	}
}

func TestDiffDiagnostics_Mixed(t *testing.T) {
	baseline := []types.LSPDiagnostic{
		{
			Range: types.Range{
				Start: types.Position{Line: 5, Character: 10},
				End:   types.Position{Line: 5, Character: 20},
			},
			Severity: 1,
			Message:  "old error 1",
			Source:   "gopls",
		},
		{
			Range: types.Range{
				Start: types.Position{Line: 10, Character: 5},
				End:   types.Position{Line: 10, Character: 15},
			},
			Severity: 2,
			Message:  "warning that stays",
			Source:   "gopls",
		},
		{
			Range: types.Range{
				Start: types.Position{Line: 20, Character: 1},
				End:   types.Position{Line: 20, Character: 10},
			},
			Severity: 1,
			Message:  "old error 2",
			Source:   "gopls",
		},
	}
	current := []types.LSPDiagnostic{
		{
			Range: types.Range{
				Start: types.Position{Line: 10, Character: 5},
				End:   types.Position{Line: 10, Character: 15},
			},
			Severity: 2,
			Message:  "warning that stays",
			Source:   "gopls",
		},
		{
			Range: types.Range{
				Start: types.Position{Line: 15, Character: 8},
				End:   types.Position{Line: 15, Character: 18},
			},
			Severity: 1,
			Message:  "new error 1",
			Source:   "gopls",
		},
		{
			Range: types.Range{
				Start: types.Position{Line: 25, Character: 2},
				End:   types.Position{Line: 25, Character: 12},
			},
			Severity: 3,
			Message:  "new info",
			Source:   "gopls",
		},
	}

	introduced, resolved := DiffDiagnostics(baseline, current)

	// Only errors (severity 1) and warnings (severity 2) count toward delta.
	// Info (severity 3) is filtered out.
	if len(introduced) != 1 {
		t.Errorf("expected 1 introduced error (info filtered), got %d", len(introduced))
	}
	if len(resolved) != 2 {
		t.Errorf("expected 2 resolved errors, got %d", len(resolved))
	}

	// Check introduced diagnostics (only "new error 1", not "new info")
	if len(introduced) > 0 && introduced[0].Message != "new error 1" {
		t.Errorf("introduced message doesn't match: got %q, want %q", introduced[0].Message, "new error 1")
	}

	// Check resolved diagnostics
	if len(resolved) >= 2 {
		if resolved[0].Message != "old error 1" || resolved[1].Message != "old error 2" {
			t.Errorf("resolved messages don't match: %q, %q", resolved[0].Message, resolved[1].Message)
		}
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		severity int
		want     string
	}{
		{1, "error"},
		{2, "warning"},
		{3, "info"},
		{4, "hint"},
		{0, "unknown"},
		{5, "unknown"},
		{-1, "unknown"},
	}

	for _, tt := range tests {
		got := SeverityString(tt.severity)
		if got != tt.want {
			t.Errorf("SeverityString(%d) = %q, want %q", tt.severity, got, tt.want)
		}
	}
}
