// warmup.go implements multi-signal workspace readiness detection.
//
// Problem: not all language servers emit $/progress tokens. Pyright, jedi-language-server,
// and several others provide no signal that indexing is complete. The existing
// waitForWorkspaceReady (which watches $/progress) returns immediately for these
// servers, causing subsequent reference queries to time out because the workspace
// isn't loaded yet.
//
// Solution: a readiness gate that uses multiple signals in priority order:
//
//  1. $/progress tokens (gopls, rust-analyzer, jdtls) — existing path, fast
//  2. Diagnostic arrival — most servers emit publishDiagnostics once analysis is done
//  3. Hover canary — if hover on an opened file returns type info, the workspace is warm
//  4. Adaptive first-query timeout — first reference query gets 5x the normal timeout
//
// The gate runs transparently inside GetReferences on first call. It doesn't block
// start_lsp (which should return fast) but does block the first expensive operation.
package lsp

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// warmupState tracks the multi-signal readiness of the workspace.
type warmupState struct {
	// completed is set to true once any warmup signal confirms readiness.
	completed atomic.Bool

	// mu protects the warmup flow from concurrent execution.
	mu sync.Mutex

	// diagnosticReceived is set when any publishDiagnostics arrives.
	diagnosticReceived atomic.Bool

	// firstRefTimeout is the extended timeout for the first reference query
	// on a cold workspace. Subsequent queries use the normal 120s timeout.
	firstRefTimeout time.Duration
}

func newWarmupState() *warmupState {
	return &warmupState{
		firstRefTimeout: 300 * time.Second, // 5 minutes for first reference query
	}
}

// EnsureReady runs the readiness gate if the workspace hasn't been confirmed ready.
// This is called by GetReferences before issuing the actual LSP request.
// It's a no-op if the workspace is already warm (either via $/progress or prior warmup).
//
// The gate proceeds through signals in order until one confirms readiness:
//  1. If workspaceLoaded is already true ($/progress completed), return immediately
//  2. Wait for first diagnostic notification (up to 30s)
//  3. Issue a hover canary on the opened file (up to 10s)
//  4. If neither, proceed anyway (the reference query will use extended timeout)
func (w *warmupState) EnsureReady(ctx context.Context, client *LSPClient, fileURI string) {
	if w.completed.Load() {
		return
	}
	if client.workspaceLoaded.Load() {
		w.completed.Store(true)
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Double-check after acquiring lock.
	if w.completed.Load() || client.workspaceLoaded.Load() {
		w.completed.Store(true)
		return
	}

	logging.Log(logging.LevelDebug, "warmup: starting readiness gate (no $/progress signal)")

	// Signal 2: Wait for first diagnostic notification.
	if w.waitForDiagnostic(ctx, client, 30*time.Second) {
		logging.Log(logging.LevelDebug, "warmup: ready (diagnostic received)")
		w.completed.Store(true)
		return
	}

	// Signal 3: Hover canary. If hover returns non-empty, the file is analyzed.
	if w.hoverCanary(ctx, client, fileURI) {
		logging.Log(logging.LevelDebug, "warmup: ready (hover returned type info)")
		w.completed.Store(true)
		return
	}

	// No signal confirmed readiness. The first reference query will use extended timeout.
	logging.Log(logging.LevelDebug, "warmup: no readiness signal; proceeding with extended timeout")
}

// waitForDiagnostic waits until any publishDiagnostics notification arrives
// from the server (for any file), or the timeout elapses.
func (w *warmupState) waitForDiagnostic(ctx context.Context, client *LSPClient, timeout time.Duration) bool {
	if w.diagnosticReceived.Load() {
		return true
	}

	done := make(chan struct{})
	cb := types.DiagnosticUpdateCallback(func(uri string, diags []types.LSPDiagnostic) {
		w.diagnosticReceived.Store(true)
		select {
		case done <- struct{}{}:
		default:
		}
	})

	client.SubscribeToDiagnostics(cb)
	defer client.UnsubscribeFromDiagnostics(cb)

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	case <-ctx.Done():
		return false
	}
}

// hoverCanary issues a hover request on line 1, column 0 of the given file.
// If the server returns any non-empty hover content, the file is analyzed.
func (w *warmupState) hoverCanary(ctx context.Context, client *LSPClient, fileURI string) bool {
	if !client.hasCapability("hoverProvider") {
		return false
	}

	canaryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := client.sendRequest(canaryCtx, "textDocument/hover", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": fileURI},
		"position":     map[string]interface{}{"line": 0, "character": 0},
	})
	if err != nil {
		return false
	}

	// Any non-null result means the server has analyzed the file.
	return result != nil && string(result) != "null"
}

// firstRefDone tracks whether the first successful reference query has completed.
// Unlike completed (which tracks readiness signals), this only becomes true after
// an actual reference query returns results. Diagnostics and hover confirm the
// server is alive, but only a successful reference query confirms the workspace
// is fully indexed for cross-file resolution.
var firstRefDone atomic.Bool

// FirstRefTimeout returns the extended timeout for the first reference query.
// Returns 0 (use normal timeout) when:
//   - A reference query has already succeeded (workspace confirmed warm)
//   - The warmup gate completed via $/progress (server confirmed ready)
//
// Only returns the extended timeout when neither signal has confirmed readiness,
// which happens with servers like pyright that don't emit $/progress tokens.
func (w *warmupState) FirstRefTimeout() time.Duration {
	if firstRefDone.Load() {
		return 0
	}
	if w.completed.Load() {
		return 0 // $/progress or diagnostics confirmed readiness
	}
	return w.firstRefTimeout
}

// MarkReady marks the workspace as fully ready (after a successful reference query).
func (w *warmupState) MarkReady() {
	w.completed.Store(true)
	firstRefDone.Store(true)
}

// NotifyDiagnostic is called when any publishDiagnostics notification arrives.
// This feeds into the warmup gate's diagnostic signal.
func (w *warmupState) NotifyDiagnostic() {
	w.diagnosticReceived.Store(true)
}

// GetReferencesWithWarmup wraps the reference query with the readiness gate and
// adaptive timeout. This is the primary entry point for reference queries that
// need to handle cold workspaces gracefully.
//
// Behavior:
//  1. Run EnsureReady gate (waits for signals, up to ~40s)
//  2. Issue the reference query with extended timeout if workspace isn't confirmed ready
//  3. On success, mark workspace as ready for future queries
//  4. On timeout, return empty results with a guidance message (not an error)
func GetReferencesWithWarmup(ctx context.Context, client *LSPClient, uri string, pos types.Position, includeDecl bool) ([]types.Location, error) {
	if !client.hasCapability("referencesProvider") {
		return []types.Location{}, nil
	}

	// Fast path: if workspace is already loaded via $/progress (gopls, rust-analyzer),
	// skip the entire warmup gate and use the original direct path. This ensures
	// languages with proper $/progress support have zero overhead from the warmup system.
	if client.workspaceLoaded.Load() {
		_ = client.WaitForFileIndexed(ctx, uri, 15000)
		locs, err := client.getReferencesInternal(ctx, uri, pos, includeDecl)
		if err == nil {
			client.warmup.MarkReady()
		}
		return locs, err
	}

	// Also fast-path if warmup already completed (e.g. from a prior successful query).
	if client.warmup.completed.Load() {
		client.waitForWorkspaceReady(ctx)
		_ = client.WaitForFileIndexed(ctx, uri, 15000)
		return client.getReferencesInternal(ctx, uri, pos, includeDecl)
	}

	// For daemon clients, check readiness without blocking on standard waits.
	if client.isDaemon {
		// Refresh readiness from disk.
		if client.daemonInfo != nil && !client.daemonInfo.Ready {
			info, _ := RefreshDaemonInfo(client.daemonInfo.RootDir, client.daemonInfo.LanguageID)
			if info != nil {
				client.daemonInfo = info
				if info.Ready {
					client.warmup.MarkReady()
				}
			}
		}
		if client.daemonInfo != nil && !client.daemonInfo.Ready {
			elapsed := time.Since(client.daemonInfo.StartTime).Round(time.Second)
			return nil, fmt.Errorf("workspace is still being indexed by the daemon (started %s ago). References will be available once indexing completes. Other tools (hover, diagnostics, symbols) work immediately. Check status with get_daemon_status", elapsed)
		}
	} else {
		// Direct mode: run standard waits.
		client.waitForWorkspaceReady(ctx)
		_ = client.WaitForFileIndexed(ctx, uri, 15000)
	}

	// Run multi-signal warmup gate.
	client.warmup.EnsureReady(ctx, client, uri)

	// Determine timeout: extended for first cold query, normal for subsequent.
	extraTimeout := client.warmup.FirstRefTimeout()
	if extraTimeout > 0 {
		logging.Log(logging.LevelDebug, fmt.Sprintf("warmup: first reference query, using extended timeout %s", extraTimeout))
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, extraTimeout)
		defer cancel()
	} else {
		logging.Log(logging.LevelDebug, "warmup: workspace warm, using normal timeout")
	}

	logging.Log(logging.LevelDebug, fmt.Sprintf("warmup: issuing textDocument/references for %s", uri))
	locs, err := client.getReferencesInternal(ctx, uri, pos, includeDecl)
	if err != nil {
		// Check if this was a timeout and provide guidance.
		if ctx.Err() == context.DeadlineExceeded {
			logging.Log(logging.LevelDebug, fmt.Sprintf("warmup: reference query timed out after %s for %s", extraTimeout, uri))
			return nil, fmt.Errorf("reference query timed out after %s: workspace may still be indexing. For large Python/TypeScript repos, try narrowing scope with the 'scope' parameter on start_lsp, or wait longer for initial indexing", extraTimeout)
		}
		return nil, err
	}

	// Success: mark workspace as ready for future queries.
	logging.Log(logging.LevelDebug, fmt.Sprintf("warmup: reference query succeeded (%d results), marking workspace warm", len(locs)))
	client.warmup.MarkReady()
	return locs, nil
}
