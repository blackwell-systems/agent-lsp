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

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Go test suffix
		{"pkg/foo/bar_test.go", true},
		{"main_test.go", true},
		// JS/TS .test. pattern
		{"src/utils.test.ts", true},
		{"src/utils.test.js", true},
		{"src/utils.spec.ts", true},
		{"src/utils.spec.js", true},
		// Python test_ prefix
		{"test_models.py", true},
		{"/home/user/project/test_utils.py", true},
		// Negative cases
		{"pkg/foo/bar.go", false},
		{"main.go", false},
		{"src/utils.ts", false},
		{"src/utils.js", false},
		{"models.py", false},
		{"attestation_test_helpers.go", false}, // does not end in _test.go
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := isTestFile(tc.path)
			if got != tc.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestLangIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"internal/tools/helpers.go", "go"},
		{"src/index.ts", "typescript"},
		{"src/App.tsx", "typescript"},
		{"src/index.js", "javascript"},
		{"src/App.jsx", "javascript"},
		{"models.py", "python"},
		{"src/lib.rs", "rust"},
		{"File.cs", "csharp"},
		{"main.hs", "haskell"},
		{"app.rb", "ruby"},
		{"config.xyz", "plaintext"},
		{"README.md", "plaintext"},
		{"Makefile", "plaintext"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := lsp.LanguageIDFromPath(tc.path)
			if got != tc.want {
				t.Errorf("lsp.LanguageIDFromPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestHandleGetChangeImpact_EmptyFiles(t *testing.T) {
	ctx := context.Background()

	// CheckInitialized runs before changed_files validation. With a nil client,
	// all calls return the "not initialized" error. These tests verify that the
	// handler returns an ErrorResult (never a nil error) under these conditions.
	tests := []struct {
		name        string
		args        map[string]any
		wantErrText string
	}{
		{
			name: "missing changed_files key with nil client",
			args: map[string]any{},
			// CheckInitialized fires first when client is nil.
			wantErrText: "LSP client not initialized",
		},
		{
			name: "empty changed_files slice with nil client",
			args: map[string]any{"changed_files": []any{}},
			// CheckInitialized fires first when client is nil.
			wantErrText: "LSP client not initialized",
		},
		{
			name: "changed_files with only empty strings with nil client",
			args: map[string]any{"changed_files": []any{"", ""}},
			// CheckInitialized fires first when client is nil.
			wantErrText: "LSP client not initialized",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := HandleGetChangeImpact(ctx, nil, tc.args)
			if err != nil {
				t.Fatalf("unexpected non-nil error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected IsError=true, got false; content=%v", result.Content)
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty content")
			}
			got := result.Content[0].Text
			if !strings.Contains(got, tc.wantErrText) {
				t.Errorf("error text %q does not contain %q", got, tc.wantErrText)
			}
		})
	}
}

func TestHandleGetChangeImpact_NilClient(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{
		"changed_files": []any{"internal/tools/helpers.go"},
	}

	result, err := HandleGetChangeImpact(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected non-nil error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true, got false")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	got := result.Content[0].Text
	want := "LSP client not initialized"
	if !strings.Contains(got, want) {
		t.Errorf("error text %q does not contain %q", got, want)
	}
}

func TestCollectAllSymbols(t *testing.T) {
	// Create temp file for source line resolution
	dir := t.TempDir()
	src := filepath.Join(dir, "test.go")
	content := "package test\n\nfunc ExportedFunc() {}\n\nfunc unexportedHelper() {}\n\ntype myField struct{}\n"
	os.WriteFile(src, []byte(content), 0644)

	syms := []types.DocumentSymbol{
		{Name: "ExportedFunc", Kind: 12, Range: types.Range{Start: types.Position{Line: 2}, End: types.Position{Line: 2}}, SelectionRange: types.Range{Start: types.Position{Line: 2, Character: 5}}},
		{Name: "unexportedHelper", Kind: 12, Range: types.Range{Start: types.Position{Line: 4}, End: types.Position{Line: 4}}, SelectionRange: types.Range{Start: types.Position{Line: 4, Character: 5}}},
		{Name: "myField", Kind: 8, Range: types.Range{Start: types.Position{Line: 6}, End: types.Position{Line: 6}}, SelectionRange: types.Range{Start: types.Position{Line: 6, Character: 5}}},
	}

	// collectAllSymbols should include both exported and unexported, but not fields
	var all []exportedSymbol
	collectAllSymbols(syms, src, "go", &all, false)
	if len(all) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(all))
	}

	// collectExportedSymbols should only include exported
	var exported []exportedSymbol
	collectExportedSymbols(syms, src, "go", &exported, false)
	if len(exported) != 1 {
		t.Fatalf("expected 1 exported symbol, got %d", len(exported))
	}
	if exported[0].Name != "ExportedFunc" {
		t.Errorf("expected ExportedFunc, got %s", exported[0].Name)
	}
}
