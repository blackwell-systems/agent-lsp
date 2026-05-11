package notify

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// DiagChangeNotification is the payload emitted when errors increase for a file.
type DiagChangeNotification struct {
	Type    string `json:"type"`
	File    string `json:"file"`
	Errors  int    `json:"errors"`
	Delta   int    `json:"delta"`
	Message string `json:"message"`
}

// diagChangeTracker tracks previous error counts per URI and emits notifications
// when errors increase after debouncing.
type diagChangeTracker struct {
	mu       sync.Mutex
	prev     map[string]int // previous error count per URI
	pending  map[string]int // latest error count per URI (pending debounce)
	timer    *time.Timer
	interval time.Duration
	emit     func(DiagChangeNotification)
}

// newDiagChangeTracker creates a tracker that debounces diagnostic updates and
// calls emit when the error count for a URI increases compared to the last flush.
func newDiagChangeTracker(interval time.Duration, emit func(DiagChangeNotification)) *diagChangeTracker {
	return &diagChangeTracker{
		prev:     make(map[string]int),
		pending:  make(map[string]int),
		interval: interval,
		emit:     emit,
	}
}

// OnDiagnostic processes a publishDiagnostics notification for the given URI.
// It counts errors (severity==1), records the pending state, and resets the
// debounce timer.
func (t *diagChangeTracker) OnDiagnostic(uri string, diags []types.LSPDiagnostic) {
	var errors int
	for _, d := range diags {
		if d.Severity == 1 {
			errors++
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.pending[uri] = errors

	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(t.interval, t.flush)
}

// Stop stops the debounce timer and flushes any remaining pending updates.
func (t *diagChangeTracker) Stop() {
	t.mu.Lock()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.mu.Unlock()

	t.flush()
}

// flush checks each pending URI: if the error count increased compared to prev,
// emit a notification. Then update prev and clear pending.
func (t *diagChangeTracker) flush() {
	t.mu.Lock()
	if len(t.pending) == 0 {
		t.mu.Unlock()
		return
	}

	// Snapshot pending and update prev under the lock.
	type entry struct {
		uri    string
		errors int
		delta  int
	}
	var toEmit []entry

	for uri, errCount := range t.pending {
		prevCount := t.prev[uri]
		if errCount > prevCount {
			toEmit = append(toEmit, entry{
				uri:    uri,
				errors: errCount,
				delta:  errCount - prevCount,
			})
		}
		t.prev[uri] = errCount
	}
	t.pending = make(map[string]int)
	t.mu.Unlock()

	for _, e := range toEmit {
		t.emit(DiagChangeNotification{
			Type:    "diagnostic_regression",
			File:    e.uri,
			Errors:  e.errors,
			Delta:   e.delta,
			Message: fmt.Sprintf("%d new errors in %s", e.delta, e.uri),
		})
	}
}

// SubscribeDiagnosticChanges creates a diagnostic change tracker that emits
// notifications via the Hub when errors increase for any file. It registers
// with the given subscriber and returns a stop function that unsubscribes and
// stops the tracker.
func SubscribeDiagnosticChanges(hub *Hub, subscriber DiagnosticSubscriber) func() {
	emit := func(n DiagChangeNotification) {
		msg, err := json.Marshal(n)
		if err != nil {
			return
		}
		hub.Send("warning", "diagnostic_regression", string(msg))
	}

	tracker := newDiagChangeTracker(500*time.Millisecond, emit)

	cb := tracker.OnDiagnostic
	subscriber.SubscribeToDiagnostics(cb)

	return func() {
		subscriber.UnsubscribeFromDiagnostics(cb)
		tracker.Stop()
	}
}
