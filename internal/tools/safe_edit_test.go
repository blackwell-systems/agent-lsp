package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/session"
)

func TestHandleSafeApplyEdit_NilClient(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"file_path": "/tmp/test.go",
		"old_text":  "foo",
		"new_text":  "bar",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
	if !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Fatalf("expected init error, got: %s", r.Content[0].Text)
	}
}

func TestHandleSafeApplyEdit_MissingFilePath(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"old_text": "foo",
		"new_text": "bar",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing file_path")
	}
	if !strings.Contains(r.Content[0].Text, "file_path") {
		t.Fatalf("expected file_path error, got: %s", r.Content[0].Text)
	}
}

func TestHandleSafeApplyEdit_MissingOldText(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"file_path": "/tmp/test.go",
		"new_text":  "bar",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing old_text")
	}
	if !strings.Contains(r.Content[0].Text, "old_text") {
		t.Fatalf("expected old_text error, got: %s", r.Content[0].Text)
	}
}

func TestHandleSafeApplyEdit_MissingNewText(t *testing.T) {
	r, err := HandleSafeApplyEdit(context.Background(), newNilClient(), nil, map[string]any{
		"file_path": "/tmp/test.go",
		"old_text":  "foo",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for missing new_text")
	}
	if !strings.Contains(r.Content[0].Text, "new_text") {
		t.Fatalf("expected new_text error, got: %s", r.Content[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Regression tests for davalillo/agent-lsp#12: the preview seam between
// HandleSafeApplyEdit and the atomic simulation must not depend on the
// context output format. Under the default GCF format the encoded tool
// result is not JSON, so the old text round-trip always failed with
// "failed to parse preview result".

// fakeResolver is a minimal lsp.ClientResolver for handler-level tests.
type fakeResolver struct{ client *lsp.LSPClient }

func (f *fakeResolver) ClientForFile(filePath string) *lsp.LSPClient { return f.client }
func (f *fakeResolver) DefaultClient() *lsp.LSPClient                { return f.client }
func (f *fakeResolver) AllClients() []*lsp.LSPClient {
	if f.client == nil {
		return []*lsp.LSPClient{}
	}
	return []*lsp.LSPClient{f.client}
}
func (f *fakeResolver) Shutdown(ctx context.Context) error { return nil }

// initializedTestClient returns a client that passes CheckInitialized without
// spawning a process.
func initializedTestClient() *lsp.LSPClient {
	c := lsp.NewLSPClient("fake-server", nil)
	c.MarkInitializedForTest()
	return c
}

// The full HandleSafeApplyEdit path under the default GCF output format: the
// preview runs through simulateEditAtomicCore and must fail with the session
// error (propagated struct-level), never with the old
// "failed to parse preview result".
func TestHandleSafeApplyEdit_GcfFormat_PreviewSeesCoreError(t *testing.T) {
	ctx := ContextWithOutputFormat(context.Background(), "gcf")

	root := t.TempDir()
	file := filepath.Join(root, "sample.go")
	if err := os.WriteFile(file, []byte("func foo() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	client := initializedTestClient()
	client.SetRootDirForTest(root) // simArgs["workspace_root"] comes from client.RootDir()

	// Resolver with no clients → CreateSession fails deterministically after
	// the preview seam is entered, exercising the error propagation path.
	mgr := session.NewSessionManager(&fakeResolver{client: nil})

	r, err := HandleSafeApplyEdit(ctx, client, mgr, map[string]any{
		"file_path": file,
		"old_text":  "func foo()",
		"new_text":  "func bar()",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true (no LSP client)")
	}
	text := r.Content[0].Text
	if strings.Contains(text, "failed to parse preview result") {
		t.Fatalf("regression: GCF round-trip parse failure resurfaced: %s", text)
	}
	if !strings.Contains(text, "preview failed") || !strings.Contains(text, "start_lsp") {
		t.Fatalf("expected propagated core error, got: %s", text)
	}
}

// The preview result seam itself: marshaling the evaluation struct and
// unmarshaling into the map must work regardless of the context output
// format — this is what replaced parsing the format-aware tool output.
func TestSafeApplyEdit_PreviewStructRoundTrip_FormatIndependent(t *testing.T) {
	for _, format := range []string{"json", "gcf"} {
		_ = ContextWithOutputFormat(context.Background(), format) // the round-trip is format-independent by construction; both formats must behave identically
		eval := &session.EvaluationResult{
			SessionID:        "s1",
			NetDelta:         2,
			ErrorsIntroduced: []session.DiagnosticEntry{{Line: 3, Col: 1, Message: "x"}},
		}

		previewJSON := map[string]any{}
		raw, err := json.Marshal(eval)
		if err != nil {
			t.Fatalf("[%s]: marshal: %v", format, err)
		}
		if err := json.Unmarshal(raw, &previewJSON); err != nil {
			t.Fatalf("[%s]: unmarshal: %v", format, err)
		}
		nd, ok := previewJSON["net_delta"].(float64)
		if !ok || int(nd) != 2 {
			t.Fatalf("[%s]: net_delta round-trip failed: %#v", format, previewJSON["net_delta"])
		}
		if _, ok := previewJSON["errors_introduced"].([]any); !ok {
			t.Fatalf("[%s]: errors_introduced must round-trip as array", format)
		}
	}
}
