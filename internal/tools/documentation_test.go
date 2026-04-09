package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// TestDocResultMarshal verifies DocResult marshals to expected JSON keys.
func TestDocResultMarshal(t *testing.T) {
	result := DocResult{
		Symbol:    "fmt.Println",
		Language:  "go",
		Source:    "toolchain",
		Doc:       "Println formats using the default formats for its operands",
		Signature: "func Println(a ...any) (n int, err error)",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	for _, key := range []string{"symbol", "language", "source", "doc", "signature"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key: %q", key)
		}
	}
	if m["symbol"] != "fmt.Println" {
		t.Errorf("expected symbol=fmt.Println, got %v", m["symbol"])
	}
	if m["source"] != "toolchain" {
		t.Errorf("expected source=toolchain, got %v", m["source"])
	}
	// error field should be omitted when empty
	if _, ok := m["error"]; ok {
		t.Error("error key should be omitted when empty")
	}
}

// TestStripANSI verifies the ANSI regex strips escape codes correctly.
func TestStripANSI(t *testing.T) {
	input := "\x1b[32mHello\x1b[0m \x1b[1;31mWorld\x1b[0m"
	want := "Hello World"
	got := reANSI.ReplaceAllString(input, "")
	if got != want {
		t.Errorf("StripANSI: got %q, want %q", got, want)
	}
}

// TestGoDocDispatch_MissingSymbol verifies that calling with empty symbol returns ErrorResult.
func TestGoDocDispatch_MissingSymbol(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"symbol":      "",
		"language_id": "go",
	}
	result, err := HandleGetSymbolDocumentation(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing symbol")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "symbol is required") {
		t.Errorf("expected 'symbol is required' message, got %v", result.Content)
	}
}

// TestGoDocDispatch_UnsupportedLang verifies that unsupported languages return DocResult with Source="error".
func TestGoDocDispatch_UnsupportedLang(t *testing.T) {
	ctx := context.Background()
	for _, lang := range []string{"typescript", "javascript", "cobol"} {
		args := map[string]interface{}{
			"symbol":      "someSymbol",
			"language_id": lang,
		}
		result, err := HandleGetSymbolDocumentation(ctx, args)
		if err != nil {
			t.Fatalf("lang %s: unexpected error: %v", lang, err)
		}
		if result.IsError {
			t.Errorf("lang %s: expected IsError=false (structured error), got IsError=true", lang)
		}
		if len(result.Content) == 0 {
			t.Fatalf("lang %s: expected content", lang)
		}
		var doc DocResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &doc); err != nil {
			t.Fatalf("lang %s: unmarshal DocResult: %v", lang, err)
		}
		if doc.Source != "error" {
			t.Errorf("lang %s: expected Source=error, got %q", lang, doc.Source)
		}
		if !strings.Contains(doc.Error, "unsupported language") {
			t.Errorf("lang %s: expected 'unsupported language' in error, got %q", lang, doc.Error)
		}
	}
}

// TestGoDocDispatch_Integration calls go doc fmt.Println and verifies output.
// Skipped if go binary is not in PATH.
func TestGoDocDispatch_Integration(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not in PATH; skipping integration test")
	}
	ctx := context.Background()
	args := map[string]interface{}{
		"symbol":      "fmt.Println",
		"language_id": "go",
	}
	result, err := HandleGetSymbolDocumentation(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	var doc DocResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &doc); err != nil {
		t.Fatalf("unmarshal DocResult: %v", err)
	}
	if doc.Source != "toolchain" {
		t.Errorf("expected Source=toolchain, got %q (error: %s)", doc.Source, doc.Error)
	}
	if !strings.Contains(doc.Doc, "Println") {
		t.Errorf("expected 'Println' in doc text, got %q", doc.Doc)
	}
	if doc.Signature == "" {
		t.Error("expected non-empty signature")
	}
}
