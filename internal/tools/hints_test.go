package tools

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestAppendHint(t *testing.T) {
	result := types.TextResult("some content")
	got := appendHint(result, "Use get_references to find usages.")

	// Hint should be a separate content item, not appended to Content[0].
	if got.Content[0].Text != "some content" {
		t.Errorf("appendHint() Content[0] = %q, want %q", got.Content[0].Text, "some content")
	}
	if len(got.Content) != 2 {
		t.Fatalf("appendHint() should add a second content item, got %d items", len(got.Content))
	}
	wantHint := "Next step: Use get_references to find usages."
	if got.Content[1].Text != wantHint {
		t.Errorf("appendHint() Content[1] = %q, want %q", got.Content[1].Text, wantHint)
	}
	if got.IsError {
		t.Error("appendHint() should not set IsError")
	}
}

func TestAppendHint_EmptyHint(t *testing.T) {
	result := types.TextResult("some content")
	got := appendHint(result, "")

	if got.Content[0].Text != "some content" {
		t.Errorf("appendHint() with empty hint should not modify content, got %q", got.Content[0].Text)
	}
}

func TestAppendHint_EmptyContent(t *testing.T) {
	result := types.TextResult("")
	got := appendHint(result, "some hint")

	if got.Content[0].Text != "" {
		t.Errorf("appendHint() with empty content should not modify content, got %q", got.Content[0].Text)
	}
}

func TestAppendHint_ErrorResult(t *testing.T) {
	result := types.ErrorResult("something failed")
	got := appendHint(result, "some hint")

	if got.Content[0].Text != "something failed" {
		t.Errorf("appendHint() should not modify error results, got %q", got.Content[0].Text)
	}
	if !got.IsError {
		t.Error("appendHint() should preserve IsError=true")
	}
}

func TestAppendHint_NoContentItems(t *testing.T) {
	result := types.ToolResult{Content: []types.ContentItem{}}
	got := appendHint(result, "some hint")

	if len(got.Content) != 0 {
		t.Errorf("appendHint() with no content items should return unchanged, got %d items", len(got.Content))
	}
}
