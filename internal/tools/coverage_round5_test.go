package tools

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- extractRange: start-after-end and same-position edge cases ---

func TestExtractRange_StartAfterEnd_SameLine(t *testing.T) {
	args := map[string]any{
		"start_line":   float64(5),
		"start_column": float64(10),
		"end_line":     float64(5),
		"end_column":   float64(3),
	}
	_, err := extractRange(args)
	if err == nil {
		t.Error("expected error when start column > end column on same line")
	}
}

func TestExtractRange_StartAfterEnd_DifferentLines(t *testing.T) {
	args := map[string]any{
		"start_line":   float64(10),
		"start_column": float64(1),
		"end_line":     float64(5),
		"end_column":   float64(1),
	}
	_, err := extractRange(args)
	if err == nil {
		t.Error("expected error when start line > end line")
	}
}

func TestExtractRange_ZeroWidthRange(t *testing.T) {
	args := map[string]any{
		"start_line":   float64(3),
		"start_column": float64(7),
		"end_line":     float64(3),
		"end_column":   float64(7),
	}
	r, err := extractRange(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start != r.End {
		t.Errorf("expected same start and end for zero-width range")
	}
}

func TestExtractRange_NonNumericTypes(t *testing.T) {
	args := map[string]any{
		"start_line":   "not-a-number",
		"start_column": float64(1),
		"end_line":     float64(2),
		"end_column":   float64(1),
	}
	_, err := extractRange(args)
	if err == nil {
		t.Error("expected error for non-numeric start_line")
	}
}

// --- toInt: nil and bool variants ---

func TestToInt_NilValue(t *testing.T) {
	args := map[string]any{"val": nil}
	_, err := toInt(args, "val")
	if err == nil {
		t.Error("expected error for nil value")
	}
}

func TestToInt_BoolValue(t *testing.T) {
	args := map[string]any{"val": true}
	_, err := toInt(args, "val")
	if err == nil {
		t.Error("expected error for bool value")
	}
}

// --- symbolPaginationWindow: comprehensive edge cases ---

func TestSymbolPaginationWindow_ExactFit(t *testing.T) {
	// limit equals remaining items
	start, end, p := symbolPaginationWindow(5, 2, 3)
	if p == nil {
		t.Fatal("expected non-nil pagination")
	}
	if start != 2 || end != 5 {
		t.Errorf("got start=%d end=%d, want 2, 5", start, end)
	}
	if p.More {
		t.Error("More should be false when window reaches end")
	}
}

func TestSymbolPaginationWindow_OffsetEqualsTotal(t *testing.T) {
	_, _, p := symbolPaginationWindow(5, 5, 3)
	if p != nil {
		t.Error("expected nil pagination when offset == total")
	}
}

// --- bestSymbolMatch: dotted path with test file preference ---

func TestBestSymbolMatch_DottedPath_PrefersNonTest(t *testing.T) {
	parent := "Svc"
	candidates := []types.SymbolInformation{
		{Name: "Do", ContainerName: &parent, Location: types.Location{URI: "file:///svc_test.go"}},
		{Name: "Do", ContainerName: &parent, Location: types.Location{URI: "file:///svc.go"}},
	}
	result := bestSymbolMatch(candidates, "Svc.Do")
	if result == nil || result.Location.URI != "file:///svc.go" {
		t.Errorf("expected non-test file, got %v", result)
	}
}

func TestBestSymbolMatch_DottedPath_OnlyTestFiles(t *testing.T) {
	parent := "Svc"
	candidates := []types.SymbolInformation{
		{Name: "Do", ContainerName: &parent, Location: types.Location{URI: "file:///svc_test.go"}},
	}
	result := bestSymbolMatch(candidates, "Svc.Do")
	if result == nil || result.Location.URI != "file:///svc_test.go" {
		t.Errorf("should fallback to test file when no non-test exists")
	}
}

func TestBestSymbolMatch_NonDotted_OnlyTestFiles(t *testing.T) {
	candidates := []types.SymbolInformation{
		{Name: "Func", Location: types.Location{URI: "file:///foo_test.go"}},
	}
	result := bestSymbolMatch(candidates, "Func")
	if result == nil || result.Name != "Func" {
		t.Error("should match in test file when no non-test exists")
	}
}

// --- HandleSetLogLevel edge cases ---

func TestHandleSetLogLevel_NonStringLevel(t *testing.T) {
	args := map[string]any{"level": 42}
	result, err := HandleSetLogLevel(nil, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for non-string level")
	}
}

// --- renderOutline: deep nesting ---

func TestRenderOutline_DeepNesting(t *testing.T) {
	symbols := []types.DocumentSymbol{
		{
			Name:  "L0",
			Kind:  5, // Class
			Range: types.Range{Start: types.Position{Line: 1}},
			Children: []types.DocumentSymbol{
				{
					Name:  "L1",
					Kind:  6, // Method
					Range: types.Range{Start: types.Position{Line: 3}},
					Children: []types.DocumentSymbol{
						{
							Name:  "L2",
							Kind:  13, // Variable
							Range: types.Range{Start: types.Position{Line: 5}},
						},
					},
				},
			},
		},
	}
	got := renderOutline(symbols, 0)
	if !stringContains(got, "L0 [Class] :1") {
		t.Errorf("missing L0 in output:\n%s", got)
	}
	if !stringContains(got, "  L1 [Method] :3") {
		t.Errorf("missing indented L1 in output:\n%s", got)
	}
	if !stringContains(got, "    L2 [Variable] :5") {
		t.Errorf("missing double-indented L2 in output:\n%s", got)
	}
}

// --- shiftDocumentSymbol coverage ---

func TestShiftDocumentSymbol_WithChildren(t *testing.T) {
	sym := types.DocumentSymbol{
		Name: "Parent",
		Range: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 10, Character: 5},
		},
		SelectionRange: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 0, Character: 6},
		},
		Children: []types.DocumentSymbol{
			{
				Name: "Child",
				Range: types.Range{
					Start: types.Position{Line: 2, Character: 4},
					End:   types.Position{Line: 5, Character: 3},
				},
				SelectionRange: types.Range{
					Start: types.Position{Line: 2, Character: 4},
					End:   types.Position{Line: 2, Character: 9},
				},
			},
		},
	}

	shifted := shiftDocumentSymbol(sym)
	// Parent range should be 1-indexed
	if shifted.Range.Start.Line != 1 || shifted.Range.Start.Character != 1 {
		t.Errorf("parent start: got (%d,%d), want (1,1)", shifted.Range.Start.Line, shifted.Range.Start.Character)
	}
	if shifted.Range.End.Line != 11 || shifted.Range.End.Character != 6 {
		t.Errorf("parent end: got (%d,%d), want (11,6)", shifted.Range.End.Line, shifted.Range.End.Character)
	}
	// Child should also be shifted
	child := shifted.Children[0]
	if child.Range.Start.Line != 3 || child.Range.Start.Character != 5 {
		t.Errorf("child start: got (%d,%d), want (3,5)", child.Range.Start.Line, child.Range.Start.Character)
	}
}

// --- symbolKindName unknown kinds ---

func TestSymbolKindName_AllUnknown(t *testing.T) {
	unknowns := []int{0, 15, 16, 17, 18, 19, 20, 21, 24, 25, 27, 100}
	for _, k := range unknowns {
		got := symbolKindName(k)
		if got == "" {
			t.Errorf("symbolKindName(%d) returned empty string", k)
		}
		if !stringContains(got, "Kind") {
			t.Errorf("symbolKindName(%d) = %q, expected KindN format", k, got)
		}
	}
}

// --- appendHint: no content items ---

func TestAppendHint_NoContent(t *testing.T) {
	r := types.ToolResult{Content: nil}
	result := appendHint(r, "hint")
	if len(result.Content) != 0 {
		t.Error("should not add hint when content slice is nil/empty")
	}
}

// --- ParseScopePaths: additional edge case ---

func TestParseScopePaths_SliceWithAllEmpty(t *testing.T) {
	raw := []any{"", "", ""}
	paths := ParseScopePaths(raw)
	if len(paths) != 0 {
		t.Errorf("expected 0 paths for all-empty slice, got %v", paths)
	}
}

func TestParseScopePaths_SingleItemSlice(t *testing.T) {
	raw := []any{"src/main"}
	paths := ParseScopePaths(raw)
	if len(paths) != 1 || paths[0] != "src/main" {
		t.Errorf("got %v, want [src/main]", paths)
	}
}

// --- utf16Offset edge cases ---

func TestUtf16Offset_MultiByteBMP(t *testing.T) {
	// U+00E9 (e with acute) is 2 bytes in UTF-8 but 1 UTF-16 code unit
	line := "caf\u00E9"
	// byte offsets: c=0, a=1, f=2, é=3..4
	got := utf16Offset(line, 5) // full string
	if got != 4 {
		t.Errorf("utf16Offset for BMP multibyte = %d, want 4", got)
	}
}

func TestUtf16Offset_SurrogatePairCounting(t *testing.T) {
	// Two emoji characters (each 4 bytes UTF-8, 2 UTF-16 code units)
	line := "\U0001F600\U0001F601"
	// Full string is 8 bytes, 4 UTF-16 code units
	got := utf16Offset(line, 8)
	if got != 4 {
		t.Errorf("utf16Offset for two emoji = %d, want 4", got)
	}
}

// --- resolveInContent: multiline and pattern at end ---

func TestResolveInContent_PatternAtEndOfFile(t *testing.T) {
	content := "line1\nline2\ntarget"
	line, col, err := resolveInContent(content, "@@target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 3 {
		t.Errorf("line = %d, want 3", line)
	}
	if col != 1 {
		t.Errorf("col = %d, want 1", col)
	}
}

func TestResolveInContent_MarkerInMiddle(t *testing.T) {
	content := "func handleRequest(ctx context.Context) error {"
	line, col, err := resolveInContent(content, "func @@handleRequest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 1 {
		t.Errorf("line = %d, want 1", line)
	}
	if col != 6 {
		t.Errorf("col = %d, want 6", col)
	}
}

func TestResolveInContent_EmptyPrefix(t *testing.T) {
	content := "hello"
	line, col, err := resolveInContent(content, "@@hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 1 || col != 1 {
		t.Errorf("got (%d,%d), want (1,1)", line, col)
	}
}

func TestResolveInContent_EmptySuffix(t *testing.T) {
	content := "hello world"
	line, col, err := resolveInContent(content, "hello @@")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 1 || col != 7 {
		t.Errorf("got (%d,%d), want (1,7)", line, col)
	}
}

// helper
func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
