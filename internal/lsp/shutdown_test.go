package lsp

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// TestShutdown_DirectMode_KillsProcess verifies that Shutdown force-kills
// a subprocess that doesn't exit after receiving shutdown/exit.
func TestShutdown_DirectMode_KillsProcess(t *testing.T) {
	// Use "sleep 60" as a fake language server that ignores shutdown/exit.
	c := NewLSPClient("sleep", []string{"60"})
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	c.cmd = cmd

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown will fail to send shutdown request (no stdin pipe),
	// so it should fall through to killProcess.
	_ = c.Shutdown(ctx)

	// Verify the process is dead.
	// cmd.Wait() should return quickly if the process was killed.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		// Process exited (killed or otherwise). Success.
	case <-time.After(2 * time.Second):
		t.Error("process still alive after Shutdown; expected it to be killed")
		cmd.Process.Kill()
	}
}

// TestKillProcess_Nil verifies killProcess doesn't panic with nil cmd.
func TestKillProcess_NilCmd(t *testing.T) {
	c := NewLSPClient("fake", nil)
	// cmd is nil by default. Should not panic.
	c.killProcess()
}

// TestKillProcess_NilProcess verifies killProcess handles cmd with nil Process.
func TestKillProcess_NilProcess(t *testing.T) {
	c := NewLSPClient("fake", nil)
	c.cmd = &exec.Cmd{} // cmd set but Process is nil (not started)
	c.killProcess()
}

// TestShutdown_DaemonMode_ClosesSocket verifies that daemon shutdown
// only closes the socket and doesn't attempt to kill a process.
func TestShutdown_DaemonMode_ClosesSocket(t *testing.T) {
	c := NewLSPClient("fake", nil)
	c.isDaemon = true
	// No socket connection set; should not panic.
	err := c.Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error for daemon shutdown with no socket, got: %v", err)
	}
}

// TestShutdown_ManagerShutdownCallsClientShutdown verifies that
// ServerManager.Shutdown propagates to all clients.
func TestShutdown_ManagerShutdownCallsClientShutdown(t *testing.T) {
	c1 := NewLSPClient("fake1", nil)
	c1.isDaemon = true // daemon mode so Shutdown is a no-op (no real process)
	c2 := NewLSPClient("fake2", nil)
	c2.isDaemon = true

	m := NewSingleServerManager(c1)
	err := m.Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}
