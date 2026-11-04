package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

const refactorFixture = `package main

type Person struct {
	FirstName string
	Age       int
}

type Greeter interface {
	Greet() string
}

func main() {}
`

// TestHandleRefactor_LiveGopls drives gopls's interactive refactorings
// (v0.24+) through the refactor tool: listing, the question form, a lazyEnum
// search, a rejected answer, dry_run, and applying. Skips without a gopls
// that supports interactive resolve.
func TestHandleRefactor_LiveGopls(t *testing.T) {
	gopls := findGopls()
	if gopls == "" {
		t.Skip("gopls not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(refactorFixture), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	client := lsp.NewLSPClient(gopls, nil)
	if err := client.Initialize(ctx, dir); err != nil {
		t.Fatalf("initialize gopls: %v", err)
	}
	defer client.Shutdown(context.Background())
	if !client.SupportsInteractiveResolve() {
		t.Skip("gopls does not support interactive resolve (needs v0.24+)")
	}

	call := func(args map[string]any) (string, bool) {
		t.Helper()
		args["file_path"] = src
		r, err := HandleRefactor(ctx, client, args)
		if err != nil {
			t.Fatalf("HandleRefactor: %v", err)
		}
		return resultText(r), r.IsError
	}

	// Listing at the struct offers implement-interface.
	out, isErr := call(map[string]any{"line": 3, "column": 6})
	if isErr || !strings.Contains(out, "implement an interface") {
		t.Fatalf("list: %s", out)
	}

	// Without answers, the questions come back; the search finds Greeter.
	action := "implement an interface"
	out, isErr = call(map[string]any{"line": 3, "column": 6, "action": action, "option_queries": map[string]any{"interface": "Greeter"}})
	if isErr || !strings.Contains(out, `"status":"needs_input"`) || !strings.Contains(out, "x.Greeter") {
		t.Fatalf("form: %s", out)
	}

	// A rejected answer comes back with an error on the field.
	out, isErr = call(map[string]any{"line": 3, "column": 6, "action": action, "answers": map[string]any{"interface": "x.NoSuchInterface"}})
	if isErr || !strings.Contains(out, "needs_input") || !strings.Contains(out, `"error"`) {
		t.Fatalf("rejected answer: %s", out)
	}

	// dry_run returns the edit and writes nothing.
	answers := map[string]any{"interface": "x.Greeter"}
	out, isErr = call(map[string]any{"line": 3, "column": 6, "action": action, "answers": answers, "dry_run": true})
	if isErr || !strings.Contains(out, "workspace_edits") || !strings.Contains(out, "Greet") {
		t.Fatalf("dry run: %s", out)
	}
	if got, _ := os.ReadFile(src); string(got) != refactorFixture {
		t.Fatalf("dry_run wrote the file:\n%s", got)
	}

	// Applying writes the method.
	out, isErr = call(map[string]any{"line": 3, "column": 6, "action": action, "answers": answers})
	if isErr || !strings.Contains(out, "Applied") {
		t.Fatalf("apply: %s", out)
	}
	if got, _ := os.ReadFile(src); !strings.Contains(string(got), "func (p *Person) Greet() string") {
		t.Fatalf("method not written:\n%s", got)
	}

	// Struct tags: select the fields; answers in one call.
	out, isErr = call(map[string]any{"line": 4, "column": 2, "end_line": 5, "end_column": 11, "action": "Add struct tags",
		"answers": map[string]any{"addTags": "json", "transform": "snakecase"}})
	if isErr || !strings.Contains(out, "Applied") {
		t.Fatalf("add tags: %s", out)
	}
	if got, _ := os.ReadFile(src); !strings.Contains(string(got), "`json:\"first_name\"`") {
		t.Fatalf("tags not written:\n%s", got)
	}
}

// execute_command on an interactive command points to refactor instead of
// failing on the missing answers.
func TestHandleExecuteCommand_InteractiveCommandPointsToRefactor(t *testing.T) {
	gopls := findGopls()
	if gopls == "" {
		t.Skip("gopls not found")
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.21\n"), 0644)
	src := filepath.Join(dir, "main.go")
	_ = os.WriteFile(src, []byte(refactorFixture), 0644)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := lsp.NewLSPClient(gopls, nil)
	if err := client.Initialize(ctx, dir); err != nil {
		t.Fatalf("initialize gopls: %v", err)
	}
	defer client.Shutdown(context.Background())
	if !client.SupportsInteractiveResolve() {
		t.Skip("gopls does not support interactive resolve (needs v0.24+)")
	}
	arg := map[string]any{"Interface": "", "Location": map[string]any{
		"uri":   lsp.PathToFileURI(src),
		"range": map[string]any{"start": map[string]any{"line": 2, "character": 5}, "end": map[string]any{"line": 2, "character": 11}},
	}}
	raw, _ := json.Marshal([]any{arg})
	var cmdArgs []any
	_ = json.Unmarshal(raw, &cmdArgs)
	r, _ := HandleExecuteCommand(ctx, client, map[string]any{"command": "gopls.implement_interface", "arguments": cmdArgs})
	if !r.IsError || !strings.Contains(resultText(r), "refactor tool") {
		t.Fatalf("result = %s, want a pointer to the refactor tool", resultText(r))
	}
}
