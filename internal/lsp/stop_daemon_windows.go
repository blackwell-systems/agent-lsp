//go:build windows

package lsp

import "syscall"

// PROCESS_TERMINATE is the Windows access right required to call
// TerminateProcess on a process handle.
const processTerminate = 0x0001

// terminateProcess asks Windows to terminate the given PID.
//
// Replaces the POSIX `proc.Signal(syscall.SIGTERM)` idiom — Go's
// Windows runtime rejects any non-Kill signal with "not supported by
// windows", so the original StopDaemon implementation was a no-op
// here, leaving the `agent-lsp daemon-stop` CLI unable to actually
// stop a daemon on Windows.
//
// We pass exit code 0x1 (matching the conventional "killed by signal"
// status; the daemon-broker's signal handler isn't reachable from
// Windows anyway, so this is a hard terminate).
func terminateProcess(pid int) error {
	h, err := syscall.OpenProcess(processTerminate, false, uint32(pid))
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	return syscall.TerminateProcess(h, 1)
}
