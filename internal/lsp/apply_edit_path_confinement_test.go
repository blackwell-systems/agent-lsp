package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestApplyWorkspaceEdit_RejectsPathOutsideRoot verifies that ApplyWorkspaceEdit
// (via applyEditsToFile, the "changes" map branch) refuses to write to a path
// outside the client's configured workspace root, even when the URI it is given
// resolves (after ../ traversal) to a real file outside that root.
//
// Regression test for the finding that apply_edit had no root-confinement check:
// an LLM agent steered by untrusted content (a malicious file, a poisoned
// dependency doc) could otherwise use apply_edit to overwrite arbitrary files
// with the same filesystem access as the developer's shell.
func TestApplyWorkspaceEdit_RejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir() // sibling temp dir, guaranteed outside root

	secret := filepath.Join(outsideDir, "secret.txt")
	original := "do not touch me\n"
	if err := os.WriteFile(secret, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	// Craft a traversal URI: root/../<outsideDir-basename>/secret.txt resolves
	// (after Abs+Clean) to the real secret file outside root.
	traversal := filepath.Join(root, "..", filepath.Base(outsideDir), "secret.txt")
	uri := PathToFileURI(traversal)

	client := NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root) // simulate an initialized client scoped to root

	edit := map[string]any{
		"changes": map[string]any{
			uri: []any{
				textEditJSON(0, 0, 0, len(original)-1, "PWNED"),
			},
		},
	}

	err := client.ApplyWorkspaceEdit(context.Background(), edit)
	if err == nil {
		t.Fatal("expected ApplyWorkspaceEdit to reject a path outside the workspace root, got nil error")
	}

	got, readErr := os.ReadFile(secret)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != original {
		t.Fatalf("file outside workspace root was modified: got %q, want unchanged %q", got, original)
	}
}

// TestApplyWorkspaceEdit_DocumentChanges_RejectsCreateOutsideRoot verifies the
// documentChanges "create" branch also enforces root confinement.
func TestApplyWorkspaceEdit_DocumentChanges_RejectsCreateOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "backdoor.sh")

	traversal := filepath.Join(root, "..", filepath.Base(outsideDir), "backdoor.sh")
	uri := PathToFileURI(traversal)

	client := NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root)

	edit := map[string]any{
		"documentChanges": []any{
			map[string]any{
				"kind": "create",
				"uri":  uri,
			},
		},
	}

	err := client.ApplyWorkspaceEdit(context.Background(), edit)
	if err == nil {
		t.Fatal("expected ApplyWorkspaceEdit to reject a create outside the workspace root, got nil error")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("file outside workspace root was created at %s", target)
	}
}

// TestApplyWorkspaceEdit_AllowsPathInsideRoot is the positive-case control: a
// well-formed edit to a file genuinely inside the root must still succeed once
// root confinement is enforced, so the fix doesn't just reject everything.
func TestApplyWorkspaceEdit_AllowsPathInsideRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	src := "package main\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	uri := PathToFileURI(path)

	client := NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root)

	edit := map[string]any{
		"changes": map[string]any{
			uri: []any{
				textEditJSON(0, 8, 0, 12, "other"),
			},
		},
	}

	if err := client.ApplyWorkspaceEdit(context.Background(), edit); err != nil && !isNotStartedErr(err) {
		t.Fatalf("ApplyWorkspaceEdit: unexpected error for in-root path: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package other\n" {
		t.Fatalf("in-root edit did not apply: got %q", got)
	}
}
