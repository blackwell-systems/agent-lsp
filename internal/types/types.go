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
