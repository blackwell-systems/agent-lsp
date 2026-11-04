package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const existingCodexConfig = `# My Codex settings
model = "gpt-5" # inline comment kept
approval_policy = "on-request"

[mcp_servers.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp"]

[mcp_servers.context7.env]
API_KEY = "secret"

[profiles.fast]
model = "gpt-5-mini"
`

// Merging keeps every other line of the user's config byte for byte,
// comments and other servers (with their env) included.
func TestWriteCodexConfig_KeepsEverythingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(existingCodexConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeCodexConfig(path, []string{"go:gopls", "typescript:typescript-language-server,--stdio"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := strings.TrimRight(existingCodexConfig, "\n") + "\n\n" +
		"[mcp_servers.agent-lsp]\ncommand = \"agent-lsp\"\nargs = [\"go:gopls\", \"typescript:typescript-language-server,--stdio\"]\n"
	if string(got) != want {
		t.Fatalf("config =\n%s\nwant\n%s", got, want)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 kept", info.Mode().Perm())
	}
}

// Re-running init replaces the existing agent-lsp table and its sub-tables,
// in bare or quoted form, without duplicating it.
func TestWriteCodexConfig_ReplacesExistingTable(t *testing.T) {
	for _, header := range []string{`[mcp_servers.agent-lsp]`, `[mcp_servers."agent-lsp"]`} {
		path := filepath.Join(t.TempDir(), "config.toml")
		old := "model = \"gpt-5\"\n\n" + header + "\ncommand = \"agent-lsp\"\nargs = [\"go:gopls\"]\n\n[mcp_servers.agent-lsp.env]\nX = \"1\"\n\n[mcp_servers.other]\ncommand = \"other\"\n"
		if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeCodexConfig(path, []string{"python:pyright-langserver,--stdio"}); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(path)
		want := "model = \"gpt-5\"\n\n[mcp_servers.other]\ncommand = \"other\"\n\n" +
			"[mcp_servers.agent-lsp]\ncommand = \"agent-lsp\"\nargs = [\"python:pyright-langserver,--stdio\"]\n"
		if string(got) != want {
			t.Fatalf("%s: config =\n%s\nwant\n%s", header, got, want)
		}
	}
}

func TestWriteCodexConfig_NewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", ".codex", "config.toml")
	if err := writeCodexConfig(path, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := "[mcp_servers.agent-lsp]\ncommand = \"agent-lsp\"\nargs = []\n"; string(got) != want {
		t.Fatalf("config = %q, want %q", got, want)
	}
}

func TestTOMLString_Escapes(t *testing.T) {
	for in, want := range map[string]string{
		`plain`:           `"plain"`,
		`C:\tools\gopls`:  `"C:\\tools\\gopls"`,
		`say "hi"`:        `"say \"hi\""`,
		"tab\there":       `"tab\there"`,
		"bell\x07":        `"bell\u0007"`,
		"unicode: café ✓": `"unicode: café ✓"`,
	} {
		if got := tomlString(in); got != want {
			t.Errorf("tomlString(%q) = %s, want %s", in, got, want)
		}
	}
}

// uninstall removes only the agent-lsp table.
func TestCleanCodexConfig_RemovesOnlyAgentLSP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := existingCodexConfig + "\n[mcp_servers.agent-lsp]\ncommand = \"agent-lsp\"\nargs = []\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if removed, _ := cleanCodexConfig(path, true); removed != 1 {
		t.Fatalf("dry run removed = %d, want 1", removed)
	}
	if got, _ := os.ReadFile(path); string(got) != content {
		t.Fatal("dry run changed the file")
	}
	if removed, _ := cleanCodexConfig(path, false); removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	got, _ := os.ReadFile(path)
	if string(got) != existingCodexConfig+"\n" {
		t.Fatalf("config after uninstall =\n%s", got)
	}
	if removed, skipped := cleanCodexConfig(path, false); removed != 0 || skipped != 1 {
		t.Fatalf("second clean = (%d, %d), want (0, 1)", removed, skipped)
	}
}

func TestCodexPaths_FollowCODEX_HOME(t *testing.T) {
	home, codex, cwd := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(cwd)
	wd, _ := os.Getwd()

	t.Setenv("CODEX_HOME", "")
	if got, _ := resolveTargetPath(11, ""); got != filepath.Join(home, ".codex", "config.toml") {
		t.Errorf("default user config = %s", got)
	}
	t.Setenv("CODEX_HOME", codex)
	for _, c := range []struct {
		name, got, want string
	}{
		{"user config", must(resolveTargetPath(11, "")), filepath.Join(codex, "config.toml")},
		{"project config", must(resolveTargetPath(12, "")), filepath.Join(wd, ".codex", "config.toml")},
		{"user rules", resolveRulesPath(11), filepath.Join(codex, "AGENTS.md")},
		{"project rules", resolveRulesPath(12), filepath.Join(wd, "AGENTS.md")},
		{"skills", resolveSkillsDest(11), filepath.Join(codex, "skills")},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
	found := false
	for _, p := range managedRulesPaths() {
		if p == filepath.Join(codex, "AGENTS.md") {
			found = true
		}
	}
	if !found {
		t.Error("uninstall does not clean the Codex AGENTS.md")
	}
}

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

func TestAddCodexUserScope_UsesCLI(t *testing.T) {
	var calls [][]string
	old := codexCLI
	codexCLI = func(args ...string) error {
		calls = append(calls, args)
		return nil
	}
	t.Cleanup(func() { codexCLI = old })
	if err := addCodexUserScope([]string{"go:gopls"}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"mcp", "remove", "agent-lsp"},
		{"mcp", "add", "agent-lsp", "--", "agent-lsp", "go:gopls"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("codex CLI calls = %v, want %v", calls, want)
	}
}
