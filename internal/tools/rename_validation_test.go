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

func TestIsIdentifierToken(t *testing.T) {
	valid := []string{"prefix_columns", "value_columns", "x", "_private", "$ref", "变量", "café", "A1_b"}
	for _, s := range valid {
		if !isIdentifierToken(s) {
			t.Errorf("isIdentifierToken(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "额在域与历史够长之合; 两", `"`, "with space", "a-b", "1abc", `a.b`, "\n"}
	for _, s := range invalid {
		if isIdentifierToken(s) {
			t.Errorf("isIdentifierToken(%q) = true, want false", s)
		}
	}
}

func TestValidateRenameTargetsAcceptsIdentifiers(t *testing.T) {
	line := `prefix_columns = 成交额`
	rng := lspRangeForTest(line, 0, len("prefix_columns"))
	result := map[string]any{
		"changes": map[string]any{
			"file:///w/cluster.py": []any{renameEditMap("file:///w/cluster.py", rng, "value_columns")},
		},
	}
	readFile := func(uri string) (string, error) { return line, nil }
	if err := validateRenameTargets(result, readFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	readFile := func(uri string) (string, error) { return line, nil }
	err := validateRenameTargets(result, readFile)
	if err == nil {
		t.Fatal("expected error for prose edit, got nil")
	}
	if !strings.Contains(err.Error(), "single identifier token") {
		t.Fatalf("error does not explain the guard: %v", err)
	}
}

func TestValidateRenameTargetsRejectsQuote(t *testing.T) {
	line := `""value_columns`
	rng := lspRangeForTest(line, 0, 1) // covers the first double quote, not an identifier
	result := map[string]any{
		"changes": map[string]any{
			"file:///w/c.py": []any{renameEditMap("file:///w/c.py", rng, "value_columns")},
		},
	}
	readFile := func(uri string) (string, error) { return line, nil }
	if err := validateRenameTargets(result, readFile); err == nil {
		t.Fatal("expected error for quote edit, got nil")
	}
}

func TestValidateRenameTargetsReadError(t *testing.T) {
	line := "prefix_columns"
	rng := lspRangeForTest(line, 0, len(line))
	result := map[string]any{
		"changes": map[string]any{
			"file:///w/c.py": []any{renameEditMap("file:///w/c.py", rng, "x")},
		},
	}
	readFile := func(uri string) (string, error) { return "", errTestRead }
	if err := validateRenameTargets(result, readFile); err == nil {
		t.Fatal("expected error when file cannot be read, got nil")
	}
}

var errTestRead = &testError{"read failed"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestCollectRenameEditsBothForms(t *testing.T) {
	line := "prefix_columns"
	rng := lspRangeForTest(line, 0, len(line))
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
	edits := collectRenameEdits(result)
	if len(edits) != 2 {
		t.Fatalf("got %d edits, want 2: %+v", len(edits), edits)
	}
	if edits[0].NewText != "v1" || edits[1].NewText != "v2" {
		t.Fatalf("unexpected edits: %+v", edits)
	}
}

func TestValidateRenameTargetsFromJSON(t *testing.T) {
	line := "prefix_columns"
	rng := lspRangeForTest(line, 0, len(line))
	raw := `{"changes":{"file:///w/c.py":[{"range":` + jsonRange(t, rng) + `,"newText":"value_columns"}]}}`
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	readFile := func(uri string) (string, error) { return line, nil }
	if err := validateRenameTargets(result, readFile); err != nil {
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
