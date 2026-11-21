package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func lspRangeForTest(line string, from, to int) types.Range {
	start := len(utf16.Encode([]rune(line[:from])))
	end := len(utf16.Encode([]rune(line[:to])))
	return types.Range{
		Start: types.Position{Line: 0, Character: start},
		End:   types.Position{Line: 0, Character: end},
	}
}

func renameEditMap(uri string, rng types.Range, newText string) map[string]any {
	return map[string]any{
		"range":   rng,
		"newText": newText,
	}
}

func TestCollectRenameEditsBothForms(t *testing.T) {
	rng := lspRangeForTest("prefix_columns", 0, len("prefix_columns"))
	result := map[string]any{
		"changes": map[string]any{
			"file:///w/a.py": []any{renameEditMap("file:///w/a.py", rng, "v1")},
		},
		"documentChanges": []any{
			map[string]any{
				"textDocument": map[string]any{"uri": "file:///w/b.py"},
				"edits":        []any{renameEditMap("file:///w/b.py", rng, "v2")},
			},
		},
	}
	edits, err := collectRenameEdits(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edits) != 2 {
		t.Fatalf("got %d edits, want 2: %+v", len(edits), edits)
	}
	if edits[0].NewText != "v1" || edits[1].NewText != "v2" {
		t.Fatalf("unexpected edits: %+v", edits)
	}
}

func TestCollectRenameEditsFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		result any
	}{
		{"unmarshalable shape", map[string]any{"changes": 42}},
		{"unmarshalable value", func() {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := collectRenameEdits(tc.result); err == nil {
				t.Fatal("expected error, got nil (guard would fail open)")
			}
		})
	}
}

func TestRenameTokenAt(t *testing.T) {
	cases := []struct {
		line string
		col  int
		want string
	}{
		{"prefix_columns = 成交额", 1, "prefix_columns"},
		{"    sigma-pool x", 5, "sigma-pool"},
		{"成交额 = 1", 1, "成交额"},
		{"    foo.bar()", 5, "foo.bar"},
		{"额在域 与历史", 1, "额在域"},
	}
	for _, tc := range cases {
		if got := renameTokenAt(tc.line, tc.col); got != tc.want {
			t.Errorf("renameTokenAt(%q, %d) = %q, want %q", tc.line, tc.col, got, tc.want)
		}
	}
}

// The guard must not carry its own identifier grammar: hyphenated, dotted and
// editOver builds a one-file WorkspaceEdit that replaces line[from:to].
func editOver(uri, line string, from, to int, newText string) map[string]any {
	return map[string]any{
		"changes": map[string]any{
			uri: []any{renameEditMap(uri, lspRangeForTest(line, from, to), newText)},
		},
	}
}

// Edits the server legitimately produces cover text that contains the old
// name without being exactly it.
func TestValidateRenameTargetsAcceptsLegitimateEdits(t *testing.T) {
	cases := []struct{ name, line, covered, oldName string }{
		{"plain", "x = prefix_columns(df)", "prefix_columns", "prefix_columns"},
		{"attribute, cursor token is qualified", "x = self.prefix + 1", "prefix", "self.prefix"},
		{"cjk name", "x = 成交额 + 1", "成交额", "成交额"},
		{"accessor", "obj.getFoo()", "getFoo", "foo"},
		{"destructor", "~Foo() {}", "~Foo", "Foo"},
		{"import path", `import "example.com/oldpkg"`, "example.com/oldpkg", "oldpkg"},
		{"case differs", "Foo()", "Foo", "foo"},
		{"shorthand expands", "T{name}", "name", "name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			from := strings.Index(c.line, c.covered)
			result := editOver("file:///w/f.x", c.line, from, from+len(c.covered), "new")
			if err := validateRenameTargets(result, c.oldName, func(string) (string, error) { return c.line, nil }); err != nil {
				t.Fatalf("legitimate edit refused: %v", err)
			}
		})
	}
}

// Insertions cannot overwrite anything, so they pass.
func TestValidateRenameTargetsAllowsInsertions(t *testing.T) {
	line := "prefix_columns"
	result := editOver("file:///w/c.py", line, 3, 3, "x")
	if err := validateRenameTargets(result, "prefix_columns", func(string) (string, error) { return line, nil }); err != nil {
		t.Fatalf("insertion refused: %v", err)
	}
}

// Issue #80: a range that lands inside a docstring's prose.
func TestValidateRenameTargetsRejectsDocstringProse(t *testing.T) {
	line := `"""可用掩码 — 成交额在域与历史够长之合; 两判据皆备方可入簇"""`
	from := strings.Index(line, "交额在域")
	to := from + len("交额在域与历史够长之合; 两")
	result := editOver("file:///w/cluster.py", line, from, to, "value_columns")
	err := validateRenameTargets(result, "prefix_columns", func(string) (string, error) { return line, nil })
	if err == nil || !strings.Contains(err.Error(), "does not contain the symbol") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

// Issue #80: the closing quotes of a docstring.
func TestValidateRenameTargetsRejectsClosingQuotes(t *testing.T) {
	line := `    """σ池"""`
	from := strings.LastIndex(line, `"""`)
	result := editOver("file:///w/c.py", line, from, from+3, "value_columns")
	if err := validateRenameTargets(result, "prefix_columns", func(string) (string, error) { return line, nil }); err == nil {
		t.Fatal("expected refusal for an edit over closing quotes")
	}
}

// A single bad edit among good ones refuses the whole rename.
func TestValidateRenameTargetsOneBadEditRefusesAll(t *testing.T) {
	good, bad := "prefix_columns(df)", "cols = 1"
	result := map[string]any{"changes": map[string]any{
		"file:///w/a.py": []any{renameEditMap("", lspRangeForTest(good, 0, 14), "value_columns")},
		"file:///w/b.py": []any{renameEditMap("", lspRangeForTest(bad, 0, 4), "value_columns")},
	}}
	read := func(uri string) (string, error) {
		if strings.HasSuffix(uri, "a.py") {
			return good, nil
		}
		return bad, nil
	}
	if err := validateRenameTargets(result, "prefix_columns", read); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestValidateRenameTargetsUnknownNameSkipsCheck(t *testing.T) {
	line := "anything"
	result := editOver("file:///w/c.py", line, 0, 4, "x")
	if err := validateRenameTargets(result, "", func(string) (string, error) { return line, nil }); err != nil {
		t.Fatalf("check should be skipped when the name is unknown: %v", err)
	}
}

func TestValidateRenameTargetsReadError(t *testing.T) {
	result := editOver("file:///w/c.py", "prefix_columns", 0, 14, "x")
	readFile := func(string) (string, error) { return "", errTestRead }
	if err := validateRenameTargets(result, "prefix_columns", readFile); err == nil {
		t.Fatal("expected error when file cannot be read, got nil")
	}
}

var errTestRead = &testError{"read failed"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestValidateRenameTargetsFromJSON(t *testing.T) {
	line := "prefix_columns"
	rng := lspRangeForTest(line, 0, len(line))
	raw := `{"changes":{"file:///w/c.py":[{"range":` + jsonRange(t, rng) + `,"newText":"value_columns"}]}}`
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	readFile := func(string) (string, error) { return line, nil }
	if err := validateRenameTargets(result, "prefix_columns", readFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func jsonRange(t *testing.T, rng types.Range) string {
	t.Helper()
	b, err := json.Marshal(rng)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// An edit over part of a longer identifier would turn foo_bar into baz_bar.
func TestValidateRenameTargetsRejectsPartialIdentifier(t *testing.T) {
	line := "x = foo_bar + foo"
	result := editOver("file:///w/c.py", line, 4, 7, "baz") // "foo" inside foo_bar
	err := validateRenameTargets(result, "foo", func(string) (string, error) { return line, nil })
	if err == nil || !strings.Contains(err.Error(), "middle of an identifier") {
		t.Fatalf("expected partial-identifier refusal, got %v", err)
	}
	ok := editOver("file:///w/c.py", line, 14, 17, "baz") // standalone "foo"
	if err := validateRenameTargets(ok, "foo", func(string) (string, error) { return line, nil }); err != nil {
		t.Fatalf("standalone reference refused: %v", err)
	}
}

// Each file is read once however many edits it has.
func TestValidateRenameTargetsReadsEachFileOnce(t *testing.T) {
	line := "foo foo foo"
	rngs := []any{
		renameEditMap("", lspRangeForTest(line, 0, 3), "bar"),
		renameEditMap("", lspRangeForTest(line, 4, 7), "bar"),
		renameEditMap("", lspRangeForTest(line, 8, 11), "bar"),
	}
	result := map[string]any{"changes": map[string]any{"file:///w/c.py": rngs}}
	reads := 0
	if err := validateRenameTargets(result, "foo", func(string) (string, error) { reads++; return line, nil }); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("file read %d times, want 1", reads)
	}
}
