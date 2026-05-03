// main.go is the entry point for the agent-lsp binary. It handles CLI argument
// parsing, subcommand routing (init, doctor, --version, --help), and server
// startup in one of three modes:
//
//   - Single-server:  agent-lsp go gopls
//   - Multi-server:   agent-lsp go:gopls typescript:tsserver,--stdio
//   - Auto-detect:    agent-lsp  (scans PATH for known servers)
//
// After parsing, main creates the appropriate ClientResolver (single or multi),
// activates language extensions, sets up signal handling for graceful shutdown,
// and delegates to server.Run which registers all MCP tools and starts the
// transport (stdio or HTTP).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/internal/extensions"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/logging"

)

const gracefulShutdownTimeout = 5 * time.Second

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(Version)
		os.Exit(0)
	}

	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Println("agent-lsp - MCP server for language intelligence")
		fmt.Println("")
		fmt.Println("Usage:")
		fmt.Println("  agent-lsp go:gopls typescript:tsserver,--stdio   Multi-server mode")
		fmt.Println("  agent-lsp --config /path/to/config.json          Config file mode")
		fmt.Println("  agent-lsp --http [--port 8080] [lsp-args...]     HTTP+SSE transport")
		fmt.Println("  agent-lsp                                        Auto-detect servers")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  agent-lsp init       Auto-detect servers and configure your AI tool")
		fmt.Println("  agent-lsp doctor     Check all configured language servers")
		fmt.Println("  agent-lsp --version  Print version")
		os.Exit(0)
	}

	// Subcommand routing: agent-lsp init
	if len(os.Args) >= 2 && os.Args[1] == "init" {
		runInit(os.Args[2:])
		return
	}

	// Subcommand routing: agent-lsp doctor
	if len(os.Args) >= 2 && os.Args[1] == "doctor" {
		runDoctor(os.Args[2:])
		return
	}

	logging.SetLevelFromEnv()
	parsed, err := config.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage (single-server): agent-lsp <language-id> <lsp-server-binary> [args...]")
		fmt.Fprintln(os.Stderr, "usage (multi-server):   agent-lsp go:gopls typescript:tsserver,--stdio")
		fmt.Fprintln(os.Stderr, "usage (config file):    agent-lsp --config /path/to/lsp-mcp.json")
		fmt.Fprintln(os.Stderr, "usage (auto-detect):    agent-lsp")
		fmt.Fprintln(os.Stderr, "usage (http mode):      agent-lsp --http [--port 8080] [--listen-addr 127.0.0.1] [lsp-args...]  # set AGENT_LSP_TOKEN env var for auth")
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
	if err := runWithRecovery(ctx, resolver, registry, serverPath, serverArgs, parsed.HTTPMode, parsed.HTTPPort, parsed.HTTPToken, parsed.HTTPListenAddr, parsed.HTTPNoAuth, parsed.AuditLogPath); err != nil {
		logging.Log(logging.LevelError, fmt.Sprintf("server error: %v", err))
		os.Exit(1)
	}
}

// runWithRecovery wraps server.Run with a deferred recover to catch panics.
func runWithRecovery(ctx context.Context, resolver lsp.ClientResolver, registry *extensions.ExtensionRegistry, serverPath string, serverArgs []string, httpMode bool, httpPort int, httpToken string, httpListenAddr string, httpNoAuth bool, auditLogPath string) (runErr error) {
	defer func() {
		if r := recover(); r != nil {
			logging.Log(logging.LevelError, fmt.Sprintf("panic recovered: %v", r))
			// L1: Set runErr so the caller exits with code 1.
			// Without this, process supervisors with restart-on-nonzero policies
			// will not restart the process after a panic.
			runErr = fmt.Errorf("panic: %v", r)
			// Attempt graceful shutdown after panic.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
			defer cancel()
			_ = resolver.Shutdown(shutdownCtx)
		}
	}()
	return Run(ctx, resolver, registry, serverPath, serverArgs, httpMode, httpPort, httpToken, httpListenAddr, httpNoAuth, auditLogPath)
}
