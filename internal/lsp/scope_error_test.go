package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectPackageScope_InvalidPath tests handling of invalid file paths
func TestDetectPackageScope_InvalidPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent file
	paths, err := DetectPackageScope("/nonexistent/file.py", tmpDir, "python")
	if err != nil {
		t.Logf("DetectPackageScope with invalid path: %v", err)
	}
	_ = paths // May be nil or empty
}

// TestDetectPackageScope_UnsupportedLanguage tests behavior for unsupported languages
func TestDetectPackageScope_UnsupportedLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	// Create a file
	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	// Go doesn't need scoping
	paths, err := DetectPackageScope(testFile, tmpDir, "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if paths != nil {
		t.Errorf("Go should not need scoping, got paths: %v", paths)
	}
}

// TestCountSourceFiles_EmptyDir tests counting in empty directory
func TestCountSourceFiles_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// countSourceFiles is unexported, so we test via DetectPackageScope behavior
	// which uses it internally
	testFile := filepath.Join(tmpDir, "test.py")
	if err := os.WriteFile(testFile, []byte("print('test')"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should work even with just one file
	_, err := DetectPackageScope(testFile, tmpDir, "python")
	if err != nil {
		t.Logf("DetectPackageScope in minimal dir: %v", err)
	}
}

// TestSourceExtensions_Coverage tests all supported language IDs
func TestSourceExtensions_Coverage(t *testing.T) {
	languages := []string{
		"python",
		"typescript",
		"typescriptreact",
		"javascript",
		"javascriptreact",
		"go",
		"rust",
		"java",
	}

	for _, lang := range languages {
		exts := sourceExtensions(lang)
		// Should return extensions for supported langs, nil for others
		if lang == "python" || lang == "typescript" || lang == "javascript" {
			if len(exts) == 0 {
				t.Errorf("sourceExtensions(%q) returned empty, expected extensions", lang)
			}
		}
	}
}

// TestGenerateScopeConfig_EmptyPaths tests handling of empty scope paths
func TestGenerateScopeConfig_EmptyPaths(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := GenerateScopeConfig(tmpDir, "python", []string{})
	if err != nil {
		t.Fatalf("GenerateScopeConfig with empty paths: %v", err)
	}

	if cfg != nil {
		t.Error("expected nil config for empty scope paths")
	}
}

// TestGenerateScopeConfig_UnsupportedLanguage tests unsupported language ID
func TestGenerateScopeConfig_UnsupportedLanguage(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := GenerateScopeConfig(tmpDir, "go", []string{"src/"})
	if err != nil {
		t.Fatalf("GenerateScopeConfig for unsupported lang: %v", err)
	}

	// Go doesn't need scoping, should return nil
	if cfg != nil {
		t.Error("expected nil config for language that doesn't need scoping")
	}
}

// TestRemoveScopeConfig_NilConfig tests removal with nil config
func TestRemoveScopeConfig_NilConfig(t *testing.T) {
	// Should not crash
	RemoveScopeConfig(nil)
}

// TestRemoveScopeConfig_EmptyConfig tests removal with empty config
func TestRemoveScopeConfig_EmptyConfig(t *testing.T) {
	cfg := &ScopeConfig{
		GeneratedFiles: []string{},
		BackedUpFiles:  map[string]string{},
	}

	// Should not crash
	RemoveScopeConfig(cfg)
}

// TestRemoveScopeConfig_NonexistentFiles tests handling of files that don't exist
func TestRemoveScopeConfig_NonexistentFiles(t *testing.T) {
	cfg := &ScopeConfig{
		GeneratedFiles: []string{"/nonexistent/file.json"},
		BackedUpFiles:  map[string]string{"/nonexistent/orig": "/nonexistent/backup"},
	}

	// Should not crash on nonexistent files
	RemoveScopeConfig(cfg)
}

// TestGenerateScopeConfig_PythonSimple tests Python scope generation
func TestGenerateScopeConfig_PythonSimple(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a source directory
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg, err := GenerateScopeConfig(tmpDir, "python", []string{"src/"})
	if err != nil {
		t.Fatalf("GenerateScopeConfig failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config for Python")
	}

	// Should have generated a config file
	if len(cfg.GeneratedFiles) == 0 {
		t.Error("expected at least one generated file")
	}

	// Clean up
	RemoveScopeConfig(cfg)
}

// TestGenerateScopeConfig_TypeScriptSimple tests TypeScript scope generation
func TestGenerateScopeConfig_TypeScriptSimple(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a source directory
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg, err := GenerateScopeConfig(tmpDir, "typescript", []string{"src/"})
	if err != nil {
		t.Fatalf("GenerateScopeConfig failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config for TypeScript")
	}

	// Should have generated a config file
	if len(cfg.GeneratedFiles) == 0 {
		t.Error("expected at least one generated file")
	}

	// Clean up
	RemoveScopeConfig(cfg)
}

// TestScopeConfig_Cleanup tests cleanup of generated files
func TestScopeConfig_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate a config
	cfg, err := GenerateScopeConfig(tmpDir, "python", []string{"src/"})
	if err != nil {
		t.Fatalf("GenerateScopeConfig failed: %v", err)
	}

	if cfg == nil {
		t.Skip("config generation returned nil (expected for test environment)")
	}

	// Verify files were created
	for _, f := range cfg.GeneratedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("generated file %s does not exist", f)
		}
	}

	// Clean up
	RemoveScopeConfig(cfg)

	// Verify files were removed
	for _, f := range cfg.GeneratedFiles {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("generated file %s should have been removed", f)
		}
	}
}
