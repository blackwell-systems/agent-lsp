package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanMCPConfig_PreservesOtherServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"lsp": map[string]interface{}{
				"type":    "stdio",
				"command": "agent-lsp",
				"args":    []string{"go:gopls"},
			},
			"other-server": map[string]interface{}{
				"type":    "stdio",
				"command": "other-binary",
			},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0o644)

	removed, skipped := cleanMCPConfig(path, false)
	if removed != 1 {
		t.Errorf("expected removed=1, got %d", removed)
	}
	if skipped != 0 {
		t.Errorf("expected skipped=0, got %d", skipped)
	}

	// Verify other-server is preserved.
	result, _ := os.ReadFile(path)
	var parsed map[string]interface{}
	json.Unmarshal(result, &parsed)
	servers := parsed["mcpServers"].(map[string]interface{})

	if _, ok := servers["lsp"]; ok {
		t.Error("lsp key should have been removed")
	}
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server key should have been preserved")
	}
}

func TestCleanMCPConfig_RemovesBothKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"lsp":       map[string]interface{}{"command": "agent-lsp"},
			"agent-lsp": map[string]interface{}{"command": "agent-lsp"},
			"keep":      map[string]interface{}{"command": "other"},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0o644)

	removed, _ := cleanMCPConfig(path, false)
	if removed != 2 {
		t.Errorf("expected removed=2, got %d", removed)
	}

	result, _ := os.ReadFile(path)
	var parsed map[string]interface{}
	json.Unmarshal(result, &parsed)
	servers := parsed["mcpServers"].(map[string]interface{})
	if len(servers) != 1 {
		t.Errorf("expected 1 remaining server, got %d", len(servers))
	}
}

func TestCleanMCPConfig_MissingFile(t *testing.T) {
	removed, skipped := cleanMCPConfig("/nonexistent/path/.mcp.json", false)
	if removed != 0 || skipped != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", removed, skipped)
	}
}

func TestCleanClaudeMDSection_PreservesSurroundingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	content := `# My Config

Some content before.

<!-- agent-lsp:skills:start -->
## LSP Skills
Lots of skill documentation here.
<!-- agent-lsp:skills:end -->

Some content after.
`
	os.WriteFile(path, []byte(content), 0o644)

	removed, skipped := cleanClaudeMDSection(path, false)
	if removed != 1 {
		t.Errorf("expected removed=1, got %d", removed)
	}
	if skipped != 0 {
		t.Errorf("expected skipped=0, got %d", skipped)
	}

	result, _ := os.ReadFile(path)
	resultStr := string(result)

	if expected := "Some content before."; !contains(resultStr, expected) {
		t.Error("content before sentinel should be preserved")
	}
	if expected := "Some content after."; !contains(resultStr, expected) {
		t.Error("content after sentinel should be preserved")
	}
	if contains(resultStr, "agent-lsp:skills:start") {
		t.Error("start sentinel should have been removed")
	}
	if contains(resultStr, "LSP Skills") {
		t.Error("managed section content should have been removed")
	}
}

func TestCleanClaudeMDSection_NoSentinels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(path, []byte("# Just a normal file\n"), 0o644)

	removed, skipped := cleanClaudeMDSection(path, false)
	if removed != 0 || skipped != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", removed, skipped)
	}
}

func TestCleanClaudeMDSection_MissingFile(t *testing.T) {
	removed, skipped := cleanClaudeMDSection("/nonexistent/CLAUDE.md", false)
	if removed != 0 || skipped != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", removed, skipped)
	}
}

func TestUninstallDryRun_NoSideEffects(t *testing.T) {
	dir := t.TempDir()

	// Create an MCP config.
	mcpPath := filepath.Join(dir, ".mcp.json")
	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"lsp": map[string]interface{}{"command": "agent-lsp"},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(mcpPath, data, 0o644)
	originalData, _ := os.ReadFile(mcpPath)

	// Create a CLAUDE.md with sentinels.
	claudePath := filepath.Join(dir, "CLAUDE.md")
	claudeContent := "before\n<!-- agent-lsp:skills:start -->\nstuff\n<!-- agent-lsp:skills:end -->\nafter\n"
	os.WriteFile(claudePath, []byte(claudeContent), 0o644)

	// Create a skill directory.
	skillDir := filepath.Join(dir, "skills", "lsp-test")
	os.MkdirAll(skillDir, 0o755)

	// Create a cache directory.
	cacheDir := filepath.Join(dir, "cache")
	os.MkdirAll(cacheDir, 0o755)

	// Run dry-run on individual functions.
	cleanMCPConfig(mcpPath, true)
	cleanClaudeMDSection(claudePath, true)
	cleanSkillDirs(filepath.Join(dir, "skills"), true)
	cleanPath(cacheDir, true)

	// Verify nothing was changed.
	afterData, _ := os.ReadFile(mcpPath)
	if string(afterData) != string(originalData) {
		t.Error("MCP config was modified during dry-run")
	}

	afterClaude, _ := os.ReadFile(claudePath)
	if string(afterClaude) != claudeContent {
		t.Error("CLAUDE.md was modified during dry-run")
	}

	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		t.Error("skill directory was removed during dry-run")
	}

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("cache directory was removed during dry-run")
	}
}

func TestCleanSkillDirs(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(filepath.Join(skillsDir, "lsp-explore"), 0o755)
	os.MkdirAll(filepath.Join(skillsDir, "lsp-refactor"), 0o755)
	os.MkdirAll(filepath.Join(skillsDir, "other-skill"), 0o755)

	removed, _ := cleanSkillDirs(skillsDir, false)
	if removed != 2 {
		t.Errorf("expected removed=2, got %d", removed)
	}

	// Verify lsp-* dirs are gone but other-skill remains.
	entries, _ := os.ReadDir(skillsDir)
	if len(entries) != 1 || entries[0].Name() != "other-skill" {
		t.Errorf("expected only other-skill to remain, got %v", entries)
	}
}

func TestCleanPath_MissingPath(t *testing.T) {
	removed, skipped := cleanPath("/nonexistent/path", false)
	if removed != 0 || skipped != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", removed, skipped)
	}
}

func TestCleanPath_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "data.bin"), []byte("data"), 0o644)

	removed, skipped := cleanPath(cacheDir, false)
	if removed != 1 || skipped != 0 {
		t.Errorf("expected (1,0), got (%d,%d)", removed, skipped)
	}

	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("cache directory should have been removed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
