// Package lsp implements a JSON-RPC 2.0 client for Language Server Protocol
// (LSP) subprocesses. LSPClient manages the full lifecycle of a single LSP
// server: spawning, Content-Length framing, request/response correlation,
// server-initiated request handling (workspace/applyEdit,
// workspace/configuration, client/registerCapability), diagnostic
// subscriptions, workspace progress tracking, and optional file watching.
//
// ServerManager handles multi-server configurations, routing file operations
// to the correct language server by file extension. ClientResolver is the
// interface that ServerManager implements.
//
// This package is the internal implementation. External callers should use
// github.com/blackwell-systems/agent-lsp/pkg/lsp for a stable public API.
package lsp
