package lsp

import (
	"encoding/json"
	"testing"
)

// TestHasActiveProgress verifies the accessor tracks open work-done progress
// tokens: true while a begin has no matching end, false once all tokens close.
func TestHasActiveProgress(t *testing.T) {
	c := NewLSPClient("/bin/echo", nil)

	if c.HasActiveProgress() {
		t.Fatal("expected no active progress on a fresh client")
	}

	begin := func(token string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"token": token, "value": map[string]any{"kind": "begin"}})
		return b
	}
	end := func(token string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"token": token, "value": map[string]any{"kind": "end"}})
		return b
	}

	c.handleProgress(begin("indexing"))
	if !c.HasActiveProgress() {
		t.Error("expected active progress after a begin token")
	}

	// A second concurrent token (e.g. cache priming) keeps progress active.
	c.handleProgress(begin("priming"))
	c.handleProgress(end("indexing"))
	if !c.HasActiveProgress() {
		t.Error("expected still-active progress while one token remains open")
	}

	c.handleProgress(end("priming"))
	if c.HasActiveProgress() {
		t.Error("expected no active progress once all tokens closed")
	}
}
