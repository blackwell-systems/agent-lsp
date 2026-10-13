package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestHelperFakeLSPServer is not a real test: when AGENT_LSP_FAKE_SERVER=1 the
// test binary re-executes itself as a minimal stdio LSP server (the standard Go
// helper-process pattern), so restart tests can drive a real subprocess without
// depending on an installed language server. It answers every request with a
// result (capabilities for initialize, null otherwise) and exits on "exit".
func TestHelperFakeLSPServer(t *testing.T) {
	if os.Getenv("AGENT_LSP_FAKE_SERVER") != "1" {
		return
	}
	r := NewFrameReader(bufio.NewReader(os.Stdin))
	w := bufio.NewWriter(os.Stdout)
	for {
		raw, err := r.ReadMessage()
		if err != nil {
			os.Exit(0)
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if msg.Method == "exit" {
			os.Exit(0)
		}
		if len(msg.ID) == 0 || msg.Method == "" {
			continue
		}
		var result any
		if msg.Method == "initialize" {
			result = map[string]any{"capabilities": map[string]any{}}
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
		_, _ = w.Write(EncodeMessage(body))
		_ = w.Flush()
	}
}

func newFakeServerClient(t *testing.T) *LSPClient {
	t.Helper()
	t.Setenv("AGENT_LSP_FAKE_SERVER", "1")
	return NewLSPClient(os.Args[0], []string{"-test.run=^TestHelperFakeLSPServer$"})
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

// TestExitMonitor_IgnoresReplacedProcess pins the invariant behind the
// restart_lsp_server flake deterministically: once a newer process has replaced
// a subprocess, the old process's exit must not clear the client's current
// stdin or reject its pending requests. The timing-based restart loop below
// only trips on slow or loaded machines; this test does not depend on timing.
func TestExitMonitor_IgnoresReplacedProcess(t *testing.T) {
	client := newFakeServerClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer func() {
		if client.refCache != nil {
			client.refCache.Close()
		}
	}()

	// Simulate a Restart that already started a newer process.
	var current io.WriteCloser = nopWriteCloser{}
	client.mu.Lock()
	old := client.cmd
	client.cmd = &exec.Cmd{}
	client.stdin = current
	client.mu.Unlock()
	errCh := make(chan error, 1)
	client.pendingMu.Lock()
	client.pending[1<<30] = &pendingRequest{ch: make(chan json.RawMessage, 1), err: errCh}
	client.pendingMu.Unlock()

	// The old process now exits; give its exit monitor time to run.
	_ = old.Process.Kill()
	time.Sleep(500 * time.Millisecond)

	client.mu.Lock()
	gotStdin := client.stdin
	client.mu.Unlock()
	if gotStdin != current {
		t.Error("old process's exit monitor cleared the current process's stdin")
	}
	select {
	case err := <-errCh:
		t.Errorf("old process's exit monitor rejected a current pending request: %v", err)
	default:
	}
}

// TestRestart_NewServerSurvivesOldProcessExit restarts a live subprocess
// repeatedly and requires the client to stay usable after every restart. The
// previous server's exit monitor and read loop must never touch the new
// server's stdin, frame reader, or pending requests; before the fix they wrote
// the shared client fields, so a restart could fail with "LSP process has
// exited" or leave requests unanswered. Run with -race to catch the unsynchronized
// field access even when the timing happens to work out.
func TestRestart_NewServerSurvivesOldProcessExit(t *testing.T) {
	client := newFakeServerClient(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, root); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer client.Shutdown(context.Background())

	for i := range 20 {
		if _, err := client.Restart(ctx, root); err != nil {
			t.Fatalf("restart %d: %v", i, err)
		}
		// Give the previous process's exit monitor time to run, then prove the
		// new server is still reachable.
		time.Sleep(20 * time.Millisecond)
		reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := client.SendRequest(reqCtx, "agentlsp/ping", nil)
		reqCancel()
		if err != nil {
			t.Fatalf("request after restart %d: %v", i, err)
		}
	}
}
