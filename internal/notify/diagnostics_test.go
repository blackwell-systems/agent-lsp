package notify

import (
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestDiagDebouncer_Coalesces(t *testing.T) {
	var mu sync.Mutex
	var flushes [][]DiagUpdate

	emit := func(updates []DiagUpdate) {
		mu.Lock()
		flushes = append(flushes, updates)
		mu.Unlock()
	}

	d := newDiagDebouncer(50*time.Millisecond, emit)

	// Send 10 rapid updates for same URI.
	for i := 0; i < 10; i++ {
		d.OnDiagnostic("file:///test.go", []types.LSPDiagnostic{
			{Severity: 1, Message: "error"},
		})
	}

	// Wait for debounce to fire.
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(flushes) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushes))
	}
	if len(flushes[0]) != 1 {
		t.Fatalf("expected 1 URI in flush, got %d", len(flushes[0]))
	}
	if flushes[0][0].ErrorCount != 1 {
		t.Errorf("expected ErrorCount=1, got %d", flushes[0][0].ErrorCount)
	}
}

func TestDiagDebouncer_MultipleURIs(t *testing.T) {
	var mu sync.Mutex
	var flushes [][]DiagUpdate

	emit := func(updates []DiagUpdate) {
		mu.Lock()
		flushes = append(flushes, updates)
		mu.Unlock()
	}

	d := newDiagDebouncer(50*time.Millisecond, emit)

	d.OnDiagnostic("file:///a.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "err"},
	})
	d.OnDiagnostic("file:///b.go", []types.LSPDiagnostic{
		{Severity: 2, Message: "warn"},
	})
	d.OnDiagnostic("file:///c.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "err"},
		{Severity: 2, Message: "warn"},
	})

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(flushes) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushes))
	}
	if len(flushes[0]) != 3 {
		t.Fatalf("expected 3 URIs in flush, got %d", len(flushes[0]))
	}
}

func TestDiagDebouncer_Stop(t *testing.T) {
	var mu sync.Mutex
	var flushes [][]DiagUpdate

	emit := func(updates []DiagUpdate) {
		mu.Lock()
		flushes = append(flushes, updates)
		mu.Unlock()
	}

	d := newDiagDebouncer(10*time.Second, emit) // Long interval, won't fire naturally.

	d.OnDiagnostic("file:///stop.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "err"},
	})

	// Stop should flush immediately.
	d.Stop()

	mu.Lock()
	defer mu.Unlock()

	if len(flushes) != 1 {
		t.Fatalf("expected 1 flush on Stop, got %d", len(flushes))
	}
	if flushes[0][0].URI != "file:///stop.go" {
		t.Errorf("expected URI file:///stop.go, got %s", flushes[0][0].URI)
	}
}

func TestDiagDebouncer_ErrorCounting(t *testing.T) {
	var mu sync.Mutex
	var flushes [][]DiagUpdate

	emit := func(updates []DiagUpdate) {
		mu.Lock()
		flushes = append(flushes, updates)
		mu.Unlock()
	}

	d := newDiagDebouncer(50*time.Millisecond, emit)

	d.OnDiagnostic("file:///count.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "error1"},
		{Severity: 1, Message: "error2"},
		{Severity: 1, Message: "error3"},
		{Severity: 2, Message: "warn1"},
		{Severity: 2, Message: "warn2"},
		{Severity: 3, Message: "info1"},  // not counted
		{Severity: 4, Message: "hint1"},  // not counted
	})

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(flushes) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushes))
	}
	update := flushes[0][0]
	if update.ErrorCount != 3 {
		t.Errorf("expected 3 errors, got %d", update.ErrorCount)
	}
	if update.WarnCount != 2 {
		t.Errorf("expected 2 warnings, got %d", update.WarnCount)
	}
}

// mockSubscriber implements DiagnosticSubscriber for testing.
type mockSubscriber struct {
	cb types.DiagnosticUpdateCallback
}

func (m *mockSubscriber) SubscribeToDiagnostics(cb types.DiagnosticUpdateCallback) {
	m.cb = cb
}

func (m *mockSubscriber) UnsubscribeFromDiagnostics(cb types.DiagnosticUpdateCallback) {
	m.cb = nil
}

// diagMockSender implements NotificationSender for testing.
type diagMockSender struct {
	mu       sync.Mutex
	messages []string
}

func (s *diagMockSender) SendLog(level, logger, message string) error {
	s.mu.Lock()
	s.messages = append(s.messages, message)
	s.mu.Unlock()
	return nil
}

func (s *diagMockSender) SendResourceUpdated(uri string) error {
	return nil
}

func TestSubscribeDiagnostics(t *testing.T) {
	sender := &diagMockSender{}
	hub := NewHub(sender)
	sub := &mockSubscriber{}

	stop := SubscribeDiagnostics(hub, sub)

	if sub.cb == nil {
		t.Fatal("expected callback to be registered")
	}

	// Simulate diagnostic update.
	sub.cb("file:///int.go", []types.LSPDiagnostic{
		{Severity: 1, Message: "err"},
		{Severity: 2, Message: "warn"},
	})

	// Wait for debounce (default 2s in SubscribeDiagnostics, but the debouncer
	// uses 2s which is too long for tests). Use Stop via the stop function.
	stop()

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.messages))
	}

	// Verify the message contains expected content.
	msg := sender.messages[0]
	if !contains(msg, `"type":"diagnostics"`) {
		t.Errorf("expected message to contain type field, got: %s", msg)
	}
	if !contains(msg, `"errors":1`) {
		t.Errorf("expected message to contain errors:1, got: %s", msg)
	}
	if !contains(msg, `"warnings":1`) {
		t.Errorf("expected message to contain warnings:1, got: %s", msg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
