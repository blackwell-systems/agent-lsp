package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// mockResolver is a test resolver that returns nil client.
type mockResolver struct{}

func (m *mockResolver) ClientForFile(filePath string) *lsp.LSPClient {
	return nil
}

func (m *mockResolver) DefaultClient() *lsp.LSPClient {
	return nil
}

func (m *mockResolver) AllClients() []*lsp.LSPClient {
	return nil
}

func (m *mockResolver) Shutdown(ctx context.Context) error {
	return nil
}

// TestNewSessionManager verifies that NewSessionManager creates a non-nil manager.
func TestNewSessionManager(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	if mgr == nil {
		t.Fatal("expected non-nil SessionManager")
	}
	if mgr.sessions == nil {
		t.Error("expected sessions map to be initialized")
	}
	if mgr.executor == nil {
		t.Error("expected executor to be initialized")
	}
}

// TestCreateSession_ReturnsID verifies that CreateSession returns an error when resolver returns nil client.
func TestCreateSession_ReturnsID(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	_, err := mgr.CreateSession(ctx, "/tmp/workspace", "go")
	if err == nil {
		t.Fatal("expected error when client is nil, got nil")
	}
	if err.Error() != "no LSP client available — call start_lsp first" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

// TestGetSession_NotFound verifies that GetSession returns an error for unknown session ID.
func TestGetSession_NotFound(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})

	_, err := mgr.GetSession("nonexistent-session-id")
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
	if err.Error() != "session not found: nonexistent-session-id" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

// TestApplyRangeEdit_SingleLine tests replacing text in the middle of a line.
func TestApplyRangeEdit_SingleLine(t *testing.T) {
	content := "hello world"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 6},
		End:   types.Position{Line: 0, Character: 11},
	}
	newText := "Go"

	result := applyRangeEdit(content, rng, newText)
	expected := "hello Go"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestApplyRangeEdit_MultiLine tests replacing text across multiple lines.
func TestApplyRangeEdit_MultiLine(t *testing.T) {
	content := "line 1\nline 2\nline 3"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 5},
		End:   types.Position{Line: 2, Character: 4},
	}
	newText := "replaced"

	result := applyRangeEdit(content, rng, newText)
	expected := "line replaced 3"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestApplyRangeEdit_Insert tests inserting text with an empty range (start == end).
func TestApplyRangeEdit_Insert(t *testing.T) {
	content := "hello world"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 5},
		End:   types.Position{Line: 0, Character: 5},
	}
	newText := " beautiful"

	result := applyRangeEdit(content, rng, newText)
	expected := "hello beautiful world"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestApplyRangeEdit_Delete tests deleting text by replacing with empty newText.
func TestApplyRangeEdit_Delete(t *testing.T) {
	content := "hello world"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 5},
		End:   types.Position{Line: 0, Character: 11},
	}
	newText := ""

	result := applyRangeEdit(content, rng, newText)
	expected := "hello"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestSessionDirtyState_BlocksCommit verifies that commit fails when session is dirty.
func TestSessionDirtyState_BlocksCommit(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})

	// Manually create a session in dirty state.
	session := &SimulationSession{
		ID:        "test-session",
		Status:    StatusDirty,
		DirtyErr:  nil,
		Baselines: make(map[string]DiagnosticsSnapshot),
		Versions:  make(map[string]int),
		Contents:  make(map[string]string),
	}

	mgr.mu.Lock()
	mgr.sessions["test-session"] = session
	mgr.mu.Unlock()

	ctx := context.Background()
	_, err := mgr.Commit(ctx, "test-session", "workspace", false)
	if err == nil {
		t.Fatal("expected error when committing dirty session, got nil")
	}
	// The error should mention the status.
	if err.Error() != "session test-session cannot be committed in state dirty" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

// TestEvaluate_InvalidStatus verifies that Evaluate fails when session is not
// in mutated or evaluated state.
func TestEvaluate_InvalidStatus(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	for _, status := range []SessionStatus{StatusCreated, StatusCommitted, StatusDiscarded} {
		t.Run(string(status), func(t *testing.T) {
			sess := &SimulationSession{
				ID:        "test-" + string(status),
				Status:    status,
				Baselines: make(map[string]DiagnosticsSnapshot),
				Versions:  make(map[string]int),
				Contents:  make(map[string]string),
			}
			mgr.mu.Lock()
			mgr.sessions[sess.ID] = sess
			mgr.mu.Unlock()

			_, err := mgr.Evaluate(ctx, sess.ID, "file", 0)
			if err == nil {
				t.Errorf("expected error for status %s, got nil", status)
			}
		})
	}
}

// TestCommit_InvalidStatus verifies that Commit fails when session is not in
// mutated or evaluated state.
func TestCommit_InvalidStatus(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	for _, status := range []SessionStatus{StatusCreated, StatusCommitted, StatusDiscarded, StatusDirty} {
		t.Run(string(status), func(t *testing.T) {
			sess := &SimulationSession{
				ID:        "commit-" + string(status),
				Status:    status,
				Baselines: make(map[string]DiagnosticsSnapshot),
				Versions:  make(map[string]int),
				Contents:  make(map[string]string),
			}
			mgr.mu.Lock()
			mgr.sessions[sess.ID] = sess
			mgr.mu.Unlock()

			_, err := mgr.Commit(ctx, sess.ID, "", false)
			if err == nil {
				t.Errorf("expected error for status %s, got nil", status)
			}
		})
	}
}

// TestCommit_ApplyFalse_BuildsPatch verifies that Commit with apply=false
// returns a patch map without writing to disk.
func TestCommit_ApplyFalse_BuildsPatch(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	sess := &SimulationSession{
		ID:     "commit-patch",
		Status: StatusMutated,
		Contents: map[string]string{
			"file:///tmp/test.go": "package main\n",
		},
		Baselines: make(map[string]DiagnosticsSnapshot),
		Versions:  make(map[string]int),
	}
	mgr.mu.Lock()
	mgr.sessions[sess.ID] = sess
	mgr.mu.Unlock()

	result, err := mgr.Commit(ctx, sess.ID, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FilesWritten != 0 {
		t.Errorf("expected 0 files written for apply=false, got %d", result.FilesWritten)
	}
	patch, ok := result.Patch.(map[string]string)
	if !ok {
		t.Fatalf("expected patch to be map[string]string, got %T", result.Patch)
	}
	if len(patch) != 1 {
		t.Errorf("expected 1 entry in patch, got %d", len(patch))
	}
	if sess.Status != StatusCommitted {
		t.Errorf("expected session status StatusCommitted after commit, got %s", sess.Status)
	}
}

// TestDiscard_TerminalSessionBlocked verifies that Discard fails when session
// is already in a terminal state.
func TestDiscard_TerminalSessionBlocked(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	for _, status := range []SessionStatus{StatusCommitted, StatusDiscarded, StatusDestroyed} {
		t.Run(string(status), func(t *testing.T) {
			sess := &SimulationSession{
				ID:        "discard-" + string(status),
				Status:    status,
				Baselines: make(map[string]DiagnosticsSnapshot),
				Versions:  make(map[string]int),
				Contents:  make(map[string]string),
			}
			mgr.mu.Lock()
			mgr.sessions[sess.ID] = sess
			mgr.mu.Unlock()

			err := mgr.Discard(ctx, sess.ID)
			if err == nil {
				t.Errorf("expected error for terminal status %s, got nil", status)
			}
		})
	}
}

// TestDiscard_NoOriginalContents_Skips verifies that Discard skips files
// that have no OriginalContents snapshot (no edit was applied).
func TestDiscard_NoOriginalContents_Skips(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	sess := &SimulationSession{
		ID:     "discard-no-orig",
		Status: StatusMutated,
		Contents: map[string]string{
			"file:///tmp/test.go": "modified content\n",
		},
		OriginalContents: make(map[string]string), // empty — no snapshot
		Baselines:        make(map[string]DiagnosticsSnapshot),
		Versions:         make(map[string]int),
		// Client is nil — if Discard tries to call OpenDocument it will panic.
		// The test verifies Discard skips files with no OriginalContents,
		// so OpenDocument should never be called.
	}
	mgr.mu.Lock()
	mgr.sessions[sess.ID] = sess
	mgr.mu.Unlock()

	// This should succeed without calling Client.OpenDocument
	// because OriginalContents is empty.
	err := mgr.Discard(ctx, sess.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if sess.Status != StatusDiscarded {
		t.Errorf("expected StatusDiscarded, got %s", sess.Status)
	}
}

// TestSimulateChain_EmptyEdits_ReturnsZero verifies SimulateChain with no
// edits returns an empty result with SafeToApplyThroughStep == 0.
func TestSimulateChain_EmptyEdits_ReturnsZero(t *testing.T) {
	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	sess := &SimulationSession{
		ID:               "chain-empty",
		Status:           StatusMutated,
		Baselines:        make(map[string]DiagnosticsSnapshot),
		Versions:         make(map[string]int),
		Contents:         make(map[string]string),
		OriginalContents: make(map[string]string),
	}
	mgr.mu.Lock()
	mgr.sessions[sess.ID] = sess
	mgr.mu.Unlock()

	result, err := mgr.SimulateChain(ctx, sess.ID, []ChainEdit{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SafeToApplyThroughStep != 0 {
		t.Errorf("expected SafeToApplyThroughStep=0 for empty chain, got %d", result.SafeToApplyThroughStep)
	}
	if result.CumulativeDelta != 0 {
		t.Errorf("expected CumulativeDelta=0, got %d", result.CumulativeDelta)
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(result.Steps))
	}
}

// TestSetStatus_IsThreadSafe verifies that SetStatus uses the mutex.
// This is a structural test: MarkDirty also acquires mu, and calling both
// concurrently should not deadlock or race.
func TestSetStatus_IsThreadSafe(t *testing.T) {
	sess := &SimulationSession{
		ID:     "race-test",
		Status: StatusMutated,
	}

	done := make(chan struct{})
	go func() {
		sess.SetStatus(StatusEvaluating)
		close(done)
	}()
	sess.MarkDirty(fmt.Errorf("concurrent dirty"))
	<-done
	// No assertion needed — race detector will catch unsynchronized access.
}

// TestUriToPath_PercentDecoded verifies that uriToPath correctly decodes
// percent-encoded characters in file URIs.
func TestUriToPath_PercentDecoded(t *testing.T) {
	cases := []struct {
		uri      string
		expected string
	}{
		{"file:///tmp/normal.go", "/tmp/normal.go"},
		{"file:///tmp/path%20with%20spaces/file.go", "/tmp/path with spaces/file.go"},
		{"file:///tmp/%E6%97%A5%E6%9C%AC%E8%AA%9E.go", "/tmp/\u65e5\u672c\u8a9e.go"},
	}

	for _, tc := range cases {
		t.Run(tc.uri, func(t *testing.T) {
			got := uriToPath(tc.uri)
			if got != tc.expected {
				t.Errorf("uriToPath(%q) = %q, want %q", tc.uri, got, tc.expected)
			}
		})
	}
}
