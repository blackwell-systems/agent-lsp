// Package extensions provides a thread-safe registry for per-language
// extension packages. Extensions register ToolHandlers, ResourceHandlers,
// SubscriptionHandlers, and PromptHandlers that are merged into the MCP
// server's handler tables at startup. Register adds an extension; the
// server calls the accessor methods to enumerate handlers at build time.
package extensions
