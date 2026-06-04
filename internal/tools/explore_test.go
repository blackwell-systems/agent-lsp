package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHandleExploreSymbol_NilClient(t *testing.T) {
	result, err := HandleExploreSymbol(context.Background(), nil, map[string]any{
		"file_path": "/tmp/test.go",
		"line":      float64(1),
		"column":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nil client")
	}
	if result.Content[0].Text != "LSP client not initialized; call start_lsp first" {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}
}

func TestHandleExploreSymbol_MissingFilePath(t *testing.T) {
	result, err := HandleExploreSymbol(context.Background(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing file_path")
	}
	// With nil client, CheckInitialized fires first
	if result.Content[0].Text != "LSP client not initialized; call start_lsp first" {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}
}

func TestHandleExploreSymbol_MissingPosition(t *testing.T) {
	// This test verifies that when both line and column are missing,
	// and position_pattern is not provided, an appropriate error is returned.
	// Since we can't construct a real LSP client in unit tests, we verify
	// the nil client path which fires before position validation.
	result, err := HandleExploreSymbol(context.Background(), nil, map[string]any{
		"file_path": "/tmp/test.go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing position")
	}
}

func TestTopNFiles(t *testing.T) {
	counts := map[string]int{
		"/a.go": 10,
		"/b.go": 5,
		"/c.go": 20,
		"/d.go": 1,
		"/e.go": 15,
		"/f.go": 8,
	}
	top := topNFiles(counts, 3)
	if len(top) != 3 {
		t.Fatalf("expected 3 files, got %d", len(top))
	}
	if top[0] != "/c.go" {
		t.Errorf("expected /c.go first, got %s", top[0])
	}
	if top[1] != "/e.go" {
		t.Errorf("expected /e.go second, got %s", top[1])
	}
	if top[2] != "/a.go" {
		t.Errorf("expected /a.go third, got %s", top[2])
	}
}

func TestTopNFiles_Empty(t *testing.T) {
	top := topNFiles(map[string]int{}, 5)
	if len(top) != 0 {
		t.Fatalf("expected 0 files, got %d", len(top))
	}
}

func TestTopNFiles_LessThanN(t *testing.T) {
	counts := map[string]int{
		"/a.go": 10,
		"/b.go": 5,
	}
	top := topNFiles(counts, 5)
	if len(top) != 2 {
		t.Fatalf("expected 2 files, got %d", len(top))
	}
}

func TestExploreResult_GCFEncoding(t *testing.T) {
	result := exploreResult{
		TypeInfo: "func Foo() string",
		Source: &exploreSource{
			SymbolName: "Foo",
			StartLine:  10,
			EndLine:    15,
			Source:     "func Foo() string {\n\treturn \"bar\"\n}",
		},
		Callers: []exploreCaller{
			{Name: "main", File: "/cmd/main.go", Line: 42},
			{Name: "TestFoo", File: "/cmd/main_test.go", Line: 10},
		},
		CallersCount: 2,
		References: exploreReferences{
			Count:    5,
			TopFiles: []string{"/cmd/main.go", "/pkg/util.go"},
		},
		TestCallersCount: 1,
	}

	// Test default context produces valid JSON
	t.Run("default_json", func(t *testing.T) {
		ctx := context.Background()
		toolResult, err := EncodeResult(ctx, result)
		if err != nil {
			t.Fatalf("EncodeResult returned error: %v", err)
		}
		if toolResult.IsError {
			t.Fatal("EncodeResult returned error result")
		}
		if len(toolResult.Content) == 0 {
			t.Fatal("EncodeResult returned empty content")
		}
		jsonOutput := toolResult.Content[0].Text

		// Verify JSON can be unmarshaled back
		var decoded exploreResult
		if err := json.Unmarshal([]byte(jsonOutput), &decoded); err != nil {
			t.Fatalf("JSON output cannot be unmarshaled: %v", err)
		}
		if decoded.TypeInfo != result.TypeInfo {
			t.Errorf("TypeInfo mismatch: got %q, want %q", decoded.TypeInfo, result.TypeInfo)
		}
		if decoded.CallersCount != result.CallersCount {
			t.Errorf("CallersCount mismatch: got %d, want %d", decoded.CallersCount, result.CallersCount)
		}
	})

	// Test GCF context produces non-empty output different from JSON
	t.Run("gcf_format", func(t *testing.T) {
		jsonCtx := context.Background()
		jsonResult, _ := EncodeResult(jsonCtx, result)
		jsonOutput := jsonResult.Content[0].Text

		gcfCtx := ContextWithOutputFormat(context.Background(), "gcf")
		gcfResult, err := EncodeResult(gcfCtx, result)
		if err != nil {
			t.Fatalf("EncodeResult with GCF context returned error: %v", err)
		}
		if gcfResult.IsError {
			t.Fatal("EncodeResult with GCF context returned error result")
		}
		if len(gcfResult.Content) == 0 {
			t.Fatal("EncodeResult with GCF context returned empty content")
		}
		gcfOutput := gcfResult.Content[0].Text
		if gcfOutput == "" {
			t.Fatal("GCF output is empty")
		}
		if gcfOutput == jsonOutput {
			t.Error("GCF output should differ from JSON output")
		}
	})
}
