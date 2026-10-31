package lsp

import (
	"context"
	"io"
	"testing"
	"time"
)

func sendProgress(t *testing.T, w io.Writer, token, kind string) {
	t.Helper()
	if err := writeMsg(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "$/progress",
		"params":  map[string]any{"token": token, "value": map[string]any{"kind": kind, "title": "Indexing"}},
	}); err != nil {
		t.Fatalf("writeMsg: %v", err)
	}
}

// waitFor polls cond until it holds or 2s pass.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func shortReadinessWait(t *testing.T, d time.Duration) {
	t.Helper()
	old := defaultReadinessWait
	defaultReadinessWait = d
	t.Cleanup(func() { defaultReadinessWait = old })
}

func timed(f func()) time.Duration {
	start := time.Now()
	f()
	return time.Since(start)
}

// A progress token that never ends used to cost every find_references the full
// readiness wait. It is now waited on once; a fresh token is still waited on.
func TestReadinessWait_StuckTokenWaitedOnce(t *testing.T) {
	shortReadinessWait(t, 300*time.Millisecond)
	c, serverW, _ := newTestClient(t)
	ctx := context.Background()

	sendProgress(t, serverW, "stuck", "begin")
	waitFor(t, "begin", c.HasActiveProgress)

	if d := timed(func() { c.waitForWorkspaceReady(ctx) }); d < 250*time.Millisecond {
		t.Fatalf("first wait took %v, want the full wait", d)
	}
	if d := timed(func() { c.waitForWorkspaceReady(ctx) }); d > 100*time.Millisecond {
		t.Fatalf("second wait took %v, want an immediate return for a stuck token", d)
	}

	// Fresh work is still waited on.
	sendProgress(t, serverW, "fresh", "begin")
	waitFor(t, "fresh begin", func() bool {
		c.progressMu.Lock()
		defer c.progressMu.Unlock()
		return c.pendingProgressLocked(true) == 1
	})
	go func() {
		time.Sleep(100 * time.Millisecond)
		sendProgress(t, serverW, "fresh", "end")
	}()
	if d := timed(func() { c.waitForWorkspaceReady(ctx) }); d < 80*time.Millisecond || d > 250*time.Millisecond {
		t.Fatalf("wait on a fresh token took %v, want it to end with the token (~100ms)", d)
	}

	// An explicit long wait (jdtls import, ready_timeout_seconds) still waits on
	// the stuck token rather than skipping it.
	if d := timed(func() { c.WaitForWorkspaceReadyTimeout(ctx, 200*time.Millisecond) }); d < 150*time.Millisecond {
		t.Fatalf("explicit wait took %v, want it to wait on every active token", d)
	}
	// The stuck token still counts as background work for confidence.
	if !c.HasActiveProgress() {
		t.Fatal("HasActiveProgress = false with a stuck token")
	}
}

// A server that begins its initial load after the first readiness wait (as
// rust-analyzer and jdtls do) must not leave the workspace marked loaded.
func TestReadiness_FirstProgressClearsEarlyLoaded(t *testing.T) {
	shortReadinessWait(t, 2*time.Second)
	c, serverW, _ := newTestClient(t)
	ctx := context.Background()

	c.waitForWorkspaceReady(ctx) // no progress yet: returns at once, marks loaded
	if !c.IsWorkspaceLoaded() {
		t.Fatal("expected loaded after a wait with no progress")
	}

	sendProgress(t, serverW, "index", "begin")
	waitFor(t, "loaded cleared by the first progress", func() bool { return !c.IsWorkspaceLoaded() })

	go func() {
		time.Sleep(100 * time.Millisecond)
		sendProgress(t, serverW, "index", "end")
	}()
	c.ensureWorkspaceReady(ctx)
	if !c.IsWorkspaceLoaded() {
		t.Fatal("expected loaded once the initial load ended")
	}

	// Later progress (cargo check, a gopls command) does not clear it again.
	sendProgress(t, serverW, "check", "begin")
	waitFor(t, "second begin", c.HasActiveProgress)
	if !c.IsWorkspaceLoaded() {
		t.Fatal("later progress cleared loaded")
	}
}

// After restart_lsp_server the fresh server has loaded nothing; indexed must
// not stay true from the previous process.
func TestRestart_ResetsWorkspaceLoaded(t *testing.T) {
	client := newFakeServerClient(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, root); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer client.Shutdown(context.Background())

	client.workspaceLoaded.Store(true)
	client.progressMu.Lock()
	client.seenProgress = true
	client.staleProgress = map[any]struct{}{"old": {}}
	client.progressMu.Unlock()

	if _, err := client.Restart(ctx, root); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if client.IsWorkspaceLoaded() {
		t.Error("workspace still marked loaded after restart")
	}
	client.progressMu.Lock()
	defer client.progressMu.Unlock()
	if client.seenProgress || len(client.staleProgress) != 0 {
		t.Errorf("progress state not reset: seen=%v stale=%v", client.seenProgress, client.staleProgress)
	}
}
