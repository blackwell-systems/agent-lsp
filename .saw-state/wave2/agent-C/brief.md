---
saw_name: '[SAW:wave2:agent-C] ## Role'
---

# Agent C Brief - Wave 2

**IMPL Doc:** docs/IMPL/IMPL-semantic-tokens.yaml

## Files Owned

- `cmd/lsp-mcp-go/server.go`


## Task

## Role
You are wiring the new get_semantic_tokens tool into the MCP server
registration in cmd/lsp-mcp-go/server.go.

## What to implement

### 1. Add args struct

In `cmd/lsp-mcp-go/server.go`, find the block of Args struct definitions
inside the `Run` function. The last struct is:

```go
	type CallHierarchyArgs struct {
		FilePath   string `json:"file_path"`
		LanguageID string `json:"language_id,omitempty"`
		Line       int    `json:"line"`
		Column     int    `json:"column"`
		Direction  string `json:"direction,omitempty"`
	}
```

Add the new args struct immediately after CallHierarchyArgs:

```go
	type GetSemanticTokensArgs struct {
		FilePath    string `json:"file_path"`
		LanguageID  string `json:"language_id,omitempty"`
		StartLine   int    `json:"start_line"`
		StartColumn int    `json:"start_column"`
		EndLine     int    `json:"end_line"`
		EndColumn   int    `json:"end_column"`
	}
```

### 2. Register the tool

Find the comment `// ------- Register all 25 tools -------` and update it to:
`// ------- Register all 26 tools -------`

Then find the last `mcp.AddTool` call in the tool registration block.
It is the call_hierarchy registration:

```go
	mcp.AddTool(server, &mcp.Tool{
		Name:        "call_hierarchy",
		Description: "Show call hierarchy for a symbol at a position. Returns callers (incoming), callees (outgoing), or both depending on the direction parameter. Direction defaults to \"both\". Use this to understand code flow -- which functions call this function and which functions it calls.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CallHierarchyArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleCallHierarchy(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
```

Add the new tool AFTER that closing `})`:

```go
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_semantic_tokens",
		Description: "Get semantic tokens for a range in a file. Returns each token's type (function, variable, keyword, parameter, type, etc.) and modifiers (readonly, static, deprecated, etc.) with 1-based line/character positions. Use this to understand the syntactic role of code elements — distinct from hover which gives documentation. Only available when the language server supports textDocument/semanticTokens.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetSemanticTokensArgs) (*mcp.CallToolResult, any, error) {
		r, err := tools.HandleGetSemanticTokens(ctx, cs.get(), toolArgsToMap(args))
		return makeCallToolResult(r), nil, err
	})
```

## Constraints
- Do NOT modify any other part of server.go.
- Do NOT modify files outside your ownership list (cmd/lsp-mcp-go/server.go).
- The tool name must be exactly `get_semantic_tokens` (matches the feature spec).

## Verification gate
```bash
cd /Users/dayna.blackwell/code/LSP-MCP-GO
go build ./cmd/lsp-mcp-go/...
go vet ./cmd/lsp-mcp-go/...
go test ./...
# Postcondition checks:
grep -c "get_semantic_tokens" cmd/lsp-mcp-go/server.go
# Expected: >= 1
grep -c "Register all 26 tools" cmd/lsp-mcp-go/server.go
# Expected: 1
grep -c "GetSemanticTokensArgs" cmd/lsp-mcp-go/server.go
# Expected: >= 2 (struct declaration + usage)
```



## Interface Contracts

### GetSemanticTokens

New method on LSPClient. Sends textDocument/semanticTokens/range (with
fallback to textDocument/semanticTokens/full) and returns decoded tokens
with absolute 1-based positions and human-readable type/modifier names.

```
func (c *LSPClient) GetSemanticTokens(
    ctx context.Context,
    uri string,
    rng types.Range,
) ([]types.SemanticToken, error)

```

### GetSemanticTokenLegend

Accessor to retrieve the token legend captured during Initialize.
Returns nil slices if the server did not advertise semanticTokensProvider.

```
func (c *LSPClient) GetSemanticTokenLegend() (tokenTypes []string, tokenModifiers []string)

```

### SemanticToken

Decoded token struct stored in internal/types/types.go. 1-based line and
character positions matching this project's output convention.

```
type SemanticToken struct {
    Line      int      `json:"line"`
    Character int      `json:"character"`
    Length    int      `json:"length"`
    TokenType string   `json:"tokenType"`
    Modifiers []string `json:"modifiers"`
}

```

### HandleGetSemanticTokens

Tool handler in internal/tools/semantic_tokens.go following the same
function signature as all other handlers in this package.

```
func HandleGetSemanticTokens(
    ctx context.Context,
    client *lsp.LSPClient,
    args map[string]interface{},
) (types.ToolResult, error)

```



## Quality Gates

Level: standard

- **build**: `cd /Users/dayna.blackwell/code/LSP-MCP-GO && GOWORK=off go build ./...` (required: true)
- **test**: `cd /Users/dayna.blackwell/code/LSP-MCP-GO && GOWORK=off go test $(go list ./... | grep -v '/test$')` (required: true)
- **lint**: `cd /Users/dayna.blackwell/code/LSP-MCP-GO && GOWORK=off go vet ./...` (required: true)

