package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- bestSymbolMatch tests ---

func strPtr(s string) *string { return &s }

func TestBestSymbolMatch_EmptyCandidates(t *testing.T) {
	got := bestSymbolMatch(nil, "Foo")
	if got != nil {
		t.Errorf("expected nil for empty candidates, got %v", got)
	}
}

func TestBestSymbolMatch_ExactNameNonTest(t *testing.T) {
	candidates := []types.SymbolInformation{
		{Name: "Foo", Location: types.Location{URI: "file:///src/foo_test.go"}},
		{Name: "Foo", Location: types.Location{URI: "file:///src/foo.go"}},
	}
	got := bestSymbolMatch(candidates, "Foo")
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Location.URI != "file:///src/foo.go" {
		t.Errorf("expected non-test file, got %s", got.Location.URI)
	}
}

func TestBestSymbolMatch_DottedPath_NameAndContainer(t *testing.T) {
	container := "MyClass"
	candidates := []types.SymbolInformation{
		{Name: "Method", ContainerName: nil, Location: types.Location{URI: "file:///other.go"}},
		{Name: "Method", ContainerName: &container, Location: types.Location{URI: "file:///src/myclass.go"}},
	}
	got := bestSymbolMatch(candidates, "MyClass.Method")
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Location.URI != "file:///src/myclass.go" {
		t.Errorf("expected container match, got %s", got.Location.URI)
	}
}

func TestBestSymbolMatch_DottedPath_ContainerOnlyNonTest(t *testing.T) {
	container := "Srv"
	candidates := []types.SymbolInformation{
		// Name doesn't match leaf, but container matches.
		{Name: "Other", ContainerName: &container, Location: types.Location{URI: "file:///srv_test.go"}},
		{Name: "Other", ContainerName: &container, Location: types.Location{URI: "file:///srv.go"}},
	}
	got := bestSymbolMatch(candidates, "Srv.Handle")
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	// Should prefer non-test file via container-only match (priority 3).
	if got.Location.URI != "file:///srv.go" {
		t.Errorf("expected non-test container match, got %s", got.Location.URI)
	}
}

func TestBestSymbolMatch_FallbackToFirstCandidate(t *testing.T) {
	candidates := []types.SymbolInformation{
		{Name: "Other", Location: types.Location{URI: "file:///a.go"}},
		{Name: "Another", Location: types.Location{URI: "file:///b.go"}},
	}
	got := bestSymbolMatch(candidates, "NonExistent")
	if got == nil {
		t.Fatal("expected non-nil fallback")
	}
	if got.Name != "Other" {
		t.Errorf("expected first candidate as fallback, got %s", got.Name)
	}
}

// --- extractSymbolName tests ---

func TestExtractSymbolName_Simple(t *testing.T) {
	tests := []struct {
		name  string
		hover string
		want  string
	}{
		{"plain identifier", "MyFunc", "MyFunc"},
		{"with signature", "func MyFunc(x int) error", "func"},
		{"code block", "```go\nMyStruct\n```", "MyStruct"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"starts with special char", "  (receiver) Method", "receiver"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSymbolName(tt.hover)
			if got != tt.want {
				t.Errorf("extractSymbolName(%q) = %q, want %q", tt.hover, got, tt.want)
			}
		})
	}
}

// --- appendHint edge case: multiple content items already present ---

func TestAppendHint_MultipleContentItems(t *testing.T) {
	r := types.ToolResult{Content: []types.ContentItem{
		{Type: "text", Text: "first"},
		{Type: "text", Text: "second"},
	}}
	got := appendHint(r, "do next thing")
	if len(got.Content) != 3 {
		t.Fatalf("expected 3 content items, got %d", len(got.Content))
	}
	if got.Content[2].Text != "Next step: do next thing" {
		t.Errorf("unexpected hint text: %s", got.Content[2].Text)
	}
}

// --- AppendTokenMeta edge cases ---

func TestAppendTokenMeta_EmptyContent(t *testing.T) {
	r := types.ToolResult{Content: []types.ContentItem{}}
	got := AppendTokenMeta(r, "/tmp/x.go")
	if len(got.Content) != 0 {
		t.Errorf("expected unchanged empty content, got %d items", len(got.Content))
	}
}

func TestAppendTokenMeta_EmptyText(t *testing.T) {
	r := types.ToolResult{Content: []types.ContentItem{{Type: "text", Text: ""}}}
	got := AppendTokenMeta(r, "/tmp/x.go")
	if len(got.Content) != 1 {
		t.Errorf("expected unchanged content for empty text, got %d items", len(got.Content))
	}
}

// --- resolveInDocumentSymbols: empty path segment ---

func TestResolveInDocumentSymbols_EmptyNamePath(t *testing.T) {
	symbols := []types.DocumentSymbol{makeSymbol("Foo", 0, 0, 5, 1)}
	_, err := resolveInDocumentSymbols(symbols, "")
	if err == nil {
		t.Fatal("expected error for empty name path")
	}
}

// --- HandleRestartLspServer nil client ---

func TestHandleRestartLspServer_NilClient(t *testing.T) {
	r, err := HandleRestartLspServer(nil, newNilClient(), map[string]any{"root_dir": "/tmp"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
}

func TestHandleRestartLspServer_NilClient_NoArgs(t *testing.T) {
	r, err := HandleRestartLspServer(nil, newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
}

// --- FindTestFiles: Python tests/ sibling directory ---

func TestFindTestFiles_Python_WithTestsSubdir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	testsDir := filepath.Join(dir, "tests")
	os.MkdirAll(srcDir, 0755)
	os.MkdirAll(testsDir, 0755)
	writeFile(t, filepath.Join(srcDir, "module.py"), "pass")
	writeFile(t, filepath.Join(testsDir, "test_module.py"), "pass")

	result, err := FindTestFiles(nil, dir, "python", filepath.Join(srcDir, "module.py"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TestFiles) < 1 {
		t.Errorf("expected at least 1 test file from tests/ dir, got %d", len(result.TestFiles))
	}
}

// --- FindTestFiles: TypeScript ---

func TestFindTestFiles_TypeScript_SpecAndTest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.ts"), "export {}")
	writeFile(t, filepath.Join(dir, "index.test.ts"), "test()")
	writeFile(t, filepath.Join(dir, "index.spec.ts"), "describe()")

	result, err := FindTestFiles(nil, dir, "typescript", filepath.Join(dir, "index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TestFiles) != 2 {
		t.Errorf("expected 2 test files, got %d: %v", len(result.TestFiles), result.TestFiles)
	}
}

// --- FindTestFiles: JavaScript ---

func TestFindTestFiles_JavaScript(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.js"), "module.exports = {}")
	writeFile(t, filepath.Join(dir, "app.test.js"), "test()")
	writeFile(t, filepath.Join(dir, "app.spec.js"), "describe()")

	result, err := FindTestFiles(nil, dir, "javascript", filepath.Join(dir, "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TestFiles) != 2 {
		t.Errorf("expected 2 test files, got %d: %v", len(result.TestFiles), result.TestFiles)
	}
}

// --- FindTestFiles: Rust returns source itself ---

func TestFindTestFiles_Rust_InlineTests(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "lib.rs")
	writeFile(t, src, "fn main() {}")

	result, err := FindTestFiles(nil, dir, "rust", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TestFiles) != 1 || result.TestFiles[0] != src {
		t.Errorf("expected source file as test file, got %v", result.TestFiles)
	}
}

// --- FindTestFiles: unknown language ---

func TestFindTestFiles_UnknownLanguage(t *testing.T) {
	dir := t.TempDir()
	result, err := FindTestFiles(nil, dir, "brainfuck", filepath.Join(dir, "hello.bf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TestFiles) != 0 {
		t.Errorf("expected 0 test files for unknown lang, got %d", len(result.TestFiles))
	}
}

// --- resolveBuildPath: additional cases ---

func TestResolveBuildPath_TypeScript(t *testing.T) {
	got := resolveBuildPath("typescript", "")
	if got != "" {
		t.Errorf("expected empty for typescript default, got %q", got)
	}
}

func TestResolveBuildPath_CSharp(t *testing.T) {
	got := resolveBuildPath("csharp", "")
	if got != "" {
		t.Errorf("expected empty for csharp default, got %q", got)
	}
}

// --- resolveTestPath: Go path normalization edge cases ---

func TestResolveTestPath_GoAbsolutePath(t *testing.T) {
	got := resolveTestPath("go", "/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expected path preserved for absolute, got %q", got)
	}
}

func TestResolveTestPath_GoAlreadyDotPrefix(t *testing.T) {
	got := resolveTestPath("go", "./already/prefixed")
	if got != "./already/prefixed" {
		t.Errorf("expected path unchanged, got %q", got)
	}
}

// --- ValidateFilePath: traversal attack ---

func TestValidateFilePath_TraversalAttack(t *testing.T) {
	_, err := ValidateFilePath("/home/user/project/../../etc/passwd", "/home/user/project")
	if err == nil {
		t.Error("expected error for path traversal attack")
	}
}

// --- HandleGetTestsForFile: valid path with Go tests ---

func TestHandleGetTestsForFile_GoProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	writeFile(t, filepath.Join(dir, "main_test.go"), "package main")

	r, err := HandleGetTestsForFile(nil, map[string]any{
		"file_path": filepath.Join(dir, "main.go"),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("expected success, got error: %s", r.Content[0].Text)
	}
}

// --- HandleCallHierarchy: nil client ---

func TestHandleCallHierarchy_NilClient(t *testing.T) {
	r, err := HandleCallHierarchy(nil, newNilClient(), map[string]any{
		"file_path": "/tmp/test.go",
		"line":      float64(1),
		"column":    float64(1),
		"direction": "incoming",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
}

// --- HandleExportCache: nil client ---

func TestHandleExportCache_NilClient(t *testing.T) {
	r, err := HandleExportCache(nil, newNilClient(), map[string]any{
		"dest_path": "/tmp/cache.db.gz",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
}

// --- HandleImportCache: nil client ---

func TestHandleImportCache_NilClient(t *testing.T) {
	r, err := HandleImportCache(nil, newNilClient(), map[string]any{
		"src_path": "/tmp/cache.db.gz",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
}
