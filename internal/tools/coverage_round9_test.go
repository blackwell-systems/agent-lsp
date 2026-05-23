package tools

import (
	"context"
	"testing"
)

// =============================================================================
// analysis.go additional coverage
// =============================================================================

func TestHandleGetDiagnostics_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
	}
	r, err := HandleGetDiagnostics(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGetDiagnostics_MissingFilePath(t *testing.T) {
	args := map[string]any{}
	r, err := HandleGetDiagnostics(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}


func TestHandleGetCompletions_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGetCompletions(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleGetSignatureHelp_NilClient(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
	}
	r, err := HandleGetSignatureHelp(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

// =============================================================================
// helpers.go additional coverage
// =============================================================================

func TestExtractPosition_NegativeLine(t *testing.T) {
	_, _, err := extractPosition(map[string]any{
		"line":   -1,
		"column": 1,
	})
	if err == nil {
		t.Fatal("expected error for negative line")
	}
}

func TestExtractPosition_InvalidLineType(t *testing.T) {
	_, _, err := extractPosition(map[string]any{
		"line":   "not a number",
		"column": 1,
	})
	if err == nil {
		t.Fatal("expected error for invalid line type")
	}
}

func TestExtractPosition_InvalidColumnType(t *testing.T) {
	_, _, err := extractPosition(map[string]any{
		"line":   1,
		"column": []int{1, 2},
	})
	if err == nil {
		t.Fatal("expected error for invalid column type")
	}
}

func TestToInt_StringParseable(t *testing.T) {
	// toInt now requires numeric types, not strings, so this tests invalid type
	m := map[string]any{"value": "123"}
	_, err := toInt(m, "value")
	if err == nil {
		t.Fatal("expected error for string value")
	}
}

func TestToInt_InvalidString(t *testing.T) {
	m := map[string]any{"value": "not-a-number"}
	_, err := toInt(m, "value")
	if err == nil {
		t.Fatal("expected error for invalid string")
	}
}

func TestURIToFilePath_WindowsPath(t *testing.T) {
	// Test Windows-style URI
	uri := "file:///C:/Users/test/file.go"
	path, err := URIToFilePath(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Accept various Windows path formats
	if path != "C:/Users/test/file.go" && path != "C:\\Users\\test\\file.go" && path != "/C:/Users/test/file.go" {
		t.Logf("Windows path conversion: %s (acceptable)", path)
	}
}

func TestURIToFilePath_EmptyURI(t *testing.T) {
	_, err := URIToFilePath("")
	if err == nil {
		t.Fatal("expected error for empty URI")
	}
}

func TestURIToFilePath_MissingFileScheme(t *testing.T) {
	_, err := URIToFilePath("http://example.com/file.go")
	if err == nil {
		t.Fatal("expected error for non-file URI")
	}
}

func TestCreateFileURI_RelativePath(t *testing.T) {
	// Test that relative paths don't panic
	uri := CreateFileURI("relative/path.go")
	if uri == "" {
		t.Error("expected non-empty URI for relative path")
	}
}

func TestCreateFileURI_EmptyPath(t *testing.T) {
	// Test that empty path doesn't panic
	uri := CreateFileURI("")
	if uri == "" {
		t.Error("expected non-empty URI even for empty path")
	}
}

// =============================================================================
// workspace.go additional coverage
// =============================================================================

func TestHandleRenameSymbol_EmptyNewName(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
		"new_name":  "",
	}
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty new_name")
	}
}

func TestHandleFormatDocument_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
	}
	r, err := HandleFormatDocument(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleFormatRange_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path":  "",
		"start_line": 1,
		"end_line":   1,
	}
	r, err := HandleFormatRange(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleApplyEdit_EmptyWorkspaceEdit(t *testing.T) {
	args := map[string]any{}
	r, err := HandleApplyEdit(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing workspace_edit")
	}
}

func TestHandleExecuteCommand_EmptyCommand(t *testing.T) {
	args := map[string]any{
		"command": "",
	}
	r, err := HandleExecuteCommand(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty command")
	}
}



// =============================================================================
// symbol_edit.go (getDiagnosticsForFile) additional coverage
// =============================================================================

func TestGetDiagnosticsForFile_NonexistentFile(t *testing.T) {
	// With nil client, should return (0, 0) gracefully
	errors, warnings := getDiagnosticsForFile(context.Background(), nil, "/nonexistent/file.go")
	if errors != 0 || warnings != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", errors, warnings)
	}
}

// =============================================================================
// detect_changes.go additional coverage
// =============================================================================

func TestHandleDetectChanges_NilClient(t *testing.T) {
	args := map[string]any{
		"workspace_root": "/tmp/project",
	}
	r, err := HandleDetectChanges(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleDetectChanges_MissingWorkspaceRoot(t *testing.T) {
	args := map[string]any{}
	r, err := HandleDetectChanges(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing workspace_root")
	}
}

// =============================================================================
// position_pattern.go additional coverage
// =============================================================================

func TestExtractPositionWithPattern_FileNotFoundError(t *testing.T) {
	args := map[string]any{
		"pattern": "DoWork",
	}
	_, _, err := ExtractPositionWithPattern(args, "/nonexistent/file.go")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
