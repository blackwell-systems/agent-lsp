package session

import (
	"context"
	"sync"
)

// SerializedExecutor is the V1 implementation of SessionExecutor.
// It serializes all session operations through a single mutex,
// ensuring only one session mutates LSP state at a time.
type SerializedExecutor struct {
	mu sync.Mutex
}

// NewSerializedExecutor creates a new SerializedExecutor.
func NewSerializedExecutor() *SerializedExecutor {
	return &SerializedExecutor{}
}

// Acquire locks the executor for exclusive session access.
func (e *SerializedExecutor) Acquire(ctx context.Context, s *SimulationSession) error {
	e.mu.Lock()
	return nil
}

// Release unlocks the executor after session operation completes.
func (e *SerializedExecutor) Release(s *SimulationSession) {
	e.mu.Unlock()
}
