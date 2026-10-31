package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	gcf "github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// Two servers reporting the same symbol (same name and location) yield ONE
// merged entry, tagged with the default server: the client list is
// default-first, so the default server's copy wins. (CodeRabbit #59 review
// thread 6)
func TestHandleGetWorkspaceSymbolsMulti_DedupsIdenticalResults(t *testing.T) {
	defaultServer := startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Alpha")}, false)
	secondServer := startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Alpha")}, false)

	r, err := HandleGetWorkspaceSymbolsMulti(context.Background(),
		[]*lsp.LSPClient{defaultServer, secondServer}, map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error result: %v", r.Content)
	}
	var syms []map[string]any
	if err := json.Unmarshal([]byte(r.Content[0].Text), &syms); err != nil {
		t.Fatalf("payload is not a symbol array: %v (%q)", err, r.Content[0].Text)
	}
	if len(syms) != 1 {
		t.Fatalf("expected 1 deduplicated symbol, got %d: %v", len(syms), syms)
	}
	if syms[0]["name"] != "Alpha" {
		t.Errorf("deduplicated entry name = %v, want Alpha", syms[0]["name"])
	}
	if syms[0]["server"] != "server-0" {
		t.Errorf("deduplicated entry must keep the default server's tag, got %v", syms[0]["server"])
	}
}

// Positional score decay must be per server: the default server's first
// result and another server's first result start from the same score, so a
// symbol's position in the merged list alone cannot bury non-default
// servers' results. (CodeRabbit #59 review thread 6)
func TestBuildWorkspaceSymbolsPayload_ScoreDecayIsPerServer(t *testing.T) {
	p := buildWorkspaceSymbolsPayload([]types.SymbolInformation{
		{Name: "Alpha", Kind: 12, Server: "default", Location: types.Location{URI: "file:///ws/a.mq4"}},
		{Name: "Beta", Kind: 12, Server: "other", Location: types.Location{URI: "file:///ws/b.mq4"}},
		{Name: "Gamma", Kind: 12, Server: "default", Location: types.Location{URI: "file:///ws/c.mq4"}},
	})
	if len(p.Symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d", len(p.Symbols))
	}
	scoreByName := make(map[string]float64, len(p.Symbols))
	for _, s := range p.Symbols {
		for _, name := range []string{"Alpha", "Beta", "Gamma"} {
			letter := strings.ToLower(name[:1])
			fp, _ := URIToFilePath("file:///ws/" + letter + ".mq4")
			if s.QualifiedName == gcf.QualifiedName(fp, name) {
				scoreByName[name] = s.Score
			}
		}
	}
	if scoreByName["Alpha"] != 1.0 {
		t.Errorf("default server's first result: score = %v, want 1.0", scoreByName["Alpha"])
	}
	if scoreByName["Beta"] != 1.0 {
		t.Errorf("another server's first result must not be penalized for its merged-list position: score = %v, want 1.0", scoreByName["Beta"])
	}
	if scoreByName["Gamma"] != 0.98 {
		t.Errorf("default server's second result: score = %v, want 0.98 (decay within the server)", scoreByName["Gamma"])
	}
}

// The partial-failure provenance note must name the failing server: a bare
// "1 of 2 servers failed" leaves the caller guessing which server to
// restart or re-query. (CodeRabbit #59 review thread 6)
func TestHandleGetWorkspaceSymbolsMulti_FailureNoteNamesServer(t *testing.T) {
	failing := startFakeLSP(t, []string{"workspace/symbol"}, nil, true)
	healthy := startFakeLSP(t, []string{"workspace/symbol"}, []any{wsymJSON("Beta")}, false)

	r, err := HandleGetWorkspaceSymbolsMulti(context.Background(),
		[]*lsp.LSPClient{failing, healthy}, map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error result: %v", r.Content)
	}
	all := contentText(r)
	if !strings.Contains(all, "1 of 2 servers failed") {
		t.Errorf("failure count missing from note: %q", all)
	}
	if !strings.Contains(all, "server-0") {
		t.Errorf("failure note must name the failing server, got %q", all)
	}
}
