package tools

import (
	"context"
	"strings"
	"testing"
)

// TestRepoForFile verifies repoForFile returns the correct consumer root or "primary".
func TestRepoForFile(t *testing.T) {
	tests := []struct {
		name          string
		filePath      string
		consumerRoots []string
		want          string
	}{
		{
			name:          "matches first root",
			filePath:      "/home/user/consumer-a/pkg/foo.go",
			consumerRoots: []string{"/home/user/consumer-a", "/home/user/consumer-b"},
			want:          "/home/user/consumer-a",
		},
		{
			name:          "matches second root",
			filePath:      "/home/user/consumer-b/cmd/main.go",
			consumerRoots: []string{"/home/user/consumer-a", "/home/user/consumer-b"},
			want:          "/home/user/consumer-b",
		},
		{
			name:          "no match returns primary",
			filePath:      "/home/user/unrelated/foo.go",
			consumerRoots: []string{"/home/user/consumer-a", "/home/user/consumer-b"},
			want:          "primary",
		},
		{
			name:          "empty roots returns primary",
			filePath:      "/home/user/consumer-a/foo.go",
			consumerRoots: []string{},
			want:          "primary",
		},
		{
			name:          "first root wins over deeper nested root listed second",
			filePath:      "/home/user/consumer-a/sub/foo.go",
			consumerRoots: []string{"/home/user/consumer-a", "/home/user/consumer-a/sub"},
			want:          "/home/user/consumer-a",
		},
		{
			name:          "deeper nested root wins when listed first",
			filePath:      "/home/user/consumer-a/sub/foo.go",
			consumerRoots: []string{"/home/user/consumer-a/sub", "/home/user/consumer-a"},
			want:          "/home/user/consumer-a/sub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoForFile(tt.filePath, tt.consumerRoots)
			if got != tt.want {
				t.Errorf("repoForFile(%q, %v) = %q; want %q", tt.filePath, tt.consumerRoots, got, tt.want)
			}
		})
	}
}

// TestHandleGetCrossRepoReferences_EmptyRoots verifies that calling with no
// consumer_roots key returns an error result mentioning "consumer_roots".
func TestHandleGetCrossRepoReferences_EmptyRoots(t *testing.T) {
	result, err := HandleGetCrossRepoReferences(context.Background(), nil, map[string]interface{}{
		"symbol_file": "/some/file.go",
		"line":        float64(1),
		"column":      float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when consumer_roots is missing")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "consumer_roots") {
		t.Errorf("expected error text to contain 'consumer_roots', got: %q", text)
	}
}

// TestHandleGetCrossRepoReferences_EmptyRootsSlice verifies that an explicitly
// empty consumer_roots slice is also rejected with an error about "consumer_roots".
func TestHandleGetCrossRepoReferences_EmptyRootsSlice(t *testing.T) {
	result, err := HandleGetCrossRepoReferences(context.Background(), nil, map[string]interface{}{
		"symbol_file":    "/some/file.go",
		"line":           float64(1),
		"column":         float64(1),
		"consumer_roots": []interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when consumer_roots is empty")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "consumer_roots") {
		t.Errorf("expected error text to contain 'consumer_roots', got: %q", text)
	}
}

// TestHandleGetCrossRepoReferences_NilClient verifies that a nil client is
// rejected by CheckInitialized before consumer_roots validation, returning
// an error about the uninitialized state.
func TestHandleGetCrossRepoReferences_NilClient(t *testing.T) {
	result, err := HandleGetCrossRepoReferences(context.Background(), newNilClient(), map[string]interface{}{
		"symbol_file":    "/some/file.go",
		"line":           float64(1),
		"column":         float64(1),
		"consumer_roots": []interface{}{"/home/user/consumer-a"},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "not initialized") && !strings.Contains(text, "start_lsp") {
		t.Errorf("expected error about nil/uninitialized client, got: %q", text)
	}
}
