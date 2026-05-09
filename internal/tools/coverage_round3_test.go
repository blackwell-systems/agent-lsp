package tools

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- extractPosition ---

func TestExtractPosition_Valid(t *testing.T) {
	args := map[string]interface{}{
		"line":   float64(10),
		"column": float64(5),
	}
	line, col, err := extractPosition(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 10 || col != 5 {
		t.Errorf("got line=%d col=%d, want 10, 5", line, col)
	}
}

func TestExtractPosition_MissingLine(t *testing.T) {
	args := map[string]interface{}{
		"column": float64(5),
	}
	_, _, err := extractPosition(args)
	if err == nil {
		t.Error("expected error for missing line")
	}
}

func TestExtractPosition_MissingColumn(t *testing.T) {
	args := map[string]interface{}{
		"line": float64(5),
	}
	_, _, err := extractPosition(args)
	if err == nil {
		t.Error("expected error for missing column")
	}
}

func TestExtractPosition_ZeroLine(t *testing.T) {
	args := map[string]interface{}{
		"line":   float64(0),
		"column": float64(1),
	}
	_, _, err := extractPosition(args)
	if err == nil {
		t.Error("expected error for line < 1")
	}
}

func TestExtractPosition_ZeroColumn(t *testing.T) {
	args := map[string]interface{}{
		"line":   float64(1),
		"column": float64(0),
	}
	_, _, err := extractPosition(args)
	if err == nil {
		t.Error("expected error for column < 1")
	}
}

func TestExtractPosition_IntTypes(t *testing.T) {
	args := map[string]interface{}{
		"line":   int(3),
		"column": int64(7),
	}
	line, col, err := extractPosition(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 3 || col != 7 {
		t.Errorf("got line=%d col=%d, want 3, 7", line, col)
	}
}

// --- extractRange ---

func TestExtractRange_Valid(t *testing.T) {
	args := map[string]interface{}{
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(5),
		"end_column":   float64(10),
	}
	rng, err := extractRange(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify 0-indexed conversion
	if rng.Start.Line != 0 || rng.Start.Character != 0 {
		t.Errorf("start: got %d:%d, want 0:0", rng.Start.Line, rng.Start.Character)
	}
	if rng.End.Line != 4 || rng.End.Character != 9 {
		t.Errorf("end: got %d:%d, want 4:9", rng.End.Line, rng.End.Character)
	}
}

func TestExtractRange_StartAfterEnd(t *testing.T) {
	args := map[string]interface{}{
		"start_line":   float64(10),
		"start_column": float64(1),
		"end_line":     float64(5),
		"end_column":   float64(1),
	}
	_, err := extractRange(args)
	if err == nil {
		t.Error("expected error for start after end")
	}
}

func TestExtractRange_SameLineBadColumn(t *testing.T) {
	args := map[string]interface{}{
		"start_line":   float64(5),
		"start_column": float64(20),
		"end_line":     float64(5),
		"end_column":   float64(10),
	}
	_, err := extractRange(args)
	if err == nil {
		t.Error("expected error for start column after end column on same line")
	}
}

func TestExtractRange_MissingStartLine(t *testing.T) {
	args := map[string]interface{}{
		"start_column": float64(1),
		"end_line":     float64(5),
		"end_column":   float64(1),
	}
	_, err := extractRange(args)
	if err == nil {
		t.Error("expected error for missing start_line")
	}
}

func TestExtractRange_ZeroEndLine(t *testing.T) {
	args := map[string]interface{}{
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(0),
		"end_column":   float64(1),
	}
	_, err := extractRange(args)
	if err == nil {
		t.Error("expected error for end_line < 1")
	}
}

// --- toInt ---

func TestToInt_Float64(t *testing.T) {
	args := map[string]interface{}{"n": float64(42)}
	v, err := toInt(args, "n")
	if err != nil || v != 42 {
		t.Errorf("got %d, %v; want 42, nil", v, err)
	}
}

func TestToInt_Int(t *testing.T) {
	args := map[string]interface{}{"n": int(7)}
	v, err := toInt(args, "n")
	if err != nil || v != 7 {
		t.Errorf("got %d, %v; want 7, nil", v, err)
	}
}

func TestToInt_Int64(t *testing.T) {
	args := map[string]interface{}{"n": int64(99)}
	v, err := toInt(args, "n")
	if err != nil || v != 99 {
		t.Errorf("got %d, %v; want 99, nil", v, err)
	}
}

func TestToInt_Missing(t *testing.T) {
	args := map[string]interface{}{}
	_, err := toInt(args, "n")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestToInt_WrongType(t *testing.T) {
	args := map[string]interface{}{"n": "notanumber"}
	_, err := toInt(args, "n")
	if err == nil {
		t.Error("expected error for string type")
	}
}

// --- toIntOpt ---

func TestToIntOpt_Present(t *testing.T) {
	args := map[string]interface{}{"n": float64(3)}
	v, ok := toIntOpt(args, "n")
	if !ok || v != 3 {
		t.Errorf("got %d, %v; want 3, true", v, ok)
	}
}

func TestToIntOpt_Missing(t *testing.T) {
	args := map[string]interface{}{}
	_, ok := toIntOpt(args, "n")
	if ok {
		t.Error("expected ok=false for missing key")
	}
}

// --- formatLocations ---

func TestFormatLocations_Empty(t *testing.T) {
	result, err := formatLocations(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestFormatLocations_ConvertPositions(t *testing.T) {
	locs := []types.Location{
		{
			URI: "file:///project/main.go",
			Range: types.Range{
				Start: types.Position{Line: 9, Character: 4},
				End:   types.Position{Line: 9, Character: 10},
			},
		},
	}
	result, err := formatLocations(locs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	r := result[0]
	if r.FilePath != "/project/main.go" {
		t.Errorf("FilePath=%q, want /project/main.go", r.FilePath)
	}
	// 0-indexed to 1-indexed
	if r.StartLine != 10 || r.StartCol != 5 {
		t.Errorf("start: got %d:%d, want 10:5", r.StartLine, r.StartCol)
	}
	if r.EndLine != 10 || r.EndCol != 11 {
		t.Errorf("end: got %d:%d, want 10:11", r.EndLine, r.EndCol)
	}
}

func TestFormatLocations_InvalidURI(t *testing.T) {
	locs := []types.Location{
		{
			URI:   "not-a-file-uri",
			Range: types.Range{},
		},
	}
	_, err := formatLocations(locs)
	if err == nil {
		t.Error("expected error for non-file URI")
	}
}

// --- locationsResult ---

func TestLocationsResult_Empty(t *testing.T) {
	result, err := locationsResult(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected no error for empty locations")
	}
}

func TestLocationsResult_InvalidURI(t *testing.T) {
	locs := []types.Location{
		{URI: "not-a-file-uri"},
	}
	result, err := locationsResult(locs)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid URI")
	}
}

// --- extractSignature ---

func TestExtractSignature_GoFunc(t *testing.T) {
	output := "package foo\n\nfunc Bar(x int) error\n    Bar does something.\n"
	sig := extractSignature("go", output)
	if sig != "func Bar(x int) error" {
		t.Errorf("got %q", sig)
	}
}

func TestExtractSignature_GoType(t *testing.T) {
	output := "type Config struct {\n    Name string\n}\n"
	sig := extractSignature("go", output)
	if sig != "type Config struct {" {
		t.Errorf("got %q", sig)
	}
}

func TestExtractSignature_GoVar(t *testing.T) {
	output := "var ErrNotFound = errors.New(\"not found\")\n"
	sig := extractSignature("go", output)
	if sig != "var ErrNotFound = errors.New(\"not found\")" {
		t.Errorf("got %q", sig)
	}
}

func TestExtractSignature_GoConst(t *testing.T) {
	output := "const MaxRetries = 3\n"
	sig := extractSignature("go", output)
	if sig != "const MaxRetries = 3" {
		t.Errorf("got %q", sig)
	}
}

func TestExtractSignature_GoNoMatch(t *testing.T) {
	output := "Some random documentation text\nwith no signature\n"
	sig := extractSignature("go", output)
	if sig != "" {
		t.Errorf("expected empty, got %q", sig)
	}
}

func TestExtractSignature_Python(t *testing.T) {
	output := "os.path.join(*paths)\n    Join path components.\n"
	sig := extractSignature("python", output)
	if sig != "os.path.join(*paths)" {
		t.Errorf("got %q", sig)
	}
}

func TestExtractSignature_PythonEmpty(t *testing.T) {
	sig := extractSignature("python", "")
	if sig != "" {
		t.Errorf("expected empty, got %q", sig)
	}
}

func TestExtractSignature_Rust(t *testing.T) {
	sig := extractSignature("rust", "some output")
	if sig != "" {
		t.Errorf("expected empty for rust, got %q", sig)
	}
}

func TestExtractSignature_UnknownLanguage(t *testing.T) {
	sig := extractSignature("java", "public void foo()")
	if sig != "" {
		t.Errorf("expected empty for unknown language, got %q", sig)
	}
}

// --- buildGoArgs ---

func TestBuildGoArgs_SymbolOnly(t *testing.T) {
	args := buildGoArgs("fmt.Println", "")
	if len(args) != 2 || args[0] != "doc" || args[1] != "fmt.Println" {
		t.Errorf("got %v", args)
	}
}

// --- toIntOptional ---

func TestToIntOptional_Missing(t *testing.T) {
	args := map[string]interface{}{}
	v, err := toIntOptional(args, "x")
	if err != nil || v != 0 {
		t.Errorf("got %d, %v; want 0, nil", v, err)
	}
}

func TestToIntOptional_Nil(t *testing.T) {
	args := map[string]interface{}{"x": nil}
	v, err := toIntOptional(args, "x")
	if err != nil || v != 0 {
		t.Errorf("got %d, %v; want 0, nil", v, err)
	}
}

func TestToIntOptional_Float64(t *testing.T) {
	args := map[string]interface{}{"x": float64(42)}
	v, err := toIntOptional(args, "x")
	if err != nil || v != 42 {
		t.Errorf("got %d, %v; want 42, nil", v, err)
	}
}

func TestToIntOptional_WrongType(t *testing.T) {
	args := map[string]interface{}{"x": "bad"}
	_, err := toIntOptional(args, "x")
	if err == nil {
		t.Error("expected error for string type")
	}
}

// --- resolveInContent ---

func TestResolveInContent_BasicMatch(t *testing.T) {
	content := "package main\n\nfunc Foo() {\n}\n"
	line, col, err := resolveInContent(content, "func @@Foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 3 {
		t.Errorf("line: got %d, want 3", line)
	}
	if col != 6 {
		t.Errorf("col: got %d, want 6", col)
	}
}

func TestResolveInContent_NotFound(t *testing.T) {
	_, _, err := resolveInContent("hello world", "@@missing")
	if err == nil {
		t.Error("expected error for pattern not found")
	}
}

func TestResolveInContent_AtStartOfFile(t *testing.T) {
	content := "package main\n"
	line, col, err := resolveInContent(content, "@@package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 1 || col != 1 {
		t.Errorf("got line=%d col=%d, want 1:1", line, col)
	}
}

// --- isTestFile ---

func TestIsTestFile_GoTest(t *testing.T) {
	if !isTestFile("/project/foo_test.go") {
		t.Error("expected _test.go to be recognized")
	}
}

func TestIsTestFile_JestSpec(t *testing.T) {
	if !isTestFile("/project/foo.spec.ts") {
		t.Error("expected .spec. to be recognized")
	}
}

func TestIsTestFile_JestTest(t *testing.T) {
	if !isTestFile("/project/foo.test.js") {
		t.Error("expected .test. to be recognized")
	}
}

func TestIsTestFile_PythonTest(t *testing.T) {
	if !isTestFile("/project/test_foo.py") {
		t.Error("expected test_ prefix to be recognized")
	}
}

func TestIsTestFile_NonTestFile(t *testing.T) {
	if isTestFile("/project/main.go") {
		t.Error("expected main.go to not be a test file")
	}
}

func TestIsTestFile_ContainsTestInName(t *testing.T) {
	// A file with "test" in its name but not matching patterns
	if isTestFile("/project/testing_utils.go") {
		t.Error("expected testing_utils.go to not match test file patterns")
	}
}

// --- repoForFile ---

func TestRepoForFile_MatchesFirst(t *testing.T) {
	roots := []string{"/repo/consumer-a", "/repo/consumer-b"}
	got := repoForFile("/repo/consumer-a/pkg/foo.go", roots)
	if got != "/repo/consumer-a" {
		t.Errorf("got %q, want /repo/consumer-a", got)
	}
}

func TestRepoForFile_NoMatch(t *testing.T) {
	roots := []string{"/repo/consumer-a"}
	got := repoForFile("/other/path/foo.go", roots)
	if got != "primary" {
		t.Errorf("got %q, want primary", got)
	}
}
