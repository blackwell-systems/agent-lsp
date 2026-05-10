package main

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- computeDelta: both nil ---

func TestComputeDelta_BothNil(t *testing.T) {
	got := computeDelta(nil, nil)
	if got != nil {
		t.Error("expected nil delta when both inputs are nil")
	}
}

// --- computeDelta: zero to errors ---

func TestComputeDelta_ZeroToErrors(t *testing.T) {
	before := &audit.DiagnosticState{ErrorCount: 0, WarningCount: 0}
	after := &audit.DiagnosticState{ErrorCount: 3, WarningCount: 1}
	got := computeDelta(before, after)
	if got == nil {
		t.Fatal("expected non-nil delta")
	}
	if got.Errors != 3 {
		t.Errorf("expected error delta = 3, got %d", got.Errors)
	}
	if got.Warnings != 1 {
		t.Errorf("expected warning delta = 1, got %d", got.Warnings)
	}
}

// --- computeDelta: negative (errors fixed) ---

func TestComputeDelta_ErrorsFixed(t *testing.T) {
	before := &audit.DiagnosticState{ErrorCount: 5, WarningCount: 2}
	after := &audit.DiagnosticState{ErrorCount: 0, WarningCount: 0}
	got := computeDelta(before, after)
	if got == nil {
		t.Fatal("expected non-nil delta")
	}
	if got.Errors != -5 {
		t.Errorf("expected error delta = -5, got %d", got.Errors)
	}
	if got.Warnings != -2 {
		t.Errorf("expected warning delta = -2, got %d", got.Warnings)
	}
}

// --- extractFilesFromWorkspaceEdit: single file ---

func TestExtractFilesFromWorkspaceEdit_SingleFile(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///only.go": []any{},
		},
	}
	got := extractFilesFromWorkspaceEdit(edit)
	if len(got) != 1 {
		t.Errorf("expected 1 file, got %d", len(got))
	}
}

// --- toolResultErrorMsg: error with empty content slice ---

func TestToolResultErrorMsg_EmptyContentSlice(t *testing.T) {
	r := types.ToolResult{IsError: true, Content: []types.ContentItem{}}
	got := toolResultErrorMsg(r)
	if got != "unknown error" {
		t.Errorf("expected 'unknown error', got %q", got)
	}
}

// --- handleGetDaemonStatus with no daemons ---

func TestHandleGetDaemonStatus_NoDaemons(t *testing.T) {
	result := handleGetDaemonStatus()
	if result != "No active daemons." {
		t.Errorf("expected 'No active daemons.', got %q", result)
	}
}

// --- snapshotAllDiagnostics: nil client ---

func TestSnapshotAllDiagnostics_NilClient_Round6(t *testing.T) {
	got := snapshotAllDiagnostics(nil)
	if got != nil {
		t.Error("expected nil for nil client")
	}
}
