package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runUninstall is the entry point for `agent-lsp uninstall`.
// It removes all agent-lsp configs, skills, and caches.
func runUninstall(args []string) {
	dryRun := false
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
		}
	}

	removed := 0
	skipped := 0

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not determine home directory: %v\n", err)
		os.Exit(1)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not determine working directory: %v\n", err)
		os.Exit(1)
	}

	// Step 1: MCP config cleanup.
	mcpPaths := []string{
		filepath.Join(cwd, ".mcp.json"),
		filepath.Join(homeDir, ".claude", ".mcp.json"),
		filepath.Join(cwd, ".cursor", "mcp.json"),
		filepath.Join(cwd, ".vscode", "cline_mcp_settings.json"),
		filepath.Join(cwd, ".gemini", "settings.json"),
	}

	for _, p := range mcpPaths {
		r, s := cleanMCPConfig(p, dryRun)
		removed += r
		skipped += s
	}

	// Step 2: Skill symlink/directory cleanup.
	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	r, s := cleanSkillDirs(skillsDir, dryRun)
	removed += r
	skipped += s

	// Step 3: CLAUDE.md managed section cleanup.
	claudeMDPath := filepath.Join(homeDir, ".claude", "CLAUDE.md")
	r, s = cleanClaudeMDSection(claudeMDPath, dryRun)
	removed += r
	skipped += s

	// Step 4: Cache directory cleanup.
	cachePaths := []string{
		filepath.Join(homeDir, ".agent-lsp", "cache"),
		filepath.Join(cwd, ".agent-lsp", "cache.db.gz"),
	}
	for _, p := range cachePaths {
		r, s = cleanPath(p, dryRun)
		removed += r
		skipped += s
	}

	fmt.Printf("Removed %d items. Skipped %d items (not found).\n", removed, skipped)
	fmt.Println("To remove the binary: rm $(which agent-lsp)")
}

// cleanMCPConfig reads a JSON config file, removes the "agent-lsp" and "lsp"
// keys from mcpServers, and writes the result back. Returns (removed, skipped).
func cleanMCPConfig(path string, dryRun bool) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 1
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse %s: %v\n", path, err)
		return 0, 1
	}

	serversRaw, ok := raw["mcpServers"]
	if !ok {
		return 0, 1
	}
	servers, ok := serversRaw.(map[string]interface{})
	if !ok {
		return 0, 1
	}

	keysToRemove := []string{"agent-lsp", "lsp"}
	removedCount := 0
	for _, key := range keysToRemove {
		if _, exists := servers[key]; exists {
			if dryRun {
				fmt.Printf("[dry-run] Would remove key %q from mcpServers in %s\n", key, path)
			} else {
				delete(servers, key)
			}
			removedCount++
		}
	}

	if removedCount == 0 {
		return 0, 1
	}

	if !dryRun {
		out, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not marshal %s: %v\n", path, err)
			return 0, 1
		}
		out = append(out, '\n')
		if err := os.WriteFile(path, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", path, err)
			return 0, 1
		}
	}

	return removedCount, 0
}

// cleanSkillDirs removes all lsp-* directories from the skills directory.
// Returns (removed, skipped).
func cleanSkillDirs(skillsDir string, dryRun bool) (int, int) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return 0, 1
	}

	removedCount := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "lsp-") {
			p := filepath.Join(skillsDir, e.Name())
			if dryRun {
				fmt.Printf("[dry-run] Would remove skill directory %s\n", p)
			} else {
				if err := os.RemoveAll(p); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", p, err)
					continue
				}
			}
			removedCount++
		}
	}

	if removedCount == 0 {
		return 0, 1
	}
	return removedCount, 0
}

// cleanClaudeMDSection removes the managed section between sentinel comments
// from the given file. Returns (removed, skipped).
func cleanClaudeMDSection(path string, dryRun bool) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 1
	}

	content := string(data)
	startMarker := "<!-- agent-lsp:skills:start -->"
	endMarker := "<!-- agent-lsp:skills:end -->"

	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return 0, 1
	}
	endIdx := strings.Index(content, endMarker)
	if endIdx == -1 {
		return 0, 1
	}

	endIdx += len(endMarker)
	// Also remove a trailing newline if present.
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}

	if dryRun {
		fmt.Printf("[dry-run] Would remove managed section from %s\n", path)
		return 1, 0
	}

	newContent := content[:startIdx] + content[endIdx:]
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", path, err)
		return 0, 1
	}

	return 1, 0
}

// cleanPath removes a file or directory. Returns (removed, skipped).
func cleanPath(path string, dryRun bool) (int, int) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, 1
	}

	if dryRun {
		fmt.Printf("[dry-run] Would remove %s\n", path)
		return 1, 0
	}

	if err := os.RemoveAll(path); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", path, err)
		return 0, 1
	}
	return 1, 0
}
