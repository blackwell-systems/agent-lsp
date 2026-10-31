package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentLanguageID_DerivesPlaintext(t *testing.T) {
	for _, tc := range []struct{ file, given, want string }{
		{"a.go", "plaintext", "go"},
		{"a.go", "", "go"},
		{"a.gleam", "plaintext", "gleam"},
		{"a.tsx", "plaintext", "typescriptreact"},
		{"a.py", "plaintext", "python"},
		{"notes.unknown", "plaintext", "plaintext"},
		{"a.go", "python", "python"}, // an explicit language is kept
		{"a.tsx", "typescript", "typescriptreact"},
	} {
		if got := documentLanguageID(tc.file, tc.given); got != tc.want {
			t.Errorf("documentLanguageID(%q, %q) = %q, want %q", tc.file, tc.given, got, tc.want)
		}
	}
}

// Tools that omit language_id open documents as "plaintext". The document must
// reach the server under its real language, both on didOpen and when it is
// reopened later from the tracked metadata.
func TestOpenDocument_PlaintextUsesExtensionLanguage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "person.gleam")
	if err := os.WriteFile(file, []byte("pub fn main() { Nil }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	uri := PathToFileURI(file)
	c, _, output := newTestClient(t)
	ctx := context.Background()

	done := make(chan error, 1)
	go func() { done <- c.OpenDocument(ctx, uri, "pub fn main() { Nil }\n", "plaintext") }()
	if got := didOpenLanguage(t, readNextMsg(t, output)); got != "gleam" {
		t.Fatalf("didOpen languageId = %q, want gleam", got)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	go func() { done <- c.ReopenDocument(ctx, uri) }()
	if msg := readNextMsg(t, output); msg == nil || msg["method"] != "textDocument/didClose" {
		t.Fatalf("expected didClose first, got %v", msg)
	}
	if got := didOpenLanguage(t, readNextMsg(t, output)); got != "gleam" {
		t.Fatalf("reopen didOpen languageId = %q, want gleam", got)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func didOpenLanguage(t *testing.T, msg map[string]any) string {
	t.Helper()
	if msg == nil || msg["method"] != "textDocument/didOpen" {
		t.Fatalf("expected didOpen, got %v", msg)
	}
	doc, _ := msg["params"].(map[string]any)["textDocument"].(map[string]any)
	id, _ := doc["languageId"].(string)
	return id
}
