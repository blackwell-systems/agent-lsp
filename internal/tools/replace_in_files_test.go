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
	p, errMsg := parseReplaceParams(map[string]any{"needle": "foo", "repl": "bar", "dry_run": true, "expected_count": float64(3)})
	if errMsg != "" {
		t.Fatalf("unexpected error: %q", errMsg)
	}
	if p.Mode != "literal" || p.ExpectedCount != 3 {
		t.Errorf("defaults not applied: mode=%q count=%d", p.Mode, p.ExpectedCount)
	}
	// dry_run must fail closed: missing or non-boolean never defaults into
	// apply mode on a whole-workspace tool.
	if _, errMsg := parseReplaceParams(map[string]any{"needle": "foo", "repl": "bar"}); errMsg == "" {
		t.Error("missing dry_run must be rejected")
	}
	if _, errMsg := parseReplaceParams(map[string]any{"needle": "foo", "repl": "bar", "dry_run": "true"}); errMsg == "" {
		t.Error("non-boolean dry_run must be rejected")
	}
	// expected_count = 0 must survive (require zero matches).
	p0, errMsg := parseReplaceParams(map[string]any{"needle": "foo", "repl": "bar", "dry_run": true, "expected_count": float64(0)})
	if errMsg != "" || p0.ExpectedCount != 0 {
		t.Errorf("expected_count 0 not preserved: err=%q count=%d", errMsg, p0.ExpectedCount)
	}
}

// The occurrence cap must refuse the run, not silently truncate the result
// to exactly maxReplaceOccurrences matches.
func TestPlanReplaceCapRefuses(t *testing.T) {
	root := writeTree(t, map[string]string{
		"big.mqh":   strings.Repeat("hit\n", maxReplaceOccurrences+1),
		"small.mqh": "hit\n",
	})
	plan := planReplaceInFiles(root, replaceParams{Needle: "hit", Repl: "x", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if !plan.IsError {
		t.Fatalf("expected cap refusal, got %d occurrences", len(plan.Occurrences))
	}
	if !strings.Contains(plan.Text, "NOTHING was changed") {
		t.Errorf("refusal text missing atomicity notice: %s", plan.Text)
	}
}

// A scan restricted to a subdirectory must still honor the .gitignore files
// of every ancestor directory, including the workspace root.
func TestCollectFilesRestrictedHonorsRootGitignore(t *testing.T) {
	root := writeTree(t, map[string]string{
		".gitignore":          "generated/\n",
		"sub/keep.txt":        "hit\n",
		"sub/generated/g.txt": "hit\n",
	})
	files, err := collectFilesForReplace(root, "sub", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "sub/keep.txt" {
		t.Errorf("root gitignore not honored in restricted scan, got %v", files)
	}
}

// Symlinks must be skipped, never followed: a symlinked directory would fail
// the file scan and abort the whole run.
func TestCollectFilesSkipsSymlinks(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.txt":       "hit\n",
		"other/b.txt": "hit\n",
	})
	if err := os.Symlink(filepath.Join(root, "other"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	files, err := collectFilesForReplace(root, "", nil, nil)
	if err != nil {
		t.Fatalf("symlinked dir must be skipped, got error: %v", err)
	}
	for _, f := range files {
		if strings.HasPrefix(f, "link/") || f == "link" {
			t.Errorf("symlink followed: %v", files)
		}
	}
}

// ".git" as a FILE (worktrees, submodules) must never be a scan target.
func TestCollectFilesSkipsGitFile(t *testing.T) {
	root := writeTree(t, map[string]string{
		".git":  "gitdir: /somewhere/else\n",
		"a.txt": "hit\n",
	})
	plan := planReplaceInFiles(root, replaceParams{Needle: "gitdir", Repl: "x", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if plan.IsError {
		t.Fatalf("unexpected error: %s", plan.Text)
	}
	if len(plan.Occurrences) != 0 {
		t.Errorf(".git file scanned: %v", plan.Files)
	}
}

// A gitignore pattern ending in "/" matches directories only: a file with
// the same name must stay scannable, like git.
func TestGitignoreDirOnlyPattern(t *testing.T) {
	root := writeTree(t, map[string]string{
		".gitignore":        "assets/\n",
		"assets/inside.txt": "hit\n",
		"sub/assets":        "hit\n",
	})
	files, err := collectFilesForReplace(root, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "sub/assets" {
		t.Errorf("dir-only pattern must ignore the dir but keep same-named files, got %v", files)
	}
}

// Regex-mode repl is a template: capture groups ($1, ${name}) must expand
// per occurrence in both the preview and the applied edit.
func TestRegexCaptureExpansion(t *testing.T) {
	root := writeTree(t, map[string]string{"a.mqh": "ab cd\n"})
	dry := planReplaceInFiles(root, replaceParams{Needle: `(a)(b) `, Repl: `${1}x `, Mode: "regex", DryRun: true, ExpectedCount: -1})
	if dry.IsError || len(dry.Occurrences) != 1 {
		t.Fatalf("dry-run: err=%v occs=%d", dry.IsError, len(dry.Occurrences))
	}
	if got := dry.Occurrences[0].newRepl; got != "ax " {
		t.Errorf("capture not expanded: newRepl=%q", got)
	}
	if !strings.Contains(dry.Text, "ax cd") {
		t.Errorf("preview not expanded: %s", dry.Text)
	}
	apply := planReplaceInFiles(root, replaceParams{Needle: `(a)(b) `, Repl: `${1}x `, Mode: "regex", DryRun: false, ExpectedCount: -1})
	if apply.IsError || len(apply.Selected) != 1 {
		t.Fatalf("apply plan: err=%v sel=%d", apply.IsError, len(apply.Selected))
	}
	edit, err := buildReplaceWorkspaceEdit(root, apply.Selected, `(a)(b) `, "regex", `${1}x `)
	if err != nil {
		t.Fatal(err)
	}
	changes := edit["changes"].(map[string]any)
	for _, edits := range changes {
		first := edits.([]any)[0].(map[string]any)
		if first["newText"] != "ax " {
			t.Errorf("edit newText not expanded: %v", first["newText"])
		}
	}
}

// The apply result's Files line is a JSON array so filenames containing
// ", " survive the audit round-trip.
func TestFilesLineOfJSON(t *testing.T) {
	text := "Replaced 2 occurrence(s) in 2 file(s).\nFiles: [\"a.go\",\"b, c.go\"]"
	got := filesLineOf(text)
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b, c.go" {
		t.Errorf("JSON Files line mis-parsed: %v", got)
	}
}

// Binary and unreadable files must be surfaced as notes, not silently
// dropped or fatal.
func TestPlanReplaceNotesReportSkippedFiles(t *testing.T) {
	root := writeTree(t, map[string]string{
		"text.mqh": "hit\n",
		"bin.dat":  "hit\x00hit\n",
	})
	plan := planReplaceInFiles(root, replaceParams{Needle: "hit", Repl: "x", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if plan.IsError {
		t.Fatalf("unexpected error: %s", plan.Text)
	}
	if len(plan.Occurrences) != 1 {
		t.Fatalf("binary file must not contribute occurrences, got %d", len(plan.Occurrences))
	}
	if !strings.Contains(plan.Text, "skipped (binary or over 8 MiB)") {
		t.Errorf("skipped-file note missing: %s", plan.Text)
	}
	if len(plan.Notes) == 0 {
		t.Error("plan.Notes must carry the skip note")
	}
}

// occurrencePreviewLines must not panic when a match starts on the LF byte
// of a CRLF pair (the displayed line trims the "\r", shifting the offsets).
func TestOccurrencePreviewLinesCRLFMatchesOnLF(t *testing.T) {
	src := "a\r\nb\n"
	lineNum, matchCol, oldLine, newLine := occurrencePreviewLines(src, 2, 3, "X")
	if lineNum != 1 || oldLine != "a" || newLine != "aX" || matchCol < 0 || matchCol > len(oldLine) {
		t.Errorf("unexpected preview: line=%d col=%d old=%q new=%q", lineNum, matchCol, oldLine, newLine)
	}
}

// A full plan run with a needle that lands on CRLF line feeds must complete
// without panicking.
func TestPlanReplaceCRLFNeedleLF(t *testing.T) {
	root := writeTree(t, map[string]string{"crlf.txt": "a\r\nb\r\n"})
	plan := planReplaceInFiles(root, replaceParams{Needle: "\n", Repl: "", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if plan.IsError {
		t.Fatalf("unexpected error: %s", plan.Text)
	}
	if len(plan.Occurrences) != 2 {
		t.Fatalf("want 2 LF matches, got %d", len(plan.Occurrences))
	}
}

// The bounded collection must also work in regex mode, including the
// submatch-index translation used by capture expansion.
func TestPlanReplaceCapRefusesRegex(t *testing.T) {
	root := writeTree(t, map[string]string{
		"big.mqh": strings.Repeat("hit\n", maxReplaceOccurrences+1),
	})
	plan := planReplaceInFiles(root, replaceParams{Needle: `h(i)t`, Repl: "x", Mode: "regex", DryRun: true, ExpectedCount: -1})
	if !plan.IsError {
		t.Fatalf("expected cap refusal, got %d occurrences", len(plan.Occurrences))
	}
}

// Regex matches in an under-budget file keep correct submatch data: expansion
// still references the right groups after the budget-bounded rewrite.
func TestRegexSubmatchDataUnderBudget(t *testing.T) {
	root := writeTree(t, map[string]string{"a.mqh": "ab ab ab\n"})
	plan := planReplaceInFiles(root, replaceParams{Needle: `(a)(b)`, Repl: `${2}${1}`, Mode: "regex", DryRun: true, ExpectedCount: -1})
	if plan.IsError || len(plan.Occurrences) != 3 {
		t.Fatalf("err=%v occs=%d", plan.IsError, len(plan.Occurrences))
	}
	for _, o := range plan.Occurrences {
		if o.newRepl != "ba" {
			t.Errorf("capture expansion broken: %q", o.newRepl)
		}
	}
}
