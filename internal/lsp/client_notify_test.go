package lsp

import "testing"

func TestIsWorkspaceLoaded(t *testing.T) {
	c := &LSPClient{}

	if c.IsWorkspaceLoaded() {
		t.Fatal("expected IsWorkspaceLoaded to be false initially")
	}

	c.workspaceLoaded.Store(true)

	if !c.IsWorkspaceLoaded() {
		t.Fatal("expected IsWorkspaceLoaded to be true after Store(true)")
	}
}

func TestIsAlive_NilCmd(t *testing.T) {
	c := &LSPClient{}

	// No cmd, not daemon, not passive: should be dead.
	if c.IsAlive() {
		t.Fatal("expected IsAlive to return false when cmd is nil")
	}
}

func TestIsAlive_DaemonMode(t *testing.T) {
	c := &LSPClient{isDaemon: true}

	if !c.IsAlive() {
		t.Fatal("expected IsAlive to return true for daemon mode")
	}
}

func TestIsAlive_PassiveMode(t *testing.T) {
	c := &LSPClient{isPassive: true}

	if !c.IsAlive() {
		t.Fatal("expected IsAlive to return true for passive mode")
	}
}
