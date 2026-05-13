package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/config"
)

func TestBuildLspArgs(t *testing.T) {
	tests := []struct {
		name    string
		entries []config.ServerEntry
		want    []string
	}{
		{
			name: "go/gopls no extra args",
			entries: []config.ServerEntry{
				{LanguageID: "go", Command: []string{"gopls"}},
			},
			want: []string{"go:gopls"},
		},
		{
			name: "typescript with --stdio",
			entries: []config.ServerEntry{
				{LanguageID: "typescript", Command: []string{"typescript-language-server", "--stdio"}},
			},
			want: []string{"typescript:typescript-language-server,--stdio"},
		},
		{
			name: "ruby with stdio arg",
			entries: []config.ServerEntry{
				{LanguageID: "ruby", Command: []string{"solargraph", "stdio"}},
			},
			want: []string{"ruby:solargraph,stdio"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildLspArgs(tc.entries)
			if len(got) != len(tc.want) {
				t.Fatalf("len(got)=%d, len(want)=%d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d]=%q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestWriteOrMergeConfig_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", ".mcp.json")

	err := writeOrMergeConfig(path, []string{"go:gopls"})
	if err != nil {
		t.Fatalf("writeOrMergeConfig returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read written file: %v", err)
	}

	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	entry, ok := cfg.MCPServers["agent-lsp"]
	if !ok {
		t.Fatal("MCPServers[\"agent-lsp\"] not found")
	}
	if entry.Type != "stdio" {
		t.Errorf("Type=%q, want \"stdio\"", entry.Type)
	}
	if entry.Command != "agent-lsp" {
		t.Errorf("Command=%q, want \"agent-lsp\"", entry.Command)
	}
}

func TestWriteOrMergeConfig_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	existing := `{"mcpServers":{"other-tool":{"type":"stdio","command":"other","args":[]}}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("could not write seed file: %v", err)
	}

	err := writeOrMergeConfig(path, []string{"go:gopls"})
	if err != nil {
		t.Fatalf("writeOrMergeConfig returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read merged file: %v", err)
	}

	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := cfg.MCPServers["other-tool"]; !ok {
		t.Error("\"other-tool\" key was lost after merge")
	}
	if _, ok := cfg.MCPServers["agent-lsp"]; !ok {
		t.Error("\"agent-lsp\" key not present after merge")
	}
}

func TestResolveTargetPath_ProjectFiles(t *testing.T) {
	tests := []struct {
		choice int
		suffix string
	}{
		{1, ".mcp.json"},
		{4, filepath.Join(".cursor", "mcp.json")},
		{5, filepath.Join(".vscode", "cline_mcp_settings.json")},
		{7, filepath.Join(".gemini", "settings.json")},
	}

	for _, tc := range tests {
		t.Run(tc.suffix, func(t *testing.T) {
			got, err := resolveTargetPath(tc.choice, "")
			if err != nil {
				t.Fatalf("resolveTargetPath(%d, \"\") error: %v", tc.choice, err)
			}
			if !strings.HasSuffix(got, tc.suffix) {
				t.Errorf("got %q, want suffix %q", got, tc.suffix)
			}
		})
	}
}

func TestResolveTargetPath_Custom(t *testing.T) {
	got, err := resolveTargetPath(8, "~/foo/bar.json")
	if err != nil {
		t.Fatalf("resolveTargetPath(8, ...) error: %v", err)
	}
	if strings.HasPrefix(got, "~") {
		t.Errorf("tilde was not expanded: got %q", got)
	}
	if !strings.HasSuffix(got, "foo/bar.json") {
		t.Errorf("expected path to end with foo/bar.json, got %q", got)
	}
}
