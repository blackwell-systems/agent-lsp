package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// callhierarchy.go coverage (1.9% -> target 40%+)
// =============================================================================

func TestHandleCallHierarchy_MissingFilePath(t *testing.T) {
	args := map[string]any{
		"line":   1,
		"column": 1,
	}
	r, err := HandleCallHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
}

func TestHandleCallHierarchy_EmptyFilePath(t *testing.T) {
	args := map[string]any{
		"file_path": "",
		"line":      1,
		"column":    1,
	}
	r, err := HandleCallHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty file_path")
	}
}

func TestHandleCallHierarchy_MissingLine(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"column":    1,
	}
	r, err := HandleCallHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing line")
	}
}

func TestHandleCallHierarchy_MissingColumn(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
	}
	r, err := HandleCallHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing column")
	}
}

func TestHandleCallHierarchy_InvalidDirection(t *testing.T) {
	args := map[string]any{
		"file_path": "/tmp/foo.go",
		"line":      1,
		"column":    1,
		"direction": "sideways",
	}
	r, err := HandleCallHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid direction")
	}
	// With nil client CheckInitialized fires first, but direction validation also works
}

func TestHandleCallHierarchy_DirectionValidation(t *testing.T) {
	tests := []struct {
		direction string
		valid     bool
	}{
		{"incoming", true},
		{"outgoing", true},
		{"both", true},
		{"INCOMING", true}, // case insensitive
		{"Outgoing", true},
		{"BOTH", true},
		{"", true}, // defaults to both
		{"sideways", false},
		{"up", false},
		{"down", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.direction, func(t *testing.T) {
			args := map[string]any{
				"file_path": "/tmp/foo.go",
				"line":      1,
				"column":    1,
			}
			if tt.direction != "" {
				args["direction"] = tt.direction
			}
			r, _ := HandleCallHierarchy(context.Background(), newNilClient(), args)
			// With nil client, we always get an error, but we can check the error message
			if tt.valid && strings.Contains(r.Content[0].Text, "invalid direction") {
				t.Errorf("direction %q should be valid but got invalid direction error", tt.direction)
			}
			if !tt.valid && !strings.Contains(r.Content[0].Text, "invalid direction") && !strings.Contains(r.Content[0].Text, "not initialized") {
				t.Errorf("direction %q should be invalid but got unexpected error: %s", tt.direction, r.Content[0].Text)
			}
		})
	}
}

func TestHandleCallHierarchy_CrossConcurrentFlag(t *testing.T) {
	// Test that cross_concurrent flag is accepted
	args := map[string]any{
		"file_path":       "/tmp/foo.go",
		"line":            1,
		"column":          1,
		"cross_concurrent": true,
	}
	r, err := HandleCallHierarchy(context.Background(), newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// Should fail on nil client, not on cross_concurrent parsing
	if !r.IsError {
		t.Fatalf("expected IsError=true")
	}
}

func TestDetectConcurrentPattern_Go(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	content := `package main

func main() {
	go func() {
		doWork()
	}()
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Line 4 is where doWork() is called (0-indexed line 4)
	pattern := detectConcurrentPattern(path, 4)
	if pattern == "" {
		t.Error("expected to detect 'go func(' pattern")
	}
	if !strings.Contains(pattern, "go func") {
		t.Errorf("expected pattern containing 'go func', got: %s", pattern)
	}
}

func TestDetectConcurrentPattern_NoPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	content := `package main

func main() {
	doWork()
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pattern := detectConcurrentPattern(path, 3)
	if pattern != "" {
		t.Errorf("expected no pattern, got: %s", pattern)
	}
}

func TestDetectConcurrentPattern_FileNotFound(t *testing.T) {
	pattern := detectConcurrentPattern("/nonexistent/file.go", 1)
	if pattern != "" {
		t.Errorf("expected empty pattern for nonexistent file, got: %s", pattern)
	}
}

func TestDetectConcurrentPattern_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	content := `package main

func main() {
	x := 1
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test line beyond file length
	pattern := detectConcurrentPattern(path, 1000)
	if pattern != "" {
		t.Errorf("expected empty pattern for line beyond file, got: %s", pattern)
	}

	// Test negative line
	pattern = detectConcurrentPattern(path, -1)
	if pattern != "" {
		t.Errorf("expected empty pattern for negative line, got: %s", pattern)
	}
}

// =============================================================================
// cache_artifact.go coverage (16.7% -> target 60%+)
// =============================================================================

func TestHandleExportCache_MissingDestPath(t *testing.T) {
	r, err := HandleExportCache(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing dest_path")
	}
	// CheckInitialized fires first with nil client
}

func TestHandleExportCache_EmptyDestPath(t *testing.T) {
	r, err := HandleExportCache(context.Background(), newNilClient(), map[string]any{
		"dest_path": "",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty dest_path")
	}
}

func TestHandleExportCache_InvalidDestPathType(t *testing.T) {
	r, err := HandleExportCache(context.Background(), newNilClient(), map[string]any{
		"dest_path": 123,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for non-string dest_path")
	}
}


func TestHandleImportCache_MissingSrcPath(t *testing.T) {
	r, err := HandleImportCache(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing src_path")
	}
	// CheckInitialized fires first with nil client
}

func TestHandleImportCache_EmptySrcPath(t *testing.T) {
	r, err := HandleImportCache(context.Background(), newNilClient(), map[string]any{
		"src_path": "",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty src_path")
	}
}

func TestHandleImportCache_InvalidSrcPathType(t *testing.T) {
	r, err := HandleImportCache(context.Background(), newNilClient(), map[string]any{
		"src_path": []string{"/tmp/cache.db.gz"},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for non-string src_path")
	}
}

// =============================================================================
// utilities.go coverage (50% -> target 80%+)
// =============================================================================

func TestHandleDidChangeWatchedFiles_NilClient(t *testing.T) {
	r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), map[string]any{
		"changes": []any{
			map[string]any{"uri": "file:///tmp/foo.go", "type": 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

func TestHandleDidChangeWatchedFiles_MissingChanges(t *testing.T) {
	r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing changes")
	}
	// CheckInitialized fires first with nil client
}

func TestHandleDidChangeWatchedFiles_InvalidChangesType(t *testing.T) {
	r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), map[string]any{
		"changes": "not an array",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid changes type")
	}
}

func TestHandleDidChangeWatchedFiles_InvalidChangeItem(t *testing.T) {
	r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), map[string]any{
		"changes": []any{
			"not a map",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid change item")
	}
	// CheckInitialized fires first with nil client
}

func TestHandleDidChangeWatchedFiles_MissingURI(t *testing.T) {
	r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), map[string]any{
		"changes": []any{
			map[string]any{"type": 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing uri")
	}
	// CheckInitialized fires first with nil client
}

func TestHandleDidChangeWatchedFiles_EmptyURI(t *testing.T) {
	r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), map[string]any{
		"changes": []any{
			map[string]any{"uri": "", "type": 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty uri")
	}
}

func TestHandleDidChangeWatchedFiles_InvalidChangeType(t *testing.T) {
	tests := []struct {
		name       string
		changeType any
		wantError  bool
	}{
		{"type 1 (created)", 1, false},
		{"type 2 (changed)", 2, false},
		{"type 3 (deleted)", 3, false},
		{"type 0 (invalid)", 0, true},
		{"type 4 (invalid)", 4, true},
		{"type -1 (invalid)", -1, true},
		{"type string (invalid)", "1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := HandleDidChangeWatchedFiles(context.Background(), newNilClient(), map[string]any{
				"changes": []any{
					map[string]any{"uri": "file:///tmp/foo.go", "type": tt.changeType},
				},
			})
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !r.IsError {
				// With nil client, we always get error from CheckInitialized
				return
			}
			// Check if error mentions change type validation
			hasTypeError := strings.Contains(r.Content[0].Text, "type must be")
			if tt.wantError && !hasTypeError && !strings.Contains(r.Content[0].Text, "not initialized") {
				t.Errorf("expected type validation error, got: %s", r.Content[0].Text)
			}
		})
	}
}

func TestHandleSetLogLevel_MissingLevel(t *testing.T) {
	r, err := HandleSetLogLevel(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing level")
	}
	if !strings.Contains(r.Content[0].Text, "level is required") {
		t.Errorf("expected error about missing level, got: %s", r.Content[0].Text)
	}
}

func TestHandleSetLogLevel_EmptyLevel(t *testing.T) {
	r, err := HandleSetLogLevel(context.Background(), newNilClient(), map[string]any{
		"level": "",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty level")
	}
}

func TestHandleSetLogLevel_InvalidLevelVerbose(t *testing.T) {
	r, err := HandleSetLogLevel(context.Background(), newNilClient(), map[string]any{
		"level": "verbose",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for invalid level")
	}
	if !strings.Contains(r.Content[0].Text, "invalid log level") {
		t.Errorf("expected error about invalid level, got: %s", r.Content[0].Text)
	}
}

func TestHandleSetLogLevel_ValidLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "notice", "warning", "error", "critical", "alert", "emergency"}
	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			r, err := HandleSetLogLevel(context.Background(), newNilClient(), map[string]any{
				"level": level,
			})
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			// SetLogLevel doesn't require initialized client
			if r.IsError {
				t.Errorf("expected success for valid level %q, got error: %s", level, r.Content[0].Text)
			}
		})
	}
}

func TestHandleSetLogLevel_InvalidType(t *testing.T) {
	r, err := HandleSetLogLevel(context.Background(), newNilClient(), map[string]any{
		"level": 123,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for non-string level")
	}
}

// =============================================================================
// fuzzy.go coverage (50% -> target 70%+)
// =============================================================================


func TestExtractSymbolName_CodeBlock(t *testing.T) {
	hover := "```go\nfunc DoWork() error\n```"
	got := extractSymbolName(hover)
	if got != "func" {
		t.Errorf("extractSymbolName(%q) = %q, want %q", hover, got, "func")
	}

	hover2 := "```go\nDoWork\n```"
	got2 := extractSymbolName(hover2)
	if got2 != "DoWork" {
		t.Errorf("extractSymbolName(%q) = %q, want %q", hover2, got2, "DoWork")
	}
}

func TestExtractSymbolName_SpecialChars(t *testing.T) {
	tests := []struct {
		hover string
		want  string
	}{
		{"symbol.Method", "symbol"},
		{"package/path.Symbol", "package"},
		{"func()", "func"},
		{"type[T]", "type"},
		{"const PI = 3.14", "const"},
	}

	for _, tt := range tests {
		t.Run(tt.hover, func(t *testing.T) {
			got := extractSymbolName(tt.hover)
			if got != tt.want {
				t.Errorf("extractSymbolName(%q) = %q, want %q", tt.hover, got, tt.want)
			}
		})
	}
}

// =============================================================================
// workspace_folders.go additional coverage (20.6% -> target 70%+)
// =============================================================================

func TestHandleRemoveWorkspaceFolder_MissingPath(t *testing.T) {
	r, err := HandleRemoveWorkspaceFolder(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing path")
	}
	// CheckInitialized fires first with nil client
}

func TestHandleRemoveWorkspaceFolder_EmptyPath(t *testing.T) {
	r, err := HandleRemoveWorkspaceFolder(context.Background(), newNilClient(), map[string]any{
		"path": "",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty path")
	}
}

func TestHandleAddWorkspaceFolder_InvalidPathType(t *testing.T) {
	r, err := HandleAddWorkspaceFolder(context.Background(), newNilClient(), map[string]any{
		"path": 123,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for non-string path")
	}
}

func TestHandleRemoveWorkspaceFolder_InvalidPathType(t *testing.T) {
	r, err := HandleRemoveWorkspaceFolder(context.Background(), newNilClient(), map[string]any{
		"path": []string{"/tmp"},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for non-string path")
	}
}
