package tools

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// emptySymbolsClient is a protocol stand-in for a server that successfully
// analyzes a document containing no symbols.
func emptySymbolsClient(t *testing.T, root string, capabilities map[string]any) *lsp.LSPClient {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		registrations := []map[string]any{}
		for capability, enabled := range capabilities {
			if enabled != true {
				continue
			}
			method := "textDocument/documentSymbol"
			if capability == "referencesProvider" {
				method = "textDocument/references"
			}
			registrations = append(registrations, map[string]any{"id": capability, "method": method})
		}
		registration, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": -1, "method": "client/registerCapability", "params": map[string]any{"registrations": registrations}})
		if _, err := conn.Write(lsp.EncodeMessage(registration)); err != nil {
			return
		}
		reader := lsp.NewFrameReader(conn)
		for {
			raw, err := reader.ReadMessage()
			if err != nil {
				return
			}
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			if len(request.ID) == 0 || request.Method == "" {
				continue
			}
			result := any([]any{})
			if request.Method == "initialize" {
				result = map[string]any{"capabilities": capabilities}
			} else if request.Method == "shutdown" {
				result = nil
			}
			payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			if _, err := conn.Write(lsp.EncodeMessage(payload)); err != nil {
				return
			}
		}
	}()
	client, err := lsp.NewPassiveClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client.MarkInitializedForTest()
	client.SetRootDirForTest(root)
	for deadline := time.Now().Add(2 * time.Second); ; {
		ready := true
		for capability, enabled := range capabilities {
			if enabled == true && !hasCapabilityInMap(client.GetCapabilities(), capability) {
				ready = false
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("capability registration did not arrive")
		}
		time.Sleep(time.Millisecond)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		client.Shutdown(ctx)
	})
	return client
}

func TestChangeImpact_AllFilesFailed(t *testing.T) {
	for _, format := range []string{"json", "gcf"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			client := emptySymbolsClient(t, root, map[string]any{"documentSymbolProvider": true, "referencesProvider": true})
			result, err := HandleGetChangeImpact(ContextWithOutputFormat(context.Background(), format), client, map[string]any{
				"changed_files": []any{filepath.Join(root, "missing.go")},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatalf("failed analysis reported as success: %v", result.Content)
			}
			if !strings.Contains(result.Content[0].Text, "missing.go") {
				t.Fatalf("missing failure detail: %v", result.Content)
			}
		})
	}
}

func TestChangeImpact_PartialFailureVisible(t *testing.T) {
	for _, format := range []string{"json", "gcf"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "empty.go")
			if err := os.WriteFile(file, []byte("package example\n"), 0600); err != nil {
				t.Fatal(err)
			}
			client := emptySymbolsClient(t, root, map[string]any{"documentSymbolProvider": true, "referencesProvider": true})
			result, err := HandleGetChangeImpact(ContextWithOutputFormat(context.Background(), format), client, map[string]any{
				"changed_files": []any{file, filepath.Join(root, "missing.go")},
			})
			if err != nil || result.IsError {
				t.Fatalf("partial analysis failed: %v %v", err, result.Content)
			}
			text := ""
			for _, item := range result.Content {
				text += item.Text
			}
			if !strings.Contains(text, "missing.go") || !strings.Contains(strings.ToLower(text), "warning") {
				t.Fatalf("partial result hid the failed file: %s", text)
			}
			if format == "json" {
				var payload struct {
					Warnings []string `json:"warnings"`
				}
				if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload.Warnings) != 1 {
					t.Fatalf("warnings = %v, want failed-file detail", payload.Warnings)
				}
			}
		})
	}
}

func TestChangeImpact_UnsupportedCapabilities(t *testing.T) {
	for _, missing := range []string{"documentSymbolProvider", "referencesProvider"} {
		t.Run(missing, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "empty.go")
			if err := os.WriteFile(path, []byte("package example\n"), 0600); err != nil {
				t.Fatal(err)
			}
			caps := map[string]any{"documentSymbolProvider": true, "referencesProvider": true}
			caps[missing] = false
			client := emptySymbolsClient(t, root, caps)
			result, err := HandleGetChangeImpact(ContextWithOutputFormat(context.Background(), "gcf"), client, map[string]any{"changed_files": []any{path}})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError || !strings.Contains(result.Content[0].Text, missing) {
				t.Fatalf("unsupported analysis must name %s instead of returning an empty graph: %+v", missing, result)
			}
		})
	}
}

func TestImpactCapabilities_RequiresSymbolsAndReferences(t *testing.T) {
	for _, tc := range []struct{ symbols, references, supported bool }{
		{true, true, true}, {false, true, false}, {true, false, false},
	} {
		client := emptySymbolsClient(t, t.TempDir(), map[string]any{"documentSymbolProvider": tc.symbols, "referencesProvider": tc.references})
		result, err := HandleGetServerCapabilities(context.Background(), client, nil)
		if err != nil || result.IsError {
			t.Fatalf("capabilities: %v %+v", err, result)
		}
		var capabilities ServerCapabilitiesResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &capabilities); err != nil {
			t.Fatal(err)
		}
		supported, unsupported := false, false
		for _, name := range capabilities.SupportedTools {
			if name == "blast_radius" {
				supported = true
			}
		}
		for _, name := range capabilities.UnsupportedTools {
			if name == "blast_radius" {
				unsupported = true
			}
		}
		if supported != tc.supported || unsupported == tc.supported {
			t.Fatalf("symbols=%v references=%v: supported=%v unsupported=%v", tc.symbols, tc.references, supported, unsupported)
		}
	}
}
