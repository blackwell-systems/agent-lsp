package resources

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

func outsideFile(t *testing.T) (*lsp.LSPClient, string) {
	t.Helper()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("TOPSECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := lsp.NewLSPClient("unused", nil)
	c.SetRootDirForTest(root)
	return c, outside
}

func hoverURI(scheme, path string) string {
	q := url.Values{"line": {"1"}, "column": {"1"}, "language_id": {"plaintext"}}
	return scheme + "://" + path + "?" + q.Encode()
}

// lsp-hover://, lsp-completions:// and lsp-diagnostics:// take a path from the
// client. A path outside the workspace must be refused before it is read or
// sent to the language server (issue: workspace confinement bypass).
func TestResources_RefusePathOutsideRoot(t *testing.T) {
	c, outside := outsideFile(t)
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"hover": func() error {
			_, err := HandleHoverResource(ctx, c, hoverURI("lsp-hover", outside))
			return err
		},
		"completions": func() error {
			_, err := HandleCompletionsResource(ctx, c, hoverURI("lsp-completions", outside))
			return err
		},
		"diagnostics": func() error {
			_, err := HandleDiagnosticsResource(ctx, c, "lsp-diagnostics://"+outside)
			return err
		},
	} {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "outside workspace root") {
			t.Errorf("%s: want an 'outside workspace root' error, got %v", name, err)
		}
		if err != nil && strings.Contains(err.Error(), "TOPSECRET") {
			t.Errorf("%s: error quotes the outside file: %v", name, err)
		}
	}
}

// With no workspace root nothing can be confined, so nothing is read.
func TestResources_RefuseWithoutRoot(t *testing.T) {
	_, outside := outsideFile(t)
	c := lsp.NewLSPClient("unused", nil)
	_, err := HandleHoverResource(context.Background(), c, hoverURI("lsp-hover", outside))
	if err == nil || !strings.Contains(err.Error(), "start_lsp") {
		t.Fatalf("want a 'call start_lsp first' error, got %v", err)
	}
}
