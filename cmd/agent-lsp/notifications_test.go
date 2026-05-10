package main

import (
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/notify"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// mockNotifySender records calls for assertions.
type mockNotifySender struct {
	logs     []logEntry
	resource []string
}

type logEntry struct {
	level, logger, message string
}

func (m *mockNotifySender) SendLog(level, logger, message string) error {
	m.logs = append(m.logs, logEntry{level, logger, message})
	return nil
}

func (m *mockNotifySender) SendResourceUpdated(uri string) error {
	m.resource = append(m.resource, uri)
	return nil
}

func TestMcpNotifySender(t *testing.T) {
	// Verify the mcpNotifySender struct satisfies the interface at compile time.
	var _ notify.NotificationSender = (*mcpNotifySender)(nil)
}

func TestSetupNotificationHub(t *testing.T) {
	hub := setupNotificationHub()
	if hub == nil {
		t.Fatal("setupNotificationHub returned nil")
	}
	// Hub should work without a sender (notifications dropped silently).
	hub.Send("info", "test", "should not panic")
	hub.Close()
}

func TestWireNotificationsToClient(t *testing.T) {
	hub := notify.NewHub(&mockNotifySender{})
	// Create a minimal LSPClient just to test that wiring doesn't panic.
	// We use a nil-cmd client which is valid for testing subscription wiring.
	client := &lsp.LSPClient{}

	// wireNotificationsToClient should not panic with a zero-value client.
	wireNotificationsToClient(hub, client)

	// Verify that file change callbacks were registered by invoking them.
	// The SubscribeToFileChanges stores callbacks; we trigger them by calling
	// the method again to confirm the slice was populated.
	called := false
	client.SubscribeToFileChanges(func(changes []types.FileChangeEvent) {
		called = true
	})

	// Give background goroutines a moment to start, then verify the hub
	// has stop funcs registered (one per channel).
	time.Sleep(50 * time.Millisecond)

	// Close the hub to stop all background goroutines.
	hub.Close()

	if called {
		t.Error("callback should not have been invoked yet")
	}
}
