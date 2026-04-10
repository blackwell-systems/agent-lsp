package session_test

import (
	"context"
	"testing"

	internallsp "github.com/blackwell-systems/agent-lsp/internal/lsp"
	internalsession "github.com/blackwell-systems/agent-lsp/internal/session"
	pubsession "github.com/blackwell-systems/agent-lsp/pkg/session"
)

// Compile-time assertion: *SerializedExecutor satisfies SessionExecutor.
var _ pubsession.SessionExecutor = (*internalsession.SerializedExecutor)(nil)

// TestPkgSessionCompileSmoke verifies that pkg/session re-exports are type-compatible
// with their internal/session counterparts. No LSP binary is started.
func TestPkgSessionCompileSmoke(t *testing.T) {
	t.Skip("compile smoke only — verifies type aliases compile correctly")

	// NewSerializedExecutor: func() *SerializedExecutor
	var newExecFn func() *internalsession.SerializedExecutor = pubsession.NewSerializedExecutor
	_ = newExecFn

	// NewSessionManager: func(lsp.ClientResolver) *SessionManager
	// internallsp.ClientResolver and pkg/lsp.ClientResolver are the same interface
	// via type alias; use the internal form here to avoid a cross-pkg/ import.
	var newMgrFn func(internallsp.ClientResolver) *pubsession.SessionManager = pubsession.NewSessionManager
	_ = newMgrFn

	// Type alias identity.
	var _ *internalsession.SessionManager = (*pubsession.SessionManager)(nil)
	var _ *internalsession.SimulationSession = (*pubsession.SimulationSession)(nil)
	var _ pubsession.SessionStatus = internalsession.StatusCreated

	// SessionExecutor interface method signatures.
	var exec pubsession.SessionExecutor
	var sess *pubsession.SimulationSession
	var ctx context.Context = context.Background()
	_ = exec.Acquire(ctx, sess)
	exec.Release(sess)
}
