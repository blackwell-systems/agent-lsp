// daemon_broker.go implements the persistent daemon broker subprocess.
// It owns the language server process, listens on a Unix socket, and proxies
// JSON-RPC between connected agent-lsp clients and the single language server.
//
// Invoked as: agent-lsp daemon-broker --root-dir=X --language=Y --command=Z
//
// Lifecycle:
//  1. Start the language server (pyright/tsserver) as a child process
//  2. Perform LSP initialize with rootDir
//  3. Listen on a Unix domain socket
//  4. Accept connections, proxy JSON-RPC bidirectionally
//  5. Run warmup gate; set ready=true in daemon.json on completion
//  6. Auto-exit after 30 minutes of no connected clients
//  7. Handle SIGTERM: shutdown LSP, cleanup files, exit
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

const (
	daemonInactivityTimeout = 30 * time.Minute
	daemonReadyPollInterval = 5 * time.Second
)

// BrokerConfig holds the configuration for a daemon broker instance.
type BrokerConfig struct {
	RootDir    string
	LanguageID string
	Command    []string // e.g. ["pyright-langserver", "--stdio"]
}

// RunBroker is the main entrypoint for the daemon-broker subprocess.
// It blocks until the broker exits (inactivity timeout, SIGTERM, or server crash).
func RunBroker(cfg BrokerConfig) error {
	dir := DaemonDir(cfg.RootDir, cfg.LanguageID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("daemon: failed to create state dir: %w", err)
	}

	socketPath := filepath.Join(dir, "daemon.sock")
	pidPath := filepath.Join(dir, "daemon.pid")

	// Remove stale socket if exists.
	os.Remove(socketPath)

	// Write PID file.
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("daemon: failed to write PID file: %w", err)
	}

	// Build the in-memory info but DO NOT persist it yet.
	//
	// Race-condition note: writing daemon.json here (the original
	// behaviour) is unsafe. The parent's FindRunningDaemon dials the
	// socket to verify aliveness and wipes the registry on failure;
	// while we're still inside client.Initialize (which can take
	// 30-60+s for Python projects) the socket isn't yet bound, the
	// dial fails, and the parent kills our daemon.json out from under
	// us. We move the WriteDaemonInfo call to AFTER the listener
	// binds so the registry only appears once it's connectable.
	info := &DaemonInfo{
		RootDir:      cfg.RootDir,
		LanguageID:   cfg.LanguageID,
		Command:      cfg.Command,
		SocketPath:   socketPath,
		PID:          os.Getpid(),
		Ready:        false,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
	}
	var infoMu sync.Mutex // protects writes to info fields

	// Start language server.
	logging.Log(logging.LevelInfo, fmt.Sprintf("daemon: NewLSPClient cmd=%v", cfg.Command))
	client := NewLSPClient(cfg.Command[0], cfg.Command[1:])
	ctx := context.Background()
	logging.Log(logging.LevelInfo, fmt.Sprintf("daemon: calling client.Initialize rootDir=%q", cfg.RootDir))
	if err := client.Initialize(ctx, cfg.RootDir); err != nil {
		logging.Log(logging.LevelInfo, fmt.Sprintf("daemon: Initialize FAILED: %v", err))
		cleanup(dir)
		return fmt.Errorf("daemon: LSP initialize failed: %w", err)
	}
	logging.Log(logging.LevelInfo, "daemon: client.Initialize returned ok")

	// Listen on Unix socket BEFORE publishing daemon.json so the
	// registry is only visible once we can actually be connected to.
	logging.Log(logging.LevelInfo, fmt.Sprintf("daemon: net.Listen unix socket=%q", socketPath))
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logging.Log(logging.LevelInfo, fmt.Sprintf("daemon: net.Listen FAILED: %v", err))
		_ = client.Shutdown(ctx)
		cleanup(dir)
		return fmt.Errorf("daemon: failed to listen on socket: %w", err)
	}
	logging.Log(logging.LevelInfo, "daemon: net.Listen ok, defer Close set")
	defer listener.Close()

	// Publish daemon.json now that the socket is bound.
	logging.Log(logging.LevelInfo, "daemon: WriteDaemonInfo")
	if err := WriteDaemonInfo(info); err != nil {
		_ = listener.Close()
		cleanup(dir)
		return fmt.Errorf("daemon: failed to write info: %w", err)
	}

	// Start warmup in background. Updates daemon.json with ready=true
	// once the workspace has finished indexing.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Log(logging.LevelWarning, fmt.Sprintf("daemon: panic in warmup goroutine: %v", r))
			}
		}()

		// Wait for workspace readiness using $/progress tokens first.
		client.WaitForWorkspaceReadyTimeout(ctx, 10*time.Minute)

		// Mark ready.
		infoMu.Lock()
		info.Ready = true
		info.LastActivity = time.Now()
		if err := WriteDaemonInfo(info); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("daemon: failed to write ready flag: %v", err))
		}
		infoMu.Unlock()
		logging.Log(logging.LevelDebug, "daemon: workspace indexed, marked ready")
	}()

	// Track active connections.
	var (
		connMu      sync.Mutex
		connections = make(map[net.Conn]struct{})
		connCount   atomic.Int32
		lastDisconn = time.Now()
	)

	// Accept connections in a goroutine.
	newConns := make(chan net.Conn, 8)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Log(logging.LevelWarning, fmt.Sprintf("daemon: panic in socket accept goroutine: %v", r))
			}
		}()

		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			newConns <- conn
		}
	}()

	// Handle SIGTERM for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Inactivity timer.
	inactivityTicker := time.NewTicker(30 * time.Second)
	defer inactivityTicker.Stop()

	// Main event loop.
	for {
		select {
		case conn := <-newConns:
			connMu.Lock()
			connections[conn] = struct{}{}
			connCount.Add(1)
			connMu.Unlock()

			infoMu.Lock()
			info.LastActivity = time.Now()
			_ = WriteDaemonInfo(info)
			infoMu.Unlock()

			go func(c net.Conn) {
				defer func() {
					if r := recover(); r != nil {
						logging.Log(logging.LevelWarning, fmt.Sprintf("daemon: panic in broker connection handler: %v", r))
					}
				}()
				handleBrokerConnection(ctx, c, client)
				connMu.Lock()
				delete(connections, c)
				connCount.Add(-1)
				lastDisconn = time.Now()
				connMu.Unlock()
			}(conn)

		case <-inactivityTicker.C:
			connMu.Lock()
			idle := connCount.Load() == 0 && time.Since(lastDisconn) >= daemonInactivityTimeout
			connMu.Unlock()
			if idle {
				logging.Log(logging.LevelDebug, "daemon: inactivity timeout, shutting down")
				_ = client.Shutdown(ctx)
				cleanup(dir)
				return nil
			}

		case sig := <-sigCh:
			logging.Log(logging.LevelDebug, fmt.Sprintf("daemon: received %s, shutting down", sig))
			_ = client.Shutdown(ctx)
			connMu.Lock()
			for c := range connections {
				c.Close()
			}
			connMu.Unlock()
			cleanup(dir)
			return nil
		}
	}
}

// handleBrokerConnection proxies JSON-RPC between a connected client and the
// language server. The connection uses Content-Length framing (same as LSP stdio).
// ctx is the broker's lifecycle context; forwarded requests are cancelled when
// the broker shuts down.
func handleBrokerConnection(ctx context.Context, conn net.Conn, client *LSPClient) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		// Read a Content-Length framed message from the client.
		msg, err := readFramedMessage(reader)
		if err != nil {
			return // client disconnected
		}

		// Parse to determine if it's a request or notification.
		var envelope struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		if envelope.ID != nil {
			// It's a request: forward to LSP server and send response back.
			var params any
			if envelope.Params != nil {
				_ = json.Unmarshal(envelope.Params, &params)
			}
			logging.Log(logging.LevelDebug, fmt.Sprintf("broker: forwarding request %s (id=%s)", envelope.Method, string(envelope.ID)))
			result, err := client.sendRequest(ctx, envelope.Method, params)
			var response []byte
			if err != nil {
				logging.Log(logging.LevelDebug, fmt.Sprintf("broker: request %s error: %v", envelope.Method, err))
				response, _ = json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"error":   map[string]any{"code": -32603, "message": err.Error()},
				})
			} else {
				resultLen := 0
				if result != nil {
					resultLen = len(result)
				}
				logging.Log(logging.LevelDebug, fmt.Sprintf("broker: request %s success (result %d bytes)", envelope.Method, resultLen))
				response, _ = json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"result":  result,
				})
			}
			if err := writeFramedMessage(conn, response); err != nil {
				logging.Log(logging.LevelDebug, fmt.Sprintf("broker: failed to write response: %v", err))
				return
			}
		} else {
			// It's a notification: forward to LSP server.
			logging.Log(logging.LevelDebug, fmt.Sprintf("broker: forwarding notification %s", envelope.Method))
			_ = client.sendNotification(envelope.Method, envelope.Params)
		}
	}
}

// readFramedMessage reads a Content-Length framed message from a reader.
func readFramedMessage(reader *bufio.Reader) ([]byte, error) {
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = line[:len(line)-1] // trim \n
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1] // trim \r
		}
		if line == "" {
			break // empty line separates headers from body
		}
		if len(line) > 16 && line[:16] == "Content-Length: " {
			var parseErr error
			contentLength, parseErr = strconv.Atoi(line[16:])
			if parseErr != nil {
				return nil, fmt.Errorf("invalid Content-Length: %s", line)
			}
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("no Content-Length header")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	return body, err
}

// writeFramedMessage writes a Content-Length framed message to a writer.
func writeFramedMessage(w io.Writer, msg []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
	_, err := w.Write(append([]byte(header), msg...))
	return err
}

// cleanup removes the daemon state directory, but ONLY if the daemon.pid
// inside refers to the current process. This guards against a freshly
// spawned broker that fails to bind its listener (because an existing
// broker already owns the socket) from wiping the OTHER broker's
// registry — a race that previously left the older broker running with
// no registry entry, then the parent timed out waiting for a daemon it
// could no longer find. If we don't own this dir, leave it alone.
func cleanup(dir string) {
	pidPath := filepath.Join(dir, "daemon.pid")
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		// No PID file at all — safe to wipe.
		os.RemoveAll(dir)
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		// PID file unreadable — safe to wipe.
		os.RemoveAll(dir)
		return
	}
	if pid != os.Getpid() {
		// Not ours — another broker owns this registry. Don't touch it.
		logging.Log(logging.LevelDebug, fmt.Sprintf("daemon: cleanup skipped — dir %s belongs to PID %d, not us (%d)", dir, pid, os.Getpid()))
		return
	}
	os.RemoveAll(dir)
}
