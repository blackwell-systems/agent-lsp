// differ.go computes the diagnostic delta between a session's baseline
// and the current LSP state. This is the core of speculative execution:
// the net_delta (introduced minus resolved) tells the agent whether an
// edit made things better (negative), worse (positive), or neutral (zero).
//
// Comparison uses DiagnosticsEqual which matches on range + message + severity.
// Source is only compared when both diagnostics have a non-empty Source field,
// because some language servers omit Source on certain diagnostic kinds.
package session

import (
	"fmt"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

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

// diagnosticFingerprint returns a stable string key for d using Range,
// Message, and Severity. Source is excluded to match DiagnosticsEqual
// semantics (Source ignored when either diagnostic's Source is empty).
func diagnosticFingerprint(d types.LSPDiagnostic) string {
	return fmt.Sprintf("%d\x00%d\x00%d\x00%d\x00%s\x00%d",
		d.Range.Start.Line,
		d.Range.Start.Character,
		d.Range.End.Line,
		d.Range.End.Character,
		d.Message,
		d.Severity,
	)
}

// DiffDiagnostics computes introduced and resolved diagnostics between baseline and current.
// - introduced: diagnostics present in current but not matched in baseline
// - resolved: diagnostics present in baseline but not matched in current
// Returns DiagnosticEntry slices with 1-indexed line/column positions.
func DiffDiagnostics(baseline, current []types.LSPDiagnostic) (introduced, resolved []DiagnosticEntry) {
	// Filter to errors and warnings only. Info (3) and hint (4) diagnostics
	// are style suggestions that should not affect net_delta or safe-to-apply
	// decisions. Including them causes confusing deltas when hints shift
	// due to unrelated code changes.
	baseline = filterSignificant(baseline)
	current = filterSignificant(current)

	// Build a count map from baseline for O(1) membership tests.
	baseCount := make(map[string]int, len(baseline))
	for _, d := range baseline {
		baseCount[diagnosticFingerprint(d)]++
	}

	// Find introduced: present in current but count exhausted in baseline.
	remaining := make(map[string]int, len(baseCount))
	for k, v := range baseCount {
		remaining[k] = v
	}
	for _, d := range current {
		fp := diagnosticFingerprint(d)
		if remaining[fp] > 0 {
			remaining[fp]--
		} else {
			introduced = append(introduced, DiagnosticEntry{
				Line:     d.Range.Start.Line + 1,
				Col:      d.Range.Start.Character + 1,
				Message:  d.Message,
				Severity: SeverityString(d.Severity),
				Source:   d.Source,
			})
		}
	}

	// Find resolved: present in baseline but count exhausted in current.
	baseRemaining := make(map[string]int, len(baseCount))
	for k, v := range baseCount {
		baseRemaining[k] = v
	}
	for _, d := range current {
		fp := diagnosticFingerprint(d)
		if baseRemaining[fp] > 0 {
			baseRemaining[fp]--
		}
	}
	for _, d := range baseline {
		fp := diagnosticFingerprint(d)
		if baseRemaining[fp] > 0 {
			baseRemaining[fp]--
			resolved = append(resolved, DiagnosticEntry{
				Line:     d.Range.Start.Line + 1,
				Col:      d.Range.Start.Character + 1,
				Message:  d.Message,
				Severity: SeverityString(d.Severity),
				Source:   d.Source,
			})
		}
	}

	return introduced, resolved
}

// filterSignificant returns only errors (severity 1) and warnings (severity 2).
// Info and hint diagnostics are excluded from net_delta calculations.
func filterSignificant(diags []types.LSPDiagnostic) []types.LSPDiagnostic {
	out := make([]types.LSPDiagnostic, 0, len(diags))
	for _, d := range diags {
		if d.Severity <= 2 {
			out = append(out, d)
		}
	}
	return out
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
