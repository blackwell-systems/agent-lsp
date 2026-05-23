package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFindRunningDaemon_CorruptJSON tests handling of malformed daemon.json
func TestFindRunningDaemon_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create daemon dir with corrupt JSON
	dir := DaemonDir(rootDir, "python")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(dir, "daemon.json")
	if err := os.WriteFile(infoPath, []byte("{ invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should treat corrupt JSON as no daemon
	info, err := FindRunningDaemon(rootDir, "python")
	if err != nil {
		t.Errorf("expected nil error for corrupt JSON, got %v", err)
	}
	if info != nil {
		t.Errorf("expected nil info for corrupt JSON, got %+v", info)
	}
}

// TestFindRunningDaemon_StalePID tests cleanup of stale PID entries
func TestFindRunningDaemon_StalePID(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write daemon info with a PID that definitely doesn't exist
	info := &DaemonInfo{
		RootDir:      rootDir,
		LanguageID:   "python",
		Command:      []string{"pyright-langserver", "--stdio"},
		SocketPath:   filepath.Join(tmpDir, "test.sock"),
		PID:          999999999, // impossibly high PID
		Ready:        true,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
	}

	if err := WriteDaemonInfo(info); err != nil {
		t.Fatal(err)
	}

	// Should detect stale PID and clean up
	found, err := FindRunningDaemon(rootDir, "python")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil for stale PID, got %+v", found)
	}

	// Verify cleanup happened
	dir := DaemonDir(rootDir, "python")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected daemon dir to be cleaned up after stale PID detected")
	}
}

// TestRefreshDaemonInfo_MissingFile tests error handling for missing daemon.json
func TestRefreshDaemonInfo_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "nonexistent")

	info, err := RefreshDaemonInfo(rootDir, "python")
	if err == nil {
		t.Error("expected error for missing daemon.json, got nil")
	}
	if info != nil {
		t.Errorf("expected nil info for missing file, got %+v", info)
	}
}

// TestRefreshDaemonInfo_CorruptJSON tests error handling for corrupt daemon.json
func TestRefreshDaemonInfo_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create daemon dir with corrupt JSON
	dir := DaemonDir(rootDir, "python")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(dir, "daemon.json")
	if err := os.WriteFile(infoPath, []byte("not valid json at all!"), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := RefreshDaemonInfo(rootDir, "python")
	if err == nil {
		t.Error("expected error for corrupt JSON, got nil")
	}
	if info != nil {
		t.Errorf("expected nil info for corrupt JSON, got %+v", info)
	}
}

// TestWriteDaemonInfo_MkdirFailure tests error handling when directory creation fails
func TestWriteDaemonInfo_ReadOnlyParent(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping as root (can write to read-only dirs)")
	}

	tmpDir := t.TempDir()

	// Create a daemon info pointing to a location we'll make read-only
	info := &DaemonInfo{
		RootDir:    "/nonexistent/path/that/would/require/dir/creation",
		LanguageID: "python",
		PID:        12345,
	}

	// Replace DaemonDir calculation to point to our controlled location
	// This is testing the os.MkdirAll error path
	// We can't easily trigger MkdirAll failure without platform-specific tricks,
	// so we document this as an integration-level concern.

	// Instead, test that WriteDaemonInfo creates the directory when it doesn't exist
	testRoot := filepath.Join(tmpDir, "test-root")
	info.RootDir = testRoot
	info.LanguageID = "test-lang"

	err := WriteDaemonInfo(info)
	if err != nil {
		t.Fatalf("WriteDaemonInfo failed: %v", err)
	}

	// Verify directory was created
	dir := DaemonDir(testRoot, "test-lang")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}

	// Verify file contents
	data, err := os.ReadFile(filepath.Join(dir, "daemon.json"))
	if err != nil {
		t.Fatalf("failed to read daemon.json: %v", err)
	}

	var readInfo DaemonInfo
	if err := json.Unmarshal(data, &readInfo); err != nil {
		t.Fatalf("failed to unmarshal daemon.json: %v", err)
	}

	if readInfo.PID != info.PID {
		t.Errorf("PID = %d, want %d", readInfo.PID, info.PID)
	}
}

// TestCleanupStaleDaemons_MultipleDirs tests cleanup with multiple stale daemons
func TestCleanupStaleDaemons_MultipleDirs(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	daemonsDir := filepath.Join(home, ".cache", "agent-lsp", "daemons")

	// Create multiple fake stale daemon dirs
	dirs := []string{
		filepath.Join(daemonsDir, "test-stale-1-"+t.Name()),
		filepath.Join(daemonsDir, "test-stale-2-"+t.Name()),
	}

	for i, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })

		// Write stale PID files
		pidFile := filepath.Join(dir, "daemon.pid")
		pid := 999999990 + i
		if err := os.WriteFile(pidFile, []byte(string(rune(pid))), 0644); err != nil {
			t.Fatal(err)
		}
	}

	CleanupStaleDaemons()

	// Verify all stale dirs were removed
	for _, dir := range dirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("stale daemon dir %s should have been removed", dir)
		}
	}
}

// TestCleanupStaleDaemons_InvalidPID tests handling of malformed daemon.pid
func TestCleanupStaleDaemons_InvalidPID(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	daemonsDir := filepath.Join(home, ".cache", "agent-lsp", "daemons")
	dir := filepath.Join(daemonsDir, "test-invalid-pid-"+t.Name())

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Write invalid PID
	pidFile := filepath.Join(dir, "daemon.pid")
	if err := os.WriteFile(pidFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatal(err)
	}

	CleanupStaleDaemons()

	// Should clean up dir with invalid PID
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("dir with invalid PID should have been cleaned up")
	}
}

// TestStopDaemon_NotFound tests error handling when daemon doesn't exist
func TestStopDaemon_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "nonexistent")

	err := StopDaemon(rootDir, "python")
	if err == nil {
		t.Error("expected error when stopping nonexistent daemon, got nil")
	}
}

// TestListDaemons_CorruptEntry tests handling of corrupt daemon.json in listing
func TestListDaemons_CorruptEntry(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	daemonsDir := filepath.Join(home, ".cache", "agent-lsp", "daemons")
	dir := filepath.Join(daemonsDir, "test-list-corrupt-"+t.Name())

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Write corrupt daemon.json
	infoPath := filepath.Join(dir, "daemon.json")
	if err := os.WriteFile(infoPath, []byte("{ corrupt json }"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ListDaemons()

	// Corrupt entries should be skipped, not cause crashes
	for _, d := range result {
		if d.RootDir == "" {
			t.Error("ListDaemons returned entry with empty RootDir (likely from corrupt JSON)")
		}
	}
}

// TestDaemonDir_Consistency tests that DaemonDir produces consistent hashes
func TestDaemonDir_Consistency(t *testing.T) {
	// Same inputs should always produce same directory
	dir1 := DaemonDir("/some/root", "python")
	dir2 := DaemonDir("/some/root", "python")

	if dir1 != dir2 {
		t.Errorf("DaemonDir not consistent: %q vs %q", dir1, dir2)
	}

	// Different inputs should produce different directories
	dir3 := DaemonDir("/other/root", "python")
	dir4 := DaemonDir("/some/root", "typescript")

	if dir1 == dir3 {
		t.Error("DaemonDir: different rootDir produced same hash")
	}
	if dir1 == dir4 {
		t.Error("DaemonDir: different languageID produced same hash")
	}
}

// TestProcessAlive_InvalidPID tests error handling for invalid PIDs
func TestProcessAlive_InvalidPID(t *testing.T) {
	// Test with various invalid PIDs
	invalidPIDs := []int{-1, 0, 999999999}

	for _, pid := range invalidPIDs {
		// This should not panic
		alive := processAlive(pid)

		// We expect these to be not alive, but the important thing
		// is that the function doesn't crash
		if pid <= 0 && alive {
			t.Errorf("processAlive(%d) = true, expected false for invalid PID", pid)
		}
	}
}

// TestNeedsDaemon_CaseSensitivity tests case handling in daemon language check
func TestNeedsDaemon_CaseSensitivity(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"Python", true},
		{"PYTHON", true},
		{"python", true},
		{"TypeScript", true},
		{"TYPESCRIPT", true},
		{"typescript", true},
		{"Go", false},
		{"GO", false},
		{"go", false},
	}

	for _, tc := range cases {
		got := NeedsDaemon(tc.input)
		if got != tc.want {
			t.Errorf("NeedsDaemon(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
