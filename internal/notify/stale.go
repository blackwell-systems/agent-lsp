package notify

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// FileChange represents a single file system change event.
type FileChange struct {
	URI        string `json:"uri"`
	ChangeType int    `json:"type"` // 1=created, 2=changed, 3=deleted
}

// StaleNotifier emits notifications when source files change on disk,
// indicating that cached reference results may be outdated. It debounces
// rapid changes into a single notification.
type StaleNotifier struct {
	hub      *Hub
	mu       sync.Mutex
	pending  []FileChange
	timer    *time.Timer
	interval time.Duration
	stopped  atomic.Bool
}

// NewStaleNotifier creates a StaleNotifier that debounces file change
// notifications over the given interval before emitting.
func NewStaleNotifier(hub *Hub, interval time.Duration) *StaleNotifier {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	return &StaleNotifier{
		hub:      hub,
		interval: interval,
	}
}

// OnFileChange records file changes and resets the debounce timer.
// When the timer fires, a coalesced notification is emitted via the hub.
func (n *StaleNotifier) OnFileChange(changes []FileChange) {
	if n.stopped.Load() {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.pending = append(n.pending, changes...)

	if n.timer != nil {
		n.timer.Stop()
	}
	n.timer = time.AfterFunc(n.interval, n.flush)
}

// Stop stops the debounce timer and flushes any pending notifications.
func (n *StaleNotifier) Stop() {
	if n.stopped.Swap(true) {
		return // already stopped
	}

	n.mu.Lock()
	if n.timer != nil {
		n.timer.Stop()
	}
	n.mu.Unlock()

	n.flush()
}

// flush emits the coalesced notification for all pending file changes.
func (n *StaleNotifier) flush() {
	n.mu.Lock()
	pending := n.pending
	n.pending = nil
	n.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	// Emit resource update for the references URI.
	n.hub.SendResourceUpdate("lsp-references://")

	// Emit a log notification with change details.
	payload := struct {
		Type         string `json:"type"`
		FilesChanged int    `json:"files_changed"`
		Message      string `json:"message"`
	}{
		Type:         "stale_references",
		FilesChanged: len(pending),
		Message:      fmt.Sprintf("Source files changed on disk; cached references may be outdated"),
	}

	data, _ := json.Marshal(payload)
	n.hub.Send("info", "file_watcher", string(data))
}
