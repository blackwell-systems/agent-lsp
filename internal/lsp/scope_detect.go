// scope_detect.go implements automatic package detection for selective indexing
// (Layer 2). When an agent opens a file, DetectPackageScope identifies the
// package boundary and its direct local dependencies, returning paths suitable
// for GenerateScopeConfig. This avoids loading the entire workspace graph on
// large repos while still providing cross-file intelligence within the active
// package.
//
// Languages that define explicit module boundaries (Go via go.mod, Rust via
// Cargo.toml) return nil since their language servers already scope naturally.
package lsp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// autoScopeThreshold is the minimum number of source files before auto-scoping
// activates. Below this threshold, the language server can handle the full
// workspace without performance issues.
const autoScopeThreshold = 500

// DetectPackageScope returns the package directory and its direct import
// directories for a given file path and language. The returned paths are
// relative to rootDir, suitable for passing to GenerateScopeConfig.
//
// Returns nil for languages that don't need scoping (Go, Rust) or when the
// package boundary cannot be determined.
func DetectPackageScope(filePath, rootDir, languageID string) ([]string, error) {
	switch languageID {
	case "python":
		return detectPythonPackage(filePath, rootDir)
	case "typescript", "typescriptreact", "javascript", "javascriptreact":
		return detectTSPackage(filePath, rootDir)
	default:
		// Go, Rust, and others: no scoping needed.
		return nil, nil
	}
}

// countSourceFiles counts files with matching extensions under rootDir,
// short-circuiting once the threshold is reached. Returns true if the count
// meets or exceeds the threshold.
func countSourceFiles(rootDir, languageID string) bool {
	exts := sourceExtensions(languageID)
	if len(exts) == 0 {
		return false
	}

	count := 0
	if walkErr := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if d.IsDir() {
			base := d.Name()
			// Skip common non-source directories for speed.
			if base == "node_modules" || base == ".git" || base == "__pycache__" ||
				base == ".venv" || base == "venv" || base == ".tox" ||
				base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		for _, e := range exts {
			if ext == e {
				count++
				if count >= autoScopeThreshold {
					return filepath.SkipAll
				}
				break
			}
		}
		return nil
	}); walkErr != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("auto-scope: walk error counting source files: %v", walkErr))
	}
	return count >= autoScopeThreshold
}

// sourceExtensions returns file extensions relevant for a language.
func sourceExtensions(languageID string) []string {
	switch languageID {
	case "python":
		return []string{".py"}
	case "typescript", "typescriptreact":
		return []string{".ts", ".tsx"}
	case "javascript", "javascriptreact":
		return []string{".js", ".jsx"}
	default:
		return nil
	}
}

// --- Python package detection ---

// detectPythonPackage finds the Python package containing filePath by walking
// up from the file's directory looking for __init__.py. Then parses imports
// from all .py files in that package directory to find direct local dependencies.
func detectPythonPackage(filePath, rootDir string) ([]string, error) {
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	// Find the package root: walk up from file's dir until __init__.py disappears.
	pkgDir := filepath.Dir(absFile)
	for {
		initPath := filepath.Join(pkgDir, "__init__.py")
		if _, err := os.Stat(initPath); os.IsNotExist(err) {
			// No __init__.py here; the package was the previous level.
			// But if we never found one, use the file's directory.
			break
		}
		parent := filepath.Dir(pkgDir)
		if parent == pkgDir || !strings.HasPrefix(parent, absRoot) {
			// Reached filesystem root or above workspace root.
			break
		}
		// Check if the parent also has __init__.py; if not, pkgDir is the top.
		parentInit := filepath.Join(parent, "__init__.py")
		if _, err := os.Stat(parentInit); os.IsNotExist(err) {
			break
		}
		pkgDir = parent
	}

	// Collect relative package path.
	relPkg, err := filepath.Rel(absRoot, pkgDir)
	if err != nil {
		return nil, err
	}

	// Start with the package directory itself.
	scopePaths := []string{relPkg}

	// Parse imports from .py files in the package directory (non-recursive,
	// just the top level of the package).
	imports := parsePythonImportsInDir(pkgDir)

	// Resolve imports to local directories within the workspace.
	for _, imp := range imports {
		// Convert dotted module path to directory path.
		dirPath := strings.ReplaceAll(imp, ".", string(filepath.Separator))
		absPath := filepath.Join(absRoot, dirPath)
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			rel, err := filepath.Rel(absRoot, absPath)
			if err == nil && !containsPath(scopePaths, rel) {
				scopePaths = append(scopePaths, rel)
			}
		}
	}

	return scopePaths, nil
}

var (
	pyImportRe     = regexp.MustCompile(`^import\s+(\S+)`)
	pyFromImportRe = regexp.MustCompile(`^from\s+(\S+)\s+import`)
)

// parsePythonImportsInDir scans all .py files in dir (non-recursive) and
// returns unique top-level module names from import statements.
func parsePythonImportsInDir(dir string) []string {
	seen := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".py") {
			continue
		}
		for _, imp := range parsePythonImportsFile(filepath.Join(dir, e.Name())) {
			// Take the top-level module name (first dotted component).
			parts := strings.SplitN(imp, ".", 2)
			seen[parts[0]] = true
		}
	}
	result := make([]string, 0, len(seen))
	for mod := range seen {
		result = append(result, mod)
	}
	return result
}

// parsePythonImportsFile extracts import module names from a single .py file.
func parsePythonImportsFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if m := pyImportRe.FindStringSubmatch(line); m != nil {
			imports = append(imports, m[1])
		} else if m := pyFromImportRe.FindStringSubmatch(line); m != nil {
			// Skip relative imports (from . import X, from .. import Y).
			if !strings.HasPrefix(m[1], ".") {
				imports = append(imports, m[1])
			}
		}
	}
	return imports
}

// --- TypeScript/JavaScript package detection ---

// detectTSPackage finds the nearest package.json above filePath and parses
// relative imports from source files in that directory.
func detectTSPackage(filePath, rootDir string) ([]string, error) {
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	// Walk up from file's directory looking for package.json.
	pkgDir := filepath.Dir(absFile)
	for {
		pkgJSON := filepath.Join(pkgDir, "package.json")
		if _, err := os.Stat(pkgJSON); err == nil {
			break // Found it.
		}
		parent := filepath.Dir(pkgDir)
		if parent == pkgDir || !strings.HasPrefix(parent, absRoot) {
			// No package.json found; use the file's directory.
			pkgDir = filepath.Dir(absFile)
			break
		}
		pkgDir = parent
	}

	relPkg, err := filepath.Rel(absRoot, pkgDir)
	if err != nil {
		return nil, err
	}

	scopePaths := []string{relPkg}

	// Parse relative imports from source files in the package directory.
	imports := parseTSImportsInDir(pkgDir)

	// Resolve relative imports to directories within the workspace.
	for _, imp := range imports {
		absPath := filepath.Join(pkgDir, imp)
		// Try as directory first, then as file's parent.
		resolved := resolveRelativeTSPath(absPath)
		if resolved == "" {
			continue
		}
		if !strings.HasPrefix(resolved, absRoot) {
			continue // Outside workspace.
		}
		rel, err := filepath.Rel(absRoot, resolved)
		if err == nil && !containsPath(scopePaths, rel) {
			scopePaths = append(scopePaths, rel)
		}
	}

	return scopePaths, nil
}

var (
	tsImportFromRe = regexp.MustCompile(`from\s+['"](\.[^'"]+)['"]`)
	tsRequireRe    = regexp.MustCompile(`require\s*\(\s*['"](\.[^'"]+)['"]\s*\)`)
)

// parseTSImportsInDir scans source files in dir (non-recursive) for relative
// import paths.
func parseTSImportsInDir(dir string) []string {
	seen := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	exts := map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !exts[ext] {
			continue
		}
		for _, imp := range parseTSImportsFile(filepath.Join(dir, e.Name())) {
			seen[imp] = true
		}
	}
	result := make([]string, 0, len(seen))
	for imp := range seen {
		result = append(result, imp)
	}
	return result
}

// parseTSImportsFile extracts relative import paths from a single TS/JS file.
func parseTSImportsFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := tsImportFromRe.FindAllStringSubmatch(line, -1); matches != nil {
			for _, m := range matches {
				imports = append(imports, m[1])
			}
		}
		if matches := tsRequireRe.FindAllStringSubmatch(line, -1); matches != nil {
			for _, m := range matches {
				imports = append(imports, m[1])
			}
		}
	}
	return imports
}

// resolveRelativeTSPath resolves a relative import path to a directory.
// TS imports can omit extensions, so we check for the directory itself.
func resolveRelativeTSPath(absPath string) string {
	// If it's a directory, use it directly.
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return absPath
	}
	// Otherwise use the parent directory (the import points to a file).
	dir := filepath.Dir(absPath)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// --- helpers ---

// containsPath checks if a path is already in the slice.
func containsPath(paths []string, p string) bool {
	for _, existing := range paths {
		if existing == p {
			return true
		}
	}
	return false
}

// ShouldAutoScope determines if auto-scoping should be activated for a workspace.
// Returns true when the language benefits from scoping and the workspace exceeds
// the file count threshold.
func ShouldAutoScope(rootDir, languageID string) bool {
	switch languageID {
	case "python", "typescript", "typescriptreact", "javascript", "javascriptreact":
		// Check if the workspace has enough files to warrant scoping.
		return countSourceFiles(rootDir, languageID)
	default:
		return false
	}
}

// UpdateAutoScope detects the package scope for the given file and, if it
// differs from the current scope, regenerates the scope config. Returns the
// new scope paths (or the existing ones if unchanged).
//
// This is called from the tool layer on open_document to shift the scope
// as the agent navigates between packages.
func UpdateAutoScope(client *LSPClient, filePath, languageID string) {
	if client == nil {
		return
	}

	newScope, err := DetectPackageScope(filePath, client.RootDir(), languageID)
	if err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("auto-scope: detection failed for %s: %v", filePath, err))
		return
	}
	if len(newScope) == 0 {
		return
	}

	// Compare with current scope.
	client.mu.Lock()
	current := client.currentScope
	client.mu.Unlock()

	if pathsEqual(current, newScope) {
		return // No change needed.
	}

	// Scope shift detected; regenerate config.
	logging.Log(logging.LevelInfo, fmt.Sprintf("auto-scope: shifting scope from %v to %v", current, newScope))

	sc, err := GenerateScopeConfig(client.RootDir(), languageID, newScope)
	if err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("auto-scope: failed to generate config: %v", err))
		return
	}

	// Clean up old scope config before setting new one.
	client.mu.Lock()
	if client.scopeConfig != nil {
		RemoveScopeConfig(client.scopeConfig)
	}
	client.scopeConfig = sc
	client.currentScope = newScope
	client.autoScope = true
	client.mu.Unlock()
}

// pathsEqual compares two string slices for equality (order-sensitive).
func pathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
