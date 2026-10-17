// position_pattern_double_marker_test.go — regression tests for
// davalillo/agent-lsp#13: the double-marker form "@@text@@" never matched
// because SplitN(n=2) left the closing marker inside the search text, and a
// leading UTF-8 BOM inflated line-1 columns.
package tools

import (
	"strings"
	"testing"
)

func TestResolvePositionPattern_DoubleMarker(t *testing.T) {
	content := "func CalculaRi(int tipoOrden) {\n\treturn 0\n}"
	path := writeTemp(t, content)

	line, col, err := ResolvePositionPattern(path, "@@CalculaRi@@")
	if err != nil {
		t.Fatalf("double-marker form must resolve: %v", err)
	}
	if line != 1 {
		t.Errorf("got line %d, want 1", line)
	}
	// Cursor at end of match: "func CalculaRi" is 14 chars → col 15.
	if col != 15 {
		t.Errorf("got col %d, want 15 (end of match)", col)
	}
}

func TestResolvePositionPattern_DoubleMarker_LongerPattern(t *testing.T) {
	// The exact pattern shape from the incident report.
	content := "double CalculaRi(int tipoOrden) {\n"
	path := writeTemp(t, content)

	if _, _, err := ResolvePositionPattern(path, "@@double CalculaRi(int tipoOrden@@"); err != nil {
		t.Fatalf("longer double-marker pattern must resolve: %v", err)
	}
}

func TestResolvePositionPattern_ThreeForms_Agree(t *testing.T) {
	content := "func CalculaRi(int tipoOrden) {"
	path := writeTemp(t, content)

	// All three forms must find the text. Single-marker forms put the cursor
	// where @@ sits; the double-marker form puts it at the end of the match.
	l1, c1, err1 := ResolvePositionPattern(path, "@@CalculaRi")
	l2, c2, err2 := ResolvePositionPattern(path, "CalculaRi@@")
	l3, c3, err3 := ResolvePositionPattern(path, "@@CalculaRi@@")
	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("forms: %v / %v / %v", err1, err2, err3)
	}
	if l1 != l2 || l1 != l3 || l1 != 1 {
		t.Errorf("lines disagree: %d / %d / %d", l1, l2, l3)
	}
	// "@@CalculaRi": prefix "" → cursor at match start (col 6, 1-indexed);
	// "CalculaRi@@" and "@@CalculaRi@@": cursor at end of match (col 15).
	if c1 != 6 || c2 != 15 || c3 != 15 {
		t.Errorf("cols: got %d/%d/%d, want 6/15/15", c1, c2, c3)
	}
}

func TestResolvePositionPattern_AmbiguousMarkers_Rejected(t *testing.T) {
	path := writeTemp(t, "a b c")

	for _, pattern := range []string{"a@@b@@c", "@@a@@b", "a@@b@@", "@@@@"} {
		if _, _, err := ResolvePositionPattern(path, pattern); err == nil {
			t.Errorf("pattern %q must be rejected (ambiguous marker placement)", pattern)
		} else if !strings.Contains(err.Error(), "exactly one @@ marker") {
			t.Errorf("pattern %q: error must name the valid forms, got: %v", pattern, err)
		}
	}
}

func TestResolvePositionPattern_DoubleMarker_InRange(t *testing.T) {
	content := "line one\nfunc CalculaRi(int tipoOrden) {\nline three\n"
	path := writeTemp(t, content)

	line, col, err := ResolvePositionPatternInRange(path, "@@CalculaRi@@", 1, 3)
	if err != nil {
		t.Fatalf("double-marker within range must resolve: %v", err)
	}
	if line != 2 {
		t.Errorf("got line %d, want 2", line)
	}
	if col != 15 {
		t.Errorf("got col %d, want 15", col)
	}
}

func TestResolvePositionPattern_Bom_Line1ColumnsUnaffected(t *testing.T) {
	// UTF-8 BOM + CRLF file (the incident's file shape). The BOM must not
	// count towards line-1 columns: "func foo" ends at col 9.
	content := "\uFEFFfunc foo() {}\r\nnext line\r\n"
	path := writeTemp(t, content)

	line, col, err := ResolvePositionPattern(path, "@@func foo@@")
	if err != nil {
		t.Fatalf("BOM'd file must resolve: %v", err)
	}
	if line != 1 {
		t.Errorf("got line %d, want 1", line)
	}
	if col != 9 {
		t.Errorf("got col %d, want 9 (BOM must not inflate line-1 column)", col)
	}
}

func TestResolvePositionPattern_Bom_SingleMarker(t *testing.T) {
	content := "\uFEFFdouble CalculaRi(int tipoOrden) {\r\n"
	path := writeTemp(t, content)

	// Single-marker form on line 1 of a BOM'd file must also be correct.
	line, col, err := ResolvePositionPattern(path, "double@@ CalculaRi")
	if err != nil {
		t.Fatalf("BOM'd file must resolve: %v", err)
	}
	if line != 1 {
		t.Errorf("got line %d, want 1", line)
	}
	if col != 7 {
		t.Errorf("got col %d, want 7 (after 'double')", col)
	}
}
