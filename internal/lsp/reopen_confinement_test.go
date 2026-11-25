package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ReopenDocument opens an untracked URI from disk. That read must stay inside
// the workspace like every other file read driven by a client-supplied path.
func TestReopenDocument_UntrackedPathOutsideRootIsRefused(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("TOPSECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewLSPClient("unused", nil)
	c.SetRootDirForTest(t.TempDir())
	err := c.ReopenDocument(context.Background(), PathToFileURI(outside))
	if err == nil || !strings.Contains(err.Error(), "outside workspace root") {
		t.Fatalf("want 'outside workspace root', got %v", err)
	}
}
