package phase

import "testing"

func TestMatchToolPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		tool     string
		expected bool
	}{
		// Exact match.
		{"apply_edit", "apply_edit", true},
		{"apply_edit", "apply_edits", false},
		{"apply_edit", "apply_edi", false},

		// Wildcard suffix.
		{"simulate_*", "simulate_edit", true},
		{"simulate_*", "simulate_edit_atomic", true},
		{"simulate_*", "simulate_chain", true},
		{"simulate_*", "simulate_", true},
		{"simulate_*", "simulate", false},
		{"simulate_*", "get_diagnostics", false},

		// Single-char wildcard prefix.
		{"*", "anything", true},
		{"*", "", true},

		// No wildcard, no match.
		{"run_build", "run_tests", false},
		{"Edit", "Write", false},
	}

	for _, tt := range tests {
		got := MatchToolPattern(tt.pattern, tt.tool)
		if got != tt.expected {
			t.Errorf("MatchToolPattern(%q, %q) = %v, want %v", tt.pattern, tt.tool, got, tt.expected)
		}
	}
}

func TestMatchesAny(t *testing.T) {
	patterns := []string{"apply_edit", "simulate_*", "Edit"}

	if !MatchesAny(patterns, "apply_edit") {
		t.Error("expected apply_edit to match")
	}
	if !MatchesAny(patterns, "simulate_chain") {
		t.Error("expected simulate_chain to match via wildcard")
	}
	if !MatchesAny(patterns, "Edit") {
		t.Error("expected Edit to match")
	}
	if MatchesAny(patterns, "get_diagnostics") {
		t.Error("expected get_diagnostics to NOT match")
	}
	if MatchesAny(nil, "anything") {
		t.Error("expected nil patterns to not match")
	}
}
