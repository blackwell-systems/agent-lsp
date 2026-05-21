//go:build !windows

package lsp

import (
	"os"
	"syscall"
)

// terminateProcess sends SIGTERM to the given PID on POSIX.
func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
