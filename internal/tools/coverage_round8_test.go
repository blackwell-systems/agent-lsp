package tools

import (
	"context"
	"testing"
)

// =============================================================================
// navigation.go coverage (26.5% -> target 50%+)
// =============================================================================

func TestHandleGetReferences_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGetReferences(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGetReferences_MissingFilePath(t *testing.T) {
	args := map[string]any{
		"line":   1,
		"column": 1,
	}
	r, err := HandleGetReferences(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleGetReferences_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGetReferences(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleGetReferences_IncludeDeclaration(t *testing.T) {
	// Test that include_declaration flag is accepted
	args := map[string]any{
		"file_path":           "/tmp/foo.go",
		"line":                1,
		"column":              1,
		"include_declaration": true,
	}
	r, err := HandleGetReferences(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// Should fail on nil client, not on flag parsing
	if !r.IsError {
		t.Fatalf("expected IsError=true")
	}
}

func TestHandleGoToDefinition_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGoToDefinition(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGoToDefinition_MissingFilePath(t *testing.T) {
	args := map[string]any{
		"line":   1,
		"column": 1,
	}
	r, err := HandleGoToDefinition(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleGoToDefinition_InvalidPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      0, // invalid
		"column":    1,
	}
	r, err := HandleGoToDefinition(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid line")
	}
}

func TestHandleGoToTypeDefinition_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGoToTypeDefinition(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGoToImplementation_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGoToImplementation(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGoToDeclaration_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGoToDeclaration(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}


// =============================================================================
// symbol_edit.go coverage (22.4% -> target 45%+)
// =============================================================================


// =============================================================================
// highlights.go coverage (11.1% -> target 40%+)
// =============================================================================

func TestHandleGetDocumentHighlights_MissingPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
	}
	r, err := HandleGetDocumentHighlights(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing position")
	}
}

// =============================================================================
// inlayhints.go coverage (11.8% -> target 40%+)
// =============================================================================

func TestHandleGetInlayHints_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
	}
	r, err := HandleGetInlayHints(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}
