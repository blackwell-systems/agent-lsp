// cache.go provides a persistent SQLite cache for LSP-derived data.
//
// The cache stores symbol reference results keyed by file content hash and
// symbol identity. Subsequent sessions serve cached results instantly;
// gopls/pyright are only re-queried for files that changed since last index.
//
// Invalidation: when the file watcher detects a change, all entries for that
// file are evicted. The next query re-populates from the language server.
//
// The cache is opportunistic: agent-lsp works without it. Missing, corrupted,
// or stale cache falls back to querying the language server directly.
package lsp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// SymbolRefCache is a persistent cache for symbol reference results.
type SymbolRefCache struct {
	mu sync.Mutex
	db *sql.DB
}

// NewSymbolRefCache opens or creates a SQLite cache for the given workspace.
// The database is stored at ~/.agent-lsp/cache/<workspace-hash>/refs.db.
// Returns nil (no-op cache) if the database cannot be opened.
func NewSymbolRefCache(workspaceRoot string) *SymbolRefCache {
	home, err := os.UserHomeDir()
	if err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache: cannot determine home dir: %v", err))
		return nil
	}

	hash := sha256.Sum256([]byte(workspaceRoot))
	dirName := hex.EncodeToString(hash[:8])
	cacheDir := filepath.Join(home, ".agent-lsp", "cache", dirName)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache: cannot create dir: %v", err))
		return nil
	}

	dbPath := filepath.Join(cacheDir, "refs.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache: cannot open db: %v", err))
		return nil
	}

	// Create tables.
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
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache: cannot create tables: %v", err))
		db.Close()
		return nil
	}

	return &SymbolRefCache{db: db}
}

// CachedRefs represents a cached set of reference locations for a symbol.
type CachedRefs struct {
	Locations []types.Location
}

// Get returns cached references for a symbol if the file hasn't changed.
// Returns nil if not cached or if the file hash doesn't match (stale).
func (c *SymbolRefCache) Get(filePath, symbolName string, symbolLine int) *CachedRefs {
	if c == nil {
		return nil
	}

	currentHash, err := hashFile(filePath)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var storedHash, locsJSON string
	err = c.db.QueryRow(
		`SELECT file_hash, locations FROM symbol_refs WHERE file_path = ? AND symbol_name = ? AND symbol_line = ?`,
		filePath, symbolName, symbolLine,
	).Scan(&storedHash, &locsJSON)

	if err != nil {
		return nil // not cached
	}

	if storedHash != currentHash {
		// File changed since cache entry was written. Evict.
		if _, err := c.db.Exec(`DELETE FROM symbol_refs WHERE file_path = ? AND symbol_name = ? AND symbol_line = ?`,
			filePath, symbolName, symbolLine); err != nil {
			logging.Log(logging.LevelDebug, fmt.Sprintf("cache: evict stale entry: %v", err))
		}
		return nil
	}

	var locs []types.Location
	if err := json.Unmarshal([]byte(locsJSON), &locs); err != nil {
		return nil
	}

	return &CachedRefs{Locations: locs}
}

// Put stores reference locations for a symbol.
func (c *SymbolRefCache) Put(filePath, symbolName string, symbolLine int, locs []types.Location) {
	if c == nil {
		return
	}

	currentHash, err := hashFile(filePath)
	if err != nil {
		return
	}

	locsJSON, err := json.Marshal(locs)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.db.Exec(
		`INSERT OR REPLACE INTO symbol_refs (file_path, file_hash, symbol_name, symbol_line, locations, cached_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		filePath, currentHash, symbolName, symbolLine, string(locsJSON), time.Now().Unix(),
	); err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache: put %s/%s: %v", filePath, symbolName, err))
	}
}

// InvalidateFile evicts all cached entries for a file.
// Called by the file watcher when a source file changes.
func (c *SymbolRefCache) InvalidateFile(filePath string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.db.Exec(`DELETE FROM symbol_refs WHERE file_path = ?`, filePath); err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache: invalidate %s: %v", filePath, err))
	}
}

// Stats returns the number of cached entries and the database size.
func (c *SymbolRefCache) Stats() (entries int, sizeBytes int64) {
	if c == nil {
		return 0, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.db.QueryRow(`SELECT COUNT(*) FROM symbol_refs`).Scan(&entries); err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("cache: stats: %v", err))
	}
	// Size is approximate; SQLite doesn't expose exact file size via SQL.
	return entries, 0
}

// Close closes the database connection.
func (c *SymbolRefCache) Close() error {
	if c == nil {
		return nil
	}
	return c.db.Close()
}

// hashFile returns the SHA-256 hex hash of a file's contents.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
