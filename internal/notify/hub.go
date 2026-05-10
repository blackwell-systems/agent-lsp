package notify

import (
	"sync"
	"sync/atomic"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// NotificationSender abstracts MCP session notification dispatch.
// Implemented by mcpNotificationSender in cmd/agent-lsp/notifications.go.
type NotificationSender interface {
	SendLog(level, logger, message string) error
	SendResourceUpdated(uri string) error
}

// DiagnosticSubscriber is implemented by types that support diagnostic
// publish/subscribe (e.g. LSPClient). Used by the diagnostic notification
// channel to observe language server diagnostics.
type DiagnosticSubscriber interface {
	SubscribeToDiagnostics(cb types.DiagnosticUpdateCallback)
	UnsubscribeFromDiagnostics(cb types.DiagnosticUpdateCallback)
}

// Hub coordinates all proactive notification channels.
// It holds the sender reference and manages channel lifecycle.
type Hub struct {
	mu        sync.RWMutex
	sender    NotificationSender
	stopFuncs []func()
	closed    atomic.Bool
}

// NewHub creates a Hub with the given sender. Sender may be nil
// (notifications are silently dropped until a session connects).
func NewHub(sender NotificationSender) *Hub {
	return &Hub{sender: sender}
}

// SetSender atomically replaces the notification sender.
// Called when an MCP client reconnects or a new session initializes.
func (h *Hub) SetSender(s NotificationSender) {
	h.mu.Lock()
	h.sender = s
	h.mu.Unlock()
}

// AddStopFunc registers a cleanup function that will be called on Close.
// Used by notification channels to register their teardown logic.
func (h *Hub) AddStopFunc(fn func()) {
	h.mu.Lock()
	h.stopFuncs = append(h.stopFuncs, fn)
	h.mu.Unlock()
}

// Close tears down all registered channels and marks the hub as closed.
// Subsequent Send calls become no-ops. Close is idempotent.
func (h *Hub) Close() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	h.mu.RLock()
	fns := make([]func(), len(h.stopFuncs))
	copy(fns, h.stopFuncs)
	h.mu.RUnlock()

	for _, fn := range fns {
		fn()
	}
}

// Send emits a log notification at the given level. It is best-effort:
// errors are silently dropped. No-op if the hub is closed or sender is nil.
func (h *Hub) Send(level, logger, message string) {
	if h.closed.Load() {
		return
	}
	h.mu.RLock()
	s := h.sender
	h.mu.RUnlock()
	if s == nil {
		return
	}
	_ = s.SendLog(level, logger, message)
}

// SendResourceUpdate emits a resource-updated notification. Best-effort:
// errors are silently dropped. No-op if the hub is closed or sender is nil.
func (h *Hub) SendResourceUpdate(uri string) {
	if h.closed.Load() {
		return
	}
	h.mu.RLock()
	s := h.sender
	h.mu.RUnlock()
	if s == nil {
		return
	}
	_ = s.SendResourceUpdated(uri)
}
