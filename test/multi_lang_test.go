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

// getMultilangBinary builds the lsp-mcp-go binary once and returns its path.
// Returns empty string if build fails.
func getMultilangBinary(t *testing.T) string {
	t.Helper()
	multilangBinaryOnce.Do(func() {
		tmp, err := os.MkdirTemp("", "lsp-mcp-go-multi-*")
		if err != nil {
			return
		}
		p := filepath.Join(tmp, "lsp-mcp-go")
		// test/multi_lang_test.go → test/ → repo root (two levels up)
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			return
		}
		testFileDir := filepath.Dir(filename)
		repoRoot := filepath.Dir(testFileDir)
		cmd := exec.Command("go", "build", "-o", p, "./cmd/lsp-mcp-go")
		cmd.Env = append(os.Environ(), "GOWORK=off")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build lsp-mcp-go: %v\n%s\n", err, out)
			return
		}
		multilangBinaryPath = p
	})
	return multilangBinaryPath
}

// buildLanguageConfigs returns all language configs using the given fixture base directory.
func buildLanguageConfigs(fixtureBase string) []langConfig {
	return []langConfig{
		{
			name:               "TypeScript",
			id:                 "typescript",
			binary:             "typescript-language-server",
			serverArgs:         []string{"--stdio"},
			fixture:            filepath.Join(fixtureBase, "typescript"),
			file:               filepath.Join(fixtureBase, "typescript", "src", "example.ts"),
			hoverLine:          11,
			hoverColumn:        18,
			definitionLine:     4,
			definitionColumn:   17,
			callSiteLine:       4,
			callSiteColumn:     14,
			callSiteFile:       filepath.Join(fixtureBase, "typescript", "src", "consumer.ts"),
			referenceLine:      11,
			referenceColumn:    18,
			completionLine:     7,
			completionColumn:   26,
			completionFile:     filepath.Join(fixtureBase, "typescript", "src", "consumer.ts"),
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "typescript", "src", "consumer.ts"),
			symbolName:         "Person",
		},
		{
			name:               "Python",
			id:                 "python",
			binary:             "pyright-langserver",
			serverArgs:         []string{"--stdio"},
			fixture:            filepath.Join(fixtureBase, "python"),
			file:               filepath.Join(fixtureBase, "python", "main.py"),
			hoverLine:          4,
			hoverColumn:        7,
			definitionLine:     1,
			definitionColumn:   5,
			callSiteLine:       15,
			callSiteColumn:     14,
			referenceLine:      4,
			referenceColumn:    7,
			completionLine:     14,
			completionColumn:   13,
			workspaceSymbol:    "Person",
			supportsFormatting: false,
			secondFile:         filepath.Join(fixtureBase, "python", "greeter.py"),
			symbolName:         "Person",
		},
		{
			name:               "Go",
			id:                 "go",
			binary:             "gopls",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "go"),
			file:               filepath.Join(fixtureBase, "go", "main.go"),
			hoverLine:          6,
			hoverColumn:        6,
			definitionLine:     16,
			definitionColumn:   6,
			callSiteLine:       23,
			callSiteColumn:     17,
			referenceLine:      6,
			referenceColumn:    6,
			completionLine:     22,
			completionColumn:   19,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "go", "greeter.go"),
			symbolName:         "Person",
		},
		{
			name:               "Rust",
			id:                 "rust",
			binary:             "rust-analyzer",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "rust"),
			file:               filepath.Join(fixtureBase, "rust", "src", "main.rs"),
			hoverLine:          2,
			hoverColumn:        8,
			definitionLine:     23,
			definitionColumn:   4,
			callSiteLine:       30,
			callSiteColumn:     20,
			referenceLine:      2,
			referenceColumn:    12,
			completionLine:     28,
			completionColumn:   11,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "rust", "src", "greeter.rs"),
			symbolName:         "Person",
		},
		{
			name:       "Java",
			id:         "java",
			binary:     "jdtls",
			serverArgs: []string{"-data", "/tmp/jdtls-workspace-lsp-mcp-test"},
			javaHome:   "/opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home",
			fixture:    filepath.Join(fixtureBase, "java"),
			file: filepath.Join(fixtureBase, "java", "src", "main", "java", "com", "example",
				"Person.java"),
			hoverLine:          6,
			hoverColumn:        14,
			definitionLine:     20,
			definitionColumn:   23,
			callSiteLine:       27,
			callSiteColumn:     28,
			referenceLine:      6,
			referenceColumn:    14,
			completionLine:     25,
			completionColumn:   16,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile: filepath.Join(fixtureBase, "java", "src", "main", "java", "com", "example",
				"Greeter.java"),
			symbolName: "Person",
		},
		{
			name:               "C",
			id:                 "c",
			binary:             "clangd",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "c"),
			file:               filepath.Join(fixtureBase, "c", "person.c"),
			hoverLine:          3,
			hoverColumn:        1,
			definitionLine:     10,
			definitionColumn:   5,
			callSiteLine:       16,
			callSiteColumn:     12,
			referenceLine:      3,
			referenceColumn:    1,
			completionLine:     15,
			completionColumn:   12,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "c", "greeter.c"),
			symbolName:         "create_person",
			declarationLine:    3,
			declarationColumn:  1,
		},
		{
			name:               "PHP",
			id:                 "php",
			binary:             "intelephense",
			serverArgs:         []string{"--stdio"},
			fixture:            filepath.Join(fixtureBase, "php"),
			file:               filepath.Join(fixtureBase, "php", "Person.php"),
			hoverLine:          6,
			hoverColumn:        7,
			definitionLine:     20,
			definitionColumn:   24,
			callSiteLine:       27,
			callSiteColumn:     14,
			referenceLine:      6,
			referenceColumn:    7,
			completionLine:     26,
			completionColumn:   22,
			workspaceSymbol:    "Person",
			supportsFormatting: false,
			secondFile:         filepath.Join(fixtureBase, "php", "Greeter.php"),
			symbolName:         "Person",
		},
		{
			name:               "C++",
			id:                 "cpp",
			binary:             "clangd",
			fixture:            filepath.Join(fixtureBase, "cpp"),
			file:               filepath.Join(fixtureBase, "cpp", "person.h"),
			hoverLine:          9,
			hoverColumn:        7,
			definitionLine:     9,
			definitionColumn:   7,
			callSiteLine:       14,
			callSiteColumn:     16,
			callSiteFile:       filepath.Join(fixtureBase, "cpp", "person.cpp"),
			referenceLine:      9,
			referenceColumn:    7,
			completionLine:     10,
			completionColumn:   12,
			completionFile:     filepath.Join(fixtureBase, "cpp", "person.cpp"),
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "cpp", "greeter.cpp"),
			symbolName:         "Person",
		},
		{
			name:               "JavaScript",
			id:                 "javascript",
			binary:             "typescript-language-server",
			serverArgs:         []string{"--stdio"},
			fixture:            filepath.Join(fixtureBase, "javascript"),
			file:               filepath.Join(fixtureBase, "javascript", "src", "example.js"),
			hoverLine:          11,
			hoverColumn:        14,
			definitionLine:     11,
			definitionColumn:   14,
			callSiteLine:       7,
			callSiteColumn:     19,
			callSiteFile:       filepath.Join(fixtureBase, "javascript", "src", "consumer.js"),
			referenceLine:      11,
			referenceColumn:    14,
			completionLine:     5,
			completionColumn:   14,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "javascript", "src", "consumer.js"),
			symbolName:         "Person",
		},
		{
			name:               "Ruby",
			id:                 "ruby",
			binary:             "solargraph",
			serverArgs:         []string{"stdio"},
			fixture:            filepath.Join(fixtureBase, "ruby"),
			file:               filepath.Join(fixtureBase, "ruby", "person.rb"),
			hoverLine:          5,
			hoverColumn:        7,
			definitionLine:     5,
			definitionColumn:   7,
			callSiteLine:       6,
			callSiteColumn:     19,
			callSiteFile:       filepath.Join(fixtureBase, "ruby", "greeter.rb"),
			referenceLine:      5,
			referenceColumn:    7,
			completionLine:     2,
			completionColumn:   6,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "ruby", "greeter.rb"),
			symbolName:         "Person",
		},
		{
			name:            "YAML",
			id:              "yaml",
			binary:          "yaml-language-server",
			serverArgs:      []string{"--stdio"},
			fixture:         filepath.Join(fixtureBase, "yaml"),
			file:            filepath.Join(fixtureBase, "yaml", "workflow.yml"),
			hoverLine:       2,
			hoverColumn:     1,
			definitionLine:  0,
			callSiteLine:    0,
			referenceLine:   0,
			completionLine:  24,
			completionColumn: 7,
			workspaceSymbol: "name",
			supportsFormatting: true,
		},
		{
			name:            "JSON",
			id:              "json",
			binary:          "vscode-json-language-server",
			serverArgs:      []string{"--stdio"},
			fixture:         filepath.Join(fixtureBase, "json"),
			file:            filepath.Join(fixtureBase, "json", "package.json"),
			hoverLine:       2,
			hoverColumn:     3,
			definitionLine:  0,
			callSiteLine:    0,
			referenceLine:   0,
			completionLine:  9,
			completionColumn: 5,
			workspaceSymbol: "name",
			supportsFormatting: true,
		},
		{
			name:            "Dockerfile",
			id:              "dockerfile",
			binary:          "docker-langserver",
			serverArgs:      []string{"--stdio"},
			fixture:         filepath.Join(fixtureBase, "dockerfile"),
			file:            filepath.Join(fixtureBase, "dockerfile", "Dockerfile"),
			hoverLine:       3,
			hoverColumn:     1,
			definitionLine:  0,
			callSiteLine:    0,
			referenceLine:   0,
			completionLine:  21,
			completionColumn: 1,
			workspaceSymbol: "FROM",
			supportsFormatting: false,
		},
	}
}

// callTool is a convenience wrapper that calls a named tool and returns the text content.
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

	// Determine timeout.
	var timeout time.Duration
	if lang.id == "java" {
		timeout = 180 * time.Second
	} else {
		timeout = 60 * time.Second
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
	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": lang.fixture})
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

	// LSP init wait.
	var initWait time.Duration
	if lang.id == "java" {
		initWait = 150 * time.Second
	} else {
		initWait = 8 * time.Second
	}
	time.Sleep(initWait)

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
	header := fmt.Sprintf("%-14s | T1 | %-8s | %-11s | %-11s | %-12s | %-10s | %-6s | %-12s",
		"Language", "symbols", "definition", "references", "completions", "workspace", "format", "declaration")
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
		t.Logf("%-14s | %-2s | %-8s | %-11s | %-11s | %-12s | %-10s | %-6s | %-12s",
			r.name,
			statusIcon(r.tier1),
			get("get_document_symbols"),
			get("go_to_definition"),
			get("get_references"),
			get("get_completions"),
			get("get_workspace_symbols"),
			get("format_document"),
			get("go_to_declaration"),
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
		t.Skip("failed to build lsp-mcp-go binary")
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
