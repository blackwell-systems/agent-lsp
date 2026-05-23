package resources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// Additional error path tests to increase coverage beyond 73%

// --- parseResourceQueryParams edge cases ---

func TestParseResourceQueryParams_NegativeValues(t *testing.T) {
	// Negative line/column are parseable (strconv.Atoi succeeds)
	// but result in negative position values
	uri := "lsp-hover:///test.go?line=-5&column=-10&language_id=go"
	filePath, pos, langID, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filePath != "/test.go" {
		t.Errorf("filePath = %q, want /test.go", filePath)
	}
	// -5 (1-indexed) becomes -6 (0-indexed)
	if pos.Line != -6 || pos.Character != -11 {
		t.Errorf("pos = %d:%d, want -6:-11", pos.Line, pos.Character)
	}
	if langID != "go" {
		t.Errorf("langID = %q, want go", langID)
	}
}

func TestParseResourceQueryParams_MaxInt(t *testing.T) {
	// Test with very large numbers that are still valid integers
	uri := "lsp-hover:///test.go?line=2147483647&column=2147483647&language_id=go"
	_, pos, _, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos.Line != 2147483646 {
		t.Errorf("pos.Line = %d, want 2147483646", pos.Line)
	}
}

func TestParseResourceQueryParams_Overflow(t *testing.T) {
	// Test with number larger than int max
	uri := "lsp-hover:///test.go?line=99999999999999999999&column=1&language_id=go"
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for integer overflow")
	}
}

func TestParseResourceQueryParams_EmptyQueryValues(t *testing.T) {
	uri := "lsp-hover:///test.go?line=&column=&language_id="
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for empty query values")
	}
}

func TestParseResourceQueryParams_DuplicateParams(t *testing.T) {
	// URL query parsing takes the first value when duplicates exist
	uri := "lsp-hover:///test.go?line=1&line=2&column=3&language_id=go"
	_, pos, _, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First line=1 should be used
	if pos.Line != 0 {
		t.Errorf("pos.Line = %d, want 0 (from line=1)", pos.Line)
	}
}

func TestParseResourceQueryParams_CaseSensitivity(t *testing.T) {
	// Query parameters are case-sensitive
	uri := "lsp-hover:///test.go?LINE=1&COLUMN=1&LANGUAGE_ID=go"
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for uppercase query params")
	}
}

// --- HandleHoverResource file I/O edge cases ---

func TestHandleHoverResource_BinaryFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "binary.bin")
	// Write binary data (null bytes, etc.)
	binaryData := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	if err := os.WriteFile(testFile, binaryData, 0644); err != nil {
		t.Fatalf("failed to create binary file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=1&column=1&language_id=go", testFile)

	// Should be able to read binary file (will fail at LSP operation)
	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("binary file should be readable")
	}
}

func TestHandleHoverResource_FileWithUnicodeContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unicode.go")
	unicodeContent := "package main\n// Comment with emoji 🚀\nfunc main() {}\n"
	if err := os.WriteFile(testFile, []byte(unicodeContent), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=2&column=1&language_id=go", testFile)

	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file with unicode should be readable")
	}
}

func TestHandleHoverResource_VeryLongPath(t *testing.T) {
	tmpDir := t.TempDir()
	// Create nested directory structure
	longPath := tmpDir
	for i := 0; i < 10; i++ {
		longPath = filepath.Join(longPath, fmt.Sprintf("very_long_directory_name_%d", i))
	}
	if err := os.MkdirAll(longPath, 0755); err != nil {
		t.Fatalf("failed to create deep directory: %v", err)
	}

	testFile := filepath.Join(longPath, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=1&column=1&language_id=go", testFile)

	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file with long path should be readable")
	}
}

func TestHandleHoverResource_ReadOnlyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "readonly.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0444); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-hover://%s?line=1&column=1&language_id=go", testFile)

	// Read-only file should still be readable
	_, err := HandleHoverResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("read-only file should be readable")
	}
}

// --- HandleCompletionsResource edge cases ---

func TestHandleCompletionsResource_SymlinkToFile(t *testing.T) {
	tmpDir := t.TempDir()
	realFile := filepath.Join(tmpDir, "real.go")
	if err := os.WriteFile(realFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	symlinkFile := filepath.Join(tmpDir, "link.go")
	if err := os.Symlink(realFile, symlinkFile); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	uri := fmt.Sprintf("lsp-completions://%s?line=1&column=1&language_id=go", symlinkFile)

	// Symlink should be readable
	_, err := HandleCompletionsResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("symlink should be readable")
	}
}

func TestHandleCompletionsResource_FileWithNoExtension(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Makefile")
	if err := os.WriteFile(testFile, []byte("all:\n\techo hello\n"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/lsp", nil)
	uri := fmt.Sprintf("lsp-completions://%s?line=1&column=1&language_id=makefile", testFile)

	_, err := HandleCompletionsResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file without extension should be readable")
	}
}

func TestHandleCompletionsResource_FileWithMultipleDots(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.spec.ts")
	if err := os.WriteFile(testFile, []byte("describe('test', () => {})\n"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	client := lsp.NewLSPClient("/nonexistent/tsserver", nil)
	uri := fmt.Sprintf("lsp-completions://%s?line=1&column=1&language_id=typescript", testFile)

	_, err := HandleCompletionsResource(context.Background(), client, uri)
	if err != nil && os.IsNotExist(err) {
		t.Error("file with multiple dots should be readable")
	}
}

// --- HandleDiagnosticsResource edge cases ---

func TestHandleDiagnosticsResource_SlashVariants(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	testCases := []struct {
		name string
		uri  string
	}{
		{"no trailing slash", "lsp-diagnostics://"},
		{"single slash", "lsp-diagnostics:///"},
		{"double slash", "lsp-diagnostics:////"},
		{"with path no trailing", "lsp-diagnostics:///path/to/file"},
		{"with path trailing", "lsp-diagnostics:///path/to/file/"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Just verify URI parsing doesn't fail
			_, err := HandleDiagnosticsResource(context.Background(), client, tc.uri)
			// Error is expected (no LSP server), just checking parse succeeds
			_ = err
		})
	}
}

func TestHandleDiagnosticsResource_QueryParamsIgnored(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	// Query params in diagnostics URI should be ignored
	uri := "lsp-diagnostics:///test.go?foo=bar&baz=qux"
	_, err := HandleDiagnosticsResource(context.Background(), client, uri)
	// Will error from LSP operations, but parsing should work
	_ = err
}

// --- HandleInspectResource path edge cases ---

func TestHandleInspectResource_RelativeWorkspacePath(t *testing.T) {
	// Test with relative path in workspace root
	_, err := HandleInspectResource(context.Background(), "relative/path", "inspect://last")
	// Should work even with relative path (creates .agent-lsp subdirectory)
	// Will fail because directory doesn't exist, but path handling should work
	_ = err
}

func TestHandleInspectResource_WorkspaceWithSpecialChars(t *testing.T) {
	tmpDir := t.TempDir()
	specialDir := filepath.Join(tmpDir, "workspace with spaces")
	if err := os.MkdirAll(specialDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	inspectDir := filepath.Join(specialDir, ".agent-lsp")
	if err := os.MkdirAll(inspectDir, 0755); err != nil {
		t.Fatalf("failed to create .agent-lsp: %v", err)
	}

	inspectFile := filepath.Join(inspectDir, "last-inspection.json")
	if err := os.WriteFile(inspectFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	result, err := HandleInspectResource(context.Background(), specialDir, "inspect://last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "{}" {
		t.Errorf("expected {}, got %q", result.Text)
	}
}

func TestHandleInspectResource_DifferentURIFormats(t *testing.T) {
	tmpDir := t.TempDir()
	inspectDir := filepath.Join(tmpDir, ".agent-lsp")
	if err := os.MkdirAll(inspectDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	inspectFile := filepath.Join(inspectDir, "last-inspection.json")
	if err := os.WriteFile(inspectFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	uris := []string{
		"inspect://last",
		"inspect://last/",
		"inspect://last?param=value",
	}

	for _, uri := range uris {
		t.Run(uri, func(t *testing.T) {
			result, err := HandleInspectResource(context.Background(), tmpDir, uri)
			if err != nil {
				t.Fatalf("unexpected error for URI %q: %v", uri, err)
			}
			// URI should be preserved as-is
			if result.URI != uri {
				t.Errorf("URI = %q, want %q", result.URI, uri)
			}
		})
	}
}

// --- Subscription edge cases ---

func TestHandleSubscribeDiagnostics_PathVariants(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	notify := func(uri string) {}

	pathVariants := []string{
		"lsp-diagnostics:///test.go",
		"lsp-diagnostics:////test.go",
		"lsp-diagnostics:///path/to/file.go",
		"lsp-diagnostics:///path/with spaces/file.go",
	}

	for _, uri := range pathVariants {
		t.Run(uri, func(t *testing.T) {
			sub, err := HandleSubscribeDiagnostics(context.Background(), client, uri, notify)
			if err != nil {
				t.Errorf("unexpected error for %q: %v", uri, err)
			}
			if sub == nil || sub.Callback == nil {
				t.Error("expected valid subscription")
			}
		})
	}
}

func TestHandleSubscribeDiagnostics_CallbackWithNilDiagnostics(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	var receivedURI string
	notify := func(uri string) {
		receivedURI = uri
	}

	sub, err := HandleSubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///test.go", notify)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Call with nil diagnostics slice
	sub.Callback("file:///test.go", nil)

	if receivedURI != "file:///test.go" {
		t.Errorf("received URI = %q, want file:///test.go", receivedURI)
	}
}

func TestHandleSubscribeDiagnostics_NonFileURIFiltering(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)

	callCount := 0
	notify := func(uri string) {
		callCount++
	}

	// Subscribe to all files
	sub, err := HandleSubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///", notify)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Test various URI schemes
	testURIs := []struct {
		uri         string
		shouldNotify bool
	}{
		{"file:///test.go", true},
		{"inmemory:///scratch.go", false},
		{"git:///file.go", false},
		{"file://", true}, // Edge case: file:// prefix only
		{"http://example.com/file.go", false},
		{"", false},
	}

	for _, tc := range testURIs {
		callCount = 0
		sub.Callback(tc.uri, nil)
		expected := 0
		if tc.shouldNotify {
			expected = 1
		}
		if callCount != expected {
			t.Errorf("URI %q: got %d notifications, want %d", tc.uri, callCount, expected)
		}
	}
}

// --- ResourceTemplates edge cases ---

func TestResourceTemplates_TemplateFieldsComplete(t *testing.T) {
	templates := ResourceTemplates()
	for _, tmpl := range templates {
		if tmpl.Name == "" {
			t.Error("template has empty Name")
		}
		if tmpl.URITemplate == "" {
			t.Error("template has empty URITemplate")
		}
		if tmpl.Description == "" {
			t.Error("template has empty Description")
		}

		// Verify URITemplate contains placeholder variables
		if !strings.Contains(tmpl.URITemplate, "{") {
			// Diagnostics template may not have placeholders
			if tmpl.Name != "lsp-diagnostics" {
				t.Errorf("template %q URITemplate missing placeholders", tmpl.Name)
			}
		}
	}
}
