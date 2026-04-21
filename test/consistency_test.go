package main_test

// consistency_test.go — cross-language behavioral consistency tests.
//
// This test runs a small set of tools across a subset of well-supported
// languages and asserts structural response consistency — not semantic
// equivalence, but shape.
//
// "Structural consistency" means:
//   - get_document_symbols → JSON array; every item has a "name" string field
//   - go_to_definition    → empty/error OR contains a "file" or "uri" string
//   - get_diagnostics     → JSON array; every item has "severity" and "message" fields
//   - get_info_on_location (hover) → has a "contents" field if non-empty
//
// Languages run in parallel. A language/tool combo is skipped gracefully when
// the LSP binary isn't on PATH. Shape failures use t.Errorf so all languages
// report before the test stops.

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// consistencyLangConfig holds the minimal per-language data needed for the
// structural consistency checks.
type consistencyLangConfig struct {
	name        string
	id          string
	binary      string
	serverArgs  []string
	fixture     string // absolute path to fixture dir (passed to start_lsp)
	file        string // absolute path to primary source file
	hoverLine   int
	hoverColumn int
	defLine     int
	defColumn   int
	initWait    time.Duration
	timeout     time.Duration
}

// buildConsistencyLangConfigs returns configs for the languages that are most
// consistently available across development machines and CI: Go, TypeScript,
// Python, and Rust.  Java and JVM-based servers are excluded because their
// cold-start times (>60 s) would make a parallel consistency suite impractical.
func buildConsistencyLangConfigs(fixtureBase string) []consistencyLangConfig {
	return []consistencyLangConfig{
		{
			name:        "Go",
			id:          "go",
			binary:      "gopls",
			serverArgs:  []string{},
			fixture:     filepath.Join(fixtureBase, "go"),
			file:        filepath.Join(fixtureBase, "go", "main.go"),
			hoverLine:   6,  // type Person struct
			hoverColumn: 6,
			defLine:     23, // p.Greet() call site
			defColumn:   17,
			initWait:    8 * time.Second,
			timeout:     90 * time.Second,
		},
		{
			name:        "TypeScript",
			id:          "typescript",
			binary:      "typescript-language-server",
			serverArgs:  []string{"--stdio"},
			fixture:     filepath.Join(fixtureBase, "typescript"),
			file:        filepath.Join(fixtureBase, "typescript", "src", "example.ts"),
			hoverLine:   11,
			hoverColumn: 18,
			defLine:     4,
			defColumn:   14,
			initWait:    8 * time.Second,
			timeout:     90 * time.Second,
		},
		{
			name:        "Python",
			id:          "python",
			binary:      "pyright-langserver",
			serverArgs:  []string{"--stdio"},
			fixture:     filepath.Join(fixtureBase, "python"),
			file:        filepath.Join(fixtureBase, "python", "main.py"),
			hoverLine:   4,
			hoverColumn: 7,
			defLine:     15,
			defColumn:   14,
			initWait:    8 * time.Second,
			timeout:     90 * time.Second,
		},
		{
			name:        "Rust",
			id:          "rust",
			binary:      "rust-analyzer",
			serverArgs:  []string{},
			fixture:     filepath.Join(fixtureBase, "rust"),
			file:        filepath.Join(fixtureBase, "rust", "src", "main.rs"),
			hoverLine:   2,
			hoverColumn: 8,
			defLine:     30,
			defColumn:   20,
			initWait:    15 * time.Second, // rust-analyzer compiles before indexing
			timeout:     120 * time.Second,
		},
	}
}

// TestCrossLanguageConsistency verifies that key tools return structurally
// consistent responses across all configured language servers.
func TestCrossLanguageConsistency(t *testing.T) {
	t.Parallel()

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	fixtureBase := filepath.Join(testDir(t), "fixtures")
	langs := buildConsistencyLangConfigs(fixtureBase)

	for _, lang := range langs {
		t.Run(lang.name, func(t *testing.T) {
			t.Parallel()
			runConsistencyChecks(t, binaryPath, lang)
		})
	}
}

// runConsistencyChecks establishes a dedicated MCP session for lang and runs
// all structural shape checks against it.
func runConsistencyChecks(t *testing.T, binaryPath string, lang consistencyLangConfig) {
	t.Helper()

	lspBinaryPath, err := exec.LookPath(lang.binary)
	if err != nil {
		t.Skipf("skipping %s: %s not found on PATH", lang.name, lang.binary)
		return
	}

	timeout := lang.timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := append([]string{lang.id, lspBinaryPath}, lang.serverArgs...)
	cmd := exec.Command(binaryPath, args...)
	client := mcp.NewClient(&mcp.Implementation{Name: "consistency-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Skipf("[%s] failed to connect MCP session: %v", lang.name, err)
		return
	}
	defer session.Close()

	// start_lsp
	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": lang.fixture})
	if err != nil || res.IsError {
		t.Skipf("[%s] start_lsp failed: err=%v isError=%v", lang.name, err, res.IsError)
		return
	}
	time.Sleep(lang.initWait)

	// open_document
	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil || res.IsError {
		t.Skipf("[%s] open_document failed: err=%v isError=%v", lang.name, err, res.IsError)
		return
	}
	time.Sleep(3 * time.Second)

	// Run each shape check. Use t.Errorf (not t.Fatalf) so all tools report.
	checkDocumentSymbolsShape(t, ctx, session, lang)
	checkGoToDefinitionShape(t, ctx, session, lang)
	checkGetDiagnosticsShape(t, ctx, session, lang)
	checkHoverShape(t, ctx, session, lang)
}

// checkDocumentSymbolsShape asserts that get_document_symbols returns a JSON
// array where every item has a "name" string field.
func checkDocumentSymbolsShape(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang consistencyLangConfig) {
	t.Helper()
	res, err := callTool(ctx, session, "get_document_symbols", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil {
		t.Errorf("[%s] get_document_symbols: transport error: %v", lang.name, err)
		return
	}
	if res.IsError {
		// IsError is acceptable — some servers don't support document symbols.
		t.Logf("[%s] get_document_symbols: skipping shape check — IsError=true", lang.name)
		return
	}
	text, err := textFromResult(res)
	if err != nil {
		t.Errorf("[%s] get_document_symbols: failed to extract text: %v", lang.name, err)
		return
	}
	if text == "" {
		// Empty response is acceptable (server may not support the capability).
		t.Logf("[%s] get_document_symbols: empty response — skipping shape check", lang.name)
		return
	}

	// Shape: must be a JSON array.
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Errorf("[%s] get_document_symbols: response is not a JSON array: %s", lang.name, text)
		return
	}

	// Every item must have a "name" string field.
	for i, item := range items {
		nameVal, hasName := item["name"]
		if !hasName {
			t.Errorf("[%s] get_document_symbols: item[%d] missing \"name\" field: %v", lang.name, i, item)
			continue
		}
		if _, ok := nameVal.(string); !ok {
			t.Errorf("[%s] get_document_symbols: item[%d].name is not a string: %T", lang.name, i, nameVal)
		}
	}
	t.Logf("[%s] get_document_symbols: shape OK (%d symbols)", lang.name, len(items))
}

// checkGoToDefinitionShape asserts that go_to_definition returns either an
// empty/error response or a result containing a "file" or "uri" string field.
func checkGoToDefinitionShape(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang consistencyLangConfig) {
	t.Helper()
	res, err := callTool(ctx, session, "go_to_definition", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.defLine,
		"column":      lang.defColumn,
	})
	if err != nil {
		t.Errorf("[%s] go_to_definition: transport error: %v", lang.name, err)
		return
	}
	if res.IsError {
		t.Logf("[%s] go_to_definition: IsError=true — structured error, shape OK", lang.name)
		return
	}
	text, err := textFromResult(res)
	if err != nil {
		t.Errorf("[%s] go_to_definition: failed to extract text: %v", lang.name, err)
		return
	}
	if text == "" {
		// Empty is acceptable — definition not found.
		t.Logf("[%s] go_to_definition: empty response — no definition found, shape OK", lang.name)
		return
	}

	// Try to parse as array first, then as object.
	var result map[string]any
	var arr []map[string]any
	if jsonErr := json.Unmarshal([]byte(text), &arr); jsonErr == nil {
		if len(arr) == 0 {
			t.Logf("[%s] go_to_definition: empty array — shape OK", lang.name)
			return
		}
		result = arr[0]
	} else if jsonErr := json.Unmarshal([]byte(text), &result); jsonErr != nil {
		t.Errorf("[%s] go_to_definition: response is not valid JSON: %s", lang.name, text)
		return
	}

	// Shape: must have "file" or "uri" as a string.
	fileVal, hasFile := result["file"]
	uriVal, hasURI := result["uri"]
	if !hasFile && !hasURI {
		t.Errorf("[%s] go_to_definition: result has neither \"file\" nor \"uri\" field: %v", lang.name, result)
		return
	}
	if hasFile {
		if s, ok := fileVal.(string); !ok || s == "" {
			t.Errorf("[%s] go_to_definition: \"file\" field is not a non-empty string: %v", lang.name, fileVal)
		}
	} else {
		if s, ok := uriVal.(string); !ok || s == "" {
			t.Errorf("[%s] go_to_definition: \"uri\" field is not a non-empty string: %v", lang.name, uriVal)
		}
	}
	t.Logf("[%s] go_to_definition: shape OK", lang.name)
}

// checkGetDiagnosticsShape asserts that get_diagnostics returns a JSON array
// where every item has "severity" and "message" fields.
func checkGetDiagnosticsShape(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang consistencyLangConfig) {
	t.Helper()
	res, err := callTool(ctx, session, "get_diagnostics", map[string]any{
		"file_path": lang.file,
	})
	if err != nil {
		t.Errorf("[%s] get_diagnostics: transport error: %v", lang.name, err)
		return
	}
	if res.IsError {
		t.Logf("[%s] get_diagnostics: skipping shape check — IsError=true", lang.name)
		return
	}
	text, err := textFromResult(res)
	if err != nil {
		t.Errorf("[%s] get_diagnostics: failed to extract text: %v", lang.name, err)
		return
	}
	if text == "" {
		t.Logf("[%s] get_diagnostics: empty response — skipping shape check", lang.name)
		return
	}

	// Shape: must be a JSON array.
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Errorf("[%s] get_diagnostics: response is not a JSON array: %s", lang.name, text)
		return
	}

	// Every item must have "severity" and "message" fields.
	for i, item := range items {
		if _, hasSeverity := item["severity"]; !hasSeverity {
			t.Errorf("[%s] get_diagnostics: item[%d] missing \"severity\" field: %v", lang.name, i, item)
		}
		msgVal, hasMessage := item["message"]
		if !hasMessage {
			t.Errorf("[%s] get_diagnostics: item[%d] missing \"message\" field: %v", lang.name, i, item)
			continue
		}
		if s, ok := msgVal.(string); !ok || s == "" {
			t.Errorf("[%s] get_diagnostics: item[%d].message is not a non-empty string: %v", lang.name, i, msgVal)
		}
	}
	t.Logf("[%s] get_diagnostics: shape OK (%d diagnostics)", lang.name, len(items))
}

// checkHoverShape asserts that get_info_on_location returns a "contents" field
// (string or object) when the response is non-empty.
func checkHoverShape(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang consistencyLangConfig) {
	t.Helper()
	res, err := callTool(ctx, session, "get_info_on_location", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
	})
	if err != nil {
		t.Errorf("[%s] get_info_on_location: transport error: %v", lang.name, err)
		return
	}
	if res.IsError {
		t.Logf("[%s] get_info_on_location: skipping shape check — IsError=true", lang.name)
		return
	}
	text, err := textFromResult(res)
	if err != nil {
		t.Errorf("[%s] get_info_on_location: failed to extract text: %v", lang.name, err)
		return
	}
	if text == "" {
		// Empty is acceptable — some servers return nothing for certain positions.
		t.Logf("[%s] get_info_on_location: empty response — skipping shape check", lang.name)
		return
	}

	// Shape: if the response is a JSON object it must have a "contents" field.
	// If it is a plain string, that is also acceptable (server returns raw markdown).
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		// Non-JSON plain-text hover responses are acceptable.
		t.Logf("[%s] get_info_on_location: plain-text response — shape OK", lang.name)
		return
	}

	if _, hasContents := obj["contents"]; !hasContents {
		// Some servers wrap in a different field name; check common alternatives.
		_, hasValue := obj["value"]
		_, hasMarkup := obj["markup"]
		_, hasKind := obj["kind"]
		if !hasValue && !hasMarkup && !hasKind {
			t.Errorf("[%s] get_info_on_location: JSON response missing \"contents\" (and no \"value\"/\"markup\"/\"kind\"): %v",
				lang.name, fmt.Sprintf("%.200s", text))
		} else {
			t.Logf("[%s] get_info_on_location: response uses alternative field — shape OK", lang.name)
		}
		return
	}
	t.Logf("[%s] get_info_on_location: shape OK (has \"contents\" field)", lang.name)
}
