// Package notify provides proactive notification dispatch for the agent-lsp
// MCP server. It defines the NotificationSender interface and the Hub type
// that coordinates all notification channels (diagnostics, progress, resource
// updates) without depending on the MCP SDK directly.
//
// The Hub is created once during server startup and passed to subsystems that
// need to emit notifications. It is safe for concurrent use.
package notify
