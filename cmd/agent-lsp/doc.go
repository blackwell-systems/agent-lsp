// Command agent-lsp is a Model Context Protocol (MCP) server that wraps one
// or more Language Server Protocol (LSP) subprocesses, exposing LSP
// capabilities as MCP tools callable by AI agents and language-aware editors.
//
// Usage modes:
//
//	agent-lsp <language-id> <lsp-binary> [args...]   # single-server
//	agent-lsp go:gopls typescript:tsserver,--stdio   # multi-server
//	agent-lsp --config /path/to/lsp-mcp.json         # config file
//	agent-lsp                                          # auto-detect installed servers
//
// See README.md for full documentation and the tools reference.
package main
