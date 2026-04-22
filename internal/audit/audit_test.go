package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLoggerWritesValidJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	logger, err := NewLogger(path, 16)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Log(Record{
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Tool:       "apply_edit",
		Files:      []string{"/tmp/foo.go"},
		Success:    true,
		DurationMs: 42,
	})
	logger.Log(Record{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Tool:         "rename_symbol",
		Files:        []string{"/tmp/bar.go"},
		Success:      false,
		ErrorMessage: "rename failed",
		DurationMs:   100,
	})

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), string(data))
	}

	for i, line := range lines {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestLogNonBlockingWhenFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	// Buffer size 1 — fill it, then verify Log doesn't block.
	logger, err := NewLogger(path, 1)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	// Fill the buffer. We need to pause the writer goroutine somehow, but
	// the simplest check is just to call Log many times rapidly and ensure
	// it returns within a deadline.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			logger.Log(Record{
				Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
				Tool:       "test",
				DurationMs: int64(i),
			})
		}
		close(done)
	}()

	select {
	case <-done:
		// OK — Log didn't deadlock.
	case <-time.After(2 * time.Second):
		t.Fatal("Log blocked — potential deadlock with full buffer")
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestCloseDrainsRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	logger, err := NewLogger(path, 256)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	for i := 0; i < 10; i++ {
		logger.Log(Record{
			Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
			Tool:       "test",
			DurationMs: int64(i),
		})
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines after drain, got %d", len(lines))
	}
}

func TestResolvePathEnvOverride(t *testing.T) {
	t.Setenv("AGENT_LSP_AUDIT_LOG", "/custom/audit.jsonl")
	got := ResolvePath("")
	if got != "/custom/audit.jsonl" {
		t.Errorf("ResolvePath with env: got %q, want /custom/audit.jsonl", got)
	}
}

func TestResolvePathFlagTakesPrecedence(t *testing.T) {
	t.Setenv("AGENT_LSP_AUDIT_LOG", "/env/audit.jsonl")
	got := ResolvePath("/flag/audit.jsonl")
	if got != "/flag/audit.jsonl" {
		t.Errorf("ResolvePath with flag: got %q, want /flag/audit.jsonl", got)
	}
}

func TestNoOpLoggerWhenPathEmpty(t *testing.T) {
	t.Setenv("AGENT_LSP_AUDIT_LOG", "")
	// ResolvePath returns default (~/.agent-lsp/audit.jsonl), but
	// we test the no-op path by passing empty string directly to NewLogger.
	logger, err := NewLogger("", 16)
	if err != nil {
		t.Fatalf("NewLogger with empty path: %v", err)
	}
	// Should not panic or block.
	logger.Log(Record{Tool: "test"})
	if err := logger.Close(); err != nil {
		t.Fatalf("Close on no-op logger: %v", err)
	}
}

func TestTruncate(t *testing.T) {
	s := strings.Repeat("a", 300)
	got := Truncate(s, 200)
	if len(got) != 200 {
		t.Errorf("Truncate: got len %d, want 200", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("Truncate: expected ... suffix")
	}

	short := "hello"
	if Truncate(short, 200) != short {
		t.Error("Truncate should not modify short strings")
	}
}
