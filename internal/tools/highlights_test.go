package tools

import (
	"context"
	"testing"
)

func TestHandleGetDocumentHighlights_NilClient(t *testing.T) {
	r, err := HandleGetDocumentHighlights(context.Background(), newNilClient(), map[string]interface{}{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGetDocumentHighlights_MissingFilePath(t *testing.T) {
	r, err := HandleGetDocumentHighlights(context.Background(), newNilClient(), map[string]interface{}{
		"line": 1, "column": 1,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleGetDocumentHighlights_MissingLine(t *testing.T) {
	r, err := HandleGetDocumentHighlights(context.Background(), newNilClient(), map[string]interface{}{
		"file_path": "/tmp/foo.go",
		"column":    1,
		// line intentionally missing
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing line")
	}
}
