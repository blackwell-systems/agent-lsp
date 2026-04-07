package lsp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/blackwell-systems/lsp-mcp-go/internal/config"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"
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

	for _, e := range m.entries {
		if len(e.command) == 0 {
			continue
		}
		client := NewLSPClient(e.command[0], e.command[1:])
		logging.Log(logging.LevelDebug, fmt.Sprintf("ServerManager.StartAll: starting %s", e.command[0]))
		if err := client.Initialize(ctx, rootDir); err != nil {
			return fmt.Errorf("initialize server %s: %w", e.command[0], err)
		}
		e.client = client
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
