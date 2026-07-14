package session

import "testing"

// TestEvalConfidence covers the confidence decision matrix, in particular the
// "clean result while the server is still indexing" downgrade to ConfidenceLow
// that keeps a still-priming server (e.g. rust-analyzer) from reporting a false
// all-clear.
func TestEvalConfidence(t *testing.T) {
	cases := []struct {
		name           string
		scope          string
		timedOut       bool
		netDelta       int
		activeProgress bool
		want           Confidence
	}{
		{"clean, ready, file scope", "file", false, 0, false, ConfidenceHigh},
		{"errors found, ready", "file", false, 2, false, ConfidenceHigh},
		{"workspace scope, clean, ready", "workspace", false, 0, false, ConfidenceEventual},
		{"timed out, clean, ready", "file", true, 0, false, ConfidencePartial},
		// The load-bearing case: no new errors but the server is still indexing.
		{"clean, still indexing", "file", false, 0, true, ConfidenceLow},
		{"clean, still indexing, negative delta", "file", false, -1, true, ConfidenceLow},
		// Errors were surfaced: trustworthy even while indexing (errors do not
		// appear spuriously), so not downgraded.
		{"errors found while indexing", "file", false, 3, true, ConfidenceHigh},
		// Active progress wins over a timeout for a clean result.
		{"clean, timed out, still indexing", "file", true, 0, true, ConfidenceLow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evalConfidence(tc.scope, tc.timedOut, tc.netDelta, tc.activeProgress)
			if got != tc.want {
				t.Errorf("evalConfidence(%q, timedOut=%v, netDelta=%d, activeProgress=%v) = %q, want %q",
					tc.scope, tc.timedOut, tc.netDelta, tc.activeProgress, got, tc.want)
			}
		})
	}
}
