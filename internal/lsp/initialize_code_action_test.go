package lsp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Check the actual initialize message: rust-analyzer returns no actions without
// literal support, and needs a resolve request if lazy edits are advertised.
func TestInitialize_CodeActionLiteralSupport(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(root, "initialize.json")
	t.Setenv("AGENT_LSP_TEST_INITIALIZE_CAPTURE", capture)
	t.Setenv("AGENT_LSP_DISABLE_WATCHER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client := NewLSPClient(executable, []string{"-test.run=^TestCodeActionInitializeServer$"})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Shutdown(ctx)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, root); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var params struct {
		Capabilities struct {
			TextDocument struct {
				CodeAction struct {
					LiteralSupport *struct {
						CodeActionKind struct {
							ValueSet []string `json:"valueSet"`
						} `json:"codeActionKind"`
					} `json:"codeActionLiteralSupport"`
					ResolveSupport *struct {
						Properties []string `json:"properties"`
					} `json:"resolveSupport"`
				} `json:"codeAction"`
			} `json:"textDocument"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatal(err)
	}
	actions := params.Capabilities.TextDocument.CodeAction
	if actions.LiteralSupport == nil {
		t.Fatal("initialize omitted codeActionLiteralSupport; rust-analyzer will return no actions")
	}
	for _, kind := range []string{"", "quickfix", "refactor", "refactor.extract", "refactor.inline", "refactor.rewrite", "source", "source.organizeImports"} {
		if !slices.Contains(actions.LiteralSupport.CodeActionKind.ValueSet, kind) {
			t.Errorf("codeActionKind.valueSet does not include %q", kind)
		}
	}
	if actions.ResolveSupport != nil && slices.Contains(actions.ResolveSupport.Properties, "edit") {
		t.Error("lazy edits require codeAction/resolve, which the client does not implement")
	}
}

// Run the test executable as a small stdio server, without an external LSP.
func TestCodeActionInitializeServer(t *testing.T) {
	capture := os.Getenv("AGENT_LSP_TEST_INITIALIZE_CAPTURE")
	if capture == "" {
		return
	}
	reader := NewFrameReader(os.Stdin)
	for {
		raw, err := reader.ReadMessage()
		if err != nil {
			os.Exit(1)
		}
		var request struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			os.Exit(1)
		}
		var result any
		switch request.Method {
		case "initialize":
			if err := os.WriteFile(capture, request.Params, 0o600); err != nil {
				os.Exit(1)
			}
			result = map[string]any{"capabilities": map[string]any{}}
		case "shutdown":
		case "exit":
			os.Exit(0)
		default:
			continue
		}
		if err := writeMsg(os.Stdout, map[string]any{
			"jsonrpc": "2.0", "id": request.ID, "result": result,
		}); err != nil {
			os.Exit(1)
		}
	}
}
