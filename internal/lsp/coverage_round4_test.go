package lsp

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestIsJDTLS(t *testing.T) {
	tests := []struct {
		name       string
		serverPath string
		want       bool
	}{
		{"jdtls exact", "jdtls", true},
		{"jdtls path", "/usr/local/bin/jdtls", true},
		{"jdtls with version", "eclipse-jdtls-1.0", true},
		{"non-jdtls gopls", "gopls", false},
		{"non-jdtls pyright", "pyright-langserver", false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &LSPClient{serverPath: tt.serverPath}
			got := c.isJDTLS()
			if got != tt.want {
				t.Errorf("isJDTLS() for %q = %v, want %v", tt.serverPath, got, tt.want)
			}
		})
	}
}

func TestRejectPending(t *testing.T) {
	c := &LSPClient{
		pending: make(map[int]*pendingRequest),
	}

	ch1 := make(chan json.RawMessage, 1)
	err1 := make(chan error, 1)
	ch2 := make(chan json.RawMessage, 1)
	err2 := make(chan error, 1)
	c.pending[1] = &pendingRequest{ch: ch1, err: err1}
	c.pending[2] = &pendingRequest{ch: ch2, err: err2}

	testErr := errors.New("server crashed")
	c.rejectPending(testErr)

	if len(c.pending) != 0 {
		t.Errorf("pending map should be empty after rejectPending, got %d entries", len(c.pending))
	}

	// Both error channels should have received the error
	select {
	case e := <-err1:
		if e != testErr {
			t.Errorf("err1 = %v, want %v", e, testErr)
		}
	default:
		t.Error("err1 channel should have an error")
	}

	select {
	case e := <-err2:
		if e != testErr {
			t.Errorf("err2 = %v, want %v", e, testErr)
		}
	default:
		t.Error("err2 channel should have an error")
	}
}

func TestRejectPending_Empty(t *testing.T) {
	c := &LSPClient{
		pending: make(map[int]*pendingRequest),
	}
	// Should not panic with empty pending map
	c.rejectPending(errors.New("test"))
}

func TestLanguageIDFromURI_AdditionalCases(t *testing.T) {
	// These cases supplement the existing TestLanguageIDFromURI in client_test.go
	// and focus on edge cases not covered there.
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///path/file.lhs", "haskell"},
		{"file:///path/file.mli", "ocaml"},
		{"file:///path/file.cxx", "cpp"},
		{"file:///path/file.cc", "cpp"},
		{"file:///path/file.kts", "kotlin"},
		{"file:///noext", "plaintext"},
		{"file:///path/to/app.Py?query=1", "python"},
		{"file:///path/to/app.ts#fragment", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := languageIDFromURI(tt.uri)
			if got != tt.want {
				t.Errorf("languageIDFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestNewWarmupState_Defaults(t *testing.T) {
	w := newWarmupState()
	if w.completed.Load() {
		t.Error("new warmup state should not be completed")
	}
	if w.diagnosticReceived.Load() {
		t.Error("new warmup state should not have diagnosticReceived")
	}
	if w.firstRefDone.Load() {
		t.Error("new warmup state should not have firstRefDone")
	}
	if w.firstRefTimeout == 0 {
		t.Error("firstRefTimeout should be non-zero")
	}
}

func TestWarmupState_MarkReady(t *testing.T) {
	w := newWarmupState()
	w.MarkReady()
	if !w.completed.Load() {
		t.Error("expected completed=true after MarkReady")
	}
	if !w.firstRefDone.Load() {
		t.Error("expected firstRefDone=true after MarkReady")
	}
}

func TestWarmupState_NotifyDiagnostic(t *testing.T) {
	w := newWarmupState()
	w.NotifyDiagnostic()
	if !w.diagnosticReceived.Load() {
		t.Error("expected diagnosticReceived=true after NotifyDiagnostic")
	}
}

func TestWarmupState_FirstRefTimeout_BeforeReady(t *testing.T) {
	w := newWarmupState()
	timeout := w.FirstRefTimeout()
	if timeout == 0 {
		t.Error("expected non-zero FirstRefTimeout before any readiness signal")
	}
}

func TestWarmupState_FirstRefTimeout_AfterMarkReady(t *testing.T) {
	w := newWarmupState()
	w.MarkReady()
	timeout := w.FirstRefTimeout()
	if timeout != 0 {
		t.Errorf("expected 0 timeout after MarkReady, got %s", timeout)
	}
}

func TestWarmupState_FirstRefTimeout_AfterCompleted(t *testing.T) {
	w := newWarmupState()
	w.completed.Store(true)
	timeout := w.FirstRefTimeout()
	if timeout != 0 {
		t.Errorf("expected 0 timeout after completed=true, got %s", timeout)
	}
}
