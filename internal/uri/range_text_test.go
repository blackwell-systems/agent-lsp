package uri

import (
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestRangeTextCJK(t *testing.T) {
	line := `    doc = "可用掩码 — 成交额在域与历史够长之合"`
	// Convert the byte index of the target through UTF-16 units, as LSP does.
	byteStart := strings.Index(line, "成交额")
	byteEnd := byteStart + len("成交额")
	start := len(utf16.Encode([]rune(line[:byteStart])))
	end := len(utf16.Encode([]rune(line[:byteEnd])))

	rng := types.Range{
		Start: types.Position{Line: 0, Character: start},
		End:   types.Position{Line: 0, Character: end},
	}
	if got, want := RangeText(line, rng), "成交额"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRangeTextMultiLine(t *testing.T) {
	content := "alpha\nbeta\ngamma"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 2},
		End:   types.Position{Line: 2, Character: 3},
	}
	if got, want := RangeText(content, rng), "pha\nbeta\ngam"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRangeTextClampsPastEnd(t *testing.T) {
	content := "abc"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 1},
		End:   types.Position{Line: 9, Character: 99},
	}
	if got, want := RangeText(content, rng), "bc"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
