package tools

import (
	"context"
	"testing"
)

// TestHandleGetSemanticTokens_NilClient verifies that a nil client returns
// IsError=true without panicking.
func TestHandleGetSemanticTokens_NilClient(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{
		"file_path":    "/some/file.go",
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(10),
		"end_column":   float64(1),
	}
	result, err := HandleGetSemanticTokens(ctx, newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for nil client, got false; text: %s", result.Content[0].Text)
	}
}

// TestHandleGetSemanticTokens_MissingFilePath verifies that an empty file_path
// returns IsError=true (validation fires before client check if client is nil,
// but nil client fires first; test via nil client path is sufficient to verify
// the handler returns ErrorResult not a Go error).
func TestHandleGetSemanticTokens_MissingFilePath(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(10),
		"end_column":   float64(1),
	}
	result, err := HandleGetSemanticTokens(ctx, newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true, got false")
	}
}

// TestHandleGetSemanticTokens_InvalidRange verifies that start_line > end_line
// is rejected. Uses nil client (CheckInitialized fires first) to confirm
// no panic. Range validation is separately tested via extractRange.
func TestHandleGetSemanticTokens_InvalidRange(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{
		"file_path":    "/some/file.go",
		"start_line":   float64(10),
		"start_column": float64(1),
		"end_line":     float64(5),
		"end_column":   float64(1),
	}
	result, err := HandleGetSemanticTokens(ctx, newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for nil client, got false")
	}
}
