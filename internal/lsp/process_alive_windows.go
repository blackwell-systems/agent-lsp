//go:build windows

package lsp

import "syscall"

// Windows constants — local copies to avoid pulling
// golang.org/x/sys/windows for two values.
const (
	// PROCESS_QUERY_LIMITED_INFORMATION is the minimal-rights flag for
	// querying a process's exit code; works for processes we don't
	// fully own (e.g. detached daemons in a separate process group).
	processQueryLimitedInformation = 0x1000
	// STILL_ACTIVE is the magic exit-code value Windows returns from
	// GetExitCodeProcess for a process that has not yet exited.
	stillActive = 259
)

// processAlive reports whether the given PID belongs to a live process
// on Windows.
//
// The original implementation used `proc.Signal(syscall.Signal(0)) == nil`
// — a POSIX idiom that Go's Windows runtime explicitly rejects
// ("not supported by windows"), so it returned false for every PID,
// including processes that were obviously running. This single bug was
// the root cause of multiple downstream Windows-only failures:
//
//   - `CleanupStaleDaemons` deleted EVERY daemon registry on EVERY call.
//   - `FindRunningDaemon` deleted the registry of any daemon it queried,
//     immediately after the daemon wrote it.
//
// Use OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION) + GetExitCodeProcess
// and treat STILL_ACTIVE (259) as alive.
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == stillActive
}
