package main

import (
	"strings"
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
	client := &lsp.LSPClient{}

	wireNotificationsToClient(hub, client)

	called := false
	client.SubscribeToFileChanges(func(changes []types.FileChangeEvent) {
		called = true
	})

	time.Sleep(50 * time.Millisecond)

	hub.Close()

	if called {
		t.Error("callback should not have been invoked yet")
	}
}

func TestDiagnosticNotificationEndToEnd(t *testing.T) {
	sender := &mockNotifySender{}
	hub := notify.NewHub(sender)
	defer hub.Close()

	// Subscribe using the same path as wireNotificationsToClient.
	sub := &testDiagSubscriber{}
	stopDiag := notify.SubscribeDiagnostics(hub, sub)

	// Simulate gopls publishing diagnostics with errors.
	sub.fire("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "undefined: foo"},
		{Severity: 2, Message: "unused variable"},
	})

	// Stop the debouncer, which flushes pending notifications.
	stopDiag()

	if len(sender.logs) == 0 {
		t.Fatal("expected at least 1 diagnostic notification, got 0")
	}

	msg := sender.logs[0].message
	if sender.logs[0].logger != "diagnostics" {
		t.Errorf("expected logger='diagnostics', got %q", sender.logs[0].logger)
	}
	if !strings.Contains(msg, `"errors":1`) {
		t.Errorf("expected errors:1 in message, got: %s", msg)
	}
	if !strings.Contains(msg, `"warnings":1`) {
		t.Errorf("expected warnings:1 in message, got: %s", msg)
	}
	t.Logf("notification sent: %s", msg)
}

// testDiagSubscriber implements notify.DiagnosticSubscriber for integration testing.
type testDiagSubscriber struct {
	cb types.DiagnosticUpdateCallback
}

func (s *testDiagSubscriber) SubscribeToDiagnostics(cb types.DiagnosticUpdateCallback) {
	s.cb = cb
}

func (s *testDiagSubscriber) UnsubscribeFromDiagnostics(cb types.DiagnosticUpdateCallback) {
	s.cb = nil
}

func (s *testDiagSubscriber) fire(uri string, diags []types.LSPDiagnostic) {
	if s.cb != nil {
		s.cb(uri, diags)
	}
}
