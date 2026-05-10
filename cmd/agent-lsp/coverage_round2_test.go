package main

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	"github.com/google/jsonschema-go/jsonschema"
)

// --- computeDelta ---

func TestComputeDelta(t *testing.T) {
	cases := []struct {
		name    string
		before  *audit.DiagnosticState
		after   *audit.DiagnosticState
		wantNil bool
		wantErr int
		wantWrn int
	}{
		{
			"nil before",
			nil,
			&audit.DiagnosticState{ErrorCount: 1},
			true, 0, 0,
		},
		{
			"nil after",
			&audit.DiagnosticState{ErrorCount: 1},
			nil,
			true, 0, 0,
		},
		{
			"both nil",
			nil, nil,
			true, 0, 0,
		},
		{
			"no change",
			&audit.DiagnosticState{ErrorCount: 2, WarningCount: 3},
			&audit.DiagnosticState{ErrorCount: 2, WarningCount: 3},
			false, 0, 0,
		},
		{
			"errors increased",
			&audit.DiagnosticState{ErrorCount: 1, WarningCount: 1},
			&audit.DiagnosticState{ErrorCount: 3, WarningCount: 1},
			false, 2, 0,
		},
		{
			"errors decreased",
			&audit.DiagnosticState{ErrorCount: 5, WarningCount: 2},
			&audit.DiagnosticState{ErrorCount: 2, WarningCount: 0},
			false, -3, -2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			delta := computeDelta(tc.before, tc.after)
			if tc.wantNil {
				if delta != nil {
					t.Errorf("expected nil delta, got %+v", delta)
				}
				return
			}
			if delta == nil {
				t.Fatal("expected non-nil delta")
			}
			if delta.Errors != tc.wantErr {
				t.Errorf("errors: want %d, got %d", tc.wantErr, delta.Errors)
			}
			if delta.Warnings != tc.wantWrn {
				t.Errorf("warnings: want %d, got %d", tc.wantWrn, delta.Warnings)
			}
		})
	}
}

// --- fileToURI ---

func TestFileToURI(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/home/user/file.go", "file:///home/user/file.go"},
		{"/a.py", "file:///a.py"},
	}
	for _, tc := range cases {
		got := fileToURI(tc.path)
		if got != tc.want {
			t.Errorf("fileToURI(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// --- extractFilesFromWorkspaceEdit ---

func TestExtractFilesFromWorkspaceEdit(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///a.go": []any{},
			"file:///b.go": []any{},
		},
	}
	files := extractFilesFromWorkspaceEdit(edit)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestExtractFilesFromWorkspaceEdit_NoChanges(t *testing.T) {
	edit := map[string]any{}
	files := extractFilesFromWorkspaceEdit(edit)
	if files != nil {
		t.Errorf("expected nil for missing changes key, got %v", files)
	}
}

func TestExtractFilesFromWorkspaceEdit_WrongType(t *testing.T) {
	edit := map[string]any{
		"changes": "not-a-map",
	}
	files := extractFilesFromWorkspaceEdit(edit)
	if files != nil {
		t.Errorf("expected nil for wrong type, got %v", files)
	}
}

// --- isToolResultError ---

func TestIsToolResultError(t *testing.T) {
	ok := types.ToolResult{IsError: false}
	if isToolResultError(ok) {
		t.Error("expected false for non-error result")
	}
	errResult := types.ToolResult{IsError: true}
	if !isToolResultError(errResult) {
		t.Error("expected true for error result")
	}
}

// --- toolResultErrorMsg ---

func TestToolResultErrorMsg(t *testing.T) {
	// Non-error result returns empty string.
	ok := types.ToolResult{IsError: false}
	if msg := toolResultErrorMsg(ok); msg != "" {
		t.Errorf("expected empty message for non-error, got %q", msg)
	}

	// Error with content.
	errWithContent := types.ToolResult{
		IsError: true,
		Content: []types.ContentItem{{Text: "something broke"}},
	}
	if msg := toolResultErrorMsg(errWithContent); msg != "something broke" {
		t.Errorf("expected 'something broke', got %q", msg)
	}

	// Error with no content.
	errNoContent := types.ToolResult{
		IsError: true,
		Content: nil,
	}
	if msg := toolResultErrorMsg(errNoContent); msg != "unknown error" {
		t.Errorf("expected 'unknown error', got %q", msg)
	}
}

// --- boolPtr ---

func TestBoolPtr(t *testing.T) {
	pTrue := boolPtr(true)
	if pTrue == nil || *pTrue != true {
		t.Error("boolPtr(true) should return pointer to true")
	}
	pFalse := boolPtr(false)
	if pFalse == nil || *pFalse != false {
		t.Error("boolPtr(false) should return pointer to false")
	}
	// Verify they're distinct pointers.
	if pTrue == pFalse {
		t.Error("boolPtr should return distinct pointers")
	}
}

// --- fixNullableArrays ---

func TestFixNullableArrays_Nil(t *testing.T) {
	// Should not panic on nil.
	fixNullableArrays(nil)
}

func TestFixNullableArrays_CollapsesNullableArray(t *testing.T) {
	schema := &jsonschema.Schema{
		Types: []string{"null", "array"},
	}
	fixNullableArrays(schema)
	if schema.Type != "array" {
		t.Errorf("expected Type=array, got %q", schema.Type)
	}
	if schema.Types != nil {
		t.Errorf("expected Types=nil, got %v", schema.Types)
	}
}

func TestFixNullableArrays_CollapsesNullableObject(t *testing.T) {
	schema := &jsonschema.Schema{
		Types: []string{"null", "object"},
	}
	fixNullableArrays(schema)
	if schema.Type != "object" {
		t.Errorf("expected Type=object, got %q", schema.Type)
	}
	if schema.Types != nil {
		t.Errorf("expected Types=nil, got %v", schema.Types)
	}
}

func TestFixNullableArrays_SingleType(t *testing.T) {
	schema := &jsonschema.Schema{
		Types: []string{"string"},
	}
	fixNullableArrays(schema)
	// Single type should be left alone (len <= 1).
	if schema.Type != "" {
		t.Errorf("single type should not be collapsed, got Type=%q", schema.Type)
	}
}

func TestFixNullableArrays_RecursesIntoProperties(t *testing.T) {
	child := &jsonschema.Schema{
		Types: []string{"null", "array"},
	}
	parent := &jsonschema.Schema{
		Properties: map[string]*jsonschema.Schema{
			"items": child,
		},
	}
	fixNullableArrays(parent)
	if child.Type != "array" {
		t.Errorf("expected nested property Type=array, got %q", child.Type)
	}
}

func TestFixNullableArrays_RecursesIntoItems(t *testing.T) {
	items := &jsonschema.Schema{
		Types: []string{"null", "string"},
	}
	parent := &jsonschema.Schema{
		Type:  "array",
		Items: items,
	}
	fixNullableArrays(parent)
	if items.Type != "string" {
		t.Errorf("expected Items Type=string, got %q", items.Type)
	}
}

// --- snapshotDiagnostics nil client ---

func TestSnapshotDiagnostics_NilClient(t *testing.T) {
	state := snapshotDiagnostics(nil, []string{"/a.go"})
	if state != nil {
		t.Error("expected nil for nil client")
	}
}

// --- snapshotAllDiagnostics nil client ---

func TestSnapshotAllDiagnostics_NilClient(t *testing.T) {
	state := snapshotAllDiagnostics(nil)
	if state != nil {
		t.Error("expected nil for nil client")
	}
}
