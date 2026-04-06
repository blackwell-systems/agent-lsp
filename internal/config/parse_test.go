package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/lsp-mcp-go/internal/config"
)

func TestParseArgs_Legacy(t *testing.T) {
	// Create a temporary fake binary so os.Stat passes.
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "gopls")
	if err := os.WriteFile(bin, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	result, err := config.ParseArgs([]string{"go", bin})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsSingleServer {
		t.Error("expected IsSingleServer=true")
	}
	if result.LanguageID != "go" {
		t.Errorf("expected LanguageID=go, got %q", result.LanguageID)
	}
	if result.ServerPath != bin {
		t.Errorf("expected ServerPath=%q, got %q", bin, result.ServerPath)
	}
	if len(result.ServerArgs) != 0 {
		t.Errorf("expected no ServerArgs, got %v", result.ServerArgs)
	}
}

func TestParseArgs_MultiArg(t *testing.T) {
	result, err := config.ParseArgs([]string{
		"go:gopls",
		"typescript:typescript-language-server,--stdio",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Config == nil {
		t.Fatal("expected Config to be set")
	}
	if len(result.Config.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(result.Config.Servers))
	}

	goEntry := result.Config.Servers[0]
	if len(goEntry.Extensions) != 1 || goEntry.Extensions[0] != "go" {
		t.Errorf("expected go extensions=[go], got %v", goEntry.Extensions)
	}

	tsEntry := result.Config.Servers[1]
	if len(tsEntry.Extensions) != 2 {
		t.Errorf("expected typescript extensions=[ts,tsx], got %v", tsEntry.Extensions)
	}
	foundTS, foundTSX := false, false
	for _, ext := range tsEntry.Extensions {
		if ext == "ts" {
			foundTS = true
		}
		if ext == "tsx" {
			foundTSX = true
		}
	}
	if !foundTS || !foundTSX {
		t.Errorf("expected ts and tsx in extensions, got %v", tsEntry.Extensions)
	}
}

func TestParseArgs_ConfigFlag(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "lsp-mcp.json")

	cfg := config.Config{
		Servers: []config.ServerEntry{
			{Extensions: []string{"go"}, Command: []string{"gopls"}},
			{Extensions: []string{"ts", "tsx"}, Command: []string{"typescript-language-server", "--stdio"}},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := config.ParseArgs([]string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Config == nil {
		t.Fatal("expected Config to be set")
	}
	if len(result.Config.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(result.Config.Servers))
	}
	if result.Config.Servers[0].Extensions[0] != "go" {
		t.Errorf("expected first server extension=go, got %v", result.Config.Servers[0].Extensions)
	}
}

func TestParseArgs_AutoEmpty(t *testing.T) {
	// NOTE: This test requires at least one language server in PATH to pass.
	// If no servers are found, AutodetectServers() will return an error.
	result, err := config.ParseArgs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Config == nil {
		t.Fatal("expected Config to be set")
	}
	if len(result.Config.Servers) == 0 {
		t.Error("expected at least one server found via auto-detect")
	}
	if result.IsSingleServer {
		t.Error("expected IsSingleServer=false for auto-detect mode")
	}
}

func TestParseArgs_AutoFlag(t *testing.T) {
	// NOTE: This test requires at least one language server in PATH to pass.
	// If no servers are found, AutodetectServers() will return an error.
	result, err := config.ParseArgs([]string{"--auto"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Config == nil {
		t.Fatal("expected Config to be set")
	}
	if len(result.Config.Servers) == 0 {
		t.Error("expected at least one server found via auto-detect")
	}
	if result.IsSingleServer {
		t.Error("expected IsSingleServer=false for auto-detect mode")
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	data := `{"servers":[{"extensions":["go"],"command":["gopls"]}]}`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if len(cfg.Servers[0].Extensions) != 1 || cfg.Servers[0].Extensions[0] != "go" {
		t.Errorf("expected Extensions=[go], got %v", cfg.Servers[0].Extensions)
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	_, err := config.LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Error("expected error for nonexistent config file, got nil")
	}
}
