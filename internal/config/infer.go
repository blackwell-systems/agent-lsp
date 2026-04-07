package config

import (
	"os"
	"path/filepath"
)

// inferWorkspaceRoot walks the directory tree from filePath upward, searching
// for workspace root markers in priority order:
//
//	go.mod        → language "go"
//	package.json  → language "typescript"
//	Cargo.toml    → language "rust"
//	pyproject.toml or setup.py → language "python"
//	.git          → language "" (fallback)
//
// Returns ("", "", nil) when no marker is found at or above the file's directory.
// Returns (root, languageID, nil) on success.
// Returns ("", "", err) only for unexpected os.Stat or os.Getwd errors.
func inferWorkspaceRoot(filePath string) (root, languageID string, err error) {
	// Determine the starting directory.
	info, statErr := os.Stat(filePath)
	var dir string
	if statErr == nil && info.IsDir() {
		dir = filePath
	} else {
		dir = filepath.Dir(filePath)
	}

	// Markers checked in priority order; .git is last (fallback).
	type marker struct {
		name string
		lang string
	}
	markers := []marker{
		{"go.mod", "go"},
		{"package.json", "typescript"},
		{"Cargo.toml", "rust"},
		{"pyproject.toml", "python"},
		{"setup.py", "python"},
		{".git", ""},
	}

	for {
		for _, m := range markers {
			candidate := filepath.Join(dir, m.name)
			_, statErr := os.Stat(candidate)
			if statErr == nil {
				// Marker found.
				return dir, m.lang, nil
			}
			if !os.IsNotExist(statErr) {
				// Unexpected I/O error.
				return "", "", statErr
			}
		}

		// Move to parent directory.
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root with no marker found.
			return "", "", nil
		}
		dir = parent
	}
}
