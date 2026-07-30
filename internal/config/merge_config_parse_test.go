package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/config"
)

// writeConfig marshals cfg to a temp file and returns its path.
func writeConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lsp-mcp.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// findServer returns the merged entry whose key (LanguageID or first extension)
// matches key, or nil.
func findServer(cfg *config.Config, key string) *config.ServerEntry {
	for i := range cfg.Servers {
		e := &cfg.Servers[i]
		k := e.LanguageID
		if k == "" && len(e.Extensions) > 0 {
			k = e.Extensions[0]
		}
		if k == key {
			return e
		}
	}
	return nil
}

// --- Additional MergeConfigs edge cases ------------------------------------

// A base entry keyed by LanguageID and an override entry keyed only by
// Extensions must collapse onto the same key ("go") and override in place.
func TestMergeConfigs_MixedKeyingCollapses(t *testing.T) {
	base := &config.Config{Servers: []config.ServerEntry{
		{LanguageID: "go", Extensions: []string{"go"}, Command: []string{"gopls"}},
	}}
	override := &config.Config{Servers: []config.ServerEntry{
		{Extensions: []string{"go"}, Command: []string{"/custom/gopls"}}, // no LanguageID
	}}
	merged := config.MergeConfigs(base, override)
	if len(merged.Servers) != 1 {
		t.Fatalf("expected langID/extension keys to collapse, got %d entries", len(merged.Servers))
	}
	if merged.Servers[0].Command[0] != "/custom/gopls" {
		t.Errorf("override did not win across mixed keying: %v", merged.Servers[0].Command)
	}
}

// Two override entries with the same key: the last one wins.
func TestMergeConfigs_DuplicateOverrideKeyLastWins(t *testing.T) {
	base := &config.Config{Servers: []config.ServerEntry{
		{LanguageID: "go", Command: []string{"gopls"}},
	}}
	override := &config.Config{Servers: []config.ServerEntry{
		{LanguageID: "go", Command: []string{"first"}},
		{LanguageID: "go", Command: []string{"last"}},
	}}
	merged := config.MergeConfigs(base, override)
	if len(merged.Servers) != 1 {
		t.Fatalf("expected 1 entry after duplicate-key overrides, got %d", len(merged.Servers))
	}
	if merged.Servers[0].Command[0] != "last" {
		t.Errorf("expected last override to win, got %v", merged.Servers[0].Command)
	}
}

// An override with no servers leaves the base untouched.
func TestMergeConfigs_EmptyOverrideKeepsBase(t *testing.T) {
	base := &config.Config{Servers: []config.ServerEntry{
		{LanguageID: "go", Command: []string{"gopls"}},
		{LanguageID: "python", Command: []string{"pyright-langserver"}},
	}}
	merged := config.MergeConfigs(base, &config.Config{})
	if len(merged.Servers) != 2 {
		t.Fatalf("empty override should keep both base entries, got %d", len(merged.Servers))
	}
}

// The merged slice must be independent of the base slice (no aliasing that could
// let a later mutation leak backwards).
func TestMergeConfigs_DoesNotAliasBaseSlice(t *testing.T) {
	base := &config.Config{Servers: []config.ServerEntry{{LanguageID: "go", Command: []string{"gopls"}}}}
	merged := config.MergeConfigs(base, &config.Config{Servers: []config.ServerEntry{{LanguageID: "zig", Command: []string{"zls"}}}})
	if len(base.Servers) != 1 {
		t.Fatalf("base slice mutated by merge: now %d entries", len(base.Servers))
	}
	if len(merged.Servers) != 2 {
		t.Fatalf("merged should have 2 entries, got %d", len(merged.Servers))
	}
}

// --- --merge-config parse-branch coverage ----------------------------------

// A valid --merge-config: a config-only language must appear, and an overridden
// language must carry the config's command. Deterministic regardless of which
// servers happen to be installed on the test host.
func TestParseArgs_MergeConfig_OverridesAndAdds(t *testing.T) {
	cfgPath := writeConfig(t, config.Config{Servers: []config.ServerEntry{
		{LanguageID: "go", Extensions: []string{"go"}, Command: []string{"/opt/custom-gopls"}},
		{LanguageID: "cobol", Extensions: []string{"cbl"}, Command: []string{"cobol-ls"}},
	}})

	result, err := config.ParseArgs([]string{"--merge-config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Config == nil {
		t.Fatal("expected Config to be set")
	}
	// Overridden "go" carries the config command whether or not gopls is on PATH.
	goEntry := findServer(result.Config, "go")
	if goEntry == nil || goEntry.Command[0] != "/opt/custom-gopls" {
		t.Errorf("go override missing or wrong: %+v", goEntry)
	}
	// Config-only language is always present (auto-detection never provides it).
	if cobol := findServer(result.Config, "cobol"); cobol == nil || cobol.Command[0] != "cobol-ls" {
		t.Errorf("config-only cobol server missing: %+v", cobol)
	}
}

// --merge-config with an empty config still succeeds (result is the auto-detected
// base, possibly empty), and never errors on the merge itself.
func TestParseArgs_MergeConfig_EmptyConfig(t *testing.T) {
	cfgPath := writeConfig(t, config.Config{})
	result, err := config.ParseArgs([]string{"--merge-config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Config == nil {
		t.Fatal("expected a (possibly empty) Config, got nil")
	}
}

// --merge-config must compose with HTTP flags, in either order.
func TestParseArgs_MergeConfig_WithHTTPFlags(t *testing.T) {
	cfgPath := writeConfig(t, config.Config{Servers: []config.ServerEntry{
		{LanguageID: "cobol", Extensions: []string{"cbl"}, Command: []string{"cobol-ls"}},
	}})
	for _, args := range [][]string{
		{"--merge-config", cfgPath, "--http", "--port", "9001"},
		{"--http", "--port", "9001", "--merge-config", cfgPath},
	} {
		result, err := config.ParseArgs(args)
		if err != nil {
			t.Fatalf("args %v: unexpected error: %v", args, err)
		}
		if !result.HTTPMode || result.HTTPPort != 9001 {
			t.Errorf("args %v: HTTP flags not applied (mode=%v port=%d)", args, result.HTTPMode, result.HTTPPort)
		}
		if findServer(result.Config, "cobol") == nil {
			t.Errorf("args %v: merged config missing cobol server", args)
		}
	}
}

// --merge-config with no path argument is a usage error.
func TestParseArgs_MergeConfig_MissingPath(t *testing.T) {
	_, err := config.ParseArgs([]string{"--merge-config"})
	if err == nil {
		t.Fatal("expected error for --merge-config with no path")
	}
	if !strings.Contains(err.Error(), "--merge-config requires a file path") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --merge-config pointing at a nonexistent file surfaces the load error.
func TestParseArgs_MergeConfig_NonexistentFile(t *testing.T) {
	_, err := config.ParseArgs([]string{"--merge-config", "/no/such/file.json"})
	if err == nil {
		t.Fatal("expected error for nonexistent merge-config file")
	}
	if !strings.Contains(err.Error(), "load merge-config") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --merge-config pointing at malformed JSON surfaces a parse error.
func TestParseArgs_MergeConfig_InvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.ParseArgs([]string{"--merge-config", p})
	if err == nil {
		t.Fatal("expected error for malformed merge-config JSON")
	}
	if !strings.Contains(err.Error(), "load merge-config") {
		t.Errorf("unexpected error message: %v", err)
	}
}
