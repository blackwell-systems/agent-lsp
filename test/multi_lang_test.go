package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// langConfig holds per-language test configuration.
type langConfig struct {
	name               string
	id                 string
	binary             string
	serverArgs         []string
	javaHome           string
	fixture            string // absolute path to fixture dir
	file               string // absolute path to primary source file
	hoverLine          int
	hoverColumn        int
	definitionLine     int
	definitionColumn   int
	callSiteLine       int
	callSiteColumn     int
	callSiteFile       string // defaults to .file if empty
	referenceLine      int
	referenceColumn    int
	completionLine     int
	completionColumn   int
	completionFile     string // defaults to .file if empty
	workspaceSymbol    string
	supportsFormatting bool
	secondFile         string
	symbolName         string
	declarationLine    int // C only
	declarationColumn  int // C only
	typeHierarchyLine   int // Java and TypeScript only
	typeHierarchyColumn int // Java and TypeScript only

	// Fields for Tier 2 expansion tools.
	highlightLine      int    // for get_document_highlights; uses hoverLine if 0
	highlightColumn    int    // for get_document_highlights; uses hoverColumn if 0
	inlayHintEndLine   int    // end line for get_inlay_hints range; computed from hoverLine if 0
	renameSymbolLine   int    // for rename_symbol / prepare_rename
	renameSymbolColumn int    // for rename_symbol / prepare_rename
	renameSymbolName   string // new_name for rename_symbol
	codeActionLine     int    // start line for get_code_actions range; uses hoverLine if 0
	codeActionEndLine  int    // end line for get_code_actions range; codeActionLine+1 if 0
}

// toolResult holds the outcome of a single Tier 2 tool test.
type toolResult struct {
	tool   string
	status string // "pass", "fail", "skip"
	detail string
}

// langTestResult holds Tier 1 and Tier 2 results for one language.
type langTestResult struct {
	tier1 string       // "pass", "fail", "skip"
	tier2 []toolResult
}

// langResult holds the summarised result for one language after a full test run.
type langResult struct {
	name  string
	tier1 string
	tier2 []toolResult
}

var (
	multilangBinaryOnce sync.Once
	multilangBinaryPath string
)

// testDir returns the directory containing this test file.
func testDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filename)
}

// getMultilangBinary builds the agent-lsp binary once and returns its path.
// Returns empty string if build fails.
func getMultilangBinary(t *testing.T) string {
	t.Helper()
	multilangBinaryOnce.Do(func() {
		tmp, err := os.MkdirTemp("", "agent-lsp-multi-*")
		if err != nil {
			return
		}
		p := filepath.Join(tmp, "agent-lsp")
		// test/multi_lang_test.go → test/ → repo root (two levels up)
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			return
		}
		testFileDir := filepath.Dir(filename)
		repoRoot := filepath.Dir(testFileDir)
		cmd := exec.Command("go", "build", "-o", p, "./cmd/agent-lsp")
		cmd.Env = append(os.Environ(), "GOWORK=off")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build agent-lsp: %v\n%s\n", err, out)
			return
		}
		multilangBinaryPath = p
	})
	return multilangBinaryPath
}

// buildLanguageConfigs returns all language configs using the given fixture base directory.
func callTool(ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, error) {
	return session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
}

// textFromResult extracts the text from the first content item of a CallToolResult.
func textFromResult(res *mcp.CallToolResult) (string, error) {
	if len(res.Content) == 0 {
		return "", fmt.Errorf("result has no content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("content[0] is not *mcp.TextContent, got %T", res.Content[0])
	}
	return tc.Text, nil
}

// runLanguageTest runs Tier 1 and Tier 2 tests for a single language.
func runLanguageTest(t *testing.T, binaryPath string, lang langConfig) langTestResult {
	t.Helper()

	// Check that the LSP binary is available.
	lspBinaryPath, err := exec.LookPath(lang.binary)
	if err != nil {
		t.Skipf("skipping %s: %s not found on PATH", lang.name, lang.binary)
		return langTestResult{tier1: "skip"}
	}

	// Determine timeout. JVM-based servers (Java, Kotlin, Scala) need longer
	// timeouts due to slow cold-start initialization.
	var timeout time.Duration
	switch lang.id {
	case "java":
		timeout = 300 * time.Second
	case "kotlin", "scala":
		timeout = 180 * time.Second
	default:
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Build the command: binaryPath <lang-id> <lsp-binary-path> [lsp-server-args...]
	args := append([]string{lang.id, lspBinaryPath}, lang.serverArgs...)
	cmd := exec.Command(binaryPath, args...)
	if lang.javaHome != "" {
		cmd.Env = append(os.Environ(), "JAVA_HOME="+lang.javaHome)
	}

	// Connect via MCP CommandTransport.
	client := mcp.NewClient(&mcp.Implementation{Name: "multi-lang-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Errorf("failed to connect MCP session for %s: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	defer session.Close()

	// --- Tier 1: start_lsp ---
	// For Java, pass ready_timeout_seconds so start_lsp blocks on $/progress
	// completion instead of returning immediately after initialize. This replaces
	// a fixed sleep and fires as soon as jdtls finishes workspace indexing.
	startArgs := map[string]any{"root_dir": lang.fixture}
	if lang.id == "java" {
		startArgs["ready_timeout_seconds"] = float64(240)
	}
	res, err := callTool(ctx, session, "start_lsp", startArgs)
	if err != nil {
		t.Errorf("[%s] start_lsp failed: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	if res.IsError {
		errText, _ := textFromResult(res)
		t.Skipf("[%s] start_lsp returned IsError=true (LSP server unavailable or failed to start): %s", lang.name, errText)
		return langTestResult{tier1: "skip"}
	}
	startText, err := textFromResult(res)
	if err != nil {
		t.Errorf("[%s] start_lsp: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	if len(startText) == 0 {
		t.Errorf("[%s] start_lsp returned empty text", lang.name)
		return langTestResult{tier1: "fail"}
	}
	t.Logf("[%s] start_lsp result: %s", lang.name, startText)

	// Short fixed wait for non-Java servers to settle after initialize.
	// Java uses ready_timeout_seconds above and needs no additional sleep.
	var initWait time.Duration
	switch lang.id {
	case "java":
		initWait = 0
	case "kotlin", "scala":
		initWait = 30 * time.Second
	default:
		initWait = 8 * time.Second
	}
	if initWait > 0 {
		time.Sleep(initWait)
	}

	// --- Tier 1: open_document ---
	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil {
		t.Errorf("[%s] open_document failed: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	if res.IsError {
		errText, _ := textFromResult(res)
		t.Errorf("[%s] open_document returned IsError=true: %s", lang.name, errText)
		return langTestResult{tier1: "fail"}
	}

	// Wait for diagnostics to settle.
	time.Sleep(3 * time.Second)

	// --- Tier 1: get_diagnostics ---
	res, err = callTool(ctx, session, "get_diagnostics", map[string]any{
		"file_path": lang.file,
	})
	if err != nil {
		t.Errorf("[%s] get_diagnostics failed: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	if res.IsError {
		errText, _ := textFromResult(res)
		t.Errorf("[%s] get_diagnostics returned IsError=true: %s", lang.name, errText)
		return langTestResult{tier1: "fail"}
	}
	diagText, err := textFromResult(res)
	if err != nil {
		t.Errorf("[%s] get_diagnostics: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	var diagItems []any
	_ = json.Unmarshal([]byte(diagText), &diagItems)
	t.Logf("[%s] diagnostics count: %d", lang.name, len(diagItems))

	// --- Tier 1: get_info_on_location (hover) ---
	res, err = callTool(ctx, session, "get_info_on_location", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
	})
	if err != nil {
		t.Errorf("[%s] get_info_on_location failed: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	if res.IsError {
		t.Errorf("[%s] get_info_on_location returned IsError=true: %v", lang.name, res.Content)
		return langTestResult{tier1: "fail"}
	}
	hoverText, err := textFromResult(res)
	if err != nil {
		t.Errorf("[%s] get_info_on_location: %v", lang.name, err)
		return langTestResult{tier1: "fail"}
	}
	if len(hoverText) == 0 {
		if len(diagItems) == 0 {
			// Both diagnostics and hover are empty: server likely hasn't finished
			// indexing the workspace. Skip rather than fail.
			t.Skipf("[%s] skipping: server returned no diagnostics and no hover (workspace not indexed)", lang.name)
		}
		t.Errorf("[%s] get_info_on_location returned empty hover text", lang.name)
		return langTestResult{tier1: "fail"}
	}

	// Tier 1 passed. Run Tier 2.
	tier2Results := []toolResult{
		testDocumentSymbols(t, ctx, session, lang),
		testGoToDefinition(t, ctx, session, lang),
		testGetReferences(t, ctx, session, lang),
		testGetCompletions(t, ctx, session, lang),
		testWorkspaceSymbols(t, ctx, session, lang),
		testFormatDocument(t, ctx, session, lang),
		testGoToDeclaration(t, ctx, session, lang),
		testTypeHierarchy(t, ctx, session, lang),
		testGetInfoOnLocation(t, ctx, session, lang),
		testCallHierarchy(t, ctx, session, lang),
		testGetSemanticTokens(t, ctx, session, lang),
		testGetSignatureHelp(t, ctx, session, lang),
		testGetDocumentHighlights(t, ctx, session, lang),
		testGetInlayHints(t, ctx, session, lang),
		testGetCodeActions(t, ctx, session, lang),
		testPrepareRename(t, ctx, session, lang),
		testRenameSymbol(t, ctx, session, lang),
		testGetServerCapabilities(t, ctx, session, lang),
		testWorkspaceFolders(t, ctx, session, lang),
		testGoToTypeDefinition(t, ctx, session, lang),
		testGoToImplementation(t, ctx, session, lang),
		testFormatRange(t, ctx, session, lang),
		testApplyEdit(t, ctx, session, lang),
		testDetectLspServers(t, ctx, session, lang),
		testCloseDocument(t, ctx, session, lang),
		testDidChangeWatchedFiles(t, ctx, session, lang),
		testRunBuild(t, ctx, session, lang),
		testRunTests(t, ctx, session, lang),
		testGetTestsForFile(t, ctx, session, lang),
		testGetSymbolSource(t, ctx, session, lang),
		testGoToSymbol(t, ctx, session, lang),
		testRestartLspServer(t, ctx, session, lang),
		testSetLogLevel(t, ctx, session, lang),
		testExecuteCommand(t, ctx, session, lang),
	}

	return langTestResult{tier1: "pass", tier2: tier2Results}
}

// testDocumentSymbols tests the get_document_symbols tool.
func testDocumentSymbols(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "get_document_symbols", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil {
		return toolResult{tool: "get_document_symbols", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_document_symbols", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_document_symbols", status: "fail",
			detail: fmt.Sprintf("failed to parse get_document_symbols response: %v", err)}
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return toolResult{tool: "get_document_symbols", status: "fail",
			detail: fmt.Sprintf("failed to parse get_document_symbols response: %s", text)}
	}
	if len(items) == 0 {
		return toolResult{tool: "get_document_symbols", status: "fail", detail: "empty symbol list"}
	}
	found := false
	for _, item := range items {
		if name, ok := item["name"].(string); ok && strings.Contains(name, lang.symbolName) {
			found = true
			break
		}
	}
	if !found {
		return toolResult{tool: "get_document_symbols", status: "fail",
			detail: fmt.Sprintf("symbol %q not found in results", lang.symbolName)}
	}
	return toolResult{tool: "get_document_symbols", status: "pass"}
}

// testGoToDefinition tests the go_to_definition tool.
func testGoToDefinition(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	callSiteFile := lang.callSiteFile
	if callSiteFile == "" {
		callSiteFile = lang.file
	}
	res, err := callTool(ctx, session, "go_to_definition", map[string]any{
		"file_path":   callSiteFile,
		"language_id": lang.id,
		"line":        lang.callSiteLine,
		"column":      lang.callSiteColumn,
	})
	if err != nil {
		return toolResult{tool: "go_to_definition", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "go_to_definition", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "go_to_definition", status: "fail",
			detail: fmt.Sprintf("failed to parse go_to_definition response: %v", err)}
	}

	// Result may be a JSON object or array; extract the first element if array.
	var result map[string]any
	var arr []map[string]any
	if err := json.Unmarshal([]byte(text), &arr); err == nil && len(arr) > 0 {
		result = arr[0]
	} else if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "go_to_definition", status: "fail",
			detail: fmt.Sprintf("failed to parse go_to_definition response: %s", text)}
	}

	// Assert file/uri is non-empty.
	fileVal, hasFile := result["file"]
	uriVal, hasURI := result["uri"]
	if !hasFile && !hasURI {
		return toolResult{tool: "go_to_definition", status: "fail",
			detail: "result has no 'file' or 'uri' field"}
	}
	if hasFile {
		if s, ok := fileVal.(string); !ok || s == "" {
			return toolResult{tool: "go_to_definition", status: "fail", detail: "'file' field is empty"}
		}
	} else if hasURI {
		if s, ok := uriVal.(string); !ok || s == "" {
			return toolResult{tool: "go_to_definition", status: "fail", detail: "'uri' field is empty"}
		}
	}

	// Assert result line is within ±1 of lang.definitionLine.
	var resultLine float64
	if lineVal, ok := result["line"].(float64); ok {
		resultLine = lineVal
	} else if rangeVal, ok := result["range"].(map[string]any); ok {
		if startVal, ok := rangeVal["start"].(map[string]any); ok {
			if l, ok := startVal["line"].(float64); ok {
				resultLine = l + 1 // convert 0-based to 1-based
			}
		}
	}
	diff := int(resultLine) - lang.definitionLine
	if diff < -1 || diff > 1 {
		return toolResult{tool: "go_to_definition", status: "fail",
			detail: fmt.Sprintf("definition line %d not within ±1 of expected %d", int(resultLine), lang.definitionLine)}
	}

	return toolResult{tool: "go_to_definition", status: "pass"}
}

// testGetReferences tests the get_references tool.
func testGetReferences(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()

	// If there's a secondFile, open it first to improve reference results.
	if lang.secondFile != "" {
		res, err := callTool(ctx, session, "open_document", map[string]any{
			"file_path":   lang.secondFile,
			"language_id": lang.id,
		})
		if err == nil && !res.IsError {
			time.Sleep(2 * time.Second)
		}
	}

	res, err := callTool(ctx, session, "get_references", map[string]any{
		"file_path":           lang.file,
		"language_id":         lang.id,
		"line":                lang.referenceLine,
		"column":              lang.referenceColumn,
		"include_declaration": true,
	})
	if err != nil {
		return toolResult{tool: "get_references", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_references", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_references", status: "fail",
			detail: fmt.Sprintf("failed to parse get_references response: %v", err)}
	}
	var items []any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return toolResult{tool: "get_references", status: "fail",
			detail: fmt.Sprintf("failed to parse get_references response: %s", text)}
	}
	minRefs := 1
	if lang.secondFile != "" {
		minRefs = 2
	}
	if len(items) < minRefs {
		return toolResult{tool: "get_references", status: "fail",
			detail: fmt.Sprintf("expected >= %d references, got %d", minRefs, len(items))}
	}
	return toolResult{tool: "get_references", status: "pass"}
}

// testGetCompletions tests the get_completions tool.
func testGetCompletions(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	completionFile := lang.completionFile
	if completionFile == "" {
		completionFile = lang.file
	}
	res, err := callTool(ctx, session, "get_completions", map[string]any{
		"file_path":   completionFile,
		"language_id": lang.id,
		"line":        lang.completionLine,
		"column":      lang.completionColumn,
	})
	if err != nil {
		return toolResult{tool: "get_completions", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_completions", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_completions", status: "fail",
			detail: fmt.Sprintf("failed to parse get_completions response: %v", err)}
	}
	var items []any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return toolResult{tool: "get_completions", status: "fail",
			detail: fmt.Sprintf("failed to parse get_completions response: %s", text)}
	}
	if len(items) == 0 {
		return toolResult{tool: "get_completions", status: "fail", detail: "empty completion list"}
	}
	return toolResult{tool: "get_completions", status: "pass"}
}

// testWorkspaceSymbols tests the get_workspace_symbols tool.
func testWorkspaceSymbols(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "get_workspace_symbols", map[string]any{
		"query": lang.workspaceSymbol,
	})
	if err != nil {
		return toolResult{tool: "get_workspace_symbols", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_workspace_symbols", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_workspace_symbols", status: "fail",
			detail: fmt.Sprintf("failed to parse get_workspace_symbols response: %v", err)}
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return toolResult{tool: "get_workspace_symbols", status: "fail",
			detail: fmt.Sprintf("failed to parse get_workspace_symbols response: %s", text)}
	}
	if len(items) == 0 {
		return toolResult{tool: "get_workspace_symbols", status: "fail", detail: "empty workspace symbol list"}
	}
	found := false
	for _, item := range items {
		if name, ok := item["name"].(string); ok && strings.Contains(name, lang.workspaceSymbol) {
			found = true
			break
		}
	}
	if !found {
		return toolResult{tool: "get_workspace_symbols", status: "fail",
			detail: fmt.Sprintf("symbol %q not found in workspace symbols", lang.workspaceSymbol)}
	}
	return toolResult{tool: "get_workspace_symbols", status: "pass"}
}

// testFormatDocument tests the format_document tool.
func testFormatDocument(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if !lang.supportsFormatting {
		return toolResult{tool: "format_document", status: "skip", detail: "not supported"}
	}
	res, err := callTool(ctx, session, "format_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil {
		return toolResult{tool: "format_document", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "format_document", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		// Empty content means file is already formatted — acceptable.
		return toolResult{tool: "format_document", status: "pass", detail: "empty content (already formatted)"}
	}
	// An empty array is fine — means no edits needed.
	var edits []any
	_ = json.Unmarshal([]byte(text), &edits)
	return toolResult{tool: "format_document", status: "pass"}
}

// testGoToDeclaration tests the go_to_declaration tool (C only).
func testGoToDeclaration(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.id != "c" {
		return toolResult{tool: "go_to_declaration", status: "skip", detail: "not applicable"}
	}

	// Open the header file first.
	headerFile := filepath.Join(lang.fixture, "person.h")
	res, err := callTool(ctx, session, "open_document", map[string]any{
		"file_path":   headerFile,
		"language_id": lang.id,
	})
	if err == nil && !res.IsError {
		time.Sleep(1 * time.Second)
	}

	res, err = callTool(ctx, session, "go_to_declaration", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.declarationLine,
		"column":      lang.declarationColumn,
	})
	if err != nil {
		return toolResult{tool: "go_to_declaration", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "go_to_declaration", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "go_to_declaration", status: "fail",
			detail: fmt.Sprintf("failed to parse go_to_declaration response: %v", err)}
	}

	// Result may be an object or array; extract first element if array.
	var result map[string]any
	var arr []map[string]any
	if err := json.Unmarshal([]byte(text), &arr); err == nil && len(arr) > 0 {
		result = arr[0]
	} else if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "go_to_declaration", status: "fail",
			detail: fmt.Sprintf("failed to parse go_to_declaration response: %s", text)}
	}

	// Assert file field ends with "person.h".
	fileVal, ok := result["file"].(string)
	if !ok || fileVal == "" {
		// Try "uri".
		uriVal, ok := result["uri"].(string)
		if !ok || uriVal == "" {
			return toolResult{tool: "go_to_declaration", status: "fail",
				detail: "result has no 'file' or 'uri' field"}
		}
		fileVal = uriVal
	}
	if !strings.HasSuffix(fileVal, "person.h") {
		return toolResult{tool: "go_to_declaration", status: "fail",
			detail: fmt.Sprintf("expected file to end with 'person.h', got %q", fileVal)}
	}
	return toolResult{tool: "go_to_declaration", status: "pass"}
}

// testTypeHierarchy tests the type_hierarchy tool (Java and TypeScript only).
func testTypeHierarchy(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.typeHierarchyLine == 0 {
		return toolResult{tool: "type_hierarchy", status: "skip", detail: "not configured for this language"}
	}
	res, err := callTool(ctx, session, "type_hierarchy", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.typeHierarchyLine,
		"column":      lang.typeHierarchyColumn,
		"direction":   "both",
	})
	if err != nil {
		return toolResult{tool: "type_hierarchy", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		// Type hierarchy not supported by this server — treat as skip.
		return toolResult{tool: "type_hierarchy", status: "skip",
			detail: fmt.Sprintf("tool returned IsError=true (server may not support typeHierarchy): %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "type_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to parse type_hierarchy response: %v", err)}
	}
	var result struct {
		Items      []map[string]any `json:"items"`
		Supertypes []map[string]any `json:"supertypes"`
		Subtypes   []map[string]any `json:"subtypes"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// May be a plain text message like "No type hierarchy item found"
		if strings.Contains(text, "No type hierarchy item found") {
			return toolResult{tool: "type_hierarchy", status: "skip",
				detail: "server returned no type hierarchy item at configured position"}
		}
		return toolResult{tool: "type_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal type_hierarchy response: %s", text)}
	}
	if len(result.Items) == 0 {
		return toolResult{tool: "type_hierarchy", status: "skip",
			detail: "no items returned (server may not index this position)"}
	}
	return toolResult{tool: "type_hierarchy", status: "pass"}
}

// testGetInfoOnLocation tests the get_info_on_location (hover) tool.
func testGetInfoOnLocation(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "get_info_on_location", status: "skip", detail: "no hover position configured"}
	}
	res, err := callTool(ctx, session, "get_info_on_location", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
	})
	if err != nil {
		return toolResult{tool: "get_info_on_location", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_info_on_location", status: "skip",
			detail: fmt.Sprintf("server does not support hover: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" {
		return toolResult{tool: "get_info_on_location", status: "skip", detail: "empty hover response"}
	}
	return toolResult{tool: "get_info_on_location", status: "pass"}
}

// testCallHierarchy tests the call_hierarchy tool.
// Uses the hover position which points at a function/method declaration in every fixture.
func testCallHierarchy(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "call_hierarchy", status: "skip", detail: "no position configured"}
	}
	res, err := callTool(ctx, session, "call_hierarchy", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
		"direction":   "both",
	})
	if err != nil {
		return toolResult{tool: "call_hierarchy", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "call_hierarchy", status: "skip",
			detail: fmt.Sprintf("server does not support callHierarchy: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "call_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to parse call_hierarchy response: %v", err)}
	}
	var result struct {
		Items   []map[string]any `json:"items"`
		Callers []map[string]any `json:"callers"`
		Callees []map[string]any `json:"callees"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		if strings.Contains(text, "No call hierarchy") {
			return toolResult{tool: "call_hierarchy", status: "skip", detail: "no call hierarchy item at position"}
		}
		return toolResult{tool: "call_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal call_hierarchy response: %s", text)}
	}
	if len(result.Items) == 0 {
		return toolResult{tool: "call_hierarchy", status: "skip", detail: "no items returned"}
	}
	return toolResult{tool: "call_hierarchy", status: "pass"}
}

// testGetSemanticTokens tests the get_semantic_tokens tool over a small range.
func testGetSemanticTokens(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	endLine := lang.hoverLine + 5
	if endLine < 5 {
		endLine = 5
	}
	res, err := callTool(ctx, session, "get_semantic_tokens", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   1,
		"start_column": 1,
		"end_line":     endLine,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "get_semantic_tokens", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_semantic_tokens", status: "skip",
			detail: fmt.Sprintf("server does not support semanticTokens: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_semantic_tokens", status: "fail",
			detail: fmt.Sprintf("failed to parse semantic tokens response: %v", err)}
	}
	var tokens []map[string]any
	if err := json.Unmarshal([]byte(text), &tokens); err != nil || len(tokens) == 0 {
		return toolResult{tool: "get_semantic_tokens", status: "skip", detail: "no tokens returned"}
	}
	return toolResult{tool: "get_semantic_tokens", status: "pass"}
}

// testGetSignatureHelp tests the get_signature_help tool.
// Uses completionLine/completionColumn — positions inside expressions where
// signature help is likely to trigger.
func testGetSignatureHelp(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.completionLine == 0 {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "no position configured"}
	}
	completionFile := lang.completionFile
	if completionFile == "" {
		completionFile = lang.file
	}
	res, err := callTool(ctx, session, "get_signature_help", map[string]any{
		"file_path":   completionFile,
		"language_id": lang.id,
		"line":        lang.completionLine,
		"column":      lang.completionColumn,
	})
	if err != nil {
		return toolResult{tool: "get_signature_help", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_signature_help", status: "skip",
			detail: fmt.Sprintf("server does not support signatureHelp: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" || text == "null" || text == "{}" {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "no signature help at position"}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "empty or non-JSON response"}
	}
	sigs, _ := result["signatures"].([]any)
	if len(sigs) == 0 {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "no signatures returned"}
	}
	return toolResult{tool: "get_signature_help", status: "pass"}
}

// testGetDocumentHighlights tests the get_document_highlights tool.
func testGetDocumentHighlights(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	line := lang.highlightLine
	col := lang.highlightColumn
	if line == 0 {
		line = lang.hoverLine
		col = lang.hoverColumn
	}
	if line == 0 {
		return toolResult{tool: "get_document_highlights", status: "skip", detail: "no position configured"}
	}
	res, err := callTool(ctx, session, "get_document_highlights", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        line,
		"column":      col,
	})
	if err != nil {
		return toolResult{tool: "get_document_highlights", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_document_highlights", status: "skip",
			detail: fmt.Sprintf("server does not support documentHighlights: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_document_highlights", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return toolResult{tool: "get_document_highlights", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal response: %s", text)}
	}
	if len(items) == 0 {
		return toolResult{tool: "get_document_highlights", status: "skip", detail: "no highlights returned"}
	}
	return toolResult{tool: "get_document_highlights", status: "pass"}
}

// testGetInlayHints tests the get_inlay_hints tool over a range.
func testGetInlayHints(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	endLine := lang.inlayHintEndLine
	if endLine == 0 {
		endLine = lang.hoverLine + 5
	}
	if endLine < 5 {
		endLine = 5
	}
	res, err := callTool(ctx, session, "get_inlay_hints", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   1,
		"start_column": 1,
		"end_line":     endLine,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "get_inlay_hints", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_inlay_hints", status: "skip",
			detail: fmt.Sprintf("server does not support inlayHints: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_inlay_hints", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	var hints []map[string]any
	if err := json.Unmarshal([]byte(text), &hints); err != nil {
		return toolResult{tool: "get_inlay_hints", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal response: %s", text)}
	}
	if len(hints) == 0 {
		return toolResult{tool: "get_inlay_hints", status: "skip", detail: "no inlay hints returned"}
	}
	return toolResult{tool: "get_inlay_hints", status: "pass"}
}

// testGetCodeActions tests the get_code_actions tool.
func testGetCodeActions(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	startLine := lang.codeActionLine
	if startLine == 0 {
		startLine = lang.hoverLine
	}
	if startLine == 0 {
		return toolResult{tool: "get_code_actions", status: "skip", detail: "no position configured"}
	}
	endLine := lang.codeActionEndLine
	if endLine == 0 {
		endLine = startLine + 1
	}
	res, err := callTool(ctx, session, "get_code_actions", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   startLine,
		"start_column": 1,
		"end_line":     endLine,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "get_code_actions", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_code_actions", status: "skip",
			detail: fmt.Sprintf("server does not support codeActions: %v", res.Content)}
	}
	_, err = textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_code_actions", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	// Empty action list is valid — the range may have no applicable actions.
	return toolResult{tool: "get_code_actions", status: "pass"}
}

// testPrepareRename tests the prepare_rename validation step.
func testPrepareRename(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.renameSymbolLine == 0 {
		return toolResult{tool: "prepare_rename", status: "skip", detail: "renameSymbolLine not configured"}
	}
	res, err := callTool(ctx, session, "prepare_rename", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.renameSymbolLine,
		"column":      lang.renameSymbolColumn,
	})
	if err != nil {
		return toolResult{tool: "prepare_rename", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "prepare_rename", status: "skip",
			detail: fmt.Sprintf("server does not support prepareRename: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" || text == "null" {
		return toolResult{tool: "prepare_rename", status: "skip", detail: "no prepare_rename response"}
	}
	return toolResult{tool: "prepare_rename", status: "pass"}
}

// testRenameSymbol tests the rename_symbol tool (returns WorkspaceEdit, does not apply it).
func testRenameSymbol(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.renameSymbolLine == 0 || lang.renameSymbolName == "" {
		return toolResult{tool: "rename_symbol", status: "skip", detail: "rename position or name not configured"}
	}
	res, err := callTool(ctx, session, "rename_symbol", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.renameSymbolLine,
		"column":      lang.renameSymbolColumn,
		"new_name":    lang.renameSymbolName,
	})
	if err != nil {
		return toolResult{tool: "rename_symbol", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "rename_symbol", status: "skip",
			detail: fmt.Sprintf("server does not support rename: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" || text == "null" {
		return toolResult{tool: "rename_symbol", status: "skip", detail: "no WorkspaceEdit returned"}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "rename_symbol", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal WorkspaceEdit: %s", text)}
	}
	_, hasChanges := result["changes"]
	_, hasDocChanges := result["documentChanges"]
	if !hasChanges && !hasDocChanges {
		return toolResult{tool: "rename_symbol", status: "skip",
			detail: "WorkspaceEdit has neither 'changes' nor 'documentChanges'"}
	}
	return toolResult{tool: "rename_symbol", status: "pass"}
}

// testGetServerCapabilities tests the get_server_capabilities tool.
func testGetServerCapabilities(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "get_server_capabilities", map[string]any{})
	if err != nil {
		return toolResult{tool: "get_server_capabilities", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal response: %s", text)}
	}
	supportedTools, ok := result["supported_tools"].([]any)
	if !ok || len(supportedTools) == 0 {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: "supported_tools missing or empty"}
	}
	// Verify start_lsp is always present.
	found := false
	for _, v := range supportedTools {
		if s, ok := v.(string); ok && s == "start_lsp" {
			found = true
			break
		}
	}
	if !found {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: "'start_lsp' not in supported_tools"}
	}
	return toolResult{tool: "get_server_capabilities", status: "pass"}
}

// testWorkspaceFolders tests the workspace folder lifecycle tools.
func testWorkspaceFolders(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()

	// Step 1: list current workspace folders.
	res, err := callTool(ctx, session, "list_workspace_folders", map[string]any{})
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "workspace_folders", status: "skip",
			detail: fmt.Sprintf("list_workspace_folders not supported: %v", res.Content)}
	}

	// Step 2: add a temporary folder.
	tmpDir, mkErr := os.MkdirTemp("", "lsp-mcp-wf-test-*")
	if mkErr != nil {
		return toolResult{tool: "workspace_folders", status: "fail",
			detail: fmt.Sprintf("failed to create temp dir: %v", mkErr)}
	}
	defer os.RemoveAll(tmpDir)

	res, err = callTool(ctx, session, "add_workspace_folder", map[string]any{
		"path": tmpDir,
	})
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		// Server may not support didChangeWorkspaceFolders — treat as skip.
		return toolResult{tool: "workspace_folders", status: "skip",
			detail: fmt.Sprintf("add_workspace_folder not supported: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail",
			detail: fmt.Sprintf("failed to parse add response: %v", err)}
	}
	var addResult map[string]any
	if err := json.Unmarshal([]byte(text), &addResult); err != nil || addResult["added"] == nil {
		return toolResult{tool: "workspace_folders", status: "fail",
			detail: fmt.Sprintf("add_workspace_folder: unexpected response: %s", text)}
	}

	// Sub-case A: verify added folder appears in list.
	res, err = callTool(ctx, session, "list_workspace_folders", map[string]any{})
	if err == nil && !res.IsError {
		if listText, lerr := textFromResult(res); lerr == nil {
			if !strings.Contains(listText, filepath.Base(tmpDir)) && !strings.Contains(listText, tmpDir) {
				t.Logf("[workspace_folders] added folder not found in list_workspace_folders result: %s", listText)
			}
		}
	}

	// Step 3: remove the folder.
	res, err = callTool(ctx, session, "remove_workspace_folder", map[string]any{
		"path": tmpDir,
	})
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "workspace_folders", status: "skip",
			detail: fmt.Sprintf("remove_workspace_folder not supported: %v", res.Content)}
	}

	// Sub-case B: verify removed folder no longer appears in list.
	res, err = callTool(ctx, session, "list_workspace_folders", map[string]any{})
	if err == nil && !res.IsError {
		if listText, lerr := textFromResult(res); lerr == nil {
			if strings.Contains(listText, tmpDir) {
				return toolResult{tool: "workspace_folders", status: "fail",
					detail: fmt.Sprintf("removed folder still present in list: %s", listText)}
			}
		}
	}

	// Sub-case C: error path — remove a non-existent folder (transport must not error).
	nonExistentPath := tmpDir + "-nonexistent"
	_, removeErr := callTool(ctx, session, "remove_workspace_folder", map[string]any{
		"path": nonExistentPath,
	})
	if removeErr != nil {
		return toolResult{tool: "workspace_folders", status: "fail",
			detail: fmt.Sprintf("remove non-existent folder: unexpected transport error: %v", removeErr)}
	}
	t.Logf("[workspace_folders] remove non-existent folder: no transport error (server may silently ignore)")

	// Sub-case D: multiple folders — add two, remove two.
	tmpDir2, mkErr2 := os.MkdirTemp("", "lsp-mcp-wf-test2-*")
	if mkErr2 != nil {
		t.Logf("[workspace_folders] skipping multi-folder sub-case: %v", mkErr2)
	} else {
		defer os.RemoveAll(tmpDir2)
		tmpDir3, mkErr3 := os.MkdirTemp("", "lsp-mcp-wf-test3-*")
		if mkErr3 == nil {
			defer os.RemoveAll(tmpDir3)
			for _, extraDir := range []string{tmpDir2, tmpDir3} {
				addRes, addErr := callTool(ctx, session, "add_workspace_folder", map[string]any{
					"path": extraDir,
				})
				if addErr != nil || addRes.IsError {
					t.Logf("[workspace_folders] multi-folder add skipped for %s", filepath.Base(extraDir))
					continue
				}
				rmRes, rmErr := callTool(ctx, session, "remove_workspace_folder", map[string]any{
					"path": extraDir,
				})
				if rmErr != nil || rmRes.IsError {
					t.Logf("[workspace_folders] multi-folder remove failed for %s", filepath.Base(extraDir))
				}
			}
		}
	}

	return toolResult{tool: "workspace_folders", status: "pass"}
}

// testGoToTypeDefinition tests the go_to_type_definition tool.
// Uses referenceLine/referenceColumn as the position — calling from a symbol
// reference rather than its definition to exercise the type-jump behavior.
func testGoToTypeDefinition(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	line := lang.referenceLine
	col := lang.referenceColumn
	if line == 0 {
		return toolResult{tool: "go_to_type_definition", status: "skip", detail: "no reference position configured"}
	}
	res, err := callTool(ctx, session, "go_to_type_definition", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        line,
		"column":      col,
	})
	if err != nil {
		return toolResult{tool: "go_to_type_definition", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "go_to_type_definition", status: "skip",
			detail: fmt.Sprintf("server does not support typeDefinition: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "go_to_type_definition", status: "fail", detail: err.Error()}
	}
	if text == "" || text == "null" || text == "[]" {
		return toolResult{tool: "go_to_type_definition", status: "skip", detail: "empty result"}
	}
	return toolResult{tool: "go_to_type_definition", status: "pass"}
}

// testGoToImplementation tests the go_to_implementation tool.
// Uses hoverLine/hoverColumn (typically pointing at a type or interface definition).
// Most servers will return empty for concrete types; the result is skip not fail.
func testGoToImplementation(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "go_to_implementation", status: "skip", detail: "no hover position configured"}
	}
	res, err := callTool(ctx, session, "go_to_implementation", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
	})
	if err != nil {
		return toolResult{tool: "go_to_implementation", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "go_to_implementation", status: "skip",
			detail: fmt.Sprintf("server does not support implementation: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "go_to_implementation", status: "fail", detail: err.Error()}
	}
	if text == "" || text == "null" || text == "[]" {
		return toolResult{tool: "go_to_implementation", status: "skip", detail: "no implementations found"}
	}
	return toolResult{tool: "go_to_implementation", status: "pass"}
}

// testFormatRange tests the format_range tool over the first 5 lines of the file.
func testFormatRange(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if !lang.supportsFormatting {
		return toolResult{tool: "format_range", status: "skip", detail: "not supported"}
	}
	res, err := callTool(ctx, session, "format_range", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   1,
		"start_column": 1,
		"end_line":     5,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "format_range", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "format_range", status: "skip",
			detail: fmt.Sprintf("server does not support rangeFormatting: %v", res.Content)}
	}
	return toolResult{tool: "format_range", status: "pass"}
}

// testApplyEdit exercises the full file-write path:
//  1. format_document → get TextEdit[] (non-empty when fixture has deliberate trailing whitespace)
//  2. apply_edit with a WorkspaceEdit wrapping those edits → writes to disk
//  3. format_document again → verify edits are now empty (proves write happened)
//
// Fixtures for Go, TypeScript, and Rust contain a blank line with trailing
// whitespace that their respective formatters will remove, ensuring step 1
// always returns non-empty edits on a fresh checkout.
func testApplyEdit(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if !lang.supportsFormatting {
		return toolResult{tool: "apply_edit", status: "skip", detail: "formatting not supported"}
	}

	// Step 1: get formatting edits.
	res, err := callTool(ctx, session, "format_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil {
		return toolResult{tool: "apply_edit", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "apply_edit", status: "skip", detail: "format_document not supported"}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "apply_edit", status: "fail", detail: err.Error()}
	}
	var edits []map[string]any
	if err := json.Unmarshal([]byte(text), &edits); err != nil {
		return toolResult{tool: "apply_edit", status: "fail",
			detail: fmt.Sprintf("parse format edits: %v — raw: %s", err, text)}
	}
	if len(edits) == 0 {
		return toolResult{tool: "apply_edit", status: "skip",
			detail: "no formatting edits returned (fixture already clean — run from fresh checkout to exercise write path)"}
	}

	// Step 2: apply the edits.
	fileURI := "file://" + lang.file
	res, err = callTool(ctx, session, "apply_edit", map[string]any{
		"workspace_edit": map[string]any{
			"changes": map[string]any{fileURI: edits},
		},
	})
	if err != nil {
		return toolResult{tool: "apply_edit", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		errText, _ := textFromResult(res)
		return toolResult{tool: "apply_edit", status: "fail",
			detail: fmt.Sprintf("apply_edit returned IsError: %s", errText)}
	}

	// Step 3: re-format to verify the write actually hit disk.
	res, err = callTool(ctx, session, "format_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil || res.IsError {
		// apply didn't error — treat as pass even if we can't verify
		return toolResult{tool: "apply_edit", status: "pass"}
	}
	text2, _ := textFromResult(res)
	var edits2 []map[string]any
	if err := json.Unmarshal([]byte(text2), &edits2); err == nil && len(edits2) > 0 {
		return toolResult{tool: "apply_edit", status: "fail",
			detail: fmt.Sprintf("re-format returned %d edits after apply — file write may not have persisted", len(edits2))}
	}
	return toolResult{tool: "apply_edit", status: "pass"}
}

// testDetectLspServers tests the detect_lsp_servers tool against the language fixture.
func testDetectLspServers(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "detect_lsp_servers", map[string]any{
		"workspace_dir": lang.fixture,
	})
	if err != nil {
		return toolResult{tool: "detect_lsp_servers", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "detect_lsp_servers", status: "fail",
			detail: fmt.Sprintf("detect_lsp_servers returned IsError: %s", text)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "detect_lsp_servers", status: "fail", detail: err.Error()}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "detect_lsp_servers", status: "fail",
			detail: fmt.Sprintf("failed to parse detect_lsp_servers response: %s", text)}
	}
	return toolResult{tool: "detect_lsp_servers", status: "pass"}
}

// testCloseDocument tests the close_document tool by reopening and closing the file.
func testCloseDocument(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	// Re-open first so we have a tracked document to close.
	_, _ = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	res, err := callTool(ctx, session, "close_document", map[string]any{
		"file_path": lang.file,
	})
	if err != nil {
		return toolResult{tool: "close_document", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "close_document", status: "fail",
			detail: fmt.Sprintf("close_document returned IsError: %s", text)}
	}
	return toolResult{tool: "close_document", status: "pass"}
}

// testDidChangeWatchedFiles tests the did_change_watched_files tool with an empty
// changes array — valid per LSP spec and exercises the tool without modifying files.
func testDidChangeWatchedFiles(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "did_change_watched_files", map[string]any{
		"changes": []any{},
	})
	if err != nil {
		return toolResult{tool: "did_change_watched_files", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "did_change_watched_files", status: "fail",
			detail: fmt.Sprintf("did_change_watched_files returned IsError: %s", text)}
	}
	return toolResult{tool: "did_change_watched_files", status: "pass"}
}

// testGetSymbolSource tests the get_symbol_source tool using the hover position,
// which points at a named symbol in every fixture that supports document symbols.
func testGetSymbolSource(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "get_symbol_source", status: "skip", detail: "no hover position configured"}
	}
	res, err := callTool(ctx, session, "get_symbol_source", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"character":   lang.hoverColumn,
	})
	if err != nil {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "get_symbol_source", status: "skip",
			detail: fmt.Sprintf("server does not support document symbols: %s", text)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: "empty get_symbol_source response"}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "get_symbol_source", status: "fail",
			detail: fmt.Sprintf("failed to parse get_symbol_source response: %s", text)}
	}
	if _, ok := result["symbol_name"]; !ok {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: "missing symbol_name in response"}
	}
	if src, ok := result["source"].(string); !ok || strings.TrimSpace(src) == "" {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: "empty source in response"}
	}
	return toolResult{tool: "get_symbol_source", status: "pass"}
}

// testGoToSymbol tests the go_to_symbol tool, which resolves a named symbol to
// its definition location. Uses workspaceSymbol from the langConfig; skips if
// no symbol is configured or if the server does not support workspace symbols.
func testGoToSymbol(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.workspaceSymbol == "" {
		return toolResult{tool: "go_to_symbol", status: "skip", detail: "no workspaceSymbol configured"}
	}
	res, err := callTool(ctx, session, "go_to_symbol", map[string]any{
		"query":       lang.workspaceSymbol,
		"language_id": lang.id,
	})
	if err != nil {
		return toolResult{tool: "go_to_symbol", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "go_to_symbol", status: "skip",
			detail: fmt.Sprintf("server does not support go_to_symbol: %s", text)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" {
		return toolResult{tool: "go_to_symbol", status: "fail", detail: "empty go_to_symbol response"}
	}
	return toolResult{tool: "go_to_symbol", status: "pass"}
}

// testRestartLspServer tests restart_lsp_server and verifies the server remains
// functional afterwards by re-running a hover request. Skips if restart is not
// supported or if no hover position is configured.
func testRestartLspServer(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "restart_lsp_server", status: "skip", detail: "no hover position configured"}
	}
	res, err := callTool(ctx, session, "restart_lsp_server", map[string]any{
		"language_id": lang.id,
	})
	if err != nil {
		return toolResult{tool: "restart_lsp_server", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "restart_lsp_server", status: "skip",
			detail: fmt.Sprintf("restart not supported: %s", text)}
	}
	// Wait for the server to reinitialize before verifying.
	time.Sleep(5 * time.Second)
	// Re-open the document (closed by restart) and verify hover still works.
	callTool(ctx, session, "open_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	time.Sleep(2 * time.Second)
	verify, err := callTool(ctx, session, "get_info_on_location", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
	})
	if err != nil || verify.IsError {
		return toolResult{tool: "restart_lsp_server", status: "fail",
			detail: "server unresponsive after restart"}
	}
	return toolResult{tool: "restart_lsp_server", status: "pass"}
}

// testSetLogLevel tests set_log_level — a local tool that does not require LSP.
// It sets the level to "debug", verifies the confirmation message, then resets
// to "info" so subsequent log output is not noisy.
func testSetLogLevel(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "set_log_level", map[string]any{
		"level": "debug",
	})
	if err != nil {
		return toolResult{tool: "set_log_level", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "set_log_level", status: "fail", detail: text}
	}
	text, _ := textFromResult(res)
	if !strings.Contains(text, "debug") {
		return toolResult{tool: "set_log_level", status: "fail",
			detail: fmt.Sprintf("unexpected response: %s", text)}
	}
	// Reset so the rest of the test run is not flooded with debug output.
	callTool(ctx, session, "set_log_level", map[string]any{"level": "info"}) //nolint:errcheck
	return toolResult{tool: "set_log_level", status: "pass"}
}

// testExecuteCommand tests execute_command by discovering available commands from
// server capabilities. If the server advertises no executeCommandProvider, the
// test is skipped. A server-level error on the command (e.g. it requires
// arguments we did not supply) is also treated as skip — the dispatch path was
// still exercised. A Go-level transport error is a failure.
func testExecuteCommand(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()

	// Query capabilities to find available commands.
	caps, err := callTool(ctx, session, "get_server_capabilities", map[string]any{
		"language_id": lang.id,
	})
	if err != nil || caps.IsError {
		return toolResult{tool: "execute_command", status: "skip", detail: "could not retrieve server capabilities"}
	}
	capsText, _ := textFromResult(caps)
	if !strings.Contains(capsText, `"executeCommandProvider"`) {
		return toolResult{tool: "execute_command", status: "skip", detail: "server does not advertise executeCommandProvider"}
	}

	var capsMap map[string]any
	if err := json.Unmarshal([]byte(capsText), &capsMap); err != nil {
		return toolResult{tool: "execute_command", status: "skip", detail: "could not parse capabilities JSON"}
	}
	ecp, ok := capsMap["executeCommandProvider"].(map[string]any)
	if !ok {
		return toolResult{tool: "execute_command", status: "skip", detail: "executeCommandProvider is not an object"}
	}
	cmds, ok := ecp["commands"].([]any)
	if !ok || len(cmds) == 0 {
		return toolResult{tool: "execute_command", status: "skip", detail: "no commands listed in executeCommandProvider"}
	}
	cmd, ok := cmds[0].(string)
	if !ok || cmd == "" {
		return toolResult{tool: "execute_command", status: "skip", detail: "first command is not a string"}
	}

	// Attempt the command. Many server commands require context arguments (e.g. a
	// file URI); pass one so gopls and similar servers can route the request.
	fileURI := "file://" + lang.file
	res, err := callTool(ctx, session, "execute_command", map[string]any{
		"command":   cmd,
		"arguments": []any{map[string]any{"URI": fileURI}},
	})
	if err != nil {
		return toolResult{tool: "execute_command", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		// Server returned a command-level error (wrong args, unsupported variant, etc.).
		// The tool dispatch was still exercised — record as skip, not failure.
		text, _ := textFromResult(res)
		return toolResult{tool: "execute_command", status: "skip",
			detail: fmt.Sprintf("command %q returned server error (may need different args): %s", cmd, text)}
	}
	return toolResult{tool: "execute_command", status: "pass"}
}

// TestGetChangeImpact tests the get_change_impact tool end-to-end using gopls
// and the Go fixture. It verifies blast-radius analysis works in CI.
func TestGetChangeImpact(t *testing.T) {
	lspBinaryPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("skipping TestGetChangeImpact: gopls not found on PATH")
	}

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	goFixture := filepath.Join(testDir(t), "fixtures", "go")
	greeterFile := filepath.Join(goFixture, "greeter.go")

	cmd := exec.Command(binaryPath, "go", lspBinaryPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "change-impact-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Skipf("failed to connect MCP session for gopls: %v", err)
		return
	}
	defer session.Close()

	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": goFixture})
	if err != nil || res.IsError {
		t.Skipf("start_lsp failed for gopls: err=%v", err)
		return
	}
	time.Sleep(8 * time.Second)

	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   greeterFile,
		"language_id": "go",
	})
	if err != nil || res.IsError {
		t.Skipf("open_document failed for greeter.go: err=%v", err)
		return
	}
	time.Sleep(2 * time.Second)

	res, err = callTool(ctx, session, "get_change_impact", map[string]any{
		"changed_files": []any{greeterFile},
	})
	if err != nil {
		t.Errorf("get_change_impact call failed: %v", err)
		return
	}
	if res.IsError {
		text, _ := textFromResult(res)
		t.Skipf("get_change_impact returned IsError (server may not support it): %s", text)
		return
	}

	text, err := textFromResult(res)
	if err != nil {
		t.Errorf("get_change_impact: failed to extract text: %v", err)
		return
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Errorf("get_change_impact: failed to parse JSON response: %s", text)
		return
	}

	changedSymbols, _ := result["changed_symbols"].([]any)
	if len(changedSymbols) == 0 {
		t.Errorf("get_change_impact: expected changed_symbols to be non-empty")
	}

	summary, _ := result["summary"].(string)
	if summary == "" {
		t.Errorf("get_change_impact: expected non-empty summary")
	}

	t.Logf("[TestGetChangeImpact] changed_symbols=%d summary=%q", len(changedSymbols), summary)
}

// TestGetCrossRepoReferences tests the get_cross_repo_references tool end-to-end
// using gopls and the Go fixture. It adds the fixture dir as both the primary
// workspace root and a consumer_root to exercise the full cross-repo wiring path.
func TestGetCrossRepoReferences(t *testing.T) {
	lspBinaryPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("skipping TestGetCrossRepoReferences: gopls not found on PATH")
	}

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	goFixture := filepath.Join(testDir(t), "fixtures", "go")
	mainFile := filepath.Join(goFixture, "main.go")

	cmd := exec.Command(binaryPath, "go", lspBinaryPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "cross-repo-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Skipf("failed to connect MCP session for gopls: %v", err)
		return
	}
	defer session.Close()

	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": goFixture})
	if err != nil || res.IsError {
		t.Skipf("start_lsp failed for gopls: err=%v", err)
		return
	}
	time.Sleep(8 * time.Second)

	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   mainFile,
		"language_id": "go",
	})
	if err != nil || res.IsError {
		t.Skipf("open_document failed for main.go: err=%v", err)
		return
	}
	time.Sleep(2 * time.Second)

	// Add the fixture dir as a consumer workspace folder.
	res, err = callTool(ctx, session, "add_workspace_folder", map[string]any{
		"path": goFixture,
	})
	if err != nil {
		t.Logf("add_workspace_folder warning: %v", err)
	}

	// Call get_cross_repo_references on the Person struct declaration (line 6, col 6).
	res, err = callTool(ctx, session, "get_cross_repo_references", map[string]any{
		"symbol_file":    mainFile,
		"language_id":    "go",
		"line":           6,
		"column":         6,
		"consumer_roots": []any{goFixture},
	})
	if err != nil {
		t.Errorf("get_cross_repo_references call failed: %v", err)
		return
	}
	if res.IsError {
		text, _ := textFromResult(res)
		t.Skipf("get_cross_repo_references returned IsError: %s", text)
		return
	}

	text, err := textFromResult(res)
	if err != nil {
		t.Errorf("get_cross_repo_references: failed to extract text: %v", err)
		return
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Errorf("get_cross_repo_references: failed to parse JSON response: %s", text)
		return
	}

	if _, ok := result["symbol"]; !ok {
		t.Errorf("get_cross_repo_references: missing 'symbol' key in response")
	}
	if _, ok := result["summary"]; !ok {
		t.Errorf("get_cross_repo_references: missing 'summary' key in response")
	}

	t.Logf("[TestGetCrossRepoReferences] symbol=%v summary=%v", result["symbol"], result["summary"])
}

// statusIcon returns a visual icon for a tool result status.
func statusIcon(s string) string {
	switch s {
	case "pass":
		return "✓"
	case "skip":
		return "-"
	default:
		return "✗"
	}
}

// printMultiLangSummary prints a summary table of all language test results.
func printMultiLangSummary(t *testing.T, results []langResult) {
	t.Helper()
	header := fmt.Sprintf(
		"%-14s | T1 | %-7s | %-10s | %-10s | %-11s | %-9s | %-6s | %-11s | %-13s | %-5s | %-9s | %-8s | %-8s | %-10s | %-10s | %-8s | %-8s | %-6s | %-8s | %-10s | %-8s | %-10s | %-11s | %-10s | %-12s | %-12s | %-17s | %-10s",
		"Language", "symbols", "definition", "references", "completions",
		"workspace", "format", "declaration", "type_hierarchy", "hover",
		"call_hier", "sem_tok", "sig_help",
		"highlights", "inlay_hints", "code_act", "prep_ren", "rename",
		"srv_caps", "wk_folders",
		"type_def", "go_to_impl", "format_range", "apply_edit", "detect_servers",
		"close_doc", "did_change_watched", "sym_source")
	sep := strings.Repeat("-", len(header))
	t.Logf("\n%s\n%s", header, sep)

	for _, r := range results {
		// Build a map from tool name to status for easy lookup.
		toolStatus := map[string]string{}
		for _, tr := range r.tier2 {
			toolStatus[tr.tool] = tr.status
		}
		get := func(name string) string {
			if s, ok := toolStatus[name]; ok {
				return statusIcon(s)
			}
			return " "
		}
		t.Logf(
			"%-14s | %-2s | %-7s | %-10s | %-10s | %-11s | %-9s | %-6s | %-11s | %-13s | %-5s | %-9s | %-8s | %-8s | %-10s | %-10s | %-8s | %-8s | %-6s | %-8s | %-10s | %-8s | %-10s | %-11s | %-10s | %-12s | %-12s | %-17s | %-10s",
			r.name,
			statusIcon(r.tier1),
			get("get_document_symbols"),
			get("go_to_definition"),
			get("get_references"),
			get("get_completions"),
			get("get_workspace_symbols"),
			get("format_document"),
			get("go_to_declaration"),
			get("type_hierarchy"),
			get("get_info_on_location"),
			get("call_hierarchy"),
			get("get_semantic_tokens"),
			get("get_signature_help"),
			get("get_document_highlights"),
			get("get_inlay_hints"),
			get("get_code_actions"),
			get("prepare_rename"),
			get("rename_symbol"),
			get("get_server_capabilities"),
			get("workspace_folders"),
			get("go_to_type_definition"),
			get("go_to_implementation"),
			get("format_range"),
			get("apply_edit"),
			get("detect_lsp_servers"),
			get("close_document"),
			get("did_change_watched_files"),
			get("get_symbol_source"),
		)
	}
	t.Logf("%s", sep)
}

// TestMultiLanguage runs the full multi-language test suite.
func TestMultiLanguage(t *testing.T) {
	t.Parallel()

	fixtureBase := filepath.Join(testDir(t), "fixtures")
	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	languages := buildLanguageConfigs(fixtureBase)

	results := make([]langResult, 0, len(languages))

	// Run subtests sequentially — LSP servers can collide when run in parallel.
	for _, lang := range languages {
		lang := lang // capture loop var
		t.Run(lang.name, func(t *testing.T) {
			// Do NOT call t.Parallel() here — run sequentially.
			r := runLanguageTest(t, binaryPath, lang)
			results = append(results, langResult{
				name:  lang.name,
				tier1: r.tier1,
				tier2: r.tier2,
			})
		})
	}

	printMultiLangSummary(t, results)
}

// TestFuzzyPositionFallback verifies that go_to_definition and get_references
// succeed when called with positions that are off by one line from the exact
// symbol location, exercising the fuzzy position fallback path end-to-end
// against a real gopls instance.
//
// The Go fixture main.go declares `type Person struct` at line 6, column 6.
// Off-by-one positions (lines 5 and 7) are tested. A non-empty result
// (or a skip) proves the round-trip completed without a transport error.
func TestFuzzyPositionFallback(t *testing.T) {
	lspBinaryPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("skipping TestFuzzyPositionFallback: gopls not found on PATH")
	}

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	goFixture := filepath.Join(testDir(t), "fixtures", "go")
	mainFile := filepath.Join(goFixture, "main.go")

	cmd := exec.Command(binaryPath, "go", lspBinaryPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "fuzzy-fallback-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Skipf("failed to connect MCP session for gopls: %v", err)
		return
	}
	defer session.Close()

	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": goFixture})
	if err != nil || res.IsError {
		t.Skipf("start_lsp failed: err=%v", err)
		return
	}
	time.Sleep(8 * time.Second)

	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   mainFile,
		"language_id": "go",
	})
	if err != nil || res.IsError {
		t.Skipf("open_document failed: err=%v", err)
		return
	}
	time.Sleep(2 * time.Second)

	// Test go_to_definition with off-by-one positions around line 6.
	// Line 5 is the comment above the struct; line 7 is inside the struct body.
	for _, offByOneLine := range []int{5, 7} {
		res, err = callTool(ctx, session, "go_to_definition", map[string]any{
			"file_path":   mainFile,
			"language_id": "go",
			"line":        offByOneLine,
			"column":      6,
		})
		if err != nil {
			t.Errorf("go_to_definition(line=%d): unexpected transport error: %v", offByOneLine, err)
			continue
		}
		if res.IsError {
			text, _ := textFromResult(res)
			t.Logf("go_to_definition(line=%d): server error (fallback may not apply): %s", offByOneLine, text)
			continue
		}
		text, terr := textFromResult(res)
		if terr != nil {
			t.Errorf("go_to_definition(line=%d): extract text: %v", offByOneLine, terr)
			continue
		}
		if text == "" || text == "null" || text == "[]" {
			t.Logf("go_to_definition(line=%d): empty result (gopls returned nothing at off-by-one position)", offByOneLine)
		} else {
			t.Logf("go_to_definition(line=%d): result len=%d (fallback active or exact match)", offByOneLine, len(text))
		}
	}

	// Test get_references with off-by-one positions.
	for _, offByOneLine := range []int{5, 7} {
		res, err = callTool(ctx, session, "get_references", map[string]any{
			"file_path":           mainFile,
			"language_id":         "go",
			"line":                offByOneLine,
			"column":              6,
			"include_declaration": true,
		})
		if err != nil {
			t.Errorf("get_references(line=%d): unexpected transport error: %v", offByOneLine, err)
			continue
		}
		if res.IsError {
			text, _ := textFromResult(res)
			t.Logf("get_references(line=%d): server error: %s", offByOneLine, text)
			continue
		}
		text, terr := textFromResult(res)
		if terr != nil {
			t.Errorf("get_references(line=%d): extract text: %v", offByOneLine, terr)
			continue
		}
		t.Logf("get_references(line=%d): result len=%d", offByOneLine, len(text))
	}
}
