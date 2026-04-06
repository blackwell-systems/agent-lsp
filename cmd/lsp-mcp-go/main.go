package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blackwell-systems/lsp-mcp-go/internal/extensions"
	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"

	// Compile-time extension registration.
	_ "github.com/blackwell-systems/lsp-mcp-go/extensions/haskell"
)

const gracefulShutdownTimeout = 5 * time.Second

func main() {
	args := os.Args[1:]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: lsp-mcp-go <language-id> <lsp-server-binary> [lsp-server-args...]")
		fmt.Fprintln(os.Stderr, "  language-id        the language identifier (e.g. go, haskell, typescript)")
		fmt.Fprintln(os.Stderr, "  lsp-server-binary  path to the LSP server binary")
		os.Exit(1)
	}

	languageID := args[0]
	serverPath := args[1]
	serverArgs := args[2:]

	// Validate the LSP server binary exists.
	if _, err := os.Stat(serverPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: LSP server binary not found: %s\n", serverPath)
		fmt.Fprintln(os.Stderr, "usage: lsp-mcp-go <language-id> <lsp-server-binary> [lsp-server-args...]")
		os.Exit(1)
	}

	// Create LSP client (do NOT call Initialize yet — start_lsp tool does that).
	lspClient := lsp.NewLSPClient(serverPath, serverArgs)

	// Create extension registry and activate the language extension (if any).
	registry := extensions.NewRegistry()
	if err := registry.Activate(languageID); err != nil {
		logging.Log(logging.LevelWarning, fmt.Sprintf("failed to activate extension for %s: %v", languageID, err))
	}

	// Set up context with cancellation for signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for SIGINT and SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logging.Log(logging.LevelInfo, fmt.Sprintf("received signal %s, shutting down", sig))
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer shutdownCancel()
		if err := lspClient.Shutdown(shutdownCtx); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("LSP shutdown error: %v", err))
		}
		os.Exit(0)
	}()

	// Run the MCP server with panic recovery.
	if err := runWithRecovery(ctx, lspClient, registry, serverPath, serverArgs); err != nil {
		logging.Log(logging.LevelError, fmt.Sprintf("server error: %v", err))
		os.Exit(1)
	}
}

// runWithRecovery wraps server.Run with a deferred recover to catch panics.
func runWithRecovery(ctx context.Context, lspClient *lsp.LSPClient, registry *extensions.ExtensionRegistry, serverPath string, serverArgs []string) (runErr error) {
	defer func() {
		if r := recover(); r != nil {
			logging.Log(logging.LevelError, fmt.Sprintf("panic recovered: %v", r))
			// Attempt graceful shutdown after panic.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
			defer cancel()
			_ = lspClient.Shutdown(shutdownCtx)
		}
	}()
	return Run(ctx, lspClient, registry, serverPath, serverArgs)
}
