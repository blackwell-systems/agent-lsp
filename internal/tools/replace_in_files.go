// replace_in_files.go implements the replace_in_files MCP tool handler.
//
// Find-and-replace across multiple files with a Serena-style protocol:
//
//  1. Call with dry_run=true: every occurrence is previewed as a minimal diff
//     with a stable, content-derived occurrence id. Nothing is written.
//  2. Re-issue with dry_run=false: applies all occurrences, or only those in
//     occurrence_ids. If ANY id is unknown or stale (file content changed
//     since the dry-run), NOTHING is changed — the caller re-runs the dry-run.
//
// File discovery walks the workspace respecting .gitignore files (with
// negation, directory patterns, anchoring and ** globs) plus hard safety
// skips (.git, .agent-lsp). Binary files and files above 8 MiB are skipped.
//
// Application goes through LSPClient.ApplyWorkspaceEdit so every touched file
// sends textDocument/didChange and the language server index stays in sync.
// Edits are computed on raw bytes, so BOM and CRLF outside edited ranges are
// preserved untouched.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/pkg/types"
)

const (
	// maxReplaceOccurrences caps the total match count; above it the tool
	// refuses to act (apply blindly on a 10k-match scan is never intent).
	maxReplaceOccurrences = 10000
	// maxReplaceFileSize skips files larger than this during scan.
	maxReplaceFileSize = 8 << 20
	// maxPreviewLineChars bounds each rendered diff line.
	maxPreviewLineChars = 160
)

// replaceOccurrence is one match of the needle in one file.
type replaceOccurrence struct {
	relPath  string // slash-separated, relative to the workspace root
	start    int    // byte offset of the match within the file
	end      int    // byte offset one past the match
	id       string // "<rel>:<nth>@<hash16>" — stable while file content is unchanged
	lineNum  int    // 1-based line of the match start
	matchCol int    // byte column of the match start within its line (display)
	oldLine  string // full source line containing the match start
	newLine  string // that line with the replacement applied (preview only)
	newRepl  string // replacement text for THIS occurrence (regex capture expansion; == repl in literal mode)
}

// replaceParams is the validated argument set for a replace_in_files run.
type replaceParams struct {
	Needle        string
	Repl          string
	Mode          string // "literal" | "regex"
	RelPath       string // optional file/dir restriction, root-relative
	IncludeGlob   string // comma-separated include globs ("" = all)
	ExcludeGlob   string // comma-separated exclude globs
	DryRun        bool
	OccurrenceIDs []string
	ExpectedCount int // -1 disables the guard
}

// replacePlan is the outcome of planning a run: everything needed to render
// the result text and, in apply mode, the selected occurrences to edit.
type replacePlan struct {
	Text        string
	IsError     bool
	Occurrences []replaceOccurrence
	Selected    []replaceOccurrence
	Files       []string // files with at least one occurrence, scan order
	Notes       []string // scan caveats surfaced to the caller (skipped/unreadable files)
}

// ---- glob translation (supports ** across segments) ----

// globToRegexp converts a shell-style glob into an anchored regexp matched
// against slash-separated relative paths. "**" crosses directory segments,
// "*" and "?" do not.
func globToRegexp(g string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(g); i++ {
		switch {
		case strings.HasPrefix(g[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(g[i:], "/**"):
			b.WriteString("/.*")
			i += 2
		case strings.HasPrefix(g[i:], "**"):
			b.WriteString(".*")
			i++
		case g[i] == '*':
			b.WriteString("[^/]*")
		case g[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(g[i : i+1]))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// compileGlobList splits a comma-separated glob list into regexps.
func compileGlobList(globs string) ([]*regexp.Regexp, error) {
	globs = strings.TrimSpace(globs)
	if globs == "" {
		return nil, nil
	}
	var out []*regexp.Regexp
	for _, g := range strings.Split(globs, ",") {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		// Gitignore-style trailing slash means "this directory"; translate it
		// to a glob matching everything beneath it.
		if strings.HasSuffix(g, "/") && !strings.HasSuffix(g, "**") {
			g = strings.TrimSuffix(g, "/") + "/**"
		}
		re, err := globToRegexp(g)
		if err != nil {
			return nil, fmt.Errorf("bad glob %q: %w", g, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// ---- .gitignore engine (subset: comments, negation, dir-only, anchoring, **) ----

type giRule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool // pattern ended in "/": matches directories only, like git
}

type giSet struct {
	dirRel string // directory containing the .gitignore, root-relative ("" = root)
	rules  []giRule
}

// giPatternToRegexp translates one gitignore pattern body to regexp source
// matched against paths relative to the .gitignore's directory.
func giPatternToRegexp(pat string) string {
	var b strings.Builder
	i := 0
	for i < len(pat) {
		switch {
		case strings.HasPrefix(pat[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(pat[i:], "/**"):
			b.WriteString("/.*")
			i += 3
		case strings.HasPrefix(pat[i:], "**"):
			b.WriteString(".*")
			i += 2
		case pat[i] == '*':
			b.WriteString("[^/]*")
			i++
		case pat[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(pat[i : i+1]))
			i++
		}
	}
	return b.String()
}

func compileGitignore(dirRel, content string) giSet {
	set := giSet{dirRel: dirRel}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}
		dirOnly := false
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "/") {
			dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		anchored := strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		if !anchored && strings.Contains(line, "/") {
			anchored = true // "a/b" is anchored even without a leading slash
		}
		body := giPatternToRegexp(line)
		var src string
		if anchored {
			src = "^" + body + "$"
		} else {
			src = "(?:^|.*/)" + body + "$"
		}
		re, err := regexp.Compile(src)
		if err != nil {
			continue // malformed pattern: ignore, like git does for ours
		}
		set.rules = append(set.rules, giRule{re: re, negate: negate, dirOnly: dirOnly})
	}
	return set
}

// gitignoreDecides returns whether rel (root-relative, slash-separated) is
// ignored. Deeper .gitignore sets override shallower ones; within a set the
// last matching rule wins; no match anywhere means "not ignored".
func gitignoreDecides(sets []giSet, rel string, isDir bool) bool {
	verdict := false
	for _, s := range sets {
		relToSet := rel
		if s.dirRel != "" {
			if !strings.HasPrefix(rel, s.dirRel+"/") {
				continue
			}
			relToSet = rel[len(s.dirRel)+1:]
		}
		for i := len(s.rules) - 1; i >= 0; i-- {
			r := s.rules[i]
			if r.dirOnly && !isDir {
				continue
			}
			if r.re.MatchString(relToSet) {
				verdict = !r.negate
				break
			}
		}
	}
	return verdict
}

// ---- file discovery ----

// collectFilesForReplace walks baseRel (root-relative, "" = whole workspace)
// respecting .gitignore files and the include/exclude glob filters. Always
// pruned: .git and .agent-lsp. Output is in lexical walk order.
func collectFilesForReplace(rootDir, baseRel string, includeRe, excludeRe []*regexp.Regexp) ([]string, error) {
	var files []string
	// Pre-load .gitignore sets from every ancestor of baseRel up to the root
	// so a restricted scan still honors the root's ignore rules.
	var initialSets []giSet
	if baseRel != "" {
		parts := strings.Split(baseRel, "/")
		// Ancestors of baseRel, root included (walk() loads baseRel's own
		// .gitignore on entry).
		for i := 0; i < len(parts); i++ {
			dirRel := strings.Join(parts[:i], "/")
			if data, err := os.ReadFile(filepath.Join(rootDir, filepath.FromSlash(dirRel), ".gitignore")); err == nil {
				initialSets = append(initialSets, compileGitignore(dirRel, string(data)))
			}
		}
	}
	var walk func(dirRel string, sets []giSet) error
	walk = func(dirRel string, sets []giSet) error {
		dirAbs := rootDir
		if dirRel != "" {
			dirAbs = filepath.Join(rootDir, filepath.FromSlash(dirRel))
		}
		if data, err := os.ReadFile(filepath.Join(dirAbs, ".gitignore")); err == nil {
			sets = append(sets, compileGitignore(dirRel, string(data)))
		}
		entries, err := os.ReadDir(dirAbs)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			childRel := name
			if dirRel != "" {
				childRel = dirRel + "/" + name
			}
			if e.Type()&os.ModeSymlink != 0 {
				// Never follow symlinks: a symlinked directory would fail the
				// ReadFile scan ("is a directory"), and a symlinked file can
				// point outside the workspace root.
				continue
			}
			if e.IsDir() {
				if name == ".git" || name == ".agent-lsp" {
					continue
				}
				if gitignoreDecides(sets, childRel, true) {
					continue
				}
				if err := walk(childRel, sets); err != nil {
					return err
				}
				continue
			}
			if name == ".gitignore" || name == ".git" {
				continue // never replace targets (.git is a file in worktrees/submodules)
			}
			if gitignoreDecides(sets, childRel, false) {
				continue
			}
			if len(includeRe) > 0 && !matchAny(includeRe, childRel) {
				continue
			}
			if matchAny(excludeRe, childRel) {
				continue
			}
			files = append(files, childRel)
		}
		return nil
	}
	if err := walk(baseRel, initialSets); err != nil {
		return nil, err
	}
	return files, nil
}

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// ---- occurrence scanning ----

// fileHash64 fingerprints (content, needle, mode, repl) so occurrence ids go
// stale the moment the file or the replace intent changes. 64-bit keeps the
// chance of a stale-id miss (an edit applied against drifted offsets)
// negligible.
func fileHash64(src, needle, mode, repl string) string {
	h := fnv.New64a()
	h.Write([]byte(src))
	h.Write([]byte{0})
	h.Write([]byte(needle))
	h.Write([]byte{0})
	h.Write([]byte(mode))
	h.Write([]byte{0})
	h.Write([]byte(repl))
	return fmt.Sprintf("%016x", h.Sum64())
}

// isBinaryFile reports whether the first 8 KiB contain a NUL byte.
func isBinaryFile(data []byte) bool {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	return strings.IndexByte(string(head), 0) >= 0
}

// scanFileOccurrences finds every non-overlapping match of needle in absPath.
// re is the pre-compiled pattern in regex mode (nil in literal mode). Returns
// (occurrences, false, nil) normally; skipped=true for binary/oversized
// files; an error only for unreadable text files.
func scanFileOccurrences(absPath, relPath string, re *regexp.Regexp, needle, mode, repl string, total *int) (occs []replaceOccurrence, skipped bool, err error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxReplaceFileSize || isBinaryFile(data) {
		return nil, true, nil
	}
	src := string(data)

	var matches [][2]int
	var subLocs [][]int // regex mode: absolute submatch index slices
	// Collect at most (remaining budget + 1) matches: one match past the
	// global cap is enough to prove overflow, so a pathological workspace
	// can never make the scan allocate submatch slices for millions of
	// matches. Regexes always run against the full file contents, so anchors
	// (^, \b) keep whole-file semantics; hitting the limit with zero-width
	// matches in the mix overflows conservatively — a needless refusal is
	// safe (nothing changed), an undercount would not be.
	budget := maxReplaceOccurrences - *total
	if budget < 0 {
		budget = 0
	}
	overflow := false
	if re != nil {
		limit := budget + 1
		locs := re.FindAllStringSubmatchIndex(src, limit)
		for _, loc := range locs {
			if loc[0] == loc[1] {
				continue // zero-width match: nothing to replace
			}
			matches = append(matches, [2]int{loc[0], loc[1]})
			subLocs = append(subLocs, loc)
		}
		overflow = len(locs) == limit
	} else {
		for i := 0; len(matches) <= budget; {
			idx := strings.Index(src[i:], needle)
			if idx < 0 {
				break
			}
			start := i + idx
			matches = append(matches, [2]int{start, start + len(needle)})
			i = start + len(needle)
		}
		overflow = len(matches) > budget
	}
	if overflow {
		// Push total past the cap so the plan-level guard refuses the run.
		*total += len(matches) + 1
		return occs, false, nil
	}

	hash := fileHash64(src, needle, mode, repl)
	for nth, m := range matches {
		if *total >= maxReplaceOccurrences {
			// Keep counting overflows so the caller's cap check fires instead
			// of silently truncating the result set.
			*total++
			return occs, false, nil
		}
		*total++
		newRepl := repl
		if re != nil {
			// Capture-group expansion: ${1}, ${name} (RE2 Expand semantics;
			// braces are required when the group is followed by a word char).
			newRepl = string(re.ExpandString(nil, repl, src, subLocs[nth]))
		}
		lineNum, matchCol, oldLine, newLine := occurrencePreviewLines(src, m[0], m[1], newRepl)
		occs = append(occs, replaceOccurrence{
			relPath:  relPath,
			start:    m[0],
			end:      m[1],
			id:       fmt.Sprintf("%s:%d@%s", relPath, nth, hash),
			lineNum:  lineNum,
			matchCol: matchCol,
			oldLine:  oldLine,
			newLine:  newLine,
			newRepl:  newRepl,
		})
	}
	return occs, false, nil
}

// occurrencePreviewLines computes the 1-based line number, the full source
// line containing the match start, and that line with the match replaced.
// For matches spanning lines the preview keeps the first line only and
// newLine is an approximation (the real edit is exact).
func occurrencePreviewLines(src string, start, end int, repl string) (lineNum, matchCol int, oldLine, newLine string) {
	lineStart := 0
	if i := strings.LastIndex(src[:start], "\n"); i >= 0 {
		lineStart = i + 1
	}
	var lineEndAbs int
	if i := strings.Index(src[start:], "\n"); i >= 0 {
		lineEndAbs = start + i
	} else {
		lineEndAbs = len(src)
	}
	line := strings.TrimSuffix(src[lineStart:lineEndAbs], "\r")
	lineNum = 1 + strings.Count(src[:lineStart], "\n")

	mStart := start - lineStart
	mEnd := end - lineStart
	if mEnd > len(line) {
		mEnd = len(line) // multi-line match: preview clamps to the first line
	}
	// The displayed line drops a trailing "\r" (CRLF): clamp the preview
	// offsets to it so a match touching that byte (e.g. a needle of "\n"
	// landing on the LF of a CRLF pair) cannot slice out of range. The real
	// edit is computed from raw bytes and stays exact.
	if mStart > len(line) {
		mStart = len(line)
	}
	if mStart > mEnd {
		mStart = mEnd
	}
	newLine = line[:mStart] + repl + line[mEnd:]
	return lineNum, mStart, line, newLine
}

// truncatePreview centers a long line on the match position for display.
func truncatePreview(s string, center int) string {
	if len(s) <= maxPreviewLineChars {
		return s
	}
	lo := center - maxPreviewLineChars/2
	if lo < 0 {
		lo = 0
	}
	hi := lo + maxPreviewLineChars
	if hi > len(s) {
		hi = len(s)
		lo = hi - maxPreviewLineChars
		if lo < 0 {
			lo = 0
		}
	}
	// Snap the window to rune boundaries so the ellipses never split one.
	for lo > 0 && !isRuneBoundary(s, lo) {
		lo--
	}
	for hi < len(s) && !isRuneBoundary(s, hi) {
		hi++
	}
	return "…" + s[lo:hi] + "…"
}

func isRuneBoundary(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	return s[i]&0xC0 != 0x80
}

// renderOccurrences produces the per-file diff block for dry-run output.
func renderOccurrences(occs []replaceOccurrence) string {
	var b strings.Builder
	byFile := map[string][]replaceOccurrence{}
	var order []string
	for _, o := range occs {
		if _, seen := byFile[o.relPath]; !seen {
			order = append(order, o.relPath)
		}
		byFile[o.relPath] = append(byFile[o.relPath], o)
	}
	for _, f := range order {
		list := byFile[f]
		fmt.Fprintf(&b, "\n%s (%d occurrence(s)):\n", f, len(list))
		for _, o := range list {
			fmt.Fprintf(&b, "  [%s] line %d\n", o.id, o.lineNum)
			fmt.Fprintf(&b, "    - %s\n", truncatePreview(o.oldLine, o.matchCol))
			fmt.Fprintf(&b, "    + %s\n", truncatePreview(o.newLine, o.matchCol))
		}
	}
	return b.String()
}

// ---- planning ----

func parseReplaceParams(args map[string]any) (replaceParams, string) {
	p := replaceParams{Mode: "literal", ExpectedCount: -1}
	p.Needle, _ = args["needle"].(string)
	if p.Needle == "" {
		return p, "needle is required"
	}
	p.Repl, _ = args["repl"].(string)
	if m, ok := args["mode"].(string); ok && m != "" {
		if m != "literal" && m != "regex" {
			return p, fmt.Sprintf("mode must be \"literal\" or \"regex\", got %q", m)
		}
		p.Mode = m
	}
	p.RelPath, _ = args["relative_path"].(string)
	p.IncludeGlob, _ = args["paths_include_glob"].(string)
	p.ExcludeGlob, _ = args["paths_exclude_glob"].(string)
	// Fail closed: a missing or non-boolean dry_run must never default into
	// apply mode on a whole-workspace tool.
	v, ok := args["dry_run"].(bool)
	if !ok {
		return p, "dry_run is required and must be a boolean (true = preview, false = apply)"
	}
	p.DryRun = v
	if raw, ok := args["occurrence_ids"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				p.OccurrenceIDs = append(p.OccurrenceIDs, s)
			}
		}
	}
	switch v := args["expected_count"].(type) {
	case float64:
		p.ExpectedCount = int(v)
	case int:
		p.ExpectedCount = v
	}
	return p, ""
}

// planReplaceInFiles resolves the file set, scans occurrences and, in apply
// mode, selects the occurrences to edit. It never writes to disk. On any
// guard failure it returns IsError with a "nothing changed" text.
func planReplaceInFiles(rootDir string, p replaceParams) replacePlan {
	includeRe, err := compileGlobList(p.IncludeGlob)
	if err != nil {
		return replacePlan{Text: err.Error(), IsError: true}
	}
	excludeRe, err := compileGlobList(p.ExcludeGlob)
	if err != nil {
		return replacePlan{Text: err.Error(), IsError: true}
	}
	var re *regexp.Regexp
	if p.Mode == "regex" {
		re, err = regexp.Compile(p.Needle)
		if err != nil {
			return replacePlan{Text: fmt.Sprintf("invalid regex: %s", err), IsError: true}
		}
	}

	// Resolve the candidate file set.
	var candidates []string
	if p.RelPath != "" {
		abs, err := ValidateFilePath(p.RelPath, rootDir)
		if err != nil {
			return replacePlan{Text: fmt.Sprintf("invalid relative_path: %s", err), IsError: true}
		}
		info, err := os.Stat(abs)
		if err != nil {
			return replacePlan{Text: fmt.Sprintf("stat %s: %s", p.RelPath, err), IsError: true}
		}
		rel, err := filepath.Rel(rootDir, abs)
		if err != nil {
			return replacePlan{Text: err.Error(), IsError: true}
		}
		if !info.IsDir() {
			// Explicit single file: bypass glob filters (explicit intent),
			// keep gitignore out of the way too — the caller named the file.
			candidates = []string{filepath.ToSlash(rel)}
		} else {
			candidates, err = collectFilesForReplace(rootDir, filepath.ToSlash(rel), includeRe, excludeRe)
			if err != nil {
				return replacePlan{Text: err.Error(), IsError: true}
			}
		}
	} else {
		candidates, err = collectFilesForReplace(rootDir, "", includeRe, excludeRe)
		if err != nil {
			return replacePlan{Text: err.Error(), IsError: true}
		}
	}

	// Scan. Unreadable files are collected and reported instead of aborting
	// the whole scan; the caller decides with full information.
	total := 0
	skipped := 0
	var failed []string
	var notes []string
	var occs []replaceOccurrence
	fileSet := map[string]bool{}
	for _, rel := range candidates {
		abs := filepath.Join(rootDir, filepath.FromSlash(rel))
		fo, sk, err := scanFileOccurrences(abs, rel, re, p.Needle, p.Mode, p.Repl, &total)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %s", rel, err))
			continue
		}
		if sk {
			skipped++
			continue
		}
		if len(fo) > 0 {
			fileSet[rel] = true
			occs = append(occs, fo...)
		}
	}
	if skipped > 0 {
		notes = append(notes, fmt.Sprintf("%d file(s) skipped (binary or over 8 MiB); their contents were NOT scanned", skipped))
	}
	if len(failed) > 0 {
		shown := failed
		if len(shown) > 10 {
			shown = append(shown[:10], fmt.Sprintf("… and %d more", len(failed)-10))
		}
		notes = append(notes, fmt.Sprintf("%d file(s) could not be read and were NOT scanned:\n  - %s", len(failed), strings.Join(shown, "\n  - ")))
	}
	if total > maxReplaceOccurrences {
		return replacePlan{Text: fmt.Sprintf("found more than %d occurrences; NOTHING was changed. Narrow the scope with relative_path or globs and retry", maxReplaceOccurrences), IsError: true}
	}

	var files []string
	for _, rel := range candidates {
		if fileSet[rel] {
			files = append(files, rel)
		}
	}

	plan := replacePlan{Occurrences: occs, Files: files}

	// expected_count guard.
	if p.ExpectedCount >= 0 && len(occs) != p.ExpectedCount {
		plan.IsError = true
		plan.Notes = notes
		plan.Text = fmt.Sprintf("expected_count guard: found %d occurrence(s), expected %d. NOTHING was changed.\n%s%s",
			len(occs), p.ExpectedCount, renderOccurrences(occs), renderNotes(notes))
		return plan
	}

	if p.DryRun {
		plan.Notes = notes
		if len(occs) == 0 {
			plan.Text = "Found 0 occurrence(s). DRY RUN - no changes were applied." + renderNotes(notes)
			return plan
		}
		plan.Text = fmt.Sprintf("Found %d occurrence(s) in %d file(s). DRY RUN - no changes were applied.\nRe-issue with dry_run=false to apply all of them, or pass occurrence_ids with the ids of the occurrences to replace.%s%s",
			len(occs), len(files), renderOccurrences(occs), renderNotes(notes))
		return plan
	}

	// Apply mode: select occurrences.
	if len(p.OccurrenceIDs) > 0 {
		byID := map[string]replaceOccurrence{}
		for _, o := range occs {
			byID[o.id] = o
		}
		var missing []string
		for _, id := range p.OccurrenceIDs {
			if o, ok := byID[id]; ok {
				plan.Selected = append(plan.Selected, o)
			} else {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			plan.IsError = true
			plan.Notes = notes
			plan.Text = fmt.Sprintf("%d occurrence id(s) unknown or stale (file contents changed since the dry-run?): %s. NOTHING was changed — re-run with dry_run=true for a fresh id list.%s%s",
				len(missing), strings.Join(missing, ", "), renderOccurrences(occs), renderNotes(notes))
			return plan
		}
	} else {
		plan.Selected = occs
	}
	plan.Notes = notes
	return plan
}

// ---- application ----

// byteRangeToLSPRange converts byte offsets into an LSP range with UTF-16
// character offsets (same conversion as textMatchApply).
func byteRangeToLSPRange(src string, startByte, endByte int) (startLine, startChar, endLine, endChar int) {
	before := src[:startByte]
	startLine = strings.Count(before, "\n")
	var lineBegin int
	if lastNL := strings.LastIndex(before, "\n"); lastNL < 0 {
		lineBegin = 0
	} else {
		lineBegin = lastNL + 1
	}
	startChar = utf16Offset(src[lineBegin:startByte], startByte-lineBegin)

	segment := src[startByte:endByte]
	endLine = startLine + strings.Count(segment, "\n")
	if lastNL := strings.LastIndex(segment, "\n"); lastNL < 0 {
		endChar = startChar + utf16Offset(segment, len(segment))
	} else {
		endContent := segment[lastNL+1:]
		endChar = utf16Offset(endContent, len(endContent))
	}
	return
}

// buildReplaceWorkspaceEdit groups the selected occurrences per file and
// builds a multi-file LSP WorkspaceEdit. Before building, every file is
// re-read and its fingerprint re-verified against the occurrence ids — a
// file that changed since the scan aborts the whole build with an error.
func buildReplaceWorkspaceEdit(rootDir string, occs []replaceOccurrence, needle, mode, repl string) (map[string]any, error) {
	if len(occs) == 0 {
		return nil, fmt.Errorf("no occurrences selected")
	}
	perFile := map[string][]replaceOccurrence{}
	var order []string
	for _, o := range occs {
		if _, seen := perFile[o.relPath]; !seen {
			order = append(order, o.relPath)
		}
		perFile[o.relPath] = append(perFile[o.relPath], o)
	}
	sort.Strings(order)

	changes := map[string]any{}
	for _, rel := range order {
		abs := filepath.Join(rootDir, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", rel, err)
		}
		src := string(data)
		if fileHash64(src, needle, mode, repl) != fileIDOf(occs, rel) {
			return nil, fmt.Errorf("%s changed since the dry-run (occurrence ids stale); nothing applied — re-run with dry_run=true", rel)
		}
		list := perFile[rel]
		sort.Slice(list, func(i, j int) bool { return list[i].start < list[j].start })
		var edits []any
		for _, o := range list {
			sl, sc, el, ec := byteRangeToLSPRange(src, o.start, o.end)
			edits = append(edits, map[string]any{
				"range": map[string]any{
					"start": map[string]any{"line": sl, "character": sc},
					"end":   map[string]any{"line": el, "character": ec},
				},
				"newText": o.newRepl,
			})
		}
		changes[CreateFileURI(abs)] = edits
	}
	return map[string]any{"changes": changes}, nil
}

// fileIDOf returns the hash component of any occurrence belonging to rel.
func fileIDOf(occs []replaceOccurrence, rel string) string {
	for _, o := range occs {
		if o.relPath == rel {
			if at := strings.LastIndex(o.id, "@"); at >= 0 {
				return o.id[at+1:]
			}
		}
	}
	return ""
}

// applyText renders the post-apply summary. filesLine is machine-parsed by
// the MCP registration layer for audit records.
func applyText(occs []replaceOccurrence) string {
	perFile := map[string]int{}
	var order []string
	for _, o := range occs {
		if _, seen := perFile[o.relPath]; !seen {
			order = append(order, o.relPath)
		}
		perFile[o.relPath]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Replaced %d occurrence(s) in %d file(s). Verify with get_diagnostics.\n", len(occs), len(order))
	for _, f := range order {
		fmt.Fprintf(&b, "  %s: %d replacement(s)\n", f, perFile[f])
	}
	// JSON array so filenames containing ", " survive the audit round-trip.
	if jb, err := json.Marshal(order); err == nil {
		fmt.Fprintf(&b, "Files: %s", jb)
	} else {
		fmt.Fprintf(&b, "Files: %s", strings.Join(order, ", "))
	}
	return b.String()
}

// filesLineOf extracts the machine-readable "Files:" line from a tool result
// text (empty string when absent, e.g. dry-run with 0 matches).
func filesLineOf(text string) []string {
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "Files: "); ok {
			var out []string
			if err := json.Unmarshal([]byte(rest), &out); err == nil {
				return out
			}
			// Legacy comma-separated format.
			for _, f := range strings.Split(rest, ", ") {
				if f = strings.TrimSpace(f); f != "" {
					out = append(out, f)
				}
			}
			return out
		}
	}
	return nil
}

// renderNotes formats scan caveats for appending to result text.
func renderNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	return "\n\nNotes:\n- " + strings.Join(notes, "\n- ")
}

// ---- MCP handler ----

// HandleReplaceInFiles is the replace_in_files tool entry point.
func HandleReplaceInFiles(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}
	p, errMsg := parseReplaceParams(args)
	if errMsg != "" {
		return types.ErrorResult(errMsg), nil
	}

	plan := planReplaceInFiles(client.RootDir(), p)
	if plan.IsError {
		return types.ErrorResult(plan.Text), nil
	}
	if p.DryRun {
		return types.TextResult(plan.Text), nil
	}
	if len(plan.Selected) == 0 {
		return types.TextResult("Found 0 occurrence(s). Nothing to replace."), nil
	}

	edit, err := buildReplaceWorkspaceEdit(client.RootDir(), plan.Selected, p.Needle, p.Mode, p.Repl)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}
	if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
		return types.ErrorResult(fmt.Sprintf("replace_in_files: %s", err)), nil
	}
	return types.TextResult(applyText(plan.Selected) + renderNotes(plan.Notes)), nil
}
