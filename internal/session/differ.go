package session

import "github.com/blackwell-systems/agent-lsp/internal/types"

// DiagnosticsEqual reports whether two LSP diagnostics are semantically identical.
// Two diagnostics are equal if they have matching:
// - Range (start and end positions)
// - Message
// - Severity
// - Source (ignored if either diagnostic has an empty Source field)
func DiagnosticsEqual(a, b types.LSPDiagnostic) bool {
	// Compare range
	if a.Range.Start.Line != b.Range.Start.Line ||
		a.Range.Start.Character != b.Range.Start.Character ||
		a.Range.End.Line != b.Range.End.Line ||
		a.Range.End.Character != b.Range.End.Character {
		return false
	}

	// Compare message
	if a.Message != b.Message {
		return false
	}

	// Compare severity
	if a.Severity != b.Severity {
		return false
	}

	// Compare source (skip if either is empty)
	if a.Source != "" && b.Source != "" && a.Source != b.Source {
		return false
	}

	return true
}

// DiffDiagnostics computes introduced and resolved diagnostics between baseline and current.
// - introduced: diagnostics present in current but not matched in baseline
// - resolved: diagnostics present in baseline but not matched in current
// Returns DiagnosticEntry slices with 1-indexed line/column positions.
func DiffDiagnostics(baseline, current []types.LSPDiagnostic) (introduced, resolved []DiagnosticEntry) {
	// Find introduced diagnostics (in current but not in baseline)
	for _, curr := range current {
		found := false
		for _, base := range baseline {
			if DiagnosticsEqual(curr, base) {
				found = true
				break
			}
		}
		if !found {
			introduced = append(introduced, DiagnosticEntry{
				Line:     curr.Range.Start.Line + 1,      // Convert to 1-indexed
				Col:      curr.Range.Start.Character + 1, // Convert to 1-indexed
				Message:  curr.Message,
				Severity: SeverityString(curr.Severity),
				Source:   curr.Source,
			})
		}
	}

	// Find resolved diagnostics (in baseline but not in current)
	for _, base := range baseline {
		found := false
		for _, curr := range current {
			if DiagnosticsEqual(base, curr) {
				found = true
				break
			}
		}
		if !found {
			resolved = append(resolved, DiagnosticEntry{
				Line:     base.Range.Start.Line + 1,      // Convert to 1-indexed
				Col:      base.Range.Start.Character + 1, // Convert to 1-indexed
				Message:  base.Message,
				Severity: SeverityString(base.Severity),
				Source:   base.Source,
			})
		}
	}

	return introduced, resolved
}

// SeverityString converts an LSP severity int to a human-readable string.
// LSP severity values: 1=error, 2=warning, 3=info, 4=hint
func SeverityString(severity int) string {
	switch severity {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "unknown"
	}
}
