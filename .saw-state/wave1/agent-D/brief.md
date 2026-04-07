---
saw_name: '[SAW:wave1:agent-D] ## Agent D — LSP Client: applyEdit Response, rootURI Encoding, ApplyWorkspa...'
---

# Agent D Brief - Wave 1

**IMPL Doc:** /Users/dayna.blackwell/code/scout-and-wave/docs/IMPL/IMPL-lsp-mcp-go-audit-fixes.yaml

## Files Owned

- `internal/lsp/client.go`
- `internal/resources/resources.go`
- `internal/lsp/manager.go`


## Task

## Agent D — LSP Client: applyEdit Response, rootURI Encoding, ApplyWorkspaceEdit Extract; Resources URI Encoding; Manager Multi-Error Shutdown

### Context
Four bugs span `internal/lsp/client.go`, `internal/resources/resources.go`,
and `internal/lsp/manager.go`:
1. `workspace/applyEdit` dispatch always responds `applied:true` even on failure
2. `Initialize` uses bare `"file://" + rootDir` instead of url.URL
3. `HandleHoverResource` and `HandleCompletionsResource` use bare `"file://" + filePath`
4. `Shutdown` discards all but the last error from multi-server shutdown

Additionally two refactors (warning severity):
5. Extract `applyDocumentChanges` helper from `ApplyWorkspaceEdit`
6. Extract `parseResourceQueryParams` helper shared by Hover and Completions resource handlers

**Files to modify:**
- `internal/lsp/client.go`
- `internal/resources/resources.go`
- `internal/lsp/manager.go`

### Task 1 — Fix workspace/applyEdit error propagation (client.go)

In the dispatch function (around line 279), find this exact block:
```go
	case "workspace/applyEdit":
		// Apply the workspace edit and respond with ApplyWorkspaceEditResult.
		if msg.ID != nil {
			var p struct {
				Edit interface{} `json:"edit"`
			}
			if err := json.Unmarshal(msg.Params, &p); err == nil && p.Edit != nil {
				_ = c.ApplyWorkspaceEdit(context.Background(), p.Edit)
			}
			c.sendResponse(*msg.ID, map[string]interface{}{"applied": true})
		}
```

Replace with:
```go
	case "workspace/applyEdit":
		// Apply the workspace edit and respond with ApplyWorkspaceEditResult.
		// Per LSP spec: respond applied=false with failureReason on error.
		if msg.ID != nil {
			var p struct {
				Edit interface{} `json:"edit"`
			}
			var applyErr error
			if err := json.Unmarshal(msg.Params, &p); err == nil && p.Edit != nil {
				applyErr = c.ApplyWorkspaceEdit(context.Background(), p.Edit)
			}
			result := map[string]interface{}{"applied": applyErr == nil}
			if applyErr != nil {
				result["failureReason"] = applyErr.Error()
			}
			c.sendResponse(*msg.ID, result)
		}
```

### Task 2 — Fix rootURI encoding in Initialize (client.go)

In `Initialize` (line 488), find:
```go
	c.rootDir = rootDir
	rootURI := "file://" + rootDir
```

Replace with:
```go
	c.rootDir = rootDir
	rootURI := (&url.URL{Scheme: "file", Path: rootDir}).String()
```

`net/url` is already imported in client.go (used by `uriToPath` at line 1797).
No import change needed.

### Task 3 — Extract applyDocumentChanges helper (client.go)

The `documentChanges` dispatch branch inside `ApplyWorkspaceEdit` (lines
1409-1462) is 54 lines handling 4 cases. Extract it to a private helper.

In `ApplyWorkspaceEdit`, find the block starting with:
```go
	// Process documentChanges first if present.
	if dc, ok := editMap["documentChanges"]; ok {
		b, _ := json.Marshal(dc)
		// documentChanges is (TextDocumentEdit | CreateFile | RenameFile | DeleteFile)[].
		// Each entry is discriminated by the presence of a "kind" field.
		var raw []json.RawMessage
		if err := json.Unmarshal(b, &raw); err == nil {
```

Replace the entire `if dc, ok := editMap["documentChanges"]; ok { ... return nil }` block
(ending at the `return nil` on line ~1462) with a call to the new helper:
```go
	// Process documentChanges first if present.
	if dc, ok := editMap["documentChanges"]; ok {
		return c.applyDocumentChanges(ctx, dc)
	}
```

Then add the extracted helper immediately before `ApplyWorkspaceEdit`:
```go
// applyDocumentChanges handles the documentChanges branch of a WorkspaceEdit.
// documentChanges is (TextDocumentEdit | CreateFile | RenameFile | DeleteFile)[].
// Each entry is discriminated by the presence of a "kind" field.
func (c *LSPClient) applyDocumentChanges(ctx context.Context, dc interface{}) error {
	b, _ := json.Marshal(dc)
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil // not an array; ignore
	}
	for _, entry := range raw {
		var disc struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(entry, &disc)
		switch disc.Kind {
		case "create":
			var op struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(entry, &op); err == nil && op.URI != "" {
				path := uriToPath(op.URI)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					_ = os.WriteFile(path, []byte{}, 0644)
				}
			}
		case "rename":
			var op struct {
				OldURI string `json:"oldUri"`
				NewURI string `json:"newUri"`
			}
			if err := json.Unmarshal(entry, &op); err == nil {
				_ = os.Rename(uriToPath(op.OldURI), uriToPath(op.NewURI))
			}
		case "delete":
			var op struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(entry, &op); err == nil && op.URI != "" {
				_ = os.Remove(uriToPath(op.URI))
			}
		default:
			// TextDocumentEdit (no kind field).
			var te struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
				Edits []textEdit `json:"edits"`
			}
			if err := json.Unmarshal(entry, &te); err == nil && te.TextDocument.URI != "" {
				if err := c.applyEditsToFile(ctx, te.TextDocument.URI, te.Edits); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
```

### Task 4 — Fix bare URI construction in resources.go

In `HandleDiagnosticsResource` (resources.go line 91), find:
```go
	fileURI := "file://" + path
```
This is already using `url.Parse` for the incoming URI, but constructs the
outgoing URI with bare concatenation. Replace with:
```go
	fileURI := (&url.URL{Scheme: "file", Path: path}).String()
```

In `HandleHoverResource` (resources.go line 152), find:
```go
	fileURI := "file://" + filePath
```
Replace with:
```go
	fileURI := (&url.URL{Scheme: "file", Path: filePath}).String()
```

In `HandleCompletionsResource` (resources.go line 205), find:
```go
	fileURI := "file://" + filePath
```
Replace with:
```go
	fileURI := (&url.URL{Scheme: "file", Path: filePath}).String()
```

`"net/url"` is already imported in resources.go (line 7) — no import change needed.

### Task 5 — Extract parseResourceQueryParams helper (resources.go)

`HandleHoverResource` (lines 116-167) and `HandleCompletionsResource`
(lines 169-225) share identical 20-line prefix code for parsing query params.
Extract a private helper.

Add this function before `HandleHoverResource`:
```go
// parseResourceQueryParams parses the file path, position, and language ID
// from an lsp-hover:// or lsp-completions:// URI.
// Returns an error if required query params are missing or invalid.
func parseResourceQueryParams(uri string) (filePath string, pos types.Position, languageID string, err error) {
	parsed, pErr := url.Parse(uri)
	if pErr != nil {
		return "", types.Position{}, "", fmt.Errorf("invalid URI %q: %w", uri, pErr)
	}

	filePath = parsed.Path
	q := parsed.Query()

	lineStr := q.Get("line")
	colStr := q.Get("column")
	languageID = q.Get("language_id")

	if lineStr == "" || colStr == "" || languageID == "" {
		return "", types.Position{}, "", fmt.Errorf("URI missing required query params (line, column, language_id)")
	}

	line, lErr := strconv.Atoi(lineStr)
	if lErr != nil {
		return "", types.Position{}, "", fmt.Errorf("invalid line %q: %w", lineStr, lErr)
	}
	col, cErr := strconv.Atoi(colStr)
	if cErr != nil {
		return "", types.Position{}, "", fmt.Errorf("invalid column %q: %w", colStr, cErr)
	}

	// URI params are 1-indexed; LSP is 0-indexed.
	pos = types.Position{Line: line - 1, Character: col - 1}
	return filePath, pos, languageID, nil
}
```

Then refactor `HandleHoverResource` to use the helper. Replace lines 118-145:
```go
func HandleHoverResource(ctx context.Context, client *lsp.LSPClient, uri string) (ResourceResult, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid URI %q: %w", uri, err)
	}

	filePath := parsed.Path
	q := parsed.Query()

	lineStr := q.Get("line")
	colStr := q.Get("column")
	languageID := q.Get("language_id")

	if lineStr == "" || colStr == "" || languageID == "" {
		return ResourceResult{}, fmt.Errorf("hover URI missing required query params (line, column, language_id)")
	}

	line, err := strconv.Atoi(lineStr)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid line %q: %w", lineStr, err)
	}
	col, err := strconv.Atoi(colStr)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid column %q: %w", colStr, err)
	}

	// URI params are 1-indexed; LSP is 0-indexed.
	pos := types.Position{Line: line - 1, Character: col - 1}
```

With:
```go
func HandleHoverResource(ctx context.Context, client *lsp.LSPClient, uri string) (ResourceResult, error) {
	filePath, pos, languageID, err := parseResourceQueryParams(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("hover resource: %w", err)
	}
```

Apply the same refactoring to `HandleCompletionsResource` (lines 171-198):
Replace the identical prefix block with:
```go
func HandleCompletionsResource(ctx context.Context, client *lsp.LSPClient, uri string) (ResourceResult, error) {
	filePath, pos, languageID, err := parseResourceQueryParams(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("completions resource: %w", err)
	}
```

After refactoring, the `strconv` import in resources.go may still be needed
inside `parseResourceQueryParams` — confirm it remains in the import block.
Remove unused imports if `url` or `strconv` usage in the old handlers was
the only site.

### Task 6 — Fix Shutdown multi-error in manager.go

In `internal/lsp/manager.go` `Shutdown` (lines 144-157), replace the
last-error-only pattern with `errors.Join`:

Find:
```go
	var lastErr error
	for _, e := range m.entries {
		if e.client != nil {
			if err := e.client.Shutdown(ctx); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
```

Replace with:
```go
	var errs []error
	for _, e := range m.entries {
		if e.client != nil {
			if err := e.client.Shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
```

Add `"errors"` to the import block of `internal/lsp/manager.go`.
Check the existing import block and add it alphabetically.

### Verification Gate
```
cd /Users/dayna.blackwell/workspace/code/LSP-MCP-GO
go build ./internal/lsp/... ./internal/resources/...
go vet ./internal/lsp/... ./internal/resources/...
go test ./internal/lsp/... ./internal/resources/...
# Postcondition checks:
grep '"applied": true' internal/lsp/client.go
# Expected: 0 (hardcoded true removed)
grep 'applyErr == nil' internal/lsp/client.go
# Expected: 1 match (new dynamic applied field)
grep 'applyDocumentChanges' internal/lsp/client.go
# Expected: 2 matches (definition + call site)
grep '"file://" + rootDir' internal/lsp/client.go
# Expected: 0 (bare concat removed)
grep '"file://" + ' internal/resources/resources.go
# Expected: 0 (all 3 bare concats replaced)
grep 'errors.Join' internal/lsp/manager.go
# Expected: 1 match
grep 'parseResourceQueryParams' internal/resources/resources.go
# Expected: 3 matches (definition + 2 call sites)
```

### Constraints
- Do NOT modify `internal/tools/helpers.go` — not in your ownership.
- Do NOT modify `cmd/lsp-mcp-go/server.go` — owned by Agent C.
- `errors.Join` requires Go 1.20+. The go.mod specifies `go 1.25.0` so this
  is safe.
- After refactoring Hover/Completions handlers with the helper, run
  `go build` to verify no unused imports remain.


## Interface Contracts

### SimulationSession.SetStatus

New mutex-guarded method for all external status transitions on
SimulationSession. Replaces direct session.Status = X assignments
in manager.go. Added by Agent A; called by Agent A in manager.go;
tested by Agent E in manager_test.go.

```
// SetStatus transitions the session to the given status under s.mu.
// Callers must NOT hold s.mu when calling this method.
func (s *SimulationSession) SetStatus(status SessionStatus) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.Status = status
}

```

### SimulationSession.OriginalContents

New field added to SimulationSession to store per-file pre-edit content
at baseline time. Used by Discard to revert LSP state without re-reading
disk. Added by Agent A in types.go; populated in ApplyEdit (baseline branch).

```
// In SimulationSession struct:
OriginalContents map[string]string // per-file original content at baseline time

```

### uriToPath (session package — replacement)

Replace session.uriToPath with the url.Parse-based implementation already
present in internal/lsp/client.go. Must handle percent-encoded paths (spaces,
Unicode). Agent A owns this change; no cross-agent dependency.

```
// uriToPath converts a file:// URI to a filesystem path,
// correctly decoding percent-encoded characters per RFC 3986.
func uriToPath(uri string) string {
    if u, err := url.Parse(uri); err == nil && u.Path != "" {
        return u.Path
    }
    if strings.HasPrefix(uri, "file://") {
        return uri[len("file://"):]
    }
    return uri
}

```

### HandleExecuteCommand args key fix

In HandleExecuteCommand (workspace.go line 207), the map lookup key must
be "arguments" (matching the JSON tag on ExecuteCommandArgs.Arguments in
server.go) not "args". One-line fix; no signature change.

```
// Change:
if v, ok := args["args"].([]interface{}); ok {
// To:
if v, ok := args["arguments"].([]interface{}); ok {

```

### ValidateFilePath call in HandleOpenDocument

HandleOpenDocument (session.go) must call ValidateFilePath before
CreateFileURI. The filePath must be validated against client.RootDir()
the same way WithDocument does internally. Agent B owns this change.

```
// After extracting filePath and before CreateFileURI:
if _, err := ValidateFilePath(filePath, client.RootDir()); err != nil {
    return types.ErrorResult(fmt.Sprintf("invalid file_path: %s", err)), nil
}

```

### workspace/applyEdit error propagation

In client.go dispatch (around line 286), the ApplyWorkspaceEdit error must
be checked and the response set to applied=false with failureReason when it
fails. Agent D owns this change.

```
// Change (in workspace/applyEdit case):
applyErr := c.ApplyWorkspaceEdit(context.Background(), p.Edit)
applied := applyErr == nil
result := map[string]interface{}{"applied": applied}
if applyErr != nil {
    result["failureReason"] = applyErr.Error()
}
c.sendResponse(*msg.ID, result)

```

### clientForFile routing fix

clientForFile (server.go line 108) must delegate to resolver.ClientForFile(filePath)
instead of always returning cs.get(). The csResolver.ClientForFile already
delegates to the real resolver correctly; clientForFile just needs to use it.
Agent C owns this change.

```
// Change:
func clientForFile(resolver lsp.ClientResolver, cs *clientState, filePath string) *lsp.LSPClient {
    return cs.get()
}
// To:
func clientForFile(resolver lsp.ClientResolver, cs *clientState, filePath string) *lsp.LSPClient {
    if filePath == "" {
        return cs.get()
    }
    if c := resolver.ClientForFile(filePath); c != nil {
        return c
    }
    return cs.get()
}

```

### Initialize rootURI encoding fix

In client.go Initialize (line 488), rootURI must use url.URL{Scheme:"file",
Path:rootDir}.String() instead of bare "file://"+rootDir concatenation.
Agent D owns this change; import "net/url" is already present in client.go.

```
// Change:
rootURI := "file://" + rootDir
// To:
rootURI := (&url.URL{Scheme: "file", Path: rootDir}).String()

```



## Quality Gates

Level: standard

- **build**: `go build ./...` (required: true)
- **lint**: `go vet ./...` (required: true)
- **test**: `go test ./...` (required: true)

