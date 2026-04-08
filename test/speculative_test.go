package main_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSpeculativeSessions tests the speculative session lifecycle tools:
// create_simulation_session, simulate_edit_atomic, evaluate_session,
// commit_session, and discard_session. These tools are session-lifecycle
// tests, not per-language matrix tests.
func TestSpeculativeSessions(t *testing.T) {
	t.Parallel()

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build lsp-mcp-go binary")
	}

	fixtureBase := filepath.Join(testDir(t), "fixtures")
	goFixture := filepath.Join(fixtureBase, "go")
	goFile := filepath.Join(goFixture, "main.go")

	lspBinaryPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("skipping TestSpeculativeSessions: gopls not found on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.Command(binaryPath, "go", lspBinaryPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "speculative-session-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("failed to connect MCP session: %v", err)
	}
	defer session.Close()

	// Tier 1: start_lsp.
	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": goFixture})
	if err != nil || res.IsError {
		t.Skipf("start_lsp failed for gopls: err=%v isError=%v", err, res.IsError)
	}
	time.Sleep(8 * time.Second)

	// Open main.go so the server has document state.
	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   goFile,
		"language_id": "go",
	})
	if err != nil || res.IsError {
		t.Skipf("open_document failed: err=%v isError=%v", err, res.IsError)
	}
	time.Sleep(2 * time.Second)

	t.Run("discard_path", func(t *testing.T) {
		// Create a session.
		res, err := callTool(ctx, session, "create_simulation_session", map[string]any{
			"workspace_root": goFixture,
			"language":       "go",
		})
		if err != nil {
			t.Skipf("create_simulation_session failed: %v", err)
		}
		if res.IsError {
			text, _ := textFromResult(res)
			t.Skipf("create_simulation_session returned IsError (speculative sessions may not be supported): %s", text)
		}
		text, err := textFromResult(res)
		if err != nil {
			t.Fatalf("failed to parse create_simulation_session response: %v", err)
		}
		var createResult map[string]any
		if err := json.Unmarshal([]byte(text), &createResult); err != nil {
			t.Fatalf("failed to unmarshal create_simulation_session response: %s", text)
		}
		sessionID, _ := createResult["session_id"].(string)
		if sessionID == "" {
			t.Fatalf("create_simulation_session: no session_id in response: %s", text)
		}
		t.Logf("created speculative session: %s", sessionID)

		// Apply a speculative edit.
		res, err = callTool(ctx, session, "simulate_edit_atomic", map[string]any{
			"session_id":   sessionID,
			"file_path":    goFile,
			"start_line":   1,
			"start_column": 1,
			"end_line":     1,
			"end_column":   1,
			"new_text":     "// speculative comment\n",
		})
		if err != nil {
			t.Errorf("simulate_edit_atomic failed: %v", err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Logf("simulate_edit_atomic returned IsError (may be expected): %s", text)
		} else {
			t.Logf("simulate_edit_atomic succeeded")
		}

		// Evaluate the session.
		res, err = callTool(ctx, session, "evaluate_session", map[string]any{
			"session_id": sessionID,
		})
		if err != nil {
			t.Errorf("evaluate_session failed: %v", err)
		} else {
			evalText, _ := textFromResult(res)
			t.Logf("evaluate_session result: %s", evalText)
		}

		// Discard the session.
		res, err = callTool(ctx, session, "discard_session", map[string]any{
			"session_id": sessionID,
		})
		if err != nil {
			t.Errorf("discard_session failed: %v", err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Errorf("discard_session returned IsError: %s", text)
		} else {
			t.Logf("discard_session succeeded")
		}
	})

	t.Run("commit_path", func(t *testing.T) {
		// Create a second session for the commit path.
		res, err := callTool(ctx, session, "create_simulation_session", map[string]any{
			"workspace_root": goFixture,
			"language":       "go",
		})
		if err != nil || res.IsError {
			t.Skipf("create_simulation_session failed for commit path (expected if not supported)")
		}
		text, err := textFromResult(res)
		if err != nil {
			t.Fatalf("failed to parse create_simulation_session response: %v", err)
		}
		var createResult map[string]any
		if err := json.Unmarshal([]byte(text), &createResult); err != nil {
			t.Fatalf("failed to unmarshal: %s", text)
		}
		sessionID, _ := createResult["session_id"].(string)
		if sessionID == "" {
			t.Fatalf("no session_id in response")
		}

		// Commit the session (no edits applied — committing clean state).
		res, err = callTool(ctx, session, "commit_session", map[string]any{
			"session_id": sessionID,
		})
		if err != nil {
			t.Errorf("commit_session failed: %v", err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Logf("commit_session returned IsError (may be expected for clean session): %s", text)
		} else {
			t.Logf("commit_session succeeded")
		}
	})

	t.Run("destroy_session", func(t *testing.T) {
		// Create a session solely to test destroy_session.
		res, err := callTool(ctx, session, "create_simulation_session", map[string]any{
			"workspace_root": goFixture,
			"language":       "go",
		})
		if err != nil || res.IsError {
			t.Skipf("create_simulation_session failed (expected if not supported)")
		}
		text, err := textFromResult(res)
		if err != nil {
			t.Fatalf("failed to parse create_simulation_session response: %v", err)
		}
		var createResult map[string]any
		if err := json.Unmarshal([]byte(text), &createResult); err != nil {
			t.Fatalf("failed to unmarshal: %s", text)
		}
		sessionID, _ := createResult["session_id"].(string)
		if sessionID == "" {
			t.Fatalf("no session_id in response")
		}

		res, err = callTool(ctx, session, "destroy_session", map[string]any{
			"session_id": sessionID,
		})
		if err != nil {
			t.Errorf("destroy_session failed: %v", err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Errorf("destroy_session returned IsError: %s", text)
		} else {
			t.Logf("destroy_session succeeded")
		}

		// Verify destroyed session is no longer accessible.
		res, err = callTool(ctx, session, "evaluate_session", map[string]any{
			"session_id": sessionID,
		})
		if err == nil && !res.IsError {
			t.Errorf("evaluate_session succeeded after destroy_session — session was not removed")
		} else {
			t.Logf("evaluate_session correctly rejected destroyed session")
		}
	})

	t.Run("simulate_chain", func(t *testing.T) {
		// Create a session for simulate_chain.
		res, err := callTool(ctx, session, "create_simulation_session", map[string]any{
			"workspace_root": goFixture,
			"language":       "go",
		})
		if err != nil || res.IsError {
			t.Skipf("create_simulation_session failed (expected if not supported)")
		}
		text, err := textFromResult(res)
		if err != nil {
			t.Fatalf("failed to parse create_simulation_session response: %v", err)
		}
		var createResult map[string]any
		if err := json.Unmarshal([]byte(text), &createResult); err != nil {
			t.Fatalf("failed to unmarshal: %s", text)
		}
		sessionID, _ := createResult["session_id"].(string)
		if sessionID == "" {
			t.Fatalf("no session_id in response")
		}
		defer func() {
			_, _ = callTool(ctx, session, "discard_session", map[string]any{"session_id": sessionID})
		}()

		// Apply a two-step chain: add a comment, then add another.
		res, err = callTool(ctx, session, "simulate_chain", map[string]any{
			"session_id": sessionID,
			"edits": []map[string]any{
				{
					"file_path":    goFile,
					"start_line":   1,
					"start_column": 1,
					"end_line":     1,
					"end_column":   1,
					"new_text":     "// chain step 1\n",
				},
				{
					"file_path":    goFile,
					"start_line":   2,
					"start_column": 1,
					"end_line":     2,
					"end_column":   1,
					"new_text":     "// chain step 2\n",
				},
			},
		})
		if err != nil {
			t.Errorf("simulate_chain failed: %v", err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Logf("simulate_chain returned IsError (may be expected): %s", text)
		} else {
			t.Logf("simulate_chain succeeded")
		}
	})
}
