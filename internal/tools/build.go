// build.go implements the run_build, run_tests, and get_tests_for_file MCP tools.
// These tools run language-specific toolchain commands (go build, tsc, cargo build,
// mypy, etc.) and parse their output into structured BuildError/TestFailure types.
//
// Key design:
//   - The runners dispatch table maps language IDs to build/test commands with
//     template args. "{path}" placeholders are replaced at runtime.
//   - Build and test output parsers are language-specific regex matchers that
//     extract file:line:column:message tuples from compiler output.
//   - These tools do not require an LSP client. They shell out to the
//     language's native toolchain independently.
//   - For Go, GOWORK=off is always set to prevent the inherited shell workspace
//     from affecting builds in the analyzed directory.
package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleRunBuild is an MCP tool handler for run_build. It runs the language's
// build tool in the specified workspace directory and returns structured output.
// No *lsp.LSPClient parameter — runs shell commands independently of any LSP session.
func HandleRunBuild(ctx context.Context, args map[string]interface{}) (types.ToolResult, error) {
	workspaceDir, _ := args["workspace_dir"].(string)
	if workspaceDir == "" {
		return types.ErrorResult("workspace_dir is required"), nil
	}

	path, _ := args["path"].(string)
	lang, _ := args["language"].(string)

	if lang == "" {
		root, detectedLang, err := config.InferWorkspaceRoot(workspaceDir)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("inferring workspace root: %s", err)), nil
		}
		if root != "" {
			workspaceDir = root
		}
		lang = detectedLang
	}

	if _, ok := runners[lang]; !ok {
		return types.ErrorResult(fmt.Sprintf("unsupported language: %s", lang)), nil
	}

	result, err := RunBuild(ctx, workspaceDir, lang, path)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("run_build: %s", err)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling build result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleRunTests is an MCP tool handler for run_tests. It runs the language's
// test tool in the specified workspace directory and returns structured output.
// No *lsp.LSPClient parameter — runs shell commands independently of any LSP session.
func HandleRunTests(ctx context.Context, args map[string]interface{}) (types.ToolResult, error) {
	workspaceDir, _ := args["workspace_dir"].(string)
	if workspaceDir == "" {
		return types.ErrorResult("workspace_dir is required"), nil
	}

	path, _ := args["path"].(string)
	lang, _ := args["language"].(string)

	if lang == "" {
		root, detectedLang, err := config.InferWorkspaceRoot(workspaceDir)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("inferring workspace root: %s", err)), nil
		}
		if root != "" {
			workspaceDir = root
		}
		lang = detectedLang
	}

	if _, ok := runners[lang]; !ok {
		return types.ErrorResult(fmt.Sprintf("unsupported language: %s", lang)), nil
	}

	result, err := RunTests(ctx, workspaceDir, lang, path)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("run_tests: %s", err)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling test result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// HandleGetTestsForFile is an MCP tool handler for get_tests_for_file.
// Returns test files that exercise the given source file.
// No *lsp.LSPClient parameter — performs static file lookup without LSP.
func HandleGetTestsForFile(ctx context.Context, args map[string]interface{}) (types.ToolResult, error) {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	root, lang, err := config.InferWorkspaceRoot(filePath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("inferring workspace root: %s", err)), nil
	}

	result, err := FindTestFiles(ctx, root, lang, filePath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("find_test_files: %s", err)), nil
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling test file result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// RunBuild executes the language's build command in the given root directory
// and parses output into structured BuildErrors.
func RunBuild(ctx context.Context, root, lang, path string) (BuildResult, error) {
	runner, ok := runners[lang]
	if !ok {
		return BuildResult{}, fmt.Errorf("unsupported language: %s", lang)
	}

	// Resolve the path placeholder.
	resolvedPath := resolveBuildPath(lang, path)

	// Build args by replacing "{path}" placeholder or appending path.
	args := applyPathArg(runner.buildArgs, resolvedPath)

	cmd := exec.CommandContext(ctx, runner.buildCmd, args...)
	cmd.Dir = root

	// For Go, always disable workspace mode to build all modules in the directory
	// (go.work may exist in parent directories and affect behavior)
	if lang == "go" {
		cmd.Env = append(os.Environ(), "GOWORK=off")
	}

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return BuildResult{}, fmt.Errorf("executing build: %w", err)
		}
	}

	errors := parseBuildErrors(lang, output)
	return BuildResult{
		Success: exitCode == 0,
		Errors:  errors,
		Raw:     string(output),
	}, nil
}

// RunTests executes the language's test command in the given root directory
// and parses output into structured TestFailures.
func RunTests(ctx context.Context, root, lang, path string) (TestResult, error) {
	runner, ok := runners[lang]
	if !ok {
		return TestResult{}, fmt.Errorf("unsupported language: %s", lang)
	}

	// Resolve the path placeholder.
	resolvedPath := resolveTestPath(lang, path)

	// Build args by replacing "{path}" placeholder or appending path.
	args := applyPathArg(runner.testArgs, resolvedPath)

	cmd := exec.CommandContext(ctx, runner.testCmd, args...)
	cmd.Dir = root

	// For Go, always disable workspace mode to test all modules in the directory
	// (go.work may exist in parent directories and affect behavior)
	if lang == "go" {
		cmd.Env = append(os.Environ(), "GOWORK=off")
	}

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return TestResult{}, fmt.Errorf("executing tests: %w", err)
		}
	}

	failures := parseTestFailures(lang, root, output)
	return TestResult{
		Passed:   exitCode == 0,
		Failures: failures,
		Raw:      string(output),
	}, nil
}

// FindTestFiles performs a static file lookup to find test files for a source file,
// without running any build or test commands.
func FindTestFiles(_ context.Context, root, lang, sourceFile string) (TestFileResult, error) {
	absSource, err := filepath.Abs(sourceFile)
	if err != nil {
		absSource = sourceFile
	}

	dir := filepath.Dir(absSource)

	var testFiles []string

	switch lang {
	case "go":
		matches, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
		testFiles = matches

	case "python":
		var found []string
		patterns := []string{
			filepath.Join(dir, "test_*.py"),
			filepath.Join(dir, "*_test.py"),
		}
		for _, p := range patterns {
			m, _ := filepath.Glob(p)
			found = append(found, m...)
		}
		// Also check tests/ sibling directory.
		if root != "" {
			testsDir := filepath.Join(root, "tests")
			testPatterns := []string{
				filepath.Join(testsDir, "test_*.py"),
				filepath.Join(testsDir, "*_test.py"),
			}
			for _, p := range testPatterns {
				m, _ := filepath.Glob(p)
				found = append(found, m...)
			}
		}
		testFiles = found

	case "typescript", "javascript":
		var found []string
		exts := []string{".ts", ".js"}
		suffixes := []string{"test", "spec"}
		for _, ext := range exts {
			for _, sfx := range suffixes {
				m, _ := filepath.Glob(filepath.Join(dir, "*."+sfx+ext))
				found = append(found, m...)
			}
		}
		testFiles = found

	case "rust":
		// Tests are inline in Rust source files.
		testFiles = []string{absSource}

	case "csharp":
		var found []string
		csPatterns := []string{
			filepath.Join(dir, "*Test*.cs"),
			filepath.Join(dir, "*Tests.cs"),
		}
		for _, p := range csPatterns {
			m, _ := filepath.Glob(p)
			found = append(found, m...)
		}
		testFiles = found

	case "swift":
		m, _ := filepath.Glob(filepath.Join(dir, "*Tests.swift"))
		testFiles = m

	case "zig":
		// Zig tests are inline in source files, same as Rust.
		testFiles = []string{absSource}

	case "kotlin":
		var found []string
		ktPatterns := []string{
			filepath.Join(dir, "*Test.kt"),
			filepath.Join(dir, "*Tests.kt"),
		}
		for _, p := range ktPatterns {
			m, _ := filepath.Glob(p)
			found = append(found, m...)
		}
		testFiles = found

	default:
		testFiles = []string{}
	}

	if testFiles == nil {
		testFiles = []string{}
	}

	return TestFileResult{
		SourceFile: absSource,
		TestFiles:  testFiles,
	}, nil
}

// --- helpers ---

// resolveBuildPath returns the effective path argument for a build command.
func resolveBuildPath(lang, path string) string {
	if path != "" {
		return path
	}
	switch lang {
	case "go":
		return "./..."
	case "python":
		return "."
	default:
		return ""
	}
}

// resolveTestPath returns the effective path argument for a test command.
func resolveTestPath(lang, path string) string {
	if path != "" {
		return path
	}
	switch lang {
	case "go":
		return "./..."
	case "python":
		return "."
	default:
		return ""
	}
}

// applyPathArg replaces "{path}" placeholders in args with resolvedPath.
// If no placeholder exists and resolvedPath is non-empty, appends it.
func applyPathArg(templateArgs []string, resolvedPath string) []string {
	if resolvedPath == "" {
		// Remove any "{path}" placeholders.
		var out []string
		for _, a := range templateArgs {
			if a != "{path}" {
				out = append(out, a)
			}
		}
		return out
	}

	result := make([]string, len(templateArgs))
	replaced := false
	for i, a := range templateArgs {
		if a == "{path}" {
			result[i] = resolvedPath
			replaced = true
		} else {
			result[i] = a
		}
	}
	if !replaced && resolvedPath != "" {
		result = append(result, resolvedPath)
	}
	return result
}

// --- build output parsers ---
//
// Each language's build tool emits errors in a different format. These regexes
// extract structured file:line:column:message tuples from raw compiler output.
// The patterns match the most common error format for each toolchain:
//   - Go:         file.go:42:10: message
//   - TypeScript: file.ts(42,10): error TS2304: message
//   - Python:     file.py:42: error: message  (mypy format, no column)
//   - C#:         file.cs(42,10): error CS1234: message
//   - Swift/Zig:  file.swift:42:10: error: message
//   - Kotlin:     e: file.kt: (42, 10): error: message (Gradle format)
//
// Rust is a special case: errors span multiple lines with "error[E0123]:"
// on one line and "--> file:line:col" on a subsequent line.

var (
	reBuildGo         = regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s+(.+)$`)
	reBuildTypeScript = regexp.MustCompile(`^([^(]+)\((\d+),(\d+)\): error TS\d+: (.+)$`)
	reBuildPython     = regexp.MustCompile(`^([^:]+):(\d+):\s+error:\s+(.+)$`)
	reBuildCSharp     = regexp.MustCompile(`^([^(]+)\((\d+),(\d+)\): error [A-Z]+\d+: (.+)$`)
	reBuildSwift      = regexp.MustCompile(`^([^:]+):(\d+):(\d+): error: (.+)$`)
	reBuildZig        = regexp.MustCompile(`^([^:]+):(\d+):(\d+): error: (.+)$`)
	reBuildKotlin     = regexp.MustCompile(`^e: ([^:]+): \((\d+), (\d+)\): error: (.+)$`)
)

func parseBuildErrors(lang string, output []byte) []BuildError {
	var errors []BuildError
	scanner := bufio.NewScanner(bytes.NewReader(output))
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	switch lang {
	case "go":
		for _, line := range lines {
			m := reBuildGo.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lineNum, _ := strconv.Atoi(m[2])
			colNum, _ := strconv.Atoi(m[3])
			errors = append(errors, BuildError{
				File:    m[1],
				Line:    lineNum,
				Column:  colNum,
				Message: m[4],
			})
		}

	case "typescript":
		for _, line := range lines {
			m := reBuildTypeScript.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lineNum, _ := strconv.Atoi(m[2])
			colNum, _ := strconv.Atoi(m[3])
			errors = append(errors, BuildError{
				File:    strings.TrimSpace(m[1]),
				Line:    lineNum,
				Column:  colNum,
				Message: m[4],
			})
		}

	case "rust":
		// Rust: look for "error[" lines, then "--> file:line:col" on next line.
		for i, line := range lines {
			if !strings.HasPrefix(line, "error[") && !strings.HasPrefix(line, "error:") {
				continue
			}
			msg := ""
			if idx := strings.Index(line, "] "); idx >= 0 {
				msg = line[idx+2:]
			} else if idx := strings.Index(line, ": "); idx >= 0 {
				msg = line[idx+2:]
			}
			// Look for "--> file:line:col" on next few lines.
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				trimmed := strings.TrimSpace(lines[j])
				if strings.HasPrefix(trimmed, "--> ") {
					loc := trimmed[4:]
					parts := strings.Split(loc, ":")
					if len(parts) >= 3 {
						lineNum, _ := strconv.Atoi(parts[len(parts)-2])
						colNum, _ := strconv.Atoi(parts[len(parts)-1])
						filePart := strings.Join(parts[:len(parts)-2], ":")
						errors = append(errors, BuildError{
							File:    filePart,
							Line:    lineNum,
							Column:  colNum,
							Message: msg,
						})
					}
					break
				}
			}
		}

	case "python":
		for _, line := range lines {
			m := reBuildPython.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lineNum, _ := strconv.Atoi(m[2])
			errors = append(errors, BuildError{
				File:    m[1],
				Line:    lineNum,
				Column:  0,
				Message: m[3],
			})
		}

	case "csharp":
		for _, line := range lines {
			m := reBuildCSharp.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lineNum, _ := strconv.Atoi(m[2])
			colNum, _ := strconv.Atoi(m[3])
			errors = append(errors, BuildError{
				File:    strings.TrimSpace(m[1]),
				Line:    lineNum,
				Column:  colNum,
				Message: m[4],
			})
		}

	case "swift":
		for _, line := range lines {
			m := reBuildSwift.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lineNum, _ := strconv.Atoi(m[2])
			colNum, _ := strconv.Atoi(m[3])
			errors = append(errors, BuildError{
				File:    m[1],
				Line:    lineNum,
				Column:  colNum,
				Message: m[4],
			})
		}

	case "zig":
		for _, line := range lines {
			m := reBuildZig.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lineNum, _ := strconv.Atoi(m[2])
			colNum, _ := strconv.Atoi(m[3])
			errors = append(errors, BuildError{
				File:    m[1],
				Line:    lineNum,
				Column:  colNum,
				Message: m[4],
			})
		}

	case "kotlin":
		for _, line := range lines {
			m := reBuildKotlin.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lineNum, _ := strconv.Atoi(m[2])
			colNum, _ := strconv.Atoi(m[3])
			errors = append(errors, BuildError{
				File:    strings.TrimSpace(m[1]),
				Line:    lineNum,
				Column:  colNum,
				Message: m[4],
			})
		}
	}

	return errors
}

// --- test output parsers ---

// goTestEvent is a single JSON line from go test -json output.
type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// reGoTestFileLine matches "file.go:42:" in go test output lines.
var reGoTestFileLine = regexp.MustCompile(`([^:\s]+\.go):(\d+)`)

func parseTestFailures(lang, root string, output []byte) []TestFailure {
	var failures []TestFailure

	switch lang {
	case "go":
		failures = parseGoTestFailures(root, output)

	case "rust":
		failures = parseRustTestFailures(output)

	case "python":
		failures = parsePythonTestFailures(output)

	case "csharp":
		failures = parseDotnetTestFailures(output)

	case "swift":
		failures = parseSwiftTestFailures(output)

	case "zig":
		// zig build test exits non-zero if tests fail; output is plain text.
		// Minimal: return empty failures list (raw output preserved in TestResult.Raw).
		// No structured parsing needed.

	case "kotlin":
		failures = parseGradleTestFailures(output)

	default:
		// TypeScript/JavaScript: raw output only, no structured parsing.
	}

	return failures
}

func parseGoTestFailures(root string, output []byte) []TestFailure {
	var failures []TestFailure
	// Collect all output lines per test for file:line extraction.
	outputLines := make(map[string][]string)
	var failedTests []string

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Test == "" {
			continue
		}
		if ev.Action == "output" {
			outputLines[ev.Test] = append(outputLines[ev.Test], ev.Output)
		}
		if ev.Action == "fail" {
			failedTests = append(failedTests, ev.Test)
		}
	}

	for _, testName := range failedTests {
		tf := TestFailure{
			TestName: testName,
		}
		// Scan output lines for a file:line pattern.
		for _, ol := range outputLines[testName] {
			m := reGoTestFileLine.FindStringSubmatch(ol)
			if m != nil {
				lineNum, _ := strconv.Atoi(m[2])
				tf.File = m[1]
				tf.Line = lineNum
				tf.Message = strings.TrimSpace(ol)
				break
			}
		}
		// Collect all output as message if no file/line found.
		if tf.Message == "" && len(outputLines[testName]) > 0 {
			tf.Message = strings.Join(outputLines[testName], "")
		}
		normalizeTestFailureLocation(root, &tf)
		failures = append(failures, tf)
	}
	return failures
}

// rustTestEvent is a JSON line from cargo test --message-format=json.
type rustTestEvent struct {
	Type    string `json:"type"`
	Event   string `json:"event"`
	Name    string `json:"name"`
	Stdout  string `json:"stdout"`
}

func parseRustTestFailures(output []byte) []TestFailure {
	var failures []TestFailure
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		var ev rustTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type == "test" && ev.Event == "failed" {
			failures = append(failures, TestFailure{
				TestName: ev.Name,
				Message:  ev.Stdout,
			})
		}
	}
	return failures
}

// pytestJSONResult is the top-level pytest JSON output structure.
type pytestJSONResult struct {
	Tests []struct {
		NodeID   string `json:"nodeid"`
		Longrepr string `json:"longrepr"`
		Outcome  string `json:"outcome"`
	} `json:"tests"`
}

func parsePythonTestFailures(output []byte) []TestFailure {
	var failures []TestFailure

	// Try JSON parse first.
	var result pytestJSONResult
	if err := json.Unmarshal(output, &result); err == nil {
		for _, t := range result.Tests {
			if t.Outcome == "failed" {
				failures = append(failures, TestFailure{
					TestName: t.NodeID,
					Message:  t.Longrepr,
				})
			}
		}
		return failures
	}

	// Fallback: raw output, no structured parsing.
	return failures
}

// reSwiftTestFailed matches XCTest failure lines:
// "  Test Case '-[Suite testName]' failed (0.001 seconds)."
var reSwiftTestFailed = regexp.MustCompile(`Test Case '(.+)' failed`)

func parseSwiftTestFailures(output []byte) []TestFailure {
	var failures []TestFailure
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		m := reSwiftTestFailed.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		failures = append(failures, TestFailure{
			TestName: m[1],
			Message:  strings.TrimSpace(line),
		})
	}
	return failures
}

// reDotnetTestFailed matches dotnet test verbose output failure lines:
// "  Failed TestName [12 ms]"
var reDotnetTestFailed = regexp.MustCompile(`^\s+Failed\s+(\S+)\s+\[`)

func parseDotnetTestFailures(output []byte) []TestFailure {
	var failures []TestFailure
	scanner := bufio.NewScanner(bytes.NewReader(output))
	var currentTest string
	var collectMessage bool
	var msgLines []string
	for scanner.Scan() {
		line := scanner.Text()
		m := reDotnetTestFailed.FindStringSubmatch(line)
		if m != nil {
			if currentTest != "" {
				failures = append(failures, TestFailure{
					TestName: currentTest,
					Message:  strings.Join(msgLines, "\n"),
				})
			}
			currentTest = m[1]
			collectMessage = false
			msgLines = nil
			continue
		}
		if currentTest != "" && strings.Contains(line, "Error Message:") {
			collectMessage = true
			continue
		}
		if collectMessage && strings.TrimSpace(line) != "" {
			msgLines = append(msgLines, strings.TrimSpace(line))
			collectMessage = false
		}
	}
	if currentTest != "" {
		failures = append(failures, TestFailure{
			TestName: currentTest,
			Message:  strings.Join(msgLines, "\n"),
		})
	}
	return failures
}

// reGradleTestFailed matches Gradle test failure summary lines:
// "FAILED: ClassName > testName FAILED"  or
// "  com.example.TestClass > testName FAILED"
var reGradleTestFailed = regexp.MustCompile(`(\S+)\s+FAILED\s*$`)

func parseGradleTestFailures(output []byte) []TestFailure {
	var failures []TestFailure
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := reGradleTestFailed.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		failures = append(failures, TestFailure{
			TestName: m[1],
			Message:  line,
		})
	}
	return failures
}

// normalizeTestFailureLocation sets the Location field on a TestFailure
// using LSP URI format and 0-based line numbers.
func normalizeTestFailureLocation(root string, tf *TestFailure) {
	if tf.File == "" || tf.Line <= 0 {
		return
	}
	absPath, err := filepath.Abs(filepath.Join(root, tf.File))
	if err != nil {
		absPath = filepath.Join(root, tf.File)
	}
	tf.Location = &types.Location{
		URI: "file://" + absPath,
		Range: types.Range{
			Start: types.Position{Line: tf.Line - 1, Character: 0},
			End:   types.Position{Line: tf.Line - 1, Character: 0},
		},
	}
}
