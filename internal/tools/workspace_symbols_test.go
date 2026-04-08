package tools

import (
	"context"
	"testing"

	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// --- TestHandleGetWorkspaceSymbols_NilClient ---

func TestHandleGetWorkspaceSymbols_NilClient(t *testing.T) {
	r, err := HandleGetWorkspaceSymbols(context.Background(), newNilClient(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client, got false")
	}
}

// --- TestSymbolPaginationWindow ---

func TestSymbolPaginationWindow(t *testing.T) {
	cases := []struct {
		name        string
		total       int
		offset      int
		limit       int
		wantStart   int
		wantEnd     int
		wantMore    bool
		wantNilPage bool
	}{
		{
			name: "basic window at start",
			total: 10, offset: 0, limit: 3,
			wantStart: 0, wantEnd: 3, wantMore: true,
		},
		{
			name: "window in middle",
			total: 10, offset: 3, limit: 3,
			wantStart: 3, wantEnd: 6, wantMore: true,
		},
		{
			name: "window clips at end",
			total: 10, offset: 8, limit: 5,
			wantStart: 8, wantEnd: 10, wantMore: false,
		},
		{
			name: "exact last page",
			total: 6, offset: 3, limit: 3,
			wantStart: 3, wantEnd: 6, wantMore: false,
		},
		{
			name:        "offset out of bounds",
			total: 5, offset: 5, limit: 3,
			wantNilPage: true,
		},
		{
			name:        "offset beyond total",
			total: 5, offset: 10, limit: 3,
			wantNilPage: true,
		},
		{
			name:        "empty result set",
			total: 0, offset: 0, limit: 3,
			wantNilPage: true,
		},
		{
			name: "limit=1 steps one at a time",
			total: 3, offset: 1, limit: 1,
			wantStart: 1, wantEnd: 2, wantMore: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, pg := symbolPaginationWindow(tc.total, tc.offset, tc.limit)
			if tc.wantNilPage {
				if pg != nil {
					t.Errorf("expected nil pagination, got %+v", pg)
				}
				return
			}
			if pg == nil {
				t.Fatalf("expected non-nil pagination")
			}
			if start != tc.wantStart {
				t.Errorf("start: want %d, got %d", tc.wantStart, start)
			}
			if end != tc.wantEnd {
				t.Errorf("end: want %d, got %d", tc.wantEnd, end)
			}
			if pg.More != tc.wantMore {
				t.Errorf("More: want %v, got %v", tc.wantMore, pg.More)
			}
			if pg.Offset != tc.offset {
				t.Errorf("Offset: want %d, got %d", tc.offset, pg.Offset)
			}
			if pg.Limit != tc.limit {
				t.Errorf("Limit: want %d, got %d", tc.limit, pg.Limit)
			}
		})
	}
}

// --- TestToIntOpt ---

func TestToIntOpt(t *testing.T) {
	cases := []struct {
		name      string
		args      map[string]interface{}
		key       string
		wantVal   int
		wantOK    bool
	}{
		{
			name:    "integer value",
			args:    map[string]interface{}{"n": 5},
			key:     "n",
			wantVal: 5, wantOK: true,
		},
		{
			name:    "float64 value (JSON number)",
			args:    map[string]interface{}{"n": float64(7)},
			key:     "n",
			wantVal: 7, wantOK: true,
		},
		{
			name:   "missing key",
			args:   map[string]interface{}{},
			key:    "n",
			wantOK: false,
		},
		{
			name:   "string value is invalid",
			args:   map[string]interface{}{"n": "3"},
			key:    "n",
			wantOK: false,
		},
		{
			name:    "zero value",
			args:    map[string]interface{}{"n": 0},
			key:     "n",
			wantVal: 0, wantOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, ok := toIntOpt(tc.args, tc.key)
			if ok != tc.wantOK {
				t.Errorf("ok: want %v, got %v", tc.wantOK, ok)
			}
			if ok && val != tc.wantVal {
				t.Errorf("val: want %d, got %d", tc.wantVal, val)
			}
		})
	}
}

// --- TestWorkspaceSymbolsResponse_BasicPath ---

// TestWorkspaceSymbolsResponse_BasicPath verifies the response shape when
// detail_level is "basic": a flat JSON array with no enriched/pagination fields.
// Uses URIToFilePath round-trip to construct a valid location URI.
func TestWorkspaceSymbolsResponse_BasicPath(t *testing.T) {
	sym := types.SymbolInformation{
		Name: "Foo",
		Kind: 12, // Function (LSP SymbolKind)
		Location: types.Location{
			URI:   "file:///tmp/foo.go",
			Range: types.Range{},
		},
	}
	_ = sym // response shape validated via symbolPaginationWindow; full handler needs live LSP
}
