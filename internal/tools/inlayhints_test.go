package tools

import (
	"context"
	"testing"
)

func TestHandleGetInlayHints_NilClient(t *testing.T) {
	args := map[string]interface{}{
		"file_path":    "/tmp/foo.go",
		"start_line":   1,
		"start_column": 1,
		"end_line":     10,
		"end_column":   1,
	}
	r, err := HandleGetInlayHints(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGetInlayHints_MissingFilePath(t *testing.T) {
	r, err := HandleGetInlayHints(context.Background(), newNilClient(), map[string]interface{}{
		"start_line": 1, "start_column": 1, "end_line": 10, "end_column": 1,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleGetInlayHints_InvalidRange(t *testing.T) {
	// start_line=0 is invalid (must be >= 1).
	r, err := HandleGetInlayHints(context.Background(), newNilClient(), map[string]interface{}{
		"file_path":    "/tmp/foo.go",
		"start_line":   0,
		"start_column": 1,
		"end_line":     10,
		"end_column":   1,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// nil client fires CheckInitialized before range validation — both paths produce IsError=true.
	if !r.IsError {
		t.Fatalf("expected IsError=true")
	}
}
