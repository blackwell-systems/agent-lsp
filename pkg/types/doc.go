// Package types provides the public type definitions for agent-lsp's LSP client
// and MCP tool layer. It re-exports all types from the internal implementation
// package as type aliases, so values are interchangeable with internal usage.
//
// Importers of agent-lsp as a library should use this package rather than
// the internal/types path, which is not covered by the Go module's public API
// stability guarantees.
//
// Key types:
//
//   - Position, Range, Location: LSP coordinate types (0-based, LSP convention)
//   - LSPDiagnostic: diagnostic returned by the language server
//   - DocumentSymbol, SymbolInformation: symbol shapes from documentSymbol/workspaceSymbol
//   - CallHierarchyItem, TypeHierarchyItem: call/type hierarchy nodes
//   - CompletionItem, CompletionList: completion response types
//   - CodeAction, TextEdit: edit and action types
//   - InlayHint, DocumentHighlight, SemanticToken: display annotation types
//   - ToolResult, ContentItem: MCP tool response envelope
//   - Extension: interface for per-language extension packages
package types

import internaltypes "github.com/blackwell-systems/agent-lsp/internal/types"

type Position = internaltypes.Position
type Range = internaltypes.Range
type Location = internaltypes.Location
type FormattedLocation = internaltypes.FormattedLocation
type LSPDiagnostic = internaltypes.LSPDiagnostic
type DiagnosticUpdateCallback = internaltypes.DiagnosticUpdateCallback
type FileChangeEvent = internaltypes.FileChangeEvent
type ToolResult = internaltypes.ToolResult
type ContentItem = internaltypes.ContentItem
type SymbolKind = internaltypes.SymbolKind
type SymbolTag = internaltypes.SymbolTag
type CallHierarchyItem = internaltypes.CallHierarchyItem
type CallHierarchyIncomingCall = internaltypes.CallHierarchyIncomingCall
type CallHierarchyOutgoingCall = internaltypes.CallHierarchyOutgoingCall
type TypeHierarchyItem = internaltypes.TypeHierarchyItem
type SemanticToken = internaltypes.SemanticToken
type TextEdit = internaltypes.TextEdit
type SymbolInformation = internaltypes.SymbolInformation
type DocumentSymbol = internaltypes.DocumentSymbol
type Command = internaltypes.Command
type CompletionItem = internaltypes.CompletionItem
type CompletionList = internaltypes.CompletionList
type CodeAction = internaltypes.CodeAction
type ToolHandler = internaltypes.ToolHandler
type ResourceHandler = internaltypes.ResourceHandler
type Extension = internaltypes.Extension
type InlayHintKind = internaltypes.InlayHintKind
type InlayHintLabelPart = internaltypes.InlayHintLabelPart
type InlayHint = internaltypes.InlayHint
type DocumentHighlightKind = internaltypes.DocumentHighlightKind
type DocumentHighlight = internaltypes.DocumentHighlight

const (
	InlayHintKindType      = internaltypes.InlayHintKindType
	InlayHintKindParameter = internaltypes.InlayHintKindParameter
	DocumentHighlightText  = internaltypes.DocumentHighlightText
	DocumentHighlightRead  = internaltypes.DocumentHighlightRead
	DocumentHighlightWrite = internaltypes.DocumentHighlightWrite
)

var (
	TextResult  = internaltypes.TextResult
	ErrorResult = internaltypes.ErrorResult
)
