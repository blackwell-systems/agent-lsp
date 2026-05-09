package lsp

import (
	"compress/gzip"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// writeFakeSource creates a temporary source file so that hashFile succeeds
// during cache Put operations. Returns the file path.
func writeFakeSource(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write fake source: %v", err)
	}
	return p
}

func TestCacheArtifactExportCreatesGzip(t *testing.T) {
	cache := newTestCache(t)
	defer cache.Close()

	srcDir := t.TempDir()
	fakeFile := writeFakeSource(t, srcDir, "fake.go", "package main\nfunc TestFunc() {}\n")

	cache.Put(fakeFile, "TestFunc", 10, []types.Location{
		{URI: "file://" + fakeFile, Range: types.Range{
			Start: types.Position{Line: 9, Character: 0},
			End:   types.Position{Line: 9, Character: 8},
		}},
	})

	destPath := filepath.Join(t.TempDir(), "cache.db.gz")
	if err := cache.ExportArtifact(destPath); err != nil {
		t.Fatalf("ExportArtifact failed: %v", err)
	}

	// Verify the file exists and is valid gzip.
	f, err := os.Open(destPath)
	if err != nil {
		t.Fatalf("failed to open exported file: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("exported file is not valid gzip: %v", err)
	}
	gz.Close()
}

func TestCacheArtifactImportRestoresData(t *testing.T) {
	srcDir := t.TempDir()
	fakeFile := writeFakeSource(t, srcDir, "fake.go", "package main\nfunc Hello() {}\n")

	// Create a cache with data and export it.
	cache1 := newTestCache(t)

	cache1.Put(fakeFile, "Hello", 5, []types.Location{
		{URI: "file://" + fakeFile, Range: types.Range{
			Start: types.Position{Line: 4, Character: 0},
			End:   types.Position{Line: 4, Character: 5},
		}},
	})

	artifactPath := filepath.Join(t.TempDir(), "export.db.gz")
	if err := cache1.ExportArtifact(artifactPath); err != nil {
		t.Fatalf("ExportArtifact failed: %v", err)
	}
	cache1.Close()

	// Create a fresh cache and import into it.
	cache2 := newTestCache(t)
	defer cache2.Close()

	if err := cache2.ImportArtifact(artifactPath); err != nil {
		t.Fatalf("ImportArtifact failed: %v", err)
	}

	// Query the db directly (Get checks file hash which is path-dependent).
	var count int
	cache2.mu.Lock()
	err := cache2.db.QueryRow(`SELECT COUNT(*) FROM symbol_refs WHERE symbol_name = ?`, "Hello").Scan(&count)
	cache2.mu.Unlock()
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported row, got %d", count)
	}
}

func TestCacheArtifactRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	fakeFile := writeFakeSource(t, srcDir, "fake.go", "package main\nfunc Foo() {}\nfunc Bar() {}\n")

	cache1 := newTestCache(t)

	locs := []types.Location{
		{URI: "file://" + fakeFile, Range: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 0, Character: 3},
		}},
		{URI: "file://" + fakeFile, Range: types.Range{
			Start: types.Position{Line: 10, Character: 0},
			End:   types.Position{Line: 10, Character: 3},
		}},
	}
	cache1.Put(fakeFile, "Foo", 1, locs)
	cache1.Put(fakeFile, "Bar", 2, locs[:1])

	entries1, _ := cache1.Stats()
	if entries1 != 2 {
		t.Fatalf("expected 2 entries before export, got %d", entries1)
	}

	// Export.
	artifactPath := filepath.Join(t.TempDir(), "roundtrip.db.gz")
	if err := cache1.ExportArtifact(artifactPath); err != nil {
		t.Fatalf("ExportArtifact failed: %v", err)
	}

	// Clear the database.
	cache1.mu.Lock()
	cache1.db.Exec(`DELETE FROM symbol_refs`)
	cache1.mu.Unlock()

	entries2, _ := cache1.Stats()
	if entries2 != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", entries2)
	}

	// Import.
	if err := cache1.ImportArtifact(artifactPath); err != nil {
		t.Fatalf("ImportArtifact failed: %v", err)
	}

	entries3, _ := cache1.Stats()
	if entries3 != 2 {
		t.Fatalf("expected 2 entries after import, got %d", entries3)
	}

	cache1.Close()
}

func TestCacheArtifactNilSafety(t *testing.T) {
	var cache *SymbolRefCache

	if err := cache.ExportArtifact("/tmp/nope.gz"); err == nil {
		t.Fatal("expected error for nil cache export")
	}

	if err := cache.ImportArtifact("/tmp/nope.gz"); err == nil {
		t.Fatal("expected error for nil cache import")
	}
}

// newTestCache creates a SymbolRefCache backed by a temporary SQLite database.
func newTestCache(t *testing.T) *SymbolRefCache {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_refs.db")

	db, err := openTestDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	return &SymbolRefCache{db: db, dbPath: dbPath}
}

func openTestDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS symbol_refs (
			file_path   TEXT NOT NULL,
			file_hash   TEXT NOT NULL,
			symbol_name TEXT NOT NULL,
			symbol_line INTEGER NOT NULL,
			locations   TEXT NOT NULL,
			cached_at   INTEGER NOT NULL,
			PRIMARY KEY (file_path, symbol_name, symbol_line)
		);
		CREATE INDEX IF NOT EXISTS idx_symbol_refs_file ON symbol_refs(file_path);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
