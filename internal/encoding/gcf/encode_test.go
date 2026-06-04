package gcf

import (
	"strings"
	"testing"
)

// symbolRef represents a typical agent-lsp tool response structure.
type symbolRef struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
}

func TestEncode_StructSlice(t *testing.T) {
	data := []symbolRef{
		{Name: "Encode", File: "encode.go", Line: 10},
		{Name: "Decode", File: "decode.go", Line: 20},
	}
	result, err := Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result for struct slice")
	}
	// Verify field names appear in output
	if !strings.Contains(result, "Name") && !strings.Contains(result, "name") {
		t.Errorf("expected output to contain field name 'Name' or 'name', got: %s", result)
	}
	// Verify values appear in output
	if !strings.Contains(result, "Encode") {
		t.Errorf("expected output to contain value 'Encode', got: %s", result)
	}
	if !strings.Contains(result, "Decode") {
		t.Errorf("expected output to contain value 'Decode', got: %s", result)
	}
	t.Logf("Struct slice output:\n%s", result)
}

func TestEncode_MapWithNestedSlice(t *testing.T) {
	data := map[string]any{
		"symbols": []string{"foo", "bar", "baz"},
		"count":   3,
	}
	result, err := Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result for map")
	}
	t.Logf("Map output:\n%s", result)
}

func TestEncode_Nil(t *testing.T) {
	result, err := Encode(nil)
	if err != nil {
		t.Fatalf("unexpected error for nil: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string for nil, got: %q", result)
	}
}

func TestEncode_SingleStruct(t *testing.T) {
	data := symbolRef{Name: "Main", File: "main.go", Line: 1}
	result, err := Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result for single struct")
	}
	if !strings.Contains(result, "Main") {
		t.Errorf("expected output to contain 'Main', got: %s", result)
	}
	t.Logf("Single struct output:\n%s", result)
}

func TestEncode_RoundTrip_FieldsAndValues(t *testing.T) {
	data := []symbolRef{
		{Name: "FindReferences", File: "references.go", Line: 42},
		{Name: "GetDefinition", File: "definition.go", Line: 15},
		{Name: "ListSymbols", File: "symbols.go", Line: 88},
	}
	result, err := Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All values should be present in the output
	for _, sym := range data {
		if !strings.Contains(result, sym.Name) {
			t.Errorf("expected output to contain name %q", sym.Name)
		}
		if !strings.Contains(result, sym.File) {
			t.Errorf("expected output to contain file %q", sym.File)
		}
	}
	t.Logf("Round-trip output:\n%s", result)
}

func TestEncode_EmptySlice(t *testing.T) {
	data := []symbolRef{}
	result, err := Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty slice should produce some output (possibly just headers or empty)
	t.Logf("Empty slice output: %q", result)
}

func TestEncode_Primitive(t *testing.T) {
	result, err := Encode("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "hello world") {
		t.Errorf("expected output to contain 'hello world', got: %q", result)
	}
}
