package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestImpactLSPProcess runs a framed-stdio language server stand-in in a
// child test process, so the test needs no installed language server.
func TestImpactLSPProcess(t *testing.T) {
	if os.Args[len(os.Args)-1] != "impact-lsp-peer" {
		return
	}
	reader := lsp.NewFrameReader(os.Stdin)
	for {
		raw, err := reader.ReadMessage()
		if err != nil {
			os.Exit(0)
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(raw, &message) != nil {
			os.Exit(1)
		}
		if message.Method == "exit" {
			os.Exit(0)
		}
		if len(message.ID) == 0 {
			continue
		}
		result := any([]any{})
		if message.Method == "initialize" {
			result = map[string]any{"capabilities": map[string]any{"documentSymbolProvider": true, "referencesProvider": true}}
		} else if message.Method == "shutdown" {
			result = nil
		}
		payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
		if _, err := os.Stdout.Write(lsp.EncodeMessage(payload)); err != nil {
			os.Exit(1)
		}
	}
}

// callToolText calls a tool and returns its concatenated text content.
func callToolText(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, content := range result.Content {
		if item, ok := content.(*mcp.TextContent); ok {
			text += item.Text
		}
	}
	return text, result.IsError
}

// A cold blast_radius call must start the server configured for the changed
// file, even when another configured server cannot start. Once that server is
// running, a file for the other language must not be routed to it.
func TestBlastRadius_InitializesSelectedServer(t *testing.T) {
	t.Setenv("AGENT_LSP_DISABLE_WATCHER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "empty.go")
	if err := os.WriteFile(file, []byte("package example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rustFile := filepath.Join(root, "script.rs")
	if err := os.WriteFile(rustFile, []byte("fn main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	resolver := lsp.NewMultiServerManager([]config.ServerEntry{
		{LanguageID: "rust", Extensions: []string{"rs"}, Command: []string{filepath.Join(root, "unavailable-rust-server")}},
		{LanguageID: "go", Extensions: []string{"go"}, Command: []string{executable, "-test.run=^TestImpactLSPProcess$", "--", "impact-lsp-peer"}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer resolver.Shutdown(context.Background())
	state := &clientState{}
	var initMu sync.Mutex
	server := mcp.NewServer(&mcp.Implementation{Name: "impact-test", Version: "1"}, nil)
	registerAnalysisTools(toolDeps{
		server:                    server,
		cs:                        state,
		resolver:                  resolver,
		clientForFileWithAutoInit: newClientForFileWithAutoInit(ctx, resolver, state, &initMu),
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	// The first call starts the server; the second reuses it.
	for i := 0; i < 2; i++ {
		text, isError := callToolText(t, ctx, session, "blast_radius", map[string]any{"changed_files": []string{file}})
		if isError {
			t.Fatalf("call %d: impact query failed: %s", i+1, text)
		}
		if !strings.Contains(text, "changed_symbols") {
			t.Fatalf("call %d: missing impact result: %s", i+1, text)
		}
	}
	// The running Go server must not answer for the configured, unstarted
	// Rust server. The unavailable Rust command must surface as an error.
	if text, isError := callToolText(t, ctx, session, "blast_radius", map[string]any{"changed_files": []string{rustFile}}); !isError {
		t.Fatalf("Rust query used the running Go server: %s", text)
	}
	// Failing to start Rust must leave the Go server usable.
	if text, isError := callToolText(t, ctx, session, "blast_radius", map[string]any{"changed_files": []string{file}}); isError {
		t.Fatalf("Go query failed after Rust startup failure: %s", text)
	}
}
