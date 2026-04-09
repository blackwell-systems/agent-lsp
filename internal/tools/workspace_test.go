package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
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
	if !isEmptyWorkspaceEdit(map[string]interface{}{}) {
		t.Error("expected empty map to be considered empty (marshals to {})")
	}
}

// TestIsEmptyWorkspaceEdit_NonEmpty verifies that a map with actual edit content
// is not considered empty.
func TestIsEmptyWorkspaceEdit_NonEmpty(t *testing.T) {
	edit := map[string]interface{}{
		"changes": map[string]interface{}{
			"file:///project/main.go": []interface{}{"some edit"},
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
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandleRenameSymbol(context.Background(), newFakeClient(), map[string]interface{}{
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
	r, err := HandlePrepareRename(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandlePrepareRename(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandlePrepareRename(context.Background(), newFakeClient(), map[string]interface{}{
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
	r, err := HandleFormatDocument(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandleFormatDocument(context.Background(), newNilClient(), map[string]interface{}{})
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
	r, err := HandleFormatDocument(context.Background(), newFakeClient(), map[string]interface{}{
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
	r, err := HandleFormatRange(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandleFormatRange(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandleFormatRange(context.Background(), newFakeClient(), map[string]interface{}{
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
	r, err := HandleApplyEdit(context.Background(), newNilClient(), map[string]interface{}{
		"workspace_edit": map[string]interface{}{},
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
	r, err := HandleApplyEdit(context.Background(), newNilClient(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing workspace_edit")
	}
}

// --- HandleExecuteCommand ---

// TestHandleExecuteCommand_NilClient verifies that a nil client returns an error result.
func TestHandleExecuteCommand_NilClient(t *testing.T) {
	r, err := HandleExecuteCommand(context.Background(), newNilClient(), map[string]interface{}{
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
	r, err := HandleExecuteCommand(context.Background(), newFakeClient(), map[string]interface{}{})
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
