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

	// Write initial daemon info.
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
	if err := WriteDaemonInfo(info); err != nil {
		return fmt.Errorf("daemon: failed to write info: %w", err)
	}

	// Start language server.
	client := NewLSPClient(cfg.Command[0], cfg.Command[1:])
	ctx := context.Background()
	if err := client.Initialize(ctx, cfg.RootDir); err != nil {
		cleanup(dir)
		return fmt.Errorf("daemon: LSP initialize failed: %w", err)
	}

	// Start warmup in background.
	go func() {
		// Wait for workspace readiness using $/progress tokens first.
		client.WaitForWorkspaceReadyTimeout(ctx, 10*time.Minute)

		// Mark ready.
		info.Ready = true
		info.LastActivity = time.Now()
		_ = WriteDaemonInfo(info)
		logging.Log(logging.LevelDebug, "daemon: workspace indexed, marked ready")
	}()

	// Listen on Unix socket.
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = client.Shutdown(ctx)
		cleanup(dir)
		return fmt.Errorf("daemon: failed to listen on socket: %w", err)
	}
	defer listener.Close()

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

			info.LastActivity = time.Now()
			_ = WriteDaemonInfo(info)

			go func(c net.Conn) {
				handleBrokerConnection(c, client)
				connMu.Lock()
				delete(connections, c)
				connCount.Add(-1)
				lastDisconn = time.Now()
				connMu.Unlock()
			}(conn)

		case <-inactivityTicker.C:
			if connCount.Load() == 0 && time.Since(lastDisconn) >= daemonInactivityTimeout {
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
func handleBrokerConnection(conn net.Conn, client *LSPClient) {
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
			var params interface{}
			if envelope.Params != nil {
				_ = json.Unmarshal(envelope.Params, &params)
			}
			logging.Log(logging.LevelDebug, fmt.Sprintf("broker: forwarding request %s (id=%s)", envelope.Method, string(envelope.ID)))
			result, err := client.sendRequest(context.Background(), envelope.Method, params)
			var response []byte
			if err != nil {
				logging.Log(logging.LevelDebug, fmt.Sprintf("broker: request %s error: %v", envelope.Method, err))
				response, _ = json.Marshal(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"error":   map[string]interface{}{"code": -32603, "message": err.Error()},
				})
			} else {
				resultLen := 0
				if result != nil {
					resultLen = len(result)
				}
				logging.Log(logging.LevelDebug, fmt.Sprintf("broker: request %s success (result %d bytes)", envelope.Method, resultLen))
				response, _ = json.Marshal(map[string]interface{}{
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
			contentLength, _ = strconv.Atoi(line[16:])
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

func cleanup(dir string) {
	os.RemoveAll(dir)
}
