package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestSymbolRefCache_PutAndGet(t *testing.T) {
	// Create a temp workspace with a real file.
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.go")
	os.WriteFile(testFile, []byte("package main\nfunc Foo() {}"), 0644)

	cache := NewSymbolRefCache(dir)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	defer cache.Close()

	// Initially empty.
	if got := cache.Get(testFile, "Foo", 2); got != nil {
		t.Error("expected nil for uncached symbol")
	}

	// Put and get.
	locs := []types.Location{
		{URI: "file:///other.go", Range: types.Range{Start: types.Position{Line: 10, Character: 0}}},
	}
	cache.Put(testFile, "Foo", 2, locs)

	got := cache.Get(testFile, "Foo", 2)
	if got == nil {
		t.Fatal("expected cached result")
	}
	if len(got.Locations) != 1 {
		t.Errorf("expected 1 location, got %d", len(got.Locations))
	}
	if got.Locations[0].URI != "file:///other.go" {
		t.Errorf("expected URI file:///other.go, got %s", got.Locations[0].URI)
	}
}

func TestSymbolRefCache_InvalidateFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.go")
	os.WriteFile(testFile, []byte("package main"), 0644)

	cache := NewSymbolRefCache(dir)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	defer cache.Close()

	cache.Put(testFile, "Foo", 1, []types.Location{{URI: "file:///a.go"}})

	// Verify it's cached.
	if got := cache.Get(testFile, "Foo", 1); got == nil {
		t.Fatal("expected cached result before invalidation")
	}

	// Invalidate.
	cache.InvalidateFile(testFile)

	// Should be gone.
	if got := cache.Get(testFile, "Foo", 1); got != nil {
		t.Error("expected nil after invalidation")
	}
}

func TestSymbolRefCache_StaleHash(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.go")
	os.WriteFile(testFile, []byte("package main\nfunc Foo() {}"), 0644)

	cache := NewSymbolRefCache(dir)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	defer cache.Close()

	// Cache with original content.
	cache.Put(testFile, "Foo", 2, []types.Location{{URI: "file:///a.go"}})

	// Modify the file (changes the hash).
	os.WriteFile(testFile, []byte("package main\nfunc Foo() { println() }"), 0644)

	// Cache should return nil (stale hash).
	if got := cache.Get(testFile, "Foo", 2); got != nil {
		t.Error("expected nil for stale file hash")
	}
}

func TestSymbolRefCache_NilSafe(t *testing.T) {
	var cache *SymbolRefCache
	// All methods should be no-ops on nil.
	cache.Put("/fake", "Foo", 1, nil)
	cache.InvalidateFile("/fake")
	if got := cache.Get("/fake", "Foo", 1); got != nil {
		t.Error("expected nil from nil cache")
	}
	entries, _ := cache.Stats()
	if entries != 0 {
		t.Errorf("expected 0 entries from nil cache, got %d", entries)
	}
	if err := cache.Close(); err != nil {
		t.Errorf("expected nil error from nil cache close, got %v", err)
	}
}

func TestSymbolRefCache_Stats(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.go")
	os.WriteFile(testFile, []byte("package main"), 0644)

	cache := NewSymbolRefCache(dir)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	defer cache.Close()

	entries, _ := cache.Stats()
	if entries != 0 {
		t.Errorf("expected 0 entries initially, got %d", entries)
	}

	cache.Put(testFile, "A", 1, []types.Location{})
	cache.Put(testFile, "B", 2, []types.Location{})

	entries, _ = cache.Stats()
	if entries != 2 {
		t.Errorf("expected 2 entries, got %d", entries)
	}
}
