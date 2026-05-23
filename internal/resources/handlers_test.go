package resources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// --- HandleDiagnosticsResource unit tests ---

func TestHandleDiagnosticsResource_InvalidURI(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	// Use invalid URI with null byte
	_, err := HandleDiagnosticsResource(context.Background(), client, "lsp-diagnostics://\x00invalid")
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
}

func TestHandleDiagnosticsResource_URIVariants(t *testing.T) {
	// Test that different URI formats are parseable
	// These will fail later due to no running server, but URI parsing should work
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	testCases := []struct {
		name      string
		uri       string
		shouldErr bool
	}{
		{"empty path", "lsp-diagnostics://", false},
		{"root path", "lsp-diagnostics:///", false},
		{"specific file", "lsp-diagnostics:///test.go", false},
		{"invalid null byte", "lsp-diagnostics://\x00", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := HandleDiagnosticsResource(context.Background(), client, tc.uri)
			if tc.shouldErr && err == nil {
				t.Error("expected error for invalid URI")
			}
			// If !shouldErr, error is expected from LSP operations, not parsing
		})
	}
}

// --- HandleHoverResource unit tests ---

func TestHandleHoverResource_ParseError(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	testCases := []struct {
		name string
		uri  string
	}{
		{"missing query params", "lsp-hover:///test.go"},
		{"missing line", "lsp-hover:///test.go?column=1&language_id=go"},
		{"missing column", "lsp-hover:///test.go?line=1&language_id=go"},
		{"missing language_id", "lsp-hover:///test.go?line=1&column=1"},
		{"invalid line", "lsp-hover:///test.go?line=abc&column=1&language_id=go"},
		{"invalid column", "lsp-hover:///test.go?line=1&column=xyz&language_id=go"},
		{"invalid URI", "lsp-hover://\x00invalid?line=1&column=1&language_id=go"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := HandleHoverResource(context.Background(), client, tc.uri)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			// Error message should be informative
			if err.Error() == "" {
				t.Error("expected non-empty error message")
			}
		})
	}
}

func TestHandleHoverResource_FileNotFound(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	_, err := HandleHoverResource(
		context.Background(),
		client,
		"lsp-hover:///nonexistent/file/that/does/not/exist.go?line=1&column=1&language_id=go",
	)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	// Should be a file read error
	if !os.IsNotExist(err) {
		// Check that error mentions file reading
		if err.Error() == "" {
			t.Error("expected non-empty error message")
		}
	}
}

func TestHandleHoverResource_ValidFileReading(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	content := "package main\nfunc main() {}\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=2&column=6&language_id=go", testFile)

	// This will fail at OpenDocument because server isn't running,
	// but it confirms file reading works (error won't be IsNotExist)
	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file exists, so error should not be IsNotExist (should fail at LSP operation)")
	}
}

func TestHandleHoverResource_SpecialCharactersInPath(t *testing.T) {
	tmpDir := t.TempDir()
	// Create file with spaces in name
	testFile := filepath.Join(tmpDir, "test file with spaces.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=1&column=1&language_id=go", testFile)

	// Should successfully read file (error will be from LSP, not file I/O)
	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file with spaces should be readable, got IsNotExist error")
	}
}

func TestHandleHoverResource_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.go")
	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=1&column=1&language_id=go", testFile)

	// Empty file should be readable
	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("empty file should be readable")
	}
}

func TestHandleHoverResource_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.go")

	// Create a large file (1MB)
	largeContent := "package main\n"
	for i := 0; i < 50000; i++ {
		largeContent += fmt.Sprintf("// Comment line %d\n", i)
	}

	if err := os.WriteFile(testFile, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=1&column=1&language_id=go", testFile)

	// Should be able to read large file
	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("large file should be readable")
	}
}

// --- HandleCompletionsResource unit tests ---

func TestHandleCompletionsResource_ParseError(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	testCases := []struct {
		name string
		uri  string
	}{
		{"missing all params", "lsp-completions:///test.go"},
		{"missing line", "lsp-completions:///test.go?column=1&language_id=typescript"},
		{"missing column", "lsp-completions:///test.go?line=1&language_id=typescript"},
		{"missing language_id", "lsp-completions:///test.go?line=1&column=1"},
		{"invalid line", "lsp-completions:///test.go?line=bad&column=1&language_id=typescript"},
		{"invalid column", "lsp-completions:///test.go?line=1&column=bad&language_id=typescript"},
		{"invalid URI", "lsp-completions://\x00?line=1&column=1&language_id=go"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := HandleCompletionsResource(context.Background(), client, tc.uri)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestHandleCompletionsResource_FileNotFound(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/tsserver", nil)

	_, err := HandleCompletionsResource(
		context.Background(),
		client,
		"lsp-completions:///does/not/exist.ts?line=1&column=1&language_id=typescript",
	)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestHandleCompletionsResource_ValidFileReading(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.py")
	content := "import sys\nprint(sys.)\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/pylsp", nil)
	uri := fmt.Sprintf("lsp-completions://%s?line=2&column=11&language_id=python", testFile)

	// Will fail at OpenDocument, but file reading should succeed
	_, err := HandleCompletionsResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file should exist, error should not be IsNotExist")
	}
}

func TestHandleCompletionsResource_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.go")
	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-completions://%s?line=1&column=1&language_id=go", testFile)

	// Empty file should be readable
	_, err := HandleCompletionsResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("empty file should be readable")
	}
}

func TestHandleCompletionsResource_LargeLineColumn(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	// Use very large line/column numbers
	uri := fmt.Sprintf("lsp-completions://%s?line=9999&column=500&language_id=go", testFile)

	// Should not error on parsing (will error on LSP operations)
	_, err := HandleCompletionsResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file should exist")
	}
}

func TestHandleCompletionsResource_DifferentLanguages(t *testing.T) {
	tmpDir := t.TempDir()

	languages := []struct {
		ext      string
		langID   string
		content  string
	}{
		{".go", "go", "package main\n"},
		{".py", "python", "import sys\n"},
		{".ts", "typescript", "const x = 1;\n"},
		{".rs", "rust", "fn main() {}\n"},
		{".java", "java", "public class Test {}\n"},
	}

	client := lsp.NewLSPClient("/nonexistent/lsp", nil)

	for _, lang := range languages {
		t.Run(lang.langID, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, "test"+lang.ext)
			if err := os.WriteFile(testFile, []byte(lang.content), 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			uri := fmt.Sprintf("lsp-completions://%s?line=1&column=1&language_id=%s", testFile, lang.langID)
			_, err := HandleCompletionsResource(context.Background(), client, uri)
			if err != nil && os.IsNotExist(err) {
				t.Errorf("file should exist for %s", lang.langID)
			}
		})
	}
}

// --- HandleInspectResource additional tests ---

func TestHandleInspectResource_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	inspectDir := filepath.Join(tmpDir, ".agent-lsp")
	if err := os.MkdirAll(inspectDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Create a file with no read permissions
	inspectFile := filepath.Join(inspectDir, "last-inspection.json")
	if err := os.WriteFile(inspectFile, []byte("{}"), 0000); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer os.Chmod(inspectFile, 0644) // Clean up permissions

	// Test may be skipped on systems where root can still read
	if data, err := os.ReadFile(inspectFile); err == nil {
		t.Skipf("cannot test read error: file is readable despite permissions (got %d bytes)", len(data))
	}

	_, err := HandleInspectResource(context.Background(), tmpDir, "inspect://last")
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
	if os.IsNotExist(err) {
		t.Error("error should not be IsNotExist for permission denied")
	}
}

func TestHandleInspectResource_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	inspectDir := filepath.Join(tmpDir, ".agent-lsp")
	if err := os.MkdirAll(inspectDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Create empty inspection file
	inspectFile := filepath.Join(inspectDir, "last-inspection.json")
	if err := os.WriteFile(inspectFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	result, err := HandleInspectResource(context.Background(), tmpDir, "inspect://last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("expected empty text for empty file, got %q", result.Text)
	}
	if result.MIMEType != "application/json" {
		t.Errorf("expected application/json, got %s", result.MIMEType)
	}
}

func TestHandleInspectResource_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	inspectDir := filepath.Join(tmpDir, ".agent-lsp")
	if err := os.MkdirAll(inspectDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Create file with invalid JSON
	inspectFile := filepath.Join(inspectDir, "last-inspection.json")
	invalidJSON := `{"findings": [}` // malformed JSON
	if err := os.WriteFile(inspectFile, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	result, err := HandleInspectResource(context.Background(), tmpDir, "inspect://last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Handler should return content as-is without validating JSON
	if result.Text != invalidJSON {
		t.Errorf("expected raw content, got %q", result.Text)
	}
}

func TestHandleInspectResource_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	inspectDir := filepath.Join(tmpDir, ".agent-lsp")
	if err := os.MkdirAll(inspectDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Create a large inspection file (1MB of JSON)
	inspectFile := filepath.Join(inspectDir, "last-inspection.json")
	largeContent := `{"findings":[`
	for i := 0; i < 10000; i++ {
		if i > 0 {
			largeContent += ","
		}
		largeContent += fmt.Sprintf(`{"id":%d,"message":"test"}`, i)
	}
	largeContent += `]}`

	if err := os.WriteFile(inspectFile, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	result, err := HandleInspectResource(context.Background(), tmpDir, "inspect://last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Text) != len(largeContent) {
		t.Errorf("expected content length %d, got %d", len(largeContent), len(result.Text))
	}
}

func TestHandleInspectResource_URIPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	inspectDir := filepath.Join(tmpDir, ".agent-lsp")
	if err := os.MkdirAll(inspectDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	inspectFile := filepath.Join(inspectDir, "last-inspection.json")
	if err := os.WriteFile(inspectFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	result, err := HandleInspectResource(context.Background(), tmpDir, "inspect://last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.URI != "inspect://last" {
		t.Errorf("expected URI to be preserved, got %q", result.URI)
	}
}

// --- Edge cases for subscriptions ---

func TestHandleSubscribeDiagnostics_ValidURIs(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	notify := func(uri string) {}

	testCases := []struct {
		name string
		uri  string
	}{
		{"specific file", "lsp-diagnostics:///test.go"},
		{"all files empty", "lsp-diagnostics://"},
		{"all files root", "lsp-diagnostics:///"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := HandleSubscribeDiagnostics(context.Background(), client, tc.uri, notify)
			if err != nil {
				t.Errorf("unexpected error for %s: %v", tc.name, err)
			}
			if sub == nil {
				t.Error("expected non-nil subscription context")
			}
			if sub.Callback == nil {
				t.Error("expected non-nil callback")
			}
		})
	}
}

func TestHandleUnsubscribeDiagnostics_MultipleUnsubscribes(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	notify := func(uri string) {}

	sub, err := HandleSubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///test.go", notify)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// First unsubscribe should succeed
	err = HandleUnsubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///test.go", sub)
	if err != nil {
		t.Errorf("first unsubscribe failed: %v", err)
	}

	// Second unsubscribe of same context should not error
	err = HandleUnsubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///test.go", sub)
	if err != nil {
		t.Errorf("second unsubscribe failed: %v", err)
	}
}

func TestHandleSubscribeDiagnostics_CallbackInvocation(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	called := false
	var receivedURI string
	notify := func(uri string) {
		called = true
		receivedURI = uri
	}

	// Subscribe to specific file
	sub, err := HandleSubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///test.go", notify)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Manually invoke callback to test notification filtering
	sub.Callback("file:///test.go", nil)
	if !called {
		t.Error("notify should have been called for matching file")
	}
	if receivedURI != "file:///test.go" {
		t.Errorf("received URI = %q, want file:///test.go", receivedURI)
	}

	// Reset and test with non-matching file
	called = false
	receivedURI = ""
	sub.Callback("file:///other.go", nil)
	if called {
		t.Error("notify should not have been called for non-matching file")
	}
}

func TestHandleSubscribeDiagnostics_AllFilesCallback(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	callCount := 0
	notify := func(uri string) {
		callCount++
	}

	// Subscribe to all files
	sub, err := HandleSubscribeDiagnostics(context.Background(), client, "lsp-diagnostics://", notify)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// All file:// URIs should trigger notification
	sub.Callback("file:///test.go", nil)
	sub.Callback("file:///other.go", nil)
	sub.Callback("inmemory:///scratch.go", nil) // Should not count

	if callCount != 2 {
		t.Errorf("expected 2 notifications for file:// URIs, got %d", callCount)
	}
}
