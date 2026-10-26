package session

import (
	"context"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// TestEvaluate_NoFreshDiagnostics_ReportsTimeout covers the safe_apply_edit
// bug where a server that had not yet published diagnostics for the edit left
// the pre-edit cache in place: the wait returned nil on its deadline, Evaluate
// reported net_delta 0 with high confidence, and the unverified edit was
// written to disk. With no publication the result must be a timeout with
// partial confidence, which safe_apply_edit refuses.
func TestEvaluate_NoFreshDiagnostics_ReportsTimeout(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	uri := "file:///tmp/agent-lsp-evaluate-timeout/main.go"
	sess := &SimulationSession{
		ID:     "evaluate-timeout",
		Status: StatusMutated,
		// gopls republishes after every change, so silence means the edit
		// was not analysed. No process is started: Evaluate only waits on
		// and reads the diagnostics cache.
		Client:           lsp.NewLSPClient("gopls", nil),
		Baselines:        map[string]DiagnosticsSnapshot{uri: {URI: uri, Confidence: ConfidenceHigh}},
		Versions:         map[string]int{uri: 1},
		Contents:         map[string]string{uri: "package main\n"},
		OriginalContents: map[string]string{uri: "package main\n"},
	}
	mgr.mu.Lock()
	mgr.sessions[sess.ID] = sess
	mgr.mu.Unlock()

	res, err := mgr.Evaluate(context.Background(), sess.ID, "file", 300)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !res.Timeout {
		t.Errorf("Timeout = false, want true: no diagnostics were published for the edit")
	}
	if res.Confidence != ConfidencePartial {
		t.Errorf("Confidence = %q, want %q", res.Confidence, ConfidencePartial)
	}
}
