package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// --- resolveTargetPath additional coverage ---

func TestResolveTargetPath_GlobalClaude(t *testing.T) {
	got, err := resolveTargetPath(2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, ".claude") || !strings.HasSuffix(got, ".mcp.json") {
		t.Errorf("got %q, want path containing .claude and ending in .mcp.json", got)
	}
}

func TestResolveTargetPath_ClaudeDesktop(t *testing.T) {
	got, err := resolveTargetPath(3, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "claude_desktop_config.json") {
		t.Errorf("got %q, want suffix claude_desktop_config.json", got)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(got, "Application Support") {
		t.Errorf("on macOS expected Application Support in path, got %q", got)
	}
}

func TestResolveTargetPath_Windsurf(t *testing.T) {
	got, err := resolveTargetPath(6, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "windsurf") || !strings.HasSuffix(got, "mcp_config.json") {
		t.Errorf("got %q, want path containing windsurf and ending in mcp_config.json", got)
	}
}

func TestResolveTargetPath_DefaultFallback(t *testing.T) {
	got, err := resolveTargetPath(999, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, ".mcp.json") {
		t.Errorf("got %q, want suffix .mcp.json", got)
	}
}

// --- writeOrMergeConfig: invalid JSON ---

func TestWriteOrMergeConfig_InvalidExistingJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.json"
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeOrMergeConfig(path, []string{"go:gopls"})
	if err == nil {
		t.Error("expected error for invalid JSON in existing file")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected error to mention parse, got: %v", err)
	}
}
