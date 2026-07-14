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
	if err := client.ApplyWorkspaceEdit(context.Background(), edit); err != nil && !isNotStartedErr(err) {
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
	if err := client.ApplyWorkspaceEdit(context.Background(), edit); err != nil && !isNotStartedErr(err) {
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
