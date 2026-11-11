package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

// Common Lisp is detected from cl-lsp on PATH, like every other server.
func TestAutodetect_LispDetectsClLsp(t *testing.T) {
	dir := fakeBinaries(t, "cl-lsp")
	t.Setenv("PATH", dir)

	cfg, err := AutodetectServers()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range cfg.Servers {
		if e.LanguageID != "lisp" {
			continue
		}
		if want := []string{filepath.Join(dir, "cl-lsp")}; !reflect.DeepEqual(e.Command, want) {
			t.Fatalf("command = %v, want %v", e.Command, want)
		}
		if want := []string{"lisp", "lsp", "cl", "asd"}; !reflect.DeepEqual(e.Extensions, want) {
			t.Fatalf("extensions = %v, want %v", e.Extensions, want)
		}
		return
	}
	t.Fatal("no lisp server detected")
}
