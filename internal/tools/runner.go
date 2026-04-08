package tools

import "github.com/blackwell-systems/lsp-mcp-go/internal/types"

// BuildError is a single compilation error or diagnostic returned by run_build.
type BuildError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// BuildResult is the structured output of run_build.
type BuildResult struct {
	Success bool         `json:"success"`
	Errors  []BuildError `json:"errors"`
	Raw     string       `json:"raw"`
}

// TestFailure is a single test failure entry returned by run_tests.
// Location is an LSP-normalized location (file URI + range) for interop
// with go_to_definition, get_references, and other LSP tools.
type TestFailure struct {
	File     string          `json:"file"`
	Line     int             `json:"line"`
	TestName string          `json:"test_name"`
	Message  string          `json:"message"`
	Location *types.Location `json:"location,omitempty"`
}

// TestResult is the structured output of run_tests.
type TestResult struct {
	Passed   bool          `json:"passed"`
	Failures []TestFailure `json:"failures"`
	Raw      string        `json:"raw"`
}

// TestFileResult is the structured output of get_tests_for_file.
type TestFileResult struct {
	SourceFile string   `json:"source_file"`
	TestFiles  []string `json:"test_files"`
}
