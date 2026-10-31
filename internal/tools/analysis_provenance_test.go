package tools

import (
	"strings"
	"testing"

	gcf "github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// A server name with whitespace must not ride into GCF provenance raw: the
// GCF decoder splits symbol lines with strings.Fields, so
// "lsp_resolved@MQL Language Server" decodes as provenance "lsp_resolved@MQL"
// with the trailing words corrupting the line. Disallowed characters become
// underscores. (CodeRabbit #59 review thread 5)
func TestBuildWorkspaceSymbolsPayload_ServerNameWithSpacesSanitized(t *testing.T) {
	p := buildWorkspaceSymbolsPayload([]types.SymbolInformation{
		{Name: "OnTick", Kind: 12, Server: "MQL Language Server", Location: types.Location{URI: "file:///ws/a.mq5"}},
	})
	out, err := gcf.EncodeGraph(p)
	if err != nil {
		t.Fatalf("EncodeGraph: %v", err)
	}
	if !strings.Contains(out, "lsp_resolved@MQL_Language_Server") {
		t.Errorf("sanitized server provenance missing in GCF payload:\n%s", out)
	}
	if strings.Contains(out, "lsp_resolved@MQL Language") {
		t.Errorf("raw server name with spaces leaked into GCF payload:\n%s", out)
	}

	// The '@' source/instance separator inside a server name must not create
	// a second split point in the lsp_resolved@<server> convention either.
	p2 := buildWorkspaceSymbolsPayload([]types.SymbolInformation{
		{Name: "OnTick", Kind: 12, Server: "a@b", Location: types.Location{URI: "file:///ws/a.mq5"}},
	})
	out2, err := gcf.EncodeGraph(p2)
	if err != nil {
		t.Fatalf("EncodeGraph: %v", err)
	}
	if !strings.Contains(out2, "lsp_resolved@a_b") {
		t.Errorf("'@' in server name must be sanitized, got:\n%s", out2)
	}
}
