package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blackwell-systems/lsp-mcp-go/internal/config"
	"github.com/blackwell-systems/lsp-mcp-go/internal/extensions"
	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"

	// Compile-time extension registration.
	_ "github.com/blackwell-systems/lsp-mcp-go/extensions/haskell"
)

const gracefulShutdownTimeout = 5 * time.Second

func main() {
	parsed, err := config.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		fmt.Fprintln(os.Stderr, "usage (single-server): lsp-mcp-go <language-id> <lsp-server-binary> [args...]")
		fmt.Fprintln(os.Stderr, "usage (multi-server):   lsp-mcp-go go:gopls typescript:tsserver,--stdio")
		fmt.Fprintln(os.Stderr, "usage (config file):    lsp-mcp-go --config /path/to/lsp-mcp.json")
		fmt.Fprintln(os.Stderr, "usage (auto-detect):    lsp-mcp-go")
		os.Exit(1)
	}

	var resolver lsp.ClientResolver
	registry := extensions.NewRegistry()

	// Determine serverPath/serverArgs for start_lsp restart (single-server only).
	var serverPath string
	var serverArgs []string

	if parsed.IsSingleServer {
		// Legacy single-server mode.
		lspClient := lsp.NewLSPClient(parsed.ServerPath, parsed.ServerArgs)
		resolver = lsp.NewSingleServerManager(lspClient)
		serverPath = parsed.ServerPath
		serverArgs = parsed.ServerArgs
		if err := registry.Activate(parsed.LanguageID); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("failed to activate extension for %s: %v", parsed.LanguageID, err))
		}
	} else {
		// Multi-server mode.
		resolver = lsp.NewMultiServerManager(parsed.Config.Servers)
		// Activate extensions for each server entry.
		for _, entry := range parsed.Config.Servers {
			langID := entry.LanguageID
			if langID == "" && len(entry.Extensions) > 0 {
				langID = entry.Extensions[0]
			}
			if err := registry.Activate(langID); err != nil {
				logging.Log(logging.LevelWarning, fmt.Sprintf("failed to activate extension for %s: %v", langID, err))
			}
		}
		// For start_lsp restart in multi-server mode: use first server's command.
		if len(parsed.Config.Servers) > 0 && len(parsed.Config.Servers[0].Command) > 0 {
			serverPath = parsed.Config.Servers[0].Command[0]
			if len(parsed.Config.Servers[0].Command) > 1 {
				serverArgs = parsed.Config.Servers[0].Command[1:]
			}
		}
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
		if err := resolver.Shutdown(shutdownCtx); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("LSP shutdown error: %v", err))
		}
		os.Exit(0)
	}()

	// Run the MCP server with panic recovery.
	if err := runWithRecovery(ctx, resolver, registry, serverPath, serverArgs); err != nil {
		logging.Log(logging.LevelError, fmt.Sprintf("server error: %v", err))
		os.Exit(1)
	}
}

// runWithRecovery wraps server.Run with a deferred recover to catch panics.
func runWithRecovery(ctx context.Context, resolver lsp.ClientResolver, registry *extensions.ExtensionRegistry, serverPath string, serverArgs []string) (runErr error) {
	defer func() {
		if r := recover(); r != nil {
			logging.Log(logging.LevelError, fmt.Sprintf("panic recovered: %v", r))
			// Attempt graceful shutdown after panic.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
			defer cancel()
			_ = resolver.Shutdown(shutdownCtx)
		}
	}()
	return Run(ctx, resolver, registry, serverPath, serverArgs)
}
