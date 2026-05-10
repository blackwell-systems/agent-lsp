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

	// Stop it.
	stop()

	// Simulate crash after stop.
	checker.setAlive(false)
	time.Sleep(20 * time.Millisecond)

	// No crash message should be emitted.
	msgs := sender.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages after stop, got %d", len(msgs))
	}
}
