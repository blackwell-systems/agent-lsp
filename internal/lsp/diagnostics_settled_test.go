package lsp

import (
	"context"
	"io"
	"testing"
	"time"
)

func publishDiags(t *testing.T, w io.Writer, uri string, version *int) {
	t.Helper()
	params := map[string]any{"uri": uri, "diagnostics": []any{}}
	if version != nil {
		params["version"] = *version
	}
	if err := writeMsg(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params":  params,
	}); err != nil {
		t.Fatalf("writeMsg: %v", err)
	}
}

// waitForGen blocks until the client has processed n publications for uri.
func waitForGen(t *testing.T, c *LSPClient, uri string, n uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if gen, _, _ := c.diagnosticsState(uri); gen >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("publication %d for %s not processed", n, uri)
}

func intp(v int) *int { return &v }

// strictClient returns a test client whose server is known to republish
// diagnostics after every change, as gopls does.
func strictClient(t *testing.T) (*LSPClient, io.WriteCloser) {
	c, serverW, _ := newTestClient(t)
	c.diagMu.Lock()
	c.republishesUnchanged = true
	c.diagMu.Unlock()
	return c, serverW
}

func TestWaitForDiagnosticsSettled_TimeoutIsNotSettled(t *testing.T) {
	c, _ := strictClient(t)
	uri := "file:///tmp/settled/a.go"
	start := time.Now()
	settled, err := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, nil, 300)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if settled {
		t.Fatal("settled = true with no publication")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("wait took %v, want about the 300ms timeout", d)
	}
}

// A cached publication from before the edit must not settle the wait.
func TestWaitForDiagnosticsSettled_CachedPublicationDoesNotCount(t *testing.T) {
	c, serverW := strictClient(t)
	uri := "file:///tmp/settled/a.go"
	publishDiags(t, serverW, uri, nil)
	waitForGen(t, c, uri, 1)

	settled, _ := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, nil, 300)
	if settled {
		t.Fatal("settled = true on a publication that predates the wait")
	}
}

// The first publication for a URI with an empty cache is fresh; the old wait
// skipped it as if it were the subscription's cached replay.
func TestWaitForDiagnosticsSettled_FirstPublicationCounts(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	uri := "file:///tmp/settled/a.go"
	go func() {
		time.Sleep(50 * time.Millisecond)
		publishDiags(t, serverW, uri, nil)
	}()
	settled, err := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, nil, 3000)
	if err != nil || !settled {
		t.Fatalf("settled = %v, err = %v; want settled", settled, err)
	}
}

// A publication for an older document version is stale even though it
// arrived after the mark.
func TestWaitForDiagnosticsSettled_StaleVersionDoesNotSettle(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	uri := "file:///tmp/settled/a.go"
	mark := c.MarkDiagnostics(uri)
	mark.Version = 3
	publishDiags(t, serverW, uri, intp(2))
	waitForGen(t, c, uri, 1)

	settled, _ := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, map[string]DiagnosticsMark{uri: mark}, 800)
	if settled {
		t.Fatal("settled = true on diagnostics for version 2 when the edit made version 3")
	}
}

// A publication that reached the edit's version settles the wait even if it
// arrived before the wait started (between the edit and Evaluate).
func TestWaitForDiagnosticsSettled_CoveringVersionSettles(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	uri := "file:///tmp/settled/a.go"
	mark := c.MarkDiagnostics(uri)
	mark.Version = 3
	publishDiags(t, serverW, uri, intp(3))
	waitForGen(t, c, uri, 1)

	settled, err := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, map[string]DiagnosticsMark{uri: mark}, 3000)
	if err != nil || !settled {
		t.Fatalf("settled = %v, err = %v; want settled", settled, err)
	}
}

// A server that sends no versions falls back to "published after the mark".
func TestWaitForDiagnosticsSettled_VersionlessPublicationAfterMarkSettles(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	uri := "file:///tmp/settled/a.go"
	publishDiags(t, serverW, uri, nil)
	waitForGen(t, c, uri, 1)
	mark := c.MarkDiagnostics(uri)
	mark.Version = 2
	publishDiags(t, serverW, uri, nil)
	waitForGen(t, c, uri, 2)

	settled, err := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, map[string]DiagnosticsMark{uri: mark}, 3000)
	if err != nil || !settled {
		t.Fatalf("settled = %v, err = %v; want settled", settled, err)
	}
}

func TestWaitForDiagnosticsSettled_ContextCanceled(t *testing.T) {
	c, _, _ := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	settled, err := WaitForDiagnosticsSettled(ctx, c, []string{"file:///tmp/settled/a.go"}, nil, 3000)
	if settled || err == nil {
		t.Fatalf("settled = %v, err = %v; want not settled with ctx error", settled, err)
	}
}

// A server that stays silent when diagnostics do not change (TypeScript,
// rust-analyzer, Dart) leaves a clean edit with no publication; that counts as
// settled once the deadline passes, as it always has.
func TestWaitForDiagnosticsSettled_SilentServerSettlesAtDeadline(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	uri := "file:///tmp/settled/a.go"
	publishDiags(t, serverW, uri, intp(1))
	waitForGen(t, c, uri, 1)
	mark := c.MarkDiagnostics(uri)
	mark.Version = 2

	settled, err := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, map[string]DiagnosticsMark{uri: mark}, 600)
	if err != nil || !settled {
		t.Fatalf("settled = %v, err = %v; want settled for a server that does not republish", settled, err)
	}
}

// A stale publication is not settled even on a server that is silent on
// unchanged diagnostics: it shows the server is behind.
func TestWaitForDiagnosticsSettled_StaleVersionNotSettledOnSilentServer(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	uri := "file:///tmp/settled/a.go"
	mark := c.MarkDiagnostics(uri)
	mark.Version = 3
	publishDiags(t, serverW, uri, intp(2))
	waitForGen(t, c, uri, 1)

	settled, _ := WaitForDiagnosticsSettled(context.Background(), c, []string{uri}, map[string]DiagnosticsMark{uri: mark}, 800)
	if settled {
		t.Fatal("settled = true on a stale-version publication")
	}
}

func TestRepublishesUnchangedDiagnostics_GoplsSeeded(t *testing.T) {
	for _, path := range []string{"gopls", "/usr/local/bin/gopls", `gopls.exe`} {
		if !NewLSPClient(path, nil).RepublishesUnchangedDiagnostics() {
			t.Errorf("%s: not seeded", path)
		}
	}
	if NewLSPClient("typescript-language-server", nil).RepublishesUnchangedDiagnostics() {
		t.Error("typescript-language-server seeded")
	}
}
