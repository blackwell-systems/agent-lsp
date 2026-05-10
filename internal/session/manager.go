// manager.go implements the SessionManager which controls the lifecycle of
// simulation sessions for speculative execution.
//
// A simulation session snapshots the current diagnostic state from the LSP
// server, then tracks virtual edits applied by the agent. When the agent
// evaluates the session, the manager:
//  1. Applies each edit to the real LSP server (via didChange/didOpen).
//  2. Waits for diagnostics to settle.
//  3. Computes the diagnostic delta (new errors minus baseline errors).
//  4. Reverts the LSP server state (via ReopenDocument from disk).
//
// If the agent commits, edits are written to disk and the LSP state is left
// updated. If the agent discards, everything reverts to the baseline.
//
// Thread safety: all public methods acquire m.mu, and per-session operations
// are serialized through the SessionExecutor to prevent interleaved evaluations
// from corrupting LSP state.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"sync"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	internaluri "github.com/blackwell-systems/agent-lsp/internal/uri"
)

// SessionManager manages the lifecycle of simulation sessions.
// All public methods are thread-safe.
type SessionManager struct {
	sessions map[string]*SimulationSession
	executor SessionExecutor
	resolver lsp.ClientResolver
	mu       sync.RWMutex
}

// NewSessionManager creates a new SessionManager with the given resolver.
func NewSessionManager(resolver lsp.ClientResolver) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*SimulationSession),
		executor: NewSerializedExecutor(),
		resolver: resolver,
	}
}

// CreateSession creates a new simulation session and returns its ID.
func (m *SessionManager) CreateSession(ctx context.Context, workspaceRoot, language string) (string, error) {
	// Generate session ID using crypto/rand.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session ID: %w", err)
	}
	id := hex.EncodeToString(b)

	// Resolve LSP client. Use ClientForFile with a language-derived extension
	// so multi-server mode picks the right server (gopls for .go, clangd for .c).
	// After start_lsp calls StartAll, all delegate clients are initialized.
	ext := languageToExtension(language)
	client := m.resolver.ClientForFile(workspaceRoot + "/dummy" + ext)
	if client == nil {
		return "", fmt.Errorf("no LSP client available — call start_lsp first")
	}

	// Create new session.
	session := &SimulationSession{
		ID:               id,
		Status:           StatusCreated,
		Client:           client,
		Edits:            []AppliedEdit{},
		Baselines:        make(map[string]DiagnosticsSnapshot),
		Versions:         make(map[string]int),
		Contents:         make(map[string]string),
		OriginalContents: make(map[string]string),
		Workspace:        workspaceRoot,
		Language:         language,
	}

	// Store in sessions map under write lock.
	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	logging.Log(logging.LevelDebug, "session.created: "+id)
	return id, nil
}

// GetSession retrieves a session by ID.
func (m *SessionManager) GetSession(sessionID string) (*SimulationSession, error) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return session, nil
}

// ApplyEdit applies a range edit to a file within a session.
func (m *SessionManager) ApplyEdit(ctx context.Context, sessionID, fileURI string, rng types.Range, newText string) (*EditResult, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Check session is not terminal or dirty.
	if session.IsTerminal() {
		return nil, fmt.Errorf("session %s is in terminal state: %s", sessionID, session.Status)
	}
	if session.IsDirty() {
		return nil, fmt.Errorf("session %s is dirty: %w", sessionID, session.DirtyError())
	}

	// Acquire executor lock for serialized LSP access.
	if err := m.executor.Acquire(ctx, session); err != nil {
		return nil, fmt.Errorf("acquiring executor: %w", err)
	}
	defer m.executor.Release(session)

	// Lazy baseline: establish baseline for this file if not already set.
	if _, exists := session.Baselines[fileURI]; !exists {
		// Wait for diagnostics to stabilize.
		if err := lsp.WaitForDiagnostics(ctx, session.Client, []string{fileURI}, 3000); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("session.baseline_timeout: %s file=%s", sessionID, fileURI))
		}

		// Snapshot diagnostics.
		diagnostics := session.Client.GetDiagnostics(fileURI)
		session.Baselines[fileURI] = DiagnosticsSnapshot{
			URI:         fileURI,
			Diagnostics: diagnostics,
			Confidence:  ConfidenceHigh,
		}

		// Read file content from disk.
		path := internaluri.URIToPath(fileURI)
		content, err := os.ReadFile(path)
		if err != nil {
			session.MarkDirty(fmt.Errorf("reading file %s: %w", path, err))
			return nil, fmt.Errorf("reading file %s: %w", path, err)
		}
		session.Contents[fileURI] = string(content)

		// Snapshot original content for Discard revert (only on first edit per file).
		if _, alreadySnapped := session.OriginalContents[fileURI]; !alreadySnapped {
			session.OriginalContents[fileURI] = string(content)
		}

		// Open the document in LSP.
		if err := session.Client.OpenDocument(ctx, fileURI, session.Contents[fileURI], session.Language); err != nil {
			session.MarkDirty(fmt.Errorf("opening document %s: %w", fileURI, err))
			return nil, fmt.Errorf("opening document %s: %w", fileURI, err)
		}
	}

	// Increment version.
	session.Versions[fileURI]++

	// Apply range edit in-memory.
	session.Contents[fileURI] = applyRangeEdit(session.Contents[fileURI], rng, newText)

	// Send to LSP via OpenDocument (which sends didChange when already open).
	if err := session.Client.OpenDocument(ctx, fileURI, session.Contents[fileURI], session.Language); err != nil {
		session.MarkDirty(fmt.Errorf("sending didChange for %s: %w", fileURI, err))
		return nil, fmt.Errorf("sending didChange for %s: %w", fileURI, err)
	}

	// Record edit.
	session.Edits = append(session.Edits, AppliedEdit{
		FileURI: fileURI,
		Range:   rng,
		NewText: newText,
		Version: session.Versions[fileURI],
	})

	// Update status.
	session.SetStatus(StatusMutated)

	logging.Log(logging.LevelDebug, fmt.Sprintf("session.edit_applied: %s file=%s version=%d", sessionID, fileURI, session.Versions[fileURI]))

	return &EditResult{
		SessionID:    sessionID,
		EditApplied:  true,
		VersionAfter: session.Versions[fileURI],
	}, nil
}

// Evaluate evaluates the current session state against baselines.
func (m *SessionManager) Evaluate(ctx context.Context, sessionID, scope string, timeoutMs int) (*EvaluationResult, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Apply defaults.
	if scope == "" {
		scope = "file"
	}
	if timeoutMs == 0 {
		if scope == "file" {
			timeoutMs = 3000
		} else {
			timeoutMs = 8000
		}
	}

	// Verify status is valid for evaluation — read under lock to avoid data race.
	// Use go test -race to catch regressions if this guard is changed.
	session.mu.Lock()
	currentStatus := session.Status
	session.mu.Unlock()
	if currentStatus != StatusMutated && currentStatus != StatusEvaluated {
		return nil, fmt.Errorf("session %s cannot be evaluated in state %s", sessionID, currentStatus)
	}

	// Acquire executor lock before marking evaluating — a failed acquire must
	// leave the session retryable (StatusMutated or StatusEvaluated), not
	// permanently stuck in StatusEvaluating.
	if err := m.executor.Acquire(ctx, session); err != nil {
		return nil, fmt.Errorf("acquiring executor: %w", err)
	}
	defer m.executor.Release(session)

	// Update status to evaluating only after acquisition succeeds.
	session.SetStatus(StatusEvaluating)

	start := time.Now()
	var allIntroduced, allResolved []DiagnosticEntry
	timedOut := false

	// Collect URIs to wait for.
	var uris []string
	for uri := range session.Baselines {
		uris = append(uris, uri)
	}

	// Wait for diagnostics to stabilize.
	if err := lsp.WaitForDiagnostics(ctx, session.Client, uris, timeoutMs); err != nil {
		logging.Log(logging.LevelWarning, fmt.Sprintf("session.evaluate_timeout: %s", sessionID))
		timedOut = true
	}

	// For each file in baselines, compute diff.
	for uri, baseline := range session.Baselines {
		current := session.Client.GetDiagnostics(uri)
		introduced, resolved := DiffDiagnostics(baseline.Diagnostics, current)
		allIntroduced = append(allIntroduced, introduced...)
		allResolved = append(allResolved, resolved...)
	}

	netDelta := len(allIntroduced) - len(allResolved)

	// Determine confidence.
	confidence := ConfidenceHigh
	if scope == "workspace" {
		confidence = ConfidenceEventual
	}
	if timedOut {
		confidence = ConfidencePartial
	}

	// Update status.
	session.SetStatus(StatusEvaluated)

	logging.Log(logging.LevelDebug, fmt.Sprintf("session.evaluate_complete: %s scope=%s netDelta=%d", sessionID, scope, netDelta))

	return &EvaluationResult{
		SessionID:        sessionID,
		ErrorsIntroduced: allIntroduced,
		ErrorsResolved:   allResolved,
		NetDelta:         netDelta,
		Scope:            scope,
		Confidence:       confidence,
		Timeout:          timedOut,
		DurationMs:       time.Since(start).Milliseconds(),
	}, nil
}

// SimulateChain applies a sequence of edits and evaluates after each one.
func (m *SessionManager) SimulateChain(ctx context.Context, sessionID string, edits []ChainEdit, timeoutMs int) (*ChainResult, error) {
	var steps []ChainStepResult

	for i, edit := range edits {
		// Apply edit.
		if _, err := m.ApplyEdit(ctx, sessionID, edit.FileURI, edit.Range, edit.NewText); err != nil {
			return nil, fmt.Errorf("applying edit at step %d: %w", i+1, err)
		}

		// Evaluate.
		evalResult, err := m.Evaluate(ctx, sessionID, "file", timeoutMs)
		if err != nil {
			return nil, fmt.Errorf("evaluating at step %d: %w", i+1, err)
		}

		// Record step result.
		steps = append(steps, ChainStepResult{
			Step:             i + 1,
			NetDelta:         evalResult.NetDelta,
			ErrorsIntroduced: evalResult.ErrorsIntroduced,
		})
	}

	// Compute SafeToApplyThroughStep: find last step where NetDelta==0.
	safeStep := 0
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].NetDelta == 0 {
			safeStep = steps[i].Step
			break
		}
	}

	// Cumulative delta is the final step's NetDelta.
	cumulativeDelta := 0
	if len(steps) > 0 {
		cumulativeDelta = steps[len(steps)-1].NetDelta
	}

	return &ChainResult{
		SessionID:              sessionID,
		Steps:                  steps,
		SafeToApplyThroughStep: safeStep,
		CumulativeDelta:        cumulativeDelta,
	}, nil
}

// Commit commits the session changes to disk or returns a patch.
func (m *SessionManager) Commit(ctx context.Context, sessionID, target string, apply bool) (*CommitResult, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Verify status is valid for commit — read under lock to avoid data race.
	// Use go test -race to catch regressions if this guard is changed.
	session.mu.Lock()
	currentStatus := session.Status
	session.mu.Unlock()
	if currentStatus != StatusMutated && currentStatus != StatusEvaluated {
		return nil, fmt.Errorf("session %s cannot be committed in state %s", sessionID, currentStatus)
	}

	// Build workspace edit patch.
	patch := make(map[string]string, len(session.Contents))
	maps.Copy(patch, session.Contents)

	filesWritten := 0

	// If apply=true, write files to disk.
	if apply {
		for fileURI, content := range session.Contents {
			path := internaluri.URIToPath(fileURI)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				session.MarkDirty(fmt.Errorf("writing file %s: %w", path, err))
				return nil, fmt.Errorf("writing file %s: %w", path, err)
			}
			filesWritten++

			// Notify LSP of the change by sending didChange.
			// If notification fails, mark session dirty — LSP state is now stale
			// relative to disk content and callers must be informed.
			if err := session.Client.OpenDocument(ctx, fileURI, content, session.Language); err != nil {
				session.MarkDirty(fmt.Errorf("LSP notification failed after commit for %s: %w", fileURI, err))
				logging.Log(logging.LevelWarning, fmt.Sprintf("notifying LSP of committed change for %s: %v", fileURI, err))
			}
		}
	}

	// Update status.
	session.SetStatus(StatusCommitted)

	logging.Log(logging.LevelDebug, fmt.Sprintf("session.committed: %s apply=%v files=%d", sessionID, apply, filesWritten))

	return &CommitResult{
		SessionID:    sessionID,
		FilesWritten: filesWritten,
		Patch:        patch,
	}, nil
}

// Discard discards session changes and reverts LSP state to original.
func (m *SessionManager) Discard(ctx context.Context, sessionID string) error {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("discard: %w", err)
	}

	// Check session is not terminal.
	if session.IsTerminal() {
		return fmt.Errorf("session %s is already in terminal state: %s", sessionID, session.Status)
	}

	// Acquire executor lock.
	if err := m.executor.Acquire(ctx, session); err != nil {
		return fmt.Errorf("acquiring executor: %w", err)
	}
	defer m.executor.Release(session)

	// Revert each file to original content.
	for uri := range session.Contents {
		originalContent, ok := session.OriginalContents[uri]
		if !ok {
			// Fallback: if no snapshot exists (session created but no edit applied),
			// nothing to revert for this URI.
			continue
		}

		// Send original content back to LSP.
		if err := session.Client.OpenDocument(ctx, uri, originalContent, session.Language); err != nil {
			session.MarkDirty(fmt.Errorf("reverting document %s: %w", uri, err))
			return fmt.Errorf("reverting document %s: %w", uri, err)
		}
	}

	// Update status.
	session.SetStatus(StatusDiscarded)

	logging.Log(logging.LevelDebug, "session.discarded: "+sessionID)
	return nil
}

// Destroy destroys a session and removes it from the manager.
func (m *SessionManager) Destroy(ctx context.Context, sessionID string) error {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("destroy: %w", err)
	}

	// Write-lock sessions map.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update status and remove from map.
	session.SetStatus(StatusDestroyed)
	delete(m.sessions, sessionID)

	logging.Log(logging.LevelDebug, "session.destroyed: "+sessionID)
	return nil
}

// applyRangeEdit applies a range edit to content in-memory and returns the new content.
// Delegates to internaluri.ApplyRangeEdit — canonical shared implementation (L5).
func applyRangeEdit(content string, rng types.Range, newText string) string {
	return internaluri.ApplyRangeEdit(content, rng, newText)
}

// languageToExtension maps a language ID to a file extension for ClientForFile routing.
func languageToExtension(language string) string {
	switch language {
	case "go":
		return ".go"
	case "python":
		return ".py"
	case "typescript":
		return ".ts"
	case "javascript":
		return ".js"
	case "rust":
		return ".rs"
	case "c":
		return ".c"
	case "cpp", "c++":
		return ".cpp"
	case "java":
		return ".java"
	case "ruby":
		return ".rb"
	default:
		return "." + language
	}
}
