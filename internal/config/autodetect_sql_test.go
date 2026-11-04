package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fakeBinaries(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func sqlEntry(t *testing.T) *ServerEntry {
	t.Helper()
	cfg, err := AutodetectServers()
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Servers {
		if cfg.Servers[i].LanguageID == "sql" {
			return &cfg.Servers[i]
		}
	}
	return nil
}

// SQL prefers Supabase's Postgres Language Server and falls back to sqls.
func TestAutodetect_SQLPrefersPostgresLanguageServer(t *testing.T) {
	dir := fakeBinaries(t, "postgres-language-server", "sqls")
	t.Setenv("PATH", dir)
	e := sqlEntry(t)
	if e == nil {
		t.Fatal("no sql server detected")
	}
	if want := []string{filepath.Join(dir, "postgres-language-server"), "lsp-proxy"}; !reflect.DeepEqual(e.Command, want) {
		t.Fatalf("command = %v, want %v", e.Command, want)
	}

	dir = fakeBinaries(t, "sqls")
	t.Setenv("PATH", dir)
	e = sqlEntry(t)
	if e == nil || !reflect.DeepEqual(e.Command, []string{filepath.Join(dir, "sqls")}) {
		t.Fatalf("fallback entry = %+v, want sqls", e)
	}

	t.Setenv("PATH", t.TempDir())
	if e := sqlEntry(t); e != nil {
		t.Fatalf("detected %+v with neither server on PATH", e)
	}
}
