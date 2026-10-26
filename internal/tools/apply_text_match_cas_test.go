package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// TestApplyTextMatch_ConcurrentWriteRefused is issue #76 end to end: a write
// landing between the text-match plan and the apply must not be spliced over.
func TestApplyTextMatch_ConcurrentWriteRefused(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nTARGET\nline4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	client := lsp.NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root)

	external := "INSERTED\nDONE\nline1\nline2\nTARGET\nline4\n"
	textMatchBeforeApply = func(p string) {
		if err := os.WriteFile(p, []byte(external), 0644); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { textMatchBeforeApply = nil })

	res := applyTextMatch(context.Background(), client, path, "TARGET", "REPLACED", nil)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "changed after the edit was planned") {
		t.Fatalf("result = %+v, want a file-changed refusal", res)
	}
	got, _ := os.ReadFile(path)
	if string(got) != external {
		t.Fatalf("file = %q, want the other writer's content untouched", got)
	}
}

// safe_apply_edit passes the content its preview checked; a file that changed
// after the preview must not be written.
func TestApplyTextMatch_RefusesContentOtherThanPreviewed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	current := "x := 1\ny := 2\n"
	if err := os.WriteFile(path, []byte(current), 0644); err != nil {
		t.Fatal(err)
	}
	client := lsp.NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root)

	res := applyTextMatch(context.Background(), client, path, "y := 2", "y := 3", []byte("y := 2\n"))
	if !res.IsError {
		t.Fatalf("result = %+v, want a refusal", res)
	}
	got, _ := os.ReadFile(path)
	if string(got) != current {
		t.Fatalf("file = %q, want unchanged", got)
	}
}
