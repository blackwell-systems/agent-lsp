package lsp

import (
	"reflect"
	"testing"
)

// Metals asks the editor whether to import the build; with no answer it never
// imports, so agent-lsp answers its configuration with autoImportBuilds. Other
// sections still get an empty object ("use defaults").
func TestWorkspaceConfiguration_MetalsAutoImport(t *testing.T) {
	_, serverW, clientR := newTestClient(t)
	if err := writeMsg(serverW, map[string]any{
		"jsonrpc": "2.0", "id": 77, "method": "workspace/configuration",
		"params": map[string]any{"items": []any{
			map[string]any{"section": "metals"},
			map[string]any{"section": "gopls"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	resp := readNextMsg(t, clientR)
	got, _ := resp["result"].([]any)
	want := []any{map[string]any{"autoImportBuilds": "initial"}, map[string]any{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("configuration result = %v, want %v", got, want)
	}
}
