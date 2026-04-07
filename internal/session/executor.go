package session

import (
	"context"
)

// SerializedExecutor is the V1 implementation of SessionExecutor.
// It serializes all session operations through a single buffered-channel
// semaphore, ensuring only one session mutates LSP state at a time.
// Unlike a bare sync.Mutex, this implementation respects context cancellation:
// Acquire returns ctx.Err() if the context is cancelled while waiting.
type SerializedExecutor struct {
	sem chan struct{}
}

// NewSerializedExecutor creates a new SerializedExecutor.
func NewSerializedExecutor() *SerializedExecutor {
	return &SerializedExecutor{sem: make(chan struct{}, 1)}
}

// Acquire locks the executor for exclusive session access.
// Returns ctx.Err() if the context is cancelled before the lock is acquired.
func (e *SerializedExecutor) Acquire(ctx context.Context, s *SimulationSession) error {
	select {
	case e.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release unlocks the executor after session operation completes.
func (e *SerializedExecutor) Release(s *SimulationSession) {
	<-e.sem
}
