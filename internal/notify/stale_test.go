package notify

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// staleMockSender records calls for test verification.
type staleMockSender struct {
	mu       sync.Mutex
	logs     []logEntry
	updates  []string
}

type logEntry struct {
	level, logger, message string
}

func (m *staleMockSender) SendLog(level, logger, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, logEntry{level, logger, message})
	return nil
}

func (m *staleMockSender) SendResourceUpdated(uri string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, uri)
	return nil
}

func TestStaleNotifier_Debounce(t *testing.T) {
	sender := &staleMockSender{}
	hub := NewHub(sender)
	n := NewStaleNotifier(hub, 50*time.Millisecond)

	// Fire multiple rapid changes.
	for i := 0; i < 5; i++ {
		n.OnFileChange([]FileChange{{URI: "file:///a.go", ChangeType: 2}})
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to fire.
	time.Sleep(100 * time.Millisecond)

	sender.mu.Lock()
	logCount := len(sender.logs)
	sender.mu.Unlock()

	if logCount != 1 {
		t.Fatalf("expected 1 coalesced emission, got %d", logCount)
	}
}

func TestStaleNotifier_Emits(t *testing.T) {
	sender := &staleMockSender{}
	hub := NewHub(sender)
	n := NewStaleNotifier(hub, 20*time.Millisecond)

	n.OnFileChange([]FileChange{
		{URI: "file:///a.go", ChangeType: 2},
		{URI: "file:///b.go", ChangeType: 1},
		{URI: "file:///c.go", ChangeType: 3},
	})

	time.Sleep(50 * time.Millisecond)

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(sender.logs))
	}

	entry := sender.logs[0]
	if entry.level != "info" || entry.logger != "file_watcher" {
		t.Fatalf("unexpected log entry: level=%s logger=%s", entry.level, entry.logger)
	}

	var payload struct {
		Type         string `json:"type"`
		FilesChanged int    `json:"files_changed"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal([]byte(entry.message), &payload); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}
	if payload.Type != "stale_references" {
		t.Errorf("expected type stale_references, got %s", payload.Type)
	}
	if payload.FilesChanged != 3 {
		t.Errorf("expected 3 files_changed, got %d", payload.FilesChanged)
	}
}

func TestStaleNotifier_ResourceUpdate(t *testing.T) {
	sender := &staleMockSender{}
	hub := NewHub(sender)
	n := NewStaleNotifier(hub, 20*time.Millisecond)

	n.OnFileChange([]FileChange{{URI: "file:///x.go", ChangeType: 2}})
	time.Sleep(50 * time.Millisecond)

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.updates) != 1 {
		t.Fatalf("expected 1 resource update, got %d", len(sender.updates))
	}
	if sender.updates[0] != "lsp-references://" {
		t.Errorf("expected lsp-references://, got %s", sender.updates[0])
	}
}

func TestStaleNotifier_Stop(t *testing.T) {
	sender := &staleMockSender{}
	hub := NewHub(sender)
	n := NewStaleNotifier(hub, 5*time.Second) // long interval

	n.OnFileChange([]FileChange{{URI: "file:///z.go", ChangeType: 2}})
	// Stop should flush immediately without waiting for timer.
	n.Stop()

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.logs) != 1 {
		t.Fatalf("expected Stop to flush pending, got %d logs", len(sender.logs))
	}
	if len(sender.updates) != 1 {
		t.Fatalf("expected Stop to flush resource update, got %d", len(sender.updates))
	}
}

func TestAdaptFileChangeEvents(t *testing.T) {
	events := []types.FileChangeEvent{
		{URI: "file:///foo.go", Type: 1},
		{URI: "file:///bar.go", Type: 2},
		{URI: "file:///baz.go", Type: 3},
	}

	result := AdaptFileChangeEvents(events)

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	for i, r := range result {
		if r.URI != events[i].URI {
			t.Errorf("[%d] URI mismatch: %s vs %s", i, r.URI, events[i].URI)
		}
		if r.ChangeType != events[i].Type {
			t.Errorf("[%d] Type mismatch: %d vs %d", i, r.ChangeType, events[i].Type)
		}
	}
}
