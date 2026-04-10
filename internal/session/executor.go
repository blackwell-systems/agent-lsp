package session

import (
	"context"
	"sync"
)

// SerializedExecutor serializes operations per-session using a per-session
// channel semaphore map. Independent sessions do not block each other, unlike
// the previous global semaphore approach.
type SerializedExecutor struct {
	mu           sync.Mutex
	sessionLocks map[string]chan struct{}
}

// NewSerializedExecutor creates a new SerializedExecutor.
func NewSerializedExecutor() *SerializedExecutor {
	return &SerializedExecutor{
		sessionLocks: make(map[string]chan struct{}),
	}
}

// lockFor returns the per-session semaphore channel for s, creating it if needed.
func (e *SerializedExecutor) lockFor(s *SimulationSession) chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ch, ok := e.sessionLocks[s.ID]; ok {
		return ch
	}
	ch := make(chan struct{}, 1)
	e.sessionLocks[s.ID] = ch
	return ch
}

// Acquire locks the per-session semaphore for exclusive session access.
// Returns ctx.Err() if the context is cancelled before the lock is acquired.
func (e *SerializedExecutor) Acquire(ctx context.Context, s *SimulationSession) error {
	ch := e.lockFor(s)
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release unlocks the per-session semaphore after the operation completes.
func (e *SerializedExecutor) Release(s *SimulationSession) {
	e.mu.Lock()
	ch := e.sessionLocks[s.ID]
	e.mu.Unlock()
	if ch != nil {
		<-ch
	}
}
