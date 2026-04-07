package session

import (
	"context"
	"sync"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// SessionStatus represents the lifecycle phase of a simulation session.
type SessionStatus string

const (
	StatusCreated    SessionStatus = "created"
	StatusMutated    SessionStatus = "mutated"
	StatusEvaluating SessionStatus = "evaluating"
	StatusEvaluated  SessionStatus = "evaluated"
	StatusCommitted  SessionStatus = "committed"
	StatusDiscarded  SessionStatus = "discarded"
	StatusDirty      SessionStatus = "dirty"
	StatusDestroyed  SessionStatus = "destroyed"
	StatusTimedOut   SessionStatus = "timed_out"
)

// Confidence indicates diagnostic reliability.
type Confidence string

const (
	ConfidenceHigh     Confidence = "high"
	ConfidencePartial  Confidence = "partial"
	ConfidenceStale    Confidence = "stale"
	ConfidenceEventual Confidence = "eventual"
)

// AppliedEdit records a single edit applied within a session.
type AppliedEdit struct {
	FileURI string
	Range   types.Range
	NewText string
	Version int
}

// DiagnosticsSnapshot is a frozen copy of diagnostics for a URI.
type DiagnosticsSnapshot struct {
	URI         string
	Diagnostics []types.LSPDiagnostic
	Confidence  Confidence
}

// EvaluationResult is the structured output of evaluate_session.
type EvaluationResult struct {
	SessionID        string            `json:"session_id"`
	ErrorsIntroduced []DiagnosticEntry `json:"errors_introduced"`
	ErrorsResolved   []DiagnosticEntry `json:"errors_resolved"`
	NetDelta         int               `json:"net_delta"`
	Scope            string            `json:"scope"`
	Confidence       Confidence        `json:"confidence"`
	Timeout          bool              `json:"timeout"`
	DurationMs       int64             `json:"duration_ms"`
}

// DiagnosticEntry is a single diagnostic in an evaluation result.
type DiagnosticEntry struct {
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Source   string `json:"source,omitempty"`
}

// EditResult is the response from simulate_edit.
type EditResult struct {
	SessionID    string `json:"session_id"`
	EditApplied  bool   `json:"edit_applied"`
	VersionAfter int    `json:"version_after"`
}

// ChainEdit is a single edit in a simulate_chain request.
type ChainEdit struct {
	FileURI string
	Range   types.Range
	NewText string
}

// ChainStepResult is one step in a simulate_chain response.
type ChainStepResult struct {
	Step             int               `json:"step"`
	NetDelta         int               `json:"net_delta"`
	ErrorsIntroduced []DiagnosticEntry `json:"errors_introduced"`
}

// ChainResult is the response from simulate_chain.
type ChainResult struct {
	SessionID              string            `json:"session_id"`
	Steps                  []ChainStepResult `json:"steps"`
	SafeToApplyThroughStep int               `json:"safe_to_apply_through_step"`
	CumulativeDelta        int               `json:"cumulative_delta"`
}

// CommitResult is the response from commit_session.
type CommitResult struct {
	SessionID    string      `json:"session_id"`
	FilesWritten int         `json:"files_written,omitempty"`
	Patch        interface{} `json:"patch"`
}

// SessionExecutor abstracts how a session acquires and releases LSP access.
type SessionExecutor interface {
	Acquire(ctx context.Context, session *SimulationSession) error
	Release(session *SimulationSession)
}

// SimulationSession holds per-session state.
type SimulationSession struct {
	ID        string
	Status    SessionStatus
	Client    *lsp.LSPClient
	Edits     []AppliedEdit
	Baselines map[string]DiagnosticsSnapshot
	Versions  map[string]int
	Contents  map[string]string // per-file current content (in-memory)
	Workspace string
	Language  string
	DirtyErr  error
	mu        sync.Mutex
}

// MarkDirty sets the session to dirty state with the given error.
func (s *SimulationSession) MarkDirty(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = StatusDirty
	s.DirtyErr = err
	logging.Log(logging.LevelError, "session.dirty: "+s.ID+": "+err.Error())
}

// IsDirty reports whether the session is in dirty state.
func (s *SimulationSession) IsDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Status == StatusDirty
}

// IsTerminal reports whether the session is in a terminal state.
func (s *SimulationSession) IsTerminal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Status == StatusCommitted || s.Status == StatusDiscarded ||
		s.Status == StatusDestroyed || s.Status == StatusDirty
}
