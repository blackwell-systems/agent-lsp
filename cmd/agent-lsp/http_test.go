package main

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/config"
)

func TestParseArgs_HTTPFlags(t *testing.T) {
	t.Run("http with port and token", func(t *testing.T) {
		result, err := config.ParseArgs([]string{"--http", "--port", "9999", "--token", "secret", "go:gopls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.HTTPMode {
			t.Error("expected HTTPMode=true")
		}
		if result.HTTPPort != 9999 {
			t.Errorf("expected HTTPPort=9999, got %d", result.HTTPPort)
		}
		if result.HTTPToken != "secret" {
			t.Errorf("expected HTTPToken=secret, got %q", result.HTTPToken)
		}
	})

	t.Run("http only uses default port", func(t *testing.T) {
		// --http alone: remainder is empty, triggers auto-detect path.
		// We can't fully test auto-detect in unit test, but we verify the
		// HTTP flags are parsed correctly by using a valid lsp arg.
		result, err := config.ParseArgs([]string{"--http", "go:gopls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.HTTPMode {
			t.Error("expected HTTPMode=true")
		}
		if result.HTTPPort != 8080 {
			t.Errorf("expected HTTPPort=8080 (default), got %d", result.HTTPPort)
		}
	})

	t.Run("no http flag leaves HTTPMode false", func(t *testing.T) {
		result, err := config.ParseArgs([]string{"go:gopls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.HTTPMode {
			t.Error("expected HTTPMode=false")
		}
	})
}
