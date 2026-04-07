package tools

import (
	"context"
	"strings"
	"testing"
)

// TestHandleCreateSimulationSession_MissingWorkspaceRoot tests that missing workspace_root returns error.
func TestHandleCreateSimulationSession_MissingWorkspaceRoot(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"language": "go",
	}

	result, err := HandleCreateSimulationSession(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "workspace_root is required") {
		t.Errorf("expected 'workspace_root is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleCreateSimulationSession_MissingLanguage tests that missing language returns error.
func TestHandleCreateSimulationSession_MissingLanguage(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"workspace_root": "/tmp/test",
	}

	result, err := HandleCreateSimulationSession(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "language is required") {
		t.Errorf("expected 'language is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateEdit_MissingSessionID tests that missing session_id returns error.
func TestHandleSimulateEdit_MissingSessionID(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"file_path":    "/tmp/test.go",
		"start_line":   1,
		"start_column": 1,
		"end_line":     1,
		"end_column":   10,
		"new_text":     "foo",
	}

	result, err := HandleSimulateEdit(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "session_id is required") {
		t.Errorf("expected 'session_id is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateEdit_MissingFilePath tests that missing file_path returns error.
func TestHandleSimulateEdit_MissingFilePath(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"session_id":   "test-session",
		"start_line":   1,
		"start_column": 1,
		"end_line":     1,
		"end_column":   10,
		"new_text":     "foo",
	}

	result, err := HandleSimulateEdit(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "file_path is required") {
		t.Errorf("expected 'file_path is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateEdit_InvalidRange tests that invalid range values return error.
func TestHandleSimulateEdit_InvalidRange(t *testing.T) {
	testCases := []struct {
		name        string
		args        map[string]interface{}
		wantErrText string
	}{
		{
			name: "missing start_line",
			args: map[string]interface{}{
				"session_id":   "test-session",
				"file_path":    "/tmp/test.go",
				"start_column": 1,
				"end_line":     1,
				"end_column":   10,
				"new_text":     "foo",
			},
			wantErrText: "start_line",
		},
		{
			name: "zero start_line",
			args: map[string]interface{}{
				"session_id":   "test-session",
				"file_path":    "/tmp/test.go",
				"start_line":   0,
				"start_column": 1,
				"end_line":     1,
				"end_column":   10,
				"new_text":     "foo",
			},
			wantErrText: "start_line must be >= 1",
		},
		{
			name: "start after end",
			args: map[string]interface{}{
				"session_id":   "test-session",
				"file_path":    "/tmp/test.go",
				"start_line":   2,
				"start_column": 1,
				"end_line":     1,
				"end_column":   10,
				"new_text":     "foo",
			},
			wantErrText: "start position",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := HandleSimulateEdit(ctx, nil, tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !result.IsError {
				t.Errorf("expected error result, got success")
			}
			if !strings.Contains(result.Content[0].Text, tc.wantErrText) {
				t.Errorf("expected error containing %q, got: %s", tc.wantErrText, result.Content[0].Text)
			}
		})
	}
}

// TestHandleEvaluateSession_MissingSessionID tests that missing session_id returns error.
func TestHandleEvaluateSession_MissingSessionID(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := HandleEvaluateSession(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "session_id is required") {
		t.Errorf("expected 'session_id is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateChain_MissingEdits tests that missing edits array returns error.
func TestHandleSimulateChain_MissingEdits(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"session_id": "test-session",
	}

	result, err := HandleSimulateChain(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "edits array is required") {
		t.Errorf("expected 'edits array is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateChain_EmptyEdits tests that empty edits array returns error.
func TestHandleSimulateChain_EmptyEdits(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"session_id": "test-session",
		"edits":      []interface{}{},
	}

	result, err := HandleSimulateChain(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "edits array is required") {
		t.Errorf("expected 'edits array is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleCommitSession_MissingSessionID tests that missing session_id returns error.
func TestHandleCommitSession_MissingSessionID(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := HandleCommitSession(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "session_id is required") {
		t.Errorf("expected 'session_id is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleDestroySession_MissingSessionID tests that missing session_id returns error.
func TestHandleDestroySession_MissingSessionID(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{}

	result, err := HandleDestroySession(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "session_id is required") {
		t.Errorf("expected 'session_id is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateEditAtomic_MissingWorkspaceRoot tests that missing workspace_root returns error.
func TestHandleSimulateEditAtomic_MissingWorkspaceRoot(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"language":     "go",
		"file_path":    "/tmp/test.go",
		"start_line":   1,
		"start_column": 1,
		"end_line":     1,
		"end_column":   10,
		"new_text":     "foo",
	}

	result, err := HandleSimulateEditAtomic(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "workspace_root is required") {
		t.Errorf("expected 'workspace_root is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateEditAtomic_MissingLanguage tests that missing language returns error.
func TestHandleSimulateEditAtomic_MissingLanguage(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"workspace_root": "/tmp/test",
		"file_path":      "/tmp/test.go",
		"start_line":     1,
		"start_column":   1,
		"end_line":       1,
		"end_column":     10,
		"new_text":       "foo",
	}

	result, err := HandleSimulateEditAtomic(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "language is required") {
		t.Errorf("expected 'language is required' error, got: %s", result.Content[0].Text)
	}
}

// TestHandleSimulateEditAtomic_MissingFilePath tests that missing file_path returns error.
func TestHandleSimulateEditAtomic_MissingFilePath(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"workspace_root": "/tmp/test",
		"language":       "go",
		"start_line":     1,
		"start_column":   1,
		"end_line":       1,
		"end_column":     10,
		"new_text":       "foo",
	}

	result, err := HandleSimulateEditAtomic(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Errorf("expected error result, got success")
	}
	if !strings.Contains(result.Content[0].Text, "file_path is required") {
		t.Errorf("expected 'file_path is required' error, got: %s", result.Content[0].Text)
	}
}
