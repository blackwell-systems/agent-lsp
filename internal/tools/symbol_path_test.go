package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// --- TestHandleGoToSymbol_NilClient ---

// TestHandleGoToSymbol_NilClient verifies that a nil client returns an error
// result containing "not initialized".
func TestHandleGoToSymbol_NilClient(t *testing.T) {
	r, err := HandleGoToSymbol(context.Background(), newNilClient(), map[string]interface{}{
		"symbol_path": "pkg.Function",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client, got false")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected error text containing 'not initialized', got %v", r.Content)
	}
}

// --- TestHandleGoToSymbol_EmptySymbolPath ---

// TestHandleGoToSymbol_EmptySymbolPath verifies that an empty symbol_path returns
// an error result.
func TestHandleGoToSymbol_EmptySymbolPath(t *testing.T) {
	r, err := HandleGoToSymbol(context.Background(), newNilClient(), map[string]interface{}{
		"symbol_path": "",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// nil client triggers CheckInitialized before symbol_path check;
	// either way we expect an error result
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty symbol_path, got false")
	}
}

// --- TestBestSymbolMatch_NoDots ---

// TestBestSymbolMatch_NoDots verifies that when the symbol path has no dots,
// the first candidate is returned regardless of ContainerName.
func TestBestSymbolMatch_NoDots(t *testing.T) {
	containerA := "PkgA"
	containerB := "PkgB"
	candidates := []types.SymbolInformation{
		{Name: "Alpha", ContainerName: &containerA},
		{Name: "Alpha", ContainerName: &containerB},
	}

	result := bestSymbolMatch(candidates, "Alpha")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerName == nil || *result.ContainerName != containerA {
		t.Errorf("expected first candidate (ContainerName=%q), got %v", containerA, result.ContainerName)
	}
}

// --- TestBestSymbolMatch_WithDots ---

// TestBestSymbolMatch_WithDots verifies that when the path is dotted, the candidate
// whose ContainerName equals the parent component is preferred.
func TestBestSymbolMatch_WithDots(t *testing.T) {
	containerWrong := "OtherPkg"
	containerRight := "MyClass"
	candidates := []types.SymbolInformation{
		{Name: "method", ContainerName: &containerWrong},
		{Name: "method", ContainerName: &containerRight},
	}

	result := bestSymbolMatch(candidates, "MyClass.method")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerName == nil || *result.ContainerName != containerRight {
		t.Errorf("expected candidate with ContainerName=%q, got %v", containerRight, result.ContainerName)
	}
}

// --- TestBestSymbolMatch_Empty ---

// TestBestSymbolMatch_Empty verifies that an empty candidate list returns nil.
func TestBestSymbolMatch_Empty(t *testing.T) {
	if got := bestSymbolMatch(nil, "pkg.Fn"); got != nil {
		t.Errorf("expected nil for empty candidates, got %+v", got)
	}
	if got := bestSymbolMatch([]types.SymbolInformation{}, "Fn"); got != nil {
		t.Errorf("expected nil for empty slice, got %+v", got)
	}
}

// --- TestBestSymbolMatch_NilContainerName ---

// TestBestSymbolMatch_NilContainerName verifies that candidates with a nil
// ContainerName do not panic during dotted-path matching.
func TestBestSymbolMatch_NilContainerName(t *testing.T) {
	right := "MyStruct"
	candidates := []types.SymbolInformation{
		{Name: "Method", ContainerName: nil},       // nil — must not dereference
		{Name: "Method", ContainerName: &right},
	}
	result := bestSymbolMatch(candidates, "MyStruct.Method")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerName == nil || *result.ContainerName != right {
		t.Errorf("expected ContainerName=%q, got %v", right, result.ContainerName)
	}
}

// --- TestBestSymbolMatch_PrefersNonTestOverTest_NoDot ---

// TestBestSymbolMatch_PrefersNonTestOverTest_NoDot verifies that for a non-dotted
// path, a non-test file candidate is preferred over a test file candidate with the
// same name.
func TestBestSymbolMatch_PrefersNonTestOverTest_NoDot(t *testing.T) {
	candidates := []types.SymbolInformation{
		{Name: "HandleFoo", Location: types.Location{URI: "file:///project/tools/foo_test.go"}},
		{Name: "HandleFoo", Location: types.Location{URI: "file:///project/tools/foo.go"}},
	}
	result := bestSymbolMatch(candidates, "HandleFoo")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if strings.HasSuffix(result.Location.URI, "_test.go") {
		t.Errorf("expected non-test file, got %q", result.Location.URI)
	}
}

// --- TestBestSymbolMatch_PrefersNonTestOverTest_Dotted ---

// TestBestSymbolMatch_PrefersNonTestOverTest_Dotted verifies that for a dotted path,
// when both candidates have the correct ContainerName, the non-test file wins.
// This is the core scenario from the reported bug: gopls returns the test wrapper
// first, then the real implementation.
func TestBestSymbolMatch_PrefersNonTestOverTest_Dotted(t *testing.T) {
	container := "MyType"
	candidates := []types.SymbolInformation{
		{
			Name:          "DoWork",
			ContainerName: &container,
			Location:      types.Location{URI: "file:///project/tools/mytype_test.go"},
		},
		{
			Name:          "DoWork",
			ContainerName: &container,
			Location:      types.Location{URI: "file:///project/tools/mytype.go"},
		},
	}
	result := bestSymbolMatch(candidates, "MyType.DoWork")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if strings.HasSuffix(result.Location.URI, "_test.go") {
		t.Errorf("expected non-test implementation file, got %q", result.Location.URI)
	}
}

// --- TestBestSymbolMatch_SingleTestOnly_Dotted ---

// TestBestSymbolMatch_SingleTestOnly_Dotted documents the known limitation: when
// gopls returns only one candidate and it is a test file, bestSymbolMatch has no
// alternative and returns it. This is the case where gopls indexing is incomplete.
func TestBestSymbolMatch_SingleTestOnly_Dotted(t *testing.T) {
	container := "MyType"
	candidates := []types.SymbolInformation{
		{
			Name:          "DoWork",
			ContainerName: &container,
			Location:      types.Location{URI: "file:///project/tools/mytype_test.go"},
		},
	}
	result := bestSymbolMatch(candidates, "MyType.DoWork")
	if result == nil {
		t.Fatal("expected non-nil result even when only candidate is a test file")
	}
	// Document: returns the test file candidate — this is the known limitation when
	// gopls has not yet indexed the implementation file.
	if !strings.HasSuffix(result.Location.URI, "_test.go") {
		t.Errorf("expected test file candidate to be returned, got %q", result.Location.URI)
	}
}

// --- TestBestSymbolMatch_ContainerFallback ---

// TestBestSymbolMatch_ContainerFallback verifies that when no candidate has the
// exact leaf name, the container-only match still wins over candidates[0].
func TestBestSymbolMatch_ContainerFallback(t *testing.T) {
	right := "MyStruct"
	wrong := "OtherStruct"
	candidates := []types.SymbolInformation{
		// candidates[0] has a non-matching container
		{Name: "WrongName", ContainerName: &wrong, Location: types.Location{URI: "file:///other.go"}},
		// candidates[1] has the correct container but a different leaf name
		{Name: "WrongName", ContainerName: &right, Location: types.Location{URI: "file:///myfile.go"}},
	}
	// path leaf is "Target" — no exact-name match exists, so container-match wins
	result := bestSymbolMatch(candidates, "MyStruct.Target")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerName == nil || *result.ContainerName != right {
		t.Errorf("expected container-match candidate, got ContainerName=%v URI=%q",
			result.ContainerName, result.Location.URI)
	}
}

// --- TestBestSymbolMatch_NoMatchFallback ---

// TestBestSymbolMatch_NoMatchFallback verifies that when no candidate matches on
// name or container, candidates[0] is returned as a last resort.
func TestBestSymbolMatch_NoMatchFallback(t *testing.T) {
	unrelated := "Unrelated"
	candidates := []types.SymbolInformation{
		{Name: "SomethingElse", ContainerName: &unrelated, Location: types.Location{URI: "file:///other.go"}},
	}
	result := bestSymbolMatch(candidates, "MyStruct.DoWork")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "SomethingElse" {
		t.Errorf("expected candidates[0] fallback, got %q", result.Name)
	}
}

// --- TestBestSymbolMatch_DeeplyNestedPath ---

// TestBestSymbolMatch_DeeplyNestedPath verifies that "a.b.c" splits correctly:
// parent="a.b", leaf="c". ContainerName matching uses the full parent segment.
func TestBestSymbolMatch_DeeplyNestedPath(t *testing.T) {
	parentShallow := "b"   // would match "b.c" but not "a.b.c"
	parentDeep := "a.b"    // correct full parent for "a.b.c"
	candidates := []types.SymbolInformation{
		{Name: "c", ContainerName: &parentShallow, Location: types.Location{URI: "file:///shallow.go"}},
		{Name: "c", ContainerName: &parentDeep, Location: types.Location{URI: "file:///deep.go"}},
	}
	result := bestSymbolMatch(candidates, "a.b.c")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerName == nil || *result.ContainerName != parentDeep {
		t.Errorf("expected ContainerName=%q (deep parent), got %v", parentDeep, result.ContainerName)
	}
}
