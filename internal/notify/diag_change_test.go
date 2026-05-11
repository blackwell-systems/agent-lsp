package notify

import (
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestDiagChangeNotifier_FiresOnIncrease(t *testing.T) {
	var mu sync.Mutex
	var notifications []DiagChangeNotification

	emit := func(n DiagChangeNotification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	}

	tracker := newDiagChangeTracker(50*time.Millisecond, emit)

	// Error count goes 0 -> 2.
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
		{Severity: 1, Message: "error2"},
	})

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	n := notifications[0]
	if n.Type != "diagnostic_regression" {
		t.Errorf("expected type diagnostic_regression, got %s", n.Type)
	}
	if n.File != "file:///test.go" {
		t.Errorf("expected file file:///test.go, got %s", n.File)
	}
	if n.Errors != 2 {
		t.Errorf("expected errors=2, got %d", n.Errors)
	}
	if n.Delta != 2 {
		t.Errorf("expected delta=2, got %d", n.Delta)
	}
	if n.Message != "2 new errors in file:///test.go" {
		t.Errorf("unexpected message: %s", n.Message)
	}
}

func TestDiagChangeNotifier_SilentOnDecrease(t *testing.T) {
	var mu sync.Mutex
	var notifications []DiagChangeNotification

	emit := func(n DiagChangeNotification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	}

	tracker := newDiagChangeTracker(50*time.Millisecond, emit)

	// Set initial state: 3 errors.
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
		{Severity: 1, Message: "error2"},
		{Severity: 1, Message: "error3"},
	})
	time.Sleep(150 * time.Millisecond)

	// Clear notifications from the initial increase (0->3).
	mu.Lock()
	notifications = nil
	mu.Unlock()

	// Error count goes 3 -> 1.
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
	})
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(notifications) != 0 {
		t.Fatalf("expected 0 notifications on decrease, got %d", len(notifications))
	}
}

func TestDiagChangeNotifier_SilentOnSame(t *testing.T) {
	var mu sync.Mutex
	var notifications []DiagChangeNotification

	emit := func(n DiagChangeNotification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	}

	tracker := newDiagChangeTracker(50*time.Millisecond, emit)

	// Set initial state: 2 errors.
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
		{Severity: 1, Message: "error2"},
	})
	time.Sleep(150 * time.Millisecond)

	// Clear notifications from the initial increase (0->2).
	mu.Lock()
	notifications = nil
	mu.Unlock()

	// Error count stays at 2.
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
		{Severity: 1, Message: "error2"},
	})
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(notifications) != 0 {
		t.Fatalf("expected 0 notifications on same count, got %d", len(notifications))
	}
}

func TestDiagChangeNotifier_Debounces(t *testing.T) {
	var mu sync.Mutex
	var notifications []DiagChangeNotification

	emit := func(n DiagChangeNotification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	}

	tracker := newDiagChangeTracker(100*time.Millisecond, emit)

	// Rapid updates that should coalesce.
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
	})
	time.Sleep(20 * time.Millisecond)
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
		{Severity: 1, Message: "error2"},
	})
	time.Sleep(20 * time.Millisecond)
	tracker.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
		{Severity: 1, Message: "error2"},
		{Severity: 1, Message: "error3"},
	})

	// Wait for the single debounced flush.
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Should get exactly 1 notification (coalesced), with the final error count.
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification (debounced), got %d", len(notifications))
	}
	if notifications[0].Errors != 3 {
		t.Errorf("expected errors=3 (final value), got %d", notifications[0].Errors)
	}
	if notifications[0].Delta != 3 {
		t.Errorf("expected delta=3, got %d", notifications[0].Delta)
	}
}

func TestSubscribeDiagnosticChanges(t *testing.T) {
	sender := &diagMockSender{}
	hub := NewHub(sender)
	sub := &mockSubscriber{}

	stop := SubscribeDiagnosticChanges(hub, sub)

	if sub.cb == nil {
		t.Fatal("expected callback to be registered")
	}

	// Simulate diagnostic update with errors increasing.
	sub.cb("file:///int.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "err1"},
		{Severity: 1, Message: "err2"},
	})

	// Stop flushes.
	stop()

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.messages))
	}

	msg := sender.messages[0]
	if !contains(msg, `"type":"diagnostic_regression"`) {
		t.Errorf("expected diagnostic_regression type, got: %s", msg)
	}
	if !contains(msg, `"errors":2`) {
		t.Errorf("expected errors:2, got: %s", msg)
	}
	if !contains(msg, `"delta":2`) {
		t.Errorf("expected delta:2, got: %s", msg)
	}
}
