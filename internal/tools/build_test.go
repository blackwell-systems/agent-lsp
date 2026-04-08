package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandleRunBuild_MissingWorkspaceDir verifies that an empty args map
// returns an error result requiring workspace_dir.
func TestHandleRunBuild_MissingWorkspaceDir(t *testing.T) {
	r, err := HandleRunBuild(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for missing workspace_dir")
	}
}

// TestHandleRunTests_MissingWorkspaceDir verifies that an empty args map
// returns an error result requiring workspace_dir.
func TestHandleRunTests_MissingWorkspaceDir(t *testing.T) {
	r, err := HandleRunTests(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for missing workspace_dir")
	}
}

// TestHandleGetTestsForFile_MissingFilePath verifies that an empty args map
// returns an error result requiring file_path.
func TestHandleGetTestsForFile_MissingFilePath(t *testing.T) {
	r, err := HandleGetTestsForFile(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for missing file_path")
	}
}

// TestHandleRunBuild_UnsupportedLanguage verifies that specifying an unknown
// language returns an error result.
func TestHandleRunBuild_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	r, err := HandleRunBuild(context.Background(), map[string]interface{}{
		"workspace_dir": dir,
		"language":      "cobol",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for unsupported language")
	}
	if !strings.Contains(r.Content[0].Text, "cobol") {
		t.Errorf("expected error message to contain 'cobol', got: %s", r.Content[0].Text)
	}
}

// TestHandleRunTests_UnsupportedLanguage verifies that specifying an unknown
// language returns an error result.
func TestHandleRunTests_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	r, err := HandleRunTests(context.Background(), map[string]interface{}{
		"workspace_dir": dir,
		"language":      "cobol",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for unsupported language")
	}
	if !strings.Contains(r.Content[0].Text, "cobol") {
		t.Errorf("expected error message to contain 'cobol', got: %s", r.Content[0].Text)
	}
}

// TestFindTestFiles_Go verifies that FindTestFiles finds *_test.go in the same
// directory as the source file.
func TestFindTestFiles_Go(t *testing.T) {
	dir := t.TempDir()

	// Create main.go and main_test.go.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testFilePath := filepath.Join(dir, "main_test.go")
	if err := os.WriteFile(testFilePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FindTestFiles(context.Background(), dir, "go", filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, tf := range result.TestFiles {
		if tf == testFilePath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main_test.go in TestFiles, got: %v", result.TestFiles)
	}
}

// TestFindTestFiles_Go_NoTests verifies that FindTestFiles returns an empty
// list when no *_test.go files exist.
func TestFindTestFiles_Go_NoTests(t *testing.T) {
	dir := t.TempDir()

	// Create only main.go.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FindTestFiles(context.Background(), dir, "go", filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.TestFiles) != 0 {
		t.Errorf("expected empty TestFiles, got: %v", result.TestFiles)
	}
}

// TestLocationNormalization verifies that a TestFailure with File and Line set
// gets a correctly normalized LSP Location after normalizeTestFailureLocation.
func TestLocationNormalization(t *testing.T) {
	root := t.TempDir()

	tf := &TestFailure{
		TestName: "TestSomething",
		File:     "pkg_test.go",
		Line:     42,
		Message:  "assertion failed",
	}

	normalizeTestFailureLocation(root, tf)

	if tf.Location == nil {
		t.Fatal("expected Location to be set")
	}
	if !strings.HasPrefix(tf.Location.URI, "file://") {
		t.Errorf("expected URI to start with 'file://', got: %s", tf.Location.URI)
	}
	if tf.Location.Range.Start.Line != 41 {
		t.Errorf("expected Start.Line=41 (0-indexed from line 42), got: %d", tf.Location.Range.Start.Line)
	}
}

// TestBuildResultShape verifies that RunBuild returns a BuildResult with the
// expected shape when run against a valid Go module directory.
func TestBuildResultShape(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal Go module.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RunBuild(context.Background(), dir, "go", "./...")
	if err != nil {
		t.Fatalf("unexpected error from RunBuild: %v", err)
	}

	if !result.Success {
		t.Errorf("expected Success=true for valid Go module, got false. Raw: %s", result.Raw)
	}

	// Verify Raw is a string (may be empty).
	_ = result.Raw // always a string in Go

	// Verify the shape can be marshaled and unmarshaled.
	data, mErr := json.Marshal(result)
	if mErr != nil {
		t.Fatalf("marshal error: %v", mErr)
	}
	var roundTrip BuildResult
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if roundTrip.Success != result.Success {
		t.Errorf("Success mismatch after round-trip")
	}
}
