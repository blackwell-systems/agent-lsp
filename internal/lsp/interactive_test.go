package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// While a command runs under ExecuteCommandCapturingEdits, a workspace/applyEdit
// from the server is captured and acknowledged, not written; outside a capture
// it is applied as before.
func TestExecuteCommandCapturingEdits_CapturesWithoutWriting(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.go")
	if err := os.WriteFile(file, []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c, serverW, clientR := newTestClient(t)
	c.SetRootDirForTest(root)
	uri := PathToFileURI(file)
	edit := map[string]any{"changes": map[string]any{uri: []any{map[string]any{
		"range":   map[string]any{"start": map[string]any{"line": 0, "character": 8}, "end": map[string]any{"line": 0, "character": 9}},
		"newText": "b",
	}}}}

	type result struct {
		edits []any
		err   error
	}
	done := make(chan result, 1)
	go func() {
		e, err := c.ExecuteCommandCapturingEdits(context.Background(), "gopls.modify_tags", []any{"arg"}, []FormAnswer{{ID: "addTags", Value: "json"}})
		done <- result{e, err}
	}()

	req := readNextMsg(t, clientR)
	if req["method"] != "workspace/executeCommand" {
		t.Fatalf("got %v, want workspace/executeCommand", req["method"])
	}
	params := req["params"].(map[string]any)
	if answers, _ := params["formAnswers"].([]any); len(answers) != 1 {
		t.Fatalf("formAnswers not sent: %v", params)
	}
	// The server asks the client to apply the edit before answering.
	_ = writeMsg(serverW, map[string]any{"jsonrpc": "2.0", "id": 900, "method": "workspace/applyEdit", "params": map[string]any{"edit": edit}})
	ack := readNextMsg(t, clientR)
	if res, _ := ack["result"].(map[string]any); res["applied"] != true {
		t.Fatalf("applyEdit answer = %v, want applied true", ack)
	}
	_ = writeMsg(serverW, map[string]any{"jsonrpc": "2.0", "id": req["id"], "result": nil})

	select {
	case r := <-done:
		if r.err != nil || len(r.edits) != 1 {
			t.Fatalf("edits=%v err=%v, want the one edit", r.edits, r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	if got, _ := os.ReadFile(file); string(got) != "package a\n" {
		t.Fatalf("captured edit was written: %q", got)
	}
}
