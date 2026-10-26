// readiness_test.go covers the event-driven readiness semaphore (upstream issue #77):
// markReady/AwaitReady primitives, the readiness producers ($/progress drain,
// first publishDiagnostics, first server response after the handshake), the
// truthful workspaceLoaded semantics, and the grace-period boundary fix.
//
// The full-flow tests use the repo's fake-server pattern: the test binary
// re-execs itself as a stdio LSP server selected by AGENT_LSP_FAKE_MODE.
package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// ---------------------------------------------------------------------------
// Unit tests (no I/O)

func TestMarkReady_Idempotent(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	if c.IsReady() {
		t.Fatal("fresh client must not be ready")
	}
	c.markReady()
	c.markReady()
	c.markReady()
	if !c.IsReady() {
		t.Fatal("client must be ready after markReady")
	}
	// A closed channel stays closed; repeated markReady must not panic.
	if err := c.AwaitReady(context.Background(), 0); err != nil {
		t.Fatalf("AwaitReady on ready client: %v", err)
	}
}

func TestAwaitReady_AlreadyReady_ReturnsImmediately(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	c.markReady()
	done := make(chan error, 1)
	go func() { done <- c.AwaitReady(context.Background(), 0) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AwaitReady: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitReady blocked on an already-ready client")
	}
}

func TestAwaitReady_MaxWaitCap(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	start := time.Now()
	if err := c.AwaitReady(context.Background(), 200*time.Millisecond); err == nil {
		t.Fatal("expected error (deadline) when no signal fires")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("AwaitReady exceeded its cap: %v", elapsed)
	}
}

func TestAwaitReady_ContextCancelled(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if err := c.AwaitReady(ctx, 0); err == nil {
		t.Fatal("expected context error")
	}
}

// Producer 2: first publishDiagnostics fires the readiness signal.
func TestProducer_FirstDiagnostics_MarksReady(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	c.MarkInitializedForTest()
	c.handlePublishDiagnostics(json.RawMessage(`{"uri":"file:///a.mq4","diagnostics":[]}`))
	if !c.IsReady() {
		t.Fatal("first publishDiagnostics must mark the client ready")
	}
}

// Producer 1: $/progress drain sets workspaceLoaded truthfully and marks ready.
func TestProducer_ProgressDrain_SetsLoadedAndReady(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	c.handleProgress(json.RawMessage(`{"token":"t1","value":{"kind":"begin","title":"indexing"}}`))
	if !c.hasSeenProgress.Load() {
		t.Fatal("begin must set hasSeenProgress")
	}
	if c.workspaceLoaded.Load() {
		t.Fatal("active token must NOT mark workspace loaded")
	}
	c.handleProgress(json.RawMessage(`{"token":"t1","value":{"kind":"end"}}`))
	if !c.workspaceLoaded.Load() {
		t.Fatal("drained progress must set workspaceLoaded")
	}
	if !c.IsReady() {
		t.Fatal("drained progress must mark the client ready")
	}
}

// Truthful flag: a server that never emits progress must not be claimed loaded,
// and waitForWorkspaceReady must not idle (it used to set loaded unconditionally).
func TestWaitForWorkspaceReady_NoProgress_ReturnsFastAndStaysUnloaded(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	done := make(chan struct{})
	go func() {
		c.waitForWorkspaceReady(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForWorkspaceReady must be a no-op fast path when no progress signal exists")
	}
	if c.workspaceLoaded.Load() {
		t.Fatal("workspaceLoaded must stay false without any progress signal")
	}
}

// ---------------------------------------------------------------------------
// Daemon readiness gate (PR C): every tool surfaces the still-indexing error

func newFakeDaemonClient(t *testing.T, info *DaemonInfo) *LSPClient {
	t.Helper()
	c := NewLSPClient("fake-server", nil)
	c.isDaemon = true
	c.setDaemonInfo(info)
	return c
}

func TestDaemonReadinessError_NonDaemon_Nil(t *testing.T) {
	c := NewLSPClient("fake-server", nil)
	if err := c.daemonReadinessError(); err != nil {
		t.Fatalf("non-daemon client must pass the gate: %v", err)
	}
}

func TestDaemonReadinessError_NotReady_GuidanceError(t *testing.T) {
	c := newFakeDaemonClient(t, &DaemonInfo{
		RootDir: t.TempDir(), LanguageID: "python", StartTime: time.Now().Add(-3 * time.Second),
	})
	err := c.daemonReadinessError()
	if err == nil {
		t.Fatal("expected still-indexing guidance error")
	}
	for _, want := range []string{"still being indexed", "get_daemon_status"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q must contain %q", err.Error(), want)
		}
	}
	if c.IsReady() {
		t.Fatal("not-ready daemon must not mark the client ready")
	}
}

func TestDaemonReadinessError_ReadyFlag_MarksClientReady(t *testing.T) {
	c := newFakeDaemonClient(t, &DaemonInfo{
		RootDir: t.TempDir(), LanguageID: "python", StartTime: time.Now().Add(-time.Minute), Ready: true,
	})
	if err := c.daemonReadinessError(); err != nil {
		t.Fatalf("ready daemon must pass the gate: %v", err)
	}
	if !c.IsReady() || !c.workspaceLoaded.Load() {
		t.Fatal("ready daemon must mark the client ready and loaded")
	}
}

// The broker rewrites daemon.json with ready=true once its indexing wait
// completes; the gate must refresh the in-memory (stale) state from disk.
func TestDaemonReadinessError_RefreshesReadyFromDisk(t *testing.T) {
	root := t.TempDir()
	c := newFakeDaemonClient(t, &DaemonInfo{
		RootDir: root, LanguageID: "python", StartTime: time.Now(), Ready: false,
	})
	if err := c.daemonReadinessError(); err == nil {
		t.Fatal("expected still-indexing error before the broker flags ready")
	}

	// Broker marks ready on disk.
	info := &DaemonInfo{
		RootDir: root, LanguageID: "python", StartTime: time.Now().Add(-time.Minute),
		Ready: true, PID: os.Getpid(),
	}
	if err := WriteDaemonInfo(info); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(DaemonDir(root, "python")) })

	if err := c.daemonReadinessError(); err != nil {
		t.Fatalf("gate must refresh from disk and pass: %v", err)
	}
	if !c.IsReady() || !c.workspaceLoaded.Load() {
		t.Fatal("client must become ready after the disk refresh")
	}
}

// All read tools go through ensureWorkspaceReady, so the guidance error must
// propagate from it.
func TestEnsureWorkspaceReady_DaemonNotReady_PropagatesError(t *testing.T) {
	c := newFakeDaemonClient(t, &DaemonInfo{
		RootDir: t.TempDir(), LanguageID: "typescript", StartTime: time.Now(),
	})
	if err := c.ensureWorkspaceReady(context.Background()); err == nil {
		t.Fatal("expected still-indexing error from ensureWorkspaceReady")
	}
}

// Concurrency: daemonReadinessError is called by every read tool, so multiple
// MCP tool calls can run it at once while the broker flags the daemon ready on
// disk. This exercises the daemonInfo refresh path under -race.
func TestDaemonReadinessError_ConcurrentRefreshRace(t *testing.T) {
	root := t.TempDir()
	c := newFakeDaemonClient(t, &DaemonInfo{
		RootDir: root, LanguageID: "python", StartTime: time.Now().Add(-time.Minute),
		Ready: false, PID: os.Getpid(),
	})
	t.Cleanup(func() { os.RemoveAll(DaemonDir(root, "python")) })

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Writer: flip ready=true/false on disk continuously.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ready := false
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = WriteDaemonInfo(&DaemonInfo{
				RootDir: root, LanguageID: "python",
				StartTime: time.Now().Add(-time.Minute), Ready: ready, PID: os.Getpid(),
			})
			ready = !ready
		}
	}()

	// Readers: hammer the concurrent refresh path.
	const readers = 8
	var rg sync.WaitGroup
	for i := 0; i < readers; i++ {
		rg.Add(1)
		go func() {
			defer rg.Done()
			for j := 0; j < 200; j++ {
				_ = c.daemonReadinessError()
			}
		}()
	}
	rg.Wait()
	close(stop)
	wg.Wait()
}

// Regression for the redundant wait after daemon adoption: once
// daemonReadinessError adopts the broker's readiness it sets workspaceLoaded,
// so ensureWorkspaceReady must return immediately instead of falling into the
// progress wait and blocking up to 60s on an active $/progress token that the
// broker already told us to ignore.
func TestEnsureWorkspaceReady_DaemonAdoption_SkipsProgressWait(t *testing.T) {
	root := t.TempDir()
	c := newFakeDaemonClient(t, &DaemonInfo{
		RootDir: root, LanguageID: "python", StartTime: time.Now().Add(-time.Minute),
		Ready: true, PID: os.Getpid(),
	})

	// Inject an active $/progress begin token so the pre-fix code would block in
	// WaitForWorkspaceReadyTimeout(60s) after adoption. handleProgress is the
	// same path the reader goroutine uses, so this is safe to call directly here.
	c.handleProgress(json.RawMessage(`{"token":"t1","value":{"kind":"begin","title":"indexing"}}`))
	if !c.HasActiveProgress() {
		t.Fatal("test setup: expected an active progress token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- c.ensureWorkspaceReady(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ensureWorkspaceReady after daemon adoption: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ensureWorkspaceReady blocked after daemon adoption; the gate must skip the progress wait")
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("ensureWorkspaceReady took %v; expected a fast return (<1s)", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Helper-process fake server (stdio LSP, re-exec of the test binary)

// TestReadinessFakeServer is never run as a test: when AGENT_LSP_FAKE_MODE is
// set, the test binary acts as a minimal stdio LSP server for the parent test.
func TestReadinessFakeServer(t *testing.T) {
	mode := os.Getenv("AGENT_LSP_FAKE_MODE")
	if mode == "" {
		t.Skip("helper process mode only")
	}
	writeMsg := func(v any) {
		body, _ := json.Marshal(v)
		fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	fr := NewFrameReader(os.Stdin)

	var initTime time.Time
	pushed := false
	for {
		raw, err := fr.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.ID != nil {
			if msg.Method == "initialize" {
				writeMsg(map[string]any{
					"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"capabilities": map[string]any{}},
				})
				continue
			}
			// Simulate a lazy-init window: drop (never answer) requests for
			// 2s after the handshake, then answer everything immediately.
			if mode == "drop2s" && !initTime.IsZero() && time.Since(initTime) < 2*time.Second {
				continue
			}
			writeMsg(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": nil})
			continue
		}
		switch {
		case msg.Method == "initialized" && !initTime.IsZero():
			// already handled
		case msg.Method == "initialized":
			initTime = time.Now()
			if mode == "progress" && !pushed {
				pushed = true
				writeMsg(map[string]any{"jsonrpc": "2.0", "method": "$/progress", "params": map[string]any{
					"token": "t1", "value": map[string]any{"kind": "begin", "title": "indexing"}}})
				go func() {
					time.Sleep(600 * time.Millisecond)
					writeMsg(map[string]any{"jsonrpc": "2.0", "method": "$/progress", "params": map[string]any{
						"token": "t1", "value": map[string]any{"kind": "end"}}})
				}()
			}
		}
	}
}

func newReadinessServerClient(t *testing.T, mode string) *LSPClient {
	t.Helper()
	t.Setenv("AGENT_LSP_FAKE_MODE", mode)
	t.Setenv("AGENT_LSP_DISABLE_WATCHER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := NewLSPClient(executable, []string{"-test.run=^TestReadinessFakeServer$"})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Shutdown(ctx)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Initialize(ctx, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return c
}

// Producer 3 end-to-end: a server that silently drops requests during its
// lazy-init window must NOT leave ensureWorkspaceReady waiting for the 120s
// safety cap — the liveness probe gets answered as soon as the window ends
// and its response fires the readiness event.
func TestEnsureWorkspaceReady_SilentWindow_UnblocksOnFirstResponse(t *testing.T) {
	c := newReadinessServerClient(t, "drop2s")

	start := time.Now()
	c.ensureWorkspaceReady(context.Background())
	elapsed := time.Since(start)

	// The drop window lasts 2s from the handshake; probes are sent every 5s
	// with an immediate first probe. The first probe sent after the window
	// closes must unblock us — well under the 120s safety cap.
	if elapsed >= 120*time.Second {
		t.Fatalf("ensureWorkspaceReady hit the safety cap (%v); event did not fire", elapsed)
	}
	if !c.IsReady() {
		t.Fatal("client must be ready after the first response")
	}
}

// Grace boundary: with a 60s wait (previously excluded by `timeout > 60s`)
// a late-emitting begin token must extend the wait instead of returning
// immediately. The server pushes begin right after the handshake and end
// 600ms later.
// TestEnsureWorkspaceReady_NoProgressServer_ImmediateHotPath pins review
// direction 1: on a server that never emits $/progress, the gate must stay
// off the hot path (0-2 ms, like main) AND latch the truthful readiness
// state — workspaceLoaded true and the readiness event fired, so
// IsWorkspaceLoaded and workspace_ready work on every server.
func TestEnsureWorkspaceReady_NoProgressServer_ImmediateHotPath(t *testing.T) {
	c := newReadinessServerClient(t, "silent") // answers but never signals readiness
	start := time.Now()
	if err := c.ensureWorkspaceReady(context.Background()); err != nil {
		t.Fatalf("ensureWorkspaceReady: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("gate took %v on a progress-less server; it must not wait on the hot path", elapsed)
	}
	if !c.workspaceLoaded.Load() {
		t.Fatal("workspaceLoaded must latch true for a progress-less server")
	}
	if !c.IsReady() {
		t.Fatal("the readiness event must fire for a progress-less server")
	}
	if !c.IsWorkspaceLoaded() {
		t.Fatal("IsWorkspaceLoaded must report true after the gate")
	}
}

// TestEnsureWorkspaceReady_TimedOutWaitSurfacedAndLatched pins review
// directions 2 and 3: a readiness wait that runs to its cap is surfaced to
// the agent as an error (never discarded with _ =) and latched, so later
// calls proceed instead of paying the same wait again.
func TestEnsureWorkspaceReady_TimedOutWaitSurfacedAndLatched(t *testing.T) {
	c := NewLSPClient("fake-server", nil) // never started: no signals, no probes
	old := readinessWaitCap
	readinessWaitCap = 80 * time.Millisecond
	t.Cleanup(func() { readinessWaitCap = old })

	// Simulate a progress server whose token was seen and never drained:
	// the progress wait runs, leaves the workspace unloaded, and the
	// fallback wait then runs to its (shrunk) cap.
	c.hasSeenProgress.Store(true)
	c.progressMu.Lock()
	c.progressTokens["stuck-token"] = struct{}{}
	c.progressMu.Unlock()
	oldWait := workspaceProgressWait
	workspaceProgressWait = 20 * time.Millisecond
	t.Cleanup(func() { workspaceProgressWait = oldWait })
	start := time.Now()
	err := c.ensureWorkspaceReady(context.Background())
	if err == nil {
		t.Fatal("the capped readiness wait must surface an error to the agent")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Errorf("the surfaced error must name the timeout, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("first wait took %v; it must end at the (shrunk) cap", elapsed)
	}
	if !c.readinessWaitExhausted.Load() {
		t.Fatal("the timed-out wait must latch readinessWaitExhausted")
	}
	// Later calls never re-pay it.
	start = time.Now()
	if err := c.ensureWorkspaceReady(context.Background()); err != nil {
		t.Fatalf("later calls must proceed without waiting: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("later calls re-paid the wait (%v)", elapsed)
	}
	// A caller whose OWN context ends gets its error verbatim and no latch:
	// same stuck-token setup, but the ctx is cancelled up front.
	c2 := NewLSPClient("fake-server", nil)
	c2.hasSeenProgress.Store(true)
	c2.progressMu.Lock()
	c2.progressTokens["stuck-token"] = struct{}{}
	c2.progressMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c2.ensureWorkspaceReady(ctx); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("a cancelled caller must see its own error, got: %v", err)
	}
	if c2.readinessWaitExhausted.Load() {
		t.Error("a caller-caused cancellation must not latch the readiness verdict")
	}
}

// TestAwaitReady_ProbeServesLaterWaiters pins the shared-probe fix: the
// probe loop's lifetime follows the waiter count, not one caller's context —
// when the first waiter's ctx ends, the second waiter must still get probes
// and finish as soon as the server answers.
func TestAwaitReady_ProbeServesLaterWaiters(t *testing.T) {
	c := newReadinessServerClient(t, "silent") // answers probes, never signals passively
	c.probeIntervalNs.Store(int64(100 * time.Millisecond))
	c.probeTimeoutNs.Store(int64(500 * time.Millisecond))

	// A: waits briefly with a short cap, leaving the loop (its ctx dies).
	ctxA, cancelA := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelA()
	_ = c.AwaitReady(ctxA, 40*time.Millisecond)
	// A's cancellation must not stop the probe loop for B.
	done := make(chan error, 1)
	go func() { done <- c.AwaitReady(context.Background(), 5*time.Second) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("B must be unblocked by the probe's answer: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("B waited without probes after A left — the probe loop died with A's context")
	}
}

// TestRestartResetsReadinessState pins the CodeRabbit finding: the readiness
// state belongs to the outgoing server session. After Restart the client must
// be unready (fresh process, no signals yet) — not treated as ready at once.
func TestRestartResetsReadinessState(t *testing.T) {
	c := newReadinessServerClient(t, "progress")
	c.markReady()
	c.workspaceLoaded.Store(true)
	c.hasSeenProgress.Store(true)
	c.readinessWaitExhausted.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := c.Restart(ctx, c.RootDir()); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if c.IsReady() {
		t.Error("after a restart the client must be unready until the new process signals")
	}
	if c.workspaceLoaded.Load() || c.hasSeenProgress.Load() || c.readinessWaitExhausted.Load() {
		t.Error("readiness state must reset with the process")
	}
}

func TestWaitForWorkspaceReadyTimeout_GraceAppliesBeyond60s(t *testing.T) {
	c := newReadinessServerClient(t, "progress")

	// The 10 s grace for a late begin token applies only to callers requesting
	// MORE than 60 s: at exactly 60 s (the gate's own wait) the grace no
	// longer applies, so callers passing 60 do not pay it. (PR #69 maintainer
	// review, Minor)
	start := time.Now()
	c.WaitForWorkspaceReadyTimeout(context.Background(), 61*time.Second)
	elapsed := time.Since(start)
	if elapsed < 500*time.Millisecond {
		t.Fatalf("returned after %v; grace should have waited for the late begin token", elapsed)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("waited %v; should have returned once progress drained", elapsed)
	}
	if !c.workspaceLoaded.Load() {
		t.Fatal("workspaceLoaded must be true after progress drained")
	}

	// At exactly 60 s no grace: the wait returns at once when no tokens are
	// active and none were ever seen.
	c2 := newReadinessServerClient(t, "progress")
	c2.progressMu.Lock()
	c2.progressTokens = make(map[any]struct{})
	c2.hasSeenProgress.Store(false)
	c2.progressMu.Unlock()
	start = time.Now()
	c2.WaitForWorkspaceReadyTimeout(context.Background(), 60*time.Second)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("exactly-60s wait took %v; the grace period must not apply at 60s", elapsed)
	}
}
