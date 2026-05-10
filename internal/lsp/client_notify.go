package lsp

import "github.com/blackwell-systems/agent-lsp/pkg/types"

// SubscribeToFileChanges registers a callback that is invoked whenever the
// auto-watcher detects file changes in the workspace. The callback receives
// the same batch of FileChangeEvent values sent to the language server via
// didChangeWatchedFiles.
func (c *LSPClient) SubscribeToFileChanges(cb func([]types.FileChangeEvent)) {
	c.watcherMu.Lock()
	c.fileChangeCbs = append(c.fileChangeCbs, cb)
	c.watcherMu.Unlock()
}

// IsAlive reports whether the language server process is running. For daemon
// and passive mode clients the server is managed externally and always
// considered alive.
func (c *LSPClient) IsAlive() bool {
	if c.isDaemon || c.isPassive {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil {
		return false
	}
	// ProcessState is non-nil only after the process has exited.
	return c.cmd.ProcessState == nil
}

// IsWorkspaceLoaded returns true once the language server has finished
// indexing the workspace (all $/progress tokens resolved).
func (c *LSPClient) IsWorkspaceLoaded() bool {
	return c.workspaceLoaded.Load()
}
