package resources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleInspectResource_NoWorkspace(t *testing.T) {
	_, err := HandleInspectResource(context.Background(), "", "inspect://last")
	if err == nil {
		t.Fatal("expected error for empty workspace root")
	}
}

func TestHandleInspectResource_NoFile(t *testing.T) {
	dir := t.TempDir()
	result, err := HandleInspectResource(context.Background(), dir, "inspect://last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MIMEType != "application/json" {
		t.Errorf("expected application/json, got %s", result.MIMEType)
	}
	if result.Text == "" {
		t.Error("expected non-empty response for missing file")
	}
}

func TestHandleInspectResource_FileExists(t *testing.T) {
	dir := t.TempDir()
	inspectDir := filepath.Join(dir, ".agent-lsp")
	os.MkdirAll(inspectDir, 0755)
	content := `{"findings":[],"summary":{"errors":0,"warnings":1,"info":2}}`
	os.WriteFile(filepath.Join(inspectDir, "last-inspection.json"), []byte(content), 0644)

	result, err := HandleInspectResource(context.Background(), dir, "inspect://last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != content {
		t.Errorf("expected %q, got %q", content, result.Text)
	}
	if result.URI != "inspect://last" {
		t.Errorf("expected URI inspect://last, got %s", result.URI)
	}
}
