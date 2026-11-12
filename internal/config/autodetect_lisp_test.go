package config

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// fakeLispBinary writes a cl-lsp executable that exec.LookPath resolves on
// every platform, returning the directory and the binary name.
func fakeLispBinary(t *testing.T) (string, string) {
	t.Helper()
	name := "cl-lsp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return fakeBinaries(t, name), name
}

// Common Lisp is detected from cl-lsp on PATH, like every other server.
func TestAutodetect_LispDetectsClLsp(t *testing.T) {
	dir, name := fakeLispBinary(t)
	t.Setenv("PATH", dir)

	cfg, err := AutodetectServers()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range cfg.Servers {
		if e.LanguageID != "lisp" {
			continue
		}
		if want := []string{filepath.Join(dir, name)}; !reflect.DeepEqual(e.Command, want) {
			t.Fatalf("command = %v, want %v", e.Command, want)
		}
		if want := []string{"lisp", "lsp", "cl", "asd"}; !reflect.DeepEqual(e.Extensions, want) {
			t.Fatalf("extensions = %v, want %v", e.Extensions, want)
		}
		return
	}
	t.Fatal("no lisp server detected")
}
