package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestInitNonInteractive runs `agent-lsp init --non-interactive` in a temp
// directory and verifies it creates a valid .mcp.json with the correct shape.
func TestInitNonInteractive(t *testing.T) {
	binaryPath := getMultilangBinary(t)

	tmpDir := t.TempDir()
	cmd := exec.Command(binaryPath, "init", "--non-interactive")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// No language servers on PATH is a valid CI state — skip rather than fail.
		t.Skipf("agent-lsp init --non-interactive exited non-zero (no servers on PATH?): %v\n%s", err, out)
	}

	configPath := filepath.Join(tmpDir, ".mcp.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected .mcp.json to be created, got error: %v", err)
	}

	var cfg struct {
		MCPServers map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse .mcp.json: %v\ncontents: %s", err, data)
	}

	lsp, ok := cfg.MCPServers["lsp"]
	if !ok {
		t.Fatalf("expected mcpServers.lsp key, got: %v", cfg.MCPServers)
	}
	if lsp.Type != "stdio" {
		t.Errorf("expected type=stdio, got %q", lsp.Type)
	}
	if lsp.Command != "agent-lsp" {
		t.Errorf("expected command=agent-lsp, got %q", lsp.Command)
	}
	if len(lsp.Args) == 0 {
		t.Error("expected at least one arg (language:server), got empty args")
	}
}

// TestInitNonInteractiveMerge verifies that init merges into an existing
// config file without overwriting other mcpServers entries.
func TestInitNonInteractiveMerge(t *testing.T) {
	binaryPath := getMultilangBinary(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".mcp.json")

	// Pre-create a config with an existing server entry.
	existing := `{
  "mcpServers": {
    "other-tool": {
      "type": "stdio",
      "command": "other-tool",
      "args": []
    }
  }
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to write pre-existing config: %v", err)
	}

	cmd := exec.Command(binaryPath, "init", "--non-interactive")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("agent-lsp init --non-interactive exited non-zero (no servers on PATH?): %v\n%s", err, out)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config after merge: %v", err)
	}

	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse merged config: %v\ncontents: %s", err, data)
	}

	if _, ok := cfg.MCPServers["other-tool"]; !ok {
		t.Error("merge overwrote existing 'other-tool' entry — should have been preserved")
	}
	if _, ok := cfg.MCPServers["lsp"]; !ok {
		t.Error("expected 'lsp' entry to be added by init")
	}
}

// TestInitHelpFlag verifies that agent-lsp --version still works after
// adding init subcommand routing (regression guard).
func TestInitVersionFlag(t *testing.T) {
	binaryPath := getMultilangBinary(t)

	out, err := exec.Command(binaryPath, "--version").Output()
	if err != nil {
		t.Fatalf("--version flag failed: %v", err)
	}
	if len(out) == 0 {
		t.Error("--version produced no output")
	}
}
