// client.go is the core LSP subprocess client. It manages the full lifecycle
// of a language server process: spawning, JSON-RPC 2.0 framing, request/response
// correlation via integer IDs, server-initiated request handling, diagnostic
// caching with subscriber notifications, workspace progress tracking ($/progress),
// and automatic file watching via fsnotify.
//
// The client is thread-safe. All public methods may be called concurrently.
// Internal state is protected by fine-grained mutexes (mu for process state,
// pendingMu for in-flight requests, diagMu for diagnostics, etc.).
//
// Lifecycle:
//   1. NewLSPClient creates an unstarted client.
//   2. Initialize spawns the subprocess, performs the LSP handshake,
//      stores server capabilities, and starts the auto-watcher.
//   3. Tool handlers call methods like GetReferences, GetDefinition, etc.
//   4. Shutdown sends shutdown+exit and stops the watcher.
//   5. Restart combines Shutdown + Initialize for server recovery.
//
// The readLoop goroutine dispatches all incoming messages: responses are
// matched to pending requests by ID; notifications (diagnostics, progress)
// are handled inline; server-initiated requests (workspace/configuration,
// workspace/applyEdit) are responded to immediately.
package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	uripkg "github.com/blackwell-systems/agent-lsp/internal/uri"
	"github.com/fsnotify/fsnotify"
)

// Per-method request timeouts. Mirrors the TypeScript REQUEST_TIMEOUTS table
// in src/lspClient.ts for parity. References require full workspace indexing;
// initialize can be slow on cold-start JVM-based servers.
var requestTimeouts = map[string]time.Duration{
	"initialize":                    300 * time.Second,
	"textDocument/references":       120 * time.Second,
	"textDocument/hover":            30 * time.Second,
	"textDocument/completion":       30 * time.Second,
	"textDocument/codeAction":       30 * time.Second,
	"textDocument/definition":       30 * time.Second,
	"textDocument/documentSymbol":   30 * time.Second,
	"workspace/symbol":              30 * time.Second,
	"textDocument/signatureHelp":    30 * time.Second,
	"textDocument/formatting":       30 * time.Second,
	"textDocument/rename":           30 * time.Second,
	"workspace/executeCommand":      30 * time.Second,
	"textDocument/declaration":      30 * time.Second,
	"textDocument/prepareRename":    30 * time.Second,
	"textDocument/prepareCallHierarchy": 30 * time.Second,
	"callHierarchy/incomingCalls":        60 * time.Second,
	"callHierarchy/outgoingCalls":         60 * time.Second,
	"textDocument/prepareTypeHierarchy": 30 * time.Second,
	"typeHierarchy/supertypes":           60 * time.Second,
	"typeHierarchy/subtypes":             60 * time.Second,
	"textDocument/inlayHint":              30 * time.Second,
	"textDocument/documentHighlight":      10 * time.Second,
	"textDocument/semanticTokens/range":   30 * time.Second,
	"textDocument/semanticTokens/full":    30 * time.Second,
}

const defaultTimeout = 30 * time.Second

func timeoutFor(method string) time.Duration {
	if d, ok := requestTimeouts[method]; ok {
		return d
	}
	return defaultTimeout
}

// jsonrpcMsg is a generic JSON-RPC 2.0 message.
// ID is json.RawMessage so it accepts both integer and string IDs per the
// JSON-RPC 2.0 spec (some servers, e.g. prisma-language-server, use strings).
type jsonrpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// pendingRequest holds the reply channel for an outgoing request.
type pendingRequest struct {
	ch  chan json.RawMessage
	err chan error
}

// docMeta holds per-document metadata needed for reopening.
type docMeta struct {
	filePath   string
	languageID string
	version    int
}

// LSPClient is the core LSP subprocess client. It spawns the LSP binary, handles
// Content-Length framing, request/response correlation, server-initiated requests,
// and workspace progress tracking. Thread-safe.
type LSPClient struct {
	// constructor params
	serverPath string
	serverArgs []string

	// workspace root (set during Initialize)
	rootDir string

	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	frameReader *FrameReader
	nextID      atomic.Int64

	initialized bool

	// pending RPC requests
	pendingMu sync.Mutex
	pending   map[int]*pendingRequest

	// open documents
	openDocs  map[string]docMeta // uri -> meta

	// diagnostics
	diagMu    sync.RWMutex
	diags     map[string][]types.LSPDiagnostic
	diagSubs  []types.DiagnosticUpdateCallback

	// workspace readiness ($/progress)
	progressMu     sync.Mutex
	progressTokens map[interface{}]struct{} // active begin tokens
	progressCond   *sync.Cond // signalled when progressTokens becomes empty

	// server capabilities and identity (from initialize response)
	capsMu          sync.RWMutex
	capabilities    map[string]interface{}
	serverName      string
	serverVersion   string
	workspaceFolders []workspaceFolder

	// semantic token legend (from initialize response)
	legendMu        sync.RWMutex
	legendTypes     []string
	legendModifiers []string

	// stderr drain
	stderrBuf []byte
	stderrMu  sync.Mutex

	// auto-watcher: watches the workspace root and notifies the server
	// of file changes automatically, keeping the LSP index fresh.
	watcherMu   sync.Mutex // guards watcherStop (C2: prevents data race)
	watcherStop chan struct{}
	watcher     *fsnotify.Watcher  // C1: held so addWatcherRoot can Add() new dirs
}

// NewLSPClient creates a new, unstarted LSP client.
func NewLSPClient(serverPath string, serverArgs []string) *LSPClient {
	c := &LSPClient{
		serverPath:     serverPath,
		serverArgs:     serverArgs,
		pending:        make(map[int]*pendingRequest),
		openDocs:       make(map[string]docMeta),
		diags:          make(map[string][]types.LSPDiagnostic),
		progressTokens: make(map[interface{}]struct{}),
		capabilities:   make(map[string]interface{}),
	}
	c.nextID.Store(0)
	c.progressCond = sync.NewCond(&c.progressMu)
	return c
}

// start spawns the subprocess and begins reading responses.
func (c *LSPClient) start() error {
	cmd := exec.Command(c.serverPath, c.serverArgs...)
	// Strip GOWORK from the subprocess environment. The inherited shell value
	// is a session artifact pointing at whatever workspace the user had active,
	// which is almost never the workspace being analyzed here. Removing it lets
	// gopls discover the correct go.work naturally by walking up from root_dir.
	cmd.Env = removeEnv(os.Environ(), "GOWORK")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.frameReader = NewFrameReader(stdout)

	go c.drainStderr(stderr)
	go c.readLoop()

	// Monitor process exit.
	go func() {
		err := cmd.Wait()
		exitErr := fmt.Errorf("lsp process exited: %w", err)
		c.rejectPending(exitErr)
		c.mu.Lock()
		c.initialized = false
		c.mu.Unlock()
		if err != nil {
			c.stderrMu.Lock()
			buf := string(c.stderrBuf)
			c.stderrMu.Unlock()
			logging.Log(logging.LevelError, fmt.Sprintf("LSP server exited with error. Last stderr:\n%s", buf))
		}
	}()

	return nil
}

// drainStderr reads stderr and buffers the last 4KB.
func (c *LSPClient) drainStderr(r io.Reader) {
	buf := make([]byte, 512)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			c.stderrMu.Lock()
			c.stderrBuf = append(c.stderrBuf, buf[:n]...)
			if len(c.stderrBuf) > 4096 {
				c.stderrBuf = c.stderrBuf[len(c.stderrBuf)-4096:]
			}
			c.stderrMu.Unlock()
			logging.Log(logging.LevelDebug, "LSP stderr: "+string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

// readLoop reads and dispatches all incoming messages.
func (c *LSPClient) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			logging.Log(logging.LevelError, fmt.Sprintf("readLoop panic: %v\n%s", r, debug.Stack()))
		}
	}()
	for {
		raw, err := c.frameReader.ReadMessage()
		if err != nil {
			if err != io.EOF {
				logging.Log(logging.LevelDebug, "LSP read loop ended: "+err.Error())
			}
			return
		}
		c.dispatch(raw)
	}
}

// dispatch decodes and routes one incoming message.
func (c *LSPClient) dispatch(raw []byte) {
	var msg jsonrpcMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		logging.Log(logging.LevelDebug, "LSP dispatch parse error: "+err.Error())
		return
	}

	// Response to one of our requests (we always send integer IDs).
	if msg.ID != nil && msg.Method == "" {
		var intID int
		if err := json.Unmarshal(msg.ID, &intID); err == nil {
			c.pendingMu.Lock()
			req, ok := c.pending[intID]
			if ok {
				delete(c.pending, intID)
			}
			c.pendingMu.Unlock()
			if ok {
				if msg.Error != nil {
					req.err <- fmt.Errorf("lsp error %d: %s", msg.Error.Code, msg.Error.Message)
				} else {
					req.ch <- msg.Result
				}
				return
			}
		}
		// ID present but not an integer we sent — fall through to handle as
		// a server-initiated request (some servers use string IDs).
	}

	// Notification or server-initiated request.
	switch msg.Method {
	case "textDocument/publishDiagnostics":
		c.handlePublishDiagnostics(msg.Params)
	case "$/progress":
		c.handleProgress(msg.Params)
	case "window/workDoneProgress/create":
		// Pre-register token; respond null.
		var p struct {
			Token interface{} `json:"token"`
		}
		if err := json.Unmarshal(msg.Params, &p); err == nil && p.Token != nil {
			c.progressMu.Lock()
			c.progressTokens[p.Token] = struct{}{}
			c.progressMu.Unlock()
		}
		if msg.ID != nil {
			c.sendResponse(msg.ID, nil)
		}
	case "workspace/configuration":
		// Respond with an array of empty objects (one per requested item).
		// Using {} instead of null is critical for servers like jdtls that
		// interpret null as "no configuration available" and skip project
		// import, while {} means "use defaults" and triggers Maven/Gradle
		// import with default settings.
		if msg.ID != nil {
			var p struct {
				Items []struct {
					Section string `json:"section"`
				} `json:"items"`
			}
			if err := json.Unmarshal(msg.Params, &p); err != nil {
				logging.Log(logging.LevelDebug, fmt.Sprintf("workspace/configuration: unmarshal params: %v", err))
			}
			results := make([]interface{}, len(p.Items))
			for i := range results {
				results[i] = map[string]interface{}{}
			}
			c.sendResponse(msg.ID, results)
		}
	case "workspace/applyEdit":
		// Apply the workspace edit and respond with ApplyWorkspaceEditResult.
		// Per LSP spec: respond applied=false with failureReason on error.
		if msg.ID != nil {
			var p struct {
				Edit interface{} `json:"edit"`
			}
			var applyErr error
			if err := json.Unmarshal(msg.Params, &p); err == nil && p.Edit != nil {
				// Fresh context: readLoop has no per-request context (it processes
				// server-initiated requests outside any client call). context.Background()
				// is intentional — consistent with H4 pattern in server.go.
				applyCtx, applyCancel := context.WithTimeout(context.Background(), defaultTimeout)
				applyErr = c.ApplyWorkspaceEdit(applyCtx, p.Edit)
				applyCancel()
			}
			result := map[string]interface{}{"applied": applyErr == nil}
			if applyErr != nil {
				result["failureReason"] = applyErr.Error()
			}
			c.sendResponse(msg.ID, result)
		}
	case "client/registerCapability":
		if msg.ID != nil {
			c.sendResponse(msg.ID, nil)
		}
	default:
		// Unknown server request — respond null to unblock.
		if msg.ID != nil {
			c.sendResponse(msg.ID, nil)
		}
	}
}

// sendResponse sends a JSON-RPC response for a server-initiated request.
// id is echoed back verbatim as json.RawMessage, preserving the original type
// (integer or string) as required by JSON-RPC 2.0.
func (c *LSPClient) sendResponse(id json.RawMessage, result interface{}) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return
	}
	c.writeRaw(body)
}

// handlePublishDiagnostics processes textDocument/publishDiagnostics notifications.
func (c *LSPClient) handlePublishDiagnostics(params json.RawMessage) {
	var p struct {
		URI         string                `json:"uri"`
		Diagnostics []types.LSPDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}

	c.diagMu.Lock()
	c.diags[p.URI] = p.Diagnostics
	subs := make([]types.DiagnosticUpdateCallback, len(c.diagSubs))
	copy(subs, c.diagSubs)
	c.diagMu.Unlock()

	for _, cb := range subs {
		cb(p.URI, p.Diagnostics)
	}
}

// handleProgress processes $/progress notifications.
func (c *LSPClient) handleProgress(params json.RawMessage) {
	var p struct {
		Token interface{} `json:"token"`
		Value struct {
			Kind string `json:"kind"`
		} `json:"value"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	c.progressMu.Lock()
	defer c.progressMu.Unlock()
	switch p.Value.Kind {
	case "begin":
		c.progressTokens[p.Token] = struct{}{}
	case "report":
		logging.Log(logging.LevelDebug, fmt.Sprintf("$/progress report token=%v", p.Token))
	case "end":
		delete(c.progressTokens, p.Token)
		if len(c.progressTokens) == 0 {
			c.progressCond.Broadcast()
		}
	}
}

// waitForWorkspaceReady blocks until activeProgressTokens is empty or 60s
// elapses. Uses a condition variable so handleProgress can signal immediately.
// A timer goroutine guarantees the deadline fires even if gopls never emits
// a matching "end" progress token (preventing an indefinite block).
func (c *LSPClient) waitForWorkspaceReady(ctx context.Context) {
	c.WaitForWorkspaceReadyTimeout(ctx, 60*time.Second)
}

// WaitForWorkspaceReadyTimeout blocks until all active $/progress tokens are
// done or the given timeout elapses. Use this when the default 60s cap is
// insufficient (e.g. jdtls Maven workspace indexing).
//
// If no progress tokens are active yet, waits up to 10 seconds for the first
// $/progress begin to arrive. Servers like jdtls emit progress tokens
// asynchronously after initialize — without this grace period, the wait
// returns immediately before indexing has even started.
func (c *LSPClient) WaitForWorkspaceReadyTimeout(ctx context.Context, timeout time.Duration) {
	c.progressMu.Lock()

	// Grace period: if no progress tokens yet and caller requested an extended
	// wait (>60s), wait briefly for the first token. Avoids 10s delay for
	// default-timeout callers (GetReferences, etc.) on servers that never emit progress.
	if len(c.progressTokens) == 0 && timeout > 60*time.Second {
		grace := make(chan struct{})
		go func() {
			select {
			case <-time.After(10 * time.Second):
				c.progressCond.Broadcast()
			case <-ctx.Done():
				c.progressCond.Broadcast()
			case <-grace:
			}
		}()
		graceDeadline := time.Now().Add(10 * time.Second)
		for len(c.progressTokens) == 0 {
			if time.Now().After(graceDeadline) || ctx.Err() != nil {
				close(grace)
				c.progressMu.Unlock()
				return
			}
			c.progressCond.Wait()
		}
		close(grace)
	}

	// Guarantee a wakeup at the deadline so the cond var doesn't block forever
	// if the server emits a begin token without a corresponding end token.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-time.After(timeout):
			c.progressCond.Broadcast()
		case <-ctx.Done():
			c.progressCond.Broadcast()
		case <-done:
		}
	}()

	deadline := time.Now().Add(timeout)
	for len(c.progressTokens) > 0 {
		if time.Now().After(deadline) || ctx.Err() != nil {
			return
		}
		c.progressCond.Wait()
	}
}

// rejectPending rejects all pending requests with the given error.
func (c *LSPClient) rejectPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, req := range c.pending {
		req.err <- err
		delete(c.pending, id)
	}
}

// writeRaw writes a framed message to stdin.
func (c *LSPClient) writeRaw(body []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("LSP client not started")
	}
	_, err := c.stdin.Write(EncodeMessage(body))
	if err != nil {
		return fmt.Errorf("writeRaw: %w", err)
	}
	return nil
}

// SendRequest sends a JSON-RPC request and waits for the response.
// This is the low-level method that skips capability checks and indexing waits.
// Use the typed methods (GetReferences, GetInfoOnLocation, etc.) for normal
// usage. SendRequest is useful for batch/measurement scenarios where the
// workspace is already indexed.
func (c *LSPClient) SendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	return c.sendRequest(ctx, method, params)
}

// sendRequest sends a JSON-RPC request and waits for the response.
func (c *LSPClient) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	idJSON, _ := json.Marshal(id)
	msg := jsonrpcMsg{
		JSONRPC: "2.0",
		ID:      idJSON,
		Method:  method,
		Params:  p,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	ch := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)

	c.pendingMu.Lock()
	c.pending[id] = &pendingRequest{ch: ch, err: errCh}
	c.pendingMu.Unlock()

	if err := c.writeRaw(body); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	timeout := timeoutFor(method)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-ch:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-timer.C:
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("request %s timed out after %s", method, timeout)
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

// sendNotification sends a JSON-RPC notification (no ID, no response expected).
func (c *LSPClient) sendNotification(method string, params interface{}) error {
	p, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("sendNotification %s: marshal params: %w", method, err)
	}
	msg := jsonrpcMsg{
		JSONRPC: "2.0",
		Method:  method,
		Params:  p,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("sendNotification %s: marshal message: %w", method, err)
	}
	return c.writeRaw(body)
}

// RootDir returns the workspace root directory set during Initialize.
func (c *LSPClient) RootDir() string {
	return c.rootDir
}

// IsInitialized reports whether the LSP handshake has completed successfully.
// A non-nil client is not necessarily initialized — NewLSPClient creates a
// client object but Initialize must be called to start the process and complete
// the LSP handshake.
func (c *LSPClient) IsInitialized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initialized
}

// isJDTLS reports whether the server binary appears to be Eclipse jdtls.
// Checks the binary name for "jdtls" (covers /usr/local/bin/jdtls and
// wrapper scripts named jdtls).
func (c *LSPClient) isJDTLS() bool {
	base := filepath.Base(c.serverPath)
	return strings.Contains(strings.ToLower(base), "jdtls")
}

// Initialize starts the LSP process and performs the LSP handshake.
func (c *LSPClient) Initialize(ctx context.Context, rootDir string) error {
	if err := c.start(); err != nil {
		return err
	}

	c.rootDir = rootDir
	rootURI := (&url.URL{Scheme: "file", Path: rootDir}).String()
	c.capsMu.Lock()
	c.workspaceFolders = []workspaceFolder{{URI: rootURI, Name: rootDir}}
	c.capsMu.Unlock()

	initParams := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		// rootPath is deprecated in favour of rootUri; omitted per LSP 3.17.
		"clientInfo": map[string]interface{}{
			"name":    "agent-lsp",
			"version": "0.1.0",
		},
		"capabilities": map[string]interface{}{
			"workspace": map[string]interface{}{
				"configuration":    true,
				"workDoneProgress": true,
				"applyEdit":        true,
				"workspaceEdit": map[string]interface{}{
					"documentChanges": true,
				},
				"didChangeConfiguration": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"didChangeWatchedFiles": map[string]interface{}{
					"dynamicRegistration": true,
				},
			},
			"textDocument": map[string]interface{}{
				"hover": map[string]interface{}{
					"dynamicRegistration": true,
					"contentFormat":       []string{"markdown", "plaintext"},
				},
				"completion": map[string]interface{}{
					"dynamicRegistration":  true,
					"completionItem":       map[string]interface{}{},
					"contextSupport":       true,
				},
				"references": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"definition": map[string]interface{}{
					"dynamicRegistration": true,
					"linkSupport":         true,
				},
				"implementation": map[string]interface{}{
					"dynamicRegistration": true,
					"linkSupport":         true,
				},
				"typeDefinition": map[string]interface{}{
					"dynamicRegistration": true,
					"linkSupport":         true,
				},
				"declaration": map[string]interface{}{
					"dynamicRegistration": true,
					"linkSupport":         true,
				},
				"codeAction": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"signatureHelp": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"documentSymbol": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"rename": map[string]interface{}{
					"dynamicRegistration": true,
					"prepareSupport":      true,
				},
				"formatting": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"rangeFormatting": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"publishDiagnostics": map[string]interface{}{
					"relatedInformation": true,
					"tagSupport": map[string]interface{}{
						"valueSet": []int{1, 2},
					},
				},
				"callHierarchy": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"typeHierarchy": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"inlayHint": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"documentHighlight": map[string]interface{}{
					"dynamicRegistration": true,
				},
				"semanticTokens": map[string]interface{}{
					"dynamicRegistration": true,
					"requests": map[string]interface{}{
						"range": true,
						"full":  true,
					},
					"tokenTypes":              []string{},
					"tokenModifiers":          []string{},
					"formats":                 []string{"relative"},
					"overlappingTokenSupport": false,
					"multilineTokenSupport":   false,
				},
			},
			"window": map[string]interface{}{
				"workDoneProgress": true,
			},
		},
		"workspaceFolders": []map[string]interface{}{
			{"uri": rootURI, "name": rootDir},
		},
	}

	// jdtls (Java) needs initializationOptions with settings and
	// extendedClientCapabilities to trigger Maven/Gradle project import
	// and emit $/progress tokens during workspace indexing. Without these,
	// jdtls starts but silently skips project import and never indexes.
	if c.isJDTLS() {
		initParams["initializationOptions"] = map[string]interface{}{
			"settings": map[string]interface{}{
				"java": map[string]interface{}{
					"import": map[string]interface{}{
						"maven": map[string]interface{}{
							"enabled": true,
						},
						"gradle": map[string]interface{}{
							"enabled": true,
						},
					},
					"autobuild": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			"extendedClientCapabilities": map[string]interface{}{
				"progressReportProvider":    true,
				"classFileContentsSupport":  true,
				"overrideMethodsPromptSupport": true,
				"hashCodeEqualsPromptSupport":  true,
				"advancedOrganizeImportsSupport": true,
				"generateToStringPromptSupport": true,
				"advancedGenerateAccessorsSupport": true,
				"generateConstructorsPromptSupport": true,
				"generateDelegateMethodsPromptSupport": true,
				"advancedExtractRefactoringSupport": true,
			},
		}
	}

	result, err := c.sendRequest(ctx, "initialize", initParams)
	if err != nil {
		return fmt.Errorf("initialize request: %w", err)
	}

	// Store server capabilities and identity.
	var initResult struct {
		Capabilities map[string]interface{} `json:"capabilities"`
		ServerInfo   *struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(result, &initResult); err == nil {
		c.capsMu.Lock()
		if initResult.Capabilities != nil {
			c.capabilities = initResult.Capabilities
		}
		if initResult.ServerInfo != nil {
			c.serverName = initResult.ServerInfo.Name
			c.serverVersion = initResult.ServerInfo.Version
		}
		c.capsMu.Unlock()
	}

	// Extract semantic token legend if advertised.
	var fullResult struct {
		Capabilities struct {
			SemanticTokensProvider interface{} `json:"semanticTokensProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &fullResult); err == nil && fullResult.Capabilities.SemanticTokensProvider != nil {
		// Legend may be nested inside an options object or at the top level.
		var legend struct {
			Legend struct {
				TokenTypes     []string `json:"tokenTypes"`
				TokenModifiers []string `json:"tokenModifiers"`
			} `json:"legend"`
			// Handle case where semanticTokensProvider IS the options object directly.
			TokenTypes     []string `json:"tokenTypes"`
			TokenModifiers []string `json:"tokenModifiers"`
		}
		b, _ := json.Marshal(fullResult.Capabilities.SemanticTokensProvider)
		if err := json.Unmarshal(b, &legend); err == nil {
			c.legendMu.Lock()
			if len(legend.Legend.TokenTypes) > 0 {
				c.legendTypes = legend.Legend.TokenTypes
				c.legendModifiers = legend.Legend.TokenModifiers
			} else if len(legend.TokenTypes) > 0 {
				c.legendTypes = legend.TokenTypes
				c.legendModifiers = legend.TokenModifiers
			}
			c.legendMu.Unlock()
		}
	}

	// Set initialized = true BEFORE sending the notification (race fix).
	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()

	// Start auto-watcher so file changes are reflected in the LSP index
	// without requiring manual did_change_watched_files calls.
	c.startWatcher(rootDir)

	return c.sendNotification("initialized", map[string]interface{}{})
}

// Shutdown gracefully shuts down the LSP server.
func (c *LSPClient) Shutdown(ctx context.Context) error {
	c.stopWatcher()
	_, err := c.sendRequest(ctx, "shutdown", nil)
	if err != nil {
		return fmt.Errorf("shutdown request: %w", err)
	}
	_ = c.sendNotification("exit", nil)
	c.mu.Lock()
	if c.stdin != nil {
		c.stdin.Close()
		c.stdin = nil
	}
	c.mu.Unlock()
	return nil
}

// Restart shuts down the current server and reinitializes it.
func (c *LSPClient) Restart(ctx context.Context, rootDir string) error {
	// Try graceful shutdown; ignore errors since we restart anyway.
	_ = c.Shutdown(ctx)

	// Reset state.
	c.mu.Lock()
	c.initialized = false
	c.mu.Unlock()
	c.pendingMu.Lock()
	c.pending = make(map[int]*pendingRequest)
	c.pendingMu.Unlock()
	c.progressMu.Lock()
	c.progressTokens = make(map[interface{}]struct{})
	c.progressMu.Unlock()
	c.capsMu.Lock()
	c.capabilities = make(map[string]interface{})
	c.capsMu.Unlock()

	// C1: clear per-session state so the fresh server receives didOpen
	// (not didChange) for all documents and serves no stale diagnostics.
	c.mu.Lock()
	c.openDocs = make(map[string]docMeta)
	c.mu.Unlock()
	c.diagMu.Lock()
	c.diags = make(map[string][]types.LSPDiagnostic)
	c.diagMu.Unlock()
	c.legendMu.Lock()
	c.legendTypes = nil
	c.legendModifiers = nil
	c.legendMu.Unlock()

	return c.Initialize(ctx, rootDir)
}

// OpenDocument sends textDocument/didOpen or didChange if already open.
func (c *LSPClient) OpenDocument(ctx context.Context, uri, text, languageID string) error {
	c.mu.Lock()
	meta, alreadyOpen := c.openDocs[uri]
	c.mu.Unlock()

	if alreadyOpen {
		// Send didChange (full sync), increment version.
		c.mu.Lock()
		meta.version++
		c.openDocs[uri] = meta
		c.mu.Unlock()
		return c.sendNotification("textDocument/didChange", map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":     uri,
				"version": meta.version,
			},
			"contentChanges": []map[string]interface{}{
				{"text": text},
			},
		})
	}

	c.mu.Lock()
	c.openDocs[uri] = docMeta{
		filePath:   uripkg.URIToPath(uri),
		languageID: languageID,
		version:    1,
	}
	c.mu.Unlock()

	return c.sendNotification("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": languageID,
			"version":    1,
			"text":       text,
		},
	})
}

// CloseDocument sends textDocument/didClose and removes tracking.
func (c *LSPClient) CloseDocument(ctx context.Context, uri string) error {
	c.mu.Lock()
	delete(c.openDocs, uri)
	c.mu.Unlock()
	return c.sendNotification("textDocument/didClose", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
}

// isDocumentOpen reports whether uri is currently tracked as open.
func (c *LSPClient) isDocumentOpen(uri string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.openDocs[uri]
	return ok
}

// GetOpenDocuments returns a snapshot of all currently open document URIs.
func (c *LSPClient) GetOpenDocuments() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.openDocs))
	for uri := range c.openDocs {
		out = append(out, uri)
	}
	return out
}

// GetDiagnostics returns the current diagnostics for uri.
func (c *LSPClient) GetDiagnostics(uri string) []types.LSPDiagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	d := c.diags[uri]
	if d == nil {
		return []types.LSPDiagnostic{}
	}
	out := make([]types.LSPDiagnostic, len(d))
	copy(out, d)
	return out
}

// GetAllDiagnostics returns a copy of all diagnostics.
func (c *LSPClient) GetAllDiagnostics() map[string][]types.LSPDiagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	out := make(map[string][]types.LSPDiagnostic, len(c.diags))
	for uri, d := range c.diags {
		cp := make([]types.LSPDiagnostic, len(d))
		copy(cp, d)
		out[uri] = cp
	}
	return out
}

// SubscribeToDiagnostics registers cb to be called on every publishDiagnostics notification.
// It immediately fires cb for every URI already in the diagnostics cache so that
// new subscribers do not miss diagnostics published before they registered.
func (c *LSPClient) SubscribeToDiagnostics(cb types.DiagnosticUpdateCallback) {
	c.diagMu.Lock()
	c.diagSubs = append(c.diagSubs, cb)
	// Replay existing diagnostics under the same lock to avoid races.
	snapshot := make(map[string][]types.LSPDiagnostic, len(c.diags))
	for uri, diags := range c.diags {
		cp := make([]types.LSPDiagnostic, len(diags))
		copy(cp, diags)
		snapshot[uri] = cp
	}
	c.diagMu.Unlock()
	for uri, diags := range snapshot {
		cb(uri, diags)
	}
}

// UnsubscribeFromDiagnostics removes a previously registered callback.
// Uses reflect to compare function pointers (the only way to compare func values in Go).
func (c *LSPClient) UnsubscribeFromDiagnostics(cb types.DiagnosticUpdateCallback) {
	c.diagMu.Lock()
	defer c.diagMu.Unlock()
	cbPtr := reflect.ValueOf(cb).Pointer()
	subs := make([]types.DiagnosticUpdateCallback, 0, len(c.diagSubs))
	for _, s := range c.diagSubs {
		if reflect.ValueOf(s).Pointer() != cbPtr {
			subs = append(subs, s)
		}
	}
	c.diagSubs = subs
}

// languageIDFromURI infers a language ID from a file:// URI by extension.
// Mirrors the extension→language mapping in internal/lsp/manager.go inferLanguageID.
// Falls back to "plaintext" for unknown extensions.
func languageIDFromURI(uri string) string {
	// Strip query/fragment and extract path.
	lower := strings.ToLower(uri)
	// Find last dot after the last slash.
	if idx := strings.LastIndex(lower, "."); idx >= 0 {
		ext := lower[idx+1:]
		// Strip any fragment or query suffix.
		if end := strings.IndexAny(ext, "?#"); end >= 0 {
			ext = ext[:end]
		}
		switch ext {
		case "go":
			return "go"
		case "ts", "tsx":
			return "typescript"
		case "js", "jsx":
			return "javascript"
		case "py":
			return "python"
		case "rs":
			return "rust"
		case "hs", "lhs":
			return "haskell"
		case "rb":
			return "ruby"
		case "cs":
			return "csharp"
		case "kt", "kts":
			return "kotlin"
		case "ml", "mli":
			return "ocaml"
		case "c":
			return "c"
		case "cpp", "cc", "cxx":
			return "cpp"
		case "java":
			return "java"
		}
	}
	return "plaintext"
}

// ReopenDocument closes (didClose without removing metadata), re-reads from disk, re-opens.
func (c *LSPClient) ReopenDocument(ctx context.Context, uri string) error {
	c.mu.Lock()
	meta, ok := c.openDocs[uri]
	c.mu.Unlock()
	if !ok {
		// URI not tracked — attempt to open from disk, mirroring TypeScript behavior.
		logging.Log(logging.LevelDebug, "ReopenDocument: URI not tracked, attempting disk open: "+uri)
		filePath := uripkg.URIToPath(uri)
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return fmt.Errorf("ReopenDocument: URI not tracked and disk read failed for %s: %w", uri, readErr)
		}
		return c.OpenDocument(ctx, uri, string(data), languageIDFromURI(uri))
	}

	// didClose without removing metadata.
	if err := c.sendNotification("textDocument/didClose", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	}); err != nil {
		return err
	}

	// Re-read from disk.
	data, err := os.ReadFile(meta.filePath)
	if err != nil {
		return fmt.Errorf("reopen read %s: %w", meta.filePath, err)
	}

	// Re-open.
	c.mu.Lock()
	meta.version++
	c.openDocs[uri] = meta
	c.mu.Unlock()

	return c.sendNotification("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": meta.languageID,
			"version":    meta.version,
			"text":       string(data),
		},
	})
}

// ReopenAllDocuments reopens all tracked open documents.
func (c *LSPClient) ReopenAllDocuments(ctx context.Context) error {
	c.mu.Lock()
	uris := make([]string, 0, len(c.openDocs))
	for uri := range c.openDocs {
		uris = append(uris, uri)
	}
	c.mu.Unlock()

	for _, uri := range uris {
		if err := c.ReopenDocument(ctx, uri); err != nil {
			return err
		}
	}
	return nil
}

// WaitForFileIndexed waits until the URI has received at least one diagnostic
// notification (or fires immediately if diagnostics are already cached for
// the URI via SubscribeToDiagnostics replay), then waits for a 1500ms quiet
// window with no further notifications. This matches the TypeScript reference: gopls runs a
// cross-package background load after the first publishDiagnostics, and the
// stability window lets that finish so cross-file references are available.
//
// If diagnostics are already cached for the URI (meaning gopls has already
// processed this file at least once), the function returns immediately. This
// prevents blocking for the full timeout on clean files in already-indexed
// workspaces, which is the common case for batch reference queries.
func (c *LSPClient) WaitForFileIndexed(ctx context.Context, uri string, timeoutMs int) error {
	const stabilityMs = 1500

	// Fast path: if diagnostics are already cached for this URI, gopls has
	// already processed the file. No need to wait for a notification that
	// already happened.
	c.diagMu.RLock()
	_, alreadyCached := c.diags[uri]
	c.diagMu.RUnlock()
	if alreadyCached {
		return nil
	}

	// stabilize is reset on every matching diagnostic notification.
	stabilize := make(chan struct{}, 1)
	cb := func(u string, _ []types.LSPDiagnostic) {
		if u == uri {
			select {
			case stabilize <- struct{}{}:
			default:
				// Drain and re-send so the timer always resets to the latest notification.
				select {
				case <-stabilize:
				default:
				}
				stabilize <- struct{}{}
			}
		}
	}
	c.SubscribeToDiagnostics(cb)
	defer c.UnsubscribeFromDiagnostics(cb)

	timeout := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	defer timeout.Stop()

	// Wait for the first notification before starting the stability window.
	select {
	case <-stabilize:
	case <-timeout.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}

	// Reset stability timer on every subsequent notification.
	stability := time.NewTimer(time.Duration(stabilityMs) * time.Millisecond)
	defer stability.Stop()
	for {
		select {
		case <-stabilize:
			if !stability.Stop() {
				select {
				case <-stability.C:
				default:
				}
			}
			stability.Reset(time.Duration(stabilityMs) * time.Millisecond)
		case <-stability.C:
			return nil
		case <-timeout.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ---- LSP Operations ----
//
// Each method below wraps a single LSP request. The pattern is consistent:
//   1. Check hasCapability (skip gracefully if server doesn't support it).
//   2. Send the JSON-RPC request via sendRequest (which handles timeouts).
//   3. Parse the response, normalizing across LSP response variants
//      (e.g. Location vs LocationLink, CompletionList vs CompletionItem[]).
//
// Methods that depend on workspace indexing (GetReferences) wait for the
// server to finish indexing before issuing the request.

// GetInfoOnLocation performs a hover request and returns the hover text.
func (c *LSPClient) GetInfoOnLocation(ctx context.Context, uri string, pos types.Position) (string, error) {
	if !c.hasCapability("hoverProvider") {
		logging.Log(logging.LevelDebug, "server does not support hover")
		return "", nil
	}
	result, err := c.sendRequest(ctx, "textDocument/hover", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return "", err
	}
	if result == nil || string(result) == "null" {
		return "", nil
	}
	var hover struct {
		Contents interface{} `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil {
		return "", err
	}
	switch v := hover.Contents.(type) {
	case string:
		return v, nil
	case map[string]interface{}:
		// MarkupContent: {kind: "markdown"|"plaintext", value: "..."}
		if val, ok := v["value"].(string); ok {
			return val, nil
		}
		return fmt.Sprintf("%v", v), nil
	case []interface{}:
		// MarkedString array: each element is string or {language, value}
		var parts []string
		for _, item := range v {
			switch s := item.(type) {
			case string:
				parts = append(parts, s)
			case map[string]interface{}:
				if val, ok := s["value"].(string); ok {
					parts = append(parts, val)
				}
			}
		}
		return strings.Join(parts, "\n"), nil
	default:
		return fmt.Sprintf("%v", hover.Contents), nil
	}
}

// GetCompletion requests completion items at a position, normalized to CompletionList.
// The LSP server may return CompletionList or CompletionItem[]; both are normalized
// to CompletionList (bare array wrapped with isIncomplete=false).
func (c *LSPClient) GetCompletion(ctx context.Context, uri string, pos types.Position) (types.CompletionList, error) {
	if !c.hasCapability("completionProvider") {
		logging.Log(logging.LevelDebug, "server does not support completion")
		return types.CompletionList{Items: []types.CompletionItem{}}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/completion", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return types.CompletionList{}, err
	}
	return NormalizeCompletion(result)
}

// GetCodeActions requests code actions, normalized to []types.CodeAction.
// The LSP server may return (Command | CodeAction)[]; bare Commands are
// synthesized into CodeAction{Title, Command: &cmd}.
func (c *LSPClient) GetCodeActions(ctx context.Context, uri string, rng types.Range) ([]types.CodeAction, error) {
	if !c.hasCapability("codeActionProvider") {
		logging.Log(logging.LevelDebug, "server does not support codeAction")
		return []types.CodeAction{}, nil
	}
	// Retrieve diagnostics that overlap the requested range.
	c.diagMu.RLock()
	allDiags := c.diags[uri]
	c.diagMu.RUnlock()
	var overlapping []types.LSPDiagnostic
	for _, d := range allDiags {
		diagStart := d.Range.Start
		diagEnd := d.Range.End
		beforeRange := diagEnd.Line < rng.Start.Line ||
			(diagEnd.Line == rng.Start.Line && diagEnd.Character < rng.Start.Character)
		afterRange := diagStart.Line > rng.End.Line ||
			(diagStart.Line == rng.End.Line && diagStart.Character > rng.End.Character)
		if !beforeRange && !afterRange {
			overlapping = append(overlapping, d)
		}
	}
	if overlapping == nil {
		overlapping = []types.LSPDiagnostic{}
	}
	result, err := c.sendRequest(ctx, "textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range":        rng,
		"context":      map[string]interface{}{"diagnostics": overlapping},
	})
	if err != nil {
		return nil, err
	}
	return NormalizeCodeActions(result)
}

// GetDefinition returns the definition location(s).
func (c *LSPClient) GetDefinition(ctx context.Context, uri string, pos types.Position) ([]types.Location, error) {
	if !c.hasCapability("definitionProvider") {
		logging.Log(logging.LevelDebug, "server does not support definition")
		return []types.Location{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	return parseLocations(result), nil
}

// GetTypeDefinition returns the type definition location(s).
func (c *LSPClient) GetTypeDefinition(ctx context.Context, uri string, pos types.Position) ([]types.Location, error) {
	if !c.hasCapability("typeDefinitionProvider") {
		logging.Log(logging.LevelDebug, "server does not support typeDefinition")
		return []types.Location{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/typeDefinition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	return parseLocations(result), nil
}

// GetImplementation returns the implementation location(s).
func (c *LSPClient) GetImplementation(ctx context.Context, uri string, pos types.Position) ([]types.Location, error) {
	if !c.hasCapability("implementationProvider") {
		logging.Log(logging.LevelDebug, "server does not support implementation")
		return []types.Location{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/implementation", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	return parseLocations(result), nil
}

// GetDeclaration returns the declaration location(s).
func (c *LSPClient) GetDeclaration(ctx context.Context, uri string, pos types.Position) ([]types.Location, error) {
	if !c.hasCapability("declarationProvider") {
		logging.Log(logging.LevelDebug, "server does not support declaration")
		return []types.Location{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/declaration", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	return parseLocations(result), nil
}

// GetReferences returns all references to the symbol at position.
// Waits for workspace to be ready before issuing the request.
func (c *LSPClient) GetReferences(ctx context.Context, uri string, pos types.Position, includeDecl bool) ([]types.Location, error) {
	if !c.hasCapability("referencesProvider") {
		logging.Log(logging.LevelDebug, "server does not support references")
		return []types.Location{}, nil
	}
	c.waitForWorkspaceReady(ctx)
	_ = c.WaitForFileIndexed(ctx, uri, 2000)
	result, err := c.sendRequest(ctx, "textDocument/references", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
		"context":      map[string]interface{}{"includeDeclaration": includeDecl},
	})
	if err != nil {
		return nil, err
	}
	return parseLocationsFlat(result), nil
}

// GetDocumentSymbols returns document symbols normalized to []types.DocumentSymbol.
// The LSP server may return DocumentSymbol[] or SymbolInformation[]; both are
// normalized to the richer DocumentSymbol shape.
func (c *LSPClient) GetDocumentSymbols(ctx context.Context, uri string) ([]types.DocumentSymbol, error) {
	if !c.hasCapability("documentSymbolProvider") {
		logging.Log(logging.LevelDebug, "server does not support documentSymbol")
		return []types.DocumentSymbol{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	return NormalizeDocumentSymbols(result)
}

// GetWorkspaceSymbols queries workspace symbols.
func (c *LSPClient) GetWorkspaceSymbols(ctx context.Context, query string) ([]types.SymbolInformation, error) {
	if !c.hasCapability("workspaceSymbolProvider") {
		logging.Log(logging.LevelDebug, "server does not support workspaceSymbol")
		return []types.SymbolInformation{}, nil
	}
	result, err := c.sendRequest(ctx, "workspace/symbol", map[string]interface{}{
		"query": query,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.SymbolInformation{}, nil
	}
	var syms []types.SymbolInformation
	if err := json.Unmarshal(result, &syms); err != nil {
		return nil, err
	}
	return syms, nil
}

// PrepareCallHierarchy resolves the call hierarchy item at a position.
// Returns a typed slice or an empty slice if unsupported.
func (c *LSPClient) PrepareCallHierarchy(ctx context.Context, uri string, pos types.Position) ([]types.CallHierarchyItem, error) {
	if !c.hasCapability("callHierarchyProvider") {
		logging.Log(logging.LevelDebug, "server does not support callHierarchy")
		return []types.CallHierarchyItem{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/prepareCallHierarchy", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.CallHierarchyItem{}, nil
	}
	var items []types.CallHierarchyItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetIncomingCalls returns all callers of the given call hierarchy item.
func (c *LSPClient) GetIncomingCalls(ctx context.Context, item types.CallHierarchyItem) ([]types.CallHierarchyIncomingCall, error) {
	result, err := c.sendRequest(ctx, "callHierarchy/incomingCalls", map[string]interface{}{
		"item": item,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.CallHierarchyIncomingCall{}, nil
	}
	var calls []types.CallHierarchyIncomingCall
	if err := json.Unmarshal(result, &calls); err != nil {
		return nil, err
	}
	return calls, nil
}

// GetOutgoingCalls returns all functions called by the given call hierarchy item.
func (c *LSPClient) GetOutgoingCalls(ctx context.Context, item types.CallHierarchyItem) ([]types.CallHierarchyOutgoingCall, error) {
	result, err := c.sendRequest(ctx, "callHierarchy/outgoingCalls", map[string]interface{}{
		"item": item,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.CallHierarchyOutgoingCall{}, nil
	}
	var calls []types.CallHierarchyOutgoingCall
	if err := json.Unmarshal(result, &calls); err != nil {
		return nil, err
	}
	return calls, nil
}

// GetInlayHints returns inlay hints (inferred type annotations and parameter
// name labels) for a range within a document.
// Returns an empty slice when the server does not support inlayHintProvider.
func (c *LSPClient) GetInlayHints(ctx context.Context, uri string, rng types.Range) ([]types.InlayHint, error) {
	if !c.hasCapability("inlayHintProvider") {
		logging.Log(logging.LevelDebug, "server does not support inlayHint")
		return []types.InlayHint{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/inlayHint", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range":        rng,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.InlayHint{}, nil
	}
	var hints []types.InlayHint
	if err := json.Unmarshal(result, &hints); err != nil {
		return nil, err
	}
	return hints, nil
}

// PrepareTypeHierarchy resolves the type hierarchy item at a position.
// Returns a typed slice or an empty slice if unsupported.
func (c *LSPClient) PrepareTypeHierarchy(ctx context.Context, uri string, pos types.Position) ([]types.TypeHierarchyItem, error) {
	if !c.hasCapability("typeHierarchyProvider") {
		logging.Log(logging.LevelDebug, "server does not support typeHierarchy")
		return []types.TypeHierarchyItem{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/prepareTypeHierarchy", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.TypeHierarchyItem{}, nil
	}
	var items []types.TypeHierarchyItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetSupertypes returns all supertypes (parent classes/interfaces) of the given type hierarchy item.
func (c *LSPClient) GetSupertypes(ctx context.Context, item types.TypeHierarchyItem) ([]types.TypeHierarchyItem, error) {
	result, err := c.sendRequest(ctx, "typeHierarchy/supertypes", map[string]interface{}{
		"item": item,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.TypeHierarchyItem{}, nil
	}
	var items []types.TypeHierarchyItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetSubtypes returns all subtypes (subclasses/implementations) of the given type hierarchy item.
func (c *LSPClient) GetSubtypes(ctx context.Context, item types.TypeHierarchyItem) ([]types.TypeHierarchyItem, error) {
	result, err := c.sendRequest(ctx, "typeHierarchy/subtypes", map[string]interface{}{
		"item": item,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.TypeHierarchyItem{}, nil
	}
	var items []types.TypeHierarchyItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetSignatureHelp returns signature help at a position.
func (c *LSPClient) GetSignatureHelp(ctx context.Context, uri string, pos types.Position) (interface{}, error) {
	if !c.hasCapability("signatureHelpProvider") {
		logging.Log(logging.LevelDebug, "server does not support signatureHelp")
		return nil, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/signatureHelp", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || string(result) == "null" {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return v, nil
}

// FormatDocument formats the entire document.
func (c *LSPClient) FormatDocument(ctx context.Context, uri string, tabSize int, insertSpaces bool) ([]types.TextEdit, error) {
	if !c.hasCapability("documentFormattingProvider") {
		logging.Log(logging.LevelDebug, "server does not support formatting")
		return []types.TextEdit{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/formatting", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"options": map[string]interface{}{
			"tabSize":      tabSize,
			"insertSpaces": insertSpaces,
		},
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.TextEdit{}, nil
	}
	var edits []types.TextEdit
	if err := json.Unmarshal(result, &edits); err != nil {
		return nil, err
	}
	return edits, nil
}

// FormatRange formats a range within the document.
func (c *LSPClient) FormatRange(ctx context.Context, uri string, rng types.Range, tabSize int, insertSpaces bool) ([]types.TextEdit, error) {
	if !c.hasCapability("documentRangeFormattingProvider") {
		logging.Log(logging.LevelDebug, "server does not support rangeFormatting")
		return []types.TextEdit{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/rangeFormatting", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range":        rng,
		"options": map[string]interface{}{
			"tabSize":      tabSize,
			"insertSpaces": insertSpaces,
		},
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.TextEdit{}, nil
	}
	var edits []types.TextEdit
	if err := json.Unmarshal(result, &edits); err != nil {
		return nil, err
	}
	return edits, nil
}

// RenameSymbol performs a rename refactor.
func (c *LSPClient) RenameSymbol(ctx context.Context, uri string, pos types.Position, newName string) (interface{}, error) {
	if !c.hasCapability("renameProvider") {
		logging.Log(logging.LevelDebug, "server does not support rename")
		return nil, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/rename", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
		"newName":      newName,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || string(result) == "null" {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return v, nil
}

// PrepareRename checks if the symbol at position can be renamed.
func (c *LSPClient) PrepareRename(ctx context.Context, uri string, pos types.Position) (interface{}, error) {
	cap := c.getCapabilityRaw("renameProvider")
	switch v := cap.(type) {
	case map[string]interface{}:
		if pp, ok := v["prepareProvider"].(bool); !ok || !pp {
			logging.Log(logging.LevelDebug, "server does not support prepareRename")
			return nil, nil
		}
	case bool:
		// renameProvider: true means rename is supported but no prepareProvider declared.
		logging.Log(logging.LevelDebug, "server does not support prepareRename (renameProvider is bool, no options object)")
		return nil, nil
	case nil:
		logging.Log(logging.LevelDebug, "server does not support rename/prepareRename")
		return nil, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/prepareRename", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || string(result) == "null" {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return v, nil
}

// ExecuteCommand runs a workspace command.
func (c *LSPClient) ExecuteCommand(ctx context.Context, command string, args []interface{}) (interface{}, error) {
	result, err := c.sendRequest(ctx, "workspace/executeCommand", map[string]interface{}{
		"command":   command,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || string(result) == "null" {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return v, nil
}

// DidChangeWatchedFiles notifies the server of watched file changes.
func (c *LSPClient) DidChangeWatchedFiles(changes []types.FileChangeEvent) error {
	items := make([]map[string]interface{}, len(changes))
	for i, ch := range changes {
		items[i] = map[string]interface{}{
			"uri":  ch.URI,
			"type": ch.Type,
		}
	}
	return c.sendNotification("workspace/didChangeWatchedFiles", map[string]interface{}{
		"changes": items,
	})
}

// applyDocumentChanges handles the documentChanges branch of a WorkspaceEdit.
// documentChanges is (TextDocumentEdit | CreateFile | RenameFile | DeleteFile)[].
// Each entry is discriminated by the presence of a "kind" field.
func (c *LSPClient) applyDocumentChanges(ctx context.Context, dc interface{}) error {
	b, _ := json.Marshal(dc)
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("applyDocumentChanges: documentChanges is not a JSON array: %w", err)
	}
	for _, entry := range raw {
		var disc struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(entry, &disc); err != nil {
			logging.Log(logging.LevelDebug, fmt.Sprintf("applyDocumentChanges: unmarshal discriminator: %v", err))
			continue
		}
		switch disc.Kind {
		case "create":
			var op struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(entry, &op); err == nil && op.URI != "" {
				path := uripkg.URIToPath(op.URI)
				if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
					if writeErr := os.WriteFile(path, []byte{}, 0644); writeErr != nil {
						return fmt.Errorf("applyDocumentChanges create %s: %w", path, writeErr)
					}
				}
			}
		case "rename":
			var op struct {
				OldURI string `json:"oldUri"`
				NewURI string `json:"newUri"`
			}
			if err := json.Unmarshal(entry, &op); err == nil {
				oldPath := uripkg.URIToPath(op.OldURI)
				newPath := uripkg.URIToPath(op.NewURI)
				if renameErr := os.Rename(oldPath, newPath); renameErr != nil {
					return fmt.Errorf("applyDocumentChanges rename %s -> %s: %w", oldPath, newPath, renameErr)
				}
			}
		case "delete":
			var op struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(entry, &op); err == nil && op.URI != "" {
				path := uripkg.URIToPath(op.URI)
				if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
					return fmt.Errorf("applyDocumentChanges delete %s: %w", path, removeErr)
				}
			}
		default:
			// TextDocumentEdit (no kind field).
			var te struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
				Edits []textEdit `json:"edits"`
			}
			if err := json.Unmarshal(entry, &te); err == nil && te.TextDocument.URI != "" {
				if err := c.applyEditsToFile(ctx, te.TextDocument.URI, te.Edits); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ApplyWorkspaceEdit applies a workspace edit received from the server or a tool.
// Supports both changes (map<uri, TextEdit[]>) and documentChanges (TextDocumentEdit[]).
func (c *LSPClient) ApplyWorkspaceEdit(ctx context.Context, edit interface{}) error {
	editMap, ok := edit.(map[string]interface{})
	if !ok {
		// Try re-marshal/unmarshal to get a map.
		b, err := json.Marshal(edit)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &editMap); err != nil {
			return fmt.Errorf("invalid workspace edit: %w", err)
		}
	}

	// Process documentChanges first if present.
	if dc, ok := editMap["documentChanges"]; ok {
		return c.applyDocumentChanges(ctx, dc)
	}

	// Fallback to changes map.
	if changes, ok := editMap["changes"]; ok {
		b, _ := json.Marshal(changes)
		var changeMap map[string][]textEdit
		if err := json.Unmarshal(b, &changeMap); err != nil {
			return err
		}
		for uri, edits := range changeMap {
			if err := c.applyEditsToFile(ctx, uri, edits); err != nil {
				return err
			}
		}
	}
	return nil
}

type textEdit struct {
	Range   types.Range `json:"range"`
	NewText string      `json:"newText"`
}

// applyEditsToFile applies text edits in reverse order to a file and sends didChange.
func (c *LSPClient) applyEditsToFile(ctx context.Context, uri string, edits []textEdit) error {
	path := uripkg.URIToPath(uri)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("applyEdit read %s: %w", path, err)
	}

	// Apply edits in reverse order (bottom-to-top) using the canonical
	// ApplyRangeEdit from internal/uri (L5 deduplication).
	content := string(data)
	for i := len(edits) - 1; i >= 0; i-- {
		content = uripkg.ApplyRangeEdit(content, edits[i].Range, edits[i].NewText)
	}

	newContent := content
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("applyEdit write %s: %w", path, err)
	}

	// Send didChange with the incremented version number.
	c.mu.Lock()
	version := 1
	if meta, ok := c.openDocs[uri]; ok {
		meta.version++
		c.openDocs[uri] = meta
		version = meta.version
	}
	c.mu.Unlock()

	return c.sendNotification("textDocument/didChange", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":     uri,
			"version": version,
		},
		"contentChanges": []map[string]interface{}{
			{"text": newContent},
		},
	})
}

// ---- Capability Helpers ----

func (c *LSPClient) hasCapability(key string) bool {
	c.capsMu.RLock()
	defer c.capsMu.RUnlock()
	v, ok := c.capabilities[key]
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return v != nil
}

func (c *LSPClient) getCapabilityRaw(key string) interface{} {
	c.capsMu.RLock()
	defer c.capsMu.RUnlock()
	return c.capabilities[key]
}

// GetCapabilities returns a shallow copy of the server's capability map.
// The map reflects what the server advertised in the initialize response.
func (c *LSPClient) GetCapabilities() map[string]interface{} {
	c.capsMu.RLock()
	defer c.capsMu.RUnlock()
	out := make(map[string]interface{}, len(c.capabilities))
	for k, v := range c.capabilities {
		out[k] = v
	}
	return out
}

// GetServerInfo returns the server name and version from the initialize response.
// Both fields may be empty if the server did not advertise serverInfo.
func (c *LSPClient) GetServerInfo() (name, version string) {
	c.capsMu.RLock()
	defer c.capsMu.RUnlock()
	return c.serverName, c.serverVersion
}

// workspaceFolder is a single LSP workspace folder entry.
type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// GetWorkspaceFolders returns the current list of workspace folders.
func (c *LSPClient) GetWorkspaceFolders() []workspaceFolder {
	c.capsMu.RLock()
	defer c.capsMu.RUnlock()
	out := make([]workspaceFolder, len(c.workspaceFolders))
	copy(out, c.workspaceFolders)
	return out
}

// AddWorkspaceFolder adds a directory to the workspace via
// workspace/didChangeWorkspaceFolders. The LSP server re-indexes the new root
// and cross-folder references become available immediately.
func (c *LSPClient) AddWorkspaceFolder(ctx context.Context, path string) error {
	folderURI := (&url.URL{Scheme: "file", Path: path}).String()
	folder := workspaceFolder{URI: folderURI, Name: path}

	c.capsMu.Lock()
	for _, f := range c.workspaceFolders {
		if f.URI == folderURI {
			c.capsMu.Unlock()
			return nil // already present
		}
	}
	c.workspaceFolders = append(c.workspaceFolders, folder)
	c.capsMu.Unlock()

	if err := c.sendNotification("workspace/didChangeWorkspaceFolders", map[string]interface{}{
		"event": map[string]interface{}{
			"added":   []workspaceFolder{folder},
			"removed": []workspaceFolder{},
		},
	}); err != nil {
		return err
	}

	// Extend the auto-watcher to cover the new folder without restarting it.
	// C1: addWatcherRoot adds the new path to the existing watcher goroutine
	// so the original workspace root remains watched.
	c.addWatcherRoot(path)
	return nil
}

// RemoveWorkspaceFolder removes a directory from the workspace via
// workspace/didChangeWorkspaceFolders.
func (c *LSPClient) RemoveWorkspaceFolder(ctx context.Context, path string) error {
	folderURI := (&url.URL{Scheme: "file", Path: path}).String()
	folder := workspaceFolder{URI: folderURI, Name: path}

	c.capsMu.Lock()
	found := false
	updated := c.workspaceFolders[:0]
	for _, f := range c.workspaceFolders {
		if f.URI == folderURI {
			found = true
		} else {
			updated = append(updated, f)
		}
	}
	if !found {
		c.capsMu.Unlock()
		return nil // not present, nothing to do
	}
	c.workspaceFolders = updated
	c.capsMu.Unlock()

	return c.sendNotification("workspace/didChangeWorkspaceFolders", map[string]interface{}{
		"event": map[string]interface{}{
			"added":   []workspaceFolder{},
			"removed": []workspaceFolder{folder},
		},
	})
}

// watcherSkipDirs are directory names that the auto-watcher skips entirely.
// These directories change frequently but are not part of the source index.
var watcherSkipDirs = map[string]bool{
	".git":        true,
	"node_modules": true,
	"target":      true,
	"build":       true,
	"dist":        true,
	"vendor":      true,
	"__pycache__": true,
	".venv":       true,
	"venv":        true,
	".next":       true,
	".cache":      true,
	".idea":       true,
	".vscode":     true,
}

// GetDocumentHighlights returns all occurrences of the symbol at the given
// position within the same document. Each highlight includes a range and an
// optional kind (Text=1, Read=2, Write=3). Returns an empty slice when the
// server does not support documentHighlightProvider.
func (c *LSPClient) GetDocumentHighlights(ctx context.Context, uri string, pos types.Position) ([]types.DocumentHighlight, error) {
	if !c.hasCapability("documentHighlightProvider") {
		logging.Log(logging.LevelDebug, "server does not support documentHighlight")
		return []types.DocumentHighlight{}, nil
	}
	result, err := c.sendRequest(ctx, "textDocument/documentHighlight", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     pos,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.DocumentHighlight{}, nil
	}
	var highlights []types.DocumentHighlight
	if err := json.Unmarshal(result, &highlights); err != nil {
		return nil, err
	}
	return highlights, nil
}

// startWatcher starts an auto-watcher on rootDir that notifies the LSP server
// whenever files change on disk — keeping the index fresh without manual
// did_change_watched_files calls. Uses a 150ms debounce to batch rapid edits.
// A previous watcher (if any) is stopped first.
func (c *LSPClient) startWatcher(rootDir string) {
	c.watcherMu.Lock()
	c.stopWatcherLocked()
	stop := make(chan struct{})
	c.watcherStop = stop
	c.watcherMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Log(logging.LevelError, fmt.Sprintf("startWatcher panic: %v\n%s", r, debug.Stack()))
			}
		}()
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			logging.Log(logging.LevelDebug, "auto-watcher: failed to create watcher: "+err.Error())
			return
		}
		// Register Close first so nil-assignment defer (registered after) runs first (LIFO).
		defer watcher.Close()
		c.watcherMu.Lock()
		c.watcher = watcher
		c.watcherMu.Unlock()
		defer func() {
			c.watcherMu.Lock()
			c.watcher = nil
			c.watcherMu.Unlock()
		}()

		// Walk the workspace and add all non-excluded directories.
		_ = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if watcherSkipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
				return filepath.SkipDir
			}
			_ = watcher.Add(path)
			return nil
		})

		// debounce: collect events for 150ms then flush as a batch.
		const debounce = 150 * time.Millisecond
		pending := make(map[string]fsnotify.Op)
		var timer *time.Timer
		flush := func() {
			if len(pending) == 0 {
				return
			}
			changes := make([]types.FileChangeEvent, 0, len(pending))
			for path, op := range pending {
				changeType := 2 // Changed
				if op&fsnotify.Create != 0 {
					changeType = 1 // Created
				} else if op&(fsnotify.Remove|fsnotify.Rename) != 0 {
					changeType = 3 // Deleted
				}
				changes = append(changes, types.FileChangeEvent{
					URI:  "file://" + path,
					Type: changeType,
				})
			}
			pending = make(map[string]fsnotify.Op)
			if err := c.DidChangeWatchedFiles(changes); err != nil {
				logging.Log(logging.LevelDebug, "auto-watcher: didChangeWatchedFiles error: "+err.Error())
			}
		}

		for {
			select {
			case <-stop:
				if timer != nil {
					timer.Stop()
				}
				flush()
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Skip directory events and hidden files.
				name := filepath.Base(event.Name)
				if strings.HasPrefix(name, ".") {
					continue
				}
				pending[event.Name] = pending[event.Name] | event.Op
				// If a new directory was created, add it to the watcher.
				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						if !watcherSkipDirs[name] {
							_ = watcher.Add(event.Name)
						}
					}
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(debounce, flush)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logging.Log(logging.LevelDebug, "auto-watcher error: "+err.Error())
			}
		}
	}()
}

// stopWatcher stops the auto-watcher if one is running.
func (c *LSPClient) stopWatcher() {
	c.watcherMu.Lock()
	c.stopWatcherLocked()
	c.watcherMu.Unlock()
}

// stopWatcherLocked is the lock-free inner body of stopWatcher.
// Caller must hold c.watcherMu.
func (c *LSPClient) stopWatcherLocked() {
	if c.watcherStop != nil {
		close(c.watcherStop)
		c.watcherStop = nil
	}
}

// addWatcherRoot adds path and its subdirectories to the existing running
// watcher. It is a no-op if no watcher is running (c.watcher == nil).
// This allows AddWorkspaceFolder to extend coverage without restarting
// the watcher and losing the original workspace root.
func (c *LSPClient) addWatcherRoot(path string) {
	c.watcherMu.Lock()
	w := c.watcher
	c.watcherMu.Unlock()
	if w == nil {
		return
	}
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if watcherSkipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
			return filepath.SkipDir
		}
		_ = w.Add(p)
		return nil
	})
}

// GetSemanticTokenLegend returns the token type and modifier name arrays
// captured from the initialize response. Returns nil slices if the server
// did not advertise semanticTokensProvider.
func (c *LSPClient) GetSemanticTokenLegend() (tokenTypes []string, tokenModifiers []string) {
	c.legendMu.RLock()
	defer c.legendMu.RUnlock()
	return c.legendTypes, c.legendModifiers
}

// GetSemanticTokens sends textDocument/semanticTokens/range for the given range.
// Falls back to textDocument/semanticTokens/full when range is unsupported.
// Returns decoded tokens with absolute 1-based positions and human-readable
// type/modifier names resolved from the legend captured during Initialize.
func (c *LSPClient) GetSemanticTokens(ctx context.Context, uri string, rng types.Range) ([]types.SemanticToken, error) {
	// Check capability: semanticTokensProvider may be bool, object, or absent.
	cap := c.getCapabilityRaw("semanticTokensProvider")
	if cap == nil {
		logging.Log(logging.LevelDebug, "server does not support semanticTokens")
		return []types.SemanticToken{}, nil
	}

	c.legendMu.RLock()
	tokenTypes := make([]string, len(c.legendTypes))
	copy(tokenTypes, c.legendTypes)
	tokenModifiers := make([]string, len(c.legendModifiers))
	copy(tokenModifiers, c.legendModifiers)
	c.legendMu.RUnlock()

	// Try range request first; fall back to full if not supported.
	useRange := false
	switch v := cap.(type) {
	case map[string]interface{}:
		if req, ok := v["requests"].(map[string]interface{}); ok {
			_, useRange = req["range"]
		}
	case bool:
		// bool capability: full is implied, range may not be.
		useRange = false
	}

	var raw json.RawMessage
	var err error
	if useRange {
		raw, err = c.sendRequest(ctx, "textDocument/semanticTokens/range", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri},
			"range":        rng,
		})
	} else {
		raw, err = c.sendRequest(ctx, "textDocument/semanticTokens/full", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri},
		})
	}
	if err != nil {
		return nil, err
	}
	if raw == nil || string(raw) == "null" {
		return []types.SemanticToken{}, nil
	}

	var result struct {
		Data []int `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal semanticTokens response: %w", err)
	}

	return decodeSemanticTokens(result.Data, tokenTypes, tokenModifiers), nil
}

// decodeSemanticTokens converts the flat delta-encoded int array from LSP into
// absolute-position SemanticToken values. The LSP spec encodes tokens as a
// flat []int with 5 integers per token:
//   [deltaLine, deltaStartChar, length, tokenTypeIndex, tokenModifierBitmask]
// Positions are delta-encoded: deltaLine is relative to previous token's line;
// deltaStartChar is relative to previous token's startChar on the SAME line
// (resets to absolute when line changes).
func decodeSemanticTokens(data []int, tokenTypes []string, tokenModifiers []string) []types.SemanticToken {
	tokens := make([]types.SemanticToken, 0, len(data)/5)
	prevLine := 0
	prevChar := 0
	for i := 0; i+4 < len(data); i += 5 {
		deltaLine := data[i]
		deltaChar := data[i+1]
		length := data[i+2]
		typeIdx := data[i+3]
		modBitmask := data[i+4]

		if deltaLine > 0 {
			prevLine += deltaLine
			prevChar = deltaChar
		} else {
			prevChar += deltaChar
		}

		tokenType := ""
		if typeIdx >= 0 && typeIdx < len(tokenTypes) {
			tokenType = tokenTypes[typeIdx]
		}

		var modifiers []string
		for bit := 0; bit < len(tokenModifiers); bit++ {
			if modBitmask&(1<<bit) != 0 {
				modifiers = append(modifiers, tokenModifiers[bit])
			}
		}
		if modifiers == nil {
			modifiers = []string{}
		}

		tokens = append(tokens, types.SemanticToken{
			Line:      prevLine + 1,
			Character: prevChar + 1,
			Length:    length,
			TokenType: tokenType,
			Modifiers: modifiers,
		})
	}
	return tokens
}

// ---- Parse Helpers ----
//
// LSP location responses come in multiple shapes depending on the server
// and method: a single Location, a []Location, or a []LocationLink (which
// has targetUri/targetRange instead of uri/range). The parsers below
// normalize all variants into []types.Location for a consistent API.

// parseLocations parses an LSP response that can be a Location, []Location, or []LocationLink.
func parseLocations(raw json.RawMessage) []types.Location {
	if raw == nil || string(raw) == "null" {
		return []types.Location{}
	}

	// Try array first.
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		return parseLocationItems(items)
	}

	// Single location object.
	var loc locationOrLink
	if err := json.Unmarshal(raw, &loc); err == nil {
		if l := loc.toLocation(); l != nil {
			return []types.Location{*l}
		}
	}
	return []types.Location{}
}

// parseLocationsFlat parses a flat []Location array (e.g. references response).
func parseLocationsFlat(raw json.RawMessage) []types.Location {
	if raw == nil || string(raw) == "null" {
		return []types.Location{}
	}
	var locs []types.Location
	if err := json.Unmarshal(raw, &locs); err != nil {
		return []types.Location{}
	}
	return locs
}

func parseLocationItems(items []json.RawMessage) []types.Location {
	out := make([]types.Location, 0, len(items))
	for _, item := range items {
		var loc locationOrLink
		if err := json.Unmarshal(item, &loc); err == nil {
			if l := loc.toLocation(); l != nil {
				out = append(out, *l)
			}
		}
	}
	return out
}

// locationOrLink handles both Location and LocationLink shapes.
type locationOrLink struct {
	// Location fields
	URI   string      `json:"uri"`
	Range types.Range `json:"range"`
	// LocationLink fields
	TargetURI   string      `json:"targetUri"`
	TargetRange types.Range `json:"targetRange"`
}

func (l *locationOrLink) toLocation() *types.Location {
	if l.TargetURI != "" {
		return &types.Location{URI: l.TargetURI, Range: l.TargetRange}
	}
	if l.URI != "" {
		return &types.Location{URI: l.URI, Range: l.Range}
	}
	return nil
}

// removeEnv returns a copy of env with all entries for the given key removed.
func removeEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}



