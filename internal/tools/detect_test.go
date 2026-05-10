package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleDetectLspServers_MissingDir(t *testing.T) {
	r, err := HandleDetectLspServers(context.Background(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing workspace_dir")
	}
}

func TestHandleDetectLspServers_GoWorkspace(t *testing.T) {
	dir := t.TempDir()
	// Simulate a Go workspace: go.mod root marker + .go files.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := HandleDetectLspServers(context.Background(), nil, map[string]any{
		"workspace_dir": dir,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error result: %s", r.Content[0].Text)
	}

	var result DetectResult
	if err := json.Unmarshal([]byte(r.Content[0].Text), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.WorkspaceLanguages) == 0 {
		t.Fatal("expected at least one workspace language")
	}
	if result.WorkspaceLanguages[0] != "go" {
		t.Errorf("expected 'go' as top language, got %q", result.WorkspaceLanguages[0])
	}
	if result.WorkspaceDir != dir {
		t.Errorf("workspace_dir: want %q, got %q", dir, result.WorkspaceDir)
	}
}

func TestHandleDetectLspServers_MultiLanguage(t *testing.T) {
	dir := t.TempDir()
	// Go + TypeScript workspace.
	files := map[string]string{
		"go.mod":        "module example.com/foo\n",
		"main.go":       "package main\n",
		"app.ts":        "const x = 1;\n",
		"tsconfig.json": "{}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r, err := HandleDetectLspServers(context.Background(), nil, map[string]any{
		"workspace_dir": dir,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error result: %s", r.Content[0].Text)
	}

	var result DetectResult
	if err := json.Unmarshal([]byte(r.Content[0].Text), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	hasLang := func(lang string) bool {
		for _, l := range result.WorkspaceLanguages {
			if l == lang {
				return true
			}
		}
		return false
	}
	if !hasLang("go") {
		t.Error("expected 'go' in workspace_languages")
	}
	if !hasLang("typescript") {
		t.Error("expected 'typescript' in workspace_languages")
	}
}

func TestHandleDetectLspServers_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	// Put a .go file inside node_modules — should be ignored.
	nmDir := filepath.Join(dir, "node_modules", "some-pkg")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "index.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Put a real .py file at root.
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := HandleDetectLspServers(context.Background(), nil, map[string]any{
		"workspace_dir": dir,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	var result DetectResult
	if err := json.Unmarshal([]byte(r.Content[0].Text), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, lang := range result.WorkspaceLanguages {
		if lang == "go" {
			t.Error("'go' should not be detected — its only .go file is inside node_modules")
		}
	}
}

// --- TestBuildConfigEntry ---

func TestBuildConfigEntry(t *testing.T) {
	cases := []struct {
		def  lspServerDef
		want string
	}{
		{
			def:  lspServerDef{Language: "go", Binary: "gopls"},
			want: "go:gopls",
		},
		{
			def:  lspServerDef{Language: "typescript", Binary: "typescript-language-server", Args: []string{"--stdio"}},
			want: "typescript:typescript-language-server,--stdio",
		},
		{
			def:  lspServerDef{Language: "ruby", Binary: "solargraph", Args: []string{"stdio"}},
			want: "ruby:solargraph,stdio",
		},
	}

	for _, tc := range cases {
		got := buildConfigEntry(tc.def)
		if got != tc.want {
			t.Errorf("buildConfigEntry(%q): want %q, got %q", tc.def.Language, tc.want, got)
		}
	}
}

// --- TestSuggestedConfigDeduplication ---

// TestSuggestedConfigDeduplication verifies that c and cpp both map to clangd
// but suggested_config only emits one clangd entry.
func TestSuggestedConfigDeduplication(t *testing.T) {
	dir := t.TempDir()
	// C and C++ files.
	for _, name := range []string{"main.c", "util.cpp"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r, err := HandleDetectLspServers(context.Background(), nil, map[string]any{
		"workspace_dir": dir,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	var result DetectResult
	if err := json.Unmarshal([]byte(r.Content[0].Text), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Count clangd entries in suggested_config.
	clangdCount := 0
	for _, entry := range result.SuggestedConfig {
		if strings.Contains(entry, "clangd") {
			clangdCount++
		}
	}
	if clangdCount > 1 {
		t.Errorf("expected at most 1 clangd entry in suggested_config, got %d", clangdCount)
	}
}
