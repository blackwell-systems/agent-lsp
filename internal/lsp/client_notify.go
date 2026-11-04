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

// IsWorkspaceLoaded reports whether the language server has finished
// indexing the workspace. It is what tools report as "indexed".
//
// A readiness wait that ran to its cap marks the workspace loaded so later
// queries do not wait again, but the server is still working: while any
// progress token that outlasted a wait (a stale token) is active, this
// returns false, so the agent is told its results may be incomplete instead
// of the timeout being silently swallowed. (issue #77)
func (c *LSPClient) IsWorkspaceLoaded() bool {
	if !c.workspaceLoaded.Load() {
		return false
	}
	c.progressMu.Lock()
	defer c.progressMu.Unlock()
	for tok := range c.staleProgress {
		if _, active := c.progressTokens[tok]; active {
			return false
		}
	}
	return true
}
