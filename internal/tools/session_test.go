package tools

import (
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
