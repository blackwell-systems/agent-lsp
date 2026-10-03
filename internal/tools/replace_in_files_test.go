package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree creates files under a temp workspace root. Keys are slash paths.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"src/**", "src/a.mqh", true},
		{"src/**", "src/deep/nested/a.mqh", true},
		{"src/**", "srcx/a.mqh", false},
		{"**/*.mqh", "a.mqh", true},
		{"**/*.mqh", "x/y/a.mqh", true},
		{"**/*.mqh", "a.mq4", false},
		{"src/*.mqh", "src/a.mqh", true},
		{"src/*.mqh", "src/deep/a.mqh", false},
		{"?oo/bar.go", "foo/bar.go", true},
		{"?oo/bar.go", "too/bar.go", true},
		{"?oo/bar.go", "fooo/bar.go", false},
	}
	for _, c := range cases {
		re, err := globToRegexp(c.glob)
		if err != nil {
			t.Fatalf("glob %q: %v", c.glob, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("glob %q vs %q = %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

func TestCollectFilesGitignore(t *testing.T) {
	root := writeTree(t, map[string]string{
		".gitignore":       "*.log\n!keep.log\nbuild/\n/rootonly.txt\n",
		"a.mqh":            "a",
		"keep.log":         "keep",
		"skip.log":         "skip",
		"build/x.mqh":      "x",
		"rootonly.txt":     "r",
		"sub/rootonly.txt": "r2", // anchored pattern must NOT match here
		"sub/b.mqh":        "b",
		"sub/.gitignore":   "c.tmp\n",
		"sub/c.tmp":        "c",
		"sub/deep/d.mqh":   "d",
	})
	files, err := collectFilesForReplace(root, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(files, ",")
	want := strings.Join([]string{
		"a.mqh",
		"keep.log", // negation re-includes it
		"sub/b.mqh",
		"sub/deep/d.mqh",
		"sub/rootonly.txt",
	}, ",")
	if got != want {
		t.Errorf("collected:\n got  %s\n want %s", got, want)
	}
}

func TestCollectFilesExcludeGlob(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.mqh":        "a",
		"backup/b.mqh": "b",
		"sub/c.mqh":    "c",
	})
	excl, err := compileGlobList("backup-*/, backup/")
	if err != nil {
		t.Fatal(err)
	}
	files, err := collectFilesForReplace(root, "", nil, excl)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(files, ",")
	if got != "a.mqh,sub/c.mqh" {
		t.Errorf("got %s", got)
	}
}

func TestPlanReplaceDryRunAndApplySelection(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.mqh": "StopLong(Bid, x)\nother\nStopLong(Bid, y)\n",
		"b.mqh": "call StopLong(Bid, z)\n",
	})
	p := replaceParams{Needle: "StopLong(Bid, ", Repl: "StopLongPips(Bid, ", Mode: "literal", DryRun: true, ExpectedCount: -1}
	plan := planReplaceInFiles(root, p)
	if plan.IsError {
		t.Fatalf("unexpected plan error: %s", plan.Text)
	}
	if len(plan.Occurrences) != 3 || len(plan.Files) != 2 {
		t.Fatalf("want 3 occurrences in 2 files, got %d in %d", len(plan.Occurrences), len(plan.Files))
	}
	if !strings.Contains(plan.Text, "DRY RUN") {
		t.Errorf("dry-run text missing marker: %s", plan.Text)
	}
	for _, o := range plan.Occurrences {
		if !strings.Contains(o.id, "@") {
			t.Errorf("bad id format: %q", o.id)
		}
	}

	// Selective apply plan: only the two occurrences from a.mqh.
	var ids []string
	for _, o := range plan.Occurrences {
		if strings.HasPrefix(o.id, "a.mqh:") {
			ids = append(ids, o.id)
		}
	}
	p2 := p
	p2.DryRun = false
	p2.OccurrenceIDs = ids
	plan2 := planReplaceInFiles(root, p2)
	if plan2.IsError {
		t.Fatalf("unexpected selection error: %s", plan2.Text)
	}
	if len(plan2.Selected) != 2 {
		t.Fatalf("want 2 selected, got %d", len(plan2.Selected))
	}

	// Disk must still be untouched after dry-run/plan (no client involved).
	data, _ := os.ReadFile(filepath.Join(root, "a.mqh"))
	if !strings.Contains(string(data), "StopLong(Bid, x)") {
		t.Error("dry-run wrote to disk")
	}
}

func TestPlanReplaceStaleIDsAtomic(t *testing.T) {
	root := writeTree(t, map[string]string{"a.mqh": "foo\nfoo\n"})
	dry := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: true, ExpectedCount: -1})
	ids := []string{}
	for _, o := range dry.Occurrences {
		ids = append(ids, o.id)
	}

	// File changes after the dry-run -> every id must go stale.
	if err := os.WriteFile(filepath.Join(root, "a.mqh"), []byte("CHANGED\nfoo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	apply := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: false, OccurrenceIDs: ids, ExpectedCount: -1})
	if !apply.IsError {
		t.Fatal("expected stale-id error")
	}
	if !strings.Contains(apply.Text, "NOTHING was changed") {
		t.Errorf("error text missing atomicity notice: %s", apply.Text)
	}
}

func TestPlanReplaceExpectedCountGuard(t *testing.T) {
	root := writeTree(t, map[string]string{"a.mqh": "foo foo\n"})
	plan := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: false, ExpectedCount: 5})
	if !plan.IsError || !strings.Contains(plan.Text, "expected_count guard") {
		t.Fatalf("want guard error, got: %s", plan.Text)
	}
}

func TestPlanReplaceRegexMode(t *testing.T) {
	root := writeTree(t, map[string]string{"a.mqh": "StopLong(x)\nStopShort(y)\nkeep\n"})
	plan := planReplaceInFiles(root, replaceParams{Needle: `Stop\w+\(`, Repl: "X(", Mode: "regex", DryRun: true, ExpectedCount: -1})
	if plan.IsError {
		t.Fatalf("plan error: %s", plan.Text)
	}
	if len(plan.Occurrences) != 2 {
		t.Fatalf("want 2 regex matches, got %d", len(plan.Occurrences))
	}
}

func TestScanSkipsBinaryFiles(t *testing.T) {
	root := writeTree(t, map[string]string{
		"text.mqh": "foo\n",
		"bin.dat":  "foo\x00foo\n",
	})
	plan := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if len(plan.Occurrences) != 1 {
		t.Fatalf("binary file should be skipped, got %d occurrences", len(plan.Occurrences))
	}
}

func TestBuildReplaceWorkspaceEditRangesBOMCRLF(t *testing.T) {
	root := writeTree(t, map[string]string{})
	abs := filepath.Join(root, "a.mqh")
	src := "\xef\xbb\xbfline1\r\nfoo bar\r\nfoo baz\r\n"
	if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	dry := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "X", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if len(dry.Occurrences) != 2 {
		t.Fatalf("want 2, got %d", len(dry.Occurrences))
	}
	edit, err := buildReplaceWorkspaceEdit(root, dry.Occurrences, "foo", "literal", "X")
	if err != nil {
		t.Fatal(err)
	}
	changes, ok := edit["changes"].(map[string]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("bad edit structure: %#v", edit)
	}
	var edits []any
	for _, v := range changes {
		edits = v.([]any)
	}
	if len(edits) != 2 {
		t.Fatalf("want 2 edits, got %d", len(edits))
	}
	first := edits[0].(map[string]any)
	rng := first["range"].(map[string]any)
	start := rng["start"].(map[string]any)
	end := rng["end"].(map[string]any)
	// "foo" is on line 1 (0-based; line 0 holds the BOM), columns 0-3.
	if start["line"] != 1 || start["character"] != 0 || end["line"] != 1 || end["character"] != 3 {
		t.Errorf("unexpected range: %#v", rng)
	}
	// Staleness: rewrite the file, rebuild must fail atomically.
	if err := os.WriteFile(abs, []byte(src+"extra\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := buildReplaceWorkspaceEdit(root, dry.Occurrences, "foo", "literal", "X"); err == nil {
		t.Error("expected staleness error after file change")
	}
}

func TestBuildReplaceWorkspaceEditMultipleSameLine(t *testing.T) {
	root := writeTree(t, map[string]string{"a.mqh": "foo foo foo\n"})
	dry := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "X", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if len(dry.Occurrences) != 3 {
		t.Fatalf("want 3, got %d", len(dry.Occurrences))
	}
	edit, err := buildReplaceWorkspaceEdit(root, dry.Occurrences, "foo", "literal", "X")
	if err != nil {
		t.Fatal(err)
	}
	var edits []any
	for _, v := range edit["changes"].(map[string]any) {
		edits = v.([]any)
	}
	// Edits must be ascending by start so ApplyWorkspaceEdit's reverse-order
	// application lands each replacement on the right column.
	if len(edits) != 3 {
		t.Fatal("want 3 edits")
	}
	prev := -1
	for _, e := range edits {
		s := e.(map[string]any)["range"].(map[string]any)["start"].(map[string]any)
		col := s["character"].(int)
		if col <= prev {
			t.Errorf("edits not ascending: %d after %d", col, prev)
		}
		prev = col
	}
}

func TestFilesLineOf(t *testing.T) {
	text := "Replaced 3 occurrence(s) in 2 file(s).\n  a: 2\n  b: 1\nFiles: a, b"
	got := filesLineOf(text)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %#v", got)
	}
	if filesLineOf("no files line") != nil {
		t.Error("want nil when absent")
	}
}

func TestHandleReplaceInFilesNilClient(t *testing.T) {
	r, err := HandleReplaceInFiles(context.Background(), newNilClient(), map[string]any{
		"needle": "foo",
		"repl":   "bar",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError for nil client")
	}
}

func TestParseReplaceParamsValidation(t *testing.T) {
	// Missing needle.
	if _, errMsg := parseReplaceParams(map[string]any{}); !strings.Contains(errMsg, "needle") {
		t.Errorf("want needle error, got %q", errMsg)
	}
	// Bad mode.
	if _, errMsg := parseReplaceParams(map[string]any{"needle": "foo", "repl": "bar", "mode": "fancy"}); !strings.Contains(errMsg, "mode") {
		t.Errorf("want mode error, got %q", errMsg)
	}
	// Defaults and expected_count coercion.
	p, errMsg := parseReplaceParams(map[string]any{"needle": "foo", "repl": "bar", "expected_count": float64(3)})
	if errMsg != "" {
		t.Fatalf("unexpected error: %q", errMsg)
	}
	if p.Mode != "literal" || p.ExpectedCount != 3 {
		t.Errorf("defaults not applied: mode=%q count=%d", p.Mode, p.ExpectedCount)
	}
}
