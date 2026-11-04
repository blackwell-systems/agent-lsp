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
	if !strings.Contains(err.Error(), "requires Java 21 or newer") {
		t.Fatalf("error = %v, want it to include the server's stderr", err)
	}
}
