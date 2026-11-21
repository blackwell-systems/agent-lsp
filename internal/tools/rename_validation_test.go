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

func oneEditResult(uri, line, newText string) map[string]any {
	rng := lspRangeForTest(line, 0, len(line))
	return map[string]any{
		"changes": map[string]any{
			uri: []any{renameEditMap(uri, rng, newText)},
		},
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

func TestIsSingleRenameToken(t *testing.T) {
	valid := []string{"prefix_columns", "value_columns", "x", "_private", "$ref", "变量", "café", "A1_b", "sigma-pool", "foo.bar", "with-open-file"}
	for _, s := range valid {
		if !isSingleRenameToken(s) {
			t.Errorf("isSingleRenameToken(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "额在域与历史够长之合; 两", `"`, "with space", "\n", "a\tb"}
	for _, s := range invalid {
		if isSingleRenameToken(s) {
			t.Errorf("isSingleRenameToken(%q) = true, want false", s)
		}
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
// CJK names are all valid when every edit covers the symbol at the cursor.
func TestValidateRenameTargetsAcceptsExoticIdentifiers(t *testing.T) {
	for _, name := range []string{"prefix_columns", "sigma-pool", "foo.bar", "成交额", "$ref"} {
		t.Run(name, func(t *testing.T) {
			result := oneEditResult("file:///w/f.py", name, "new")
			readFile := func(string) (string, error) { return name, nil }
			if err := validateRenameTargets(result, name, readFile); err != nil {
				t.Fatalf("unexpected error for %q: %v", name, err)
			}
		})
	}
}

func TestValidateRenameTargetsRejectsDocstringProse(t *testing.T) {
	line := `"""可用掩码 — 成交额在域与历史够长之合; 两判据皆备方可入簇"""`
	from := strings.Index(line, "额在域")
	to := from + len("额在域与历史够长之合; 两")
	rng := lspRangeForTest(line, from, to)
	result := map[string]any{
		"documentChanges": []any{
			map[string]any{
				"textDocument": map[string]any{"uri": "file:///w/cluster.py"},
				"edits":        []any{renameEditMap("file:///w/cluster.py", rng, "value_columns")},
			},
		},
	}
	readFile := func(string) (string, error) { return line, nil }
	err := validateRenameTargets(result, "prefix_columns", readFile)
	if err == nil {
		t.Fatal("expected error for prose edit, got nil")
	}
	if !strings.Contains(err.Error(), "not a single identifier token") {
		t.Fatalf("error does not explain the guard: %v", err)
	}
}

// A wrong identifier is a single token, so it is only caught by comparing
// with the symbol resolved at the cursor (finding #2 gap).
func TestValidateRenameTargetsRejectsWrongIdentifier(t *testing.T) {
	result := oneEditResult("file:///w/c.py", "cols", "value_columns")
	readFile := func(string) (string, error) { return "cols", nil }
	err := validateRenameTargets(result, "prefix_columns", readFile)
	if err == nil || !strings.Contains(err.Error(), "symbol at the cursor") {
		t.Fatalf("expected wrong-identifier error, got %v", err)
	}
}

func TestValidateRenameTargetsRejectsMixedTargets(t *testing.T) {
	rng := lspRangeForTest("prefix_columns", 0, len("prefix_columns"))
	result := map[string]any{
		"changes": map[string]any{
			"file:///w/a.py": []any{renameEditMap("file:///w/a.py", rng, "value_columns")},
			"file:///w/b.py": []any{renameEditMap("file:///w/b.py", rng, "value_columns")},
		},
	}
	call := 0
	readFile := func(string) (string, error) {
		call++
		if call == 1 {
			return "prefix_columns", nil
		}
		return "cols", nil
	}
	err := validateRenameTargets(result, "prefix_columns", readFile)
	if err == nil || !strings.Contains(err.Error(), "different text") {
		t.Fatalf("expected mixed-target error, got %v", err)
	}
}

func TestValidateRenameTargetsRejectsEmptyRange(t *testing.T) {
	line := "prefix_columns"
	rng := lspRangeForTest(line, 3, 3)
	result := map[string]any{
		"changes": map[string]any{
			"file:///w/c.py": []any{renameEditMap("file:///w/c.py", rng, "x")},
		},
	}
	readFile := func(string) (string, error) { return line, nil }
	if err := validateRenameTargets(result, "prefix_columns", readFile); err == nil {
		t.Fatal("expected error for empty range, got nil")
	}
}

func TestValidateRenameTargetsReadError(t *testing.T) {
	result := oneEditResult("file:///w/c.py", "prefix_columns", "x")
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
