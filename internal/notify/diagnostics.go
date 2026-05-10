package notify

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// DiagUpdate represents the diagnostic counts for a single URI.
type DiagUpdate struct {
	URI        string `json:"uri"`
	ErrorCount int    `json:"errors"`
	WarnCount  int    `json:"warnings"`
}

// DiagnosticSubscriber allows registration of diagnostic update callbacks.
// Implemented by the LSP client/manager.
type DiagnosticSubscriber interface {
	SubscribeToDiagnostics(cb types.DiagnosticUpdateCallback)
	UnsubscribeFromDiagnostics(cb types.DiagnosticUpdateCallback)
}

// diagState holds error/warning counts for a single URI.
type diagState struct {
	ErrorCount int
	WarnCount  int
}

// diagDebouncer coalesces rapid diagnostic updates into periodic flushes.
type diagDebouncer struct {
	mu       sync.Mutex
	pending  map[string]diagState
	timer    *time.Timer
	interval time.Duration
	emit     func(updates []DiagUpdate)
}

// newDiagDebouncer creates a debouncer that calls emit after interval of quiet.
func newDiagDebouncer(interval time.Duration, emit func([]DiagUpdate)) *diagDebouncer {
	return &diagDebouncer{
		pending:  make(map[string]diagState),
		interval: interval,
		emit:     emit,
	}
}

// OnDiagnostic processes a publishDiagnostics notification for the given URI.
// It counts errors (severity==1) and warnings (severity==2), updates pending
// state, and resets the debounce timer.
func (d *diagDebouncer) OnDiagnostic(uri string, diags []types.LSPDiagnostic) {
	var errors, warnings int
	for _, diag := range diags {
		switch diag.Severity {
		case 1:
			errors++
		case 2:
			warnings++
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.pending[uri] = diagState{ErrorCount: errors, WarnCount: warnings}

	// Reset or start the debounce timer.
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.interval, d.flush)
}

// Stop stops the debounce timer and flushes any remaining pending updates.
func (d *diagDebouncer) Stop() {
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.mu.Unlock()

	d.flush()
}

// flush collects all pending updates, clears the map, and calls emit.
func (d *diagDebouncer) flush() {
	d.mu.Lock()
	if len(d.pending) == 0 {
		d.mu.Unlock()
		return
	}

	updates := make([]DiagUpdate, 0, len(d.pending))
	for uri, state := range d.pending {
		updates = append(updates, DiagUpdate{
			URI:        uri,
			ErrorCount: state.ErrorCount,
			WarnCount:  state.WarnCount,
		})
	}
	d.pending = make(map[string]diagState)
	d.mu.Unlock()

	d.emit(updates)
}

// SubscribeDiagnostics creates a diagnostic debouncer that emits periodic
// summary notifications via the Hub. It registers with the given subscriber
// and returns a stop function that unsubscribes and stops the debouncer.
func SubscribeDiagnostics(hub *Hub, subscriber DiagnosticSubscriber) func() {
	emit := func(updates []DiagUpdate) {
		payload := struct {
			Type    string       `json:"type"`
			Updates []DiagUpdate `json:"updates"`
		}{
			Type:    "diagnostics",
			Updates: updates,
		}
		msg, err := json.Marshal(payload)
		if err != nil {
			return
		}
		if hub.sender != nil {
			hub.sender.SendLog("info", "diagnostics", string(msg))
		}
	}

	debouncer := newDiagDebouncer(2*time.Second, emit)

	cb := debouncer.OnDiagnostic
	subscriber.SubscribeToDiagnostics(cb)

	return func() {
		subscriber.UnsubscribeFromDiagnostics(cb)
		debouncer.Stop()
	}
}
