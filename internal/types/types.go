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
	Range    Range       `json:"range"`
	Severity int         `json:"severity"` // 1=error 2=warning 3=info 4=hint
	Code     interface{} `json:"code,omitempty"`
	Source   string      `json:"source,omitempty"`
	Message  string      `json:"message"`
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
	Data           interface{} `json:"data,omitempty"`
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

// ToolHandler is the function signature for tool handler callbacks registered
// by extensions. The ctx, client, and args mirror the standard tool handler args.
type ToolHandler func(ctx interface{}, args map[string]interface{}) (ToolResult, error)

// ResourceHandler is the function signature for resource read callbacks registered
// by extensions.
type ResourceHandler func(ctx interface{}, uri string) (interface{}, error)

// Extension is implemented by per-language extension packages.
type Extension interface {
	ToolHandlers() map[string]ToolHandler
	ResourceHandlers() map[string]ResourceHandler
	SubscriptionHandlers() map[string]ResourceHandler
	PromptHandlers() map[string]interface{}
}
