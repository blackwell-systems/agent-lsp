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
func emptySymbolsClient(t *testing.T, root string) *lsp.LSPClient {
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
			if len(request.ID) == 0 {
				continue
			}
			result := any([]any{})
			if request.Method == "shutdown" {
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
			client := lsp.NewLSPClient("", nil)
			client.SetRootDirForTest(root)
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
			client := emptySymbolsClient(t, root)
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
