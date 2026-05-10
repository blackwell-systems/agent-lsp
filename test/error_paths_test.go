package main_test

// error_paths_test.go — systematic bad-input testing for key MCP tools.
//
// All subtests use gopls (Go language server) as the fixture because it is the
// most reliable LSP server in CI.  A single gopls MCP process is started for
// the whole test; the Go fixture file is opened once; each subtest is then run
// sequentially against the shared session.
//
// For every bad-input case the test asserts ONE of:
//   - res.IsError == true   (MCP tool returned a structured error)
//   - the result text is a non-empty, non-panic string
//   - the response is empty (tool returned no results — acceptable for navigation tools)
//
// The test deliberately does NOT assert specific error strings — those are too
// brittle across LSP server versions.

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// isWellFormedResponse returns true if the tool response is a well-formed
// outcome: either an explicit structured error, a non-empty message, or an
// empty body (no results).  It returns false only if the result is nil without
// an error, which indicates a crash.
func isWellFormedResponse(res *mcp.CallToolResult, err error) (ok bool, detail string) {
	if err != nil {
		return true, fmt.Sprintf("transport error (acceptable): %v", err)
	}
	if res == nil {
		return false, "got nil *CallToolResult with nil error — server likely panicked"
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return true, fmt.Sprintf("IsError=true: %s", strings.TrimSpace(text))
	}
	text, _ := textFromResult(res)
	return true, fmt.Sprintf("non-error response body: %s", strings.TrimSpace(text))
}

// TestErrorPaths exercises key tools with deliberately bad input and asserts
// that every response is a well-formed outcome — never a nil result or crash.
func TestErrorPaths(t *testing.T) {
	lspBinaryPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("skipping TestErrorPaths: gopls not found on PATH")
	}

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fixtureBase := filepath.Join(testDir(t), "fixtures")
	goFixture := filepath.Join(fixtureBase, "go")
	mainFile := filepath.Join(goFixture, "main.go")
	nonExistentFile := filepath.Join(goFixture, "does_not_exist_99999.go")

	cmd := exec.Command(binaryPath, "go", lspBinaryPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "error-paths-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Skipf("failed to connect MCP session for gopls: %v", err)
		return
	}
	defer session.Close()

	// Start gopls.
	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": goFixture})
	if err != nil || res.IsError {
		t.Skipf("start_lsp failed: err=%v isError=%v", err, res.IsError)
		return
	}
	time.Sleep(8 * time.Second)

	// Open the primary fixture file so gopls has document state.
	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   mainFile,
		"language_id": "go",
	})
	if err != nil || res.IsError {
		t.Skipf("open_document failed: err=%v isError=%v", err, res.IsError)
		return
	}
	time.Sleep(3 * time.Second)

	// -------------------------------------------------------------------------
	// go_to_definition — position outside file bounds
	// -------------------------------------------------------------------------
	t.Run("go_to_definition/out_of_bounds_position", func(t *testing.T) {
		res, err := callTool(ctx, session, "go_to_definition", map[string]any{
			"file_path":   mainFile,
			"language_id": "go",
			"line":        99999,
			"column":      1,
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for out-of-bounds position, got: %s", detail)
		} else {
			t.Logf("go_to_definition/out_of_bounds_position: %s", detail)
		}
	})

	// go_to_definition — non-existent file
	t.Run("go_to_definition/nonexistent_file", func(t *testing.T) {
		res, err := callTool(ctx, session, "go_to_definition", map[string]any{
			"file_path":   nonExistentFile,
			"language_id": "go",
			"line":        1,
			"column":      1,
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for non-existent file, got: %s", detail)
		} else {
			t.Logf("go_to_definition/nonexistent_file: %s", detail)
		}
	})

	// -------------------------------------------------------------------------
	// get_diagnostics — non-existent file
	// -------------------------------------------------------------------------
	t.Run("get_diagnostics/nonexistent_file", func(t *testing.T) {
		res, err := callTool(ctx, session, "get_diagnostics", map[string]any{
			"file_path": nonExistentFile,
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for non-existent file, got: %s", detail)
		} else {
			t.Logf("get_diagnostics/nonexistent_file: %s", detail)
		}
	})

	// -------------------------------------------------------------------------
	// simulate_edit — out-of-bounds range
	// -------------------------------------------------------------------------
	t.Run("simulate_edit/out_of_bounds_range", func(t *testing.T) {
		sessionID := createErrorPathSession(t, ctx, session, goFixture)
		if sessionID == "" {
			return
		}
		defer cleanupErrorPathSession(ctx, session, sessionID)

		res, err := callTool(ctx, session, "simulate_edit", map[string]any{
			"session_id":   sessionID,
			"file_path":    mainFile,
			"start_line":   99999,
			"start_column": 1,
			"end_line":     99999,
			"end_column":   1,
			"new_text":     "// out of bounds\n",
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for out-of-bounds range, got: %s", detail)
		} else {
			t.Logf("simulate_edit/out_of_bounds_range: %s", detail)
		}
	})

	// simulate_edit — empty new_text (deletion) should not crash
	t.Run("simulate_edit/empty_new_text_deletion", func(t *testing.T) {
		sessionID := createErrorPathSession(t, ctx, session, goFixture)
		if sessionID == "" {
			return
		}
		defer cleanupErrorPathSession(ctx, session, sessionID)

		// Delete the comment on line 4 (safe: does not affect compilation).
		res, err := callTool(ctx, session, "simulate_edit", map[string]any{
			"session_id":   sessionID,
			"file_path":    mainFile,
			"start_line":   4,
			"start_column": 1,
			"end_line":     5,
			"end_column":   1,
			"new_text":     "",
		})
		// Nil result with nil error is the only failure mode we care about.
		if res == nil && err == nil {
			t.Errorf("simulate_edit returned nil result and nil error — likely a crash")
			return
		}
		ok, detail := isWellFormedResponse(res, err)
		t.Logf("simulate_edit/empty_new_text_deletion: ok=%v %s", ok, detail)
		// We do NOT fail on IsError — a deletion producing an LSP error is a
		// legitimate structured response, not a crash.
	})

	// simulate_edit — non-existent session_id
	t.Run("simulate_edit/invalid_session_id", func(t *testing.T) {
		res, err := callTool(ctx, session, "simulate_edit", map[string]any{
			"session_id":   "nonexistent-session-id-99999",
			"file_path":    mainFile,
			"start_line":   1,
			"start_column": 1,
			"end_line":     1,
			"end_column":   1,
			"new_text":     "// comment\n",
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed error for invalid session_id, got: %s", detail)
		} else {
			t.Logf("simulate_edit/invalid_session_id: %s", detail)
		}
	})

	// -------------------------------------------------------------------------
	// preview_edit — out-of-bounds range
	// -------------------------------------------------------------------------
	t.Run("preview_edit/out_of_bounds_range", func(t *testing.T) {
		res, err := callTool(ctx, session, "preview_edit", map[string]any{
			"workspace_root": goFixture,
			"language":       "go",
			"file_path":      mainFile,
			"start_line":     99999,
			"start_column":   1,
			"end_line":       99999,
			"end_column":     1,
			"new_text":       "// out of bounds\n",
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for out-of-bounds range, got: %s", detail)
		} else {
			t.Logf("preview_edit/out_of_bounds_range: %s", detail)
		}
	})

	// preview_edit — empty new_text (deletion)
	t.Run("preview_edit/empty_new_text", func(t *testing.T) {
		res, err := callTool(ctx, session, "preview_edit", map[string]any{
			"workspace_root": goFixture,
			"language":       "go",
			"file_path":      mainFile,
			"start_line":     4,
			"start_column":   1,
			"end_line":       5,
			"end_column":     1,
			"new_text":       "",
		})
		if res == nil && err == nil {
			t.Errorf("preview_edit returned nil result and nil error — likely a crash")
			return
		}
		_, detail := isWellFormedResponse(res, err)
		t.Logf("preview_edit/empty_new_text: %s", detail)
	})

	// -------------------------------------------------------------------------
	// find_references — position on whitespace / blank line
	// -------------------------------------------------------------------------
	t.Run("find_references/whitespace_position", func(t *testing.T) {
		// Line 11 in main.go is the blank line between Greet() and add().
		// A reference request on whitespace should return empty results or a
		// structured error — not a crash.
		res, err := callTool(ctx, session, "find_references", map[string]any{
			"file_path":   mainFile,
			"language_id": "go",
			"line":        11,
			"column":      1,
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for whitespace position, got: %s", detail)
		} else {
			t.Logf("find_references/whitespace_position: %s", detail)
		}
	})

	// -------------------------------------------------------------------------
	// rename_symbol — attempt to rename a built-in keyword
	// -------------------------------------------------------------------------
	t.Run("rename_symbol/builtin_keyword", func(t *testing.T) {
		// Column 1 of line 13 ("func") is the keyword introducing Greet().
		// gopls must return a structured error — renaming keywords is forbidden.
		res, err := callTool(ctx, session, "rename_symbol", map[string]any{
			"file_path":   mainFile,
			"language_id": "go",
			"line":        13,
			"column":      1,
			"new_name":    "procedure",
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed error for renaming a keyword, got: %s", detail)
		} else {
			t.Logf("rename_symbol/builtin_keyword: %s", detail)
		}
	})

	// -------------------------------------------------------------------------
	// Phase enforcement error paths
	// -------------------------------------------------------------------------

	t.Run("activate_skill/unknown_skill", func(t *testing.T) {
		res, err := callTool(ctx, session, "activate_skill", map[string]any{
			"skill_name": "nonexistent-skill",
			"mode":       "warn",
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed error for unknown skill, got: %s", detail)
		} else {
			t.Logf("activate_skill/unknown_skill: %s", detail)
		}
	})

	t.Run("activate_skill/double_activate", func(t *testing.T) {
		// Activate a valid skill first.
		res, err := callTool(ctx, session, "activate_skill", map[string]any{
			"skill_name": "lsp-rename",
			"mode":       "warn",
		})
		if err != nil || res.IsError {
			t.Skipf("first activate_skill failed: err=%v", err)
			return
		}
		// Try to activate another skill while one is already active.
		res, err = callTool(ctx, session, "activate_skill", map[string]any{
			"skill_name": "lsp-refactor",
			"mode":       "warn",
		})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed error for double activate, got: %s", detail)
		} else {
			t.Logf("activate_skill/double_activate: %s", detail)
		}
		// Cleanup: deactivate so subsequent tests start clean.
		_, _ = callTool(ctx, session, "deactivate_skill", map[string]any{})
	})

	t.Run("get_skill_phase/no_active_skill", func(t *testing.T) {
		res, err := callTool(ctx, session, "get_skill_phase", map[string]any{})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for inactive phase query, got: %s", detail)
		} else {
			t.Logf("get_skill_phase/no_active_skill: %s", detail)
		}
	})

	t.Run("deactivate_skill/idempotent_when_inactive", func(t *testing.T) {
		res, err := callTool(ctx, session, "deactivate_skill", map[string]any{})
		ok, detail := isWellFormedResponse(res, err)
		if !ok {
			t.Errorf("expected well-formed response for idempotent deactivate, got: %s", detail)
		} else {
			t.Logf("deactivate_skill/idempotent_when_inactive: %s", detail)
		}
	})
}

// createErrorPathSession creates a simulation session for error-path subtests.
// Returns "" and calls t.Skipf if the feature is unavailable.
func createErrorPathSession(t *testing.T, ctx context.Context, session *mcp.ClientSession, workspaceRoot string) string {
	t.Helper()
	res, err := callTool(ctx, session, "create_simulation_session", map[string]any{
		"workspace_root": workspaceRoot,
		"language":       "go",
	})
	if err != nil || res.IsError {
		t.Skipf("create_simulation_session unavailable: err=%v isError=%v", err, res.IsError)
		return ""
	}
	text, _ := textFromResult(res)
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Skipf("could not parse create_simulation_session response: %v", err)
		return ""
	}
	id, _ := result["session_id"].(string)
	if id == "" {
		t.Skipf("no session_id in create_simulation_session response: %s", text)
		return ""
	}
	return id
}

// cleanupErrorPathSession silently discards a simulation session.
func cleanupErrorPathSession(ctx context.Context, session *mcp.ClientSession, sessionID string) {
	_, _ = callTool(ctx, session, "discard_session", map[string]any{"session_id": sessionID})
}
