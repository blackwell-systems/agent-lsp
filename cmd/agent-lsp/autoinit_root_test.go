package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAutoInitRootAllowed pins review #60 finding 4: workspace-level
// auto-init (replace_in_files) must never root a whole-workspace write tool
// at $HOME — directly or through a symlink. A home directory that is itself
// a git repo (dotfiles) would become the scan root, bounded only by the
// occurrence cap.
func TestAutoInitRootAllowed(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	if autoInitRootAllowed(home) {
		t.Errorf("$HOME (%q) must not be auto-rootable", home)
	}

	// A symlinked path resolving to $HOME is also refused.
	link := filepath.Join(t.TempDir(), "homelink")
	if err := os.Symlink(home, link); err == nil {
		if autoInitRootAllowed(link) {
			t.Errorf("symlink to $HOME (%q) must not be auto-rootable", link)
		}
	}

	// Normal project directories stay allowed.
	tmp := t.TempDir()
	if !autoInitRootAllowed(tmp) {
		t.Errorf("temp project dir %q should be auto-rootable", tmp)
	}
	if autoInitRootAllowed("") {
		t.Errorf("empty dir should be refused")
	}
}
