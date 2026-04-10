// Package session implements speculative code editing: applying LSP-validated
// edits in memory, diffing resulting diagnostics against a pre-edit baseline,
// and committing or discarding the result. The design isolates speculative
// changes from the real filesystem until the caller decides to commit.
//
// SessionManager is the entry point. Each simulation session tracks its own
// set of in-memory file contents, diagnostic baselines, and edit history.
// Sessions serialize concurrent operations per-session via SerializedExecutor.
//
// This package is the internal implementation. External callers should use
// github.com/blackwell-systems/agent-lsp/pkg/session for a stable public API.
package session
