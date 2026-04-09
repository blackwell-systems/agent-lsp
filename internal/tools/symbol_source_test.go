package tools

import (
	"context"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// --- TestContainsPosition ---

func TestContainsPosition(t *testing.T) {
	r := types.Range{
		Start: types.Position{Line: 5, Character: 3},
		End:   types.Position{Line: 10, Character: 7},
	}

	cases := []struct {
		name      string
		line      int
		character int
		want      bool
	}{
		{
			name:      "position before range start",
			line:      4,
			character: 5,
			want:      false,
		},
		{
			name:      "position at exact range start",
			line:      5,
			character: 3,
			want:      true,
		},
		{
			name:      "position inside range (middle line)",
			line:      7,
			character: 0,
			want:      true,
		},
		{
			name:      "position at exact range end",
			line:      10,
			character: 7,
			want:      true,
		},
		{
			name:      "position after range end",
			line:      11,
			character: 0,
			want:      false,
		},
		{
			name:      "same line as start but character before start.Character",
			line:      5,
			character: 2,
			want:      false,
		},
		{
			name:      "same line as end but character after end.Character",
			line:      10,
			character: 8,
			want:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := containsPosition(r, tc.line, tc.character)
			if got != tc.want {
				t.Errorf("containsPosition(r, %d, %d) = %v, want %v", tc.line, tc.character, got, tc.want)
			}
		})
	}
}

// --- TestFindInnermostSymbol ---

func TestFindInnermostSymbol_Empty(t *testing.T) {
	got := findInnermostSymbol([]types.DocumentSymbol{}, 5, 3)
	if got != nil {
		t.Errorf("expected nil for empty slice, got %+v", got)
	}
}

func TestFindInnermostSymbol_OuterOnly(t *testing.T) {
	sym := types.DocumentSymbol{
		Name: "MyFunc",
		Range: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 10, Character: 1},
		},
	}

	got := findInnermostSymbol([]types.DocumentSymbol{sym}, 5, 0)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Name != "MyFunc" {
		t.Errorf("expected MyFunc, got %q", got.Name)
	}
}

func TestFindInnermostSymbol_Nested(t *testing.T) {
	child := types.DocumentSymbol{
		Name: "ChildFunc",
		Range: types.Range{
			Start: types.Position{Line: 3, Character: 0},
			End:   types.Position{Line: 6, Character: 1},
		},
	}
	parent := types.DocumentSymbol{
		Name: "ParentFunc",
		Range: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 10, Character: 1},
		},
		Children: []types.DocumentSymbol{child},
	}

	// Cursor inside child range — should return child.
	got := findInnermostSymbol([]types.DocumentSymbol{parent}, 4, 5)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Name != "ChildFunc" {
		t.Errorf("expected ChildFunc, got %q", got.Name)
	}
}

func TestFindInnermostSymbol_ParentNotChild(t *testing.T) {
	child := types.DocumentSymbol{
		Name: "ChildFunc",
		Range: types.Range{
			Start: types.Position{Line: 3, Character: 0},
			End:   types.Position{Line: 6, Character: 1},
		},
	}
	parent := types.DocumentSymbol{
		Name: "ParentFunc",
		Range: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 10, Character: 1},
		},
		Children: []types.DocumentSymbol{child},
	}

	// Cursor inside parent but outside child — should return parent.
	got := findInnermostSymbol([]types.DocumentSymbol{parent}, 8, 0)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Name != "ParentFunc" {
		t.Errorf("expected ParentFunc, got %q", got.Name)
	}
}

func TestFindInnermostSymbol_NoMatch(t *testing.T) {
	sym := types.DocumentSymbol{
		Name: "MyFunc",
		Range: types.Range{
			Start: types.Position{Line: 0, Character: 0},
			End:   types.Position{Line: 5, Character: 1},
		},
	}

	// Cursor is well outside the symbol range.
	got := findInnermostSymbol([]types.DocumentSymbol{sym}, 20, 0)
	if got != nil {
		t.Errorf("expected nil for position outside all symbols, got %+v", got)
	}
}

// --- TestHandleGetSymbolSource_NilClient ---

func TestHandleGetSymbolSource_NilClient(t *testing.T) {
	ctx := context.Background()
	args := map[string]interface{}{
		"file_path": "/some/file.go",
		"line":      float64(1),
		"character": float64(1),
	}

	result, err := HandleGetSymbolSource(ctx, newNilClient(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for nil client, got false; text: %s", result.Content[0].Text)
	}
}
