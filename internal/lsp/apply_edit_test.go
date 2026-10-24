package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isNotStartedErr reports whether err is the benign "client not started" error
// from the trailing textDocument/didChange notification. ApplyWorkspaceEdit
// writes the file to disk BEFORE notifying the server, so with an unstarted test
// client the on-disk result is already correct and only the notify step errors.
func isNotStartedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not started")
}

// TestApplyWorkspaceEdit_MultiEditRename applies a well-behaved multi-location
// rename (the common case: several small TextEdits over one file) and asserts the
// file on disk is renamed correctly with no corruption. This is the path
// rename_symbol now drives server-side instead of handing the edit back to the
// caller. See issue #12.
func TestApplyWorkspaceEdit_MultiEditRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nvar LOGGER = 1\n\nfunc use() int { return LOGGER + LOGGER }\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	uri := PathToFileURI(path)

	// Rename LOGGER -> MYLOGGER at its three occurrences (line/character are
	// 0-based, matching LSP). Order is intentionally not reverse-sorted;
	// applyEditsToFile applies bottom-to-top itself.
	edit := map[string]any{
		"changes": map[string]any{
			uri: []any{
				textEditJSON(2, 4, 2, 10, "MYLOGGER"),  // var LOGGER
				textEditJSON(4, 24, 4, 30, "MYLOGGER"), // return LOGGER
				textEditJSON(4, 33, 4, 39, "MYLOGGER"), // + LOGGER
			},
		},
	}

	client := NewLSPClient("/bin/echo", nil)
	if _, err := client.ApplyWorkspaceEdit(context.Background(), edit); err != nil && !isNotStartedErr(err) {
		t.Fatalf("ApplyWorkspaceEdit: unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\nvar MYLOGGER = 1\n\nfunc use() int { return MYLOGGER + MYLOGGER }\n"
	if string(got) != want {
		t.Errorf("file corrupted after rename apply:\n got  %q\n want %q", string(got), want)
	}
	if strings.Count(string(got), "LOGGER") != 3 || strings.Count(string(got), "MYLOGGER") != 3 {
		t.Errorf("wrong occurrence counts: %q", string(got))
	}
}

// TestApplyWorkspaceEdit_SingleBigEdit reproduces the exact jdtls shape from
// issue #12: one TextEdit whose range spans many lines and whose newText carries
// the whole replaced span verbatim (newlines, quotes, and a pipe). Applying it
// server-side must reproduce the span byte-for-byte — this is precisely the edit
// that got transposed/truncated when the LLM had to copy it back through GCF.
func TestApplyWorkspaceEdit_SingleBigEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Main.java")
	// A file whose lines 2..4 will be replaced wholesale by one edit.
	src := "class Main {\n" +
		"  static Logger LOGGER = get();\n" +
		"  double calc(double a) {\n" +
		"    if (a > 0 | a < 10) { return a; }\n" +
		"  }\n" +
		"}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	uri := PathToFileURI(path)

	// Replace from line 1 col 16 (the "LOGGER" declaration onward) through line 3
	// col 37 (end of the if-statement line) with a verbatim multi-line newText.
	newText := "MYLOGGER = get();\n" +
		"  double calc(double a) {\n" +
		"    if (a > 0 | a < 10) { return a * 2; }"
	edit := map[string]any{
		"changes": map[string]any{
			uri: []any{
				textEditJSON(1, 16, 3, 37, newText),
			},
		},
	}

	client := NewLSPClient("/bin/echo", nil)
	if _, err := client.ApplyWorkspaceEdit(context.Background(), edit); err != nil && !isNotStartedErr(err) {
		t.Fatalf("ApplyWorkspaceEdit: unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "class Main {\n" +
		"  static Logger MYLOGGER = get();\n" +
		"  double calc(double a) {\n" +
		"    if (a > 0 | a < 10) { return a * 2; }\n" +
		"  }\n" +
		"}\n"
	if string(got) != want {
		t.Errorf("big-edit apply corrupted the file:\n got  %q\n want %q", string(got), want)
	}
	// The verbatim newText (newlines, pipe) must survive intact.
	if !strings.Contains(string(got), "a > 0 | a < 10") || !strings.Contains(string(got), "return a * 2;") {
		t.Errorf("verbatim newText not preserved: %q", string(got))
	}
}

// TestApplyWorkspaceEdit_SortedOrderAndWrittenOnPartialFailure pins review
// #60 finding 2: files in a changes map are applied in sorted URI order (Go
// map iteration is randomized) and a mid-batch failure reports what was
// written instead of dropping the information.
func TestApplyWorkspaceEdit_SortedOrderAndWrittenOnPartialFailure(t *testing.T) {
	dir := t.TempDir()
	// a.mqh is a directory where a file is expected: reading it fails
	// deterministically BEFORE anything is written, proving it is processed
	// first under sorted order. With randomized map iteration b.mqh would go
	// first about half the time and its trailing didChange error ("not
	// started") would surface instead.
	aPath := filepath.Join(dir, "a.mqh")
	if err := os.Mkdir(aPath, 0o755); err != nil {
		t.Fatal(err)
	}
	bPath := filepath.Join(dir, "b.mqh")
	if err := os.WriteFile(bPath, []byte("foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	edit := map[string]any{
		"changes": map[string]any{
			PathToFileURI(bPath): []any{textEditJSON(0, 0, 0, 3, "bar")},
			PathToFileURI(aPath): []any{textEditJSON(0, 0, 0, 3, "bar")},
		},
	}
	client := NewLSPClient("/bin/echo", nil)
	written, err := client.ApplyWorkspaceEdit(context.Background(), edit)
	if err == nil {
		t.Fatal("expected error from the unreadable sorted-first file")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected the sorted-first file (directory) to fail the batch, got: %v", err)
	}
	if len(written) != 0 {
		t.Fatalf("nothing reported written before the failure, got %v", written)
	}
	got, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "foo\n" {
		t.Fatalf("later file must be untouched after mid-batch failure: %q", got)
	}
}

// TestApplyWorkspaceEdit_WrittenReportedAfterNotifyError pins the accounting
// semantics: a file whose bytes were written but whose trailing didChange
// notification failed (unstarted test client) still counts as written, and
// the batch stops there — later files stay untouched.
func TestApplyWorkspaceEdit_WrittenReportedAfterNotifyError(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.mqh")
	bPath := filepath.Join(dir, "b.mqh")
	for _, p := range []string{aPath, bPath} {
		if err := os.WriteFile(p, []byte("foo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	edit := map[string]any{
		"changes": map[string]any{
			PathToFileURI(bPath): []any{textEditJSON(0, 0, 0, 3, "bar")},
			PathToFileURI(aPath): []any{textEditJSON(0, 0, 0, 3, "bar")},
		},
	}
	client := NewLSPClient("/bin/echo", nil)
	written, err := client.ApplyWorkspaceEdit(context.Background(), edit)
	if err == nil {
		t.Fatal("expected the trailing didChange error from the unstarted client")
	}
	if len(written) != 1 || !strings.HasSuffix(written[0], "/a.mqh") {
		t.Fatalf("expected exactly a.mqh reported written, got %v", written)
	}
	got, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bar\n" {
		t.Fatalf("reported-written file must hold the new bytes: %q", got)
	}
	gotB, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotB) != "foo\n" {
		t.Fatalf("later file must be untouched after mid-batch failure: %q", gotB)
	}
}

// textEditJSON builds an LSP TextEdit as a JSON-shaped map with 0-based
// line/character coordinates.
func textEditJSON(startLine, startChar, endLine, endChar int, newText string) map[string]any {
	return map[string]any{
		"range": map[string]any{
			"start": map[string]any{"line": startLine, "character": startChar},
			"end":   map[string]any{"line": endLine, "character": endChar},
		},
		"newText": newText,
	}
}
