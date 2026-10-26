package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	"github.com/blackwell-systems/agent-lsp/internal/uri"
)

// newFakeClient returns a non-nil *lsp.LSPClient that passes CheckInitialized
// but is not connected to any real server. Use only for tests that short-circuit
// before any LSP network call (argument validation, etc.).
func newFakeClient() *lsp.LSPClient {
	return lsp.NewLSPClient("/bin/echo", nil)
}

// --- isEmptyWorkspaceEdit ---

// TestIsEmptyWorkspaceEdit_Nil verifies that a nil interface{} is empty.
func TestIsEmptyWorkspaceEdit_Nil(t *testing.T) {
	if !isEmptyWorkspaceEdit(nil) {
		t.Error("expected nil to be considered an empty WorkspaceEdit")
	}
}

// TestIsEmptyWorkspaceEdit_TypedNil verifies that a typed nil pointer (which
// marshals to JSON "null") is considered empty.
func TestIsEmptyWorkspaceEdit_TypedNil(t *testing.T) {
	type fakeEdit struct{ Changes map[string]string }
	var p *fakeEdit // typed nil — non-nil interface but marshals to "null"
	if !isEmptyWorkspaceEdit(p) {
		t.Error("expected typed nil pointer to be considered empty (marshals to null)")
	}
}

// TestIsEmptyWorkspaceEdit_EmptyObject verifies that a struct/map with no
// content marshals to "{}" and is treated as empty.
func TestIsEmptyWorkspaceEdit_EmptyObject(t *testing.T) {
	if !isEmptyWorkspaceEdit(map[string]any{}) {
		t.Error("expected empty map to be considered empty (marshals to {})")
	}
}

// TestIsEmptyWorkspaceEdit_NonEmpty verifies that a map with actual edit content
// is not considered empty.
func TestIsEmptyWorkspaceEdit_NonEmpty(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///project/main.go": []any{"some edit"},
		},
	}
	if isEmptyWorkspaceEdit(edit) {
		t.Error("expected non-empty edit to not be considered empty")
	}
}

// --- HandleRenameSymbol ---

// TestHandleRenameSymbol_NilClient verifies that a nil client returns an error
// result before any argument parsing.
func TestHandleRenameSymbol_NilClient(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
		"new_name":  "NewName",
		"line":      float64(5),
		"column":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected 'not initialized' error, got %v", r.Content)
	}
}

// TestHandleRenameSymbol_MissingFilePath verifies that a missing file_path
// returns an error (nil client fires first, but we verify the error path exists).
func TestHandleRenameSymbol_MissingFilePath(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]any{
		"new_name": "NewName",
		"line":     float64(5),
		"column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true")
	}
}

// TestHandleRenameSymbol_MissingNewName verifies that a missing new_name
// returns an error.
func TestHandleRenameSymbol_MissingNewName(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
		"line":      float64(5),
		"column":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing new_name")
	}
}

// TestHandleRenameSymbol_MissingPosition verifies that when neither line/column
// nor position_pattern is supplied, an error result is returned. Uses a fake
// (non-nil) client to reach the position-validation step.
func TestHandleRenameSymbol_MissingPosition(t *testing.T) {
	r, err := HandleRenameSymbol(context.Background(), newFakeClient(), map[string]any{
		"file_path": "/some/file.go",
		"new_name":  "NewName",
		// no line, column, or position_pattern
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing position")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "line") {
		t.Errorf("expected error mentioning 'line', got %v", r.Content)
	}
}

// --- HandlePrepareRename ---

// TestHandlePrepareRename_NilClient verifies that a nil client returns an error result.
func TestHandlePrepareRename_NilClient(t *testing.T) {
	r, err := HandlePrepareRename(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
		"line":      float64(5),
		"column":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandlePrepareRename_MissingFilePath verifies that a missing file_path
// results in an error.
func TestHandlePrepareRename_MissingFilePath(t *testing.T) {
	r, err := HandlePrepareRename(context.Background(), newNilClient(), map[string]any{
		"line":   float64(5),
		"column": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true")
	}
}

// TestHandlePrepareRename_MissingPosition verifies that missing line/column
// returns an error when the client is non-nil (reachable validation step).
func TestHandlePrepareRename_MissingPosition(t *testing.T) {
	r, err := HandlePrepareRename(context.Background(), newFakeClient(), map[string]any{
		"file_path": "/some/file.go",
		// no line or column
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing position")
	}
}

// --- renameSupportsPrepare / prepareRenameNilResult (issue #38) ---

func TestRenameSupportsPrepare(t *testing.T) {
	tests := []struct {
		name string
		caps map[string]any
		want bool
	}{
		{
			name: "bool true is not prepare support",
			caps: map[string]any{"renameProvider": true},
			want: false,
		},
		{
			name: "map with prepareProvider true",
			caps: map[string]any{"renameProvider": map[string]any{"prepareProvider": true}},
			want: true,
		},
		{
			name: "empty options map",
			caps: map[string]any{"renameProvider": map[string]any{}},
			want: false,
		},
		{
			name: "nil caps",
			caps: nil,
			want: false,
		},
		{
			name: "absent renameProvider",
			caps: map[string]any{},
			want: false,
		},
		{
			name: "non-bool prepareProvider junk",
			caps: map[string]any{"renameProvider": map[string]any{"prepareProvider": "yes"}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renameSupportsPrepare(tt.caps); got != tt.want {
				t.Errorf("renameSupportsPrepare(%v) = %v, want %v", tt.caps, got, tt.want)
			}
		})
	}
}

func TestPrepareRenameNilResult(t *testing.T) {
	ctx := context.Background()

	t.Run("supported server returns informational hint", func(t *testing.T) {
		caps := map[string]any{"renameProvider": map[string]any{"prepareProvider": true}}
		r, err := prepareRenameNilResult(ctx, caps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.IsError {
			t.Error("prepareRenameNilResult must not be an ErrorResult")
		}
		if len(r.Content) == 0 || r.Content[0].Text == "" {
			t.Fatal("expected non-empty first content item")
		}
		if len(r.Content) < 2 {
			t.Fatalf("expected a hint as second content item, got %d items", len(r.Content))
		}
		if !strings.HasPrefix(r.Content[1].Text, "Next step: ") {
			t.Errorf("hint = %q, want prefix 'Next step: '", r.Content[1].Text)
		}
	})

	t.Run("unsupported server explains capability gap", func(t *testing.T) {
		r, err := prepareRenameNilResult(ctx, map[string]any{"renameProvider": true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.IsError {
			t.Error("prepareRenameNilResult must not be an ErrorResult")
		}
		if len(r.Content) == 0 || r.Content[0].Text == "" {
			t.Fatal("expected non-empty content")
		}
		if !strings.Contains(r.Content[0].Text, "not supported") {
			t.Errorf("text = %q, want mention of 'not supported'", r.Content[0].Text)
		}
		if len(r.Content) != 1 {
			t.Errorf("expected no hint for unsupported server, got %d items", len(r.Content))
		}
		if !strings.Contains(r.Content[0].Text, "supported\":false") {
			t.Errorf("nil-result responses must be encoded through EncodeResult, got %q", r.Content[0].Text)
		}
	})

	t.Run("method-not-found error yields the unsupported response", func(t *testing.T) {
		// A server that advertises prepareProvider but answers the method with
		// JSON-RPC -32601 must get the unsupported wording, not a tool failure.
		if !lsp.IsMethodNotFound(&lsp.RPCError{Code: -32601, Message: "method not found"}) {
			t.Fatal("IsMethodNotFound must recognize -32601")
		}
		if lsp.IsMethodNotFound(&lsp.RPCError{Code: -32602, Message: "invalid params"}) {
			t.Error("IsMethodNotFound must not match other RPC error codes")
		}
		if lsp.IsMethodNotFound(context.DeadlineExceeded) {
			t.Error("IsMethodNotFound must not match non-RPC errors")
		}
	})
}

// --- HandleFormatDocument ---

// TestHandleFormatDocument_NilClient verifies that a nil client returns an error result.
func TestHandleFormatDocument_NilClient(t *testing.T) {
	r, err := HandleFormatDocument(context.Background(), newNilClient(), map[string]any{
		"file_path": "/some/file.go",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleFormatDocument_MissingFilePath verifies that a missing file_path
// returns an error.
func TestHandleFormatDocument_MissingFilePath(t *testing.T) {
	r, err := HandleFormatDocument(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing file_path")
	}
}

// TestHandleFormatDocument_InvalidTabSize verifies that a non-numeric tab_size
// returns an error. Uses a fake client to reach the tab_size validation step.
func TestHandleFormatDocument_InvalidTabSize(t *testing.T) {
	r, err := HandleFormatDocument(context.Background(), newFakeClient(), map[string]any{
		"file_path": "/some/file.go",
		"tab_size":  "bad",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for non-numeric tab_size")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "tab_size") {
		t.Errorf("expected error mentioning 'tab_size', got %v", r.Content)
	}
}

// --- HandleFormatRange ---

// TestHandleFormatRange_NilClient verifies that a nil client returns an error result.
func TestHandleFormatRange_NilClient(t *testing.T) {
	r, err := HandleFormatRange(context.Background(), newNilClient(), map[string]any{
		"file_path":    "/some/file.go",
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleFormatRange_MissingFilePath verifies that a missing file_path
// returns an error.
func TestHandleFormatRange_MissingFilePath(t *testing.T) {
	r, err := HandleFormatRange(context.Background(), newNilClient(), map[string]any{
		"start_line":   float64(1),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing file_path")
	}
}

// TestHandleFormatRange_InvalidRange verifies that start > end returns an error
// (uses a fake client to reach range validation).
func TestHandleFormatRange_InvalidRange(t *testing.T) {
	r, err := HandleFormatRange(context.Background(), newFakeClient(), map[string]any{
		"file_path":    "/some/file.go",
		"start_line":   float64(5),
		"start_column": float64(1),
		"end_line":     float64(3),
		"end_column":   float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for inverted range")
	}
}

// --- HandleApplyEdit ---

// TestHandleApplyEdit_NilClient verifies that a nil client returns an error result.
func TestHandleApplyEdit_NilClient(t *testing.T) {
	r, err := HandleApplyEdit(context.Background(), newNilClient(), map[string]any{
		"workspace_edit": map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleApplyEdit_MissingWorkspaceEdit verifies that a missing workspace_edit
// argument returns an error.
func TestHandleApplyEdit_MissingWorkspaceEdit(t *testing.T) {
	r, err := HandleApplyEdit(context.Background(), newNilClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing workspace_edit")
	}
}

// TestHandleApplyEdit_TextMatch_RejectsPathOutsideRoot verifies that text-match
// mode (file_path + old_text + new_text) refuses to read/write a file outside
// the client's workspace root, and leaves the out-of-root file untouched.
//
// Regression test: apply_edit's text-match mode previously called os.ReadFile
// on the raw file_path with no root-confinement check.
func TestHandleApplyEdit_TextMatch_RejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()

	secret := filepath.Join(outsideDir, "secret.txt")
	original := "do not touch me"
	if err := os.WriteFile(secret, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	client := newFakeClient()
	client.SetRootDirForTest(root)

	traversal := filepath.Join(root, "..", filepath.Base(outsideDir), "secret.txt")
	r, err := HandleApplyEdit(context.Background(), client, map[string]any{
		"file_path": traversal,
		"old_text":  original,
		"new_text":  "PWNED",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for a file_path outside the workspace root")
	}

	got, readErr := os.ReadFile(secret)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != original {
		t.Fatalf("file outside workspace root was modified: got %q, want unchanged %q", got, original)
	}
}

// TestHandleApplyEdit_TextMatch_AllowsPathInsideRoot is the positive-case
// control: a file genuinely inside the root must still be editable once root
// confinement is enforced.
func TestHandleApplyEdit_TextMatch_AllowsPathInsideRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	client := newFakeClient()
	client.SetRootDirForTest(root)

	r, err := HandleApplyEdit(context.Background(), client, map[string]any{
		"file_path": path,
		"old_text":  "main",
		"new_text":  "other",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// The file is written to disk before the (unstarted, fake) LSP client is
	// notified of the change, so a "not started" error result here still means
	// the in-root edit was allowed through path validation and applied — see
	// isNotStartedErr in internal/lsp/apply_edit_test.go for the same pattern.
	resultText := ""
	if len(r.Content) > 0 {
		resultText = r.Content[0].Text
	}
	if r.IsError && !strings.Contains(resultText, "not started") {
		t.Fatalf("expected success (or a benign not-started notify error) for an in-root path, got: %+v", r)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "package other\n" {
		t.Fatalf("in-root edit did not apply: got %q", got)
	}
}

// --- findText ---

// TestFindText_ExactMatch verifies that an exact substring is found.
func TestFindText_ExactMatch(t *testing.T) {
	src := "line one\nfunc Foo() {\n\treturn nil\n}\n"
	start, end, ok := findText(src, "func Foo() {")
	if !ok {
		t.Fatal("expected match, got not found")
	}
	if src[start:end] != "func Foo() {" {
		t.Errorf("got %q, want %q", src[start:end], "func Foo() {")
	}
}

// TestFindText_NormalisedMatch verifies that a pattern with different indentation
// is found via the whitespace-normalised fallback.
func TestFindText_NormalisedMatch(t *testing.T) {
	src := "line one\n\tfunc Foo() {\n\t\treturn nil\n\t}\n"
	// Pattern has no indentation; file has tabs.
	start, end, ok := findText(src, "func Foo() {\n\treturn nil\n}")
	if !ok {
		t.Fatal("expected normalised match, got not found")
	}
	got := src[start:end]
	if !strings.Contains(got, "func Foo()") {
		t.Errorf("matched region %q does not contain expected content", got)
	}
}

// TestFindText_NotFound verifies that a non-existent pattern returns false.
func TestFindText_NotFound(t *testing.T) {
	_, _, ok := findText("hello world", "xyz")
	if ok {
		t.Error("expected not found")
	}
}

// --- textMatchApply ---

// TestTextMatchApply_ExactMatch verifies that exact-match mode produces a
// WorkspaceEdit with the correct newText.
func TestTextMatchApply_ExactMatch(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.go")
	if err != nil {
		t.Fatal(err)
	}
	content := "package main\n\nfunc Foo() {\n\treturn\n}\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	edit, err := textMatchApply(f.Name(), "func Foo() {", "func Foo() error {")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := edit.(map[string]any)
	if !ok {
		t.Fatal("edit is not map[string]interface{}")
	}
	changes, ok := m["changes"].(map[string]any)
	if !ok {
		t.Fatal("missing changes key")
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 file in changes, got %d", len(changes))
	}
	for _, v := range changes {
		edits, ok := v.([]any)
		if !ok || len(edits) != 1 {
			t.Fatal("expected 1 text edit")
		}
		te := edits[0].(map[string]any)
		if te["newText"] != "func Foo() error {" {
			t.Errorf("got newText %q", te["newText"])
		}
	}
}

// TestTextMatchApply_NotFound verifies an error is returned when old_text is absent.
func TestTextMatchApply_NotFound(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.go")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("package main\n")
	f.Close()

	_, err = textMatchApply(f.Name(), "func Missing() {}", "func Missing() error {}")
	if err == nil {
		t.Error("expected error for missing old_text")
	}
}

// --- issue #18 regression tests (davalillo/agent-lsp#18) ---

// textEditFromTextMatchEdit decodes the single text edit of a textMatchApply
// WorkspaceEdit for assertion.
func textEditFromTextMatchEdit(t *testing.T, edit any) (newText string, rng types.Range) {
	t.Helper()
	m, ok := edit.(map[string]any)
	if !ok {
		t.Fatal("edit is not map[string]any")
	}
	changes, ok := m["changes"].(map[string]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("expected 1 file in changes, got %#v", m["changes"])
	}
	for _, v := range changes {
		edits, ok := v.([]any)
		if !ok || len(edits) != 1 {
			t.Fatal("expected 1 text edit")
		}
		te := edits[0].(map[string]any)
		raw, err := json.Marshal(te["range"])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &rng); err != nil {
			t.Fatal(err)
		}
		newText, _ = te["newText"].(string)
	}
	return newText, rng
}

// writeTextMatchFixture writes content to a temp file and returns its path.
func writeTextMatchFixture(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fixture.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTextMatchApply_ExactMatchReplacementIsVerbatim pins the PR #68
// maintainer-review policy change: on the exact-match path the replacement is
// applied VERBATIM — balancing used to re-attach the span's trailing newline
// (or trim the replacement's), which silently rewrote intended newline
// changes into no-ops that still reported success. An old_text ending in a
// newline replaced by a new_text without one is the caller's ask.
func TestTextMatchApply_ExactMatchReplacementIsVerbatim(t *testing.T) {
	p := writeTextMatchFixture(t, "package main\n\nfunc Foo() {\n\treturn\n}\n\nfunc Bar() {}\n")

	edit, err := textMatchApply(p, "func Foo() {\n\treturn\n}\n", "func Foo() error {\n\treturn nil\n}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	newText, _ := textEditFromTextMatchEdit(t, edit)
	if want := "func Foo() error {\n\treturn nil\n}"; newText != want {
		t.Errorf("exact-match replacement must be verbatim:\n got %q\nwant %q", newText, want)
	}
}

// TestTextMatchApply_NewTextTrailingNewlineApplied verifies the verbatim
// policy on the exact-match path (PR #68 maintainer review): a new_text that
// ends with a newline the old_text span does not include is applied as asked
// — the caller explicitly requested the extra line break — and the splice
// inserts it.
func TestTextMatchApply_NewTextTrailingNewlineApplied(t *testing.T) {
	content := "package main\n\nfunc Foo() {}\n\nfunc Bar() {}\n"
	p := writeTextMatchFixture(t, content)

	edit, err := textMatchApply(p, "func Foo() {}", "func Foo() error {}\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	newText, rng := textEditFromTextMatchEdit(t, edit)
	if want := "func Foo() error {}\n"; newText != want {
		t.Errorf("new_text trailing newline must not be trimmed:\n got %q\nwant %q", newText, want)
	}
	if got := uri.ApplyRangeEdit(content, rng, newText); got != "package main\n\nfunc Foo() error {}\n\n\nfunc Bar() {}\n" {
		t.Errorf("unexpected spliced content: %q", got)
	}
}

// The FALLBACK path keeps the opposite rule: its span is the full matched
// lines (including their trailing newline), which is NOT what the caller
// quoted, so the replacement is EOL-balanced against the span and the next
// line must not fuse onto the replacement (the original issue #18 bug).
func TestTextMatchApply_NormalizedFallbackBalancesTrailingEOL(t *testing.T) {
	content := "package main\n\n  foo()\n\treturn\n"
	p := writeTextMatchFixture(t, content)

	// "    foo()" (wrong indentation, not an exact substring) only matches the
	// whitespace-normalised full-line path, whose span is "  foo()\n".
	edit, err := textMatchApply(p, "    foo()", "\t\tbar()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	newText, rng := textEditFromTextMatchEdit(t, edit)
	if want := "\t\tbar()\n"; newText != want {
		t.Errorf("fallback replacement must re-attach the span's newline:\n got %q\nwant %q", newText, want)
	}
	if got := uri.ApplyRangeEdit(content, rng, newText); got != "package main\n\n\t\tbar()\n\treturn\n" {
		t.Errorf("unexpected spliced content: %q", got)
	}
}

// TestTextMatchApply_CRLFExactMatch verifies issue #18 fix 4: an old_text
// written with LF endings matches a CRLF region, the file's CRLF endings are
// preserved in the replacement, and the produced range applies cleanly.
func TestTextMatchApply_CRLFExactMatch(t *testing.T) {
	content := "package main\r\n\r\nfunc Foo() {\r\n\treturn\r\n}\r\n\r\nfunc Bar() {}\r\n"
	p := writeTextMatchFixture(t, content)

	edit, err := textMatchApply(p, "func Foo() {\n\treturn\n}", "func Foo() error {\n\treturn nil\n}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	newText, rng := textEditFromTextMatchEdit(t, edit)
	got := uri.ApplyRangeEdit(content, rng, newText)
	want := "package main\r\n\r\nfunc Foo() error {\r\n\treturn nil\r\n}\r\n\r\nfunc Bar() {}\r\n"
	if got != want {
		t.Errorf("CRLF endings corrupted:\n got %q\nwant %q", got, want)
	}
}

// TestTextMatchApply_BOMDoesNotShiftOffsets verifies issue #18 fix 4: a
// leading UTF-8 BOM is part of src, so strings.Index and byteSpanToLSPRange
// agree and the edit lands at the intended site rather than being shifted.
func TestTextMatchApply_BOMDoesNotShiftOffsets(t *testing.T) {
	content := "\xef\xbb\xbfpackage main\n\nfunc Foo() {\n\treturn\n}\n"
	p := writeTextMatchFixture(t, content)

	edit, err := textMatchApply(p, "func Foo() {", "func Foo() error {")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	newText, rng := textEditFromTextMatchEdit(t, edit)
	if newText != "func Foo() error {" {
		t.Errorf("got newText %q", newText)
	}
	if rng.Start.Line != 2 || rng.Start.Character != 0 || rng.End.Line != 2 || rng.End.Character != 12 {
		t.Errorf("unexpected range: %#v", rng)
	}
	if got := uri.ApplyRangeEdit(content, rng, newText); got != "\xef\xbb\xbfpackage main\n\nfunc Foo() error {\n\treturn\n}\n" {
		t.Errorf("edit shifted by BOM:\n got %q", got)
	}
}

// fakeTextMatchApplyFn returns an applier that mimics LSPClient.ApplyWorkspaceEdit
// for one file: it re-reads the file from disk, refuses the write when the
// content no longer matches the edit's x-expected-snapshot precondition (the
// same compare-and-swap applyEditsToFile now performs), splices the edit's
// LSP ranges with the canonical uri.ApplyRangeEdit (the same path
// applyEditsToFile uses), and writes the result back — without any language
// server.
func fakeTextMatchApplyFn(filePath string) func(context.Context, textMatchPlan) error {
	return func(_ context.Context, plan textMatchPlan) error {
		edit := plan.edit
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		content := string(data)
		m, ok := edit.(map[string]any)
		if !ok {
			return fmt.Errorf("edit is not map[string]any")
		}
		// Compare-and-swap precondition, mirroring applyEditsToFile.
		if snap, _ := m["x-expected-snapshot"].(string); snap != "" && snap != content {
			return fmt.Errorf("applyEdit: %s changed since the edit was computed (concurrent modification); NOTHING was written — re-run against the current file contents", filePath)
		}
		changes, ok := m["changes"].(map[string]any)
		if !ok {
			return fmt.Errorf("edit is missing changes")
		}
		for u, v := range changes {
			p, err := URIToFilePath(u)
			if err != nil {
				return err
			}
			if p != filePath {
				return fmt.Errorf("unexpected edit uri %q", u)
			}
			edits, ok := v.([]any)
			if !ok {
				return fmt.Errorf("changes[%q] is not a list", u)
			}
			for _, e := range edits {
				te, ok := e.(map[string]any)
				if !ok {
					return fmt.Errorf("text edit is not map[string]any")
				}
				raw, err := json.Marshal(te["range"])
				if err != nil {
					return err
				}
				var rng types.Range
				if err := json.Unmarshal(raw, &rng); err != nil {
					return err
				}
				nt, _ := te["newText"].(string)
				content = uri.ApplyRangeEdit(content, rng, nt)
			}
		}
		return os.WriteFile(filePath, []byte(content), 0o644)
	}
}

// concatResultText concatenates the text content of a tool result.
func concatResultText(r types.ToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		b.WriteString(c.Text)
	}
	return b.String()
}

// TestApplyTextMatchEdit_ConcurrentAppliesLandAtIntendedSites verifies issue
// #18 fix 2: two concurrent text-match applies to the SAME file must both land
// at their intended sites, instead of the second one applying a range computed
// against the pre-edit disk snapshot (observed: it replaced the file's first
// two lines). Run with -race.
func TestApplyTextMatchEdit_ConcurrentAppliesLandAtIntendedSites(t *testing.T) {
	p := filepath.Join(t.TempDir(), "shared.go")
	src := "alpha one\nmiddle\nomega one\n"
	for i := 0; i < 25; i++ {
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, err := applyTextMatchEdit(context.Background(), p, "alpha one", "alpha TWO", fakeTextMatchApplyFn(p))
			errs[0] = err
		}()
		go func() {
			defer wg.Done()
			<-start
			_, err := applyTextMatchEdit(context.Background(), p, "omega one", "omega TWO", fakeTextMatchApplyFn(p))
			errs[1] = err
		}()
		close(start)
		wg.Wait()
		for j, err := range errs {
			if err != nil {
				t.Fatalf("iteration %d goroutine %d: %v", i, j, err)
			}
		}
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if want := "alpha TWO\nmiddle\nomega TWO\n"; string(got) != want {
			t.Fatalf("iteration %d: concurrent applies mis-located:\n got %q\nwant %q", i, got, want)
		}
	}
}

// TestApplyTextMatchEdit_VerificationFailureNamesFile verifies issue #18 fix 3:
// when the file on disk does not contain the replacement at the intended site
// after the apply step, the tool fails with an error naming the file (which
// HandleApplyEdit converts into an error result) instead of a bare success.
func TestApplyTextMatchEdit_VerificationFailureNamesFile(t *testing.T) {
	p := writeTextMatchFixture(t, "package main\n\nfunc Foo() {\n}\n")
	// An applier that reports success but writes nothing: the post-apply
	// verification must catch that the replacement never landed.
	noop := func(context.Context, textMatchPlan) error { return nil }
	_, err := applyTextMatchEdit(context.Background(), p, "func Foo() {", "func Foo() error {", noop)
	if err == nil {
		t.Fatal("expected an error when post-apply verification fails")
	}
	if !strings.Contains(err.Error(), p) {
		t.Errorf("error should name the file %s, got %q", p, err.Error())
	}
}

// TestApplyTextMatchEdit_ReplacementEmbedsOldText verifies that a replacement
// which legitimately contains the old text (e.g. "return nil" ->
// "return nil // guard") is not falsely rejected by the post-apply
// verification: the absence check must exclude the applied site.
func TestApplyTextMatchEdit_ReplacementEmbedsOldText(t *testing.T) {
	p := writeTextMatchFixture(t, "package main\n\nfunc Foo() error {\n\treturn nil\n}\n")

	res, err := applyTextMatchEdit(context.Background(), p, "return nil", "return nil // guard", fakeTextMatchApplyFn(p))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("verification falsely rejected an embedded replacement: %s", concatResultText(res))
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := "package main\n\nfunc Foo() error {\n\treturn nil // guard\n}\n"; string(got) != want {
		t.Errorf("unexpected file content:\n got %q\nwant %q", got, want)
	}
}

// TestApplyTextMatchEdit_SuccessResultIncludesSite verifies issue #18 fix 3:
// the success result keeps the "Edit applied successfully" prefix (existing
// callers) and adds the file path plus the byte range where new_text now
// lives.
func TestApplyTextMatchEdit_SuccessResultIncludesSite(t *testing.T) {
	p := writeTextMatchFixture(t, "package main\n\nfunc Foo() {\n\treturn\n}\n")

	res, err := applyTextMatchEdit(context.Background(), p, "func Foo() {\n\treturn\n}\n", "func Foo() error {\n\treturn nil\n}", fakeTextMatchApplyFn(p))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", concatResultText(res))
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := "package main\n\nfunc Foo() error {\n\treturn nil\n}"; string(got) != want {
		t.Errorf("unexpected file content:\n got %q\nwant %q", got, want)
	}
	text := concatResultText(res)
	if !strings.Contains(text, "Edit applied successfully") {
		t.Errorf("success prefix missing: %q", text)
	}
	if !strings.Contains(text, p) {
		t.Errorf("file path missing: %q", text)
	}
	// The exact-match replacement is verbatim (PR #68 policy change): the
	// caller's new_text has no trailing newline, so the site is bytes 14-46
	// ("package main\n\n" is 14 bytes; the replacement is 32 bytes long).
	if !strings.Contains(text, "bytes 14-46") {
		t.Errorf("byte range missing: %q", text)
	}
}

// TestApplyTextMatchEdit_NoChangeReportedNotSuccess pins the PR #68
// maintainer-review rule: when EOL handling makes the replacement
// byte-identical to the matched span (here the normalised full-line span is
// restored by balancing), the tool must say NOTHING changed instead of
// reporting "Edit applied successfully" over an untouched file.
func TestApplyTextMatchEdit_NoChangeReportedNotSuccess(t *testing.T) {
	p := writeTextMatchFixture(t, "package main\n\n  foo()\n\treturn\n")
	applied := false
	res, err := applyTextMatchEdit(context.Background(), p, "    foo()", "  foo()", func(context.Context, textMatchPlan) error {
		applied = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("a no-op must not be an error result: %s", concatResultText(res))
	}
	if applied {
		t.Errorf("the edit must not be applied when the replacement is byte-identical to the span")
	}
	text := concatResultText(res)
	if !strings.Contains(text, "No change applied") {
		t.Errorf("no-change result missing: %q", text)
	}
	if strings.Contains(text, "Edit applied successfully") {
		t.Errorf("a no-op must not report success: %q", text)
	}
}

// TestApplyTextMatchEdit_ConcurrentWriterRefusedByCAS pins the PR #68
// maintainer-review fix: a writer outside the per-file lock landing between
// the plan's read and the apply must NOT corrupt the file. The applier here
// simulates the external write and then performs the same
// compare-against-snapshot the real applyEditsToFile does: the write is
// refused and the external writer's content survives intact.
func TestApplyTextMatchEdit_ConcurrentWriterRefusedByCAS(t *testing.T) {
	p := writeTextMatchFixture(t, "line1\nTARGET\nline3\nline4\n")
	src, _ := os.ReadFile(p)
	externalWrite := []byte("INSERTED\nDONE\nline2\nTARGET\nline4\n")
	_, err := applyTextMatchEdit(context.Background(), p, "TARGET", "REPLACED", func(_ context.Context, plan textMatchPlan) error {
		// The concurrent writer lands here, between the plan's read and the
		// apply.
		if wErr := os.WriteFile(p, externalWrite, 0o644); wErr != nil {
			return wErr
		}
		// The real client's CAS: read once, refuse unless it still matches
		// the plan's snapshot.
		cur, rErr := os.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		if string(cur) != plan.src {
			return fmt.Errorf("applyEdit: %s changed since the edit was computed (concurrent modification); NOTHING was written — re-run against the current file contents", p)
		}
		return nil
	})
	if err == nil {
		t.Fatal("the CAS precondition must refuse the stale edit")
	}
	if !strings.Contains(err.Error(), "changed since the edit was computed") {
		t.Errorf("unexpected refusal error: %v", err)
	}
	after, _ := os.ReadFile(p)
	if string(after) != string(externalWrite) {
		t.Errorf("the refused edit must leave the concurrent writer's content intact, got %q", after)
	}
	if bytes.Contains(after, []byte("REPLACED")) {
		t.Errorf("the refused edit must not splice anything, got %q", after)
	}
	_ = src
}

// TestApplyWorkspaceEditCASRefusesChangedFile exercises the real client-side
// precondition: ApplyWorkspaceEdit with an x-expected-snapshot must refuse
// (and write nothing) when the file changed since the edit was computed.
func TestApplyWorkspaceEditCASRefusesChangedFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := lsp.NewLSPClient("fake", nil)
	c.SetRootDirForTest(root)
	edit := map[string]any{
		"changes": map[string]any{
			CreateFileURI(p): []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 8},
					},
					"newText": "REPLACED",
				},
			},
		},
		"x-expected-snapshot": "original\n",
	}
	// The file changes after the plan was computed.
	if err := os.WriteFile(p, []byte("modified by someone else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyWorkspaceEdit(context.Background(), edit); err == nil {
		t.Fatal("ApplyWorkspaceEdit must refuse a stale snapshot")
	} else if !strings.Contains(err.Error(), "changed since the edit was computed") {
		t.Errorf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "modified by someone else\n" {
		t.Errorf("the refused edit must write nothing, got %q", got)
	}

	// Matching snapshot: the edit applies.
	if err := os.WriteFile(p, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Matching snapshot: the edit applies. (The trailing didChange fails on
	// this fake, unstarted client — the file write is what this test pins.)
	if err := c.ApplyWorkspaceEdit(context.Background(), edit); err != nil && !strings.Contains(err.Error(), "LSP client not started") {
		t.Fatalf("matching snapshot must apply: %v", err)
	}
	got, _ = os.ReadFile(p)
	if string(got) != "REPLACED\n" {
		t.Errorf("unexpected content after apply: %q", got)
	}
}

// TestApplyTextMatchEdit_AbsenceCheckDoesNotJoinBoundaries pins the CodeRabbit
// fix: deleting "()" from "(())" applies correctly and must not be rejected by
// an absence check that forms a false match by concatenating the text before
// and after the site (")" + "(" joins into "()").
func TestApplyTextMatchEdit_AbsenceCheckDoesNotJoinBoundaries(t *testing.T) {
	p := writeTextMatchFixture(t, "f(())\n")
	res, err := applyTextMatchEdit(context.Background(), p, "()", "", fakeTextMatchApplyFn(p))
	if err != nil {
		t.Fatalf("a correct deletion must not be rejected: %v", err)
	}
	if res.IsError {
		t.Fatalf("verification falsely rejected a boundary join: %s", concatResultText(res))
	}
	got, _ := os.ReadFile(p)
	if string(got) != "f()\n" {
		t.Errorf("unexpected file content: %q", got)
	}
}

// TestApplyTextMatchEdit_LockHonorsContextAndPrunes pins the PR #68
// maintainer-review lock fixes: waiting for the per-file lock honors the
// caller's context, and the lock map is pruned once the last holder leaves.
func TestApplyTextMatchEdit_LockHonorsContextAndPrunes(t *testing.T) {
	p := writeTextMatchFixture(t, "a\nb\n")
	// Hold the lock from another goroutine.
	release, lerr := lockTextMatchFile(context.Background(), p)
	if lerr != nil {
		t.Fatal(lerr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := applyTextMatchEdit(ctx, p, "a", "b", fakeTextMatchApplyFn(p))
	release()
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("a cancelled caller must be released from the lock wait, got %v", err)
	}
	// The cancelled waiter must not leak its reference: give the drain
	// goroutine a moment, then the map entry must be pruned once released.
	time.Sleep(100 * time.Millisecond)
	textMatchFileLocksMu.Lock()
	left := len(textMatchFileLocks)
	textMatchFileLocksMu.Unlock()
	if left != 0 {
		t.Errorf("lock map must be pruned after the last holder/waiter leaves, %d entries left", left)
	}
	// And a fresh apply still works.
	if _, err := applyTextMatchEdit(context.Background(), p, "a", "b", fakeTextMatchApplyFn(p)); err != nil {
		t.Fatalf("apply after the cancelled waiter must succeed: %v", err)
	}
}

// --- HandleExecuteCommand ---

// TestHandleExecuteCommand_NilClient verifies that a nil client returns an error result.
func TestHandleExecuteCommand_NilClient(t *testing.T) {
	r, err := HandleExecuteCommand(context.Background(), newNilClient(), map[string]any{
		"command": "editor.action.foo",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for nil client")
	}
}

// TestHandleExecuteCommand_MissingCommand verifies that a missing command argument
// returns an error. Uses a fake (non-nil) client to reach the command validation step.
func TestHandleExecuteCommand_MissingCommand(t *testing.T) {
	r, err := HandleExecuteCommand(context.Background(), newFakeClient(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Error("expected IsError=true for missing command")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "command") {
		t.Errorf("expected error mentioning 'command', got %v", r.Content)
	}
}

// --- filterWorkspaceEditByGlobs ---

// TestFilterWorkspaceEditByGlobs_Nil verifies that a nil result passes through unchanged.
func TestFilterWorkspaceEditByGlobs_Nil(t *testing.T) {
	result := filterWorkspaceEditByGlobs(nil, []string{"vendor/**"})
	if result != nil {
		t.Error("expected nil to pass through unchanged")
	}
}

// TestFilterWorkspaceEditByGlobs_EmptyGlobs verifies that empty globs return the input unchanged.
func TestFilterWorkspaceEditByGlobs_EmptyGlobs(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///project/main.go": []any{"edit1"},
		},
	}
	result := filterWorkspaceEditByGlobs(edit, nil)
	// result should be the same interface value (same underlying pointer) as edit.
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map[string]interface{} result")
	}
	if len(resultMap) != len(edit) {
		t.Error("expected nil globs to return input unchanged")
	}
}

// TestFilterWorkspaceEditByGlobs_RetainsNonMatchingChanges verifies non-matching
// URIs are retained in the "changes" map format.
func TestFilterWorkspaceEditByGlobs_RetainsNonMatchingChanges(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///project/main.go":     []any{"edit1"},
			"file:///project/main_gen.go": []any{"edit2"},
		},
	}
	result := filterWorkspaceEditByGlobs(edit, []string{"*_gen.go"})
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	changes, _ := m["changes"].(map[string]any)
	if _, found := changes["file:///project/main.go"]; !found {
		t.Error("expected main.go to be retained")
	}
	if _, found := changes["file:///project/main_gen.go"]; found {
		t.Error("expected main_gen.go to be excluded by *_gen.go pattern")
	}
}

// --- extractStringSlice ---

// TestExtractStringSlice_Typed verifies []string input is returned as-is.
func TestExtractStringSlice_Typed(t *testing.T) {
	args := map[string]any{
		"exclude_globs": []string{"vendor/**", "*_gen.go"},
	}
	got := extractStringSlice(args, "exclude_globs")
	if len(got) != 2 || got[0] != "vendor/**" {
		t.Errorf("unexpected result: %v", got)
	}
}

// TestExtractStringSlice_Interface verifies []interface{} (JSON-decoded shape) works.
func TestExtractStringSlice_Interface(t *testing.T) {
	args := map[string]any{
		"exclude_globs": []any{"vendor/**", "*_gen.go"},
	}
	got := extractStringSlice(args, "exclude_globs")
	if len(got) != 2 || got[1] != "*_gen.go" {
		t.Errorf("unexpected result: %v", got)
	}
}

// TestExtractStringSlice_Missing verifies a missing key returns nil.
func TestExtractStringSlice_Missing(t *testing.T) {
	args := map[string]any{}
	got := extractStringSlice(args, "exclude_globs")
	if got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

// TestSummarizeWorkspaceEdit_Changes counts files and locations in the "changes" form.
func TestSummarizeWorkspaceEdit_Changes(t *testing.T) {
	edit := map[string]any{
		"changes": map[string]any{
			"file:///proj/a/Main.java": []any{map[string]any{}, map[string]any{}, map[string]any{}},
			"file:///proj/b/Util.java": []any{map[string]any{}},
		},
	}
	files, locations, names := summarizeWorkspaceEdit(edit)
	if files != 2 {
		t.Errorf("files = %d, want 2", files)
	}
	if locations != 4 {
		t.Errorf("locations = %d, want 4", locations)
	}
	// Basenames, sorted deterministically.
	if strings.Join(names, ",") != "Main.java,Util.java" {
		t.Errorf("names = %v, want [Main.java Util.java]", names)
	}
}

// TestSummarizeWorkspaceEdit_DocumentChanges counts the documentChanges form.
func TestSummarizeWorkspaceEdit_DocumentChanges(t *testing.T) {
	edit := map[string]any{
		"documentChanges": []any{
			map[string]any{
				"textDocument": map[string]any{"uri": "file:///proj/x.ts"},
				"edits":        []any{map[string]any{}, map[string]any{}},
			},
		},
	}
	files, locations, names := summarizeWorkspaceEdit(edit)
	if files != 1 || locations != 2 {
		t.Errorf("got files=%d locations=%d, want 1/2", files, locations)
	}
	if len(names) != 1 || names[0] != "x.ts" {
		t.Errorf("names = %v, want [x.ts]", names)
	}
}

// TestSummarizeWorkspaceEdit_Empty returns zeros for nil/empty edits.
func TestSummarizeWorkspaceEdit_Empty(t *testing.T) {
	files, locations, names := summarizeWorkspaceEdit(map[string]any{})
	if files != 0 || locations != 0 || names != nil {
		t.Errorf("got files=%d locations=%d names=%v, want 0/0/nil", files, locations, names)
	}
}

// TestWorkspaceEditURIs verifies the URI extraction used to verify every file
// touched by a rename: both the "changes" map and the "documentChanges" list,
// deduplicated and sorted. (issue #44 review follow-up)
func TestWorkspaceEditURIs(t *testing.T) {
	changes := map[string]any{
		"changes": map[string]any{
			"file:///b.go": []map[string]any{{"range": map[string]any{}, "newText": "x"}},
			"file:///a.go": []map[string]any{{"range": map[string]any{}, "newText": "y"}},
		},
	}
	got := workspaceEditURIs(changes)
	want := []string{"file:///a.go", "file:///b.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("workspaceEditURIs(changes) = %v, want %v", got, want)
	}

	docChanges := map[string]any{
		"documentChanges": []map[string]any{
			{"textDocument": map[string]any{"uri": "file:///c.go"}, "edits": []any{}},
			{"textDocument": map[string]any{"uri": "file:///a.go"}, "edits": []any{}},
		},
	}
	got = workspaceEditURIs(docChanges)
	want = []string{"file:///a.go", "file:///c.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("workspaceEditURIs(documentChanges) = %v, want %v", got, want)
	}

	if got := workspaceEditURIs(map[string]any{}); len(got) != 0 {
		t.Errorf("workspaceEditURIs(empty) = %v, want none", got)
	}
}

// TestEditedURIsToVerify_SkipsRequestedAndOutOfRootURIs verifies that rename's
// multi-file channel check never reopens the requested file (the caller already
// verified it) or a URI outside the workspace root. ReopenDocument reads an
// untracked URI from disk and sends it to the server, and the ignored form of
// the edit (changes when documentChanges is present) is never validated by
// ApplyWorkspaceEdit.
func TestEditedURIsToVerify_SkipsRequestedAndOutOfRootURIs(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	requested := filepath.Join(root, "main.go")
	sibling := filepath.Join(root, "pkg", "util.go")

	edit := map[string]any{
		"changes": map[string]any{
			CreateFileURI(requested): []any{},
			CreateFileURI(sibling):   []any{},
			CreateFileURI(outside):   []any{},
		},
	}
	got := editedURIsToVerify(edit, requested, root)
	want := []string{CreateFileURI(sibling)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("editedURIsToVerify = %v, want %v", got, want)
	}
}

// TestVerifyWorkspaceEditChannels_SharesOneWait verifies that a dead
// diagnostics channel costs one bounded wait for the whole rename, not one wait
// per edited file.
func TestVerifyWorkspaceEditChannels_SharesOneWait(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	// A server whose diagnostics channel is dead: it answers requests with null
	// and never sends textDocument/publishDiagnostics.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := lsp.NewFrameReader(conn)
		for {
			raw, err := r.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				ID     *float64 `json:"id"`
				Method string   `json:"method"`
			}
			if json.Unmarshal(raw, &msg) != nil {
				return
			}
			if msg.ID == nil || msg.Method == "" {
				continue
			}
			body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
			if _, err := conn.Write(lsp.EncodeMessage(body)); err != nil {
				return
			}
		}
	}()

	client, err := lsp.NewPassiveClient(ln.Addr().String())
	if err != nil {
		t.Fatalf("NewPassiveClient: %v", err)
	}
	defer client.Shutdown(context.Background())
	root := t.TempDir()
	client.MarkInitializedForTest()
	client.SetRootDirForTest(root)

	const files = 5
	changes := map[string]any{}
	for i := 0; i < files; i++ {
		p := filepath.Join(root, fmt.Sprintf("f%d.go", i))
		if err := os.WriteFile(p, []byte("package p\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		changes[CreateFileURI(p)] = []any{}
	}

	saved := editChannelWaitMs
	editChannelWaitMs = 300
	defer func() { editChannelWaitMs = saved }()

	start := time.Now()
	unverified := verifyWorkspaceEditChannels(context.Background(), client,
		map[string]any{"changes": changes}, filepath.Join(root, "requested.go"))
	elapsed := time.Since(start)

	if len(unverified) != files {
		t.Errorf("unverified = %d files, want %d (dead channel)", len(unverified), files)
	}
	// One shared wait is ~300ms; waiting per file would take at least 1500ms.
	if elapsed > time.Second {
		t.Errorf("verification took %v for %d files; the wait must be shared, not per file", elapsed, files)
	}
}
