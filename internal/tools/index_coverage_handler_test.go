package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// --- find_symbol empty-result qualification (issue #42) ---

// An initialized server that never declares workspaceSymbolProvider: the
// silent capability gate in GetWorkspaceSymbols produces the empty result
// and the handler must say so instead of returning a bare
// "not found"-looking empty result. (A fresh client is rejected earlier by
// the initialized-client guard — CodeRabbit #59 thread 2.)
func TestHandleGetWorkspaceSymbols_CapabilityUnavailable(t *testing.T) {
	client := startFakeLSP(t, nil, []any{}, false)

	r, err := HandleGetWorkspaceSymbols(context.Background(), client, map[string]any{
		"query": "StopLong",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error result: %v", r.Content)
	}
	if len(r.Content) == 0 {
		t.Fatal("expected content")
	}
	// appendHint adds the hint as a separate content item after the primary
	// result; scan every item.
	var all string
	for _, c := range r.Content {
		all += c.Text + "\n"
	}
	if !strings.Contains(all, "does not declare the workspaceSymbolProvider capability") {
		t.Fatalf("expected capability-unavailable hint, got: %q", all)
	}
}

// --- noteIndexCoverage wording (issue #42) ---

func TestUnopenedFilesCaveatText(t *testing.T) {
	// The caveat must name the limitation and the recovery path.
	for _, want := range []string{"only index opened documents", "open_document"} {
		if !strings.Contains(unopenedFilesCaveat, want) {
			t.Errorf("caveat missing %q: %s", want, unopenedFilesCaveat)
		}
	}
}

// An empty find_symbol result must keep the established response envelope:
// the caveat is appended to the detail-specific encoding (the
// workspaceSymbolsResponse object with total/symbols for hover detail), not
// returned as a bare array instead of it. (CodeRabbit #51 thread 1)
func TestHandleGetWorkspaceSymbols_EmptyHoverKeepsEnvelope(t *testing.T) {
	// Initialized fake server without the capability → the caveat branch;
	// a fresh client would be rejected earlier by the initialized-client
	// guard (CodeRabbit #59 thread 2).
	client := startFakeLSP(t, nil, []any{}, false)

	r, err := HandleGetWorkspaceSymbols(context.Background(), client, map[string]any{
		"query": "StopLong", "detail_level": "hover",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error result: %v", r.Content)
	}
	if len(r.Content) == 0 {
		t.Fatal("expected content")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(r.Content[0].Text), &payload); err != nil {
		t.Fatalf("hover envelope broken: %v (%q)", err, r.Content[0].Text)
	}
	if _, ok := payload["total"]; !ok {
		t.Errorf("hover envelope missing total: %v", payload)
	}
	if _, ok := payload["symbols"]; !ok {
		t.Errorf("hover envelope missing symbols: %v", payload)
	}
	// The empty-cause caveat must still ride along as the hint.
	var all string
	for _, c := range r.Content {
		all += c.Text + "\n"
	}
	if !strings.Contains(all, "does not declare the workspaceSymbolProvider capability") {
		t.Fatalf("expected empty-cause caveat in hint, got: %q", all)
	}
}
