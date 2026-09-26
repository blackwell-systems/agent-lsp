package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseScopePaths_String(t *testing.T) {
	got := ParseScopePaths("src/lib")
	if len(got) != 1 || got[0] != "src/lib" {
		t.Errorf("expected [src/lib], got %v", got)
	}
}

func TestParseScopePaths_EmptyString(t *testing.T) {
	got := ParseScopePaths("")
	if got != nil {
		t.Errorf("expected nil for empty string, got %v", got)
	}
}

func TestParseScopePaths_StringSlice(t *testing.T) {
	input := []any{"src/a", "src/b", "src/c"}
	got := ParseScopePaths(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 paths, got %d", len(got))
	}
	if got[0] != "src/a" || got[1] != "src/b" || got[2] != "src/c" {
		t.Errorf("unexpected paths: %v", got)
	}
}

func TestParseScopePaths_SliceWithEmptyStrings(t *testing.T) {
	input := []any{"src/a", "", "src/c"}
	got := ParseScopePaths(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 paths (empty filtered), got %d", len(got))
	}
	if got[0] != "src/a" || got[1] != "src/c" {
		t.Errorf("unexpected paths: %v", got)
	}
}

func TestParseScopePaths_SliceWithNonStrings(t *testing.T) {
	input := []any{"src/a", 42, true, "src/b"}
	got := ParseScopePaths(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 paths (non-strings filtered), got %d", len(got))
	}
	if got[0] != "src/a" || got[1] != "src/b" {
		t.Errorf("unexpected paths: %v", got)
	}
}

func TestParseScopePaths_EmptySlice(t *testing.T) {
	input := []any{}
	got := ParseScopePaths(input)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestParseScopePaths_Nil(t *testing.T) {
	got := ParseScopePaths(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestParseScopePaths_JSONEncodedArray(t *testing.T) {
	// When scope is typed as string in the args struct but the client sends
	// a JSON array, Go unmarshals it as a stringified JSON array.
	got := ParseScopePaths(`["packages/some-project/src", "packages/other/src"]`)
	if len(got) != 2 {
		t.Fatalf("expected 2 paths from JSON array string, got %d: %v", len(got), got)
	}
	if got[0] != "packages/some-project/src" || got[1] != "packages/other/src" {
		t.Errorf("unexpected paths: %v", got)
	}
}

func TestParseScopePaths_JSONEncodedArraySingle(t *testing.T) {
	got := ParseScopePaths(`["src"]`)
	if len(got) != 1 || got[0] != "src" {
		t.Errorf("expected [src], got %v", got)
	}
}

func TestParseScopePaths_InvalidJSON(t *testing.T) {
	// Strings starting with [ but not valid JSON should be treated as a single path.
	got := ParseScopePaths("[not-json")
	if len(got) != 1 || got[0] != "[not-json" {
		t.Errorf("expected [[not-json]], got %v", got)
	}
}

func TestParseScopePaths_UnsupportedType(t *testing.T) {
	got := ParseScopePaths(42)
	if got != nil {
		t.Errorf("expected nil for int, got %v", got)
	}
}

func TestParseScopePaths_BoolType(t *testing.T) {
	got := ParseScopePaths(true)
	if got != nil {
		t.Errorf("expected nil for bool, got %v", got)
	}
}

func TestResolveOpenText_ProvidedTextWins(t *testing.T) {
	// Caller-provided content is used verbatim, even if the file on disk differs.
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte("disk content"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveOpenText(file, "caller content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "caller content" {
		t.Errorf("expected caller content verbatim, got %q", got)
	}
}

func TestResolveOpenText_EmptyReadsFromDisk(t *testing.T) {
	// Empty text means the caller omitted content: didOpen must carry the real
	// file content instead of an empty buffer (servers are not required to read
	// from disk, and some — mql-lsp-server — take the buffer literally).
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveOpenText(file, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "package main\n" {
		t.Errorf("expected disk content, got %q", got)
	}
}

func TestResolveOpenText_MissingFileErrors(t *testing.T) {
	if _, err := resolveOpenText(filepath.Join(t.TempDir(), "missing.go"), ""); err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
