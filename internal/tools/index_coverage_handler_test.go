package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// --- find_symbol empty-result qualification (issue #42) ---

// A client whose server never declared workspaceSymbolProvider produces an
// empty find_symbol result through GetWorkspaceSymbols' silent
// capability gate. The handler must say so instead of returning a bare
// "not found"-looking empty result.
func TestHandleGetWorkspaceSymbols_CapabilityUnavailable(t *testing.T) {
	client := lsp.NewLSPClient("fake", nil)

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
