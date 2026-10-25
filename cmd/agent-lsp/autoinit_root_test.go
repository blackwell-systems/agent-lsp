package main

import (
	"os"
	"path/filepath"
	"runtime"
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

// autoInitRootFromPath must guard the INFERRED root, not the caller's path:
// marker inference walks up, so a cwd under $HOME/Documents with a git repo
// at $HOME infers $HOME — the exact root a write tool must never auto-root
// at. (PR #60 maintainer review, finding 3)
// A root that is an ANCESTOR of $HOME is refused too: marker inference walks
// up, so a stray .git above $HOME (e.g. one at /Users) infers that ancestor,
// and every file under $HOME is then inside the scan.
// (PR #60 maintainer review, round 3, finding 4)
func TestAutoInitRootAllowedRefusesHomeAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path-ancestor reasoning differs on drive-letter roots")
	}
	ancestor := t.TempDir()
	home := filepath.Join(ancestor, "user", "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if autoInitRootAllowed(ancestor) {
		t.Errorf("inferred root %q (ancestor of $HOME %q) must be refused", ancestor, home)
	}
}

func TestAutoInitRootFromPathRefusesInferredHome(t *testing.T) {
	home := t.TempDir()
	// $HOME is itself a git repo (dotfiles).
	if err := os.MkdirAll(filepath.Join(home, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	docs := filepath.Join(home, "Documents")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Chdir(docs)
	if got := autoInitRootFromPath(docs); got != "" {
		t.Fatalf("inferred root %q must be refused (protected $HOME)", got)
	}
}

func TestAutoInitRootFromPathAllowsProjectRoot(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(proj, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	if got := autoInitRootFromPath(src); got != proj {
		t.Fatalf("inferred root %q, want project root %q", got, proj)
	}
}

func TestAutoInitRootFromPathEmptyOnNoMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := autoInitRootFromPath(t.TempDir()); got != "" {
		t.Fatalf("path without workspace markers must infer nothing, got %q", got)
	}
}
