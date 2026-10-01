package lsp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// --- Restart document replay (issue #3B) ---

// After a restart the fresh server must be brought back to the previous
// session's open-document state: servers like mql-lsp-server drive their
// workspace symbol index from opened documents only, so without the replay
// workspace queries stay empty until a document is reopened by hand.
func TestReplayOpenDocuments_ReopensPreviousSessionDocs(t *testing.T) {
	c, _, clientR := newTestClient(t)
	// OpenDocument writes didOpen notifications to the client→server pipe;
	// io.Pipe is synchronous, so drain it or the replay blocks forever.
	go func() { _, _ = io.Copy(io.Discard, clientR) }()
	root := t.TempDir()

	a := filepath.Join(root, "a.mq4")
	b := filepath.Join(root, "inc.mqh")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("// content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := []docMeta{
		{filePath: a, languageID: "mql", version: 1},
		{filePath: b, languageID: "mql", version: 3},
	}

	replayed := c.replayOpenDocuments(context.Background(), snapshot, root)
	if replayed != 2 {
		t.Fatalf("expected 2 documents re-opened, got %d", replayed)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.openDocs) != 2 {
		t.Fatalf("expected openDocs repopulated with 2 entries, got %d", len(c.openDocs))
	}
}

// Documents outside the new workspace root must not be replayed: they belong
// to the outgoing workspace, and re-opening them into the fresh root would
// reintroduce state the restart was supposed to leave behind.
func TestReplayOpenDocuments_SkipsOutsideNewRoot(t *testing.T) {
	c, _, _ := newTestClient(t)
	oldRoot := t.TempDir()
	newRoot := t.TempDir()

	outside := filepath.Join(oldRoot, "other.mq4")
	if err := os.WriteFile(outside, []byte("// content"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot := []docMeta{{filePath: outside, languageID: "mql", version: 1}}

	replayed := c.replayOpenDocuments(context.Background(), snapshot, newRoot)
	if replayed != 0 {
		t.Fatalf("expected 0 documents re-opened for outside-root doc, got %d", replayed)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.openDocs) != 0 {
		t.Fatalf("expected openDocs empty after outside-root skip, got %d", len(c.openDocs))
	}
}

// Missing files must be skipped without failing the restart.
func TestReplayOpenDocuments_SkipsMissingFiles(t *testing.T) {
	c, _, _ := newTestClient(t)
	root := t.TempDir()
	snapshot := []docMeta{{filePath: filepath.Join(root, "gone.mq4"), languageID: "mql", version: 1}}

	replayed := c.replayOpenDocuments(context.Background(), snapshot, root)
	if replayed != 0 {
		t.Fatalf("expected 0 for missing file, got %d", replayed)
	}
}

// --- withinRoot ---

func TestWithinRoot(t *testing.T) {
	root := t.TempDir()
	if !withinRoot(filepath.Join(root, "a.go"), root) {
		t.Error("file inside root should be within")
	}
	if !withinRoot(root, root) {
		t.Error("root itself should be within")
	}
	if withinRoot(filepath.Join(root, "..", "escape.go"), root) {
		t.Error("file outside root should not be within")
	}
}
