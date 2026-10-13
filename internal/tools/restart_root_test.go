package tools

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// --- restart_lsp_server root_dir resolution (issue #3A) ---

// The MCP schema declares root_dir optional ("If omitted, restarts with
// current root") — the handler must fall back to the client's current root
// instead of rejecting the call.
func TestResolveRestartRoot_FallsBackToCurrentRoot(t *testing.T) {
	client := lsp.NewLSPClient("fake", nil)
	client.SetRootDirForTest("/tmp/current-ws")

	// root_dir omitted → the client's current root is reused, matching the
	// MCP schema contract ("If omitted, restarts with current root").
	root, err := resolveRestartRoot(client, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root != "/tmp/current-ws" {
		t.Fatalf("expected current-root fallback, got %q", root)
	}
}

func TestResolveRestartRoot_NoRootAnywhere(t *testing.T) {
	client := lsp.NewLSPClient("fake", nil)
	// No args and no initialized root → descriptive error (not the old bare one).
	_, got := resolveRestartRoot(client, map[string]any{})
	if got == nil || !strings.Contains(got.Error(), "call start_lsp first") {
		t.Fatalf("expected descriptive error when no root exists, got %v", got)
	}
}

func TestResolveRestartRoot_ExplicitRootWins(t *testing.T) {
	client := lsp.NewLSPClient("fake", nil)
	root, err := resolveRestartRoot(client, map[string]any{"root_dir": "/tmp/ws"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root != "/tmp/ws" {
		t.Fatalf("expected explicit root_dir to win, got %q", root)
	}
}
