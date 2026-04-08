package tools

import (
	"context"
	"testing"
)

func TestHandleAddWorkspaceFolder_NilClient(t *testing.T) {
	r, err := HandleAddWorkspaceFolder(context.Background(), newNilClient(), map[string]interface{}{
		"path": "/tmp/other-project",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleAddWorkspaceFolder_MissingPath(t *testing.T) {
	r, err := HandleAddWorkspaceFolder(context.Background(), newNilClient(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing path")
	}
}

func TestHandleRemoveWorkspaceFolder_NilClient(t *testing.T) {
	r, err := HandleRemoveWorkspaceFolder(context.Background(), newNilClient(), map[string]interface{}{
		"path": "/tmp/other-project",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleListWorkspaceFolders_NilClient(t *testing.T) {
	r, err := HandleListWorkspaceFolders(context.Background(), newNilClient(), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}
