package lsp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// TestApplyTextEditIfUnchanged_RefusesChangedFile reproduces issue #76: an
// edit planned against one version of a file, applied after another writer
// changed it, spliced at the stale position and lost bytes. It must now write
// nothing and leave the other writer's content intact.
func TestApplyTextEditIfUnchanged_RefusesChangedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	planned := "line1\nline2\nTARGET\nline4\n"
	if err := os.WriteFile(path, []byte(planned), 0644); err != nil {
		t.Fatal(err)
	}
	client := NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root)

	// The plan replaces "TARGET" (line 2). Another writer then prepends lines.
	rng := types.Range{Start: types.Position{Line: 2}, End: types.Position{Line: 2, Character: 6}}
	external := "INSERTED\nDONE\nline1\nline2\nTARGET\nline4\n"
	if err := os.WriteFile(path, []byte(external), 0644); err != nil {
		t.Fatal(err)
	}

	err := client.ApplyTextEditIfUnchanged(context.Background(), PathToFileURI(path), []byte(planned), rng, "REPLACED")
	if !errors.Is(err, ErrFileChanged) {
		t.Fatalf("err = %v, want ErrFileChanged", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != external {
		t.Fatalf("file = %q, want the other writer's content untouched", got)
	}
}

func TestApplyTextEditIfUnchanged_AppliesUnchangedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	src := "line1\nTARGET\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	client := NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root)

	rng := types.Range{Start: types.Position{Line: 1}, End: types.Position{Line: 1, Character: 6}}
	err := client.ApplyTextEditIfUnchanged(context.Background(), PathToFileURI(path), []byte(src), rng, "REPLACED")
	if err != nil && !isNotStartedErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "line1\nREPLACED\n" {
		t.Fatalf("file = %q", got)
	}
}
