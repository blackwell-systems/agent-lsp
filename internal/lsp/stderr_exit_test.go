package lsp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestHelperCrashingServer is a fake language server: run as a subprocess by
// the test below, it prints an error to stderr and exits with status 1, the
// way kotlin-language-server or jdtls fail on an unsupported Java.
func TestHelperCrashingServer(t *testing.T) {
	if os.Getenv("AGENT_LSP_CRASH_SERVER") != "1" {
		t.Skip("helper process")
	}
	fmt.Fprintln(os.Stderr, "starting server")
	fmt.Fprintln(os.Stderr, "Error: this server requires Java 21 or newer (found 17)")
	os.Exit(1)
}

// A server that dies at startup used to surface only "lsp process exited:
// exit status 1"; the error must carry what the server printed.
func TestInitialize_ServerExitIncludesStderr(t *testing.T) {
	t.Setenv("AGENT_LSP_CRASH_SERVER", "1")
	c := NewLSPClient(os.Args[0], []string{"-test.run=^TestHelperCrashingServer$"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := c.Initialize(ctx, t.TempDir())
	if err == nil {
		t.Fatal("Initialize succeeded against a server that exits")
	}
	// The server may exit before or after the initialize request is written;
	// both paths must report its stderr.
	if !strings.Contains(err.Error(), "requires Java 21 or newer") {
		t.Fatalf("error = %v, want it to include the server's stderr", err)
	}
}

func TestStderrSummary_KeepsExceptionLine(t *testing.T) {
	c := NewLSPClient("x", nil)
	trace := "starting\nException in thread \"main\" java.lang.IllegalArgumentException: 25.0.1\n"
	for i := 0; i < 20; i++ {
		trace += fmt.Sprintf("\tat org.example.Frame%d(Frame.kt:%d)\n", i, i)
	}
	_, _ = stderrTail{c}.Write([]byte(trace))
	got := c.stderrSummary()
	if !strings.Contains(got, "IllegalArgumentException: 25.0.1") || !strings.Contains(got, "Frame19") {
		t.Fatalf("summary = %q, want the exception line and the last frames", got)
	}
}

// A request sent after the process already exited (stdin is gone) must also
// say why it exited, not only that it did.
func TestWriteRaw_AfterExitIncludesExitError(t *testing.T) {
	t.Setenv("AGENT_LSP_CRASH_SERVER", "1")
	c := NewLSPClient(os.Args[0], []string{"-test.run=^TestHelperCrashingServer$"})
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		c.mu.Lock()
		exited := c.exited
		c.mu.Unlock()
		if exited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper server did not exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
	err := c.sendNotification("initialized", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "requires Java 21 or newer") {
		t.Fatalf("err = %v, want the exit error with the server's stderr", err)
	}
}
