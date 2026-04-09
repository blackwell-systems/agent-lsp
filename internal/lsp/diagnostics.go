package lsp

import (
	"context"
	"sync"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// WaitForDiagnostics waits for diagnostic stabilisation for all uris.
// It skips the initial cached-replay notification per URI (matching the
// TypeScript sawInitialSnapshot logic), requires one fresh notification
// per URI after that, then waits for a 500ms quiet window.
// Resolves on timeout without error.
func WaitForDiagnostics(ctx context.Context, client *LSPClient, uris []string, timeoutMs int) error {
	if len(uris) == 0 {
		return nil
	}

	var mu sync.Mutex

	// Track which URIs have received at least one fresh notification.
	received := make(map[string]bool, len(uris))
	for _, uri := range uris {
		received[uri] = false
	}

	// seenInitial tracks whether the initial cached-replay notification has
	// been skipped per URI, matching the TypeScript sawInitialSnapshot logic.
	seenInitial := make(map[string]bool, len(uris))

	var lastEvent time.Time
	lastEvent = time.Now()

	allReceived := func() bool {
		for _, ok := range received {
			if !ok {
				return false
			}
		}
		return true
	}

	notify := make(chan struct{}, len(uris)+1)

	cb := types.DiagnosticUpdateCallback(func(uri string, _ []types.LSPDiagnostic) {
		mu.Lock()
		if _, tracked := received[uri]; tracked {
			if !seenInitial[uri] {
				// Skip the first callback per URI: it is the cached-replay
				// snapshot from SubscribeToDiagnostics, not a fresh notification.
				seenInitial[uri] = true
				mu.Unlock()
				return
			}
			received[uri] = true
		}
		lastEvent = time.Now()
		mu.Unlock()
		select {
		case notify <- struct{}{}:
		default:
		}
	})

	client.SubscribeToDiagnostics(cb)
	defer client.UnsubscribeFromDiagnostics(cb)

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	quietWindow := 500 * time.Millisecond

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil
			}
			mu.Lock()
			gotAll := allReceived()
			quiet := time.Since(lastEvent) >= quietWindow
			mu.Unlock()
			if gotAll && quiet {
				return nil
			}
		case <-notify:
			// Notification received; let ticker check quiet window.
		}
	}
}
