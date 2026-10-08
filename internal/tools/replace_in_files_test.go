package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	uripkg "github.com/blackwell-systems/agent-lsp/internal/uri"
	"github.com/blackwell-systems/agent-lsp/pkg/types"
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

// TestBuildReplaceWorkspaceEditNonASCIIBytes asserts the bytes actually
// written when the LSP ranges produced by buildReplaceWorkspaceEdit are
// applied with the canonical uripkg.ApplyRangeEdit (the same path
// LSPClient.applyEditsToFile uses). Non-ASCII text before the match must
// survive byte-for-byte: LSP characters are UTF-16 code units, not bytes,
// so the range offsets must account for multi-byte runes and the BOM.
func TestBuildReplaceWorkspaceEditNonASCIIBytes(t *testing.T) {
	cases := []struct {
		name string
		src  string // needle "foo" appears after the non-ASCII prefix
		want string
	}{
		{"accented", "é foo\n", "é bar\n"},
		{"accented_multi", "ééé foo\n", "ééé bar\n"},
		{"cjk", "日本語 foo\n", "日本語 bar\n"},
		{"cjk_and_accented", "日本 é foo\n", "日本 é bar\n"},
		{"emoji", "😀 foo\n", "😀 bar\n"},
		{"bom", "\xef\xbb\xbfé foo\n", "\xef\xbb\xbfé bar\n"},
		{"bom_cjk", "\xef\xbb\xbf日本語 foo\n", "\xef\xbb\xbf日本語 bar\n"},
		{"nonascii_after_match", "foo é bar foo\n", "bar é bar bar\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{"u.mqh": tc.src})
			dry := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: true, ExpectedCount: -1})
			if len(dry.Occurrences) == 0 {
				t.Fatalf("no occurrences found in %q", tc.src)
			}
			edit, err := buildReplaceWorkspaceEdit(root, dry.Occurrences, "foo", "literal", "bar")
			if err != nil {
				t.Fatal(err)
			}
			changes, ok := edit["changes"].(map[string]any)
			if !ok || len(changes) != 1 {
				t.Fatalf("bad edit structure: %#v", edit)
			}
			var decoded []textEditForTest
			for uri, v := range changes {
				if !strings.Contains(uri, "u.mqh") {
					t.Fatalf("unexpected uri %q", uri)
				}
				for _, e := range v.([]any) {
					m := e.(map[string]any)
					rng := m["range"].(map[string]any)
					s := rng["start"].(map[string]any)
					en := rng["end"].(map[string]any)
					decoded = append(decoded, textEditForTest{
						sl: s["line"].(int), sc: s["character"].(int),
						el: en["line"].(int), ec: en["character"].(int),
						newText: m["newText"].(string),
					})
				}
			}
			// Mirror LSPClient.applyEditsToFile: apply bottom-to-top with
			// the canonical ApplyRangeEdit, then compare written bytes.
			content := tc.src
			for i := len(decoded) - 1; i >= 0; i-- {
				e := decoded[i]
				content = uripkg.ApplyRangeEdit(content, types.Range{
					Start: types.Position{Line: e.sl, Character: e.sc},
					End:   types.Position{Line: e.el, Character: e.ec},
				}, e.newText)
			}
			if content != tc.want {
				t.Errorf("written bytes mismatch:\n got %q\nwant %q", content, tc.want)
			}
		})
	}
}

// textEditForTest is a decoded TextEdit for byte-level assertions.
type textEditForTest struct {
	sl, sc, el, ec int
	newText        string
}

// TestRelativePathResolvesAgainstRootNotCwd pins review finding 3:
// relative_path is workspace-root relative. ValidateFilePath absolutizes
// from the process cwd, so without the join onto rootDir a scan restricted
// with relative_path breaks whenever the server cwd differs from the root.
func TestRelativePathResolvesAgainstRootNotCwd(t *testing.T) {
	root := writeTree(t, map[string]string{
		"src/x.mqh": "foo\n",
		"src/y.mqh": "foo\n",
		"out/z.mqh": "foo\n",
	})
	elsewhere := t.TempDir()
	t.Chdir(elsewhere) // server cwd differs from the workspace root

	dir := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: true, ExpectedCount: -1, RelPath: "src"})
	if len(dir.Occurrences) != 2 {
		t.Fatalf("relative_path=src: want 2 occurrences, got %d (%s)", len(dir.Occurrences), dir.Text)
	}
	file := planReplaceInFiles(root, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: true, ExpectedCount: -1, RelPath: "src/x.mqh"})
	if len(file.Occurrences) != 1 {
		t.Fatalf("relative_path=src/x.mqh: want 1 occurrence, got %d (%s)", len(file.Occurrences), file.Text)
	}
	if file.Occurrences[0].relPath != "src/x.mqh" {
		t.Fatalf("unexpected relPath %q", file.Occurrences[0].relPath)
	}
}

// TestResolveReplaceRootSymlinkedRoot pins review finding 7: a root reached
// through a symlink (macOS /tmp -> /private/tmp) must be resolved once up
// front so occurrence ids stay root-relative and never contain ../..
func TestResolveReplaceRootSymlinkedRoot(t *testing.T) {
	real := writeTree(t, map[string]string{"a.mqh": "foo\n"})
	link := filepath.Join(t.TempDir(), "rootlink")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolved, err := resolveReplaceRoot(link)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != real {
		t.Fatalf("resolved root %q, want %q", resolved, real)
	}
	plan := planReplaceInFiles(resolved, replaceParams{Needle: "foo", Repl: "bar", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if len(plan.Occurrences) != 1 {
		t.Fatalf("want 1 occurrence, got %d (%s)", len(plan.Occurrences), plan.Text)
	}
	if id := plan.Occurrences[0].id; strings.Contains(id, "..") {
		t.Fatalf("occurrence id escapes the root: %q", id)
	}
}

// TestReplacePartialFailureText pins the error-path audit contract: the
// files already written must appear in the machine-readable Files: line so
// the audit record reflects the real workspace mutations.
func TestReplacePartialFailureText(t *testing.T) {
	root := t.TempDir()
	err := errors.New("applyEdit write b.mqh: permission denied")
	got := replacePartialFailureText(err, []string{
		CreateFileURI(filepath.Join(root, "b.mqh")),
		CreateFileURI(filepath.Join(root, "a.mqh")),
	}, root)
	wantSub := "2 file(s) were already written before the failure: a.mqh, b.mqh"
	if !strings.Contains(got, wantSub) {
		t.Errorf("missing written-files report:\n got %q\nwant substring %q", got, wantSub)
	}
	if !strings.Contains(got, `Files: ["a.mqh","b.mqh"]`) {
		t.Errorf("missing machine-readable Files line: %q", got)
	}
	if !strings.Contains(got, "permission denied") {
		t.Errorf("underlying error lost: %q", got)
	}
	// No files written: no Files line, only the error.
	plain := replacePartialFailureText(err, nil, root)
	if strings.Contains(plain, "Files:") || strings.Contains(plain, "already written") {
		t.Errorf("empty written list must not render a Files line: %q", plain)
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

// Regex matches must keep whole-file context for zero-width assertions:
// `^foo` over "foofoo" is ONE occurrence, not two (this guards against an
// incremental src[at:] re-anchoring regression).
func TestRegexAnchorsFullFileContext(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		needle string
		want   int
	}{
		{"caret", map[string]string{"a.mqh": "foofoo\n"}, `^foo`, 1},
		{"word-boundary", map[string]string{"a.mqh": "foo foo foofoo\n"}, `\bfoo`, 3},
		{"dollar", map[string]string{"a.mqh": "barbar"}, `bar$`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, tc.files)
			plan := planReplaceInFiles(root, replaceParams{Needle: tc.needle, Repl: "X", Mode: "regex", DryRun: true, ExpectedCount: -1})
			if plan.IsError {
				t.Fatalf("unexpected error: %s", plan.Text)
			}
			if len(plan.Occurrences) != tc.want {
				t.Errorf("%s: want %d occurrences, got %d", tc.needle, tc.want, len(plan.Occurrences))
			}
		})
	}
}

// A nullable regex that fills the match limit with zero-width matches must
// still overflow past the cap: the file's occurrences must never be dropped
// silently while other files get applied.
func TestRegexZeroWidthOverflowStillRefuses(t *testing.T) {
	root := writeTree(t, map[string]string{
		// (?m)^ matches empty at every line start: 10002 raw matches, all
		// zero-width, exactly filling the limit with nothing real collected.
		"big.mqh": strings.Repeat("x\n", 10002),
	})
	plan := planReplaceInFiles(root, replaceParams{Needle: `(?m)^`, Repl: "X", Mode: "regex", DryRun: true, ExpectedCount: -1})
	if !plan.IsError {
		t.Fatalf("expected cap refusal despite zero-width-heavy matches, got %d occurrences", len(plan.Occurrences))
	}
	if !strings.Contains(plan.Text, "NOTHING was changed") {
		t.Errorf("refusal text missing atomicity notice: %s", plan.Text)
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

// Repeating the same occurrence id in occurrence_ids must refuse the apply
// instead of emitting duplicate fixed-range edits (CodeRabbit #60 round 3).
func TestPlanReplaceRepeatedIDsRefuse(t *testing.T) {
	root := writeTree(t, map[string]string{"a.txt": "hit hit\n"})
	preview := planReplaceInFiles(root, replaceParams{Needle: "hit", Repl: "x", Mode: "literal", DryRun: true, ExpectedCount: -1})
	if preview.IsError || len(preview.Occurrences) != 2 {
		t.Fatalf("err=%v occs=%d", preview.IsError, len(preview.Occurrences))
	}
	id := preview.Occurrences[0].id
	plan := planReplaceInFiles(root, replaceParams{
		Needle: "hit", Repl: "x", Mode: "literal", DryRun: false, ExpectedCount: -1,
		OccurrenceIDs: []string{id, id},
	})
	if !plan.IsError {
		t.Fatal("repeated occurrence id must refuse the apply")
	}
	if !strings.Contains(plan.Text, "repeated") {
		t.Errorf("refusal text must mention the repeated id: %s", plan.Text)
	}
	if !strings.Contains(plan.Text, "NOTHING was changed") {
		t.Errorf("refusal text missing atomicity notice: %s", plan.Text)
	}
}

// An apply with zero occurrences must surface the scan notes (skipped or
// unreadable files) instead of a bare "nothing to replace" (CodeRabbit #60
// round 3: empty apply concealing unscanned files).
func TestApplyEmptySurfacesScanNotes(t *testing.T) {
	root := writeTree(t, map[string]string{
		"plain.txt": "no match here\n",
		"bin.dat":   "hit\x00hit\n",
	})
	plan := planReplaceInFiles(root, replaceParams{Needle: "hit", Repl: "x", Mode: "literal", DryRun: false, ExpectedCount: -1})
	if plan.IsError {
		t.Fatalf("unexpected error: %s", plan.Text)
	}
	if len(plan.Selected) != 0 {
		t.Fatalf("want 0 selected occurrences, got %d", len(plan.Selected))
	}
	if !strings.Contains(plan.Text, "Found 0 occurrence(s)") {
		t.Errorf("zero-count notice missing: %s", plan.Text)
	}
	if !strings.Contains(plan.Text, "skipped (binary or over 8 MiB)") {
		t.Errorf("scan notes missing from empty apply: %s", plan.Text)
	}
}
