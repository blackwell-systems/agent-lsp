// daemon.go manages persistent LSP daemon brokers for language servers that
// need sustained background indexing (pyright, tsserver). The daemon keeps the
// language server alive between agent sessions so the workspace stays indexed.
//
// Architecture:
//
//	agent-lsp (ephemeral MCP session)
//	    → connects via Unix socket
//	daemon-broker (persistent, one per root+language)
//	    → stdio pipes
//	pyright-langserver / tsserver (persistent)
package lsp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// DaemonInfo holds metadata about a running daemon broker.
type DaemonInfo struct {
	RootDir      string    `json:"root_dir"`
	LanguageID   string    `json:"language_id"`
	Command      []string  `json:"command"`
	SocketPath   string    `json:"socket_path"`
	PID          int       `json:"pid"`
	Ready        bool      `json:"ready"`
	StartTime    time.Time `json:"start_time"`
	LastActivity time.Time `json:"last_activity"`
}

// daemonLanguages is the allowlist of languages that benefit from daemon mode.
// These servers need sustained indexing time before references work.
var daemonLanguages = map[string]bool{
	"python":          true,
	"typescript":      true,
	"typescriptreact": true,
	"javascript":      true,
	"javascriptreact": true,
}

// NeedsDaemon returns true if the given language benefits from persistent
// daemon mode due to slow workspace indexing.
func NeedsDaemon(languageID string) bool {
	return daemonLanguages[strings.ToLower(languageID)]
}

// DaemonDir returns the directory for a daemon's state files.
// Path: ~/.cache/agent-lsp/daemons/<hash>/
func DaemonDir(rootDir, languageID string) string {
	h := sha256.Sum256([]byte(rootDir + "\x00" + languageID))
	hash := hex.EncodeToString(h[:])[:12]
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "agent-lsp", "daemons", hash)
}

// FindRunningDaemon checks if a daemon is already running for the given workspace.
// Returns nil if no daemon exists or it's stale.
func FindRunningDaemon(rootDir, languageID string) (*DaemonInfo, error) {
	dir := DaemonDir(rootDir, languageID)
	infoPath := filepath.Join(dir, "daemon.json")

	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, nil // no daemon
	}

	var info DaemonInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, nil // corrupt state, treat as no daemon
	}

	// Verify the process is still alive.
	if !processAlive(info.PID) {
		logging.Log(logging.LevelDebug, fmt.Sprintf("daemon: stale PID %d for %s, cleaning up", info.PID, languageID))
		cleanupDaemonDir(dir)
		return nil, nil
	}

	// Verify socket is reachable.
	conn, err := net.DialTimeout("unix", info.SocketPath, 2*time.Second)
	if err != nil {
		logging.Log(logging.LevelDebug, fmt.Sprintf("daemon: socket unreachable for PID %d, cleaning up", info.PID))
		cleanupDaemonDir(dir)
		return nil, nil
	}
	conn.Close()

	return &info, nil
}

// WriteDaemonInfo writes the daemon metadata to disk.
func WriteDaemonInfo(info *DaemonInfo) error {
	dir := DaemonDir(info.RootDir, info.LanguageID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "daemon.json"), data, 0644)
}

// RefreshDaemonInfo re-reads daemon.json from disk to get the latest ready status.
func RefreshDaemonInfo(rootDir, languageID string) (*DaemonInfo, error) {
	dir := DaemonDir(rootDir, languageID)
	data, err := os.ReadFile(filepath.Join(dir, "daemon.json"))
	if err != nil {
		return nil, err
	}
	var info DaemonInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// CleanupStaleDaemons removes daemon state for processes that are no longer running.
func CleanupStaleDaemons() {
	home, _ := os.UserHomeDir()
	daemonsDir := filepath.Join(home, ".cache", "agent-lsp", "daemons")
	entries, err := os.ReadDir(daemonsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(daemonsDir, entry.Name())
		pidData, err := os.ReadFile(filepath.Join(dir, "daemon.pid"))
		if err != nil {
			cleanupDaemonDir(dir)
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err != nil || !processAlive(pid) {
			cleanupDaemonDir(dir)
		}
	}
}

// StopDaemon sends SIGTERM to a running daemon.
func StopDaemon(rootDir, languageID string) error {
	info, err := FindRunningDaemon(rootDir, languageID)
	if err != nil || info == nil {
		return fmt.Errorf("no running daemon found for %s at %s", languageID, rootDir)
	}
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return fmt.Errorf("sending SIGTERM to daemon PID %d: %w", info.PID, err)
	}
	return proc.Signal(syscall.SIGTERM)
}

// ListDaemons returns info about all currently running daemons.
func ListDaemons() []*DaemonInfo {
	home, _ := os.UserHomeDir()
	daemonsDir := filepath.Join(home, ".cache", "agent-lsp", "daemons")
	entries, err := os.ReadDir(daemonsDir)
	if err != nil {
		return nil
	}
	var result []*DaemonInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(daemonsDir, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "daemon.json"))
		if err != nil {
			continue
		}
		var info DaemonInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		if processAlive(info.PID) {
			result = append(result, &info)
		}
	}
	return result
}

// processAlive checks if a process with the given PID exists.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 tests process existence without actually sending a signal.
	return proc.Signal(syscall.Signal(0)) == nil
}

func cleanupDaemonDir(dir string) {
	os.RemoveAll(dir)
}
