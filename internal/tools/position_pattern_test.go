package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestResolvePositionPattern_Basic(t *testing.T) {
	// "hello world" is 11 chars; @@ is after the 11th char so col=12
	content := "hello worldend"
	path := writeTemp(t, content)

	line, col, err := ResolvePositionPattern(path, "hello world@@end")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 1 {
		t.Errorf("line: got %d, want 1", line)
	}
	if col != 12 {
		t.Errorf("col: got %d, want 12", col)
	}
}

func TestResolvePositionPattern_MultiLine(t *testing.T) {
	// Line 1: "first\n" (6 bytes including \n)
	// Line 2: "second line\n" — cursor after "second " (7 chars into line 2)
	// Line 3: "third\n"
	content := "first\nsecond line\nthird\n"
	path := writeTemp(t, content)

	// pattern: "second @@line" -> cursor is at 's' of "line" on line 2, col 8
	line, col, err := ResolvePositionPattern(path, "second @@line")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 2 {
		t.Errorf("line: got %d, want 2", line)
	}
	if col != 8 {
		t.Errorf("col: got %d, want 8", col)
	}
}

func TestResolvePositionPattern_NoMarker(t *testing.T) {
	path := writeTemp(t, "hello world")

	_, _, err := ResolvePositionPattern(path, "hello world")
	if err == nil {
		t.Fatal("expected error for pattern without @@, got nil")
	}
}

func TestResolvePositionPattern_NotFound(t *testing.T) {
	path := writeTemp(t, "hello world")

	_, _, err := ResolvePositionPattern(path, "goodbye@@world")
	if err == nil {
		t.Fatal("expected error when pattern not found, got nil")
	}
}

func TestResolvePositionPattern_MarkerAtStart(t *testing.T) {
	// cursor at the very start of "firstword" -> col=1
	content := "firstword rest"
	path := writeTemp(t, content)

	line, col, err := ResolvePositionPattern(path, "@@firstword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 1 {
		t.Errorf("line: got %d, want 1", line)
	}
	if col != 1 {
		t.Errorf("col: got %d, want 1", col)
	}
}

func TestExtractPositionWithPattern_WithPattern(t *testing.T) {
	content := "package main\nfunc foo() {}\n"
	path := writeTemp(t, content)

	args := map[string]interface{}{
		"position_pattern": "func @@foo",
	}
	line, col, err := ExtractPositionWithPattern(args, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "func " is 5 chars on line 2; cursor at 'f' of "foo" -> col=6
	if line != 2 {
		t.Errorf("line: got %d, want 2", line)
	}
	if col != 6 {
		t.Errorf("col: got %d, want 6", col)
	}
}

func TestExtractPositionWithPattern_Fallback(t *testing.T) {
	args := map[string]interface{}{
		"line":   float64(5),
		"column": float64(3),
	}
	line, col, err := ExtractPositionWithPattern(args, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 5 {
		t.Errorf("line: got %d, want 5", line)
	}
	if col != 3 {
		t.Errorf("col: got %d, want 3", col)
	}
}

func TestResolvePositionPattern_FileNotFound(t *testing.T) {
	_, _, err := ResolvePositionPattern("/nonexistent/path/file.go", "foo@@bar")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

// TestExtractPositionWithPattern_NeitherPatternNorPosition verifies that when
// neither position_pattern nor line/column are provided, an error is returned.
func TestExtractPositionWithPattern_NeitherPatternNorPosition(t *testing.T) {
	_, _, err := ExtractPositionWithPattern(map[string]interface{}{}, "")
	if err == nil {
		t.Fatal("expected error when no position information provided, got nil")
	}
}
