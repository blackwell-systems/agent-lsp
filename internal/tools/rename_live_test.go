package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// resultText concatenates a tool result's text content.
func resultText(res types.ToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		b.WriteString(c.Text)
	}
	return b.String()
}

// findGopls locates a gopls binary, preferring PATH then $GOPATH/bin and
// $HOME/go/bin. Returns "" if none is found.
func findGopls() string {
	if p, err := exec.LookPath("gopls"); err == nil {
		return p
	}
	candidates := []string{}
	if gp := os.Getenv("GOPATH"); gp != "" {
		candidates = append(candidates, filepath.Join(gp, "bin", "gopls"))
	}
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, "go", "bin", "gopls"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// TestRenameSymbol_LiveGopls is a full end-to-end regression for issue #12: it
// drives a real textDocument/rename through gopls and the HandleRenameSymbol
// handler, then asserts the file on disk is renamed correctly and the tool
// returns a summary (not a raw workspace_edit for the caller to reconstruct).
// Skips when gopls is unavailable so CI without it stays green.
func TestRenameSymbol_LiveGopls(t *testing.T) {
	gopls := findGopls()
	if gopls == "" {
		t.Skip("gopls not found; skipping live rename integration test")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module renametest\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(dir, "main.go")
	src := "package main\n\nimport \"fmt\"\n\nfunc greet() string { return \"hi\" }\n\nfunc main() { fmt.Println(greet()) }\n"
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := lsp.NewLSPClient(gopls, nil)
	if err := client.Initialize(ctx, dir); err != nil {
		t.Fatalf("initialize gopls: %v", err)
	}
	defer client.Shutdown(context.Background())

	// Rename greet -> salute. Definition is on line 5 (1-based); "greet" begins
	// at column 6 (after "func ").
	args := map[string]any{
		"file_path":   srcPath,
		"language_id": "go",
		"line":        5,
		"column":      6,
		"new_name":    "salute",
	}
	res, err := HandleRenameSymbol(ctx, client, args)
	if err != nil {
		t.Fatalf("HandleRenameSymbol returned error: %v", err)
	}
	if res.IsError {
		t.Fatalf("HandleRenameSymbol returned error result: %s", resultText(res))
	}

	// The tool must return a summary, never a raw workspace_edit blob.
	out := resultText(res)
	if !strings.Contains(out, "Renamed") {
		t.Errorf("expected a rename summary, got: %q", out)
	}
	if strings.Contains(out, "workspace_edit") || strings.Contains(out, "newText") {
		t.Errorf("tool leaked a raw edit into its result (round-trip hazard): %q", out)
	}

	// The file on disk must be renamed at both the definition and the call site.
	got, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	if strings.Contains(gotStr, "greet") {
		t.Errorf("old name still present after rename:\n%s", gotStr)
	}
	if strings.Count(gotStr, "salute") != 2 {
		t.Errorf("expected 2 occurrences of salute (def + call), got:\n%s", gotStr)
	}
	// Structure must be intact — no corruption of surrounding code.
	if !strings.Contains(gotStr, "func salute() string { return \"hi\" }") ||
		!strings.Contains(gotStr, "fmt.Println(salute())") {
		t.Errorf("file structure corrupted after rename:\n%s", gotStr)
	}
}
