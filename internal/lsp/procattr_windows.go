//go:build windows

package lsp

import (
	"os/exec"
	"syscall"
)

// Windows process-creation flags (golang.org/x/sys/windows mirrors these,
// but we keep the constants local to avoid pulling the dependency for a
// two-flag use case).
const (
	// CREATE_NEW_PROCESS_GROUP makes the child a new process-group leader,
	// so Ctrl+C / Ctrl+Break sent to the parent's group doesn't propagate
	// to the child. This is what lets the broker survive parent death.
	createNewProcessGroup = 0x00000200
	// CREATE_NO_WINDOW gives the child a console but no visible window.
	// We deliberately do NOT use DETACHED_PROCESS here even though it
	// also "hides" the child: DETACHED_PROCESS removes the console
	// entirely, which breaks any descendant that needs to invoke a
	// Windows .cmd / .bat shim (cmd.exe interpreter requires a console).
	// On a typical agent-lsp install the language-server binaries are
	// npm-shipped `.cmd` shims (`pyright-langserver.cmd`,
	// `typescript-language-server.cmd`, etc.), so the broker MUST be
	// able to spawn them. CREATE_NO_WINDOW preserves that capability
	// while still keeping the daemon invisible to the user.
	createNoWindow = 0x08000000
)

// setSysProcAttr arranges for the spawned broker subprocess to survive
// the death of its parent (typically Claude Code's MCP stdio transport,
// which tears down at session end). Previously this was a no-op with the
// comment "Daemon mode is not currently supported on Windows" — every
// spawn left a stale `agent-lsp.exe` daemon that the next call couldn't
// reach via the daemon registry. The CHANGELOG claimed this had been
// fixed; the file as shipped contradicted that. Restore the intended
// behaviour.
func setSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewProcessGroup | createNoWindow
}
