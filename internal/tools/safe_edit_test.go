package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
// Regression tests for the preview seam between
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

// TestHandleSafeApplyEdit_LiveGopls_GCF drives the success and refusal paths of
// safe_apply_edit under GCF, the default output format, against real gopls.
// Before the preview struct was passed directly, re-parsing the GCF preview
// text as JSON failed, so safe_apply_edit never applied anything by default;
// the existing GCF test only covers an error path and could not catch that.
// Skips when gopls is unavailable.
func TestHandleSafeApplyEdit_LiveGopls_GCF(t *testing.T) {
	gopls := findGopls()
	if gopls == "" {
		t.Skip("gopls not found; skipping live safe_apply_edit test")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module safeedit\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc add(a, b int) int { return a + b }\n\nfunc main() { _ = add(1, 2) }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := lsp.NewLSPClient(gopls, nil)
	if err := client.Initialize(ctx, dir); err != nil {
		t.Fatalf("initialize gopls: %v", err)
	}
	defer client.Shutdown(context.Background())
	mgr := session.NewSessionManager(&fakeResolver{client: client})
	gcf := ContextWithOutputFormat(ctx, "gcf")

	// An edit that introduces a type error must be refused and leave the file alone.
	before, _ := os.ReadFile(src)
	r, err := HandleSafeApplyEdit(gcf, client, mgr, map[string]any{
		"file_path": src, "old_text": "return a + b", "new_text": `return a + "b"`,
	})
	if err != nil {
		t.Fatalf("type-error edit: Go error %v", err)
	}
	t.Logf("type-error edit result (IsError=%v):\n%s", r.IsError, resultText(r))
	if after, _ := os.ReadFile(src); string(after) != string(before) {
		t.Fatalf("an edit introducing a type error was written to disk: %s", resultText(r))
	}
	if strings.Contains(resultText(r), "failed to parse preview result") {
		t.Fatalf("GCF preview parse failure: %s", resultText(r))
	}

	// A safe edit must be applied.
	r, err = HandleSafeApplyEdit(gcf, client, mgr, map[string]any{
		"file_path": src, "old_text": "return a + b", "new_text": "return b + a",
	})
	if err != nil || r.IsError {
		t.Fatalf("safe edit failed under GCF: err=%v result=%s", err, resultText(r))
	}
	if after, _ := os.ReadFile(src); !strings.Contains(string(after), "return b + a") {
		t.Fatalf("safe edit was not written: %s", resultText(r))
	}
}

// safe_apply_edit must apply only verified results. net_delta is 0 when the
// diagnostics wait timed out or the server was still indexing, because nothing
// was checked; that must not be treated as safe.
func TestPreviewBlocksApply(t *testing.T) {
	tests := []struct {
		name      string
		netDelta  int
		preview   map[string]any
		wantBlock bool
	}{
		{"verified clean", 0, map[string]any{"confidence": "high", "timeout": false}, false},
		{"workspace scope clean", 0, map[string]any{"confidence": "eventual", "timeout": false}, false},
		{"introduces errors", 1, map[string]any{"confidence": "high", "timeout": false}, true},
		{"timed out", 0, map[string]any{"confidence": "partial", "timeout": true}, true},
		{"still indexing", 0, map[string]any{"confidence": "low", "timeout": false}, true},
		{"timeout flag alone", 0, map[string]any{"confidence": "high", "timeout": true}, true},
		{"resolves errors, verified", -1, map[string]any{"confidence": "high", "timeout": false}, false},
	}
	for _, tt := range tests {
		blocked, hint := previewBlocksApply(tt.netDelta, tt.preview)
		if blocked != tt.wantBlock {
			t.Errorf("%s: blocked = %v, want %v (hint %q)", tt.name, blocked, tt.wantBlock, hint)
		}
		if blocked && hint == "" {
			t.Errorf("%s: blocked without a hint", tt.name)
		}
	}
}
