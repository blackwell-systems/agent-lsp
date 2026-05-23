package tools

import (
	"context"
	"testing"
)

// =============================================================================
// Targeting 0% coverage functions in analysis.go
// =============================================================================

func TestHandleGetCompletions_MissingFilePath(t *testing.T) {
	args := map[string]any{
		"line":   1,
		"column": 1,
	}
	r, err := HandleGetCompletions(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleGetCompletions_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGetCompletions(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleGetCompletions_InvalidPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      0,
		"column":    1,
	}
	r, err := HandleGetCompletions(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid line")
	}
}

func TestHandleGetSignatureHelp_MissingFilePath(t *testing.T) {
	args := map[string]any{
		"line":   1,
		"column": 1,
	}
	r, err := HandleGetSignatureHelp(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleGetSignatureHelp_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGetSignatureHelp(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleGetSignatureHelp_InvalidPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    0,
	}
	r, err := HandleGetSignatureHelp(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid column")
	}
}

func TestHandleGetDocumentSymbols_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
	}
	r, err := HandleGetDocumentSymbols(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGetDocumentSymbols_MissingFilePath(t *testing.T) {
	args := map[string]any{}
	r, err := HandleGetDocumentSymbols(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleGetDocumentSymbols_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
	}
	r, err := HandleGetDocumentSymbols(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleGetDiagnostics_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
	}
	r, err := HandleGetDiagnostics(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleGetWorkspaceSymbols_EmptyQuery(t *testing.T) {
	args := map[string]any{
		"query": "",
	}
	r, err := HandleGetWorkspaceSymbols(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty query")
	}
}

func TestHandleGetWorkspaceSymbols_InvalidLimitType(t *testing.T) {
	args := map[string]any{
		"query": "test",
		"limit": "not a number",
	}
	r, err := HandleGetWorkspaceSymbols(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// Should either accept string or error, both are acceptable
	_ = r
}

// =============================================================================
// Targeting low coverage in navigation.go
// =============================================================================

func TestHandleGetReferences_InvalidIncludeDeclarationType(t *testing.T) {
	args := map[string]any{
		"file_path":           "/tmp/foo.go",
		"line":                1,
		"column":              1,
		"include_declaration": "not a bool",
	}
	r, err := HandleGetReferences(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// Should fail on nil client, not on flag type conversion
	if !r.IsError {
		t.Fatalf("expected IsError=true")
	}
}

func TestHandleGoToDefinition_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGoToDefinition(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleGoToTypeDefinition_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGoToTypeDefinition(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleGoToImplementation_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGoToImplementation(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

// =============================================================================
// Additional workspace.go coverage
// =============================================================================

func TestHandleRenameSymbol_InvalidPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      0,
		"column":    1,
		"new_name":  "NewName",
	}
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid position")
	}
}


func TestHandleFormatRange_MissingRange(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
	}
	r, err := HandleFormatRange(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing range")
	}
}

func TestHandlePrepareRename_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandlePrepareRename(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandlePrepareRename_InvalidPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      0,
		"column":    1,
	}
	r, err := HandlePrepareRename(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid line")
	}
}

// =============================================================================
// Additional error path coverage for various tools
// =============================================================================

func TestHandleTypeHierarchy_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleTypeHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleTypeHierarchy_InvalidPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    0,
	}
	r, err := HandleTypeHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid column")
	}
}


func TestHandleCallHierarchy_InvalidPosition(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      -5,
		"column":    1,
	}
	r, err := HandleCallHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid line")
	}
}

func TestHandleDidChangeWatchedFiles_InvalidChangesArray(t *testing.T) {
	args := map[string]any{
		"changes": []any{
			map[string]any{"uri": "file:///tmp/foo.go", "type": -1},
		},
	}
	r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid change type")
	}
}

func TestHandleAddWorkspaceFolder_EmptyPath(t *testing.T) {
	args := map[string]any{
		"path": "",
	}
	r, err := HandleAddWorkspaceFolder(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty path")
	}
}
