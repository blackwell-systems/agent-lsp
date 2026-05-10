package notify

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type mockHealthChecker struct {
	mu    sync.Mutex
	alive bool
}

func (m *mockHealthChecker) IsAlive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.alive
}

func (m *mockHealthChecker) setAlive(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alive = v
}

func TestSubscribeHealth_CrashDetection(t *testing.T) {
	sender := &recordingSender{}
	hub := NewHub(sender)
	checker := &mockHealthChecker{alive: true}

	stop := SubscribeHealth(hub, checker, 5*time.Millisecond)
	defer stop()

	// Simulate crash.
	checker.setAlive(false)
	time.Sleep(20 * time.Millisecond)

	msgs := sender.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 crash message, got %d", len(msgs))
	}

	var payload healthPayload
	if err := json.Unmarshal([]byte(msgs[0]), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Type != "process_health" {
		t.Errorf("expected type process_health, got %s", payload.Type)
	}
	if payload.Status != "crashed" {
		t.Errorf("expected status crashed, got %s", payload.Status)
	}
}

func TestSubscribeHealth_Recovery(t *testing.T) {
	sender := &recordingSender{}
	hub := NewHub(sender)
	checker := &mockHealthChecker{alive: true}

	stop := SubscribeHealth(hub, checker, 5*time.Millisecond)
	defer stop()

	// Crash first.
	checker.setAlive(false)
	time.Sleep(20 * time.Millisecond)

	// Then recover.
	checker.setAlive(true)
	time.Sleep(20 * time.Millisecond)

	msgs := sender.getMessages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages (crash + recovery), got %d", len(msgs))
	}

	var payload healthPayload
	if err := json.Unmarshal([]byte(msgs[1]), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Status != "recovered" {
		t.Errorf("expected status recovered, got %s", payload.Status)
	}
}

func TestSubscribeHealth_Stop(t *testing.T) {
	sender := &recordingSender{}
	hub := NewHub(sender)
	checker := &mockHealthChecker{alive: true}

	stop := SubscribeHealth(hub, checker, 5*time.Millisecond)

	// Let it run briefly.
	time.Sleep(15 * time.Millisecond)

	// Record messages before stop (should be zero since alive stays true).
	msgsBefore := sender.getMessages()

	// Stop it.
	stop()

	// Wait for goroutine to exit.
	time.Sleep(10 * time.Millisecond)

	// Simulate crash after stop.
	checker.setAlive(false)
	time.Sleep(20 * time.Millisecond)

	// No NEW crash message should be emitted after stop.
	msgsAfter := sender.getMessages()
	if len(msgsAfter) != len(msgsBefore) {
		t.Errorf("expected no new messages after stop, got %d new (before=%d, after=%d)",
			len(msgsAfter)-len(msgsBefore), len(msgsBefore), len(msgsAfter))
	}
}
