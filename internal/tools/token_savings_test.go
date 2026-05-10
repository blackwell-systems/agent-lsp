package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestEstimateTokenSavings_Basic(t *testing.T) {
	// Create a temp file with 400 bytes
	dir := t.TempDir()
	fp := filepath.Join(dir, "bigfile.txt")
	data := []byte(strings.Repeat("x", 400))
	if err := os.WriteFile(fp, data, 0644); err != nil {
		t.Fatal(err)
	}

	// "short" is 5 chars = 1 token (5/4 = 1 integer division)
	result := EstimateTokenSavings("short", fp)

	if result["tokens_returned"] != 1 {
		t.Errorf("tokens_returned = %d, want 1", result["tokens_returned"])
	}
	if result["tokens_full_file"] != 100 {
		t.Errorf("tokens_full_file = %d, want 100", result["tokens_full_file"])
	}
	if result["tokens_saved"] != 99 {
		t.Errorf("tokens_saved = %d, want 99", result["tokens_saved"])
	}
}

func TestEstimateTokenSavings_MissingFile(t *testing.T) {
	result := EstimateTokenSavings("hello world", "/nonexistent/path/file.go")

	if _, ok := result["tokens_returned"]; !ok {
		t.Error("expected tokens_returned key")
	}
	if _, ok := result["tokens_full_file"]; ok {
		t.Error("did not expect tokens_full_file key for missing file")
	}
	if _, ok := result["tokens_saved"]; ok {
		t.Error("did not expect tokens_saved key for missing file")
	}
}

func TestAppendTokenMeta_ErrorResult(t *testing.T) {
	errResult := types.ToolResult{
		Content: []types.ContentItem{{Type: "text", Text: "some error"}},
		IsError: true,
	}
	got := AppendTokenMeta(errResult, "/some/file.go")
	if len(got.Content) != 1 {
		t.Errorf("expected unchanged content length 1, got %d", len(got.Content))
	}
}

func TestAppendTokenMeta_AddsMetaContent(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(fp, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	input := types.ToolResult{
		Content: []types.ContentItem{{Type: "text", Text: "some content here"}},
	}
	got := AppendTokenMeta(input, fp)

	if len(got.Content) != 2 {
		t.Fatalf("expected 2 content items, got %d", len(got.Content))
	}
	if !strings.Contains(got.Content[1].Text, "_meta") {
		t.Errorf("expected second content item to contain _meta, got: %s", got.Content[1].Text)
	}
}
