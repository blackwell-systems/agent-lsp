package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// init used to decode the config into a struct that knew only mcpServers and
// type/command/args, so writing it back dropped every other setting and the
// env/url/headers of the user's other servers.
func TestWriteOrMergeConfig_PreservesEverythingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude_desktop_config.json")
	original := `{
  "preferences": {"theme": "dark", "telemetry": false},
  "numStartups": 42,
  "mcpServers": {
    "github": {"command": "gh-mcp", "args": ["serve"], "env": {"GITHUB_TOKEN": "secret"}},
    "remote": {"type": "http", "url": "https://example.com/mcp", "headers": {"Authorization": "Bearer x"}},
    "lsp": {"type": "stdio", "command": "agent-lsp", "args": []}
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeOrMergeConfig(path, []string{"go:gopls"}); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, raw)
	}
	var want map[string]any
	_ = json.Unmarshal([]byte(original), &want)

	for _, key := range []string{"preferences", "numStartups"} {
		if !reflect.DeepEqual(got[key], want[key]) {
			t.Errorf("top-level %q = %v, want %v", key, got[key], want[key])
		}
	}
	servers := got["mcpServers"].(map[string]any)
	wantServers := want["mcpServers"].(map[string]any)
	for _, name := range []string{"github", "remote"} {
		if !reflect.DeepEqual(servers[name], wantServers[name]) {
			t.Errorf("server %q = %v, want it unchanged: %v", name, servers[name], wantServers[name])
		}
	}
	if _, ok := servers["lsp"]; ok {
		t.Error("legacy lsp entry not removed")
	}
	entry := servers["agent-lsp"].(map[string]any)
	if entry["command"] != "agent-lsp" || !reflect.DeepEqual(entry["args"], []any{"go:gopls"}) {
		t.Errorf("agent-lsp entry = %v", entry)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600 kept", info.Mode().Perm())
	}
}

// Claude Code's user scope lives in ~/.claude.json; ~/.claude/.mcp.json,
// which init used to write, is not read by Claude Code.
func TestResolveTargetPath_ClaudeUserScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := resolveTargetPath(2, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".claude.json"); got != want {
		t.Fatalf("choice 2 path = %s, want %s", got, want)
	}
}

func TestAddClaudeUserScope_UsesCLI(t *testing.T) {
	var calls [][]string
	old := claudeCLI
	claudeCLI = func(args ...string) error {
		calls = append(calls, args)
		return nil
	}
	t.Cleanup(func() { claudeCLI = old })

	if err := addClaudeUserScope([]string{"go:gopls", "typescript:typescript-language-server,--stdio"}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"mcp", "remove", "--scope", "user", "agent-lsp"},
		{"mcp", "add", "--scope", "user", "agent-lsp", "--", "agent-lsp", "go:gopls", "typescript:typescript-language-server,--stdio"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("claude CLI calls = %v, want %v", calls, want)
	}
}
