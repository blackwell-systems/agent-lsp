package uri

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestURIToPath(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "standard file URI",
			uri:  "file:///home/user/project/main.go",
			want: "/home/user/project/main.go",
		},
		{
			name: "percent-encoded spaces",
			uri:  "file:///home/user/my%20project/main.go",
			want: "/home/user/my project/main.go",
		},
		{
			name: "percent-encoded special chars",
			uri:  "file:///home/user/project%23v2/main.go",
			want: "/home/user/project#v2/main.go",
		},
		{
			name: "bare path passthrough",
			uri:  "/home/user/project/main.go",
			want: "/home/user/project/main.go",
		},
		{
			name: "empty string",
			uri:  "",
			want: "",
		},
		{
			name: "file URI with no path returns prefix-stripped",
			uri:  "file://",
			want: "",
		},
		{
			name: "windows-style file URI",
			uri:  "file:///C:/Users/user/project/main.go",
			want: "/C:/Users/user/project/main.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URIToPath(tt.uri)
			if got != tt.want {
				t.Errorf("URIToPath(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestApplyRangeEdit_SingleLineReplace(t *testing.T) {
	content := "hello world"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 6},
		End:   types.Position{Line: 0, Character: 11},
	}
	got := ApplyRangeEdit(content, rng, "Go")
	if got != "hello Go" {
		t.Errorf("got %q, want %q", got, "hello Go")
	}
}

func TestApplyRangeEdit_Insert(t *testing.T) {
	content := "ab"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 1},
		End:   types.Position{Line: 0, Character: 1},
	}
	got := ApplyRangeEdit(content, rng, "X")
	if got != "aXb" {
		t.Errorf("got %q, want %q", got, "aXb")
	}
}

func TestApplyRangeEdit_MultiLineReplace(t *testing.T) {
	content := "line1\nline2\nline3"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 3},
		End:   types.Position{Line: 2, Character: 3},
	}
	got := ApplyRangeEdit(content, rng, "REPLACED")
	want := "linREPLACEDe3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyRangeEdit_DeleteLine(t *testing.T) {
	content := "first\nsecond\nthird"
	rng := types.Range{
		Start: types.Position{Line: 1, Character: 0},
		End:   types.Position{Line: 1, Character: 6},
	}
	got := ApplyRangeEdit(content, rng, "")
	want := "first\n\nthird"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyRangeEdit_InsertNewLines(t *testing.T) {
	content := "line1\nline3"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 5},
		End:   types.Position{Line: 0, Character: 5},
	}
	got := ApplyRangeEdit(content, rng, "\nline2")
	want := "line1\nline2\nline3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyRangeEdit_ClampOutOfBounds(t *testing.T) {
	content := "short"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 100},
		End:   types.Position{Line: 0, Character: 200},
	}
	got := ApplyRangeEdit(content, rng, "X")
	want := "shortX"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyRangeEdit_ClampOutOfBoundsLine(t *testing.T) {
	content := "only"
	rng := types.Range{
		Start: types.Position{Line: 99, Character: 0},
		End:   types.Position{Line: 99, Character: 4},
	}
	got := ApplyRangeEdit(content, rng, "replaced")
	want := "replaced"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyRangeEdit_ReplaceEntireContent(t *testing.T) {
	content := "abc\ndef"
	rng := types.Range{
		Start: types.Position{Line: 0, Character: 0},
		End:   types.Position{Line: 1, Character: 3},
	}
	got := ApplyRangeEdit(content, rng, "XYZ")
	want := "XYZ"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
