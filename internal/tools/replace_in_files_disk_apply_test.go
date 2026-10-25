package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// TestReplaceInFilesApplyWritesNonASCIIBytesToDisk drives the REAL write
// path: the WorkspaceEdit built by buildReplaceWorkspaceEdit goes through
// LSPClient.ApplyWorkspaceEdit (sorted URI order, ValidatePath against the
// client root, canonical ApplyRangeEdit per file, os.WriteFile). The
// in-memory replay in TestBuildReplaceWorkspaceEditNonASCIIBytes cannot
// catch a client-side write bug; this test asserts the bytes on disk.
// (PR #60 maintainer review, follow-up.)
func TestReplaceInFilesApplyWritesNonASCIIBytesToDisk(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"accented", "é foo\n", "é bar\n"},
		{"cjk", "日本語 foo\n", "日本語 bar\n"},
		{"emoji", "😀 foo\n", "😀 bar\n"},
		{"bom", "\xef\xbb\xbfé foo\n", "\xef\xbb\xbfé bar\n"},
		{"bom_cjk", "\xef\xbb\xbf日本語 foo\n", "\xef\xbb\xbf日本語 bar\n"},
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

			client := lsp.NewLSPClient("/bin/echo", nil)
			client.SetRootDirForTest(root)
			// ApplyWorkspaceEdit writes the file to disk BEFORE the trailing
			// didChange notification; with an unstarted test client only that
			// notification errors (same benign pattern as the internal/lsp
			// apply_edit tests).
			if _, err := client.ApplyWorkspaceEdit(context.Background(), edit); err != nil && !strings.Contains(err.Error(), "not started") {
				t.Fatalf("ApplyWorkspaceEdit: unexpected error: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(root, "u.mqh"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("bytes on disk mismatch:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}
