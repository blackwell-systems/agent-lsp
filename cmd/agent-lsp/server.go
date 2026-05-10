// server.go creates and configures the MCP server. It is the bridge between
// the MCP protocol (tools, resources, transports) and the internal packages
// that implement language intelligence.
//
// The Run function:
//   1. Wraps the LSP ClientResolver in a clientState for thread-safe access.
//   2. Creates shared dependencies (session manager, audit logger, phase tracker).
//   3. Registers all MCP tools via register*Tools functions (tools_*.go files).
//   4. Registers MCP resources (diagnostics://, hover://, completions://).
//   5. Starts the transport: stdio (default) or HTTP with bearer-token auth.
//
// Key abstractions:
//   - toolDeps: bundles all dependencies passed to tool registration functions.
//   - addToolWithPhaseCheck: generic wrapper that enforces skill phase permissions
//     before every tool handler, without modifying individual handlers.
//   - clientForFile / autoInitClient: multi-layered client resolution that handles
//     single-server, multi-server, and auto-initialization from file paths.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/internal/extensions"
	"github.com/blackwell-systems/agent-lsp/internal/httpauth"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/notify"
	"github.com/blackwell-systems/agent-lsp/internal/phase"
	"github.com/blackwell-systems/agent-lsp/internal/resources"
	"github.com/blackwell-systems/agent-lsp/internal/session"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpSessionSender adapts *mcp.ServerSession to the logging.logSender interface,
// which requires LogMessage(level, logger, message string) error. Called by
// logging.SetServer once the client session sends notifications/initialized.
type mcpSessionSender struct{ ss *mcp.ServerSession }

func (s *mcpSessionSender) LogMessage(level, logger, message string) error {
	// H4: LogMessage has no context parameter (interface constraint from
	// logging.SetServer). context.Background() is intentional: log sends
	// during shutdown may proceed after the main ctx is cancelled, which is
	// acceptable for best-effort diagnostic output.
	data, err := json.Marshal(message)
	if err != nil {
		// Fallback: encode an error description rather than sending JSON null.
		data, _ = json.Marshal(fmt.Sprintf("[marshal error: %v] %s", err, message))
	}
	return s.ss.Log(context.Background(), &mcp.LoggingMessageParams{
		Level:  mcp.LoggingLevel(level),
		Logger: logger,
		Data:   json.RawMessage(data),
	})
}

// clientState holds the current LSP client reference, guarded by a mutex.
type clientState struct {
	mu     sync.RWMutex
	client *lsp.LSPClient
}

func (s *clientState) get() *lsp.LSPClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *clientState) set(c *lsp.LSPClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = c
}

// csResolver wraps clientState + the original resolver to implement lsp.ClientResolver.
// DefaultClient falls back to cs.get() so start_lsp updates are visible.
// ClientForFile delegates to the real resolver for correct multi-server routing
// (e.g. gopls for .go files, clangd for .c files).
type csResolver struct {
	cs       *clientState
	delegate lsp.ClientResolver
}

func (r *csResolver) DefaultClient() *lsp.LSPClient {
	if c := r.cs.get(); c != nil {
		return c
	}
	return r.delegate.DefaultClient()
}
func (r *csResolver) ClientForFile(path string) *lsp.LSPClient {
	// Delegate to the real resolver for file-based routing (gopls for .go, etc).
	// After start_lsp calls StartAll, all delegate clients are initialized.
	return r.delegate.ClientForFile(path)
}
func (r *csResolver) AllClients() []*lsp.LSPClient       { return r.delegate.AllClients() }
func (r *csResolver) Shutdown(ctx context.Context) error { return r.delegate.Shutdown(ctx) }

// toolArgsToMap converts a typed args struct to map[string]interface{} via JSON round-trip.
func toolArgsToMap(v interface{}) map[string]interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		return map[string]interface{}{}
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(data, &m); err != nil {
		// This should not happen in practice (Marshal produced valid JSON).
		// Return empty map so callers receive field-required errors, not panics.
		logging.Log(logging.LevelDebug, fmt.Sprintf("toolArgsToMap: unmarshal error: %v", err))
		return map[string]interface{}{}
	}
	return m
}

// addToolWithPhaseCheck wraps mcp.AddTool to insert a phase enforcement check
// before every tool handler. If a skill is active and the tool call violates the
// current phase's permissions, the check returns an error result without invoking
// the handler. When no skill is active, the check is a no-op.
//
// It also pre-generates a fixed JSON Schema for the tool's input type, collapsing
// nullable array types ("type": ["null","array"]) into plain arrays ("type": "array").
// This ensures compatibility with strict OpenAPI 3.0 clients like Gemini.
func addToolWithPhaseCheck[T any](d toolDeps, tool *mcp.Tool, handler func(ctx context.Context, req *mcp.CallToolRequest, args T) (*mcp.CallToolResult, any, error)) {
	// Pre-generate and fix the schema so mcp.AddTool uses it directly
	// (it skips generation when InputSchema is non-nil).
	if tool.InputSchema == nil {
		tool.InputSchema = generateFixedSchema[T]()
	}
	toolName := tool.Name
	mcp.AddTool(d.server, tool, func(ctx context.Context, req *mcp.CallToolRequest, args T) (*mcp.CallToolResult, any, error) {
		if result := checkPhasePermission(d.phaseTracker, toolName); result != nil {
			return result, nil, nil
		}
		start := time.Now()
		result, structured, err := handler(ctx, req, args)
		elapsed := time.Since(start)

		// Log latency for every tool call. Slow calls (>5s) get a warning.
		if elapsed > 5*time.Second {
			logging.Log(logging.LevelWarning, fmt.Sprintf("tool %s: %s (slow)", toolName, elapsed.Round(time.Millisecond)))
		} else {
			logging.Log(logging.LevelDebug, fmt.Sprintf("tool %s: %s", toolName, elapsed.Round(time.Millisecond)))
		}

		return result, structured, err
	})
}

// makeCallToolResult converts a types.ToolResult to *mcp.CallToolResult.
func makeCallToolResult(r interface{}) *mcp.CallToolResult {
	data, err := json.Marshal(r)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "internal error: " + err.Error()}},
			IsError: true,
		}
	}

	var tr struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(data, &tr); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "internal error: " + err.Error()}},
			IsError: true,
		}
	}

	content := make([]mcp.Content, 0, len(tr.Content))
	for _, c := range tr.Content {
		content = append(content, &mcp.TextContent{Text: c.Text})
	}
	return &mcp.CallToolResult{
		Content: content,
		IsError: tr.IsError,
	}
}

// clientForFile returns the LSP client for the given file path.
// Prefers cs.get() when initialized — cs is updated by start_lsp and always
// reflects the most recently initialized client. Falls back to resolver.ClientForFile
// for extension-based routing (multi-server mode after start_lsp has run).
func clientForFile(resolver lsp.ClientResolver, cs *clientState, filePath string) *lsp.LSPClient {
	// cs.get() is the source of truth: start_lsp sets it via cs.set(c) after
	// successful Initialize. In single-server mode this is the only initialized
	// client. In multi-server mode it is set to DefaultClient() after StartAll.
	if c := cs.get(); c != nil && c.IsInitialized() {
		return c
	}
	// cs has no initialized client yet — try extension-based routing for
	// multi-server mode where individual clients may have been started.
	if filePath != "" {
		if c := resolver.ClientForFile(filePath); c != nil && c.IsInitialized() {
			return c
		}
	}
	// Return whatever cs has (may be nil or uninitialized — caller's
	// CheckInitialized will surface the error to the user).
	return cs.get()
}

// autoInitClient attempts to infer a workspace root from filePath and
// initialize the resolver. Safe to call concurrently via initMu.
// Returns nil if filePath is empty, inference returns no root,
// or if the file is already within the current workspace root.
func autoInitClient(
	ctx context.Context,
	resolver lsp.ClientResolver,
	cs *clientState,
	initMu *sync.Mutex,
	filePath string,
) *lsp.LSPClient {
	if filePath == "" {
		return nil
	}

	// Check if client is already initialized and file is within its root.
	if existing := cs.get(); existing != nil {
		rootDir := existing.RootDir()
		if rootDir != "" && strings.HasPrefix(filePath, rootDir+"/") {
			return existing
		}
	}

	root, _, err := config.InferWorkspaceRoot(filePath)
	if err != nil || root == "" {
		return nil
	}

	initMu.Lock()
	defer initMu.Unlock()

	// Re-check after acquiring lock (double-checked locking pattern).
	if existing := cs.get(); existing != nil {
		existingRoot := existing.RootDir()
		if existingRoot != "" && strings.HasPrefix(filePath, existingRoot+"/") {
			return existing
		}
	}

	logging.Log(logging.LevelInfo, fmt.Sprintf(
		"auto-init: inferred workspace root %q for file %q", root, filePath))

	if sm, ok := resolver.(*lsp.ServerManager); ok {
		if err := sm.StartAll(ctx, root); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("auto-init StartAll failed: %v", err))
			return nil
		}
		if c := resolver.DefaultClient(); c != nil {
			cs.set(c)
			return c
		}
		return nil
	}

	// Single-server fallback: not supported via autoInitClient because
	// serverPath/serverArgs are not in scope here. Return nil to fall
	// through to the existing "LSP not initialized" error.
	return nil
}

// toolDeps bundles the shared dependencies passed to each tool registration function.
type toolDeps struct {
	server                   *mcp.Server
	cs                       *clientState
	resolver                 lsp.ClientResolver
	clientForFileWithAutoInit func(string) *lsp.LSPClient
	sessionMgr               *session.SessionManager
	serverPath               string
	serverArgs               []string
	auditLogger              *audit.Logger
	phaseTracker             *phase.Tracker
	notifyHub                *notify.Hub
}

// Run creates and starts the MCP server.
func Run(ctx context.Context, resolver lsp.ClientResolver, registry *extensions.ExtensionRegistry, serverPath string, serverArgs []string, httpMode bool, httpPort int, httpToken string, httpListenAddr string, httpNoAuth bool, auditLogPath string) error {
	cs := &clientState{client: resolver.DefaultClient()}
	var initMu sync.Mutex
	// clientForFileWithAutoInit extends clientForFile with auto-init behavior.
	// If the resolver returns no client for filePath, attempt auto-initialization.
	clientForFileWithAutoInit := func(filePath string) *lsp.LSPClient {
		if c := clientForFile(resolver, cs, filePath); c != nil {
			return c
		}
		return autoInitClient(ctx, resolver, cs, &initMu, filePath)
	}
	sessionMgr := session.NewSessionManager(&csResolver{cs: cs, delegate: resolver})

	auditLogger, err := audit.NewLogger(audit.ResolvePath(auditLogPath), 256)
	if err != nil {
		return fmt.Errorf("audit logger: %w", err)
	}
	defer auditLogger.Close()

	notifyHub := setupNotificationHub()
	defer notifyHub.Close()

	var server *mcp.Server
	server = mcp.NewServer(&mcp.Implementation{
		Name:    "agent-lsp",
		Version: Version,
	}, &mcp.ServerOptions{
		// Wire MCP log notifications once the client session initializes.
		InitializedHandler: func(_ context.Context, req *mcp.InitializedRequest) {
			logging.SetServer(&mcpSessionSender{ss: req.Session})
			logging.MarkServerInitialized()
			notifyHub.SetSender(&mcpNotifySender{ss: req.Session, server: server})
			if c := cs.get(); c != nil {
				wireNotificationsToClient(notifyHub, c)
			}
		},
	})

	phaseTracker := phase.NewTracker(phase.BuiltinSkills(), auditLogger)

	deps := toolDeps{
		server:                   server,
		cs:                       cs,
		resolver:                 resolver,
		clientForFileWithAutoInit: clientForFileWithAutoInit,
		sessionMgr:               sessionMgr,
		serverPath:               serverPath,
		serverArgs:               serverArgs,
		auditLogger:              auditLogger,
		phaseTracker:             phaseTracker,
		notifyHub:                notifyHub,
	}

	registerWorkspaceTools(deps)
	registerNavigationTools(deps)
	registerAnalysisTools(deps)
	registerSessionTools(deps)
	registerPhaseTools(deps)

	// ------- Register prompts (skills as MCP prompts) -------
	registerPrompts(server)

	// ------- Register resources -------

	server.AddResource(&mcp.Resource{
		URI:         "lsp-diagnostics://",
		Name:        "All Diagnostics",
		Description: "LSP diagnostics for all open documents",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := cs.get()
		if client == nil {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      req.Params.URI,
					MIMEType: "application/json",
					Text:     "{}",
				}},
			}, nil
		}
		result, err := resources.HandleDiagnosticsResource(ctx, client, req.Params.URI)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      result.URI,
				MIMEType: result.MIMEType,
				Text:     result.Text,
			}},
		}, nil
	})

	server.AddResource(&mcp.Resource{
		URI:         "lsp-hover://",
		Name:        "LSP Hover",
		Description: "LSP hover information. URI format: lsp-hover:///path/to/file?line=N&column=N&language_id=X",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := cs.get()
		if client == nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		uri := req.Params.URI
		if !strings.HasPrefix(uri, "lsp-hover://") {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		result, err := resources.HandleHoverResource(ctx, client, uri)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      result.URI,
				MIMEType: result.MIMEType,
				Text:     result.Text,
			}},
		}, nil
	})

	server.AddResource(&mcp.Resource{
		URI:         "lsp-completions://",
		Name:        "LSP Completions",
		Description: "LSP completions. URI format: lsp-completions:///path/to/file?line=N&column=N&language_id=X",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := cs.get()
		if client == nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		uri := req.Params.URI
		if !strings.HasPrefix(uri, "lsp-completions://") {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		result, err := resources.HandleCompletionsResource(ctx, client, uri)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      result.URI,
				MIMEType: result.MIMEType,
				Text:     result.Text,
			}},
		}, nil
	})

	// Register URI templates for dynamic resource discovery.
	for _, tmpl := range resources.ResourceTemplates() {
		t := tmpl // capture loop variable
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			Name:        t.Name,
			URITemplate: t.URITemplate,
			Description: t.Description,
		}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		})
	}

	// Subscribe to diagnostic updates for logging purposes (all managed clients).
	for _, c := range resolver.AllClients() {
		if c != nil {
			c.SubscribeToDiagnostics(func(uri string, _ []types.LSPDiagnostic) {
				logging.Log(logging.LevelDebug, "diagnostics updated for: "+uri)
			})
		}
	}

	logging.Log(logging.LevelInfo, "agent-lsp server starting")

	if httpMode && httpToken == "" && !httpNoAuth {
		return fmt.Errorf("HTTP mode requires an auth token; set AGENT_LSP_TOKEN environment variable\n(to intentionally run without auth, pass --no-auth)")
	}
	if httpMode && httpToken == "" && httpNoAuth {
		// Reject --no-auth on non-loopback addresses — unauthenticated exposure
		// to the network is not allowed even with explicit opt-in.
		if ip := net.ParseIP(httpListenAddr); ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("--no-auth is only permitted with a loopback bind address (127.0.0.1); use --listen-addr 127.0.0.1 or set AGENT_LSP_TOKEN")
		}
		// Write directly to stderr — logging subsystem may not be initialized yet.
		fmt.Fprintln(os.Stderr, "WARNING: agent-lsp HTTP mode running without authentication — all requests accepted")
		logging.Log(logging.LevelWarning, "HTTP mode active with no auth token (--no-auth) — all requests accepted without authentication")
	}

	if httpMode {
		addr := fmt.Sprintf("%s:%d", httpListenAddr, httpPort)
		return RunHTTP(ctx, server, addr, httpToken)
	}
	transport := &mcp.StdioTransport{}
	return server.Run(ctx, transport)
}

// securityHeaders adds X-Content-Type-Options and Cache-Control headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// maxBodyHandler limits request body size to prevent memory exhaustion.
const maxRequestBodyBytes = 4 * 1024 * 1024 // 4 MB

type maxBodyHandler struct{ next http.Handler }

func (h maxBodyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	h.next.ServeHTTP(w, r)
}

// RunHTTP starts the MCP server over HTTP using the go-sdk's StreamableHTTPHandler.
// addr is "host:port". token is the Bearer token required by clients (empty = no auth).
func RunHTTP(ctx context.Context, server *mcp.Server, addr string, token string) error {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)

	// /health is unauthenticated — required for container orchestration probes.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.Handle("/", httpauth.BearerTokenMiddleware(token, maxBodyHandler{mcpHandler}))
	wrapped := securityHeaders(mux)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           wrapped,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("http listen %s: %w", addr, err)
	}
	logging.Log(logging.LevelInfo, fmt.Sprintf("agent-lsp HTTP server listening on %s", ln.Addr().String()))
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
