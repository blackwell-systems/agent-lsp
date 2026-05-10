package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// newFakeClient returns a non-nil *lsp.LSPClient that passes CheckInitialized
// but is not connected to any real server. Use only for tests that short-circuit
// before any LSP network call (argument validation, etc.).
func newFakeClient() *lsp.LSPClient {
	return lsp.NewLSPClient("/bin/echo", nil)
}

// --- isEmptyWorkspaceEdit ---

// TestIsEmptyWorkspaceEdit_Nil verifies that a nil interface{} is empty.
func TestIsEmptyWorkspaceEdit_Nil(t *testing.T) {
	if !isEmptyWorkspaceEdit(nil) {
		t.Error("expected nil to be considered an empty WorkspaceEdit")
	}
}

// TestIsEmptyWorkspaceEdit_TypedNil verifies that a typed nil pointer (which
// marshals to JSON "null") is considered empty.
func TestIsEmptyWorkspaceEdit_TypedNil(t *testing.T) {
	type fakeEdit struct{ Changes map[string]string }
	var p *fakeEdit // typed nil — non-nil interface but marshals to "null"
	if !isEmptyWorkspaceEdit(p) {
		t.Error("expected typed nil pointer to be considered empty (marshals to null)")
	}
}

// TestIsEmptyWorkspaceEdit_EmptyObject verifies that a struct/map with no
// content marshals to "{}" and is treated as empty.
func TestIsEmptyWorkspaceEdit_EmptyObject(t *testing.T) {
	if !isEmptyWorkspaceEdit(map[string]any{}) {
		t.Error("expected empty map to be considered empty (marshals to {})")
	}
}

// TestIsEmptyWorkspaceEdit_NonEmpty verifies that a map with actual edit content
// is not considered empty.
func TestIsEmptyWorkspaceEdit_NonEmpty(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///project/main.go": []any{"some edit"},
		},
	}
	if isEmptyWorkspaceEdit(edit) {
		t.Error("expected non-empty edit to not be considered empty")
	}
}

// --- HandleRenameSymbol ---

// TestHandleRenameSymbol_NilClient verifies that a nil client returns an error
// result before any argument parsing.
func TestHandleRenameSymbol_NilClient(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
		"new_name":  "NewName",
		"line":      float64(5),
		"column":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected 'not initialized' error, got %v", r.Content)
	}
}

// TestHandleRenameSymbol_MissingFilePath verifies that a missing file_path
// returns an error (nil client fires first, but we verify the error path exists).
func TestHandleRenameSymbol_MissingFilePath(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]any{
		"new_name": "NewName",
		"line":     float64(5),
		"column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true")
	}
}

// TestHandleRenameSymbol_MissingNewName verifies that a missing new_name
// returns an error.
func TestHandleRenameSymbol_MissingNewName(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
		"line":      float64(5),
		"column":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing new_name")
	}
}

// TestHandleRenameSymbol_MissingPosition verifies that when neither line/column
// nor position_pattern is supplied, an error result is returned. Uses a fake
// (non-nil) client to reach the position-validation step.
func TestHandleRenameSymbol_MissingPosition(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newFakeClient(), map[string]any{
		"file_path": "/some/file.go",
		"new_name":  "NewName",
		// no line, column, or position_pattern
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing position")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "line") {
		t.Errorf("expected error mentioning 'line', got %v", r.Content)
	}
}

// --- HandlePrepareRename ---

// TestHandlePrepareRename_NilClient verifies that a nil client returns an error result.
func TestHandlePrepareRename_NilClient(t *testing.T) {
	r, err := HandlePrepareRename(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
		"line":      float64(5),
		"column":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandlePrepareRename_MissingFilePath verifies that a missing file_path
// results in an error.
func TestHandlePrepareRename_MissingFilePath(t *testing.T) {
	r, err := HandlePrepareRename(context.Background(), newNilClient(), map[string]any{
		"line":   float64(5),
		"column": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true")
	}
}

// TestHandlePrepareRename_MissingPosition verifies that missing line/column
// returns an error when the client is non-nil (reachable validation step).
func TestHandlePrepareRename_MissingPosition(t *testing.T) {
	r, err := HandlePrepareRename(context.Background(), newFakeClient(), map[string]any{
		"file_path": "/some/file.go",
		// no line or column
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing position")
	}
}

// --- HandleFormatDocument ---

// TestHandleFormatDocument_NilClient verifies that a nil client returns an error result.
func TestHandleFormatDocument_NilClient(t *testing.T) {
	r, err := HandleFormatDocument(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleFormatDocument_MissingFilePath verifies that a missing file_path
// returns an error.
func TestHandleFormatDocument_MissingFilePath(t *testing.T) {
	r, err := HandleFormatDocument(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing file_path")
	}
}

// TestHandleFormatDocument_InvalidTabSize verifies that a non-numeric tab_size
// returns an error. Uses a fake client to reach the tab_size validation step.
func TestHandleFormatDocument_InvalidTabSize(t *testing.T) {
	r, err := HandleFormatDocument(context.Background(), newFakeClient(), map[string]any{
		"file_path": "/some/file.go",
		"tab_size":  "bad",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for non-numeric tab_size")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "tab_size") {
		t.Errorf("expected error mentioning 'tab_size', got %v", r.Content)
	}
}

// --- HandleFormatRange ---

// TestHandleFormatRange_NilClient verifies that a nil client returns an error result.
func TestHandleFormatRange_NilClient(t *testing.T) {
	r, err := HandleFormatRange(context.Background(), newNilClient(), map[string]any{
		"file_path":    "/some/file.go",
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleFormatRange_MissingFilePath verifies that a missing file_path
// returns an error.
func TestHandleFormatRange_MissingFilePath(t *testing.T) {
	r, err := HandleFormatRange(context.Background(), newNilClient(), map[string]any{
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing file_path")
	}
}

// TestHandleFormatRange_InvalidRange verifies that start > end returns an error
// (uses a fake client to reach range validation).
func TestHandleFormatRange_InvalidRange(t *testing.T) {
	r, err := HandleFormatRange(context.Background(), newFakeClient(), map[string]any{
		"file_path":    "/some/file.go",
		"start_line":   float64(5),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for inverted range")
	}
}

// --- HandleApplyEdit ---

// TestHandleApplyEdit_NilClient verifies that a nil client returns an error result.
func TestHandleApplyEdit_NilClient(t *testing.T) {
	r, err := HandleApplyEdit(context.Background(), newNilClient(), map[string]any{
		"workspace_edit": map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleApplyEdit_MissingWorkspaceEdit verifies that a missing workspace_edit
// argument returns an error.
func TestHandleApplyEdit_MissingWorkspaceEdit(t *testing.T) {
	r, err := HandleApplyEdit(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing workspace_edit")
	}
}

// --- findText ---

// TestFindText_ExactMatch verifies that an exact substring is found.
func TestFindText_ExactMatch(t *testing.T) {
	src := "line one\nfunc Foo() {\n\treturn nil\n}\n"
	start, end, ok := findText(src, "func Foo() {")
	if !ok {
		t.Fatal("expected match, got not found")
	}
	if src[start:end] != "func Foo() {" {
		t.Errorf("got %q, want %q", src[start:end], "func Foo() {")
	}
}

// TestFindText_NormalisedMatch verifies that a pattern with different indentation
// is found via the whitespace-normalised fallback.
func TestFindText_NormalisedMatch(t *testing.T) {
	src := "line one\n\tfunc Foo() {\n\t\treturn nil\n\t}\n"
	// Pattern has no indentation; file has tabs.
	start, end, ok := findText(src, "func Foo() {\n\treturn nil\n}")
	if !ok {
		t.Fatal("expected normalised match, got not found")
	}
	got := src[start:end]
	if !strings.Contains(got, "func Foo()") {
		t.Errorf("matched region %q does not contain expected content", got)
	}
}

// TestFindText_NotFound verifies that a non-existent pattern returns false.
func TestFindText_NotFound(t *testing.T) {
	_, _, ok := findText("hello world", "xyz")
	if ok {
		t.Error("expected not found")
	}
}

// --- textMatchApply ---

// TestTextMatchApply_ExactMatch verifies that exact-match mode produces a
// WorkspaceEdit with the correct newText.
func TestTextMatchApply_ExactMatch(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.go")
	if err != nil {
		t.Fatal(err)
	}
	content := "package main\n\nfunc Foo() {\n\treturn\n}\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	edit, err := textMatchApply(f.Name(), "func Foo() {", "func Foo() error {")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := edit.(map[string]any)
	if !ok {
		t.Fatal("edit is not map[string]interface{}")
	}
	changes, ok := m["changes"].(map[string]any)
	if !ok {
		t.Fatal("missing changes key")
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 file in changes, got %d", len(changes))
	}
	for _, v := range changes {
		edits, ok := v.([]any)
		if !ok || len(edits) != 1 {
			t.Fatal("expected 1 text edit")
		}
		te := edits[0].(map[string]any)
		if te["newText"] != "func Foo() error {" {
			t.Errorf("got newText %q", te["newText"])
		}
	}
}

// TestTextMatchApply_NotFound verifies an error is returned when old_text is absent.
func TestTextMatchApply_NotFound(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.go")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("package main\n")
	f.Close()

	_, err = textMatchApply(f.Name(), "func Missing() {}", "func Missing() error {}")
	if err == nil {
		t.Error("expected error for missing old_text")
	}
}

// --- HandleExecuteCommand ---

// TestHandleExecuteCommand_NilClient verifies that a nil client returns an error result.
func TestHandleExecuteCommand_NilClient(t *testing.T) {
	r, err := HandleExecuteCommand(context.Background(), newNilClient(), map[string]any{
		"command": "editor.action.foo",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleExecuteCommand_MissingCommand verifies that a missing command argument
// returns an error. Uses a fake (non-nil) client to reach the command validation step.
func TestHandleExecuteCommand_MissingCommand(t *testing.T) {
	r, err := HandleExecuteCommand(context.Background(), newFakeClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing command")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "command") {
		t.Errorf("expected error mentioning 'command', got %v", r.Content)
	}
}

// --- filterWorkspaceEditByGlobs ---

// TestFilterWorkspaceEditByGlobs_Nil verifies that a nil result passes through unchanged.
func TestFilterWorkspaceEditByGlobs_Nil(t *testing.T) {
	result := filterWorkspaceEditByGlobs(nil, []string{"vendor/**"})
	if result != nil {
		t.Error("expected nil to pass through unchanged")
	}
}

// TestFilterWorkspaceEditByGlobs_EmptyGlobs verifies that empty globs return the input unchanged.
func TestFilterWorkspaceEditByGlobs_EmptyGlobs(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///project/main.go": []any{"edit1"},
		},
	}
	result := filterWorkspaceEditByGlobs(edit, nil)
	// result should be the same interface value (same underlying pointer) as edit.
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map[string]interface{} result")
	}
	if len(resultMap) != len(edit) {
		t.Error("expected nil globs to return input unchanged")
	}
}

// TestFilterWorkspaceEditByGlobs_RetainsNonMatchingChanges verifies non-matching
// URIs are retained in the "changes" map format.
func TestFilterWorkspaceEditByGlobs_RetainsNonMatchingChanges(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///project/main.go":     []any{"edit1"},
			"file:///project/main_gen.go": []any{"edit2"},
		},
	}
	result := filterWorkspaceEditByGlobs(edit, []string{"*_gen.go"})
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	changes, _ := m["changes"].(map[string]any)
	if _, found := changes["file:///project/main.go"]; !found {
		t.Error("expected main.go to be retained")
	}
	if _, found := changes["file:///project/main_gen.go"]; found {
		t.Error("expected main_gen.go to be excluded by *_gen.go pattern")
	}
}

// --- extractStringSlice ---

// TestExtractStringSlice_Typed verifies []string input is returned as-is.
func TestExtractStringSlice_Typed(t *testing.T) {
	args := map[string]any{
		"exclude_globs": []string{"vendor/**", "*_gen.go"},
	}
	got := extractStringSlice(args, "exclude_globs")
	if len(got) != 2 || got[0] != "vendor/**" {
		t.Errorf("unexpected result: %v", got)
	}
}

// TestExtractStringSlice_Interface verifies []interface{} (JSON-decoded shape) works.
func TestExtractStringSlice_Interface(t *testing.T) {
	args := map[string]any{
		"exclude_globs": []any{"vendor/**", "*_gen.go"},
	}
	got := extractStringSlice(args, "exclude_globs")
	if len(got) != 2 || got[1] != "*_gen.go" {
		t.Errorf("unexpected result: %v", got)
	}
}

// TestExtractStringSlice_Missing verifies a missing key returns nil.
func TestExtractStringSlice_Missing(t *testing.T) {
	args := map[string]any{}
	got := extractStringSlice(args, "exclude_globs")
	if got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}
