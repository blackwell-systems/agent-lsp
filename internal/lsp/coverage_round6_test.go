package lsp

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/pkg/types"
)

// --- SubscribeToFileChanges tests ---

func TestSubscribeToFileChanges_AddsCallback(t *testing.T) {
	c := &LSPClient{}

	called := false
	c.SubscribeToFileChanges(func(events []types.FileChangeEvent) {
		called = true
	})

	c.watcherMu.Lock()
	numCbs := len(c.fileChangeCbs)
	c.watcherMu.Unlock()

	if numCbs != 1 {
		t.Errorf("expected 1 callback registered, got %d", numCbs)
	}

	// Invoke the callback to verify it was stored correctly.
	c.watcherMu.Lock()
	cb := c.fileChangeCbs[0]
	c.watcherMu.Unlock()
	cb(nil)
	if !called {
		t.Error("expected callback to be invoked")
	}
}

func TestSubscribeToFileChanges_MultipleCallbacks(t *testing.T) {
	c := &LSPClient{}

	c.SubscribeToFileChanges(func(events []types.FileChangeEvent) {})
	c.SubscribeToFileChanges(func(events []types.FileChangeEvent) {})
	c.SubscribeToFileChanges(func(events []types.FileChangeEvent) {})

	c.watcherMu.Lock()
	numCbs := len(c.fileChangeCbs)
	c.watcherMu.Unlock()

	if numCbs != 3 {
		t.Errorf("expected 3 callbacks registered, got %d", numCbs)
	}
}

// --- warmupState: concurrent access safety ---

func TestWarmupState_ConcurrentMarkReady(t *testing.T) {
	w := newWarmupState()
	start := make(chan struct{})
	done := make(chan struct{}, 10)

	for i := 0; i < 10; i++ {
		go func() {
			<-start
			w.MarkReady()
			done <- struct{}{}
		}()
	}
	close(start)
	for i := 0; i < 10; i++ {
		<-done
	}

	if !w.completed.Load() {
		t.Error("expected completed=true")
	}
	if !w.firstRefDone.Load() {
		t.Error("expected firstRefDone=true")
	}
}

func TestWarmupState_ConcurrentNotifyDiagnostic(t *testing.T) {
	w := newWarmupState()
	start := make(chan struct{})
	done := make(chan struct{}, 10)

	for i := 0; i < 10; i++ {
		go func() {
			<-start
			w.NotifyDiagnostic()
			done <- struct{}{}
		}()
	}
	close(start)
	for i := 0; i < 10; i++ {
		<-done
	}

	if !w.diagnosticReceived.Load() {
		t.Error("expected diagnosticReceived=true")
	}
}

// --- DaemonInfo serialization round-trip ---

func TestWriteAndRefreshDaemonInfo_RoundTrip(t *testing.T) {
	// Use a unique rootDir so we don't conflict with real state.
	rootDir := t.TempDir()
	langID := "testlang_roundtrip_r6"

	info := &DaemonInfo{
		RootDir:    rootDir,
		LanguageID: langID,
		Command:    []string{"fake-server", "--stdio"},
		PID:        99999,
		Ready:      true,
	}

	if err := WriteDaemonInfo(info); err != nil {
		t.Fatalf("WriteDaemonInfo: %v", err)
	}

	got, err := RefreshDaemonInfo(rootDir, langID)
	if err != nil {
		t.Fatalf("RefreshDaemonInfo: %v", err)
	}
	if got.RootDir != rootDir {
		t.Errorf("RootDir = %q, want %q", got.RootDir, rootDir)
	}
	if got.LanguageID != langID {
		t.Errorf("LanguageID = %q, want %q", got.LanguageID, langID)
	}
	if !got.Ready {
		t.Error("expected Ready=true")
	}
	if len(got.Command) != 2 || got.Command[0] != "fake-server" {
		t.Errorf("Command = %v, want [fake-server --stdio]", got.Command)
	}
}

// --- RefreshDaemonInfo: non-existent ---

func TestRefreshDaemonInfo_NonExistent(t *testing.T) {
	_, err := RefreshDaemonInfo("/nonexistent/path/xyz123", "python")
	if err == nil {
		t.Error("expected error for non-existent daemon info")
	}
}

// --- IsAlive: daemon + passive combinations ---

func TestIsAlive_BothDaemonAndPassive(t *testing.T) {
	c := &LSPClient{isDaemon: true, isPassive: true}
	if !c.IsAlive() {
		t.Error("expected IsAlive=true when both daemon and passive")
	}
}
