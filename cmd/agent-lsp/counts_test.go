// counts_test.go guards the advertised capability counts (advertisedToolCount,
// advertisedSkillCount) against drift from the actual registrations. The
// counts are surfaced in the MCP server instructions and the `agent-lsp init`
// rules content; a mismatch silently lies to clients about the server's
// surface, so this test fails when a tool or skill is added without bumping
// the constants in server.go.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// countSourceRegistrations counts tool registration call sites across the
// package sources: every addToolWithPhaseCheck(d, &mcp.Tool{...}) plus every
// direct mcp.AddTool(d.server, &mcp.Tool{...}) (tools_phase.go). The generic
// helper's internal mcp.AddTool(d.server, tool, ...) call is excluded by the
// ", &mcp.Tool{" pattern.
func countSourceRegistrations(t *testing.T) int {
	t.Helper()
	matches, err := filepath.Glob("tools_*.go")
	if err != nil {
		t.Fatalf("glob tools_*.go: %v", err)
	}
	files := append(matches, "server.go")
	total := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, pattern := range []string{
			"addToolWithPhaseCheck(d, &mcp.Tool{",
			"mcp.AddTool(d.server, &mcp.Tool{",
		} {
			total += strings.Count(string(src), pattern)
		}
	}
	return total
}

func TestAdvertisedToolCountMatchesRegistration(t *testing.T) {
	got := countSourceRegistrations(t)
	if got != advertisedToolCount {
		t.Errorf("advertisedToolCount = %d, but %d tool registration call sites found in sources; update the constant in server.go",
			advertisedToolCount, got)
	}
}

func TestAdvertisedSkillCountMatchesEmbeddedSkills(t *testing.T) {
	got := len(loadSkills())
	if got != advertisedSkillCount {
		t.Errorf("advertisedSkillCount = %d, but %d embedded skills found under skills/; update the constant in server.go",
			advertisedSkillCount, got)
	}
}
