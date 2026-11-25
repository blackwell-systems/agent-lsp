package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// secretOutside creates a workspace and a file outside it that contains
// "TOPSECRET". Neither is ever read by a correct implementation when the path
// comes from a tool argument.
func secretOutside(t *testing.T) (client *lsp.LSPClient, root, outside string) {
	t.Helper()
	root = t.TempDir()
	outside = filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("line one\nTOPSECRET value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client = lsp.NewLSPClient("unused", nil)
	client.SetRootDirForTest(root)
	return client, root, outside
}

func mustRefuse(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "outside workspace root") {
		t.Fatalf("%s: want an 'outside workspace root' error, got %v", what, err)
	}
}

// A position_pattern is matched against the file's text. For a path outside the
// workspace the pattern must not be matched at all: "found" versus "not found"
// would reveal the file's contents.
func TestExtractPositionWithPattern_RefusesPathOutsideRoot(t *testing.T) {
	_, root, outside := secretOutside(t)
	for _, pattern := range []string{"@@TOPSECRET", "@@NOT-IN-THE-FILE"} {
		_, _, err := ExtractPositionWithPattern(map[string]any{"position_pattern": pattern}, outside, root)
		mustRefuse(t, "pattern "+pattern, err)
	}
}

func TestExtractPositionWithPattern_RefusesWithoutRoot(t *testing.T) {
	_, _, outside := secretOutside(t)
	_, _, err := ExtractPositionWithPattern(map[string]any{"position_pattern": "@@TOPSECRET"}, outside, "")
	if err == nil || !strings.Contains(err.Error(), "start_lsp") {
		t.Fatalf("want a 'call start_lsp first' error, got %v", err)
	}
}

func TestExtractPositionWithPattern_InsideRootStillWorks(t *testing.T) {
	_, root, _ := secretOutside(t)
	inside := filepath.Join(root, "f.txt")
	if err := os.WriteFile(inside, []byte("alpha beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	line, col, err := ExtractPositionWithPattern(map[string]any{"position_pattern": "alpha @@beta"}, inside, root)
	if err != nil || line != 1 || col != 7 {
		t.Fatalf("inside root = (%d,%d,%v), want (1,7,nil)", line, col, err)
	}
}

// A symlink inside the workspace that points outside it is outside.
func TestExtractPositionWithPattern_RefusesSymlinkOutOfRoot(t *testing.T) {
	_, root, outside := secretOutside(t)
	link := filepath.Join(root, "innocent.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable: ", err)
	}
	_, _, err := ExtractPositionWithPattern(map[string]any{"position_pattern": "@@TOPSECRET"}, link, root)
	mustRefuse(t, "symlink to an outside file", err)
}

// The tools that resolve position_pattern before opening the document.
func TestPositionPatternTools_RefuseOutsidePath(t *testing.T) {
	client, _, outside := secretOutside(t)
	args := func(extra map[string]any) map[string]any {
		m := map[string]any{"file_path": outside, "position_pattern": "@@TOPSECRET", "language_id": "plaintext"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	for name, call := range map[string]func() (string, bool){
		"get_references": func() (string, bool) {
			r, _ := HandleGetReferences(context.Background(), client, args(nil))
			return r.Content[0].Text, r.IsError
		},
		"inspect_symbol": func() (string, bool) {
			r, _ := HandleGetInfoOnLocation(context.Background(), client, args(nil))
			return r.Content[0].Text, r.IsError
		},
		"rename_symbol": func() (string, bool) {
			r, _ := HandleRenameSymbol(context.Background(), client, args(map[string]any{"new_name": "x"}))
			return r.Content[0].Text, r.IsError
		},
	} {
		text, isErr := call()
		if !isErr || !strings.Contains(text, "outside workspace root") {
			t.Errorf("%s: want an 'outside workspace root' error, got isError=%v %q", name, isErr, text)
		}
		if strings.Contains(text, "TOPSECRET") {
			t.Errorf("%s: response quotes the outside file: %q", name, text)
		}
	}
}

func TestSafeApplyEdit_RefusesPathOutsideRoot(t *testing.T) {
	client, _, outside := secretOutside(t)
	// safe_apply_edit reads the file to locate old_text. Whether the text is in
	// the file or not, the answer must be the same refusal, never "not found"
	// (which would reveal the contents of a file outside the workspace).
	for _, oldText := range []string{"TOPSECRET", "NOT-IN-THE-FILE"} {
		r, _ := HandleSafeApplyEdit(context.Background(), client, nil, map[string]any{
			"file_path": outside, "old_text": oldText, "new_text": "x",
		})
		if !r.IsError || !strings.Contains(r.Content[0].Text, "outside workspace root") {
			t.Fatalf("old_text %q: want 'outside workspace root', got isError=%v %q", oldText, r.IsError, r.Content[0].Text)
		}
	}
}

func TestSimulateEditAtomic_RefusesFileOutsideWorkspaceRoot(t *testing.T) {
	_, root, outside := secretOutside(t)
	_, err := simulateEditAtomicCore(context.Background(), nil, map[string]any{
		"workspace_root": root, "language": "plaintext", "file_path": outside,
		"start_line": 1.0, "start_column": 1.0, "end_line": 1.0, "end_column": 2.0, "new_text": "x",
	})
	mustRefuse(t, "simulate_edit_atomic", err)
}

// export_cache creates directories and writes a file; import_cache reads one.
func TestCacheTools_RefusePathOutsideRoot(t *testing.T) {
	client, root, outside := secretOutside(t)
	target := filepath.Join(filepath.Dir(outside), "sub", "stolen.db.gz")

	r, _ := HandleExportCache(context.Background(), client, map[string]any{"dest_path": target})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "outside workspace root") {
		t.Fatalf("export_cache: want refusal, got isError=%v %q", r.IsError, r.Content[0].Text)
	}
	if _, err := os.Stat(filepath.Dir(target)); err == nil {
		t.Fatal("export_cache created a directory outside the workspace")
	}

	r, _ = HandleImportCache(context.Background(), client, map[string]any{"src_path": outside})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "outside workspace root") {
		t.Fatalf("import_cache: want refusal, got isError=%v %q", r.IsError, r.Content[0].Text)
	}

	// Relative paths are taken relative to the workspace root and are allowed.
	if p, err := confineCachePath(client, ".agent-lsp/cache.db.gz"); err != nil || p != filepath.Join(resolvedRoot(t, root), ".agent-lsp", "cache.db.gz") {
		t.Fatalf("relative cache path = %q, %v", p, err)
	}
	// ".." cannot climb out of the root.
	if _, err := confineCachePath(client, "../escape.db.gz"); err == nil {
		t.Fatal("a relative path that climbs out of the root was accepted")
	}
}

func resolvedRoot(t *testing.T, root string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
