package notify

// NotificationSender abstracts MCP session notification dispatch.
// Implemented by mcpNotificationSender in cmd/agent-lsp/notifications.go.
type NotificationSender interface {
	SendLog(level, logger, message string) error
	SendResourceUpdated(uri string) error
}

// Hub coordinates all proactive notification channels.
// It holds the sender reference and manages channel lifecycle.
type Hub struct {
	sender NotificationSender
}

// NewHub creates a Hub with the given sender. Sender may be nil
// (notifications are silently dropped until a session connects).
func NewHub(sender NotificationSender) *Hub {
	return &Hub{sender: sender}
}

// SetSender replaces the notification sender (called on reconnect/new session).
func (h *Hub) SetSender(s NotificationSender) {
	h.sender = s
}
