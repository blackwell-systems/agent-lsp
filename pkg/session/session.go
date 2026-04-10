package session

import (
	"context"

	internalsession "github.com/blackwell-systems/agent-lsp/internal/session"
)

// SessionManager manages the lifecycle of simulation sessions.
// All public methods are thread-safe.
type SessionManager = internalsession.SessionManager

// SimulationSession holds per-session state.
type SimulationSession = internalsession.SimulationSession

// SessionStatus represents the lifecycle phase of a simulation session.
type SessionStatus = internalsession.SessionStatus

// Confidence indicates diagnostic reliability.
type Confidence = internalsession.Confidence

// AppliedEdit records a single edit applied within a session.
type AppliedEdit = internalsession.AppliedEdit

// DiagnosticsSnapshot is a frozen copy of diagnostics for a URI at baseline time.
type DiagnosticsSnapshot = internalsession.DiagnosticsSnapshot

// EvaluationResult is the structured output of SessionManager.Evaluate.
type EvaluationResult = internalsession.EvaluationResult

// DiagnosticEntry is a single diagnostic in an evaluation result.
type DiagnosticEntry = internalsession.DiagnosticEntry

// EditResult is the response from SessionManager.ApplyEdit.
type EditResult = internalsession.EditResult

// ChainEdit is a single edit in a SimulateChain request.
type ChainEdit = internalsession.ChainEdit

// ChainStepResult is one step in a SimulateChain response.
type ChainStepResult = internalsession.ChainStepResult

// ChainResult is the response from SessionManager.SimulateChain.
type ChainResult = internalsession.ChainResult

// CommitResult is the response from SessionManager.Commit.
type CommitResult = internalsession.CommitResult

// SessionExecutor abstracts how a session acquires and releases LSP access.
type SessionExecutor interface {
	Acquire(ctx context.Context, session *SimulationSession) error
	Release(session *SimulationSession)
}

// Session status constants.
const (
	StatusCreated    = internalsession.StatusCreated
	StatusMutated    = internalsession.StatusMutated
	StatusEvaluating = internalsession.StatusEvaluating
	StatusEvaluated  = internalsession.StatusEvaluated
	StatusCommitted  = internalsession.StatusCommitted
	StatusDiscarded  = internalsession.StatusDiscarded
	StatusDirty      = internalsession.StatusDirty
	StatusDestroyed  = internalsession.StatusDestroyed
)

// Confidence level constants.
const (
	ConfidenceHigh     = internalsession.ConfidenceHigh
	ConfidencePartial  = internalsession.ConfidencePartial
	ConfidenceEventual = internalsession.ConfidenceEventual
)

// NewSessionManager creates a new SessionManager with the given ClientResolver.
// The resolver is used to route session operations to the correct LSP client
// based on the session's language.
//
// resolver must satisfy the ClientResolver interface from pkg/lsp or
// internal/lsp — either works because LSPClient is a type alias.
var NewSessionManager = internalsession.NewSessionManager

// NewSerializedExecutor creates a new per-session serialized executor.
var NewSerializedExecutor = internalsession.NewSerializedExecutor
