package tools

import "github.com/blackwell-systems/agent-lsp/internal/types"

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

// languageRunner maps a language ID to its build and test commands.
type languageRunner struct {
	buildCmd  string
	buildArgs []string // template args; "{path}" is replaced at runtime
	testCmd   string
	testArgs  []string // template args; "{path}" is replaced at runtime
}

// runners is the dispatch table for language-specific build and test commands.
var runners = map[string]languageRunner{
	"go": {
		buildCmd:  "go",
		buildArgs: []string{"build", "{path}"},
		testCmd:   "go",
		testArgs:  []string{"test", "-json", "{path}"},
	},
	"typescript": {
		buildCmd:  "tsc",
		buildArgs: []string{"--noEmit"},
		testCmd:   "npm",
		testArgs:  []string{"test"},
	},
	"javascript": {
		buildCmd:  "eslint",
		buildArgs: []string{"."},
		testCmd:   "npm",
		testArgs:  []string{"test"},
	},
	"python": {
		buildCmd:  "mypy",
		buildArgs: []string{"{path}"},
		testCmd:   "pytest",
		testArgs:  []string{"--tb=json", "-q", "{path}"},
	},
	"rust": {
		buildCmd:  "cargo",
		buildArgs: []string{"build"},
		testCmd:   "cargo",
		testArgs:  []string{"test", "--message-format=json"},
	},
	"csharp": {
		buildCmd:  "dotnet",
		buildArgs: []string{"build"},
		testCmd:   "dotnet",
		testArgs:  []string{"test", "--logger", "console;verbosity=detailed"},
	},
	"swift": {
		buildCmd:  "swift",
		buildArgs: []string{"build"},
		testCmd:   "swift",
		testArgs:  []string{"test"},
	},
	"zig": {
		buildCmd:  "zig",
		buildArgs: []string{"build"},
		testCmd:   "zig",
		testArgs:  []string{"build", "test"},
	},
	"kotlin": {
		buildCmd:  "gradle",
		buildArgs: []string{"build", "--quiet"},
		testCmd:   "gradle",
		testArgs:  []string{"test", "--quiet"},
	},
}
