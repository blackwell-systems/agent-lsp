package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testGetSymbolDocumentation calls the get_symbol_documentation tool and returns
// the parsed DocResult fields as a map. Fails the test if the call or parsing fails.
func testGetSymbolDocumentation(
	t *testing.T,
	ctx context.Context,
	session *mcp.ClientSession,
	symbol, languageID string,
) map[string]any {
	t.Helper()
	res, err := callTool(ctx, session, "get_symbol_documentation", map[string]any{
		"symbol":      symbol,
		"language_id": languageID,
	})
	if err != nil {
		t.Fatalf("get_symbol_documentation(%q, %q): unexpected error: %v", symbol, languageID, err)
	}
	text, err := textFromResult(res)
	if err != nil {
		t.Fatalf("get_symbol_documentation(%q, %q): failed to extract text: %v", symbol, languageID, err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("get_symbol_documentation(%q, %q): failed to parse JSON response: %s",
			symbol, languageID,
			fmt.Sprintf("parse error: %v — raw: %s", err, text))
	}
	return result
}

// TestGetSymbolDocumentation is a standalone integration test for the
// get_symbol_documentation MCP tool. It starts agent-lsp in auto-detect mode
// (no LSP server required) and exercises the Go toolchain path, the unsupported-
// language error path, and the missing-required-field error path.
func TestGetSymbolDocumentation(t *testing.T) {
	t.Parallel()

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start agent-lsp in auto-detect mode (no LSP server args needed for
	// get_symbol_documentation, which runs shell commands independently of
	// any LSP session).
	cmd := exec.CommandContext(ctx, binaryPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "documentation-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("failed to connect MCP session: %v", err)
	}
	defer session.Close()

	t.Run("go_toolchain_path", func(t *testing.T) {
		// fmt.Println is always available via go doc in CI.
		res, err := callTool(ctx, session, "get_symbol_documentation", map[string]any{
			"symbol":      "fmt.Println",
			"language_id": "go",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			text, _ := textFromResult(res)
			t.Fatalf("expected IsError=false, got IsError=true: %s", text)
		}

		text, err := textFromResult(res)
		if err != nil {
			t.Fatalf("failed to extract text: %v", err)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Fatalf("failed to parse JSON response: %s", text)
		}

		// source must be "toolchain"
		if source, _ := result["source"].(string); source != "toolchain" {
			t.Errorf("expected source=%q, got %q (full response: %s)", "toolchain", source, text)
		}
		// symbol field
		if symbol, _ := result["symbol"].(string); symbol != "fmt.Println" {
			t.Errorf("expected symbol=%q, got %q", "fmt.Println", symbol)
		}
		// language field
		if language, _ := result["language"].(string); language != "go" {
			t.Errorf("expected language=%q, got %q", "go", language)
		}
		// doc field: non-empty and contains "Println"
		doc, _ := result["doc"].(string)
		if doc == "" {
			t.Errorf("expected non-empty doc field, got empty string")
		}
		if doc != "" && !strings.Contains(doc, "Println") {
			t.Errorf("expected doc to contain %q, got: %s", "Println", doc)
		}
		// signature field: non-empty and contains "func Println"
		sig, _ := result["signature"].(string)
		if sig == "" {
			t.Errorf("expected non-empty signature field, got empty string")
		}
		if sig != "" && !strings.Contains(sig, "func Println") {
			t.Errorf("expected signature to contain %q, got: %s", "func Println", sig)
		}
	})

	t.Run("unsupported_language_returns_error_source", func(t *testing.T) {
		// TypeScript is not supported by the toolchain path.
		result := testGetSymbolDocumentation(t, ctx, session, "console.log", "typescript")
		if source, _ := result["source"].(string); source != "error" {
			raw, _ := json.Marshal(result)
			t.Errorf("expected source=%q for unsupported language, got %q (full: %s)", "error", source, raw)
		}
	})

	t.Run("missing_symbol_field_returns_is_error", func(t *testing.T) {
		// Empty symbol should trigger a validation error (IsError=true).
		res, err := callTool(ctx, session, "get_symbol_documentation", map[string]any{
			"symbol":      "",
			"language_id": "go",
		})
		if err != nil {
			// Some implementations return an error directly for invalid input.
			t.Logf("callTool returned error (acceptable for empty symbol): %v", err)
			return
		}
		if !res.IsError {
			text, _ := textFromResult(res)
			t.Errorf("expected IsError=true for empty symbol, got IsError=false: %s", text)
		}
	})
}
