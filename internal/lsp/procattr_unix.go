//go:build !windows

package lsp

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr detaches the subprocess so it survives the parent's exit.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
