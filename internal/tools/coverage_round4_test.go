package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTestPath_Defaults(t *testing.T) {
	tests := []struct {
		name string
		lang string
		path string
		want string
	}{
		{"go default", "go", "", "./..."},
		{"go with path", "go", "./pkg/foo", "./pkg/foo"},
		{"python default", "python", "", "."},
		{"python with path", "python", "tests/", "tests/"},
		{"unknown default", "rust", "", ""},
		{"unknown with path", "rust", "src/main.rs", "src/main.rs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTestPath(tt.lang, tt.path)
			if got != tt.want {
				t.Errorf("resolveTestPath(%q, %q) = %q, want %q", tt.lang, tt.path, got, tt.want)
			}
		})
	}
}

func TestApplyPathArg_WithPlaceholder(t *testing.T) {
	args := []string{"test", "{path}", "-v"}
	got := applyPathArg(args, "./pkg/foo")
	want := []string{"test", "./pkg/foo", "-v"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyPathArg_WithoutPlaceholder(t *testing.T) {
	args := []string{"test", "-v"}
	got := applyPathArg(args, "./pkg/foo")
	want := []string{"test", "-v", "./pkg/foo"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyPathArg_EmptyPath(t *testing.T) {
	args := []string{"test", "{path}", "-v"}
	got := applyPathArg(args, "")
	want := []string{"test", "-v"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyPathArg_EmptyPathNoPlaceholder(t *testing.T) {
	args := []string{"test", "-v"}
	got := applyPathArg(args, "")
	want := []string{"test", "-v"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFindGoMod_WalksUp(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "pkg", "api")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	gomodContent := "module github.com/example/myproject\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomodContent), 0644); err != nil {
		t.Fatal(err)
	}

	root, modName := findGoMod(subDir)
	if root != tmpDir {
		t.Errorf("findGoMod root = %q, want %q", root, tmpDir)
	}
	if modName != "github.com/example/myproject" {
		t.Errorf("findGoMod modName = %q, want %q", modName, "github.com/example/myproject")
	}
}

func TestFindGoMod_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	root, modName := findGoMod(tmpDir)
	if root != "" || modName != "" {
		t.Errorf("findGoMod should return empty for dir without go.mod, got root=%q mod=%q", root, modName)
	}
}

func TestParseBuildErrors_Go_MultipleErrors(t *testing.T) {
	output := []byte("main.go:42:10: undefined: foo\nutils.go:7:3: imported and not used: \"fmt\"\n")
	errors := parseBuildErrors("go", output)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}
	if errors[0].File != "main.go" || errors[0].Line != 42 || errors[0].Column != 10 {
		t.Errorf("error[0] = %+v", errors[0])
	}
	if errors[0].Message != "undefined: foo" {
		t.Errorf("error[0].Message = %q", errors[0].Message)
	}
	if errors[1].File != "utils.go" || errors[1].Line != 7 {
		t.Errorf("error[1] = %+v", errors[1])
	}
}

func TestParseBuildErrors_Go_NoMatches(t *testing.T) {
	output := []byte("Build successful!\nNo errors found.\n")
	errors := parseBuildErrors("go", output)
	if len(errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errors))
	}
}

func TestParseBuildErrors_UnknownLanguage(t *testing.T) {
	output := []byte("some output\n")
	errors := parseBuildErrors("cobol", output)
	if len(errors) != 0 {
		t.Errorf("expected 0 errors for unknown language, got %d", len(errors))
	}
}
