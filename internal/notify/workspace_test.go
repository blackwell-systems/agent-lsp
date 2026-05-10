package notify

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockWorkspaceClient struct {
	mu     sync.Mutex
	loaded bool
}

func (m *mockWorkspaceClient) IsWorkspaceLoaded() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loaded
}

func (m *mockWorkspaceClient) setLoaded(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loaded = v
}

type recordingSender struct {
	mu       sync.Mutex
	messages []string
}

func (r *recordingSender) SendLog(level, logger, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, message)
	return nil
}

func (r *recordingSender) SendResourceUpdated(uri string) error {
	return nil
}

func (r *recordingSender) getMessages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]string, len(r.messages))
	copy(cp, r.messages)
	return cp
}

func TestSubscribeWorkspaceReady_AlreadyLoaded(t *testing.T) {
	sender := &recordingSender{}
	hub := NewHub(sender)
	client := &mockWorkspaceClient{loaded: true}

	stop := SubscribeWorkspaceReady(hub, client, 10*time.Millisecond)
	defer stop()

	msgs := sender.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var payload workspaceReadyPayload
	if err := json.Unmarshal([]byte(msgs[0]), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Type != "workspace_ready" {
		t.Errorf("expected type workspace_ready, got %s", payload.Type)
	}
}

func TestSubscribeWorkspaceReady_WaitsForReady(t *testing.T) {
	sender := &recordingSender{}
	hub := NewHub(sender)
	client := &mockWorkspaceClient{loaded: false}

	stop := SubscribeWorkspaceReady(hub, client, 5*time.Millisecond)
	defer stop()

	// Shouldn't have emitted yet.
	time.Sleep(15 * time.Millisecond)
	if msgs := sender.getMessages(); len(msgs) != 0 {
		t.Fatalf("expected no messages yet, got %d", len(msgs))
	}

	// Mark loaded.
	client.setLoaded(true)

	// Wait for poll to detect it.
	time.Sleep(20 * time.Millisecond)
	msgs := sender.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after ready, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "workspace_ready") {
		t.Errorf("expected workspace_ready in message, got %s", msgs[0])
	}
}

func TestSubscribeWorkspaceReady_Timeout(t *testing.T) {
	// Patch the timeout to be very short for testing.
	// We'll test by using a never-ready client and a short poll interval,
	// then verify no message is sent after the goroutine exits.
	sender := &recordingSender{}
	hub := NewHub(sender)
	client := &mockWorkspaceClient{loaded: false}

	// Use a short poll interval. The internal 5-minute timeout means this test
	// would take too long normally, but we can verify the stop function works.
	stop := SubscribeWorkspaceReady(hub, client, 5*time.Millisecond)

	// Let it poll a few times.
	time.Sleep(30 * time.Millisecond)

	// Stop it and verify no messages were sent.
	stop()
	time.Sleep(10 * time.Millisecond)

	msgs := sender.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for never-ready workspace, got %d", len(msgs))
	}
}
