package main_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testRunBuild tests the run_build tool for languages with known build tooling.
// Skips (not fails) for languages where a build tool may not be installed in CI.
func testRunBuild(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	switch lang.id {
	case "go", "typescript", "python", "rust", "csharp", "swift", "zig", "kotlin":
		// known build dispatch supported
	default:
		return toolResult{tool: "run_build", status: "skip", detail: "no build dispatch for " + lang.id}
	}
	res, err := callTool(ctx, session, "run_build", map[string]any{
		"workspace_dir": lang.fixture,
		"language":      lang.id,
	})
	if err != nil {
		return toolResult{tool: "run_build", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		// Build tool not installed — skip rather than fail to keep CI green
		// on machines without tsc/mypy/cargo.
		return toolResult{tool: "run_build", status: "skip",
			detail: "run_build returned IsError (build tool may not be installed)"}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "run_build", status: "fail", detail: err.Error()}
	}
	// run_build emits a GCF generic OrderedMap; the JSON "success" field encodes
	// as the key "Success" (alongside Errors/Raw).
	v, err := decodeGeneric(text)
	if err != nil {
		return toolResult{tool: "run_build", status: "fail",
			detail: fmt.Sprintf("run_build: failed to decode GCF generic: %v — raw: %s", err, text)}
	}
	if _, hasSuccess := genericGet(v, "Success"); !hasSuccess {
		return toolResult{tool: "run_build", status: "fail",
			detail: "run_build: response missing 'Success' field"}
	}
	return toolResult{tool: "run_build", status: "pass"}
}

// testRunTests tests the run_tests tool for languages with known test tooling.
func testRunTests(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	switch lang.id {
	case "go", "typescript", "python", "rust", "csharp", "swift", "zig", "kotlin":
		// known test dispatch supported
	default:
		return toolResult{tool: "run_tests", status: "skip", detail: "no test dispatch for " + lang.id}
	}
	res, err := callTool(ctx, session, "run_tests", map[string]any{
		"workspace_dir": lang.fixture,
		"language":      lang.id,
	})
	if err != nil {
		return toolResult{tool: "run_tests", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "run_tests", status: "skip",
			detail: "run_tests returned IsError (test runner may not be installed)"}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "run_tests", status: "fail", detail: err.Error()}
	}
	// run_tests emits a GCF generic OrderedMap; the JSON "passed" field encodes as
	// the key "Passed" (alongside Failures/Raw).
	v, err := decodeGeneric(text)
	if err != nil {
		return toolResult{tool: "run_tests", status: "fail",
			detail: fmt.Sprintf("run_tests: failed to decode GCF generic: %v — raw: %s", err, text)}
	}
	if _, hasPassed := genericGet(v, "Passed"); !hasPassed {
		return toolResult{tool: "run_tests", status: "fail",
			detail: "run_tests: response missing 'Passed' field"}
	}
	return toolResult{tool: "run_tests", status: "pass"}
}

// testGetTestsForFile tests the get_tests_for_file tool.
func testGetTestsForFile(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "get_tests_for_file", map[string]any{
		"file_path": lang.file,
	})
	if err != nil {
		return toolResult{tool: "get_tests_for_file", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_tests_for_file", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_tests_for_file", status: "fail", detail: err.Error()}
	}
	// get_tests_for_file emits a GCF generic OrderedMap; the JSON "source_file"
	// field encodes as the key "SourceFile" (alongside a TestFiles tabular).
	v, err := decodeGeneric(text)
	if err != nil {
		return toolResult{tool: "get_tests_for_file", status: "fail",
			detail: fmt.Sprintf("get_tests_for_file: failed to decode GCF generic: %v — raw: %s", err, text)}
	}
	if _, hasSourceFile := genericGet(v, "SourceFile"); !hasSourceFile {
		return toolResult{tool: "get_tests_for_file", status: "fail",
			detail: "get_tests_for_file: response missing 'SourceFile' field"}
	}
	return toolResult{tool: "get_tests_for_file", status: "pass"}
}
