package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// applyTextMatchToFile writes src to a file, runs a text-match edit through
// the real client write path, and returns the result and the file afterwards.
func applyTextMatchToFile(t *testing.T, src, oldText, newText string) (string, bool, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "f.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	client := lsp.NewLSPClient("/bin/echo", nil)
	client.SetRootDirForTest(root)
	client.DiscardOutputForTest()
	res, changed := applyTextMatch(context.Background(), client, path, oldText, newText, nil)
	got, _ := os.ReadFile(path)
	text := ""
	if len(res.Content) > 0 {
		text = res.Content[0].Text
	}
	return string(got), changed, text
}

func TestApplyTextMatch_Edits(t *testing.T) {
	cases := []struct {
		name, src, oldText, newText, want string
	}{
		{
			// The whitespace-tolerant match used to take the matched line's
			// line break into the span, so "return" was fused onto bar().
			name:    "normalised match keeps the following line",
			src:     "func f() {\n\tfoo()\n\treturn\n}\n",
			oldText: "  foo()",
			newText: "\t\tbar()",
			want:    "func f() {\n\t\tbar()\n\treturn\n}\n",
		},
		{
			name:    "normalised match with trailing newlines on both sides",
			src:     "func f() {\n\tfoo()\n\treturn\n}\n",
			oldText: "  foo()\n",
			newText: "\t\tbar()\n",
			want:    "func f() {\n\t\tbar()\n\treturn\n}\n",
		},
		{
			name:    "normalised multi-line match",
			src:     "func f() {\n\ta()\n\tb()\n\treturn\n}\n",
			oldText: "a()\n  b()",
			newText: "\tc()",
			want:    "func f() {\n\tc()\n\treturn\n}\n",
		},
		{
			name:    "normalised match on the last line without a newline",
			src:     "x\n\tfoo()",
			oldText: "  foo()",
			newText: "bar()",
			want:    "x\nbar()",
		},
		{
			name:    "normalised match in a CRLF file keeps CRLF",
			src:     "a\r\n\tfoo()\r\n\treturn\r\n",
			oldText: "  foo()",
			newText: "\t\tbar()\n\t\tbaz()",
			want:    "a\r\n\t\tbar()\r\n\t\tbaz()\r\n\treturn\r\n",
		},
		{
			name:    "exact match is applied verbatim",
			src:     "a := 1\nb := 2\n",
			oldText: "a := 1\n",
			newText: "a := 9",
			want:    "a := 9b := 2\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed, msg := applyTextMatchToFile(t, tc.src, tc.oldText, tc.newText)
			if got != tc.want {
				t.Errorf("file = %q, want %q", got, tc.want)
			}
			if !changed {
				t.Errorf("changed = false, result %q", msg)
			}
		})
	}
}

func TestApplyTextMatch_NoChangeIsReportedAndNotWritten(t *testing.T) {
	src := "a := 1\n"
	got, changed, msg := applyTextMatchToFile(t, src, "a := 1", "a := 1")
	if changed || got != src {
		t.Fatalf("changed=%v file=%q", changed, got)
	}
	if !strings.HasPrefix(msg, "No change applied") {
		t.Fatalf("result = %q, want a no-change report", msg)
	}
}

func TestApplyTextMatch_LineBreakNote(t *testing.T) {
	_, _, msg := applyTextMatchToFile(t, "a := 1\nb := 2\n", "a := 1\n", "a := 9")
	if !strings.Contains(msg, "old_text ends with a line break and new_text does not") {
		t.Errorf("result = %q, want a note about the removed line break", msg)
	}
	_, _, msg = applyTextMatchToFile(t, "a := 1\nb := 2\n", "a := 1", "a := 9")
	if strings.Contains(msg, "Note:") {
		t.Errorf("result = %q, want no note when line breaks match", msg)
	}
}
