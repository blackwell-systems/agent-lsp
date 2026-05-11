package tools

import (
	"context"
	"strings"
	"testing"
)

func TestHandleSafeApplyEdit_NilClient(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"file_path": "/tmp/test.go",
		"old_text":  "foo",
		"new_text":  "bar",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
	if !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Fatalf("expected init error, got: %s", r.Content[0].Text)
	}
}

func TestHandleSafeApplyEdit_MissingFilePath(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"old_text": "foo",
		"new_text": "bar",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
	if !strings.Contains(r.Content[0].Text, "file_path") {
		t.Fatalf("expected file_path error, got: %s", r.Content[0].Text)
	}
}

func TestHandleSafeApplyEdit_MissingOldText(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"file_path": "/tmp/test.go",
		"new_text":  "bar",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing old_text")
	}
	if !strings.Contains(r.Content[0].Text, "old_text") {
		t.Fatalf("expected old_text error, got: %s", r.Content[0].Text)
	}
}

func TestHandleSafeApplyEdit_MissingNewText(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"file_path": "/tmp/test.go",
		"old_text":  "foo",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing new_text")
	}
	if !strings.Contains(r.Content[0].Text, "new_text") {
		t.Fatalf("expected new_text error, got: %s", r.Content[0].Text)
	}
}
