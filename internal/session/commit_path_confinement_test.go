package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// TestCommit_RejectsPathOutsideWorkspace verifies that Commit (apply=true)
// refuses to write a session's staged content to a fileURI that resolves
// outside the session's Workspace, and leaves any real file at that path
// untouched.
//
// Regression test: Commit previously converted session.Contents' fileURI keys
// straight to a path via internaluri.URIToPath and called os.WriteFile with no
// root-confinement check — an LLM agent tricked into staging an edit for a
// path like "../../.ssh/authorized_keys" could otherwise overwrite it here.
func TestCommit_RejectsPathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()

	secret := filepath.Join(outsideDir, "secret.txt")
	original := "do not touch me"
	if err := os.WriteFile(secret, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	traversal := filepath.Join(root, "..", filepath.Base(outsideDir), "secret.txt")
	fileURI := "file://" + traversal

	sess := &SimulationSession{
		ID:        "commit-traversal",
		Status:    StatusMutated,
		Workspace: root,
		Contents: map[string]string{
			fileURI: "PWNED",
		},
		Baselines:        make(map[string]DiagnosticsSnapshot),
		Versions:         make(map[string]int),
		OriginalContents: make(map[string]string),
	}
	mgr.mu.Lock()
	mgr.sessions[sess.ID] = sess
	mgr.mu.Unlock()

	_, err := mgr.Commit(ctx, sess.ID, "", true)
	if err == nil {
		t.Fatal("expected Commit to reject a fileURI outside the session workspace, got nil error")
	}

	got, readErr := os.ReadFile(secret)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != original {
		t.Fatalf("file outside workspace was modified: got %q, want unchanged %q", got, original)
	}
}

// TestApplyEdit_RejectsPathOutsideWorkspace verifies that ApplyEdit (which
// establishes a session's baseline by reading the target file from disk)
// refuses to read a fileURI outside the session's Workspace.
//
// Regression test: ApplyEdit previously called os.ReadFile on the raw
// URIToPath result with no root-confinement check, so a crafted fileURI could
// pull an arbitrary file's contents into the in-memory session (an information
// disclosure risk even before any write happens).
func TestApplyEdit_RejectsPathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()

	secret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret contents"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewSessionManager(&mockResolver{})
	ctx := context.Background()

	traversal := filepath.Join(root, "..", filepath.Base(outsideDir), "secret.txt")
	fileURI := "file://" + traversal

	sess := &SimulationSession{
		ID:               "applyedit-traversal",
		Status:           StatusCreated,
		Workspace:        root,
		Contents:         make(map[string]string),
		Baselines:        make(map[string]DiagnosticsSnapshot),
		Versions:         make(map[string]int),
		OriginalContents: make(map[string]string),
	}
	mgr.mu.Lock()
	mgr.sessions[sess.ID] = sess
	mgr.mu.Unlock()

	rng := types.Range{
		Start: types.Position{Line: 0, Character: 0},
		End:   types.Position{Line: 0, Character: 4},
	}
	_, err := mgr.ApplyEdit(ctx, sess.ID, fileURI, rng, "hack")
	if err == nil {
		t.Fatal("expected ApplyEdit to reject a fileURI outside the session workspace, got nil error")
	}
	if _, leaked := sess.Contents[fileURI]; leaked {
		t.Fatal("out-of-workspace file contents were staged into the session despite rejection")
	}
}
