package lsp

import (
	"testing"
	"time"
)

func TestNewWarmupState(t *testing.T) {
	ws := newWarmupState()
	if ws.completed.Load() {
		t.Error("new warmup state should not be completed")
	}
	if ws.firstRefTimeout != 300*time.Second {
		t.Errorf("firstRefTimeout = %v, want %v", ws.firstRefTimeout, 300*time.Second)
	}
	if ws.diagnosticReceived.Load() {
		t.Error("new warmup state should not have diagnosticReceived set")
	}
	if ws.firstRefDone.Load() {
		t.Error("new warmup state should not have firstRefDone set")
	}
}

func TestFirstRefTimeout_BeforeReady(t *testing.T) {
	ws := newWarmupState()
	got := ws.FirstRefTimeout()
	if got != 300*time.Second {
		t.Errorf("FirstRefTimeout before ready = %v, want %v", got, 300*time.Second)
	}
}

func TestFirstRefTimeout_AfterMarkReady(t *testing.T) {
	ws := newWarmupState()
	ws.MarkReady()
	got := ws.FirstRefTimeout()
	if got != 0 {
		t.Errorf("FirstRefTimeout after MarkReady = %v, want 0", got)
	}
}

func TestFirstRefTimeout_AfterCompleted(t *testing.T) {
	ws := newWarmupState()
	ws.completed.Store(true)
	got := ws.FirstRefTimeout()
	if got != 0 {
		t.Errorf("FirstRefTimeout after completed = %v, want 0", got)
	}
}

func TestNotifyDiagnostic(t *testing.T) {
	ws := newWarmupState()
	if ws.diagnosticReceived.Load() {
		t.Error("diagnosticReceived should be false initially")
	}
	ws.NotifyDiagnostic()
	if !ws.diagnosticReceived.Load() {
		t.Error("diagnosticReceived should be true after NotifyDiagnostic")
	}
}

func TestMarkReady_SetsFlags(t *testing.T) {
	ws := newWarmupState()
	ws.MarkReady()
	if !ws.completed.Load() {
		t.Error("completed should be true after MarkReady")
	}
	if !ws.firstRefDone.Load() {
		t.Error("firstRefDone should be true after MarkReady")
	}
}
