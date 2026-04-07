package config

import (
	"os"
	"path/filepath"
	"testing"
)

// helper creates a file at path with empty content.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

// mkdirAll creates directories and fails the test on error.
func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatalf("mkdirAll %s: %v", path, err)
	}
}

func TestInferWorkspaceRoot_GoMod(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "go.mod"))
	subdir := filepath.Join(root, "pkg")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "file.go")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "go" {
		t.Errorf("languageID: got %q, want %q", gotLang, "go")
	}
}

func TestInferWorkspaceRoot_PackageJSON(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "package.json"))
	subdir := filepath.Join(root, "src")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "index.ts")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "typescript" {
		t.Errorf("languageID: got %q, want %q", gotLang, "typescript")
	}
}

func TestInferWorkspaceRoot_CargoToml(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "Cargo.toml"))
	subdir := filepath.Join(root, "src")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "main.rs")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "rust" {
		t.Errorf("languageID: got %q, want %q", gotLang, "rust")
	}
}

func TestInferWorkspaceRoot_Pyproject(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "pyproject.toml"))
	subdir := filepath.Join(root, "mypackage")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "main.py")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "python" {
		t.Errorf("languageID: got %q, want %q", gotLang, "python")
	}
}

func TestInferWorkspaceRoot_SetupPy(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "setup.py"))
	subdir := filepath.Join(root, "mypackage")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "utils.py")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "python" {
		t.Errorf("languageID: got %q, want %q", gotLang, "python")
	}
}

func TestInferWorkspaceRoot_GitFallback(t *testing.T) {
	root := t.TempDir()
	// Create .git as a directory (no other markers).
	mkdirAll(t, filepath.Join(root, ".git"))
	subdir := filepath.Join(root, "src")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "main.go")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "" {
		t.Errorf("languageID: got %q, want %q", gotLang, "")
	}
}

func TestInferWorkspaceRoot_Priority_GoModOverGit(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "go.mod"))
	mkdirAll(t, filepath.Join(root, ".git"))
	filePath := filepath.Join(root, "main.go")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "go" {
		t.Errorf("languageID: got %q, want %q (go.mod should win over .git)", gotLang, "go")
	}
}

func TestInferWorkspaceRoot_NoMarker(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "file.txt")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != "" {
		t.Errorf("root: got %q, want %q", gotRoot, "")
	}
	if gotLang != "" {
		t.Errorf("languageID: got %q, want %q", gotLang, "")
	}
}

func TestInferWorkspaceRoot_FileAtRoot(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "go.mod"))
	filePath := filepath.Join(root, "main.go")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "go" {
		t.Errorf("languageID: got %q, want %q", gotLang, "go")
	}
}

func TestInferWorkspaceRoot_MarkerInParent(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "go.mod"))
	// Deeply nested: root/a/b/c/
	deep := filepath.Join(root, "a", "b", "c")
	mkdirAll(t, deep)
	filePath := filepath.Join(deep, "deep.go")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "go" {
		t.Errorf("languageID: got %q, want %q", gotLang, "go")
	}
}
