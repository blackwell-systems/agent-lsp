// Package types defines the shared data types used throughout agent-lsp:
// LSP wire types (Position, Range, Location, LSPDiagnostic, TextEdit, etc.),
// symbol types (DocumentSymbol, SymbolInformation, CallHierarchyItem,
// TypeHierarchyItem, InlayHint, DocumentHighlight, SemanticToken),
// completion and code-action types (CompletionList, CodeAction), and
// the ToolResult/ContentItem envelope used by MCP tool handlers.
//
// Extension and ToolHandler/ResourceHandler are also defined here to allow
// per-language extension packages to register with the MCP server without
// importing the full cmd/ layer.
//
// This package is the internal implementation. External callers should use
// github.com/blackwell-systems/agent-lsp/pkg/types for a stable public API.
package types
