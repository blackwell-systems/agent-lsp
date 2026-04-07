package session

import (
	"context"
	"testing"
)

func TestSerializedExecutor_AcquireRelease(t *testing.T) {
	e := NewSerializedExecutor()
	ctx := context.Background()

	// First Acquire should succeed immediately.
	if err := e.Acquire(ctx, nil); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	// Release the semaphore.
	e.Release(nil)

	// Second Acquire should succeed after Release.
	if err := e.Acquire(ctx, nil); err != nil {
		t.Fatalf("second Acquire after Release failed: %v", err)
	}
	e.Release(nil)
}

func TestSerializedExecutor_AcquireCancelledContext(t *testing.T) {
	e := NewSerializedExecutor()
	ctx := context.Background()

	// Hold the semaphore without releasing.
	if err := e.Acquire(ctx, nil); err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	// Attempt Acquire with a pre-cancelled context; it must return context.Canceled.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := e.Acquire(cancelledCtx, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Clean up: release the held semaphore.
	e.Release(nil)
}
