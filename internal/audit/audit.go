package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// Record represents a single audit trail entry written as JSONL.
type Record struct {
	Timestamp         string           `json:"timestamp"`
	Tool              string           `json:"tool"`
	SessionID         string           `json:"session_id,omitempty"`
	Files             []string         `json:"files"`
	EditSummary       *EditSummary     `json:"edit_summary,omitempty"`
	DiagnosticsBefore *DiagnosticState `json:"diagnostics_before,omitempty"`
	DiagnosticsAfter  *DiagnosticState `json:"diagnostics_after,omitempty"`
	NetDelta          *DiagnosticDelta `json:"net_delta,omitempty"`
	Success           bool             `json:"success"`
	ErrorMessage      string           `json:"error_message,omitempty"`
	DurationMs        int64            `json:"duration_ms"`
}

// EditSummary captures a summary of the edit operation.
type EditSummary struct {
	Mode           string `json:"mode"`
	FilePath       string `json:"file_path,omitempty"`
	OldTextPreview string `json:"old_text_preview,omitempty"`
	NewTextPreview string `json:"new_text_preview,omitempty"`
	OldName        string `json:"old_name,omitempty"`
	NewName        string `json:"new_name,omitempty"`
	Target         string `json:"target,omitempty"`
	Apply          bool   `json:"apply,omitempty"`
}

// DiagnosticState captures diagnostic counts at a point in time.
type DiagnosticState struct {
	ErrorCount   int      `json:"error_count"`
	WarningCount int      `json:"warning_count"`
	FilesChecked []string `json:"files_checked"`
}

// DiagnosticDelta captures the change in diagnostics.
type DiagnosticDelta struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// Logger writes audit records to a JSONL file via a buffered channel.
type Logger struct {
	ch   chan Record
	file *os.File
	done chan struct{}
	noop bool
}

// NewLogger creates an audit logger that writes JSONL to path.
// If path is empty, returns a no-op logger that discards records.
// bufSize controls the channel buffer size.
func NewLogger(path string, bufSize int) (*Logger, error) {
	if path == "" {
		return &Logger{noop: true}, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("audit: create directory %s: %w", dir, err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}

	l := &Logger{
		ch:   make(chan Record, bufSize),
		file: f,
		done: make(chan struct{}),
	}

	go l.writeLoop()
	return l, nil
}

// writeLoop drains the channel and writes records to the file.
func (l *Logger) writeLoop() {
	defer close(l.done)
	enc := json.NewEncoder(l.file)
	for r := range l.ch {
		if err := enc.Encode(r); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("audit: write error: %v", err))
		}
	}
}

// Log sends a record to the audit log. Non-blocking; drops the record
// if the buffer is full.
func (l *Logger) Log(r Record) {
	if l.noop {
		return
	}
	select {
	case l.ch <- r:
	default:
		logging.Log(logging.LevelWarning, "audit: buffer full, dropping record")
	}
}

// Close closes the channel, drains remaining records, and closes the file.
func (l *Logger) Close() error {
	if l.noop {
		return nil
	}
	close(l.ch)
	<-l.done
	return l.file.Close()
}

// mu guards concurrent ResolvePath calls (env reads are safe but this
// keeps the function deterministic under test).
var mu sync.Mutex

// ResolvePath returns the audit log file path. Priority:
// 1. flagValue (from --audit-log flag)
// 2. AGENT_LSP_AUDIT_LOG environment variable
// 3. ~/.agent-lsp/audit.jsonl
func ResolvePath(flagValue string) string {
	mu.Lock()
	defer mu.Unlock()

	if flagValue != "" {
		return flagValue
	}
	if ev := os.Getenv("AGENT_LSP_AUDIT_LOG"); ev != "" {
		return ev
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agent-lsp", "audit.jsonl")
}

// Truncate caps s at max characters, appending "..." if truncated.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
