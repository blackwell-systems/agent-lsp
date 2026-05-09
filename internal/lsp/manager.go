package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// managedEntry holds one language server along with its routing metadata.
type managedEntry struct {
	client     *LSPClient
	extensions map[string]bool // lowercase, no dot; e.g. "go", "ts", "tsx"
	languageID string
	// preserved for StartAll
	command []string
}

// ServerManager implements ClientResolver and manages one or more LSP server
// instances. In single-server mode every file routes to the same client.
// In multi-server mode routing is based on file extension.
type ServerManager struct {
	mu      sync.RWMutex
	entries []*managedEntry
}

// NewSingleServerManager wraps a single *LSPClient to satisfy ClientResolver.
// Used for the legacy single-server invocation mode. The resulting manager
// has empty extensions, so ClientForFile always falls back to DefaultClient.
func NewSingleServerManager(client *LSPClient) *ServerManager {
	return &ServerManager{
		entries: []*managedEntry{
			{
				client:     client,
				extensions: map[string]bool{},
				languageID: "",
				command:    nil,
			},
		},
	}
}

// NewMultiServerManager creates a ServerManager from multiple ServerEntry
// configs. Does NOT start servers — deferred to StartAll.
func NewMultiServerManager(entries []config.ServerEntry) *ServerManager {
	managed := make([]*managedEntry, 0, len(entries))
	for _, e := range entries {
		exts := make(map[string]bool, len(e.Extensions))
		for _, ext := range e.Extensions {
			// Lowercase and strip leading dot if present.
			ext = strings.ToLower(ext)
			ext = strings.TrimPrefix(ext, ".")
			exts[ext] = true
		}

		langID := e.LanguageID
		if langID == "" {
			langID = inferLanguageID(e)
		}

		managed = append(managed, &managedEntry{
			client:     nil,
			extensions: exts,
			languageID: langID,
			command:    e.Command,
		})
	}
	return &ServerManager{entries: managed}
}

// StartAll starts all configured LSP servers with the given root directory.
// Called from start_lsp tool handler in multi-server mode, or from main
// after initialization.
func (m *ServerManager) StartAll(ctx context.Context, rootDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var started []*LSPClient
	for _, e := range m.entries {
		if len(e.command) == 0 {
			// Single-server mode: the client was pre-created by NewSingleServerManager
			// with serverPath/serverArgs set. Initialize it in-place.
			if e.client != nil && !e.client.IsInitialized() {
				logging.Log(logging.LevelDebug, fmt.Sprintf("ServerManager.StartAll: initializing pre-created client %s", e.client.serverPath))
				if err := e.client.Initialize(ctx, rootDir); err != nil {
					for _, c := range started {
						if shutErr := c.Shutdown(ctx); shutErr != nil {
							logging.Log(logging.LevelDebug, fmt.Sprintf("StartAll rollback shutdown: %v", shutErr))
						}
					}
					return fmt.Errorf("initialize pre-created client: %w", err)
				}
				started = append(started, e.client)
			}
			continue
		}
		client := NewLSPClient(e.command[0], e.command[1:])
		logging.Log(logging.LevelDebug, fmt.Sprintf("ServerManager.StartAll: starting %s", e.command[0]))
		if err := client.Initialize(ctx, rootDir); err != nil {
			for _, c := range started {
				if shutErr := c.Shutdown(ctx); shutErr != nil {
					logging.Log(logging.LevelDebug, fmt.Sprintf("StartAll rollback shutdown: %v", shutErr))
				}
			}
			return fmt.Errorf("initialize server %s: %w", e.command[0], err)
		}
		e.client = client
		started = append(started, client)
	}
	return nil
}

// ClientForFile satisfies ClientResolver. Routes by filepath.Ext.
// Falls back to DefaultClient if extension is not mapped.
func (m *ServerManager) ClientForFile(filePath string) *LSPClient {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, e := range m.entries {
		if ext != "" && e.extensions[ext] {
			return e.client
		}
	}
	return m.defaultClientLocked()
}

// DefaultClient returns the primary (or only) LSPClient.
// Used for tools that are not file-specific (e.g. get_workspace_symbols).
func (m *ServerManager) DefaultClient() *LSPClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultClientLocked()
}

// defaultClientLocked returns entries[0].client if len > 0, else nil.
// Caller must hold at least an RLock.
func (m *ServerManager) defaultClientLocked() *LSPClient {
	if len(m.entries) > 0 {
		return m.entries[0].client
	}
	return nil
}

// AllClients returns all non-nil clients from all entries.
func (m *ServerManager) AllClients() []*LSPClient {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*LSPClient, 0, len(m.entries))
	for _, e := range m.entries {
		if e.client != nil {
			out = append(out, e.client)
		}
	}
	return out
}

// StartForLanguage starts (or restarts) the server whose languageID or extension
// set matches languageID, initialises it at rootDir, and returns the client.
// Returns an error if no server is configured for that language.
// In single-server mode (no command set) the one pre-created client is returned
// regardless of languageID — there is nothing else to choose from.
func (m *ServerManager) StartForLanguage(ctx context.Context, rootDir, languageID string) (*LSPClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	langLower := strings.ToLower(languageID)

	for _, e := range m.entries {
		// Single-server mode: command is nil; return the pre-created client.
		if len(e.command) == 0 {
			if e.client != nil && !e.client.IsInitialized() {
				if err := e.client.Initialize(ctx, rootDir); err != nil {
					return nil, fmt.Errorf("initialize server: %w", err)
				}
			}
			return e.client, nil
		}
		// Match by languageID field or by any extension in the set.
		if strings.ToLower(e.languageID) == langLower || e.extensions[langLower] {
			// Daemon mode: for languages that need sustained indexing.
			if NeedsDaemon(langLower) {
				// Close previous daemon connection (socket only; daemon stays alive).
				if e.client != nil {
					_ = e.client.Shutdown(ctx)
				}
				client, err := m.startOrConnectDaemon(ctx, rootDir, langLower, e.command)
				if err != nil {
					return nil, err
				}
				e.client = client
				return client, nil
			}

			// Direct mode: restart if already running.
			if e.client != nil {
				_ = e.client.Shutdown(ctx)
			}
			client := NewLSPClient(e.command[0], e.command[1:])
			if err := client.Initialize(ctx, rootDir); err != nil {
				return nil, fmt.Errorf("initialize server for %q: %w", languageID, err)
			}
			e.client = client
			return client, nil
		}
	}
	return nil, fmt.Errorf("no server configured for language %q; check get_server_capabilities or reconfigure with the correct server binary", languageID)
}

// startOrConnectDaemon checks for an existing daemon and connects, or spawns a new one.
func (m *ServerManager) startOrConnectDaemon(ctx context.Context, rootDir, languageID string, command []string) (*LSPClient, error) {
	// Check for existing running daemon.
	info, err := FindRunningDaemon(rootDir, languageID)
	if err == nil && info != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("daemon: connecting to existing %s daemon (PID %d, ready=%v)", languageID, info.PID, info.Ready))
		client, err := NewDaemonClient(info)
		if err == nil {
			return client, nil
		}
		// Connection failed; daemon may be dead. Clean up and respawn.
		logging.Log(logging.LevelDebug, fmt.Sprintf("daemon: connection failed, respawning: %v", err))
	}

	// Spawn a new daemon broker.
	logging.Log(logging.LevelDebug, fmt.Sprintf("daemon: spawning new %s daemon for %s", languageID, rootDir))
	if err := spawnDaemonProcess(rootDir, languageID, command); err != nil {
		return nil, fmt.Errorf("daemon: failed to spawn broker: %w", err)
	}

	// Wait briefly for the daemon to start and create its socket.
	var daemonInfo *DaemonInfo
	for i := 0; i < 20; i++ { // up to 10 seconds
		time.Sleep(500 * time.Millisecond)
		daemonInfo, _ = FindRunningDaemon(rootDir, languageID)
		if daemonInfo != nil {
			break
		}
	}
	if daemonInfo == nil {
		return nil, fmt.Errorf("daemon: broker did not start within 10 seconds")
	}

	client, err := NewDaemonClient(daemonInfo)
	if err != nil {
		return nil, fmt.Errorf("daemon: failed to connect to new broker: %w", err)
	}
	return client, nil
}

// spawnDaemonProcess launches the daemon-broker as a detached subprocess.
func spawnDaemonProcess(rootDir, languageID string, command []string) error {
	// Find our own binary path.
	self, err := os.Executable()
	if err != nil {
		return err
	}

	cmdStr := strings.Join(command, ",")
	args := []string{
		"daemon-broker",
		"--root-dir=" + rootDir,
		"--language=" + languageID,
		"--command=" + cmdStr,
	}

	cmd := exec.Command(self, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Detach: don't let the subprocess die with us.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	return cmd.Start()
}

// Shutdown gracefully shuts down all managed LSP clients.
func (m *ServerManager) Shutdown(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	for _, e := range m.entries {
		if e.client != nil {
			if err := e.client.Shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// inferLanguageID returns a reasonable language ID from the server entry.
// If LanguageID is set, return it. Otherwise use Extensions[0] or "unknown".
// Mapping follows mcp-lsp-bridge convention.
func inferLanguageID(entry config.ServerEntry) string {
	if entry.LanguageID != "" {
		return entry.LanguageID
	}
	if len(entry.Extensions) == 0 {
		return "unknown"
	}
	ext := strings.ToLower(strings.TrimPrefix(entry.Extensions[0], "."))
	switch ext {
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
	default:
		return ext
	}
}

// LanguageIDFromPath maps a file path's extension to an LSP language ID.
// Canonical implementation shared by internal/lsp and internal/tools (E5 deduplication).
// Covers Go, TypeScript, JavaScript, Python, Rust, and common language server languages.
func LanguageIDFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".cs":
		return "csharp"
	case ".hs", ".lhs":
		return "haskell"
	case ".rb":
		return "ruby"
	case ".kt", ".kts":
		return "kotlin"
	case ".ml", ".mli":
		return "ocaml"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".java":
		return "java"
	default:
		return "plaintext"
	}
}
