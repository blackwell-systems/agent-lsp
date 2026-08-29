package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func mkDirWithFiles(t *testing.T, dir string, n int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAddWatchedTree_SkipsOversizedDir is the core guard for issue #18: a
// directory with more entries than the per-directory limit (a data/cache dir)
// must not be watched, because on macOS each watched directory opens one kqueue
// fd per file in it.
func TestAddWatchedTree_SkipsOversizedDir(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	mkDirWithFiles(t, src, 3) // normal source dir
	cache := filepath.Join(root, "data-cache")
	mkDirWithFiles(t, cache, 40) // oversized, simulates a browser/leveldb cache

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	addWatchedTree(w, root, watcherLimits{maxDirEntries: 10, maxEntries: 1_000_000}, 0)

	watched := map[string]bool{}
	for _, p := range w.WatchList() {
		watched[p] = true
	}
	if !watched[src] {
		t.Errorf("expected source dir to be watched; watch list: %v", w.WatchList())
	}
	if watched[cache] {
		t.Errorf("oversized cache dir must be skipped; watch list: %v", w.WatchList())
	}
}

// TestAddWatchedTree_BudgetStops verifies the global entry budget halts the walk
// so total watched entries (≈ fds on macOS) stay bounded.
func TestAddWatchedTree_BudgetStops(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 6; i++ {
		mkDirWithFiles(t, filepath.Join(root, fmt.Sprintf("d%d", i)), 3)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	total := addWatchedTree(w, root, watcherLimits{maxDirEntries: 1000, maxEntries: 8}, 0)
	if total > 8 {
		t.Errorf("watched %d entries, exceeds budget of 8", total)
	}
	// The budget must stop the walk before all 6 dirs (+ root) are watched.
	if len(w.WatchList()) >= 7 {
		t.Errorf("expected the budget to cap watched dirs, got %d", len(w.WatchList()))
	}
}

// TestAddWatchedTree_StartTotalCarries confirms a prior root's count is honored,
// so extending coverage (addWatcherRoot) shares the same budget.
func TestAddWatchedTree_StartTotalCarries(t *testing.T) {
	root := t.TempDir()
	mkDirWithFiles(t, filepath.Join(root, "d"), 3)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// Start already at the budget: nothing new should be added.
	total := addWatchedTree(w, root, watcherLimits{maxDirEntries: 1000, maxEntries: 5}, 5)
	if total != 5 {
		t.Errorf("expected total to stay at budget (5), got %d", total)
	}
	if len(w.WatchList()) != 0 {
		t.Errorf("expected no dirs watched when starting at budget, got %v", w.WatchList())
	}
}

func TestLoadWatcherLimits_Defaults(t *testing.T) {
	t.Setenv("AGENT_LSP_WATCH_MAX_DIR_ENTRIES", "")
	t.Setenv("AGENT_LSP_WATCH_MAX_ENTRIES", "")
	lim := loadWatcherLimits()
	if lim.maxDirEntries != defaultWatchMaxDirEntries || lim.maxEntries != defaultWatchMaxEntries {
		t.Errorf("expected defaults, got %+v", lim)
	}
}

func TestLoadWatcherLimits_EnvOverrideAndInvalid(t *testing.T) {
	t.Setenv("AGENT_LSP_WATCH_MAX_DIR_ENTRIES", "500")
	t.Setenv("AGENT_LSP_WATCH_MAX_ENTRIES", "10000")
	if lim := loadWatcherLimits(); lim.maxDirEntries != 500 || lim.maxEntries != 10000 {
		t.Errorf("env override not applied: %+v", lim)
	}
	// Invalid / non-positive values keep defaults.
	t.Setenv("AGENT_LSP_WATCH_MAX_DIR_ENTRIES", "notanumber")
	t.Setenv("AGENT_LSP_WATCH_MAX_ENTRIES", "-5")
	if lim := loadWatcherLimits(); lim.maxDirEntries != defaultWatchMaxDirEntries || lim.maxEntries != defaultWatchMaxEntries {
		t.Errorf("invalid values should keep defaults: %+v", lim)
	}
}

func TestOpenFDCount(t *testing.T) {
	n := openFDCount()
	// On darwin/linux this MUST work (it reads /dev/fd or /proc/self/fd); a -1
	// there means the runtime fd guard is a no-op on the platform the leak
	// happens on (issue #18), so fail rather than skip. Only genuinely
	// unsupported platforms (e.g. windows) are allowed to return -1.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if n < 3 {
			t.Fatalf("openFDCount() = %d on %s, expected >= 3 (stdio); the fd guard would not fire", n, runtime.GOOS)
		}
	} else if n == -1 {
		t.Skipf("open-fd count unsupported on %s", runtime.GOOS)
	}
	// Opening a file should raise the count.
	before := openFDCount()
	f, err := os.CreateTemp(t.TempDir(), "fd")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if after := openFDCount(); after <= before {
		t.Errorf("expected fd count to rise after opening a file: before=%d after=%d", before, after)
	}
}

func TestLoadWatchMaxFDs(t *testing.T) {
	t.Setenv("AGENT_LSP_WATCH_MAX_FDS", "")
	if got := loadWatchMaxFDs(); got != defaultWatchMaxFDs {
		t.Errorf("default = %d, want %d", got, defaultWatchMaxFDs)
	}
	t.Setenv("AGENT_LSP_WATCH_MAX_FDS", "1000")
	if got := loadWatchMaxFDs(); got != 1000 {
		t.Errorf("override = %d, want 1000", got)
	}
	t.Setenv("AGENT_LSP_WATCH_MAX_FDS", "-1")
	if got := loadWatchMaxFDs(); got != defaultWatchMaxFDs {
		t.Errorf("non-positive should keep default, got %d", got)
	}
}

func TestWatcherDisabled(t *testing.T) {
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("AGENT_LSP_DISABLE_WATCHER", v)
		if !watcherDisabled() {
			t.Errorf("expected disabled for %q", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "off"} {
		t.Setenv("AGENT_LSP_DISABLE_WATCHER", v)
		if watcherDisabled() {
			t.Errorf("expected enabled for %q", v)
		}
	}
}
