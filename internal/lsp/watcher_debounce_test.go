package lsp

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// The watcher batches file events and delivers them after a debounce. The
// flush used to run on the debounce timer's goroutine while the event loop
// kept writing the pending map, a data race under -race; it now runs on the
// loop. Events written in a burst must still arrive.
func TestWatcher_DebouncedEventsDelivered(t *testing.T) {
	dir := t.TempDir()
	c := NewLSPClient("fake", nil)
	c.DiscardOutputForTest()

	var mu sync.Mutex
	seen := map[string]bool{}
	c.SubscribeToFileChanges(func(changes []types.FileChangeEvent) {
		mu.Lock()
		defer mu.Unlock()
		for _, ch := range changes {
			seen[filepath.Base(ch.URI)] = true
		}
	})
	c.startWatcher(dir)
	defer c.stopWatcher()

	deadline := time.Now().Add(2 * time.Second)
	for {
		c.watcherMu.Lock()
		ready := c.watcher != nil
		c.watcherMu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watcher did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // let the walk register the root

	// Two bursts with a pause longer than the debounce between them, so the
	// first batch is flushed and the loop then keeps recording events: the
	// pattern that raced when flush ran on the timer's goroutine.
	names := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	for burst := 0; burst < 2; burst++ {
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), []byte{byte(burst)}, 0644); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(300 * time.Millisecond)
	}

	deadline = time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		got := len(seen)
		mu.Unlock()
		if got == len(names) {
			return
		}
		if time.Now().After(deadline) {
			mu.Lock()
			defer mu.Unlock()
			t.Fatalf("delivered %v, want all of %v", seen, names)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
