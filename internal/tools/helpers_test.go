package tools

import (
	"context"
	"encoding/json"
	"testing"
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
