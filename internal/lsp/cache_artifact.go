// cache_artifact.go provides export/import for the symbol reference cache.
//
// ExportArtifact compacts the SQLite database and writes a gzip-compressed
// copy to the destination path. ImportArtifact reads a gzip-compressed
// database file, replaces the current cache, and validates integrity.
//
// These operations enable team-shared cache artifacts: one developer exports
// the cache, commits the .gz file, and teammates import it to skip cold-start
// indexing.
package lsp

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// ExportArtifact compacts the cache database and writes a gzip-compressed
// copy to destPath. The current database connection remains open and usable.
func (c *SymbolRefCache) ExportArtifact(destPath string) error {
	if c == nil {
		return fmt.Errorf("cache is nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db == nil {
		return fmt.Errorf("cache database is not open")
	}

	// Create a compacted copy via VACUUM INTO.
	tmpPath := destPath + ".tmp.db"
	defer os.Remove(tmpPath)

	_, err := c.db.Exec(`VACUUM INTO ?`, tmpPath)
	if err != nil {
		// Fallback: copy the database file directly.
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache_artifact: VACUUM INTO failed, falling back to file copy: %v", err))
		if cpErr := copyFile(c.dbPath, tmpPath); cpErr != nil {
			return fmt.Errorf("failed to copy database: %w", cpErr)
		}
	}

	// Read the compacted copy and compress with gzip.
	src, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to open compacted db: %w", err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	gz := gzip.NewWriter(dst)
	if _, err := io.Copy(gz, src); err != nil {
		return fmt.Errorf("failed to compress database: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to finalize gzip: %w", err)
	}

	return nil
}

// ImportArtifact reads a gzip-compressed database from srcPath, replaces the
// current cache database, and validates integrity. The existing database
// connection is closed and reopened after import.
func (c *SymbolRefCache) ImportArtifact(srcPath string) error {
	if c == nil {
		return fmt.Errorf("cache is nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dbPath == "" {
		return fmt.Errorf("cache database path is not set")
	}

	// Read and decompress the gzipped file.
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open artifact: %w", err)
	}
	defer src.Close()

	gz, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	// Write decompressed data to a temporary file first.
	tmpPath := c.dbPath + ".import.tmp"
	defer os.Remove(tmpPath)

	tmp, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tmp, gz); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to decompress artifact: %w", err)
	}
	tmp.Close()

	// Validate the decompressed database before replacing.
	testDB, err := sql.Open("sqlite", tmpPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("failed to open imported database for validation: %w", err)
	}

	var result string
	if err := testDB.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		testDB.Close()
		return fmt.Errorf("integrity check query failed: %w", err)
	}
	testDB.Close()

	if result != "ok" {
		return fmt.Errorf("imported database failed integrity check: %s", result)
	}

	// Close the current database connection.
	if c.db != nil {
		c.db.Close()
		c.db = nil
	}

	// Replace the database file.
	if err := os.Rename(tmpPath, c.dbPath); err != nil {
		return fmt.Errorf("failed to replace database file: %w", err)
	}

	// Reopen the database.
	db, err := sql.Open("sqlite", c.dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("failed to reopen database: %w", err)
	}
	c.db = db

	return nil
}

// copyFile copies src to dst using standard file I/O.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
