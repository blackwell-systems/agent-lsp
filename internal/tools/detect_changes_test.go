package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitDiffArgs(t *testing.T) {
	tests := []struct {
		scope     string
		diffRange string
		want      []string
	}{
		{"unstaged", "", []string{"diff", "--name-only"}},
		{"staged", "", []string{"diff", "--name-only", "--cached"}},
		{"committed", "", []string{"diff", "--name-only", "HEAD~1", "HEAD"}},
		{"", "", []string{"diff", "--name-only"}}, // default
		{"committed", "v0.7.0..HEAD", []string{"diff", "--name-only", "v0.7.0", "HEAD"}},
		{"committed", "abc123..def456", []string{"diff", "--name-only", "abc123", "def456"}},
		{"committed", "main", []string{"diff", "--name-only", "main~1", "main"}},
		{"staged", "v0.7.0..HEAD", []string{"diff", "--name-only", "--cached"}}, // range ignored for staged
		{"unstaged", "v0.7.0..HEAD", []string{"diff", "--name-only"}},           // range ignored for unstaged
	}

	for _, tc := range tests {
		got := gitDiffArgs(tc.scope, tc.diffRange)
		if len(got) != len(tc.want) {
			t.Errorf("gitDiffArgs(%q, %q): got %v, want %v", tc.scope, tc.diffRange, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("gitDiffArgs(%q, %q)[%d]: got %q, want %q", tc.scope, tc.diffRange, i, got[i], tc.want[i])
			}
		}
	}
}

func TestFilterChangedFiles(t *testing.T) {
	dir := t.TempDir()

	// Create files with recognized extensions.
	goFile := filepath.Join(dir, "main.go")
	pyFile := filepath.Join(dir, "app.py")
	// Create a plaintext file.
	txtFile := filepath.Join(dir, "readme.txt")
	// A file that does not exist on disk.
	missingFile := "ghost.go"

	for _, f := range []string{goFile, pyFile, txtFile} {
		if err := os.WriteFile(f, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := filterChangedFiles(dir, []string{
		goFile,      // absolute, recognized
		pyFile,      // absolute, recognized
		txtFile,     // absolute, plaintext (filtered out)
		missingFile, // relative, does not exist (filtered out)
		"main.go",   // relative, exists, recognized
	})

	// Expect goFile, pyFile, and the relative main.go (resolved to absolute).
	if len(result) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(result), result)
	}

	// Verify all results are absolute paths.
	for _, p := range result {
		if !filepath.IsAbs(p) {
			t.Errorf("expected absolute path, got %q", p)
		}
	}
}

func TestClassifyRisk(t *testing.T) {
	tests := []struct {
		name       string
		callers    []symbolRef
		symbolFile string
		want       string
	}{
		{
			name:       "no callers",
			callers:    nil,
			symbolFile: "/project/pkg/a.go",
			want:       "low",
		},
		{
			name: "same package callers",
			callers: []symbolRef{
				{Name: "Foo", File: "/project/pkg/b.go", Line: 10},
				{Name: "Foo", File: "/project/pkg/c.go", Line: 20},
			},
			symbolFile: "/project/pkg/a.go",
			want:       "medium",
		},
		{
			name: "cross package callers",
			callers: []symbolRef{
				{Name: "Foo", File: "/project/pkg/b.go", Line: 10},
				{Name: "Foo", File: "/project/other/c.go", Line: 20},
			},
			symbolFile: "/project/pkg/a.go",
			want:       "high",
		},
		{
			name: "single caller same package",
			callers: []symbolRef{
				{Name: "Bar", File: "/project/pkg/b.go", Line: 5},
			},
			symbolFile: "/project/pkg/a.go",
			want:       "medium",
		},
		{
			name: "single caller different package",
			callers: []symbolRef{
				{Name: "Bar", File: "/project/other/b.go", Line: 5},
			},
			symbolFile: "/project/pkg/a.go",
			want:       "high",
		},
	}

	for _, tc := range tests {
		got := classifyRisk(tc.callers, tc.symbolFile)
		if got != tc.want {
			t.Errorf("%s: classifyRisk() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
