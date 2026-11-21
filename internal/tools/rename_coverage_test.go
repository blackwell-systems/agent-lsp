package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func editFor(root string, names ...string) map[string]any {
	changes := map[string]any{}
	for _, n := range names {
		rng := types.Range{End: types.Position{Character: 1}}
		changes[CreateFileURI(filepath.Join(root, n))] = []any{renameEditMap("", rng, "x")}
	}
	return map[string]any{"changes": changes}
}

func TestUnrenamedMentions(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pkg/cols.py":         "def prefix_columns(df): pass\n",
		"pkg/cluster.py":      "from pkg.cols import prefix_columns\nprefix_columns(1)\n",
		"pkg/other.py":        "x = 1  # unrelated\n",
		"pkg/similar.py":      "my_prefix_columns = 2\nprefix_columns_v2 = 3\n",
		"pkg/notes.txt":       "prefix_columns is documented here\n",
		"node_modules/a/b.py": "prefix_columns()\n",
		".venv/c.py":          "prefix_columns()\n",
	})
	target := filepath.Join(root, "pkg/cols.py")

	// The rename found only cols.py: cluster.py is the missed file. Longer names,
	// other file types and skipped directories do not count.
	got := unrenamedMentions(root, target, "prefix_columns", editFor(root, "pkg/cols.py"))
	if len(got) != 1 || filepath.Base(got[0]) != "cluster.py" {
		t.Fatalf("missed = %v, want only cluster.py", got)
	}

	// A complete rename leaves nothing missed.
	if got := unrenamedMentions(root, target, "prefix_columns", editFor(root, "pkg/cols.py", "pkg/cluster.py")); len(got) != 0 {
		t.Fatalf("complete rename reported %v", got)
	}

	// A qualified cursor token searches for its last part.
	if got := unrenamedMentions(root, target, "self.prefix_columns", editFor(root, "pkg/cols.py")); len(got) != 1 {
		t.Fatalf("qualified token: missed = %v", got)
	}
}

func TestUnrenamedMentionsDegradesQuietly(t *testing.T) {
	root := writeTree(t, map[string]string{"a.py": "foo\n"})
	for name, got := range map[string][]string{
		"no root":  unrenamedMentions("", filepath.Join(root, "a.py"), "foo", nil),
		"no name":  unrenamedMentions(root, filepath.Join(root, "a.py"), "", nil),
		"no ext":   unrenamedMentions(root, filepath.Join(root, "Makefile"), "foo", nil),
		"bad root": unrenamedMentions(filepath.Join(root, "missing"), filepath.Join(root, "a.py"), "foo", nil),
	} {
		if len(got) != 0 {
			t.Errorf("%s: got %v", name, got)
		}
	}
}

func TestUnrenamedHint(t *testing.T) {
	if unrenamedHint("foo", nil) != "" {
		t.Fatal("no missed files must produce no hint")
	}
	var many []string
	for i := 0; i < 12; i++ {
		many = append(many, filepath.Join("/w", string(rune('a'+i))+".py"))
	}
	h := unrenamedHint("self.foo", many)
	for _, want := range []string{"12 other file(s)", "mention foo", "and 4 more", "run rename_symbol again"} {
		if !strings.Contains(h, want) {
			t.Errorf("hint %q lacks %q", h, want)
		}
	}
}
