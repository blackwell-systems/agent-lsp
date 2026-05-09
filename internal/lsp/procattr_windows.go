//go:build windows

package lsp

import "os/exec"

// setSysProcAttr is a no-op on Windows. Setsid is not available.
// Daemon mode is not currently supported on Windows.
func setSysProcAttr(cmd *exec.Cmd) {
	// No detach on Windows; the subprocess will die with the parent.
}
