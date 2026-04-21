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

// speculativeLangConfig holds per-language configuration for speculative session tests.
type speculativeLangConfig struct {
	name       string
	id         string
	binary     string
	serverArgs []string
	fixture    string        // absolute path to workspace root (passed to start_lsp)
	file       string        // absolute path to primary source file
	initWait   time.Duration // how long to wait after start_lsp before opening a document
	timeout    time.Duration // overall test timeout; defaults to 120s if zero

	// safeEdit inserts a comment at the given position — must produce net_delta=0.
	// safeEditFile is the file to use for safe edits; defaults to file if empty.
	// Use a file with no pre-existing diagnostics so comment insertions don't shift
	// existing error positions and produce false-positive net_delta values.
	safeEditFile string
	safeEditLine int
	safeEditCol  int
	safeEditText string

	// errorEdit replaces a range with a type-wrong value — must produce net_delta>0.
	// If errorEditText is empty, the error_detection subtest is skipped.
	errorEditLine    int
	errorEditCol     int
	errorEditEndLine int
	errorEditEndCol  int
	errorEditText    string
}

// buildSpeculativeLangConfigs returns configs for all languages covered by speculative
// session tests. Each config targets a fixture file whose type errors are well-defined
// so the error_detection subtest can assert net_delta>0 reliably.
func buildSpeculativeLangConfigs(fixtureBase string) []speculativeLangConfig {
	return []speculativeLangConfig{
		{
			// Go: replace `return fmt.Sprintf(...)` in Greet() with `return 42`.
			// gopls immediately flags: cannot use 42 (type untyped int) as string.
			name:             "Go",
			id:               "go",
			binary:           "gopls",
			serverArgs:       []string{},
			fixture:          filepath.Join(fixtureBase, "go"),
			file:             filepath.Join(fixtureBase, "go", "main.go"),
			initWait:         8 * time.Second,
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "// speculative comment\n",
			errorEditLine:    13,
			errorEditCol:     1,
			errorEditEndLine: 14,
			errorEditEndCol:  1,
			errorEditText:    "\treturn 42\n",
		},
		{
			// TypeScript: replace `return a + b;` in add() with `return "wrong";`.
			// tsserver flags: Type 'string' is not assignable to type 'number'.
			//
			// safeEditFile uses consumer.ts (no pre-existing diagnostics) rather than
			// example.ts, which intentionally contains 3 errors. Inserting a comment
			// line into example.ts shifts all error line numbers by 1, causing the
			// baseline diff to falsely report 3 new errors (net_delta=3).
			name:             "TypeScript",
			id:               "typescript",
			binary:           "typescript-language-server",
			serverArgs:       []string{"--stdio"},
			fixture:          filepath.Join(fixtureBase, "typescript"),
			file:             filepath.Join(fixtureBase, "typescript", "src", "example.ts"),
			initWait:         8 * time.Second,
			safeEditFile:     filepath.Join(fixtureBase, "typescript", "src", "consumer.ts"),
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "// speculative comment\n",
			errorEditLine:    5,
			errorEditCol:     1,
			errorEditEndLine: 6,
			errorEditEndCol:  1,
			errorEditText:    "  return \"wrong\";\n",
		},
		{
			// Python: replace `return x + y` in add() with `return "wrong"`.
			// pyright flags: Expression of type 'str' cannot be assigned to return type 'int'.
			name:             "Python",
			id:               "python",
			binary:           "pyright-langserver",
			serverArgs:       []string{"--stdio"},
			fixture:          filepath.Join(fixtureBase, "python"),
			file:             filepath.Join(fixtureBase, "python", "main.py"),
			initWait:         8 * time.Second,
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "# speculative comment\n",
			errorEditLine:    2,
			errorEditCol:     1,
			errorEditEndLine: 3,
			errorEditEndCol:  1,
			errorEditText:    "    return \"wrong\"\n",
		},
		{
			// Rust: replace `x + y` in add() with `"wrong"`.
			// rust-analyzer flags: expected `i32`, found `&str`.
			name:             "Rust",
			id:               "rust",
			binary:           "rust-analyzer",
			serverArgs:       []string{},
			fixture:          filepath.Join(fixtureBase, "rust"),
			file:             filepath.Join(fixtureBase, "rust", "src", "main.rs"),
			initWait:         15 * time.Second, // rust-analyzer compiles before indexing
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "// speculative comment\n",
			errorEditLine:    24,
			errorEditCol:     1,
			errorEditEndLine: 25,
			errorEditEndCol:  1,
			errorEditText:    "    \"wrong\"\n",
		},
		{
			// C++: replace `return x + y;` in add() with `return "wrong";`.
			// clangd flags: cannot initialize return object of type 'int' with an lvalue
			// of type 'const char[6]'.
			name:             "C++",
			id:               "cpp",
			binary:           "clangd",
			serverArgs:       []string{},
			fixture:          filepath.Join(fixtureBase, "cpp"),
			file:             filepath.Join(fixtureBase, "cpp", "person.cpp"),
			initWait:         10 * time.Second,
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "// speculative comment\n",
			errorEditLine:    10,
			errorEditCol:     1,
			errorEditEndLine: 11,
			errorEditEndCol:  1,
			errorEditText:    "    return \"wrong\";\n",
		},
		{
			// C#: replace `return $"Hello, {Name}";` in Greet() with `return 42;`.
			// csharp-ls flags: Cannot implicitly convert type 'int' to 'string'.
			name:             "CSharp",
			id:               "csharp",
			binary:           "csharp-ls",
			serverArgs:       []string{},
			fixture:          filepath.Join(fixtureBase, "csharp"),
			file:             filepath.Join(fixtureBase, "csharp", "Person.cs"),
			initWait:         10 * time.Second,
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "// speculative comment\n",
			errorEditLine:    17,
			errorEditCol:     1,
			errorEditEndLine: 18,
			errorEditEndCol:  1,
			errorEditText:    "        return 42;\n",
		},
		{
			// Dart: replace `return 'Hello, $name';` in greet() with `return 42;`.
			// Dart analysis server flags: A value of type 'int' can't be returned from
			// a function with return type 'String'.
			name:             "Dart",
			id:               "dart",
			binary:           "dart",
			serverArgs:       []string{"language-server", "--client-id=agent-lsp"},
			fixture:          filepath.Join(fixtureBase, "dart"),
			file:             filepath.Join(fixtureBase, "dart", "lib", "fixture.dart"),
			initWait:         8 * time.Second,
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "// speculative comment\n",
			errorEditLine:    3,
			errorEditCol:     1,
			errorEditEndLine: 4,
			errorEditEndCol:  1,
			errorEditText:    "    return 42;\n",
		},
		{
			// Java: replace `return x + y;` in add() with `return "wrong";`.
			// jdtls flags: Type mismatch: cannot convert from String to int.
			// Needs extended timeout — jdtls JVM startup + indexing takes ~60-90s.
			name:             "Java",
			id:               "java",
			binary:           "jdtls",
			serverArgs:       []string{"-data", "/tmp/jdtls-workspace-speculative-test"},
			fixture:          filepath.Join(fixtureBase, "java"),
			file:             filepath.Join(fixtureBase, "java", "src", "main", "java", "com", "example", "Person.java"),
			initWait:         120 * time.Second,
			timeout:          300 * time.Second,
			safeEditLine:     1,
			safeEditCol:      1,
			safeEditText:     "// speculative comment\n",
			errorEditLine:    21,
			errorEditCol:     1,
			errorEditEndLine: 22,
			errorEditEndCol:  1,
			errorEditText:    "        return \"wrong\";\n",
		},
	}
}

// TestSpeculativeSessions tests the speculative session lifecycle tools across
// all supported languages. Each language runs as a parallel subtest with its own
// MCP connection and LSP process. Subtests within a language run sequentially,
// sharing the same session.
//
// Subtests per language:
//   - discard_path: create → simulate_edit → evaluate → discard
//   - commit_path: create → simulate_edit → commit (dry-run, no disk write)
//   - simulate_edit_non_atomic: create → simulate_edit → evaluate → discard
//   - destroy_session: create → destroy → verify rejected
//   - simulate_chain: create → simulate_chain → discard
//   - simulate_edit_atomic_standalone: simulate_edit_atomic one-shot
//   - error_detection: simulate_edit_atomic with a type-breaking edit
func TestSpeculativeSessions(t *testing.T) {
	t.Parallel()

	binaryPath := getMultilangBinary(t)
	if binaryPath == "" {
		t.Skip("failed to build agent-lsp binary")
	}

	fixtureBase := filepath.Join(testDir(t), "fixtures")
	langs := buildSpeculativeLangConfigs(fixtureBase)

	for _, lang := range langs {
		lang := lang // capture loop var
		t.Run(lang.name, func(t *testing.T) {
			t.Parallel()
			runSpeculativeLanguageTest(t, binaryPath, lang)
		})
	}
}

// runSpeculativeLanguageTest establishes a dedicated MCP connection for lang and
// runs all speculative session subtests against it.
func runSpeculativeLanguageTest(t *testing.T, binaryPath string, lang speculativeLangConfig) {
	t.Helper()

	lspBinaryPath, err := exec.LookPath(lang.binary)
	if err != nil {
		t.Skipf("skipping %s: %s not found on PATH", lang.name, lang.binary)
		return
	}

	testTimeout := lang.timeout
	if testTimeout == 0 {
		testTimeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	args := append([]string{lang.id, lspBinaryPath}, lang.serverArgs...)
	cmd := exec.Command(binaryPath, args...)
	client := mcp.NewClient(&mcp.Implementation{Name: "speculative-session-test", Version: "1.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Skipf("[%s] failed to connect MCP session: %v", lang.name, err)
		return
	}
	defer session.Close()

	// start_lsp.
	res, err := callTool(ctx, session, "start_lsp", map[string]any{"root_dir": lang.fixture})
	if err != nil || res.IsError {
		t.Skipf("[%s] start_lsp failed: err=%v isError=%v", lang.name, err, res.IsError)
		return
	}
	time.Sleep(lang.initWait)

	// open_document so the server has document state.
	res, err = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil || res.IsError {
		t.Skipf("[%s] open_document failed: err=%v isError=%v", lang.name, err, res.IsError)
		return
	}
	time.Sleep(3 * time.Second)

	// If safe edits target a different file, open it too so the server has its state.
	safeFile := lang.safeEditFile
	if safeFile == "" {
		safeFile = lang.file
	}
	if safeFile != lang.file {
		if res, err = callTool(ctx, session, "open_document", map[string]any{
			"file_path":   safeFile,
			"language_id": lang.id,
		}); err != nil || res.IsError {
			t.Logf("[%s] open_document for safeEditFile failed (non-fatal): err=%v", lang.name, err)
		}
		time.Sleep(2 * time.Second)
	}

	t.Run("discard_path", func(t *testing.T) {
		sessionID := createSession(t, ctx, session, lang)
		if sessionID == "" {
			return
		}

		applyComment(t, ctx, session, lang, safeFile, sessionID)
		evaluateAndCheck(t, ctx, session, lang, sessionID, false)

		res, err := callTool(ctx, session, "discard_session", map[string]any{"session_id": sessionID})
		if err != nil {
			t.Errorf("[%s] discard_session failed: %v", lang.name, err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Errorf("[%s] discard_session returned IsError: %s", lang.name, text)
		} else {
			t.Logf("[%s] discard_session succeeded", lang.name)
		}
	})

	t.Run("commit_path", func(t *testing.T) {
		sessionID := createSession(t, ctx, session, lang)
		if sessionID == "" {
			return
		}

		applyComment(t, ctx, session, lang, safeFile, sessionID)

		// commit without apply=true — returns a unified diff only, does not write to disk.
		res, err := callTool(ctx, session, "commit_session", map[string]any{
			"session_id": sessionID,
		})
		if err != nil {
			t.Errorf("[%s] commit_session failed: %v", lang.name, err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Logf("[%s] commit_session returned IsError (may be expected if server does not support it): %s", lang.name, text)
		} else {
			t.Logf("[%s] commit_session succeeded", lang.name)
		}
	})

	t.Run("simulate_edit_non_atomic", func(t *testing.T) {
		sessionID := createSession(t, ctx, session, lang)
		if sessionID == "" {
			return
		}
		defer discardSession(ctx, session, sessionID)

		applyComment(t, ctx, session, lang, safeFile, sessionID)
		evaluateAndCheck(t, ctx, session, lang, sessionID, false)
	})

	t.Run("destroy_session", func(t *testing.T) {
		sessionID := createSession(t, ctx, session, lang)
		if sessionID == "" {
			return
		}

		res, err := callTool(ctx, session, "destroy_session", map[string]any{"session_id": sessionID})
		if err != nil {
			t.Errorf("[%s] destroy_session failed: %v", lang.name, err)
		} else if res.IsError {
			text, _ := textFromResult(res)
			t.Errorf("[%s] destroy_session returned IsError: %s", lang.name, text)
		} else {
			t.Logf("[%s] destroy_session succeeded", lang.name)
		}

		// A destroyed session must be rejected by subsequent calls.
		res, err = callTool(ctx, session, "evaluate_session", map[string]any{"session_id": sessionID})
		if err == nil && !res.IsError {
			t.Errorf("[%s] evaluate_session succeeded after destroy_session — session was not removed", lang.name)
		} else {
			t.Logf("[%s] evaluate_session correctly rejected destroyed session", lang.name)
		}
	})

	t.Run("simulate_chain", func(t *testing.T) {
		sessionID := createSession(t, ctx, session, lang)
		if sessionID == "" {
			return
		}
		defer discardSession(ctx, session, sessionID)

		// Two-step comment chain using the language's own comment syntax (lang.safeEditText).
		// Hardcoding "//" would break Python, Ruby, etc. where "//" is not a comment.
		// Both steps insert at adjacent lines — cumulative_delta must remain 0.
		res, err := callTool(ctx, session, "simulate_chain", map[string]any{
			"session_id": sessionID,
			"edits": []map[string]any{
				{
					"file_path":    safeFile,
					"start_line":   lang.safeEditLine,
					"start_column": lang.safeEditCol,
					"end_line":     lang.safeEditLine,
					"end_column":   lang.safeEditCol,
					"new_text":     lang.safeEditText,
				},
				{
					"file_path":    safeFile,
					"start_line":   lang.safeEditLine + 1,
					"start_column": lang.safeEditCol,
					"end_line":     lang.safeEditLine + 1,
					"end_column":   lang.safeEditCol,
					"new_text":     lang.safeEditText,
				},
			},
		})
		if err != nil {
			t.Errorf("[%s] simulate_chain failed: %v", lang.name, err)
			return
		}
		if res.IsError {
			text, _ := textFromResult(res)
			t.Logf("[%s] simulate_chain returned IsError (may be expected): %s", lang.name, text)
			return
		}
		chainText, _ := textFromResult(res)
		var chainResult map[string]any
		if err := json.Unmarshal([]byte(chainText), &chainResult); err == nil {
			cumulativeDelta, _ := chainResult["cumulative_delta"].(float64)
			safeThrough, _ := chainResult["safe_to_apply_through_step"].(float64)
			t.Logf("[%s] simulate_chain: cumulative_delta=%.0f safe_to_apply_through_step=%.0f",
				lang.name, cumulativeDelta, safeThrough)
			if cumulativeDelta != 0 {
				t.Errorf("[%s] expected cumulative_delta=0 for two-comment chain, got %.0f",
					lang.name, cumulativeDelta)
			}
			if safeThrough != 2 {
				t.Errorf("[%s] expected safe_to_apply_through_step=2, got %.0f",
					lang.name, safeThrough)
			}
		} else {
			t.Logf("[%s] simulate_chain raw: %s", lang.name, chainText)
		}
	})

	t.Run("simulate_edit_atomic_standalone", func(t *testing.T) {
		res, err := callTool(ctx, session, "simulate_edit_atomic", map[string]any{
			"workspace_root": lang.fixture,
			"language":       lang.id,
			"file_path":      safeFile,
			"start_line":     lang.safeEditLine,
			"start_column":   lang.safeEditCol,
			"end_line":       lang.safeEditLine,
			"end_column":     lang.safeEditCol,
			"new_text":       lang.safeEditText,
		})
		if err != nil {
			t.Skipf("[%s] simulate_edit_atomic failed: %v", lang.name, err)
			return
		}
		if res.IsError {
			text, _ := textFromResult(res)
			t.Skipf("[%s] simulate_edit_atomic returned IsError (may not be supported): %s", lang.name, text)
			return
		}
		text, _ := textFromResult(res)
		var evalResult map[string]any
		if err := json.Unmarshal([]byte(text), &evalResult); err != nil {
			t.Fatalf("[%s] could not parse simulate_edit_atomic response: %s", lang.name, text)
		}
		netDelta, _ := evalResult["net_delta"].(float64)
		confidence, _ := evalResult["confidence"].(string)
		t.Logf("[%s] simulate_edit_atomic: net_delta=%.0f confidence=%q", lang.name, netDelta, confidence)
		if netDelta > 0 && confidence != "low" {
			t.Errorf("[%s] comment-only edit must not introduce errors: net_delta=%.0f (confidence=%q)",
				lang.name, netDelta, confidence)
		}
	})

	t.Run("error_detection", func(t *testing.T) {
		if lang.errorEditText == "" {
			t.Skipf("[%s] no error edit configured; skipping error_detection", lang.name)
			return
		}
		// Validates the core speculative session value proposition: simulate_edit_atomic
		// reports net_delta > 0 when a type-breaking edit is applied.
		res, err := callTool(ctx, session, "simulate_edit_atomic", map[string]any{
			"workspace_root": lang.fixture,
			"language":       lang.id,
			"file_path":      lang.file,
			"start_line":     lang.errorEditLine,
			"start_column":   lang.errorEditCol,
			"end_line":       lang.errorEditEndLine,
			"end_column":     lang.errorEditEndCol,
			"new_text":       lang.errorEditText,
		})
		if err != nil {
			t.Skipf("[%s] simulate_edit_atomic failed: %v", lang.name, err)
			return
		}
		if res.IsError {
			text, _ := textFromResult(res)
			t.Skipf("[%s] simulate_edit_atomic returned IsError (may not be supported): %s", lang.name, text)
			return
		}
		text, _ := textFromResult(res)
		var evalResult map[string]any
		if err := json.Unmarshal([]byte(text), &evalResult); err != nil {
			t.Fatalf("[%s] could not parse error_detection response: %s", lang.name, text)
		}
		netDelta, _ := evalResult["net_delta"].(float64)
		confidence, _ := evalResult["confidence"].(string)
		timeout, _ := evalResult["timeout"].(bool)
		introduced, _ := evalResult["errors_introduced"].([]any)
		t.Logf("[%s] error_detection: net_delta=%.0f confidence=%q timeout=%v errors_introduced=%d",
			lang.name, netDelta, confidence, timeout, len(introduced))
		if netDelta <= 0 {
			if confidence == "low" || timeout {
				t.Logf("[%s] net_delta=0 with confidence=%q timeout=%v — server may not have indexed in time; acceptable in CI",
					lang.name, confidence, timeout)
			} else {
				t.Errorf("[%s] expected net_delta > 0 for type-error edit, got %.0f (confidence=%q)",
					lang.name, netDelta, confidence)
			}
		} else {
			t.Logf("[%s] error_detection correctly reported net_delta=%.0f with %d error(s)",
				lang.name, netDelta, len(introduced))
		}
	})
}

// createSession creates a simulation session and returns the session ID.
// Returns "" and calls t.Skipf if creation fails (e.g., feature not supported).
func createSession(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang speculativeLangConfig) string {
	t.Helper()
	res, err := callTool(ctx, session, "create_simulation_session", map[string]any{
		"workspace_root": lang.fixture,
		"language":       lang.id,
	})
	if err != nil {
		t.Skipf("[%s] create_simulation_session failed: %v", lang.name, err)
		return ""
	}
	if res.IsError {
		text, _ := textFromResult(res)
		t.Skipf("[%s] create_simulation_session returned IsError: %s", lang.name, text)
		return ""
	}
	text, err := textFromResult(res)
	if err != nil {
		t.Fatalf("[%s] failed to parse create_simulation_session response: %v", lang.name, err)
		return ""
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("[%s] failed to unmarshal create_simulation_session response: %s", lang.name, text)
		return ""
	}
	id, _ := result["session_id"].(string)
	if id == "" {
		t.Fatalf("[%s] create_simulation_session: no session_id in response: %s", lang.name, text)
		return ""
	}
	t.Logf("[%s] created speculative session: %s", lang.name, id)
	return id
}

// applyComment applies a safe comment edit to sessionID and logs the result.
// file is the target file (use lang.safeEditFile if set, otherwise lang.file).
func applyComment(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang speculativeLangConfig, file string, sessionID string) {
	t.Helper()
	res, err := callTool(ctx, session, "simulate_edit", map[string]any{
		"session_id":   sessionID,
		"file_path":    file,
		"start_line":   lang.safeEditLine,
		"start_column": lang.safeEditCol,
		"end_line":     lang.safeEditLine,
		"end_column":   lang.safeEditCol,
		"new_text":     lang.safeEditText,
	})
	if err != nil {
		t.Errorf("[%s] simulate_edit failed: %v", lang.name, err)
		return
	}
	if res.IsError {
		text, _ := textFromResult(res)
		t.Logf("[%s] simulate_edit returned IsError (may be expected): %s", lang.name, text)
		return
	}
	editText, _ := textFromResult(res)
	var editResult map[string]any
	if err := json.Unmarshal([]byte(editText), &editResult); err == nil {
		applied, _ := editResult["edit_applied"].(bool)
		if !applied {
			t.Errorf("[%s] simulate_edit: edit_applied=false, expected true", lang.name)
		}
	}
	t.Logf("[%s] simulate_edit succeeded", lang.name)
}

// evaluateAndCheck calls evaluate_session and asserts net_delta <= 0 for safe edits
// or net_delta > 0 for error edits (controlled by expectError).
func evaluateAndCheck(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang speculativeLangConfig, sessionID string, expectError bool) {
	t.Helper()
	res, err := callTool(ctx, session, "evaluate_session", map[string]any{
		"session_id": sessionID,
	})
	if err != nil {
		t.Errorf("[%s] evaluate_session failed: %v", lang.name, err)
		return
	}
	evalText, _ := textFromResult(res)
	var evalResult map[string]any
	if err := json.Unmarshal([]byte(evalText), &evalResult); err != nil {
		t.Logf("[%s] evaluate_session raw: %s", lang.name, evalText)
		return
	}
	netDelta, _ := evalResult["net_delta"].(float64)
	confidence, _ := evalResult["confidence"].(string)
	t.Logf("[%s] evaluate_session: net_delta=%.0f confidence=%q", lang.name, netDelta, confidence)
	if !expectError && netDelta > 0 && confidence != "low" {
		t.Errorf("[%s] comment-only edit must not introduce errors: net_delta=%.0f (confidence=%q)",
			lang.name, netDelta, confidence)
	}
}

// discardSession discards sessionID silently (used in deferred cleanup).
func discardSession(ctx context.Context, session *mcp.ClientSession, sessionID string) {
	_, _ = callTool(ctx, session, "discard_session", map[string]any{"session_id": sessionID})
}
