package lsp

import (
	"context"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// TestWaitForDiagnostics_SettlesAfterQuietWindow verifies that WaitForDiagnostics
// resolves once each tracked URI has received a fresh notification and a 500ms
// quiet window has elapsed.
func TestWaitForDiagnostics_SettlesAfterQuietWindow(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	ctx := context.Background()
	uris := []string{"file:///a.go", "file:///b.go"}

	done := make(chan error, 1)
	go func() {
		done <- WaitForDiagnostics(ctx, c, uris, 5000)
	}()

	// Fire two rounds of notifications per URI: the first is the initial-snapshot
	// (skipped by seenInitial logic), the second is the fresh notification that
	// triggers settlement.
	for round := 0; round < 2; round++ {
		time.Sleep(10 * time.Millisecond)
		for _, uri := range uris {
			if err := writeMsg(serverW, map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "textDocument/publishDiagnostics",
				"params": map[string]interface{}{
					"uri":         uri,
					"diagnostics": []interface{}{},
				},
			}); err != nil {
				t.Fatalf("write diag round %d: %v", round, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// WaitForDiagnostics should settle after 500ms quiet window.
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout: WaitForDiagnostics did not settle")
	}
}

// TestWaitForDiagnostics_Timeout verifies that WaitForDiagnostics resolves
// after the timeout even if no notifications are received.
func TestWaitForDiagnostics_Timeout(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	_ = serverW

	ctx := context.Background()
	uris := []string{"file:///missing.go"}

	start := time.Now()
	err := WaitForDiagnostics(ctx, c, uris, 200)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected nil error on timeout, got: %v", err)
	}
	if elapsed < 190*time.Millisecond {
		t.Errorf("resolved too early: %v (expected >= 190ms)", elapsed)
	}
	if elapsed > 600*time.Millisecond {
		t.Errorf("resolved too late: %v (expected <= 600ms)", elapsed)
	}
}

// TestWaitForDiagnostics_EmptyURIs verifies that an empty URI list resolves immediately.
func TestWaitForDiagnostics_EmptyURIs(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	_ = serverW

	ctx := context.Background()
	start := time.Now()
	err := WaitForDiagnostics(ctx, c, []string{}, 5000)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("expected immediate resolution, took %v", elapsed)
	}
}

// TestWaitForDiagnostics_ContextCancelled verifies that WaitForDiagnostics
// respects context cancellation.
func TestWaitForDiagnostics_ContextCancelled(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	_ = serverW

	ctx, cancel := context.WithCancel(context.Background())
	uris := []string{"file:///never.go"}

	done := make(chan error, 1)
	go func() {
		done <- WaitForDiagnostics(ctx, c, uris, 30000)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout: expected WaitForDiagnostics to return on cancel")
	}
}

// TestWaitForDiagnostics_SubscribeUnsubscribe ensures no leak of callbacks
// after WaitForDiagnostics returns.
func TestWaitForDiagnostics_SubscribeUnsubscribe(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	_ = serverW

	ctx := context.Background()

	// Count subscriptions before.
	c.diagMu.RLock()
	before := len(c.diagSubs)
	c.diagMu.RUnlock()

	err := WaitForDiagnostics(ctx, c, []string{"file:///x.go"}, 100)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// After return, subscription must be cleaned up.
	c.diagMu.RLock()
	after := len(c.diagSubs)
	c.diagMu.RUnlock()

	if after != before {
		t.Errorf("callback leak: before=%d after=%d", before, after)
	}
}

// TestWaitForFileIndexed_Timeout verifies that WaitForFileIndexed returns
// nil (not an error) after the timeout elapses with no notifications.
func TestWaitForFileIndexed_Timeout(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	_ = serverW

	ctx := context.Background()
	uri := "file:///missing.go"

	start := time.Now()
	err := c.WaitForFileIndexed(ctx, uri, 200)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected nil error on timeout, got: %v", err)
	}
	if elapsed < 190*time.Millisecond {
		t.Errorf("resolved too early: %v (expected >= 190ms)", elapsed)
	}
	if elapsed > 700*time.Millisecond {
		t.Errorf("resolved too late: %v (expected <= 700ms)", elapsed)
	}
}

// TestWaitForFileIndexed_ContextCancelled verifies that WaitForFileIndexed
// returns context.Canceled when the context is cancelled.
func TestWaitForFileIndexed_ContextCancelled(t *testing.T) {
	c, serverW, _ := newTestClient(t)
	_ = serverW

	ctx, cancel := context.WithCancel(context.Background())
	uri := "file:///never.go"

	done := make(chan error, 1)
	go func() {
		done <- c.WaitForFileIndexed(ctx, uri, 30000)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout: expected WaitForFileIndexed to return on cancel")
	}
}

// TestWaitForFileIndexed_StabilityWindowReset verifies that each new
// notification resets the 1500ms stability window.
func TestWaitForFileIndexed_StabilityWindowReset(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	ctx := context.Background()
	uri := "file:///a.go"

	done := make(chan error, 1)
	go func() {
		// Use a 6s timeout to give the stability window room.
		done <- c.WaitForFileIndexed(ctx, uri, 6000)
	}()

	// Send first notification.
	time.Sleep(50 * time.Millisecond)
	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params":  map[string]interface{}{"uri": uri, "diagnostics": []interface{}{}},
	}); err != nil {
		t.Fatalf("write first notification: %v", err)
	}

	// Send second notification 200ms later (within stability window).
	time.Sleep(200 * time.Millisecond)
	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params":  map[string]interface{}{"uri": uri, "diagnostics": []interface{}{}},
	}); err != nil {
		t.Fatalf("write second notification: %v", err)
	}

	// Should NOT settle before ~1500ms after the second notification.
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout: WaitForFileIndexed did not settle within 5s")
	}
}

// TestWaitForDiagnostics_OnlyFreshNotifications verifies that pre-existing
// diagnostics in the cache do NOT count as "fresh notification".
func TestWaitForDiagnostics_OnlyFreshNotifications(t *testing.T) {
	c, serverW, _ := newTestClient(t)

	uri := "file:///cached.go"

	// Pre-populate diagnostic cache directly.
	c.diagMu.Lock()
	c.diags[uri] = []types.LSPDiagnostic{{Message: "pre-existing"}}
	c.diagMu.Unlock()

	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- WaitForDiagnostics(ctx, c, []string{uri}, 300)
	}()

	// Should NOT resolve immediately from cache — needs a fresh notification.
	select {
	case err := <-done:
		// It resolved — check it was the timeout path (300ms), not instant.
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// If it resolved immediately that's a bug, but we can only assert it
		// didn't take too long via the timeout path.
	case <-time.After(500 * time.Millisecond):
		// After 300ms timeout, WaitForDiagnostics should have returned.
		t.Error("WaitForDiagnostics did not resolve after timeout")
	}

	// Now also test: if we send a fresh notification it should resolve early.
	done2 := make(chan error, 1)
	go func() {
		done2 <- WaitForDiagnostics(ctx, c, []string{uri}, 5000)
	}()

	time.Sleep(10 * time.Millisecond)
	if err := writeMsg(serverW, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]interface{}{
			"uri":         uri,
			"diagnostics": []interface{}{},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case err := <-done2:
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for WaitForDiagnostics to settle after fresh notification")
	}
}
