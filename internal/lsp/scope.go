// scope.go generates temporary language-server configuration files that limit
// workspace indexing to a specific subdirectory. This enables agent-lsp to work
// on large monorepos without timeout failures from full-workspace reference queries.
//
// The generated config is placed in the workspace root and removed on server shutdown.
// Each language server has its own config format:
//
//   - Python (pyright): pyrightconfig.json with "include" array
//   - TypeScript (tsserver): tsconfig.json with "include" array
//   - Java (jdtls): .settings/org.eclipse.jdt.core.prefs (not yet implemented)
//   - Go (gopls): no scoping needed (modules define boundaries)
//   - Rust (rust-analyzer): no scoping needed (Cargo.toml defines boundaries)
//
// Usage: call GenerateScopeConfig before Initialize. Call RemoveScopeConfig on Shutdown.
package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// ScopeConfig holds the state needed to clean up generated config files.
type ScopeConfig struct {
	GeneratedFiles []string          // absolute paths of files we created
	BackedUpFiles  map[string]string // original path -> backup path (if we overwrote)
}

// GenerateScopeConfig creates a language-server-specific configuration file
// that limits indexing to the given scope paths within rootDir.
// Returns a ScopeConfig for cleanup, or nil if no scoping was needed.
//
// scopePaths are relative to rootDir (e.g., "langchain_core/runnables").
// languageID determines which config format to generate.
func GenerateScopeConfig(rootDir string, languageID string, scopePaths []string) (*ScopeConfig, error) {
	if len(scopePaths) == 0 {
		return nil, nil
	}

	switch languageID {
	case "python":
		return generatePyrightScope(rootDir, scopePaths)
	case "typescript", "typescriptreact", "javascript", "javascriptreact":
		return generateTSScope(rootDir, scopePaths)
	default:
		// Languages like Go, Rust, C/C++ don't need workspace scoping;
		// their servers respect module/project boundaries natively.
		logging.Log(logging.LevelDebug, fmt.Sprintf("scope: no config needed for language %q", languageID))
		return nil, nil
	}
}

// RemoveScopeConfig removes any generated config files and restores backups.
func RemoveScopeConfig(sc *ScopeConfig) {
	if sc == nil {
		return
	}
	for _, path := range sc.GeneratedFiles {
		if backup, ok := sc.BackedUpFiles[path]; ok {
			// Restore the original file.
			if err := os.Rename(backup, path); err != nil {
				logging.Log(logging.LevelDebug, fmt.Sprintf("scope: failed to restore backup %s: %v", backup, err))
			}
		} else {
			// Remove our generated file.
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				logging.Log(logging.LevelDebug, fmt.Sprintf("scope: failed to remove %s: %v", path, err))
			}
		}
	}
}

// generatePyrightScope creates a pyrightconfig.json that limits pyright's analysis
// to the specified paths. If a pyrightconfig.json already exists, it is backed up.
func generatePyrightScope(rootDir string, scopePaths []string) (*ScopeConfig, error) {
	configPath := filepath.Join(rootDir, "pyrightconfig.json")
	sc := &ScopeConfig{
		GeneratedFiles: []string{configPath},
		BackedUpFiles:  make(map[string]string),
	}

	// Back up existing config if present.
	if _, err := os.Stat(configPath); err == nil {
		backupPath := configPath + ".agent-lsp-backup"
		if err := os.Rename(configPath, backupPath); err != nil {
			return nil, fmt.Errorf("scope: failed to backup existing pyrightconfig.json: %w", err)
		}
		sc.BackedUpFiles[configPath] = backupPath
	}

	// Normalize scope paths: ensure they use forward slashes and are relative.
	includes := make([]string, 0, len(scopePaths))
	for _, p := range scopePaths {
		p = strings.TrimPrefix(p, rootDir)
		p = strings.TrimPrefix(p, "/")
		p = filepath.ToSlash(p)
		includes = append(includes, p)
	}

	config := map[string]any{
		"include":                   includes,
		"reportMissingImports":      false,
		"reportMissingModuleSource": false,
		"pythonVersion":             "3.11",
		"typeCheckingMode":          "basic",
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("scope: failed to marshal pyrightconfig: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return nil, fmt.Errorf("scope: failed to write pyrightconfig.json: %w", err)
	}

	logging.Log(logging.LevelDebug, fmt.Sprintf("scope: generated pyrightconfig.json scoped to %v", includes))
	return sc, nil
}

// generateTSScope creates a tsconfig.json that limits TypeScript's analysis
// to the specified paths. If a tsconfig.json already exists, it is backed up.
func generateTSScope(rootDir string, scopePaths []string) (*ScopeConfig, error) {
	configPath := filepath.Join(rootDir, "tsconfig.json")
	sc := &ScopeConfig{
		GeneratedFiles: []string{configPath},
		BackedUpFiles:  make(map[string]string),
	}

	// Back up existing config if present.
	if _, err := os.Stat(configPath); err == nil {
		backupPath := configPath + ".agent-lsp-backup"
		if err := os.Rename(configPath, backupPath); err != nil {
			return nil, fmt.Errorf("scope: failed to backup existing tsconfig.json: %w", err)
		}
		sc.BackedUpFiles[configPath] = backupPath
	}

	// Normalize scope paths with glob patterns for TypeScript.
	includes := make([]string, 0, len(scopePaths))
	for _, p := range scopePaths {
		p = strings.TrimPrefix(p, rootDir)
		p = strings.TrimPrefix(p, "/")
		p = filepath.ToSlash(p)
		// Add glob suffix if it's a directory path.
		if !strings.Contains(filepath.Base(p), ".") {
			p = p + "/**/*"
		}
		includes = append(includes, p)
	}

	config := map[string]any{
		"compilerOptions": map[string]any{
			"target":       "ES2020",
			"module":       "commonjs",
			"strict":       false,
			"skipLibCheck": true,
		},
		"include": includes,
		"exclude": []string{"node_modules"},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("scope: failed to marshal tsconfig: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return nil, fmt.Errorf("scope: failed to write tsconfig.json: %w", err)
	}

	logging.Log(logging.LevelDebug, fmt.Sprintf("scope: generated tsconfig.json scoped to %v", includes))
	return sc, nil
}
