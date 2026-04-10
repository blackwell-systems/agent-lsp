// Package lsp provides a public API for spawning and communicating with
// Language Server Protocol (LSP) subprocesses from Go programs.
//
// The primary type is [LSPClient], which manages the lifecycle of a single
// LSP server process: spawning, Content-Length framing, request/response
// correlation, server-initiated request handling, diagnostic subscriptions,
// and workspace file watching.
//
// [ServerManager] routes file-specific operations to the correct language
// server in multi-server configurations. [ClientResolver] is the interface
// that ServerManager implements; it allows callers to inject alternative
// routing implementations.
//
// Typical single-server usage:
//
//	client := lsp.NewLSPClient("gopls", []string{})
//	if err := client.Initialize(ctx, "/path/to/workspace"); err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Shutdown(ctx)
//	locs, err := client.GetDefinition(ctx, fileURI, pos)
//
// All types in this package are type aliases of the internal implementation;
// values are interchangeable with internal/lsp without conversion.
package lsp
