package uri

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// utf16Range builds a types.Range using UTF-16 code-unit offsets, the way
// spec-compliant LSP servers report positions.
func utf16Range(line int, startUnits, endUnits int) types.Range {
	return types.Range{
		Start: types.Position{Line: line, Character: startUnits},
		End:   types.Position{Line: line, Character: endUnits},
	}
}

// TestApplyRangeEdit_UTF16Offsets is the regression test for the reported
// defect: ApplyRangeEdit used to treat the LSP UTF-16 character offsets as
// byte indexes, silently shifting every cut on lines that contain multi-byte
// characters before the edit point.
func TestApplyRangeEdit_UTF16Offsets(t *testing.T) {
	tests := []struct {
		name    string
		content string
		rng     types.Range
		newText string
		want    string
	}{
		{
			name: "BOM as first rune plus accented Latin (live repro, BOM+CRLF MQL encoding)",
			// BOM + "// Test de codificación con acentos" + CRLF
			content: "\uFEFF// Test de codificaci\u00f3n con acentos\r\n\r\ndouble x;\r\n",
			rng:     utf16Range(0, 0, 36), // 36 UTF-16 units: BOM(1) + visible text(35); keeps the trailing \r
			newText: "// EDITADO-UTF16",
			want:    "// EDITADO-UTF16\r\n\r\ndouble x;\r\n",
		},
		{
			name:    "edit after BOM only",
			content: "\uFEFF// hola",
			rng:     utf16Range(0, 1, 8),
			newText: "X",
			want:    "\uFEFFX",
		},
		{
			name:    "accented Latin before edit point",
			content: "caf\u00e9 = 1",
			rng:     utf16Range(0, 7, 8), // unit 7 is the "1"; é before it is 1 unit but 2 bytes
			newText: "2",
			want:    "caf\u00e9 = 2",
		},
		{
			name:    "CJK characters count one UTF-16 unit but three UTF-8 bytes",
			content: "\u4f60\u597d world",
			rng:     utf16Range(0, 2, 8),
			newText: "X",
			want:    "\u4f60\u597dX",
		},
		{
			name:    "emoji is a surrogate pair (2 UTF-16 units, 4 UTF-8 bytes)",
			content: "\U0001F600 hi",
			rng:     utf16Range(0, 2, 5),
			newText: "X",
			want:    "\U0001F600X",
		},
		{
			name:    "offset landing inside a surrogate pair resolves to rune start",
			content: "\U0001F600 hi",
			rng:     utf16Range(0, 1, 3), // 1 unit = middle of the surrogate pair
			newText: "X",
			want:    "Xhi",
		},
		{
			name:    "multi-line range with non-ASCII on both boundary lines",
			content: "caf\u00e9\n\u4f60\u597d\nplain",
			rng: types.Range{
				Start: types.Position{Line: 0, Character: 4}, // end of "café"
				End:   types.Position{Line: 1, Character: 2}, // end of "你好"
			},
			newText: "-MID-",
			want:    "caf\u00e9-MID-\nplain",
		},
		{
			name:    "clamp beyond UTF-16 line length still replaces whole line",
			content: "caf\u00e9",
			rng:     utf16Range(0, 0, 100),
			newText: "X",
			want:    "X",
		},
		{
			name:    "pure ASCII unchanged semantics",
			content: "abc\ndef",
			rng:     utf16Range(0, 1, 2),
			newText: "X",
			want:    "aXc\ndef",
		},
		{
			name:    "invalid UTF-8 bytes do not panic and count one unit each",
			content: string([]byte{'a', 0xFF, 0xFE, 'b', 'c'}),
			rng:     utf16Range(0, 0, 5),
			newText: "X",
			want:    "X",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyRangeEdit(tc.content, tc.rng, tc.newText)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUTF16LengthAndByteOffset(t *testing.T) {
	cases := []struct {
		s     string
		units int
	}{
		{"", 0},
		{"abc", 3},
		{"\u4f60\u597d", 2},
		{"\U0001F600", 2},
		{strings.Repeat("\u4e2d", 10), 10},
		{"\uFEFF// Test de codificaci\u00f3n con acentos\r", 37}, // live repro line: BOM + visible + \r
	}
	for _, c := range cases {
		if got := utf16Length(c.s); got != c.units {
			t.Errorf("utf16Length(%q) = %d, want %d", c.s, got, c.units)
		}
		// Round-trip: converting the UTF-16 length back must yield len(s).
		if got := utf16ToByteOffset(c.s, c.units); got != len(c.s) {
			t.Errorf("utf16ToByteOffset(%q, %d) = %d, want %d", c.s, c.units, got, len(c.s))
		}
		// Negative and zero offsets clamp to 0.
		if got := utf16ToByteOffset(c.s, -1); got != 0 {
			t.Errorf("utf16ToByteOffset(%q, -1) = %d, want 0", c.s, got)
		}
	}
}
