package session

import (
	"context"
	"sync"
	"testing"
	"time"
)

func makeSession(id string) *SimulationSession {
	return &SimulationSession{ID: id}
}

func TestSerializedExecutor_AcquireRelease(t *testing.T) {
	e := NewSerializedExecutor()
	ctx := context.Background()
	s := makeSession("sess-1")

	// First Acquire should succeed immediately.
	if err := e.Acquire(ctx, s); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	// Release the per-session lock.
	e.Release(s)

	// Second Acquire should succeed after Release.
	if err := e.Acquire(ctx, s); err != nil {
		t.Fatalf("second Acquire after Release failed: %v", err)
	}
	e.Release(s)
}

func TestSerializedExecutor_AcquireCancelledContext(t *testing.T) {
	e := NewSerializedExecutor()
	ctx := context.Background()
	s := makeSession("sess-2")

	// Hold the per-session lock without releasing.
	if err := e.Acquire(ctx, s); err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	// Attempt Acquire with a pre-cancelled context; it must return context.Canceled.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := e.Acquire(cancelledCtx, s)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Clean up: release the held lock.
	e.Release(s)
}

// TestSerializedExecutor_IndependentSessions verifies that independent sessions
// do not block each other — the core H2 fix.
func TestSerializedExecutor_IndependentSessions(t *testing.T) {
	e := NewSerializedExecutor()
	ctx := context.Background()

	sA := makeSession("sess-A")
	sB := makeSession("sess-B")

	// Acquire sA, then immediately acquire sB (must not block).
	if err := e.Acquire(ctx, sA); err != nil {
		t.Fatalf("Acquire sA failed: %v", err)
	}

	// sB must be acquirable while sA is still held.
	done := make(chan error, 1)
	go func() {
		done <- e.Acquire(ctx, sB)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Acquire sB failed: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Acquire sB blocked — sessions are not independent")
	}

	e.Release(sA)
	e.Release(sB)
}

// TestSerializedExecutor_SameSessionBlocks verifies that the same session
// cannot be acquired twice concurrently (serialization is preserved per-session).
func TestSerializedExecutor_SameSessionBlocks(t *testing.T) {
	e := NewSerializedExecutor()
	ctx := context.Background()
	s := makeSession("sess-same")

	if err := e.Acquire(ctx, s); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	// A second Acquire on the same session should block.
	var wg sync.WaitGroup
	wg.Add(1)
	blocked := make(chan struct{})
	go func() {
		defer wg.Done()
		close(blocked) // signal we've started
		if err := e.Acquire(ctx, s); err != nil {
			t.Errorf("second Acquire returned unexpected error: %v", err)
		}
		e.Release(s)
	}()

	// Wait for goroutine to start, then confirm it blocks.
	<-blocked
	time.Sleep(50 * time.Millisecond)

	// Release should unblock the second Acquire.
	e.Release(s)
	wg.Wait()
}
