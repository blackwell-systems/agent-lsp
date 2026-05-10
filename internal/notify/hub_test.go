package notify

import (
	"sync"
	"sync/atomic"
	"testing"
)

// mockSender records calls to SendLog and SendResourceUpdated.
type mockSender struct {
	logs    []logCall
	updates []string
	mu      sync.Mutex
	logErr  error
	updErr  error
}

type logCall struct {
	level, logger, message string
}

func (m *mockSender) SendLog(level, logger, message string) error {
	m.mu.Lock()
	m.logs = append(m.logs, logCall{level, logger, message})
	m.mu.Unlock()
	return m.logErr
}

func (m *mockSender) SendResourceUpdated(uri string) error {
	m.mu.Lock()
	m.updates = append(m.updates, uri)
	m.mu.Unlock()
	return m.updErr
}

func TestHub_NilSender(t *testing.T) {
	h := NewHub(nil)

	// Should not panic with nil sender
	h.Send("info", "test", "hello")
	h.SendResourceUpdate("file:///a.go")

	// No way to observe calls since sender is nil; just verify no panic.
}

func TestHub_SetSender(t *testing.T) {
	h := NewHub(nil)

	// Send with nil sender: no-op
	h.Send("info", "test", "before")

	ms := &mockSender{}
	h.SetSender(ms)

	h.Send("warning", "lsp", "something happened")
	h.SendResourceUpdate("file:///b.go")

	if len(ms.logs) != 1 {
		t.Fatalf("expected 1 log call, got %d", len(ms.logs))
	}
	if ms.logs[0].level != "warning" || ms.logs[0].logger != "lsp" || ms.logs[0].message != "something happened" {
		t.Fatalf("unexpected log call: %+v", ms.logs[0])
	}
	if len(ms.updates) != 1 || ms.updates[0] != "file:///b.go" {
		t.Fatalf("unexpected update calls: %v", ms.updates)
	}

	// Replace sender
	ms2 := &mockSender{}
	h.SetSender(ms2)
	h.Send("error", "diag", "new sender")

	if len(ms2.logs) != 1 {
		t.Fatalf("expected 1 log on new sender, got %d", len(ms2.logs))
	}
	// Original sender should not get new calls
	if len(ms.logs) != 1 {
		t.Fatalf("original sender got extra calls: %d", len(ms.logs))
	}
}

func TestHub_Close(t *testing.T) {
	ms := &mockSender{}
	h := NewHub(ms)

	h.Send("info", "test", "before close")
	if len(ms.logs) != 1 {
		t.Fatalf("expected 1 log before close, got %d", len(ms.logs))
	}

	var stopCalled atomic.Bool
	h.AddStopFunc(func() { stopCalled.Store(true) })

	h.Close()

	if !stopCalled.Load() {
		t.Fatal("stop function was not called")
	}

	// Sends after close should be no-ops
	h.Send("info", "test", "after close")
	h.SendResourceUpdate("file:///c.go")

	if len(ms.logs) != 1 {
		t.Fatalf("expected no new logs after close, got %d", len(ms.logs))
	}

	// Double close is safe
	h.Close()
}

func TestHub_ConcurrentAccess(t *testing.T) {
	ms := &mockSender{}
	h := NewHub(ms)

	var wg sync.WaitGroup
	const goroutines = 50

	// Hammer SetSender and Send concurrently
	wg.Add(goroutines * 3)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			h.Send("info", "concurrent", "msg")
		}()
		go func() {
			defer wg.Done()
			h.SendResourceUpdate("file:///x.go")
		}()
		go func() {
			defer wg.Done()
			h.SetSender(ms)
		}()
	}
	wg.Wait()

	// Close concurrently
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			h.Close()
		}()
	}
	wg.Wait()
}
