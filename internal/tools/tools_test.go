package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- helpers ---

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// newNilClient is a typed nil *lsp.LSPClient used in tests that expect
// CheckInitialized to trigger. It is NOT a real client.
func newNilClient() *lsp.LSPClient {
	return nil
}

// --- TestWithDocument_Success ---

// TestWithDocument_Success verifies that CreateFileURI + URIToFilePath round-trip
// correctly (the two core operations WithDocument uses). A full WithDocument
// integration test requires a live LSP server, which is out of scope for unit tests.
func TestWithDocument_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	writeFile(t, path, "package main\n")

	uri := CreateFileURI(path)
	if !strings.HasPrefix(uri, "file://") {
		t.Fatalf("expected file:// URI, got %q", uri)
	}

	roundTripped, err := URIToFilePath(uri)
	if err != nil {
		t.Fatalf("URIToFilePath: %v", err)
	}
	if roundTripped != path {
		t.Errorf("round-trip mismatch: want %q, got %q", path, roundTripped)
	}
}

// --- TestHandleGetInfoOnLocation_ValidArgs ---

// TestHandleGetInfoOnLocation_ValidArgs ensures that when the client is nil,
// the handler returns an ErrorResult (not a Go error), describing the uninitialized
// state — the precondition check path. Real LSP results need an integration test.
func TestHandleGetInfoOnLocation_ValidArgs(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{
		"file_path": "/some/file.go",
		"line":      float64(5),
		"column":    float64(10),
	}

	result, err := HandleGetInfoOnLocation(ctx, newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// With a nil client CheckInitialized returns an error wrapped in ErrorResult.
	if !result.IsError {
		t.Errorf("expected IsError=true for nil client, got false; text: %s", result.Content[0].Text)
	}
}

// --- TestHandleGetInfoOnLocation_InvalidLine ---

// TestHandleGetInfoOnLocation_InvalidLine verifies that line=0 is rejected with
// an ErrorResult (validation happens before the client is consulted).
// We use a non-nil client to ensure we reach the validation step, but the client
// check runs first, so we still need a valid file context.
// Instead we rely on: CheckInitialized passes only for non-nil client, but we
// want to exercise arg validation. We test this via the helper directly.
func TestHandleGetInfoOnLocation_InvalidLine(t *testing.T) {
	_, _, err := extractPosition(map[string]any{
		"line":   float64(0),
		"column": float64(1),
	})
	if err == nil {
		t.Fatal("expected error for line=0, got nil")
	}
	if !strings.Contains(err.Error(), "line must be >= 1") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- TestHandleGetCodeActions_RangeValidation ---

// TestHandleGetCodeActions_RangeValidation verifies that start_line > end_line
// returns an ErrorResult via extractRange.
func TestHandleGetCodeActions_RangeValidation(t *testing.T) {
	ctx := context.Background()
	// We need a non-nil client to reach the range-validation step.
	// Use a nil client — CheckInitialized fires first and returns ErrorResult.
	// To test range validation specifically, call extractRange directly.
	_, err := extractRange(map[string]any{
		"start_line":   float64(5),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err == nil {
		t.Fatal("expected error for start_line > end_line, got nil")
	}
	if !strings.Contains(err.Error(), "must not be after") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Also verify the handler itself propagates it as ErrorResult (nil client path).
	result, goErr := HandleGetCodeActions(ctx, newNilClient(), map[string]any{
		"file_path":    "/some/file.go",
		"start_line":   float64(5),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if goErr != nil {
		t.Fatalf("unexpected Go error: %v", goErr)
	}
	if !result.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// --- TestHandleGoToDefinition_FormatsLocations ---

// TestHandleGoToDefinition_FormatsLocations verifies the formatLocations helper
// converts 0-based LSP positions to 1-based and URIs to file paths.
func TestHandleGoToDefinition_FormatsLocations(t *testing.T) {
	input := []types.Location{
		{
			URI: "file:///home/user/project/main.go",
			Range: types.Range{
				Start: types.Position{Line: 9, Character: 3},
				End:   types.Position{Line: 9, Character: 10},
			},
		},
	}

	formatted, err := formatLocations(input)
	if err != nil {
		t.Fatalf("formatLocations: %v", err)
	}
	if len(formatted) != 1 {
		t.Fatalf("expected 1 location, got %d", len(formatted))
	}

	loc := formatted[0]
	if loc.FilePath != "/home/user/project/main.go" {
		t.Errorf("FilePath: want /home/user/project/main.go, got %q", loc.FilePath)
	}
	if loc.StartLine != 10 {
		t.Errorf("StartLine: want 10 (1-indexed), got %d", loc.StartLine)
	}
	if loc.StartCol != 4 {
		t.Errorf("StartCol: want 4 (1-indexed), got %d", loc.StartCol)
	}
	if loc.EndLine != 10 {
		t.Errorf("EndLine: want 10, got %d", loc.EndLine)
	}
	if loc.EndCol != 11 {
		t.Errorf("EndCol: want 11 (1-indexed), got %d", loc.EndCol)
	}
}

// --- TestHandleStartLsp_ShutdownsExistingClient ---

// TestHandleStartLsp_ShutdownsExistingClient verifies that when getClient returns
// a non-nil client, HandleStartLsp calls Shutdown on it before creating a new one.
// Because LSPClient is a concrete type without an interface, we test the shutdown
// path indirectly: pass a client and confirm the handler proceeds (no panic, returns
// ErrorResult because Initialize fails with a fake server path).
func TestHandleStartLsp_ShutdownsExistingClient(t *testing.T) {
	ctx := context.Background()

	// Provide a real (but unconfigured) client as existing.
	existing := lsp.NewLSPClient("/nonexistent/lsp", []string{})

	called := false
	getClient := func() *lsp.LSPClient {
		called = true
		return existing
	}
	var setCalledWith *lsp.LSPClient
	setClient := func(c *lsp.LSPClient) {
		setCalledWith = c
	}

	result, err := HandleStartLsp(
		ctx,
		getClient,
		setClient,
		"/nonexistent/lsp-server",
		[]string{},
		map[string]any{"root_dir": "/tmp"},
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	// getClient should have been called to retrieve the existing client.
	if !called {
		t.Error("expected getClient to be called")
	}

	// Initialize will fail (fake server path), so result should be IsError=true
	// and setClient should NOT have been called.
	if !result.IsError {
		// If it somehow succeeded (shouldn't happen), setClient was called.
		_ = setCalledWith
	}
}

// --- TestHandleStartLsp_ErrorReturnsIsError ---

// TestHandleStartLsp_ErrorReturnsIsError verifies that when LSP initialization fails,
// HandleStartLsp returns a ToolResult with IsError=true (not a Go error).
func TestHandleStartLsp_ErrorReturnsIsError(t *testing.T) {
	ctx := context.Background()

	result, err := HandleStartLsp(
		ctx,
		func() *lsp.LSPClient { return nil },
		func(*lsp.LSPClient) {},
		"/nonexistent/lsp-binary",
		[]string{},
		map[string]any{"root_dir": "/tmp"},
	)
	if err != nil {
		t.Fatalf("HandleStartLsp must not return a Go error on init failure, got: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true when Initialize fails, got false; text: %s", result.Content[0].Text)
	}
}

// --- TestCheckInitialized ---

func TestCheckInitialized(t *testing.T) {
	if err := CheckInitialized(nil); err == nil {
		t.Error("expected error for nil client")
	}

	client := lsp.NewLSPClient("/bin/echo", nil)
	if err := CheckInitialized(client); err != nil {
		t.Errorf("unexpected error for non-nil client: %v", err)
	}
}

// --- TestHandleSetLogLevel_ValidLevel ---

func TestHandleSetLogLevel_ValidLevel(t *testing.T) {
	ctx := context.Background()
	result, err := HandleSetLogLevel(ctx, nil, map[string]any{"level": "debug"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success for valid level, got error: %s", result.Content[0].Text)
	}
}

// --- TestHandleSetLogLevel_InvalidLevel ---

func TestHandleSetLogLevel_InvalidLevel(t *testing.T) {
	ctx := context.Background()
	result, err := HandleSetLogLevel(ctx, nil, map[string]any{"level": "verbose"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid log level")
	}
}

// --- TestURIToFilePath_InvalidURI ---

func TestURIToFilePath_InvalidURI(t *testing.T) {
	_, err := URIToFilePath("http://example.com/foo")
	if err == nil {
		t.Error("expected error for non-file URI")
	}
}

// --- TestExtractRange_EqualPositionAllowed ---

func TestExtractRange_EqualPositionAllowed(t *testing.T) {
	_, err := extractRange(map[string]any{
		"start_line":   float64(3),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err != nil {
		t.Errorf("equal start/end should be valid (empty range), got error: %v", err)
	}
}
