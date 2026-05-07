package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNeedsDaemon(t *testing.T) {
	cases := []struct {
		lang string
		want bool
	}{
		{"python", true},
		{"typescript", true},
		{"javascript", true},
		{"typescriptreact", true},
		{"javascriptreact", true},
		{"go", false},
		{"rust", false},
		{"c", false},
		{"java", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			if got := NeedsDaemon(tc.lang); got != tc.want {
				t.Errorf("NeedsDaemon(%q) = %v, want %v", tc.lang, got, tc.want)
			}
		})
	}
}

func TestDaemonDir(t *testing.T) {
	dir := DaemonDir("/some/root", "python")
	if !strings.Contains(dir, filepath.Join(".cache", "agent-lsp", "daemons")) {
		t.Errorf("DaemonDir path does not contain expected prefix: %s", dir)
	}

	// Different rootDir+languageID pairs produce different directories.
	dir2 := DaemonDir("/other/root", "python")
	dir3 := DaemonDir("/some/root", "typescript")
	if dir == dir2 {
		t.Errorf("expected different dirs for different rootDir, got same: %s", dir)
	}
	if dir == dir3 {
		t.Errorf("expected different dirs for different languageID, got same: %s", dir)
	}
}

func TestWriteAndRefreshDaemonInfo(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "test-project")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Millisecond)
	info := &DaemonInfo{
		RootDir:      rootDir,
		LanguageID:   "python",
		Command:      []string{"pyright-langserver", "--stdio"},
		SocketPath:   "/tmp/test.sock",
		PID:          12345,
		Ready:        true,
		StartTime:    now,
		LastActivity: now,
	}

	if err := WriteDaemonInfo(info); err != nil {
		t.Fatalf("WriteDaemonInfo: %v", err)
	}

	got, err := RefreshDaemonInfo(rootDir, "python")
	if err != nil {
		t.Fatalf("RefreshDaemonInfo: %v", err)
	}

	if got.RootDir != info.RootDir {
		t.Errorf("RootDir = %q, want %q", got.RootDir, info.RootDir)
	}
	if got.LanguageID != info.LanguageID {
		t.Errorf("LanguageID = %q, want %q", got.LanguageID, info.LanguageID)
	}
	if got.PID != info.PID {
		t.Errorf("PID = %d, want %d", got.PID, info.PID)
	}
	if got.Ready != info.Ready {
		t.Errorf("Ready = %v, want %v", got.Ready, info.Ready)
	}
	if got.SocketPath != info.SocketPath {
		t.Errorf("SocketPath = %q, want %q", got.SocketPath, info.SocketPath)
	}
	if len(got.Command) != len(info.Command) {
		t.Errorf("Command length = %d, want %d", len(got.Command), len(info.Command))
	}
}

func TestFindRunningDaemon_NoDaemon(t *testing.T) {
	// Use a unique rootDir that won't have any daemon state.
	tmpDir := t.TempDir()
	info, err := FindRunningDaemon(tmpDir, "python")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestCleanupStaleDaemons(t *testing.T) {
	// Set up a fake daemon dir under the real daemons path.
	home, _ := os.UserHomeDir()
	daemonsDir := filepath.Join(home, ".cache", "agent-lsp", "daemons")
	fakeDir := filepath.Join(daemonsDir, "test-stale-cleanup-"+t.Name())
	if err := os.MkdirAll(fakeDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(fakeDir) })

	// Write a PID file with a PID that definitely doesn't exist.
	pidFile := filepath.Join(fakeDir, "daemon.pid")
	if err := os.WriteFile(pidFile, []byte("999999999"), 0644); err != nil {
		t.Fatal(err)
	}

	CleanupStaleDaemons()

	if _, err := os.Stat(fakeDir); !os.IsNotExist(err) {
		t.Errorf("stale daemon dir should have been removed, but still exists")
	}
}

func TestListDaemons_Empty(t *testing.T) {
	// ListDaemons only returns daemons with alive processes.
	// With no actual daemon processes running for our test dirs, we expect empty/nil.
	// Create a fake daemon dir with a dead PID to ensure it's filtered out.
	home, _ := os.UserHomeDir()
	daemonsDir := filepath.Join(home, ".cache", "agent-lsp", "daemons")
	fakeDir := filepath.Join(daemonsDir, "test-list-empty-"+t.Name())
	if err := os.MkdirAll(fakeDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(fakeDir) })

	info := DaemonInfo{
		RootDir:    "/fake/root",
		LanguageID: "python",
		PID:        999999999, // not a real process
	}
	data, _ := json.Marshal(info)
	os.WriteFile(filepath.Join(fakeDir, "daemon.json"), data, 0644)

	result := ListDaemons()
	// The fake daemon has a dead PID, so it should not appear.
	for _, d := range result {
		if d.PID == 999999999 {
			t.Errorf("dead daemon should not appear in ListDaemons results")
		}
	}
}
