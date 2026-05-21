//go:build !windows

package lsp

import (
	"os"
	"syscall"
)

// processAlive reports whether the given PID belongs to a live process.
// On POSIX, sending signal 0 returns nil iff the process exists and we
// have permission to signal it; both ESRCH (gone) and EPERM (exists
// but not ours) → false here, which is fine for the daemon-registry
// liveness use case (we only ever check PIDs we spawned ourselves).
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
