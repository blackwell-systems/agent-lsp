package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- hasUnopenedFiles (issue #42) ---

func TestHasUnopenedFiles_EmptyRoot(t *testing.T) {
	root := t.TempDir()
	if hasUnopenedFiles(root, 0) {
		t.Fatal("empty workspace should not report unopened files")
	}
}

func TestHasUnopenedFiles_EmptyRootPath(t *testing.T) {
	// No root (client without a workspace) — coverage cannot be established
	// against anything, so the helper must say "no unopened files" rather
	// than always caveating.
	if hasUnopenedFiles("", 0) {
		t.Fatal("empty root path should not report unopened files")
	}
}

func TestHasUnopenedFiles_AllOpened(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if hasUnopenedFiles(root, 2) {
		t.Fatal("all files opened should not report unopened files")
	}
}

func TestHasUnopenedFiles_MoreFilesThanOpened(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !hasUnopenedFiles(root, 2) {
		t.Fatal("3 files with 2 opened should report unopened files")
	}
}

func TestHasUnopenedFiles_SkipsDotAndSkipDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "b.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	vendored := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(vendored, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendored, "c.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 1 real file, opened once → no unopened files despite 3 on disk.
	if hasUnopenedFiles(root, 1) {
		t.Fatal("hidden and skipped dirs must not count as unopened files")
	}
}

func TestHasUnopenedFiles_Recursive(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 2 real files, only the sub file opened → the root-level file is unopened.
	if !hasUnopenedFiles(root, 1) {
		t.Fatal("files in subdirectories must count toward the workspace total")
	}
}

func TestHasUnopenedFiles_BoundedByCap(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxUnopenedProbeFiles+50; i++ {
		name := filepath.Join(root, strings.Repeat("f", 1)+string(rune('a'+i%26))+itoa(i)+".go")
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Even with opened >= seen at the cap boundary, exceeding the cap stops
	// the walk and reports conservatively when seen surpassed opened — here
	// opened=0 so it must report unopened either way.
	if !hasUnopenedFiles(root, 0) {
		t.Fatal("large workspace with nothing opened must report unopened files")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
