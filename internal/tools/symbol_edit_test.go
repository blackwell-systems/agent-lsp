package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- resolveInDocumentSymbols tests ---

func makeSymbol(name string, startLine, startCol, endLine, endCol int, children ...types.DocumentSymbol) types.DocumentSymbol {
	return types.DocumentSymbol{
		Name: name,
		Range: types.Range{
			Start: types.Position{Line: startLine, Character: startCol},
			End:   types.Position{Line: endLine, Character: endCol},
		},
		SelectionRange: types.Range{
			Start: types.Position{Line: startLine, Character: startCol},
			End:   types.Position{Line: startLine, Character: startCol + len(name)},
		},
		Children: children,
	}
}

func TestResolveInDocumentSymbols_Simple(t *testing.T) {
	symbols := []types.DocumentSymbol{
		makeSymbol("Foo", 0, 0, 5, 1),
		makeSymbol("Bar", 7, 0, 12, 1),
	}

	result, err := resolveInDocumentSymbols(symbols, "Foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Foo" {
		t.Errorf("expected Foo, got %s", result.Name)
	}
	if result.Range.Start.Line != 0 || result.Range.End.Line != 5 {
		t.Errorf("unexpected range: start=%d end=%d", result.Range.Start.Line, result.Range.End.Line)
	}
}

func TestResolveInDocumentSymbols_Nested(t *testing.T) {
	child := makeSymbol("Child", 2, 4, 4, 5)
	parent := makeSymbol("Parent", 0, 0, 5, 1, child)
	symbols := []types.DocumentSymbol{parent}

	result, err := resolveInDocumentSymbols(symbols, "Parent.Child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Child" {
		t.Errorf("expected Child, got %s", result.Name)
	}
	if result.Range.Start.Line != 2 {
		t.Errorf("expected start line 2, got %d", result.Range.Start.Line)
	}
}

func TestResolveInDocumentSymbols_DeeplyNested(t *testing.T) {
	c := makeSymbol("C", 3, 8, 4, 9)
	b := makeSymbol("B", 2, 4, 5, 5, c)
	a := makeSymbol("A", 0, 0, 6, 1, b)
	symbols := []types.DocumentSymbol{a}

	result, err := resolveInDocumentSymbols(symbols, "A.B.C")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "C" {
		t.Errorf("expected C, got %s", result.Name)
	}
	if result.Range.Start.Line != 3 {
		t.Errorf("expected start line 3, got %d", result.Range.Start.Line)
	}
}

func TestResolveInDocumentSymbols_OverloadIndex(t *testing.T) {
	// Two symbols with the same name at the same level.
	method0 := makeSymbol("Method", 1, 0, 3, 1)
	method1 := makeSymbol("Method", 5, 0, 7, 1)
	symbols := []types.DocumentSymbol{method0, method1}

	result, err := resolveInDocumentSymbols(symbols, "Method[1]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Method" {
		t.Errorf("expected Method, got %s", result.Name)
	}
	if result.Range.Start.Line != 5 {
		t.Errorf("expected start line 5 (second overload), got %d", result.Range.Start.Line)
	}
}

func TestResolveInDocumentSymbols_NotFound(t *testing.T) {
	symbols := []types.DocumentSymbol{
		makeSymbol("Foo", 0, 0, 5, 1),
	}

	_, err := resolveInDocumentSymbols(symbols, "NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent symbol")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %s", err.Error())
	}
}

func TestResolveInDocumentSymbols_IndexOutOfBounds(t *testing.T) {
	symbols := []types.DocumentSymbol{
		makeSymbol("Method", 1, 0, 3, 1),
	}

	_, err := resolveInDocumentSymbols(symbols, "Method[5]")
	if err == nil {
		t.Fatal("expected error for out of bounds index")
	}
	if !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("expected 'out of bounds' in error, got: %s", err.Error())
	}
}

// --- ResolveSymbolByNamePath tests ---

func TestResolveSymbolByNamePath_NilClient(t *testing.T) {
	_, err := ResolveSymbolByNamePath(context.Background(), newNilClient(), "/tmp/test.go", "Foo")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected 'not initialized' in error, got: %s", err.Error())
	}
}

func TestResolveSymbolByNamePath_EmptyNamePath(t *testing.T) {
	// nil client is checked first, but test with non-nil would need a real client.
	// Test that the nil check fires before the empty path check.
	_, err := ResolveSymbolByNamePath(context.Background(), newNilClient(), "/tmp/test.go", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Handle* nil client tests ---

func TestHandleReplaceSymbolBody_NilClient(t *testing.T) {
	r, err := HandleReplaceSymbolBody(context.Background(), newNilClient(), map[string]any{
		"file_path":   "/tmp/test.go",
		"symbol_path": "Foo",
		"new_body":    "return nil",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
	if !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected 'not initialized', got: %s", r.Content[0].Text)
	}
}

func TestHandleInsertAfterSymbol_NilClient(t *testing.T) {
	r, err := HandleInsertAfterSymbol(context.Background(), newNilClient(), map[string]any{
		"file_path":   "/tmp/test.go",
		"symbol_path": "Foo",
		"code":        "// comment",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
	if !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected 'not initialized', got: %s", r.Content[0].Text)
	}
}

func TestHandleInsertBeforeSymbol_NilClient(t *testing.T) {
	r, err := HandleInsertBeforeSymbol(context.Background(), newNilClient(), map[string]any{
		"file_path":   "/tmp/test.go",
		"symbol_path": "Foo",
		"code":        "// comment",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
	if !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected 'not initialized', got: %s", r.Content[0].Text)
	}
}

func TestHandleSafeDeleteSymbol_NilClient(t *testing.T) {
	r, err := HandleSafeDeleteSymbol(context.Background(), newNilClient(), map[string]any{
		"file_path":   "/tmp/test.go",
		"symbol_path": "Foo",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for nil client")
	}
	if !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected 'not initialized', got: %s", r.Content[0].Text)
	}
}

// --- getDiagnosticsForFile tests ---

func TestGetDiagnosticsForFile_NilClient(t *testing.T) {
	errors, warnings := getDiagnosticsForFile(context.Background(), nil, "/tmp/test.go")
	if errors != 0 || warnings != 0 {
		t.Errorf("expected (0, 0) for nil client, got (%d, %d)", errors, warnings)
	}
}

// --- Missing args tests ---

func TestHandleReplaceSymbolBody_MissingArgs(t *testing.T) {
	r, err := HandleReplaceSymbolBody(context.Background(), newNilClient(), map[string]any{
		"file_path": "/tmp/test.go",
		"new_body":  "return nil",
		// symbol_path missing
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// nil client triggers before missing args check, so we get "not initialized"
	if !r.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestHandleSafeDeleteSymbol_MissingArgs(t *testing.T) {
	r, err := HandleSafeDeleteSymbol(context.Background(), newNilClient(), map[string]any{
		"symbol_path": "Foo",
		// file_path missing
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// nil client triggers before missing args check
	if !r.IsError {
		t.Fatal("expected IsError=true")
	}
}
