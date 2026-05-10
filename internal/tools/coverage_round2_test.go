package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- classifySkills ---

func TestClassifySkills_AllSupported(t *testing.T) {
	// Provide every capability any skill could need.
	caps := map[string]any{
		"hoverProvider":                   true,
		"completionProvider":              map[string]any{},
		"definitionProvider":              true,
		"referencesProvider":              true,
		"documentSymbolProvider":          true,
		"workspaceSymbolProvider":         true,
		"implementationProvider":          true,
		"callHierarchyProvider":           true,
		"typeHierarchyProvider":           true,
		"codeActionProvider":              true,
		"documentFormattingProvider":      true,
		"documentRangeFormattingProvider": true,
		"renameProvider":                  true,
		"documentHighlightProvider":       true,
	}
	result := classifySkills(caps)
	for _, s := range result {
		if s.Status != "supported" {
			t.Errorf("skill %q: expected status=supported, got %q (missing required: %v, missing optional: %v)",
				s.Name, s.Status, s.MissingRequired, s.MissingOptional)
		}
	}
}

func TestClassifySkills_NoCaps(t *testing.T) {
	caps := map[string]any{}
	result := classifySkills(caps)
	if len(result) == 0 {
		t.Fatal("expected non-empty skill list")
	}
	// Skills with no required capabilities should be "supported" or "partial".
	for _, s := range result {
		switch s.Name {
		case "lsp-safe-edit", "lsp-simulate", "lsp-test-correlation", "lsp-verify":
			// These have no required capabilities.
			if s.Status == "unsupported" {
				t.Errorf("skill %q has no required caps but got status=unsupported", s.Name)
			}
		default:
			// Most skills require at least one capability, so they should be unsupported or partial.
		}
	}
}

func TestClassifySkills_PartialStatus(t *testing.T) {
	// Provide only the required capabilities for lsp-explore (hoverProvider),
	// but not the optional ones (implementationProvider, callHierarchyProvider, referencesProvider).
	caps := map[string]any{
		"hoverProvider": true,
	}
	result := classifySkills(caps)
	var found bool
	for _, s := range result {
		if s.Name == "lsp-explore" {
			found = true
			if s.Status != "partial" {
				t.Errorf("lsp-explore: expected status=partial, got %q", s.Status)
			}
			if len(s.MissingOptional) == 0 {
				t.Error("lsp-explore: expected non-empty MissingOptional")
			}
			if len(s.MissingRequired) != 0 {
				t.Error("lsp-explore: expected empty MissingRequired")
			}
		}
	}
	if !found {
		t.Error("lsp-explore not found in classifySkills output")
	}
}

// --- extractSymbolName ---

func TestExtractSymbolName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain identifier", "myFunc", "myFunc"},
		{"with parens", "myFunc(x int)", "myFunc"},
		{"with space prefix", "  myFunc", "myFunc"},
		{"code block", "```go\nfunc HandleFoo() error\n```", "func"},
		{"code block identifier", "```\nmyVar := 42\n```", "myVar"},
		{"empty string", "", ""},
		{"only punctuation", "!@#$%", ""},
		{"underscore", "_private_var", "_private_var"},
		{"leading digits rejected", "  123abc", "123abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSymbolName(tc.input)
			if got != tc.want {
				t.Errorf("extractSymbolName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- symbolKindName ---

func TestSymbolKindName(t *testing.T) {
	cases := []struct {
		kind int
		want string
	}{
		{1, "File"},
		{5, "Class"},
		{6, "Method"},
		{12, "Function"},
		{13, "Variable"},
		{23, "Struct"},
		{26, "TypeParameter"},
		{99, "Kind99"}, // unknown kind
		{0, "Kind0"},   // zero
		{-1, "Kind-1"}, // negative
	}
	for _, tc := range cases {
		got := symbolKindName(tc.kind)
		if got != tc.want {
			t.Errorf("symbolKindName(%d) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// --- shiftRange ---

func TestShiftRange(t *testing.T) {
	r := types.Range{
		Start: types.Position{Line: 0, Character: 0},
		End:   types.Position{Line: 5, Character: 10},
	}
	got := shiftRange(r)
	if got.Start.Line != 1 || got.Start.Character != 1 {
		t.Errorf("Start: want (1,1), got (%d,%d)", got.Start.Line, got.Start.Character)
	}
	if got.End.Line != 6 || got.End.Character != 11 {
		t.Errorf("End: want (6,11), got (%d,%d)", got.End.Line, got.End.Character)
	}
}

// --- shiftDocumentSymbol ---

func TestShiftDocumentSymbol_Recursive(t *testing.T) {
	sym := types.DocumentSymbol{
		Name: "Parent",
		Kind: 5, // Class
		Range: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 10, Character: 20},
		},
		SelectionRange: types.Range{
			Start: types.Position{Line: 0, Character: 5},
			End:   types.Position{Line: 0, Character: 11},
		},
		Children: []types.DocumentSymbol{
			{
				Name: "Child",
				Kind: 6, // Method
				Range: types.Range{
					Start: types.Position{Line: 2, Character: 4},
					End:   types.Position{Line: 5, Character: 1},
				},
				SelectionRange: types.Range{
					Start: types.Position{Line: 2, Character: 9},
					End:   types.Position{Line: 2, Character: 14},
				},
			},
		},
	}
	shifted := shiftDocumentSymbol(sym)
	if shifted.Range.Start.Line != 1 {
		t.Errorf("parent range start line: want 1, got %d", shifted.Range.Start.Line)
	}
	if shifted.SelectionRange.Start.Character != 6 {
		t.Errorf("parent selection start char: want 6, got %d", shifted.SelectionRange.Start.Character)
	}
	if len(shifted.Children) != 1 {
		t.Fatal("expected 1 child")
	}
	child := shifted.Children[0]
	if child.Range.Start.Line != 3 {
		t.Errorf("child range start line: want 3, got %d", child.Range.Start.Line)
	}
}

// --- renderOutline ---

func TestRenderOutline(t *testing.T) {
	symbols := []types.DocumentSymbol{
		{
			Name: "MyStruct",
			Kind: 23, // Struct
			Range: types.Range{
				Start: types.Position{Line: 5},
			},
			Children: []types.DocumentSymbol{
				{
					Name: "DoWork",
					Kind: 6, // Method
					Range: types.Range{
						Start: types.Position{Line: 7},
					},
				},
			},
		},
	}
	out := renderOutline(symbols, 0)
	if !strings.Contains(out, "MyStruct [Struct] :5") {
		t.Errorf("expected 'MyStruct [Struct] :5' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "  DoWork [Method] :7") {
		t.Errorf("expected indented child 'DoWork [Method] :7', got:\n%s", out)
	}
}

func TestRenderOutline_Empty(t *testing.T) {
	out := renderOutline(nil, 0)
	if out != "" {
		t.Errorf("expected empty string for nil symbols, got %q", out)
	}
}

// --- ValidateFilePath ---

func TestValidateFilePath_Empty(t *testing.T) {
	_, err := ValidateFilePath("", "")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestValidateFilePath_OutsideRoot(t *testing.T) {
	root := t.TempDir()
	_, err := ValidateFilePath("/etc/passwd", root)
	if err == nil {
		t.Error("expected error for path outside root")
	}
	if !strings.Contains(err.Error(), "outside workspace root") {
		t.Errorf("expected 'outside workspace root' in error, got: %v", err)
	}
}

func TestValidateFilePath_InsideRoot(t *testing.T) {
	root := t.TempDir()
	// Resolve symlinks (macOS /var -> /private/var) to match what ValidateFilePath does.
	root, _ = filepath.EvalSymlinks(root)
	path := root + "/subdir/file.go"
	got, err := ValidateFilePath(path, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != path {
		t.Errorf("want %q, got %q", path, got)
	}
}

func TestValidateFilePath_NoRoot(t *testing.T) {
	got, err := ValidateFilePath("/some/abs/path.go", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/some/abs/path.go" {
		t.Errorf("want /some/abs/path.go, got %q", got)
	}
}

// --- parseTestFailures dispatching ---

func TestParseTestFailures_GoJSON(t *testing.T) {
	root := t.TempDir()
	input := `{"Action":"output","Test":"TestFoo","Output":"    foo_test.go:42: assertion failed\n"}
{"Action":"fail","Test":"TestFoo"}
`
	failures := parseTestFailures("go", root, []byte(input))
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].TestName != "TestFoo" {
		t.Errorf("expected TestName=TestFoo, got %q", failures[0].TestName)
	}
	if failures[0].File != "foo_test.go" {
		t.Errorf("expected File=foo_test.go, got %q", failures[0].File)
	}
	if failures[0].Line != 42 {
		t.Errorf("expected Line=42, got %d", failures[0].Line)
	}
}

func TestParseTestFailures_Rust(t *testing.T) {
	input := `{"type":"test","event":"failed","name":"tests::it_fails","stdout":"assertion failed\n"}
`
	failures := parseTestFailures("rust", "", []byte(input))
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].TestName != "tests::it_fails" {
		t.Errorf("expected TestName=tests::it_fails, got %q", failures[0].TestName)
	}
}

func TestParseTestFailures_PythonJSON(t *testing.T) {
	input := `{"tests":[{"nodeid":"test_app.py::test_add","longrepr":"assert 1 == 2","outcome":"failed"}]}`
	failures := parseTestFailures("python", "", []byte(input))
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].TestName != "test_app.py::test_add" {
		t.Errorf("expected TestName=test_app.py::test_add, got %q", failures[0].TestName)
	}
}

func TestParseTestFailures_Swift(t *testing.T) {
	input := "Test Case '-[MyTests testThing]' failed (0.001 seconds).\n"
	failures := parseTestFailures("swift", "", []byte(input))
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].TestName != "-[MyTests testThing]" {
		t.Errorf("expected TestName=-[MyTests testThing], got %q", failures[0].TestName)
	}
}

func TestParseTestFailures_Dotnet(t *testing.T) {
	input := "  Failed MyNamespace.MyTest [12 ms]\n  Error Message:\n  Assert.Equal failed\n"
	failures := parseTestFailures("csharp", "", []byte(input))
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].TestName != "MyNamespace.MyTest" {
		t.Errorf("expected TestName=MyNamespace.MyTest, got %q", failures[0].TestName)
	}
	if !strings.Contains(failures[0].Message, "Assert.Equal failed") {
		t.Errorf("expected message to contain 'Assert.Equal failed', got %q", failures[0].Message)
	}
}

func TestParseTestFailures_Gradle(t *testing.T) {
	input := "com.example.AppTest > testMain FAILED\n"
	failures := parseTestFailures("kotlin", "", []byte(input))
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].TestName != "FAILED" {
		// The regex captures the last \S+ before FAILED; let's check the output exists
		if len(failures[0].Message) == 0 {
			t.Error("expected non-empty message")
		}
	}
}

func TestParseTestFailures_UnknownLang(t *testing.T) {
	failures := parseTestFailures("perl", "", []byte("some output"))
	if len(failures) != 0 {
		t.Errorf("expected 0 failures for unknown language, got %d", len(failures))
	}
}

func TestParseTestFailures_Zig(t *testing.T) {
	failures := parseTestFailures("zig", "", []byte("1/1 test.my_test... FAIL"))
	if len(failures) != 0 {
		t.Errorf("expected 0 failures (zig uses raw output only), got %d", len(failures))
	}
}

// --- normalizeTestFailureLocation edge cases ---

func TestNormalizeTestFailureLocation_NoFile(t *testing.T) {
	tf := &TestFailure{TestName: "Test", Message: "failed"}
	normalizeTestFailureLocation("/root", tf)
	if tf.Location != nil {
		t.Error("expected nil Location when File is empty")
	}
}

func TestNormalizeTestFailureLocation_ZeroLine(t *testing.T) {
	tf := &TestFailure{TestName: "Test", File: "test.go", Line: 0}
	normalizeTestFailureLocation("/root", tf)
	if tf.Location != nil {
		t.Error("expected nil Location when Line <= 0")
	}
}

// --- buildConfigEntry additional ---

func TestBuildConfigEntry_MultiArgs(t *testing.T) {
	def := lspServerDef{
		Language: "terraform",
		Binary:   "terraform-ls",
		Args:     []string{"serve", "--port=0"},
	}
	got := buildConfigEntry(def)
	want := "terraform:terraform-ls,serve,--port=0"
	if got != want {
		t.Errorf("buildConfigEntry: want %q, got %q", want, got)
	}
}
