package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	gcfgo "github.com/blackwell-systems/gcf-go"
)

func TestOutputFormatFromContext_Default(t *testing.T) {
	ctx := context.Background()
	got := OutputFormatFromContext(ctx)
	if got != "json" {
		t.Errorf("OutputFormatFromContext(empty ctx) = %q, want %q", got, "json")
	}
}

func TestOutputFormatFromContext_GCF(t *testing.T) {
	ctx := ContextWithOutputFormat(context.Background(), "gcf")
	got := OutputFormatFromContext(ctx)
	if got != "gcf" {
		t.Errorf("OutputFormatFromContext(gcf ctx) = %q, want %q", got, "gcf")
	}
}

func TestOutputFormatFromContext_EmptyString(t *testing.T) {
	ctx := ContextWithOutputFormat(context.Background(), "")
	got := OutputFormatFromContext(ctx)
	if got != "json" {
		t.Errorf("OutputFormatFromContext(empty string ctx) = %q, want %q", got, "json")
	}
}

func TestEncodeResult_JSON(t *testing.T) {
	ctx := context.Background()
	data := map[string]string{"key": "value"}
	result, err := EncodeResult(ctx, data)
	if err != nil {
		t.Fatalf("EncodeResult returned error: %v", err)
	}
	if result.IsError {
		t.Fatal("EncodeResult returned error result")
	}
	if len(result.Content) == 0 {
		t.Fatal("EncodeResult returned empty content")
	}
	// Verify it's valid JSON
	var parsed map[string]string
	if err := json.Unmarshal([]byte(result.Content[0].Text), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("parsed[key] = %q, want %q", parsed["key"], "value")
	}
}

func TestEncodeResult_GCF(t *testing.T) {
	ctx := ContextWithOutputFormat(context.Background(), "gcf")
	data := map[string]string{"key": "value"}
	result, err := EncodeResult(ctx, data)
	if err != nil {
		t.Fatalf("EncodeResult returned error: %v", err)
	}
	if result.IsError {
		t.Fatal("EncodeResult returned error result")
	}
	if len(result.Content) == 0 {
		t.Fatal("EncodeResult returned empty content")
	}
	// GCF stub currently returns empty string; just verify no error
	// After Agent A implements gcf-go, this will return non-empty tabular output
}

// TestEncodeResultJSON_WorkspaceEditRoundTrips guards issue #12: a WorkspaceEdit
// handed back to apply_edit must serialize as self-labeling JSON that survives a
// byte-exact round-trip, never as GCF's flattened tabular form (which LLM callers
// corrupt by transposing range offsets and truncating a large newText).
func TestEncodeResultJSON_WorkspaceEditRoundTrips(t *testing.T) {
	// Hostile newText: newlines, quotes, and a pipe (GCF's field delimiter).
	newText := "MYLOGGER = get(\"x\");\n    if (a > 0 | a < 10) { return a; }\n"
	edit := map[string]any{
		"changes": map[string]any{
			"file:///tmp/Main.java": []any{
				map[string]any{
					"newText": newText,
					"range": map[string]any{
						"start": map[string]any{"line": 99, "character": 32},
						"end":   map[string]any{"line": 337, "character": 14},
					},
				},
			},
		},
	}

	result, err := EncodeResultJSON(edit)
	if err != nil {
		t.Fatalf("EncodeResultJSON returned error: %v", err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatal("EncodeResultJSON returned error or empty content")
	}
	out := result.Content[0].Text

	// Must be JSON, never GCF tabular.
	if strings.HasPrefix(out, "GCF profile=") {
		t.Fatalf("edit payload was GCF-encoded, want JSON: %q", out)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	changes := decoded["changes"].(map[string]any)
	edits := changes["file:///tmp/Main.java"].([]any)
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1 (no merge/split)", len(edits))
	}
	te := edits[0].(map[string]any)
	if te["newText"] != newText {
		t.Errorf("newText corrupted on round-trip:\n got  %q\n want %q", te["newText"], newText)
	}
	rng := te["range"].(map[string]any)
	start := rng["start"].(map[string]any)
	end := rng["end"].(map[string]any)
	// Offsets must not transpose: start=99:32, end=337:14.
	if start["character"].(float64) != 32 || end["character"].(float64) != 14 {
		t.Errorf("range offsets transposed: start.char=%v end.char=%v, want 32/14",
			start["character"], end["character"])
	}
}

// TestEncodeResultJSON_IgnoresGCFContext ensures the JSON encoder ignores a gcf
// output-format context: edit payloads are JSON regardless of the session format.
func TestEncodeResultJSON_IgnoresGCFContext(t *testing.T) {
	// (context intentionally unused; EncodeResultJSON is format-independent by design)
	_ = ContextWithOutputFormat(context.Background(), "gcf")
	result, err := EncodeResultJSON(map[string]any{"changes": map[string]any{}})
	if err != nil {
		t.Fatalf("EncodeResultJSON returned error: %v", err)
	}
	if strings.HasPrefix(result.Content[0].Text, "GCF profile=") {
		t.Fatal("EncodeResultJSON produced GCF output; must always be JSON")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestEncodeResult_UnknownFormat(t *testing.T) {
	ctx := ContextWithOutputFormat(context.Background(), "xml")
	data := map[string]string{"key": "value"}
	result, err := EncodeResult(ctx, data)
	if err != nil {
		t.Fatalf("EncodeResult returned error: %v", err)
	}
	if result.IsError {
		t.Fatal("EncodeResult returned error result for unknown format")
	}
	// Unknown format should fall back to JSON
	var parsed map[string]string
	if err := json.Unmarshal([]byte(result.Content[0].Text), &parsed); err != nil {
		t.Fatalf("unknown format result is not valid JSON: %v", err)
	}
}

func TestEncodeResult_GCF_GraphPayload(t *testing.T) {
	ctx := ContextWithOutputFormat(context.Background(), "gcf")
	p := &gcfgo.Payload{
		Tool: "test_tool",
		Symbols: []gcfgo.Symbol{
			{QualifiedName: "pkg.Func", Kind: "function", Score: 1.0, Provenance: "lsp_resolved", Distance: 0},
		},
		Edges: []gcfgo.Edge{
			{Source: "pkg.Func", Target: "pkg.Caller", EdgeType: "called_by"},
		},
	}
	result, err := EncodeResult(ctx, p)
	if err != nil {
		t.Fatalf("EncodeResult returned error: %v", err)
	}
	if result.IsError {
		t.Fatal("EncodeResult returned error result")
	}
	if len(result.Content) == 0 {
		t.Fatal("EncodeResult returned empty content")
	}
	text := result.Content[0].Text
	if text == "" {
		t.Fatal("EncodeResult returned empty text for graph payload")
	}
	if !strings.Contains(text, "pkg.Func") {
		t.Errorf("expected output to contain 'pkg.Func', got: %s", text)
	}
}

func TestEncodeResult_JSONMarshalError(t *testing.T) {
	ctx := context.Background()
	// Channels cannot be marshaled to JSON
	data := make(chan int)
	result, err := EncodeResult(ctx, data)
	if err != nil {
		t.Fatalf("EncodeResult returned error: %v", err)
	}
	// Should return an error result, not panic
	if !result.IsError {
		t.Fatal("EncodeResult should return error result for unmarshalable data")
	}
}
