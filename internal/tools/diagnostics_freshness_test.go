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
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// The server first verifies the initial document, then only publishes an old
// version after get_diagnostics reopens it. Cached results must not verify the
// new version, in either output format or query mode.
func TestGetDiagnostics_StaleReopenedVersionIsUnverified(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, format string
		all, grouped bool
	}{
		{"file JSON", "json", false, false},
		{"all documents GCF", "gcf", true, false},
		{"grouped file JSON", "json", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, path := diagnosticsFreshnessClient(t, "stale")
			args := map[string]any{}
			if !tc.all {
				args["file_path"] = path
			}
			if tc.grouped {
				args["group_by"] = "symbol"
			}
			ctx, cancel := context.WithTimeout(ContextWithOutputFormat(context.Background(), tc.format), 35*time.Second)
			defer cancel()
			result, err := HandleGetDiagnostics(ctx, client, args)
			if err != nil || result.IsError {
				t.Fatalf("diagnostics: %v, %+v", err, result)
			}
			text := diagnosticsResultText(result)
			if strings.Contains(text, "Safe to proceed") || !strings.Contains(text, "not confirmed current") {
				t.Fatalf("stale diagnostics must remain unverified, got: %s", text)
			}
		})
	}
}

// get_diagnostics intentionally accepts silence from servers that do not republish
// unchanged diagnostics. Keep that behavior when a verified cache exists.
func TestGetDiagnostics_SilentUnchangedServerKeepsVerifiedCache(t *testing.T) {
	t.Parallel()
	client, path := diagnosticsFreshnessClient(t, "silent")
	result, err := HandleGetDiagnostics(context.Background(), client, map[string]any{"file_path": path})
	if err != nil || result.IsError {
		t.Fatalf("diagnostics: %v, %+v", err, result)
	}
	if text := diagnosticsResultText(result); !strings.Contains(text, "No errors. Safe to proceed.") {
		t.Fatalf("unchanged silent server should retain its verified result: %s", text)
	}
}

func TestGetDiagnostics_CurrentReopenedVersionReportsDiagnostics(t *testing.T) {
	t.Parallel()
	client, path := diagnosticsFreshnessClient(t, "current")
	result, err := HandleGetDiagnostics(context.Background(), client, map[string]any{"file_path": path})
	if err != nil || result.IsError {
		t.Fatalf("diagnostics: %v, %+v", err, result)
	}
	text := diagnosticsResultText(result)
	if !strings.Contains(text, "current error") || !strings.Contains(text, "suggest_fixes") || strings.Contains(text, "not confirmed current") {
		t.Fatalf("current-version diagnostics should be reported as verified: %s", text)
	}
}

func diagnosticsResultText(result types.ToolResult) string {
	var text strings.Builder
	for _, item := range result.Content {
		text.WriteString(item.Text)
		text.WriteByte('\n')
	}
	return text.String()
}

func diagnosticsFreshnessClient(t *testing.T, mode string) (*lsp.LSPClient, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	uri := CreateFileURI(path)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		write := func(msg any) bool {
			b, _ := json.Marshal(msg)
			_, err := conn.Write(lsp.EncodeMessage(b))
			return err == nil
		}
		r := lsp.NewFrameReader(conn)
		opened := 0
		for {
			raw, err := r.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params struct {
					TextDocument struct {
						Version int `json:"version"`
					} `json:"textDocument"`
				} `json:"params"`
			}
			if json.Unmarshal(raw, &msg) != nil {
				return
			}
			if msg.Method == "textDocument/didOpen" {
				opened++
				if opened > 1 && mode == "silent" {
					continue
				}
				version := msg.Params.TextDocument.Version
				if opened > 1 && mode == "stale" {
					version--
				}
				diags := []any{}
				if opened > 1 && mode == "current" {
					diags = append(diags, map[string]any{"range": map[string]any{"start": map[string]any{"line": 0, "character": 0}, "end": map[string]any{"line": 0, "character": 1}}, "severity": 1, "message": "current error"})
				}
				params := map[string]any{"uri": uri, "diagnostics": diags}
				if mode != "silent" {
					params["version"] = version
				}
				if !write(map[string]any{"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics", "params": params}) {
					return
				}
			} else if len(msg.ID) > 0 && msg.Method != "" {
				if !write(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": nil}) {
					return
				}
			}
		}
	}()
	client, err := lsp.NewPassiveClient(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client.MarkInitializedForTest()
	client.SetRootDirForTest(root)
	t.Cleanup(func() { client.Shutdown(context.Background()) })
	if err := client.OpenDocument(context.Background(), uri, "package main\n", "go"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !client.HasPublishedDiagnostics(uri) {
		if time.Now().After(deadline) {
			t.Fatal("initial diagnostic publication did not arrive")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return client, path
}
