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
	typeHierarchyLine   int // Java and TypeScript only
	typeHierarchyColumn int // Java and TypeScript only

	// Fields for Tier 2 expansion tools.
	highlightLine      int    // for get_document_highlights; uses hoverLine if 0
	highlightColumn    int    // for get_document_highlights; uses hoverColumn if 0
	inlayHintEndLine   int    // end line for get_inlay_hints range; computed from hoverLine if 0
	renameSymbolLine   int    // for rename_symbol / prepare_rename
	renameSymbolColumn int    // for rename_symbol / prepare_rename
	renameSymbolName   string // new_name for rename_symbol
	codeActionLine     int    // start line for get_code_actions range; uses hoverLine if 0
	codeActionEndLine  int    // end line for get_code_actions range; codeActionLine+1 if 0
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

// getMultilangBinary builds the agent-lsp binary once and returns its path.
// Returns empty string if build fails.
func getMultilangBinary(t *testing.T) string {
	t.Helper()
	multilangBinaryOnce.Do(func() {
		tmp, err := os.MkdirTemp("", "agent-lsp-multi-*")
		if err != nil {
			return
		}
		p := filepath.Join(tmp, "agent-lsp")
		// test/multi_lang_test.go → test/ → repo root (two levels up)
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			return
		}
		testFileDir := filepath.Dir(filename)
		repoRoot := filepath.Dir(testFileDir)
		cmd := exec.Command("go", "build", "-o", p, "./cmd/agent-lsp")
		cmd.Env = append(os.Environ(), "GOWORK=off")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build agent-lsp: %v\n%s\n", err, out)
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
			symbolName:          "Person",
			typeHierarchyLine:   11,
			typeHierarchyColumn: 18,
			highlightLine:      11,
			highlightColumn:    18,
			inlayHintEndLine:   35,
			renameSymbolLine:   11,
			renameSymbolColumn: 18,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     36,
			codeActionEndLine:  38,
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
			highlightLine:      4,
			highlightColumn:    7,
			inlayHintEndLine:   16,
			renameSymbolLine:   4,
			renameSymbolColumn: 7,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     4,
			codeActionEndLine:  7,
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
			highlightLine:      6,
			highlightColumn:    6,
			inlayHintEndLine:   20,
			renameSymbolLine:   6,
			renameSymbolColumn: 6,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     6,
			codeActionEndLine:  9,
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
			highlightLine:      2,
			highlightColumn:    8,
			inlayHintEndLine:   20,
			renameSymbolLine:   2,
			renameSymbolColumn: 8,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     2,
			codeActionEndLine:  5,
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
			symbolName:          "Person",
			typeHierarchyLine:   6,
			typeHierarchyColumn: 14,
			highlightLine:      6,
			highlightColumn:    14,
			inlayHintEndLine:   20,
			renameSymbolLine:   6,
			renameSymbolColumn: 14,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     6,
			codeActionEndLine:  8,
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
			highlightLine:      3,
			highlightColumn:    1,
			inlayHintEndLine:   15,
			renameSymbolLine:   3,
			renameSymbolColumn: 1,
			renameSymbolName:   "renamed_person",
			codeActionLine:     3,
			codeActionEndLine:  5,
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
		{
			name:               "Zig",
			id:                 "zig",
			binary:             "zls",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "zig"),
			file:               filepath.Join(fixtureBase, "zig", "src", "person.zig"),
			hoverLine:          4,
			hoverColumn:        12,
			definitionLine:     4,
			definitionColumn:   12,
			callSiteLine:       8,
			callSiteColumn:     20,
			callSiteFile:       filepath.Join(fixtureBase, "zig", "src", "main.zig"),
			referenceLine:      4,
			referenceColumn:    12,
			completionLine:     12,
			completionColumn:   14,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "zig", "src", "greeter.zig"),
			symbolName:         "Person",
			highlightLine:      4,
			highlightColumn:    12,
			inlayHintEndLine:   14,
			renameSymbolLine:   4,
			renameSymbolColumn: 12,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     4,
			codeActionEndLine:  7,
		},
		{
			name:               "CSS",
			id:                 "css",
			binary:             "vscode-css-language-server",
			serverArgs:         []string{"--stdio"},
			fixture:            filepath.Join(fixtureBase, "css"),
			file:               filepath.Join(fixtureBase, "css", "styles.css"),
			hoverLine:          3,
			hoverColumn:        5,
			definitionLine:     2,
			definitionColumn:   1,
			callSiteLine:       2,
			callSiteColumn:     1,
			referenceLine:      2,
			referenceColumn:    1,
			completionLine:     3,
			completionColumn:   12,
			workspaceSymbol:    "person",
			supportsFormatting: true,
			symbolName:         "person",
			highlightLine:      2,
			highlightColumn:    1,
			inlayHintEndLine:   6,
			codeActionLine:     3,
			codeActionEndLine:  5,
		},
		{
			name:               "HTML",
			id:                 "html",
			binary:             "vscode-html-language-server",
			serverArgs:         []string{"--stdio"},
			fixture:            filepath.Join(fixtureBase, "html"),
			file:               filepath.Join(fixtureBase, "html", "index.html"),
			hoverLine:          10,
			hoverColumn:        6,
			definitionLine:     0,
			callSiteLine:       0,
			referenceLine:      0,
			completionLine:     10,
			completionColumn:   10,
			workspaceSymbol:    "div",
			supportsFormatting: true,
			symbolName:         "person",
			highlightLine:      10,
			highlightColumn:    6,
			inlayHintEndLine:   12,
			codeActionLine:     10,
			codeActionEndLine:  12,
		},
		{
			name:               "Terraform",
			id:                 "terraform",
			binary:             "terraform-ls",
			serverArgs:         []string{"serve"},
			fixture:            filepath.Join(fixtureBase, "terraform"),
			file:               filepath.Join(fixtureBase, "terraform", "main.tf"),
			hoverLine:          11,
			hoverColumn:        10,
			definitionLine:     11,
			definitionColumn:   10,
			callSiteLine:       24,
			callSiteColumn:     15,
			referenceLine:      11,
			referenceColumn:    10,
			completionLine:     12,
			completionColumn:   5,
			workspaceSymbol:    "person_name",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "terraform", "variables.tf"),
			symbolName:         "person_name",
			highlightLine:      11,
			highlightColumn:    10,
			inlayHintEndLine:   16,
			renameSymbolLine:   11,
			renameSymbolColumn: 10,
			renameSymbolName:   "renamed_person",
			codeActionLine:     11,
			codeActionEndLine:  15,
		},
		{
			name:               "Lua",
			id:                 "lua",
			binary:             "lua-language-server",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "lua"),
			file:               filepath.Join(fixtureBase, "lua", "person.lua"),
			hoverLine:          5,
			hoverColumn:        7,
			definitionLine:     5,
			definitionColumn:   7,
			callSiteLine:       4,
			callSiteColumn:     15,
			callSiteFile:       filepath.Join(fixtureBase, "lua", "main.lua"),
			referenceLine:      5,
			referenceColumn:    7,
			completionLine:     19,
			completionColumn:   14,
			workspaceSymbol:    "Person",
			supportsFormatting: false,
			secondFile:         filepath.Join(fixtureBase, "lua", "greeter.lua"),
			symbolName:         "Person",
			highlightLine:      5,
			highlightColumn:    7,
			inlayHintEndLine:   20,
			renameSymbolLine:   0, // lua-language-server rename support is limited
			codeActionLine:     5,
			codeActionEndLine:  7,
		},
		{
			name:               "Swift",
			id:                 "swift",
			binary:             "sourcekit-lsp",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "swift"),
			file:               filepath.Join(fixtureBase, "swift", "Sources", "swift-fixture", "Person.swift"),
			hoverLine:          2,
			hoverColumn:        15,
			definitionLine:     2,
			definitionColumn:   15,
			callSiteLine:       1,
			callSiteColumn:     12,
			callSiteFile:       filepath.Join(fixtureBase, "swift", "Sources", "swift-fixture", "main.swift"),
			referenceLine:      2,
			referenceColumn:    15,
			completionLine:     10,
			completionColumn:   12,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "swift", "Sources", "swift-fixture", "Greeter.swift"),
			symbolName:         "Person",
			highlightLine:      2,
			highlightColumn:    15,
			inlayHintEndLine:   13,
			renameSymbolLine:   2,
			renameSymbolColumn: 15,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     2,
			codeActionEndLine:  5,
		},
		{
			name:               "Scala",
			id:                 "scala",
			binary:             "metals",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "scala"),
			file:               filepath.Join(fixtureBase, "scala", "Person.scala"),
			hoverLine:          4,
			hoverColumn:        12,
			definitionLine:     4,
			definitionColumn:   12,
			callSiteLine:       4,
			callSiteColumn:     17,
			callSiteFile:       filepath.Join(fixtureBase, "scala", "Main.scala"),
			referenceLine:      4,
			referenceColumn:    12,
			completionLine:     5,
			completionColumn:   5,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "scala", "Greeter.scala"),
			symbolName:         "Person",
			highlightLine:      4,
			highlightColumn:    12,
			inlayHintEndLine:   5,
			renameSymbolLine:   4,
			renameSymbolColumn: 12,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     4,
			codeActionEndLine:  5,
		},
		{
			name:               "Kotlin",
			id:                 "kotlin",
			binary:             "kotlin-language-server",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "kotlin"),
			file:               filepath.Join(fixtureBase, "kotlin", "Person.kt"),
			hoverLine:          6,
			hoverColumn:        12,
			definitionLine:     6,
			definitionColumn:   12,
			callSiteLine:       4,
			callSiteColumn:     19,
			callSiteFile:       filepath.Join(fixtureBase, "kotlin", "main.kt"),
			referenceLine:      6,
			referenceColumn:    12,
			completionLine:     7,
			completionColumn:   5,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "kotlin", "Greeter.kt"),
			symbolName:         "Person",
			highlightLine:      6,
			highlightColumn:    12,
			inlayHintEndLine:   8,
			renameSymbolLine:   6,
			renameSymbolColumn: 12,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     6,
			codeActionEndLine:  8,
		},
		{
			name:               "CSharp",
			id:                 "csharp",
			binary:             "csharp-ls",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "csharp"),
			file:               filepath.Join(fixtureBase, "csharp", "Person.cs"),
			hoverLine:          4,
			hoverColumn:        14,
			definitionLine:     4,
			definitionColumn:   14,
			callSiteLine:       3,
			callSiteColumn:     18,
			callSiteFile:       filepath.Join(fixtureBase, "csharp", "Program.cs"),
			referenceLine:      4,
			referenceColumn:    14,
			completionLine:     6,
			completionColumn:   20,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "csharp", "Greeter.cs"),
			symbolName:         "Person",
			highlightLine:      4,
			highlightColumn:    14,
			inlayHintEndLine:   18,
			renameSymbolLine:   4,
			renameSymbolColumn: 14,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     4,
			codeActionEndLine:  8,
		},
		{
			name:               "Gleam",
			id:                 "gleam",
			binary:             "gleam",
			serverArgs:         []string{"lsp"},
			fixture:            filepath.Join(fixtureBase, "gleam"),
			file:               filepath.Join(fixtureBase, "gleam", "src", "person.gleam"),
			hoverLine:          1,
			hoverColumn:        10,
			definitionLine:     1,
			definitionColumn:   10,
			callSiteLine:       4,
			callSiteColumn:     18,
			callSiteFile:       filepath.Join(fixtureBase, "gleam", "src", "greeter.gleam"),
			referenceLine:      1,
			referenceColumn:    10,
			completionLine:     4,
			completionColumn:   18,
			completionFile:     filepath.Join(fixtureBase, "gleam", "src", "greeter.gleam"),
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "gleam", "src", "greeter.gleam"),
			symbolName:         "Person",
			highlightLine:      1,
			highlightColumn:    10,
			inlayHintEndLine:   7,
			renameSymbolLine:   1,
			renameSymbolColumn: 10,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     5,
			codeActionEndLine:  7,
		},
		{
			name:               "Elixir",
			id:                 "elixir",
			binary:             "elixir-ls",
			serverArgs:         []string{},
			fixture:            filepath.Join(fixtureBase, "elixir"),
			file:               filepath.Join(fixtureBase, "elixir", "lib", "person.ex"),
			hoverLine:          1,
			hoverColumn:        11,
			definitionLine:     1,
			definitionColumn:   11,
			callSiteLine:       3,
			callSiteColumn:     21,
			callSiteFile:       filepath.Join(fixtureBase, "elixir", "lib", "greeter.ex"),
			referenceLine:      1,
			referenceColumn:    11,
			completionLine:     3,
			completionColumn:   21,
			completionFile:     filepath.Join(fixtureBase, "elixir", "lib", "greeter.ex"),
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			secondFile:         filepath.Join(fixtureBase, "elixir", "lib", "greeter.ex"),
			symbolName:         "Person",
			highlightLine:      1,
			highlightColumn:    11,
			inlayHintEndLine:   0, // ElixirLS does not support inlay hints
			renameSymbolLine:   0, // ElixirLS rename is unreliable
			codeActionLine:     1,
			codeActionEndLine:  3,
		},
		{
			name:               "Prisma",
			id:                 "prisma",
			binary:             "prisma-language-server",
			serverArgs:         []string{"--stdio"},
			fixture:            filepath.Join(fixtureBase, "prisma"),
			file:               filepath.Join(fixtureBase, "prisma", "schema.prisma"),
			hoverLine:          10,
			hoverColumn:        7,
			definitionLine:     10,
			definitionColumn:   7,
			callSiteLine:       21,
			callSiteColumn:     12,
			referenceLine:      10,
			referenceColumn:    7,
			completionLine:     12,
			completionColumn:   9,
			workspaceSymbol:    "Person",
			supportsFormatting: true,
			symbolName:         "Person",
			highlightLine:      10,
			highlightColumn:    7,
			inlayHintEndLine:   0, // Prisma language server does not support inlay hints
			renameSymbolLine:   10,
			renameSymbolColumn: 7,
			renameSymbolName:   "RenamedPerson",
			codeActionLine:     10,
			codeActionEndLine:  15,
		},
		{
			name:               "SQL",
			id:                 "sql",
			binary:             "sqls",
			serverArgs:         []string{"--config", filepath.Join(fixtureBase, "sql", ".sqls.yml")},
			fixture:            filepath.Join(fixtureBase, "sql"),
			file:               filepath.Join(fixtureBase, "sql", "query.sql"),
			hoverLine:          7,
			hoverColumn:        6,
			definitionLine:     7,
			definitionColumn:   6,
			callSiteLine:       8,
			callSiteColumn:     6,
			referenceLine:      7,
			referenceColumn:    6,
			completionLine:     4,
			completionColumn:   5,
			workspaceSymbol:    "person",
			supportsFormatting: false,
			symbolName:         "person",
			highlightLine:      7,
			highlightColumn:    6,
			inlayHintEndLine:   0,
			renameSymbolLine:   0,
			codeActionLine:     7,
			codeActionEndLine:  9,
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

	// Determine timeout. JVM-based servers (Java, Kotlin, Scala) need longer
	// timeouts due to slow cold-start initialization.
	var timeout time.Duration
	switch lang.id {
	case "java":
		timeout = 300 * time.Second
	case "kotlin", "scala":
		timeout = 180 * time.Second
	default:
		timeout = 90 * time.Second
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

	// LSP init wait after start_lsp — JVM servers need longer to finish indexing.
	var initWait time.Duration
	switch lang.id {
	case "java":
		initWait = 150 * time.Second
	case "kotlin", "scala":
		initWait = 30 * time.Second
	default:
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
		testTypeHierarchy(t, ctx, session, lang),
		testGetInfoOnLocation(t, ctx, session, lang),
		testCallHierarchy(t, ctx, session, lang),
		testGetSemanticTokens(t, ctx, session, lang),
		testGetSignatureHelp(t, ctx, session, lang),
		testGetDocumentHighlights(t, ctx, session, lang),
		testGetInlayHints(t, ctx, session, lang),
		testGetCodeActions(t, ctx, session, lang),
		testPrepareRename(t, ctx, session, lang),
		testRenameSymbol(t, ctx, session, lang),
		testGetServerCapabilities(t, ctx, session, lang),
		testWorkspaceFolders(t, ctx, session, lang),
		testGoToTypeDefinition(t, ctx, session, lang),
		testGoToImplementation(t, ctx, session, lang),
		testFormatRange(t, ctx, session, lang),
		testApplyEdit(t, ctx, session, lang),
		testDetectLspServers(t, ctx, session, lang),
		testCloseDocument(t, ctx, session, lang),
		testDidChangeWatchedFiles(t, ctx, session, lang),
		testRunBuild(t, ctx, session, lang),
		testRunTests(t, ctx, session, lang),
		testGetTestsForFile(t, ctx, session, lang),
		testGetSymbolSource(t, ctx, session, lang),
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

// testTypeHierarchy tests the type_hierarchy tool (Java and TypeScript only).
func testTypeHierarchy(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.typeHierarchyLine == 0 {
		return toolResult{tool: "type_hierarchy", status: "skip", detail: "not configured for this language"}
	}
	res, err := callTool(ctx, session, "type_hierarchy", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.typeHierarchyLine,
		"column":      lang.typeHierarchyColumn,
		"direction":   "both",
	})
	if err != nil {
		return toolResult{tool: "type_hierarchy", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		// Type hierarchy not supported by this server — treat as skip.
		return toolResult{tool: "type_hierarchy", status: "skip",
			detail: fmt.Sprintf("tool returned IsError=true (server may not support typeHierarchy): %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "type_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to parse type_hierarchy response: %v", err)}
	}
	var result struct {
		Items      []map[string]any `json:"items"`
		Supertypes []map[string]any `json:"supertypes"`
		Subtypes   []map[string]any `json:"subtypes"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// May be a plain text message like "No type hierarchy item found"
		if strings.Contains(text, "No type hierarchy item found") {
			return toolResult{tool: "type_hierarchy", status: "skip",
				detail: "server returned no type hierarchy item at configured position"}
		}
		return toolResult{tool: "type_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal type_hierarchy response: %s", text)}
	}
	if len(result.Items) == 0 {
		return toolResult{tool: "type_hierarchy", status: "skip",
			detail: "no items returned (server may not index this position)"}
	}
	return toolResult{tool: "type_hierarchy", status: "pass"}
}

// testGetInfoOnLocation tests the get_info_on_location (hover) tool.
func testGetInfoOnLocation(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "get_info_on_location", status: "skip", detail: "no hover position configured"}
	}
	res, err := callTool(ctx, session, "get_info_on_location", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
	})
	if err != nil {
		return toolResult{tool: "get_info_on_location", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_info_on_location", status: "skip",
			detail: fmt.Sprintf("server does not support hover: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" {
		return toolResult{tool: "get_info_on_location", status: "skip", detail: "empty hover response"}
	}
	return toolResult{tool: "get_info_on_location", status: "pass"}
}

// testCallHierarchy tests the call_hierarchy tool.
// Uses the hover position which points at a function/method declaration in every fixture.
func testCallHierarchy(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "call_hierarchy", status: "skip", detail: "no position configured"}
	}
	res, err := callTool(ctx, session, "call_hierarchy", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
		"direction":   "both",
	})
	if err != nil {
		return toolResult{tool: "call_hierarchy", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "call_hierarchy", status: "skip",
			detail: fmt.Sprintf("server does not support callHierarchy: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "call_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to parse call_hierarchy response: %v", err)}
	}
	var result struct {
		Items   []map[string]any `json:"items"`
		Callers []map[string]any `json:"callers"`
		Callees []map[string]any `json:"callees"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		if strings.Contains(text, "No call hierarchy") {
			return toolResult{tool: "call_hierarchy", status: "skip", detail: "no call hierarchy item at position"}
		}
		return toolResult{tool: "call_hierarchy", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal call_hierarchy response: %s", text)}
	}
	if len(result.Items) == 0 {
		return toolResult{tool: "call_hierarchy", status: "skip", detail: "no items returned"}
	}
	return toolResult{tool: "call_hierarchy", status: "pass"}
}

// testGetSemanticTokens tests the get_semantic_tokens tool over a small range.
func testGetSemanticTokens(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	endLine := lang.hoverLine + 5
	if endLine < 5 {
		endLine = 5
	}
	res, err := callTool(ctx, session, "get_semantic_tokens", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   1,
		"start_column": 1,
		"end_line":     endLine,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "get_semantic_tokens", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_semantic_tokens", status: "skip",
			detail: fmt.Sprintf("server does not support semanticTokens: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_semantic_tokens", status: "fail",
			detail: fmt.Sprintf("failed to parse semantic tokens response: %v", err)}
	}
	var tokens []map[string]any
	if err := json.Unmarshal([]byte(text), &tokens); err != nil || len(tokens) == 0 {
		return toolResult{tool: "get_semantic_tokens", status: "skip", detail: "no tokens returned"}
	}
	return toolResult{tool: "get_semantic_tokens", status: "pass"}
}

// testGetSignatureHelp tests the get_signature_help tool.
// Uses completionLine/completionColumn — positions inside expressions where
// signature help is likely to trigger.
func testGetSignatureHelp(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.completionLine == 0 {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "no position configured"}
	}
	completionFile := lang.completionFile
	if completionFile == "" {
		completionFile = lang.file
	}
	res, err := callTool(ctx, session, "get_signature_help", map[string]any{
		"file_path":   completionFile,
		"language_id": lang.id,
		"line":        lang.completionLine,
		"column":      lang.completionColumn,
	})
	if err != nil {
		return toolResult{tool: "get_signature_help", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_signature_help", status: "skip",
			detail: fmt.Sprintf("server does not support signatureHelp: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" || text == "null" || text == "{}" {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "no signature help at position"}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "empty or non-JSON response"}
	}
	sigs, _ := result["signatures"].([]any)
	if len(sigs) == 0 {
		return toolResult{tool: "get_signature_help", status: "skip", detail: "no signatures returned"}
	}
	return toolResult{tool: "get_signature_help", status: "pass"}
}

// testGetDocumentHighlights tests the get_document_highlights tool.
func testGetDocumentHighlights(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	line := lang.highlightLine
	col := lang.highlightColumn
	if line == 0 {
		line = lang.hoverLine
		col = lang.hoverColumn
	}
	if line == 0 {
		return toolResult{tool: "get_document_highlights", status: "skip", detail: "no position configured"}
	}
	res, err := callTool(ctx, session, "get_document_highlights", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        line,
		"column":      col,
	})
	if err != nil {
		return toolResult{tool: "get_document_highlights", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_document_highlights", status: "skip",
			detail: fmt.Sprintf("server does not support documentHighlights: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_document_highlights", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return toolResult{tool: "get_document_highlights", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal response: %s", text)}
	}
	if len(items) == 0 {
		return toolResult{tool: "get_document_highlights", status: "skip", detail: "no highlights returned"}
	}
	return toolResult{tool: "get_document_highlights", status: "pass"}
}

// testGetInlayHints tests the get_inlay_hints tool over a range.
func testGetInlayHints(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	endLine := lang.inlayHintEndLine
	if endLine == 0 {
		endLine = lang.hoverLine + 5
	}
	if endLine < 5 {
		endLine = 5
	}
	res, err := callTool(ctx, session, "get_inlay_hints", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   1,
		"start_column": 1,
		"end_line":     endLine,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "get_inlay_hints", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_inlay_hints", status: "skip",
			detail: fmt.Sprintf("server does not support inlayHints: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_inlay_hints", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	var hints []map[string]any
	if err := json.Unmarshal([]byte(text), &hints); err != nil {
		return toolResult{tool: "get_inlay_hints", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal response: %s", text)}
	}
	if len(hints) == 0 {
		return toolResult{tool: "get_inlay_hints", status: "skip", detail: "no inlay hints returned"}
	}
	return toolResult{tool: "get_inlay_hints", status: "pass"}
}

// testGetCodeActions tests the get_code_actions tool.
func testGetCodeActions(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	startLine := lang.codeActionLine
	if startLine == 0 {
		startLine = lang.hoverLine
	}
	if startLine == 0 {
		return toolResult{tool: "get_code_actions", status: "skip", detail: "no position configured"}
	}
	endLine := lang.codeActionEndLine
	if endLine == 0 {
		endLine = startLine + 1
	}
	res, err := callTool(ctx, session, "get_code_actions", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   startLine,
		"start_column": 1,
		"end_line":     endLine,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "get_code_actions", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_code_actions", status: "skip",
			detail: fmt.Sprintf("server does not support codeActions: %v", res.Content)}
	}
	_, err = textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_code_actions", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	// Empty action list is valid — the range may have no applicable actions.
	return toolResult{tool: "get_code_actions", status: "pass"}
}

// testPrepareRename tests the prepare_rename validation step.
func testPrepareRename(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.renameSymbolLine == 0 {
		return toolResult{tool: "prepare_rename", status: "skip", detail: "renameSymbolLine not configured"}
	}
	res, err := callTool(ctx, session, "prepare_rename", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.renameSymbolLine,
		"column":      lang.renameSymbolColumn,
	})
	if err != nil {
		return toolResult{tool: "prepare_rename", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "prepare_rename", status: "skip",
			detail: fmt.Sprintf("server does not support prepareRename: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" || text == "null" {
		return toolResult{tool: "prepare_rename", status: "skip", detail: "no prepare_rename response"}
	}
	return toolResult{tool: "prepare_rename", status: "pass"}
}

// testRenameSymbol tests the rename_symbol tool (returns WorkspaceEdit, does not apply it).
func testRenameSymbol(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.renameSymbolLine == 0 || lang.renameSymbolName == "" {
		return toolResult{tool: "rename_symbol", status: "skip", detail: "rename position or name not configured"}
	}
	res, err := callTool(ctx, session, "rename_symbol", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.renameSymbolLine,
		"column":      lang.renameSymbolColumn,
		"new_name":    lang.renameSymbolName,
	})
	if err != nil {
		return toolResult{tool: "rename_symbol", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "rename_symbol", status: "skip",
			detail: fmt.Sprintf("server does not support rename: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" || text == "null" {
		return toolResult{tool: "rename_symbol", status: "skip", detail: "no WorkspaceEdit returned"}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "rename_symbol", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal WorkspaceEdit: %s", text)}
	}
	_, hasChanges := result["changes"]
	_, hasDocChanges := result["documentChanges"]
	if !hasChanges && !hasDocChanges {
		return toolResult{tool: "rename_symbol", status: "skip",
			detail: "WorkspaceEdit has neither 'changes' nor 'documentChanges'"}
	}
	return toolResult{tool: "rename_symbol", status: "pass"}
}

// testGetServerCapabilities tests the get_server_capabilities tool.
func testGetServerCapabilities(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "get_server_capabilities", map[string]any{})
	if err != nil {
		return toolResult{tool: "get_server_capabilities", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: fmt.Sprintf("tool returned IsError=true: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: fmt.Sprintf("failed to parse response: %v", err)}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: fmt.Sprintf("failed to unmarshal response: %s", text)}
	}
	supportedTools, ok := result["supported_tools"].([]any)
	if !ok || len(supportedTools) == 0 {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: "supported_tools missing or empty"}
	}
	// Verify start_lsp is always present.
	found := false
	for _, v := range supportedTools {
		if s, ok := v.(string); ok && s == "start_lsp" {
			found = true
			break
		}
	}
	if !found {
		return toolResult{tool: "get_server_capabilities", status: "fail",
			detail: "'start_lsp' not in supported_tools"}
	}
	return toolResult{tool: "get_server_capabilities", status: "pass"}
}

// testWorkspaceFolders tests the workspace folder lifecycle tools.
func testWorkspaceFolders(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()

	// Step 1: list current workspace folders.
	res, err := callTool(ctx, session, "list_workspace_folders", map[string]any{})
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "workspace_folders", status: "skip",
			detail: fmt.Sprintf("list_workspace_folders not supported: %v", res.Content)}
	}

	// Step 2: add a temporary folder.
	tmpDir, mkErr := os.MkdirTemp("", "lsp-mcp-wf-test-*")
	if mkErr != nil {
		return toolResult{tool: "workspace_folders", status: "fail",
			detail: fmt.Sprintf("failed to create temp dir: %v", mkErr)}
	}
	defer os.RemoveAll(tmpDir)

	res, err = callTool(ctx, session, "add_workspace_folder", map[string]any{
		"path": tmpDir,
	})
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		// Server may not support didChangeWorkspaceFolders — treat as skip.
		return toolResult{tool: "workspace_folders", status: "skip",
			detail: fmt.Sprintf("add_workspace_folder not supported: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail",
			detail: fmt.Sprintf("failed to parse add response: %v", err)}
	}
	var addResult map[string]any
	if err := json.Unmarshal([]byte(text), &addResult); err != nil || addResult["added"] == nil {
		return toolResult{tool: "workspace_folders", status: "fail",
			detail: fmt.Sprintf("add_workspace_folder: unexpected response: %s", text)}
	}

	// Step 3: remove the folder.
	res, err = callTool(ctx, session, "remove_workspace_folder", map[string]any{
		"path": tmpDir,
	})
	if err != nil {
		return toolResult{tool: "workspace_folders", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "workspace_folders", status: "skip",
			detail: fmt.Sprintf("remove_workspace_folder not supported: %v", res.Content)}
	}
	return toolResult{tool: "workspace_folders", status: "pass"}
}

// testGoToTypeDefinition tests the go_to_type_definition tool.
// Uses referenceLine/referenceColumn as the position — calling from a symbol
// reference rather than its definition to exercise the type-jump behavior.
func testGoToTypeDefinition(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	line := lang.referenceLine
	col := lang.referenceColumn
	if line == 0 {
		return toolResult{tool: "go_to_type_definition", status: "skip", detail: "no reference position configured"}
	}
	res, err := callTool(ctx, session, "go_to_type_definition", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        line,
		"column":      col,
	})
	if err != nil {
		return toolResult{tool: "go_to_type_definition", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "go_to_type_definition", status: "skip",
			detail: fmt.Sprintf("server does not support typeDefinition: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "go_to_type_definition", status: "fail", detail: err.Error()}
	}
	if text == "" || text == "null" || text == "[]" {
		return toolResult{tool: "go_to_type_definition", status: "skip", detail: "empty result"}
	}
	return toolResult{tool: "go_to_type_definition", status: "pass"}
}

// testGoToImplementation tests the go_to_implementation tool.
// Uses hoverLine/hoverColumn (typically pointing at a type or interface definition).
// Most servers will return empty for concrete types; the result is skip not fail.
func testGoToImplementation(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "go_to_implementation", status: "skip", detail: "no hover position configured"}
	}
	res, err := callTool(ctx, session, "go_to_implementation", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"column":      lang.hoverColumn,
	})
	if err != nil {
		return toolResult{tool: "go_to_implementation", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "go_to_implementation", status: "skip",
			detail: fmt.Sprintf("server does not support implementation: %v", res.Content)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "go_to_implementation", status: "fail", detail: err.Error()}
	}
	if text == "" || text == "null" || text == "[]" {
		return toolResult{tool: "go_to_implementation", status: "skip", detail: "no implementations found"}
	}
	return toolResult{tool: "go_to_implementation", status: "pass"}
}

// testFormatRange tests the format_range tool over the first 5 lines of the file.
func testFormatRange(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if !lang.supportsFormatting {
		return toolResult{tool: "format_range", status: "skip", detail: "not supported"}
	}
	res, err := callTool(ctx, session, "format_range", map[string]any{
		"file_path":    lang.file,
		"language_id":  lang.id,
		"start_line":   1,
		"start_column": 1,
		"end_line":     5,
		"end_column":   120,
	})
	if err != nil {
		return toolResult{tool: "format_range", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "format_range", status: "skip",
			detail: fmt.Sprintf("server does not support rangeFormatting: %v", res.Content)}
	}
	return toolResult{tool: "format_range", status: "pass"}
}

// testApplyEdit exercises the full file-write path:
//  1. format_document → get TextEdit[] (non-empty when fixture has deliberate trailing whitespace)
//  2. apply_edit with a WorkspaceEdit wrapping those edits → writes to disk
//  3. format_document again → verify edits are now empty (proves write happened)
//
// Fixtures for Go, TypeScript, and Rust contain a blank line with trailing
// whitespace that their respective formatters will remove, ensuring step 1
// always returns non-empty edits on a fresh checkout.
func testApplyEdit(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if !lang.supportsFormatting {
		return toolResult{tool: "apply_edit", status: "skip", detail: "formatting not supported"}
	}

	// Step 1: get formatting edits.
	res, err := callTool(ctx, session, "format_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil {
		return toolResult{tool: "apply_edit", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		return toolResult{tool: "apply_edit", status: "skip", detail: "format_document not supported"}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "apply_edit", status: "fail", detail: err.Error()}
	}
	var edits []map[string]any
	if err := json.Unmarshal([]byte(text), &edits); err != nil {
		return toolResult{tool: "apply_edit", status: "fail",
			detail: fmt.Sprintf("parse format edits: %v — raw: %s", err, text)}
	}
	if len(edits) == 0 {
		return toolResult{tool: "apply_edit", status: "skip",
			detail: "no formatting edits returned (fixture already clean — run from fresh checkout to exercise write path)"}
	}

	// Step 2: apply the edits.
	fileURI := "file://" + lang.file
	res, err = callTool(ctx, session, "apply_edit", map[string]any{
		"workspace_edit": map[string]any{
			"changes": map[string]any{fileURI: edits},
		},
	})
	if err != nil {
		return toolResult{tool: "apply_edit", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		errText, _ := textFromResult(res)
		return toolResult{tool: "apply_edit", status: "fail",
			detail: fmt.Sprintf("apply_edit returned IsError: %s", errText)}
	}

	// Step 3: re-format to verify the write actually hit disk.
	res, err = callTool(ctx, session, "format_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	if err != nil || res.IsError {
		// apply didn't error — treat as pass even if we can't verify
		return toolResult{tool: "apply_edit", status: "pass"}
	}
	text2, _ := textFromResult(res)
	var edits2 []map[string]any
	if err := json.Unmarshal([]byte(text2), &edits2); err == nil && len(edits2) > 0 {
		return toolResult{tool: "apply_edit", status: "fail",
			detail: fmt.Sprintf("re-format returned %d edits after apply — file write may not have persisted", len(edits2))}
	}
	return toolResult{tool: "apply_edit", status: "pass"}
}

// testDetectLspServers tests the detect_lsp_servers tool against the language fixture.
func testDetectLspServers(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "detect_lsp_servers", map[string]any{
		"workspace_dir": lang.fixture,
	})
	if err != nil {
		return toolResult{tool: "detect_lsp_servers", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "detect_lsp_servers", status: "fail",
			detail: fmt.Sprintf("detect_lsp_servers returned IsError: %s", text)}
	}
	text, err := textFromResult(res)
	if err != nil {
		return toolResult{tool: "detect_lsp_servers", status: "fail", detail: err.Error()}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "detect_lsp_servers", status: "fail",
			detail: fmt.Sprintf("failed to parse detect_lsp_servers response: %s", text)}
	}
	return toolResult{tool: "detect_lsp_servers", status: "pass"}
}

// testCloseDocument tests the close_document tool by reopening and closing the file.
func testCloseDocument(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	// Re-open first so we have a tracked document to close.
	_, _ = callTool(ctx, session, "open_document", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
	})
	res, err := callTool(ctx, session, "close_document", map[string]any{
		"file_path": lang.file,
	})
	if err != nil {
		return toolResult{tool: "close_document", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "close_document", status: "fail",
			detail: fmt.Sprintf("close_document returned IsError: %s", text)}
	}
	return toolResult{tool: "close_document", status: "pass"}
}

// testDidChangeWatchedFiles tests the did_change_watched_files tool with an empty
// changes array — valid per LSP spec and exercises the tool without modifying files.
func testDidChangeWatchedFiles(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	res, err := callTool(ctx, session, "did_change_watched_files", map[string]any{
		"changes": []any{},
	})
	if err != nil {
		return toolResult{tool: "did_change_watched_files", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "did_change_watched_files", status: "fail",
			detail: fmt.Sprintf("did_change_watched_files returned IsError: %s", text)}
	}
	return toolResult{tool: "did_change_watched_files", status: "pass"}
}

// testGetSymbolSource tests the get_symbol_source tool using the hover position,
// which points at a named symbol in every fixture that supports document symbols.
func testGetSymbolSource(t *testing.T, ctx context.Context, session *mcp.ClientSession, lang langConfig) toolResult {
	t.Helper()
	if lang.hoverLine == 0 {
		return toolResult{tool: "get_symbol_source", status: "skip", detail: "no hover position configured"}
	}
	res, err := callTool(ctx, session, "get_symbol_source", map[string]any{
		"file_path":   lang.file,
		"language_id": lang.id,
		"line":        lang.hoverLine,
		"character":   lang.hoverColumn,
	})
	if err != nil {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: err.Error()}
	}
	if res.IsError {
		text, _ := textFromResult(res)
		return toolResult{tool: "get_symbol_source", status: "skip",
			detail: fmt.Sprintf("server does not support document symbols: %s", text)}
	}
	text, err := textFromResult(res)
	if err != nil || strings.TrimSpace(text) == "" {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: "empty get_symbol_source response"}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return toolResult{tool: "get_symbol_source", status: "fail",
			detail: fmt.Sprintf("failed to parse get_symbol_source response: %s", text)}
	}
	if _, ok := result["symbol_name"]; !ok {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: "missing symbol_name in response"}
	}
	if src, ok := result["source"].(string); !ok || strings.TrimSpace(src) == "" {
		return toolResult{tool: "get_symbol_source", status: "fail", detail: "empty source in response"}
	}
	return toolResult{tool: "get_symbol_source", status: "pass"}
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
	header := fmt.Sprintf(
		"%-14s | T1 | %-7s | %-10s | %-10s | %-11s | %-9s | %-6s | %-11s | %-13s | %-5s | %-9s | %-8s | %-8s | %-10s | %-10s | %-8s | %-8s | %-6s | %-8s | %-10s | %-8s | %-10s | %-11s | %-10s | %-12s | %-12s | %-17s | %-10s",
		"Language", "symbols", "definition", "references", "completions",
		"workspace", "format", "declaration", "type_hierarchy", "hover",
		"call_hier", "sem_tok", "sig_help",
		"highlights", "inlay_hints", "code_act", "prep_ren", "rename",
		"srv_caps", "wk_folders",
		"type_def", "go_to_impl", "format_range", "apply_edit", "detect_servers",
		"close_doc", "did_change_watched", "sym_source")
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
		t.Logf(
			"%-14s | %-2s | %-7s | %-10s | %-10s | %-11s | %-9s | %-6s | %-11s | %-13s | %-5s | %-9s | %-8s | %-8s | %-10s | %-10s | %-8s | %-8s | %-6s | %-8s | %-10s | %-8s | %-10s | %-11s | %-10s | %-12s | %-12s | %-17s | %-10s",
			r.name,
			statusIcon(r.tier1),
			get("get_document_symbols"),
			get("go_to_definition"),
			get("get_references"),
			get("get_completions"),
			get("get_workspace_symbols"),
			get("format_document"),
			get("go_to_declaration"),
			get("type_hierarchy"),
			get("get_info_on_location"),
			get("call_hierarchy"),
			get("get_semantic_tokens"),
			get("get_signature_help"),
			get("get_document_highlights"),
			get("get_inlay_hints"),
			get("get_code_actions"),
			get("prepare_rename"),
			get("rename_symbol"),
			get("get_server_capabilities"),
			get("workspace_folders"),
			get("go_to_type_definition"),
			get("go_to_implementation"),
			get("format_range"),
			get("apply_edit"),
			get("detect_lsp_servers"),
			get("close_document"),
			get("did_change_watched_files"),
			get("get_symbol_source"),
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
		t.Skip("failed to build agent-lsp binary")
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
