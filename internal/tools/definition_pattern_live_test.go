package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// go_to_definition advertised position_pattern but read only line/column, so
// a pattern-only call failed. It must resolve the pattern like the other
// navigation tools.
func TestHandleGoToDefinition_PositionPattern_LiveGopls(t *testing.T) {
	gopls := findGopls()
	if gopls == "" {
		t.Skip("gopls not found")
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.21\n"), 0644)
	src := filepath.Join(dir, "main.go")
	_ = os.WriteFile(src, []byte("package main\n\nfunc add(a, b int) int { return a + b }\n\nfunc main() { _ = add(1, 2) }\n"), 0644)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := lsp.NewLSPClient(gopls, nil)
	if err := client.Initialize(ctx, dir); err != nil {
		t.Fatalf("initialize gopls: %v", err)
	}
	defer client.Shutdown(context.Background())

	r, err := HandleGoToDefinition(ctx, client, map[string]any{"file_path": src, "position_pattern": "_ = @@add(1, 2)"})
	if err != nil || r.IsError {
		t.Fatalf("err=%v result=%s", err, resultText(r))
	}
	// The definition is on line 3 (0-based line 2) of main.go.
	if out := resultText(r); !strings.Contains(out, "main.go") || !(strings.Contains(out, `"line":2`) || strings.Contains(out, "|3|") || strings.Contains(out, ":3")) {
		t.Fatalf("definition not resolved to line 3: %s", out)
	}
}
