package config

import (
	"path/filepath"
	"testing"
)

// TestInferWorkspaceRoot_Exported exercises the exported InferWorkspaceRoot
// wrapper to close the coverage gap on the public API.
func TestInferWorkspaceRoot_Exported(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "go.mod"))
	subdir := filepath.Join(root, "pkg")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "file.go")
	touch(t, filePath)

	gotRoot, gotLang, err := InferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("InferWorkspaceRoot: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root = %q, want %q", gotRoot, root)
	}
	if gotLang != "go" {
		t.Errorf("lang = %q, want %q", gotLang, "go")
	}
}

// TestInferWorkspaceRoot_Exported_NoMarker exercises the no-marker case
// via the exported API.
func TestInferWorkspaceRoot_Exported_NoMarker(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "empty")
	mkdirAll(t, subdir)

	gotRoot, gotLang, err := InferWorkspaceRoot(subdir)
	if err != nil {
		t.Fatalf("InferWorkspaceRoot: %v", err)
	}
	if gotRoot != "" {
		t.Errorf("root should be empty for no markers, got %q", gotRoot)
	}
	if gotLang != "" {
		t.Errorf("lang should be empty for no markers, got %q", gotLang)
	}
}
