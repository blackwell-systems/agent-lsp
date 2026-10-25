package main

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeSymServer starts a minimal fake language server that answers
// initialize with workspaceSymbolProvider declared and every
// workspace/symbol request with a single hit named sym. It returns a
// passive client already marked initialized (mirroring the fakes used by
// internal/tools/multi_server_test.go, but at the cmd layer so the
// registered MCP handler is exercised end to end).
func fakeSymServer(t *testing.T, sym string) *lsp.LSPClient {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				// Register the workspace symbol capability server-initiated
				// (LSP §3.7.2): passive clients never send initialize, so this
				// is the only way HasCapability becomes true.
				reg, _ := json.Marshal(map[string]any{
					"jsonrpc": "2.0", "id": 1,
					"method": "client/registerCapability",
					"params": map[string]any{"registrations": []map[string]any{{"id": "wsym-1", "method": "workspace/symbol"}}},
				})
				if _, err := conn.Write(lsp.EncodeMessage(reg)); err != nil {
					return
				}
				r := lsp.NewFrameReader(conn)
				for {
					raw, err := r.ReadMessage()
					if err != nil {
						return
					}
					var msg struct {
						ID     *float64        `json:"id"`
						Method string          `json:"method"`
						Params json.RawMessage `json:"params"`
					}
					if err := json.Unmarshal(raw, &msg); err != nil {
						return
					}
					if msg.ID == nil || msg.Method == "" {
						continue
					}
					var result any
					switch msg.Method {
					case "initialize":
						result = map[string]any{
							"capabilities": map[string]any{"workspaceSymbolProvider": true},
						}
					case "workspace/symbol":
						result = []map[string]any{{
							"name": sym, "kind": 12,
							"location": map[string]any{
								"uri":   "file:///ws/src/a.mq4",
								"range": map[string]any{"start": map[string]any{"line": 1, "character": 0}, "end": map[string]any{"line": 1, "character": 4}},
							},
						}}
					}
					body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
					if _, err := conn.Write(lsp.EncodeMessage(body)); err != nil {
						return
					}
				}
			}()
		}
	}()
	client, err := lsp.NewPassiveClient(ln.Addr().String())
	if err != nil {
		t.Fatalf("NewPassiveClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	client.MarkInitializedForTest()
	// The capability registration arrives asynchronously; wait for it so the
	// silent capability gate in GetWorkspaceSymbols does not swallow the
	// query and return an empty slice.
	deadline := time.Now().Add(2 * time.Second)
	for !client.HasCapability("workspaceSymbolProvider") {
		if time.Now().After(deadline) {
			t.Fatal("fake server capability never registered")
		}
		time.Sleep(2 * time.Millisecond)
	}
	return client
}

// TestFindSymbolHandler_FansOutToAllServers is the cmd-level regression test
// for the blocking review finding on upstream PR #59: find_symbol was wired
// to tools.HandleGetWorkspaceSymbols(ctx, d.cs.get(), ...) — the default
// client only — so an MQL workspace with clangd as the default server
// returned false empty results (upstream issue #55). The handler must route
// through workspaceQueryClients (every connected server), which is only
// observable by calling the registered MCP tool, not the Multi handler
// directly.
func TestFindSymbolHandler_FansOutToAllServers(t *testing.T) {
	ctx := context.Background()
	defaultClient := fakeSymServer(t, "FromDefaultServer")
	mqlClient := fakeSymServer(t, "FromMQLServer")

	cs := &clientState{}
	cs.set(defaultClient)

	server := mcp.NewServer(&mcp.Implementation{Name: "agent-lsp-test", Version: "0.0.0"}, nil)
	deps := toolDeps{
		server:                    server,
		cs:                        cs,
		resolver:                  &stubResolver{all: []*lsp.LSPClient{defaultClient, mqlClient}},
		clientForFileWithAutoInit: func(string) *lsp.LSPClient { return defaultClient },
	}
	registerAnalysisTools(deps)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "find_symbol",
		Arguments: map[string]any{"query": "Sym"},
	})
	if err != nil {
		t.Fatalf("call find_symbol: %v", err)
	}
	if res.IsError {
		t.Fatalf("find_symbol returned an error result: %v", res.Content)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	got := text.String()
	if !strings.Contains(got, "FromDefaultServer") {
		t.Errorf("result missing the default server's hit:\n%s", got)
	}
	if !strings.Contains(got, "FromMQLServer") {
		t.Errorf("result missing the second server's hit — find_symbol did not fan out to every connected server:\n%s", got)
	}
	// The result is a JSON array of symbols in basic detail (an agent hint is
	// appended after the JSON), so decode the first value and require BOTH
	// servers' hits — a plain strings.Contains(got, "2") was vacuous
	// ("kind":12 contains a 2). (PR #71 maintainer review / CodeRabbit)
	var gotSymbols []map[string]any
	if err := json.NewDecoder(strings.NewReader(got)).Decode(&gotSymbols); err != nil {
		t.Fatalf("find_symbol result does not start with a JSON symbol array: %v\n%s", err, got)
	}
	if len(gotSymbols) != 2 {
		t.Errorf("want 2 symbols (one per server), got %d:\n%s", len(gotSymbols), got)
	}
}
