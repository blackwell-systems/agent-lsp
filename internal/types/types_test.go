package types

import "testing"

func TestTextResult(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"empty string", ""},
		{"simple text", "hello world"},
		{"multiline", "line1\nline2\nline3"},
		{"json-like", `{"key": "value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := TextResult(tt.text)
			if r.IsError {
				t.Error("TextResult should not set IsError")
			}
			if len(r.Content) != 1 {
				t.Fatalf("expected 1 content item, got %d", len(r.Content))
			}
			if r.Content[0].Type != "text" {
				t.Errorf("expected type %q, got %q", "text", r.Content[0].Type)
			}
			if r.Content[0].Text != tt.text {
				t.Errorf("expected text %q, got %q", tt.text, r.Content[0].Text)
			}
		})
	}
}

func TestErrorResult(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"simple error", "something went wrong"},
		{"empty message", ""},
		{"with details", "file not found: /tmp/missing.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ErrorResult(tt.msg)
			if !r.IsError {
				t.Error("ErrorResult should set IsError=true")
			}
			if len(r.Content) != 1 {
				t.Fatalf("expected 1 content item, got %d", len(r.Content))
			}
			if r.Content[0].Type != "text" {
				t.Errorf("expected type %q, got %q", "text", r.Content[0].Type)
			}
			if r.Content[0].Text != tt.msg {
				t.Errorf("expected text %q, got %q", tt.msg, r.Content[0].Text)
			}
		})
	}
}

func TestInlayHintKindConstants(t *testing.T) {
	if InlayHintKindType != 1 {
		t.Errorf("InlayHintKindType = %d, want 1", InlayHintKindType)
	}
	if InlayHintKindParameter != 2 {
		t.Errorf("InlayHintKindParameter = %d, want 2", InlayHintKindParameter)
	}
}

func TestDocumentHighlightKindConstants(t *testing.T) {
	if DocumentHighlightText != 1 {
		t.Errorf("DocumentHighlightText = %d, want 1", DocumentHighlightText)
	}
	if DocumentHighlightRead != 2 {
		t.Errorf("DocumentHighlightRead = %d, want 2", DocumentHighlightRead)
	}
	if DocumentHighlightWrite != 3 {
		t.Errorf("DocumentHighlightWrite = %d, want 3", DocumentHighlightWrite)
	}
}
