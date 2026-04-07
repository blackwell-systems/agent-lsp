package session

import (
	"context"
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
	if err.Error() != "no LSP client available" {
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
