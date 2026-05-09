package lsp_test

import (
	"encoding/json"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// ---- NormalizeDocumentSymbols ----

func TestNormalizeDocumentSymbols_Null(t *testing.T) {
	result, err := lsp.NormalizeDocumentSymbols(nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(result))
	}
}

func TestNormalizeDocumentSymbols_EmptyArray(t *testing.T) {
	result, err := lsp.NormalizeDocumentSymbols(json.RawMessage(`[]`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(result))
	}
}

func TestNormalizeDocumentSymbols_DocumentSymbolVariant(t *testing.T) {
	raw := json.RawMessage(`[{"name":"Foo","kind":5,"range":{"start":{"line":0,"character":0},"end":{"line":10,"character":1}},"selectionRange":{"start":{"line":0,"character":5},"end":{"line":0,"character":8}}}]`)
	result, err := lsp.NormalizeDocumentSymbols(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(result))
	}
	sym := result[0]
	if sym.Name != "Foo" {
		t.Errorf("expected Name == Foo, got %q", sym.Name)
	}
	if sym.SelectionRange.Start.Line != 0 || sym.SelectionRange.Start.Character != 5 {
		t.Errorf("expected SelectionRange.Start == {0,5}, got %+v", sym.SelectionRange.Start)
	}
	if len(sym.Children) != 0 {
		t.Errorf("expected no children, got %d", len(sym.Children))
	}
}

// TestNormalizeDocumentSymbols_SymbolInformationVariant verifies that the
// SymbolInformation[] variant correctly builds a parent-child tree: MyField
// (containerName "MyStruct") appears as a child of the MyStruct root symbol.
func TestNormalizeDocumentSymbols_SymbolInformationVariant(t *testing.T) {
	raw := json.RawMessage(`[
  {"name":"MyStruct","kind":5,"location":{"uri":"file:///foo.go","range":{"start":{"line":0,"character":0},"end":{"line":5,"character":1}}}},
  {"name":"MyField","kind":8,"location":{"uri":"file:///foo.go","range":{"start":{"line":1,"character":1},"end":{"line":1,"character":10}}},"containerName":"MyStruct"}
]`)
	result, err := lsp.NormalizeDocumentSymbols(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// MyField has containerName "MyStruct" so it should not appear as a root.
	// MyStruct has no container so it is the only root.
	if len(result) != 1 {
		t.Fatalf("expected 1 root symbol, got %d", len(result))
	}
	root := result[0]
	if root.Name != "MyStruct" {
		t.Errorf("expected root Name == MyStruct, got %q", root.Name)
	}
	if root.SelectionRange != root.Range {
		t.Errorf("expected SelectionRange == Range (synthesized), got selectionRange=%+v range=%+v", root.SelectionRange, root.Range)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child in root, got %d", len(root.Children))
	}
	if root.Children[0].Name != "MyField" {
		t.Errorf("expected child Name == MyField, got %q", root.Children[0].Name)
	}
}

// ---- NormalizeCompletion ----

func TestNormalizeCompletion_Null(t *testing.T) {
	result, err := lsp.NormalizeCompletion(nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.Items == nil {
		t.Error("expected non-nil Items slice")
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty Items, got %d", len(result.Items))
	}
}

func TestNormalizeCompletion_BareArray(t *testing.T) {
	raw := json.RawMessage(`[{"label":"fmt"},{"label":"os"}]`)
	result, err := lsp.NormalizeCompletion(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsIncomplete {
		t.Error("expected IsIncomplete == false")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Label != "fmt" {
		t.Errorf("expected Items[0].Label == fmt, got %q", result.Items[0].Label)
	}
}

func TestNormalizeCompletion_CompletionList(t *testing.T) {
	raw := json.RawMessage(`{"isIncomplete":true,"items":[{"label":"Printf"},{"label":"Println"}]}`)
	result, err := lsp.NormalizeCompletion(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsIncomplete {
		t.Error("expected IsIncomplete == true")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Label != "Printf" {
		t.Errorf("expected Items[0].Label == Printf, got %q", result.Items[0].Label)
	}
}

// ---- NormalizeCodeActions ----

func TestNormalizeCodeActions_Null(t *testing.T) {
	result, err := lsp.NormalizeCodeActions(nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Error("expected non-nil slice")
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d", len(result))
	}
}

func TestNormalizeCodeActions_BareCommands(t *testing.T) {
	raw := json.RawMessage(`[{"title":"Format Document","command":"editor.action.formatDocument"}]`)
	result, err := lsp.NormalizeCodeActions(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 action, got %d", len(result))
	}
	a := result[0]
	if a.Title != "Format Document" {
		t.Errorf("expected Title == 'Format Document', got %q", a.Title)
	}
	if a.Command == nil {
		t.Fatal("expected non-nil Command")
	}
	if a.Command.Command != "editor.action.formatDocument" {
		t.Errorf("expected Command.Command == editor.action.formatDocument, got %q", a.Command.Command)
	}
	if a.Kind != nil {
		t.Errorf("expected Kind == nil, got %v", *a.Kind)
	}
}

func TestNormalizeCodeActions_CodeActions(t *testing.T) {
	raw := json.RawMessage(`[{"title":"Add import","kind":"quickfix","command":{"title":"Add import","command":"go.add.import","arguments":["fmt"]}}]`)
	result, err := lsp.NormalizeCodeActions(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 action, got %d", len(result))
	}
	a := result[0]
	if a.Title != "Add import" {
		t.Errorf("expected Title == 'Add import', got %q", a.Title)
	}
	if a.Kind == nil || *a.Kind != "quickfix" {
		t.Errorf("expected Kind == quickfix, got %v", a.Kind)
	}
	if a.Command == nil {
		t.Fatal("expected non-nil Command")
	}
	if a.Command.Command != "go.add.import" {
		t.Errorf("expected Command.Command == go.add.import, got %q", a.Command.Command)
	}
}

func TestNormalizeCodeActions_Mixed(t *testing.T) {
	raw := json.RawMessage(`[
  {"title":"Run","command":"test.run"},
  {"title":"Fix error","kind":"quickfix"}
]`)
	result, err := lsp.NormalizeCodeActions(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(result))
	}
	if result[0].Title != "Run" {
		t.Errorf("expected result[0].Title == Run, got %q", result[0].Title)
	}
	if result[0].Kind != nil {
		t.Errorf("expected result[0].Kind == nil, got %v", *result[0].Kind)
	}
	if result[1].Title != "Fix error" {
		t.Errorf("expected result[1].Title == 'Fix error', got %q", result[1].Title)
	}
	if result[1].Kind == nil || *result[1].Kind != "quickfix" {
		t.Errorf("expected result[1].Kind == quickfix, got %v", result[1].Kind)
	}
}

// ---- NormalizeCompletion edge cases ----

func TestNormalizeCompletion_NullString(t *testing.T) {
	result, err := lsp.NormalizeCompletion(json.RawMessage(`null`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty Items, got %d", len(result.Items))
	}
}

func TestNormalizeCompletion_EmptyList(t *testing.T) {
	raw := json.RawMessage(`{"isIncomplete":false,"items":[]}`)
	result, err := lsp.NormalizeCompletion(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsIncomplete {
		t.Error("expected IsIncomplete == false")
	}
	if result.Items == nil {
		t.Error("expected non-nil Items (empty, not nil)")
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
}

func TestNormalizeCompletion_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	result, err := lsp.NormalizeCompletion(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.Items == nil {
		t.Error("expected non-nil Items for empty array")
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
}

// ---- NormalizeCodeActions edge cases ----

func TestNormalizeCodeActions_EmptyArray(t *testing.T) {
	result, err := lsp.NormalizeCodeActions(json.RawMessage(`[]`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 actions, got %d", len(result))
	}
}

func TestNormalizeCodeActions_NullString(t *testing.T) {
	result, err := lsp.NormalizeCodeActions(json.RawMessage(`null`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 actions, got %d", len(result))
	}
}

func TestNormalizeCodeActions_CodeActionWithEdit(t *testing.T) {
	raw := json.RawMessage(`[{"title":"Organize imports","kind":"source.organizeImports","edit":{"changes":{}}}]`)
	result, err := lsp.NormalizeCodeActions(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 action, got %d", len(result))
	}
	if result[0].Title != "Organize imports" {
		t.Errorf("Title = %q, want %q", result[0].Title, "Organize imports")
	}
	if result[0].Kind == nil || *result[0].Kind != "source.organizeImports" {
		t.Errorf("Kind = %v, want source.organizeImports", result[0].Kind)
	}
}

// ---- NormalizeDocumentSymbols edge cases ----

func TestNormalizeDocumentSymbols_InvalidJSON(t *testing.T) {
	_, err := lsp.NormalizeDocumentSymbols(json.RawMessage(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNormalizeDocumentSymbols_NullString(t *testing.T) {
	result, err := lsp.NormalizeDocumentSymbols(json.RawMessage(`null`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d", len(result))
	}
}

func TestNormalizeDocumentSymbols_SymbolInformationOrphans(t *testing.T) {
	// SymbolInformation with a containerName that doesn't match any other symbol.
	raw := json.RawMessage(`[
  {"name":"Method","kind":6,"location":{"uri":"file:///foo.go","range":{"start":{"line":0,"character":0},"end":{"line":1,"character":0}}},"containerName":"NonExistent"}
]`)
	result, err := lsp.NormalizeDocumentSymbols(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// Method's container doesn't exist, so Method is a root.
	if len(result) != 1 {
		t.Fatalf("expected 1 root symbol, got %d", len(result))
	}
	if result[0].Name != "Method" {
		t.Errorf("Name = %q, want %q", result[0].Name, "Method")
	}
}

// Ensure the types package is used directly (suppress unused import warning).
var _ types.DocumentSymbol
