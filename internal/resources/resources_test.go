package resources

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// ---------------------------------------------------------------------------
// URI parsing tests
// ---------------------------------------------------------------------------

// TestURIParsing_Diagnostics verifies lsp-diagnostics URI path extraction.
func TestURIParsing_Diagnostics(t *testing.T) {
	tests := []struct {
		uri      string
		wantPath string
		wantAll  bool
	}{
		{"lsp-diagnostics://", "", true},
		{"lsp-diagnostics:///", "/", true},
		{"lsp-diagnostics:///path/to/file.go", "/path/to/file.go", false},
	}
	for _, tc := range tests {
		parsed, err := url.Parse(tc.uri)
		if err != nil {
			t.Errorf("url.Parse(%q): %v", tc.uri, err)
			continue
		}
		path := parsed.Path
		isAll := path == "" || path == "/"
		if isAll != tc.wantAll {
			t.Errorf("URI %q: isAll=%v, want %v (path=%q)", tc.uri, isAll, tc.wantAll, path)
		}
		if !tc.wantAll && path != tc.wantPath {
			t.Errorf("URI %q: path=%q, want %q", tc.uri, path, tc.wantPath)
		}
	}
}

// TestURIParsing_HoverCompletions verifies query param extraction for hover/completions URIs.
func TestURIParsing_HoverCompletions(t *testing.T) {
	uri := "lsp-hover:///path/to/file.go?line=10&column=5&language_id=go"
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("line") != "10" {
		t.Errorf("line=%q, want %q", q.Get("line"), "10")
	}
	if q.Get("column") != "5" {
		t.Errorf("column=%q, want %q", q.Get("column"), "5")
	}
	if q.Get("language_id") != "go" {
		t.Errorf("language_id=%q, want %q", q.Get("language_id"), "go")
	}
	if parsed.Path != "/path/to/file.go" {
		t.Errorf("path=%q, want %q", parsed.Path, "/path/to/file.go")
	}
}

// ---------------------------------------------------------------------------
// ResourceResult / ResourceTemplates tests
// ---------------------------------------------------------------------------

// TestResourceTemplates verifies that 3 static templates are returned.
func TestResourceTemplates(t *testing.T) {
	templates := ResourceTemplates()
	if len(templates) != 3 {
		t.Fatalf("len(resourceTemplates())=%d, want 3", len(templates))
	}
	names := map[string]bool{}
	for _, tmpl := range templates {
		names[tmpl.Name] = true
		if tmpl.URITemplate == "" {
			t.Errorf("template %q has empty URITemplate", tmpl.Name)
		}
	}
	for _, want := range []string{"lsp-diagnostics", "lsp-hover", "lsp-completions"} {
		if !names[want] {
			t.Errorf("missing template %q", want)
		}
	}
}

// TestResourceResultJSON verifies ResourceResult marshals correctly.
func TestResourceResultJSON(t *testing.T) {
	rr := ResourceResult{
		URI:      "lsp-diagnostics:///foo.go",
		MIMEType: "application/json",
		Text:     `{"file://foo.go":[]}`,
	}
	data, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, field := range []string{"uri", "mimeType", "text"} {
		if _, ok := out[field]; !ok {
			t.Errorf("missing JSON field %q", field)
		}
	}
}

// ---------------------------------------------------------------------------
// Subscription tests (logic only — no LSP server required)
// ---------------------------------------------------------------------------

// subscriptionCallRecorder records which URIs were passed to notify.
type subscriptionCallRecorder struct {
	calls []string
}

func (r *subscriptionCallRecorder) notify(uri string) {
	r.calls = append(r.calls, uri)
}

// TestHandleSubscribeDiagnostics_FileSpecific verifies that a file-specific
// subscription only fires notify for the correct file URI.
func TestHandleSubscribeDiagnostics_FileSpecific(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	rec := &subscriptionCallRecorder{}

	sub, err := HandleSubscribeDiagnostics(
		context.Background(),
		client,
		"lsp-diagnostics:///path/to/file.go",
		rec.notify,
	)
	if err != nil {
		t.Fatalf("HandleSubscribeDiagnostics: %v", err)
	}
	if sub == nil {
		t.Fatal("expected non-nil SubscriptionContext")
	}
	if sub.Callback == nil {
		t.Fatal("expected non-nil Callback in SubscriptionContext")
	}

	// Simulate diagnostic events.
	sub.Callback("file:///path/to/file.go", []types.LSPDiagnostic{})
	sub.Callback("file:///other/file.go", []types.LSPDiagnostic{})

	if len(rec.calls) != 1 {
		t.Errorf("notify called %d times, want 1", len(rec.calls))
	}
	if len(rec.calls) > 0 && rec.calls[0] != "file:///path/to/file.go" {
		t.Errorf("notify called with %q, want %q", rec.calls[0], "file:///path/to/file.go")
	}
}

// TestHandleSubscribeDiagnostics_AllFiles verifies that an all-files subscription
// fires notify for any file:// URI.
func TestHandleSubscribeDiagnostics_AllFiles(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	rec := &subscriptionCallRecorder{}

	sub, err := HandleSubscribeDiagnostics(
		context.Background(),
		client,
		"lsp-diagnostics://",
		rec.notify,
	)
	if err != nil {
		t.Fatalf("HandleSubscribeDiagnostics: %v", err)
	}

	sub.Callback("file:///path/to/a.go", []types.LSPDiagnostic{})
	sub.Callback("file:///path/to/b.go", []types.LSPDiagnostic{})

	if len(rec.calls) != 2 {
		t.Errorf("notify called %d times, want 2", len(rec.calls))
	}
}

// TestHandleUnsubscribeDiagnostics verifies that after unsubscription, the
// callback pointer stored in SubscriptionContext is stable (we can't assert
// it won't fire without an LSP server, but we verify no error is returned).
func TestHandleUnsubscribeDiagnostics(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	rec := &subscriptionCallRecorder{}

	sub, err := HandleSubscribeDiagnostics(
		context.Background(),
		client,
		"lsp-diagnostics:///path/to/file.go",
		rec.notify,
	)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	err = HandleUnsubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///path/to/file.go", sub)
	if err != nil {
		t.Fatalf("HandleUnsubscribeDiagnostics: %v", err)
	}

	// After unsubscription, directly invoking the callback should still work
	// (callback itself is valid), but the LSP client should no longer route to it.
	// We verify no panic occurs.
	sub.Callback("file:///path/to/file.go", nil)
}

// TestHandleUnsubscribeDiagnostics_NilSub verifies that unsubscribing with
// a nil SubscriptionContext returns no error.
func TestHandleUnsubscribeDiagnostics_NilSub(t *testing.T) {
	client := lsp.NewLSPClient("/nonexistent/gopls", nil)
	err := HandleUnsubscribeDiagnostics(context.Background(), client, "lsp-diagnostics:///foo.go", nil)
	if err != nil {
		t.Errorf("unexpected error for nil sub: %v", err)
	}
}

// TestHandleDiagnosticsResource_AllFiles and TestHandleDiagnosticsResource_SpecificFile
// are integration tests that require a running LSP server. They are skipped when
// a real server is not available.

// TestHandleDiagnosticsResource_AllFiles verifies that calling with path="" or "/"
// returns diagnostics keyed by file URI.
func TestHandleDiagnosticsResource_AllFiles(t *testing.T) {
	// Integration test: skip if no LSP server binary is present.
	t.Skip("integration test: requires running LSP server")

	client := lsp.NewLSPClient("/usr/local/bin/gopls", nil)
	ctx := context.Background()
	if err := client.Initialize(ctx, t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer client.Shutdown(ctx) //nolint:errcheck

	result, err := HandleDiagnosticsResource(ctx, client, "lsp-diagnostics://")
	if err != nil {
		t.Fatalf("HandleDiagnosticsResource: %v", err)
	}
	if result.MIMEType != "application/json" {
		t.Errorf("MIMEType=%q, want application/json", result.MIMEType)
	}

	var diags map[string][]types.LSPDiagnostic
	if err := json.Unmarshal([]byte(result.Text), &diags); err != nil {
		t.Errorf("result.Text is not valid JSON map: %v", err)
	}
}

// TestHandleDiagnosticsResource_SpecificFile verifies that calling with a file path
// returns diagnostics only for that file.
func TestHandleDiagnosticsResource_SpecificFile(t *testing.T) {
	t.Skip("integration test: requires running LSP server")

	client := lsp.NewLSPClient("/usr/local/bin/gopls", nil)
	ctx := context.Background()
	if err := client.Initialize(ctx, t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer client.Shutdown(ctx) //nolint:errcheck

	result, err := HandleDiagnosticsResource(ctx, client, "lsp-diagnostics:///path/to/file.go")
	if err != nil {
		// File may not exist, which is acceptable — we just check the URI handling.
		t.Logf("HandleDiagnosticsResource returned error (acceptable if file absent): %v", err)
		return
	}
	if result.URI != "lsp-diagnostics:///path/to/file.go" {
		t.Errorf("result.URI=%q, want %q", result.URI, "lsp-diagnostics:///path/to/file.go")
	}
}
