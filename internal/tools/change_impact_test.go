package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Go test suffix
		{"pkg/foo/bar_test.go", true},
		{"main_test.go", true},
		// JS/TS .test. pattern
		{"src/utils.test.ts", true},
		{"src/utils.test.js", true},
		{"src/utils.spec.ts", true},
		{"src/utils.spec.js", true},
		// Python test_ prefix
		{"test_models.py", true},
		{"/home/user/project/test_utils.py", true},
		// Negative cases
		{"pkg/foo/bar.go", false},
		{"main.go", false},
		{"src/utils.ts", false},
		{"src/utils.js", false},
		{"models.py", false},
		{"attestation_test_helpers.go", false}, // does not end in _test.go
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := isTestFile(tc.path)
			if got != tc.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestLangIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"internal/tools/helpers.go", "go"},
		{"src/index.ts", "typescript"},
		{"src/App.tsx", "typescript"},
		{"src/index.js", "javascript"},
		{"src/App.jsx", "javascript"},
		{"models.py", "python"},
		{"src/lib.rs", "rust"},
		{"File.cs", "csharp"},
		{"main.hs", "haskell"},
		{"app.rb", "ruby"},
		{"config.xyz", "plaintext"},
		{"README.md", "plaintext"},
		{"Makefile", "plaintext"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := lsp.LanguageIDFromPath(tc.path)
			if got != tc.want {
				t.Errorf("lsp.LanguageIDFromPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestHandleGetChangeImpact_EmptyFiles(t *testing.T) {
	ctx := context.Background()

	// CheckInitialized runs before changed_files validation. With a nil client,
	// all calls return the "not initialized" error. These tests verify that the
	// handler returns an ErrorResult (never a nil error) under these conditions.
	tests := []struct {
		name        string
		args        map[string]any
		wantErrText string
	}{
		{
			name: "missing changed_files key with nil client",
			args: map[string]any{},
			// CheckInitialized fires first when client is nil.
			wantErrText: "LSP client not initialized",
		},
		{
			name: "empty changed_files slice with nil client",
			args: map[string]any{"changed_files": []any{}},
			// CheckInitialized fires first when client is nil.
			wantErrText: "LSP client not initialized",
		},
		{
			name: "changed_files with only empty strings with nil client",
			args: map[string]any{"changed_files": []any{"", ""}},
			// CheckInitialized fires first when client is nil.
			wantErrText: "LSP client not initialized",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := HandleGetChangeImpact(ctx, nil, tc.args)
			if err != nil {
				t.Fatalf("unexpected non-nil error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected IsError=true, got false; content=%v", result.Content)
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty content")
			}
			got := result.Content[0].Text
			if !strings.Contains(got, tc.wantErrText) {
				t.Errorf("error text %q does not contain %q", got, tc.wantErrText)
			}
		})
	}
}

func TestHandleGetChangeImpact_NilClient(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{
		"changed_files": []any{"internal/tools/helpers.go"},
	}

	result, err := HandleGetChangeImpact(ctx, nil, args)
	if err != nil {
		t.Fatalf("unexpected non-nil error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true, got false")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	got := result.Content[0].Text
	want := "LSP client not initialized"
	if !strings.Contains(got, want) {
		t.Errorf("error text %q does not contain %q", got, want)
	}
}

func TestCollectAllSymbols(t *testing.T) {
	// Create temp file for source line resolution
	dir := t.TempDir()
	src := filepath.Join(dir, "test.go")
	content := "package test\n\nfunc ExportedFunc() {}\n\nfunc unexportedHelper() {}\n\ntype myField struct{}\n"
	os.WriteFile(src, []byte(content), 0644)

	syms := []types.DocumentSymbol{
		{Name: "ExportedFunc", Kind: 12, Range: types.Range{Start: types.Position{Line: 2}, End: types.Position{Line: 2}}, SelectionRange: types.Range{Start: types.Position{Line: 2, Character: 5}}},
		{Name: "unexportedHelper", Kind: 12, Range: types.Range{Start: types.Position{Line: 4}, End: types.Position{Line: 4}}, SelectionRange: types.Range{Start: types.Position{Line: 4, Character: 5}}},
		{Name: "myField", Kind: 8, Range: types.Range{Start: types.Position{Line: 6}, End: types.Position{Line: 6}}, SelectionRange: types.Range{Start: types.Position{Line: 6, Character: 5}}},
	}

	// collectAllSymbols should include both exported and unexported, but not fields
	var all []exportedSymbol
	collectAllSymbols(syms, src, "go", &all, false)
	if len(all) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(all))
	}

	// collectExportedSymbols should only include exported
	var exported []exportedSymbol
	collectExportedSymbols(syms, src, "go", &exported, false, 0, 0)
	if len(exported) != 1 {
		t.Fatalf("expected 1 exported symbol, got %d", len(exported))
	}
	if exported[0].Name != "ExportedFunc" {
		t.Errorf("expected ExportedFunc, got %s", exported[0].Name)
	}
}

// symbolNames extracts the names of collected symbols in walk order.
func symbolNames(syms []exportedSymbol) []string {
	names := make([]string, 0, len(syms))
	for _, s := range syms {
		names = append(names, s.Name)
	}
	return names
}

// assertSymbolNames fails unless the collected symbols match want exactly, in
// order. Used by the nested-scope filtering tests below.
func assertSymbolNames(t *testing.T, got []exportedSymbol, want ...string) {
	t.Helper()
	names := symbolNames(got)
	if len(names) != len(want) {
		t.Fatalf("collected symbols = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("collected symbols = %v, want %v", names, want)
		}
	}
}

// TestCollectExportedSymbols_NestedScopeFiltering covers issue #41: Go-centric
// export semantics plus unconditional recursion turned nested parameters,
// locals, fields, properties, and enum members into blast-radius targets on
// servers that report a rich documentSymbol tree (observed with
// mql-lsp-server v2.4.2). scope=exported must keep only container/callable
// nested kinds; scope=all must remain unchanged.
func TestCollectExportedSymbols_NestedScopeFiltering(t *testing.T) {
	t.Run("mql nested parameters and locals excluded at scope=exported", func(t *testing.T) {
		funcWithParams := types.DocumentSymbol{
			Name: "OnTick",
			Kind: 12, // Function
			Children: []types.DocumentSymbol{
				{Name: "symbol", Kind: 13}, // parameter (Variable)
				{Name: "buffer", Kind: 13}, // local (Variable)
			},
		}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{funcWithParams}, "OnTick.mqh", "mql", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "OnTick")
	})

	t.Run("scope=all still reports nested parameters and locals", func(t *testing.T) {
		funcWithParams := types.DocumentSymbol{
			Name: "OnTick",
			Kind: 12,
			Children: []types.DocumentSymbol{
				{Name: "symbol", Kind: 13},
				{Name: "buffer", Kind: 13},
			},
		}
		var all []exportedSymbol
		collectAllSymbols([]types.DocumentSymbol{funcWithParams}, "OnTick.mqh", "mql", &all, true)
		assertSymbolNames(t, all, "OnTick", "symbol", "buffer")
	})

	t.Run("nested class and its method both included at scope=exported", func(t *testing.T) {
		// Order is nested inside a namespace container, exercising the Class
		// kind at nested depth (depth > 0), not just at the top level.
		nestedClass := types.DocumentSymbol{
			Name: "Order",
			Kind: 5, // Class
			Children: []types.DocumentSymbol{
				{Name: "Send", Kind: 6}, // Method
			},
		}
		container := types.DocumentSymbol{
			Name:     "Orders",
			Kind:     3, // Namespace
			Children: []types.DocumentSymbol{nestedClass},
		}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{container}, "Order.mqh", "mql", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "Orders", "Order", "Send")
	})

	t.Run("function-local constant excluded, class constant kept at scope=exported", func(t *testing.T) {
		// A constant declared inside a function is a local implementation
		// detail and must not become a blast-radius target, while a constant
		// inside a class body stays targetable. (issue #41 review follow-up)
		funcWithConstant := types.DocumentSymbol{
			Name: "Start",
			Kind: 12, // Function
			Children: []types.DocumentSymbol{
				{Name: "MAX_RETRIES", Kind: 14}, // function-local Constant (dropped)
			},
		}
		classWithConstant := types.DocumentSymbol{
			Name: "Order",
			Kind: 5, // Class
			Children: []types.DocumentSymbol{
				{Name: "DEFAULT_SLIPPAGE", Kind: 14}, // class Constant (kept)
			},
		}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{funcWithConstant, classWithConstant}, "Order.mqh", "mql", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "Start", "Order", "DEFAULT_SLIPPAGE")
	})

	t.Run("nested free function included at scope=exported", func(t *testing.T) {
		// Some servers nest callable helpers (e.g. JavaScript/Python inner
		// functions) inside their enclosing function; kind 12 stays a target.
		container := types.DocumentSymbol{
			Name: "Start",
			Kind: 12, // Function
			Children: []types.DocumentSymbol{
				{Name: "helper", Kind: 12}, // nested Function (kept)
				{Name: "local", Kind: 13},  // Variable (dropped)
			},
		}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{container}, "start.js", "javascript", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "Start", "helper")
	})

	t.Run("nested struct field excluded at scope=exported", func(t *testing.T) {
		structWithField := types.DocumentSymbol{
			Name: "Hub",
			Kind: 23, // Struct
			Children: []types.DocumentSymbol{
				{Name: "mu", Kind: 8}, // Field
			},
		}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{structWithField}, "hub.go", "go", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "Hub")
	})

	t.Run("nested property and enum member excluded at scope=exported", func(t *testing.T) {
		container := types.DocumentSymbol{
			Name: "Config",
			Kind: 5, // Class
			Children: []types.DocumentSymbol{
				{Name: "Value", Kind: 7},  // Property
				{Name: "Red", Kind: 22},   // EnumMember
				{Name: "method", Kind: 6}, // Method (kept)
			},
		}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{container}, "config.py", "python", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "Config", "method")
	})

	t.Run("go nested method receiver strip still applied", func(t *testing.T) {
		goStruct := types.DocumentSymbol{
			Name: "Hub",
			Kind: 23, // Struct
			Children: []types.DocumentSymbol{
				{Name: "(*Hub).SetSender", Kind: 6}, // exported method
				{Name: "(*Hub).reset", Kind: 6},     // unexported method
			},
		}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{goStruct}, "hub.go", "go", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "Hub", "(*Hub).SetSender")
	})

	t.Run("top-level variable in non-Go still included", func(t *testing.T) {
		topVar := types.DocumentSymbol{Name: "GlobalCounter", Kind: 13}
		var exported []exportedSymbol
		collectExportedSymbols([]types.DocumentSymbol{topVar}, "config.py", "python", &exported, true, 0, 0)
		assertSymbolNames(t, exported, "GlobalCounter")
	})
}

func TestBuildSyncGuardedSet(t *testing.T) {
	// Struct with sync.Mutex field should be guarded
	symbols := []types.DocumentSymbol{
		{
			Name: "Hub",
			Kind: 23, // struct
			Children: []types.DocumentSymbol{
				{Name: "mu", Kind: 8, Detail: "sync.RWMutex"},
				{Name: "sender", Kind: 8, Detail: "NotificationSender"},
			},
		},
		{
			Name: "PureType",
			Kind: 23,
			Children: []types.DocumentSymbol{
				{Name: "name", Kind: 8, Detail: "string"},
			},
		},
	}

	guarded := buildSyncGuardedSet(symbols, nil)

	if !guarded["Hub"] {
		t.Error("expected Hub to be sync-guarded (has RWMutex in children)")
	}
	if guarded["PureType"] {
		t.Error("expected PureType to NOT be sync-guarded")
	}
}

func TestBuildSyncGuardedSet_SourceFallback(t *testing.T) {
	// When gopls doesn't provide children (Go structs), the fallback reads
	// the source lines of the struct range to find sync patterns.
	dir := t.TempDir()
	src := filepath.Join(dir, "hub.go")
	content := "package notify\n\ntype Hub struct {\n\tmu     sync.RWMutex\n\tsender NotificationSender\n}\n\ntype Plain struct {\n\tname string\n}\n"
	os.WriteFile(src, []byte(content), 0644)

	symbols := []types.DocumentSymbol{
		{
			Name: "Hub",
			Kind: 23,
			Range: types.Range{
				Start: types.Position{Line: 2},
				End:   types.Position{Line: 5},
			},
			// No children (gopls behavior for Go structs)
		},
		{
			Name: "Plain",
			Kind: 23,
			Range: types.Range{
				Start: types.Position{Line: 7},
				End:   types.Position{Line: 9},
			},
		},
	}

	filesBySymbol := map[string]string{
		"Hub":   src,
		"Plain": src,
	}

	guarded := buildSyncGuardedSet(symbols, filesBySymbol)

	if !guarded["Hub"] {
		t.Error("expected Hub to be sync-guarded via source fallback (has sync.RWMutex in source)")
	}
	if guarded["Plain"] {
		t.Error("expected Plain to NOT be sync-guarded")
	}
}

func TestIsSyncGuardedSymbol(t *testing.T) {
	guardedTypes := map[string]bool{"Hub": true}

	tests := []struct {
		name     string
		expected bool
	}{
		{"Hub", true},
		{"(*Hub).SetSender", true},
		{"(Hub).Send", true},
		{"PureFunc", false},
		{"(*Other).Method", false},
	}

	for _, tt := range tests {
		got := isSyncGuardedSymbol(tt.name, guardedTypes)
		if got != tt.expected {
			t.Errorf("isSyncGuardedSymbol(%q) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

func TestChangeImpact_EncodeResult_GCF(t *testing.T) {
	// Representative blast_radius response structure matching HandleGetChangeImpact output.
	response := map[string]any{
		"changed_symbols": []symbolRef{
			{Name: "HandleGetChangeImpact", File: "internal/tools/change_impact.go", Line: 82},
		},
		"affected_symbols": []map[string]any{
			{
				"name": "HandleGetChangeImpact",
				"file": "internal/tools/change_impact.go",
				"line": 82,
				"test_callers": []symbolRef{
					{Name: "TestHandleGetChangeImpact_EmptyFiles", File: "internal/tools/change_impact_test.go", Line: 80},
				},
				"non_test_callers": []symbolRef{
					{Name: "HandleGetChangeImpact", File: "cmd/agent-lsp/server.go", Line: 100},
				},
			},
		},
		"test_files":       []string{"internal/tools/change_impact_test.go"},
		"test_functions":   []symbolRef{{Name: "TestHandleGetChangeImpact_EmptyFiles", File: "internal/tools/change_impact_test.go", Line: 80}},
		"non_test_callers": []symbolRef{{Name: "HandleGetChangeImpact", File: "cmd/agent-lsp/server.go", Line: 100}},
		"summary":          "Found 1 changed symbols with 1 test references across 1 test files.",
		"warnings":         []string{},
	}

	t.Run("gcf format produces non-empty output different from json", func(t *testing.T) {
		ctx := ContextWithOutputFormat(context.Background(), "gcf")
		gcfResult, err := EncodeResult(ctx, response)
		if err != nil {
			t.Fatalf("EncodeResult with gcf format failed: %v", err)
		}
		if len(gcfResult.Content) == 0 || gcfResult.Content[0].Text == "" {
			t.Fatal("expected non-empty GCF output")
		}

		// JSON encoding for comparison.
		jsonData, _ := json.Marshal(response)
		jsonStr := string(jsonData)

		gcfStr := gcfResult.Content[0].Text
		if gcfStr == jsonStr {
			t.Error("GCF output should differ from JSON output")
		}
	})

	t.Run("json format regression", func(t *testing.T) {
		ctx := ContextWithOutputFormat(context.Background(), "json")
		jsonResult, err := EncodeResult(ctx, response)
		if err != nil {
			t.Fatalf("EncodeResult with json format failed: %v", err)
		}
		if len(jsonResult.Content) == 0 || jsonResult.Content[0].Text == "" {
			t.Fatal("expected non-empty JSON output")
		}

		// Should match standard json.Marshal output.
		expected, _ := json.Marshal(response)
		if jsonResult.Content[0].Text != string(expected) {
			t.Errorf("JSON format result mismatch.\ngot:  %s\nwant: %s", jsonResult.Content[0].Text, string(expected))
		}
	})

	t.Run("default format is json", func(t *testing.T) {
		ctx := context.Background() // no format set
		result, err := EncodeResult(ctx, response)
		if err != nil {
			t.Fatalf("EncodeResult with default format failed: %v", err)
		}
		expected, _ := json.Marshal(response)
		if result.Content[0].Text != string(expected) {
			t.Errorf("default format should produce JSON output")
		}
	})
}

func TestBuildChangeImpactPayload(t *testing.T) {
	target := symbolRef{Name: "Foo", File: "/src/pkg/foo.go", Line: 10}
	entries := []symbolWithCallers{
		{
			symbolRef:      target,
			NonTestCallers: []symbolRef{{Name: "Bar", File: "/src/pkg/bar.go", Line: 20}},
		},
	}
	tests := []symbolRef{{Name: "TestFoo", File: "/src/pkg/foo_test.go", Line: 5}}

	p := buildChangeImpactPayload(entries, tests)

	if p.Tool != "blast_radius" {
		t.Errorf("wrong tool: got %q, want %q", p.Tool, "blast_radius")
	}
	if len(p.Symbols) < 3 {
		t.Errorf("expected >= 3 symbols, got %d", len(p.Symbols))
	}
	// Verify target symbol is distance 0
	if p.Symbols[0].Distance != 0 {
		t.Errorf("target symbol should be distance 0, got %d", p.Symbols[0].Distance)
	}
	// Verify target symbol score is 1.0
	if p.Symbols[0].Score != 1.0 {
		t.Errorf("target symbol score should be 1.0, got %f", p.Symbols[0].Score)
	}
	// Verify caller is distance 1
	if p.Symbols[1].Distance != 1 {
		t.Errorf("caller symbol should be distance 1, got %d", p.Symbols[1].Distance)
	}
	// Verify test function is distance 1 with score 0.7
	if p.Symbols[2].Distance != 1 {
		t.Errorf("test symbol should be distance 1, got %d", p.Symbols[2].Distance)
	}
	if p.Symbols[2].Score != 0.7 {
		t.Errorf("test symbol score should be 0.7, got %f", p.Symbols[2].Score)
	}
	// Verify edges exist and are REAL caller -> target edges (issue #5:
	// the old implementation produced degenerate self-edges @N<@N).
	if len(p.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(p.Edges))
	}
	if len(p.Edges) > 0 {
		if p.Edges[0].EdgeType != "calls" {
			t.Errorf("edge type should be 'calls', got %q", p.Edges[0].EdgeType)
		}
		if p.Edges[0].Source == p.Edges[0].Target {
			t.Errorf("degenerate self-edge: source %q == target %q", p.Edges[0].Source, p.Edges[0].Target)
		}
		wantTarget := gcf.QualifiedName(target.File, target.Name)
		wantSource := gcf.QualifiedName("/src/pkg/bar.go", "Bar")
		if p.Edges[0].Source != wantSource || p.Edges[0].Target != wantTarget {
			t.Errorf("edge should be %q -> %q, got %q -> %q", wantSource, wantTarget, p.Edges[0].Source, p.Edges[0].Target)
		}
	}
}

func TestBuildChangeImpactPayload_Dedup(t *testing.T) {
	// Duplicate callers should be deduplicated by qualified name.
	caller := symbolRef{Name: "Bar", File: "/src/pkg/bar.go", Line: 20}
	entries := []symbolWithCallers{
		{
			symbolRef:      symbolRef{Name: "A", File: "/src/pkg/a.go", Line: 1},
			NonTestCallers: []symbolRef{caller, {Name: "Bar", File: "/src/pkg/bar.go", Line: 25}},
		},
		{
			symbolRef:      symbolRef{Name: "B", File: "/src/pkg/b.go", Line: 2},
			NonTestCallers: []symbolRef{{Name: "Bar", File: "/src/pkg/bar.go", Line: 30}},
		},
	}
	p := buildChangeImpactPayload(entries, nil)
	// Only one caller symbol should appear (deduplicated by qualified name).
	callerCount := 0
	for _, s := range p.Symbols {
		if s.Distance == 1 {
			callerCount++
		}
	}
	if callerCount != 1 {
		t.Errorf("expected 1 deduplicated caller, got %d", callerCount)
	}
	// But BOTH call relationships must produce caller -> target edges.
	if len(p.Edges) != 2 {
		t.Errorf("expected 2 caller->target edges, got %d", len(p.Edges))
	}
}

func TestRenestFlatSymbols_FlatMQLList(t *testing.T) {
	// Mimics mql-lsp-server v2.5.0: a FLAT documentSymbol list where the
	// function's parameters and locals are top-level siblings carrying
	// SymbolKind Function (12). Issue #5 live repro on riesgo_mq4.mqh.
	flat := []types.DocumentSymbol{
		{Name: "CalculaRiesgoTicks", Kind: 12,
			Range:          types.Range{Start: types.Position{Line: 9, Character: 0}, End: types.Position{Line: 30, Character: 4}},
			SelectionRange: types.Range{Start: types.Position{Line: 9, Character: 7}, End: types.Position{Line: 9, Character: 25}}},
		{Name: "tipoOrden", Kind: 13, // parameter (Variable), flat top-level
			Range:          types.Range{Start: types.Position{Line: 9, Character: 26}, End: types.Position{Line: 9, Character: 35}},
			SelectionRange: types.Range{Start: types.Position{Line: 9, Character: 26}, End: types.Position{Line: 9, Character: 35}}},
		{Name: "lotaje", Kind: 13, // parameter
			Range:          types.Range{Start: types.Position{Line: 9, Character: 37}, End: types.Position{Line: 9, Character: 43}},
			SelectionRange: types.Range{Start: types.Position{Line: 9, Character: 37}, End: types.Position{Line: 9, Character: 43}}},
		{Name: "cantidadTicks", Kind: 13, // local
			Range:          types.Range{Start: types.Position{Line: 13, Character: 5}, End: types.Position{Line: 13, Character: 25}},
			SelectionRange: types.Range{Start: types.Position{Line: 13, Character: 11}, End: types.Position{Line: 13, Character: 24}}},
		{Name: "CalculaLotajeDesdeRiesgo", Kind: 12,
			Range:          types.Range{Start: types.Position{Line: 37, Character: 0}, End: types.Position{Line: 65, Character: 4}},
			SelectionRange: types.Range{Start: types.Position{Line: 37, Character: 7}, End: types.Position{Line: 37, Character: 31}}},
		{Name: "i", Kind: 13, // loop local
			Range:          types.Range{Start: types.Position{Line: 46, Character: 2}, End: types.Position{Line: 46, Character: 10}},
			SelectionRange: types.Range{Start: types.Position{Line: 46, Character: 6}, End: types.Position{Line: 46, Character: 7}}},
	}

	nested := renestFlatSymbols(flat)
	if len(nested) != 2 {
		t.Fatalf("expected 2 top-level functions after re-nesting, got %d", len(nested))
	}
	if len(nested[0].Children) != 3 {
		t.Errorf("expected 3 children under CalculaRiesgoTicks, got %d", len(nested[0].Children))
	}
	if len(nested[1].Children) != 1 {
		t.Errorf("expected 1 child under CalculaLotajeDesdeRiesgo, got %d", len(nested[1].Children))
	}

	// scope=exported must now exclude the params/locals (all kind 12, but
	// nested): only the two top-level functions are targets.
	var out []exportedSymbol
	collectExportedSymbols(nested, "/tmp/fixture.mqh", "mql", &out, true, 0, 0)
	if len(out) != 2 {
		names := make([]string, 0, len(out))
		for _, s := range out {
			names = append(names, s.Name)
		}
		t.Errorf("expected 2 exported targets, got %d: %v", len(out), names)
	}
	for _, s := range out {
		if s.Name != "CalculaRiesgoTicks" && s.Name != "CalculaLotajeDesdeRiesgo" {
			t.Errorf("nested param/local %q promoted to target", s.Name)
		}
	}

	// scope=all must still see everything (8 symbols).
	var all []exportedSymbol
	collectAllSymbols(nested, "/tmp/fixture.mqh", "mql", &all, true)
	if len(all) != 6 {
		t.Errorf("scope=all expected 6 symbols, got %d", len(all))
	}
}

func TestRenestFlatSymbols_NestedTreeUnchanged(t *testing.T) {
	// A properly nested tree must survive re-nesting structurally.
	nested := []types.DocumentSymbol{
		{Name: "Outer", Kind: 12,
			Range: types.Range{Start: types.Position{Line: 0, Character: 0}, End: types.Position{Line: 10, Character: 0}},
			Children: []types.DocumentSymbol{
				{Name: "innerLocal", Kind: 13,
					Range: types.Range{Start: types.Position{Line: 2, Character: 1}, End: types.Position{Line: 2, Character: 9}}},
				{Name: "InnerFn", Kind: 12,
					Range: types.Range{Start: types.Position{Line: 4, Character: 1}, End: types.Position{Line: 8, Character: 2}}},
			}},
		{Name: "Sibling", Kind: 12,
			Range: types.Range{Start: types.Position{Line: 20, Character: 0}, End: types.Position{Line: 25, Character: 0}}},
	}

	got := renestFlatSymbols(nested)
	if len(got) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(got))
	}
	if len(got[0].Children) != 2 {
		t.Fatalf("expected 2 children on Outer, got %d", len(got[0].Children))
	}
	// Children sorted by position: innerLocal (line 2) before InnerFn (line 4).
	if got[0].Children[0].Name != "innerLocal" || got[0].Children[1].Name != "InnerFn" {
		t.Errorf("unexpected children order: %q, %q", got[0].Children[0].Name, got[0].Children[1].Name)
	}
	if len(got[0].Children[0].Children) != 0 || len(got[0].Children[1].Children) != 0 {
		t.Error("leaf symbols must not gain children")
	}
}

func TestRenestFlatSymbols_EqualRangesAreSiblings(t *testing.T) {
	// Two symbols with identical ranges must not become parent/child.
	syms := []types.DocumentSymbol{
		{Name: "A", Kind: 12, Range: types.Range{Start: types.Position{Line: 1, Character: 0}, End: types.Position{Line: 5, Character: 0}}},
		{Name: "B", Kind: 12, Range: types.Range{Start: types.Position{Line: 1, Character: 0}, End: types.Position{Line: 5, Character: 0}}},
	}
	got := renestFlatSymbols(syms)
	if len(got) != 2 {
		t.Errorf("identical ranges must stay siblings, got %d roots", len(got))
	}
}

func TestBuildChangeImpactPayload_NoSelfEdgesForSameName(t *testing.T) {
	// A caller with the same (file, name) as the target would produce a
	// degenerate self-edge; it must be dropped.
	entry := symbolWithCallers{
		symbolRef:      symbolRef{Name: "Foo", File: "/src/pkg/foo.go", Line: 10},
		NonTestCallers: []symbolRef{{Name: "Foo", File: "/src/pkg/foo.go", Line: 12}},
	}
	p := buildChangeImpactPayload([]symbolWithCallers{entry}, nil)
	if len(p.Edges) != 0 {
		t.Errorf("expected 0 edges (self-edge dropped), got %d", len(p.Edges))
	}
}
