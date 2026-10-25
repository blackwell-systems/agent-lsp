package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyTextCapsFileSummary pins the apply-result cap: the human per-file
// lines stop at maxApplyFileSummaryLines with an "and N more" pointer, while
// the machine-parsed "Files:" JSON line stays complete so the audit layer
// (filesLineOf) still sees every written file. (PR #60 review follow-up.)
func TestApplyTextCapsFileSummary(t *testing.T) {
	var occs []replaceOccurrence
	for i := 0; i < maxApplyFileSummaryLines+25; i++ {
		rel := filepath.Join("d", fmt.Sprintf("f%03d.mqh", i))
		occs = append(occs, replaceOccurrence{
			relPath: rel, start: 0, end: 3,
			id: fmt.Sprintf("%s:0@abc%d", rel, i), lineNum: 1,
			oldLine: "foo", newLine: "bar",
		})
	}
	out := applyText(occs)

	if got := strings.Count(out, "replacement(s)"); got != maxApplyFileSummaryLines {
		t.Errorf("want %d per-file lines, got %d", maxApplyFileSummaryLines, got)
	}
	if !strings.Contains(out, "and 25 more file(s)") {
		t.Errorf("missing 'and N more' summary: %s", out)
	}
	// The audit layer must still see every file.
	files := filesLineOf(out)
	if len(files) != maxApplyFileSummaryLines+25 {
		t.Errorf("Files line lost entries: want %d, got %d", maxApplyFileSummaryLines+25, len(files))
	}
}

// TestRenderOccurrencesStopsHeadersPastCap pins the dry-run fix: once the
// render limit is spent, renderOccurrences must stop emitting per-file
// headers entirely — thousands of one-match files used to leave thousands
// of orphan headers after the cap. (PR #60 maintainer review, follow-up.)
func TestRenderOccurrencesStopsHeadersPastCap(t *testing.T) {
	var occs []replaceOccurrence
	for i := 0; i < maxReplacePreviewOccurrences+80; i++ {
		rel := filepath.Join("d", fmt.Sprintf("f%03d.mqh", i))
		occs = append(occs, replaceOccurrence{
			relPath: rel, start: 0, end: 3,
			id: fmt.Sprintf("%s:0@abc%d", rel, i), lineNum: 1,
			oldLine: "foo", newLine: "bar",
		})
	}
	out := renderOccurrences(occs, maxReplacePreviewOccurrences)
	if got := strings.Count(out, "occurrence(s)):"); got != maxReplacePreviewOccurrences {
		t.Errorf("want %d file headers, got %d", maxReplacePreviewOccurrences, got)
	}
	if !strings.Contains(out, "80 more occurrence(s) not shown") {
		t.Errorf("missing unrendered summary: %s", out)
	}
}
