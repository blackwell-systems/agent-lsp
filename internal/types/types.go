package types

// Position is a 0-based line/character position (LSP convention).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a 0-based start/end position pair (LSP convention).
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a URI + range pair returned by definition/reference queries.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// FormattedLocation is the 1-indexed, file-path-based form used in tool responses.
type FormattedLocation struct {
	FilePath  string `json:"file"`
	StartLine int    `json:"line"`
	StartCol  int    `json:"column"`
	EndLine   int    `json:"end_line"`
	EndCol    int    `json:"end_column"`
}

// LSPDiagnostic mirrors the LSP publishDiagnostics Diagnostic object.
type LSPDiagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1=error 2=warning 3=info 4=hint
	Code     any    `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// DiagnosticUpdateCallback is called whenever the LSP server publishes diagnostics.
type DiagnosticUpdateCallback func(uri string, diagnostics []LSPDiagnostic)

// FileChangeEvent is a single entry for workspace/didChangeWatchedFiles.
type FileChangeEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"` // 1=created 2=changed 3=deleted
}

// ToolResult is returned by every MCP tool handler.
type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem is a single content block inside a ToolResult.
type ContentItem struct {
	Type string `json:"type"` // always "text" for this server
	Text string `json:"text"`
}

// TextResult constructs a successful ToolResult with a single text item.
func TextResult(text string) ToolResult {
	return ToolResult{Content: []ContentItem{{Type: "text", Text: text}}}
}

// ErrorResult constructs an error ToolResult (isError=true).
func ErrorResult(msg string) ToolResult {
	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: msg}},
		IsError: true,
	}
}

// SymbolKind is the LSP SymbolKind enumeration (integer codes 1–26).
type SymbolKind int

// SymbolTag is an optional modifier tag on a symbol or hierarchy item.
type SymbolTag int

// CallHierarchyItem represents a single node in a call hierarchy graph.
// See LSP 3.16 § CallHierarchyItem.
type CallHierarchyItem struct {
	Name           string      `json:"name"`
	Kind           SymbolKind  `json:"kind"`
	Tags           []SymbolTag `json:"tags,omitempty"`
	Detail         *string     `json:"detail,omitempty"`
	URI            string      `json:"uri"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Data           any         `json:"data,omitempty"`
}

// CallHierarchyIncomingCall represents a caller of a function in the call hierarchy.
type CallHierarchyIncomingCall struct {
	// From is the item that makes the call.
	From CallHierarchyItem `json:"from"`
	// FromRanges are the ranges within From at which the call appears.
	FromRanges []Range `json:"fromRanges"`
}

// CallHierarchyOutgoingCall represents a callee of a function in the call hierarchy.
type CallHierarchyOutgoingCall struct {
	// To is the item that is called.
	To CallHierarchyItem `json:"to"`
	// FromRanges are the ranges within the caller at which the call appears.
	FromRanges []Range `json:"fromRanges"`
}

// TypeHierarchyItem represents a single node in a type hierarchy graph.
// See LSP 3.17 § TypeHierarchyItem.
type TypeHierarchyItem struct {
	Name           string      `json:"name"`
	Kind           SymbolKind  `json:"kind"`
	Tags           []SymbolTag `json:"tags,omitempty"`
	Detail         *string     `json:"detail,omitempty"`
	URI            string      `json:"uri"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Data           any         `json:"data,omitempty"`
}

// SemanticToken is a single decoded semantic token with absolute 1-based position
// and resolved type/modifier names from the server's legend.
type SemanticToken struct {
	Line      int      `json:"line"`
	Character int      `json:"character"`
	Length    int      `json:"length"`
	TokenType string   `json:"tokenType"`
	Modifiers []string `json:"modifiers"`
}

// TextEdit represents a textual edit on a document (insert, replace, or delete).
// See LSP 3.16 § TextEdit.
type TextEdit struct {
	// Range of text to replace. For insertions, Start == End.
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// SymbolInformation identifies a symbol in a workspace or document.
// See LSP 3.16 § SymbolInformation.
type SymbolInformation struct {
	Name          string      `json:"name"`
	Kind          SymbolKind  `json:"kind"`
	Tags          []SymbolTag `json:"tags,omitempty"`
	Deprecated    *bool       `json:"deprecated,omitempty"`
	Location      Location    `json:"location"`
	ContainerName *string     `json:"containerName,omitempty"`
}

// DocumentSymbol is the hierarchical variant of a document symbol.
// See LSP 3.17 § DocumentSymbol.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Tags           []SymbolTag      `json:"tags,omitempty"`
	Deprecated     bool             `json:"deprecated,omitempty"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// Command is an LSP workspace command (used both standalone and embedded).
// See LSP 3.17 § Command.
type Command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

// CompletionItem represents a single completion suggestion.
// See LSP 3.17 § CompletionItem.
type CompletionItem struct {
	Label               string      `json:"label"`
	Kind                *int        `json:"kind,omitempty"`
	Tags                []SymbolTag `json:"tags,omitempty"`
	Detail              *string     `json:"detail,omitempty"`
	Documentation       any         `json:"documentation,omitempty"`
	Deprecated          bool        `json:"deprecated,omitempty"`
	Preselect           bool        `json:"preselect,omitempty"`
	SortText            *string     `json:"sortText,omitempty"`
	FilterText          *string     `json:"filterText,omitempty"`
	InsertText          *string     `json:"insertText,omitempty"`
	InsertTextFormat    *int        `json:"insertTextFormat,omitempty"`
	TextEdit            any         `json:"textEdit,omitempty"`
	AdditionalTextEdits []TextEdit  `json:"additionalTextEdits,omitempty"`
	CommitCharacters    []string    `json:"commitCharacters,omitempty"`
	Command             *Command    `json:"command,omitempty"`
	Data                any         `json:"data,omitempty"`
}

// CompletionList is the canonical completion response wrapper.
// See LSP 3.17 § CompletionList.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// CodeAction is the canonical code action shape.
// See LSP 3.17 § CodeAction.
type CodeAction struct {
	Title       string          `json:"title"`
	Kind        *string         `json:"kind,omitempty"`
	Diagnostics []LSPDiagnostic `json:"diagnostics,omitempty"`
	IsPreferred *bool           `json:"isPreferred,omitempty"`
	Disabled    *struct {
		Reason string `json:"reason"`
	} `json:"disabled,omitempty"`
	Edit    any      `json:"edit,omitempty"`
	Command *Command `json:"command,omitempty"`
	Data    any      `json:"data,omitempty"`
}

// ToolHandler is the function signature for tool handler callbacks registered
// by extensions. The ctx, client, and args mirror the standard tool handler args.
type ToolHandler func(ctx any, args map[string]any) (ToolResult, error)

// ResourceHandler is the function signature for resource read callbacks registered
// by extensions.
type ResourceHandler func(ctx any, uri string) (any, error)

// Extension is implemented by per-language extension packages.
// The current interface covers the four methods used by the extension
// registry. ToolDefinitions, UnsubscriptionHandlers, and PromptDefinitions
// (documented in docs/architecture.md) are deferred pending MCP SDK
// maturation and are not included here.
type Extension interface {
	ToolHandlers() map[string]ToolHandler
	ResourceHandlers() map[string]ResourceHandler
	SubscriptionHandlers() map[string]ResourceHandler
	PromptHandlers() map[string]any
}

// InlayHintKind indicates whether an inlay hint is for a Type annotation or
// a Parameter name. See LSP 3.17 § InlayHintKind.
type InlayHintKind int

const (
	InlayHintKindType      InlayHintKind = 1
	InlayHintKindParameter InlayHintKind = 2
)

// InlayHintLabelPart is a single part of a composite inlay hint label.
// See LSP 3.17 § InlayHintLabelPart.
type InlayHintLabelPart struct {
	Value    string    `json:"value"`
	Tooltip  string    `json:"tooltip,omitempty"`
	Location *Location `json:"location,omitempty"`
}

// InlayHint is an annotation displayed inline with source code, typically
// showing inferred types or parameter names. See LSP 3.17 § InlayHint.
//
// Label is either a plain string or a JSON array of InlayHintLabelPart.
// Use InlayHint.LabelString() for the display string in either case.
type InlayHint struct {
	Position     Position      `json:"position"`
	Label        any           `json:"label"` // string | []InlayHintLabelPart
	Kind         InlayHintKind `json:"kind,omitempty"`
	Tooltip      string        `json:"tooltip,omitempty"`
	PaddingLeft  bool          `json:"paddingLeft,omitempty"`
	PaddingRight bool          `json:"paddingRight,omitempty"`
}

// DocumentHighlightKind indicates the role of a highlighted symbol occurrence.
// See LSP 3.17 § DocumentHighlightKind.
type DocumentHighlightKind int

const (
	// DocumentHighlightText is a textual occurrence (not read or write).
	DocumentHighlightText DocumentHighlightKind = 1
	// DocumentHighlightRead is a read access of the symbol.
	DocumentHighlightRead DocumentHighlightKind = 2
	// DocumentHighlightWrite is a write access of the symbol.
	DocumentHighlightWrite DocumentHighlightKind = 3
)

// DocumentHighlight is a single highlighted occurrence of a symbol within a file.
// See LSP 3.17 § DocumentHighlight.
type DocumentHighlight struct {
	Range Range                 `json:"range"`
	Kind  DocumentHighlightKind `json:"kind,omitempty"`
}
