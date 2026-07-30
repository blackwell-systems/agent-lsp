package config_test

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/config"
)

func keys(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Servers))
	for _, e := range cfg.Servers {
		k := e.LanguageID
		if k == "" && len(e.Extensions) > 0 {
			k = e.Extensions[0]
		}
		out = append(out, k)
	}
	return out
}

func TestMergeConfigs_OverrideAddKeep(t *testing.T) {
	base := &config.Config{Servers: []config.ServerEntry{
		{LanguageID: "go", Extensions: []string{"go"}, Command: []string{"gopls"}},
		{LanguageID: "python", Extensions: []string{"py"}, Command: []string{"pyright-langserver", "--stdio"}},
	}}
	override := &config.Config{Servers: []config.ServerEntry{
		// Override the auto-detected go server with a custom binary.
		{LanguageID: "go", Extensions: []string{"go"}, Command: []string{"/opt/gopls-custom"}},
		// Add a server for a language auto-detection did not provide.
		{LanguageID: "zig", Extensions: []string{"zig"}, Command: []string{"zls"}},
	}}

	merged := config.MergeConfigs(base, override)

	// go (overridden, in place) + python (kept) + zig (added), order preserved.
	if got := keys(merged); len(got) != 3 || got[0] != "go" || got[1] != "python" || got[2] != "zig" {
		t.Fatalf("unexpected merged keys/order: %v", got)
	}
	// go entry must carry the override command, not gopls.
	if merged.Servers[0].Command[0] != "/opt/gopls-custom" {
		t.Errorf("go override did not win: %v", merged.Servers[0].Command)
	}
	// python entry (untouched base) must survive.
	if merged.Servers[1].Command[0] != "pyright-langserver" {
		t.Errorf("base python entry not kept: %v", merged.Servers[1].Command)
	}
}

func TestMergeConfigs_KeyByExtensionWhenNoLanguageID(t *testing.T) {
	base := &config.Config{Servers: []config.ServerEntry{
		{Extensions: []string{"rs"}, Command: []string{"rust-analyzer"}},
	}}
	override := &config.Config{Servers: []config.ServerEntry{
		{Extensions: []string{"rs"}, Command: []string{"/custom/rust-analyzer"}},
	}}
	merged := config.MergeConfigs(base, override)
	if len(merged.Servers) != 1 {
		t.Fatalf("expected extension-keyed override to replace, got %d entries", len(merged.Servers))
	}
	if merged.Servers[0].Command[0] != "/custom/rust-analyzer" {
		t.Errorf("extension-keyed override did not win: %v", merged.Servers[0].Command)
	}
}

func TestMergeConfigs_NilArgs(t *testing.T) {
	only := &config.Config{Servers: []config.ServerEntry{{LanguageID: "go", Command: []string{"gopls"}}}}

	if m := config.MergeConfigs(nil, only); len(m.Servers) != 1 || m.Servers[0].LanguageID != "go" {
		t.Errorf("nil base should yield override entries: %v", m.Servers)
	}
	if m := config.MergeConfigs(only, nil); len(m.Servers) != 1 || m.Servers[0].LanguageID != "go" {
		t.Errorf("nil override should yield base entries: %v", m.Servers)
	}
	if m := config.MergeConfigs(nil, nil); len(m.Servers) != 0 {
		t.Errorf("nil/nil should yield empty config, got %v", m.Servers)
	}
}

func TestMergeConfigs_KeylessEntriesAlwaysAppend(t *testing.T) {
	// Entries with neither LanguageID nor Extensions have an empty key and must
	// never collapse onto each other.
	base := &config.Config{Servers: []config.ServerEntry{{Command: []string{"a"}}}}
	override := &config.Config{Servers: []config.ServerEntry{{Command: []string{"b"}}}}
	merged := config.MergeConfigs(base, override)
	if len(merged.Servers) != 2 {
		t.Fatalf("keyless entries should both be kept, got %d", len(merged.Servers))
	}
}
