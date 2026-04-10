// Package resources implements MCP resource handlers for agent-lsp.
// Resources expose LSP data (diagnostics, workspace symbols, document
// content) via the MCP resource protocol, enabling clients to subscribe
// to live updates. Subscriptions are managed alongside the one-shot read
// handlers registered during server initialization.
package resources
