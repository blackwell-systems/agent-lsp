package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/google/jsonschema-go/jsonschema"
)

// --- fixNullableArrays: null appears second ---

func TestFixNullableArrays_NullSecondPosition(t *testing.T) {
	schema := &jsonschema.Schema{
		Types: []string{"number", "null"},
	}
	fixNullableArrays(schema)
	if schema.Type != "number" {
		t.Errorf("expected Type=%q, got %q", "number", schema.Type)
	}
	if schema.Types != nil {
		t.Errorf("expected Types=nil after collapse, got %v", schema.Types)
	}
}

func TestFixNullableArrays_DeeplyNestedThreeLevels(t *testing.T) {
	deepest := &jsonschema.Schema{
		Types: []string{"null", "boolean"},
	}
	itemSchema := &jsonschema.Schema{
		Type:                 "object",
		AdditionalProperties: deepest,
	}
	propSchema := &jsonschema.Schema{
		Type:  "array",
		Items: itemSchema,
	}
	root := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{"list": propSchema},
	}
	fixNullableArrays(root)
	if deepest.Type != "boolean" {
		t.Errorf("deepest nested Type = %q, want %q", deepest.Type, "boolean")
	}
}

func TestFixNullableArrays_OnlyNullSingle(t *testing.T) {
	schema := &jsonschema.Schema{
		Types: []string{"null"},
	}
	fixNullableArrays(schema)
	// Single-element; len(Types) == 1, not > 1, so no collapse
	if schema.Type != "" {
		t.Errorf("expected Type unchanged (empty), got %q", schema.Type)
	}
}

// --- buildLspArgs coverage ---

func TestBuildLspArgs_SingleServer(t *testing.T) {
	entries := []config.ServerEntry{
		{LanguageID: "go", Command: []string{"gopls"}},
	}
	args := buildLspArgs(entries)
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "go:gopls" {
		t.Errorf("got %q, want %q", args[0], "go:gopls")
	}
}

func TestBuildLspArgs_ServerWithExtraArgs(t *testing.T) {
	entries := []config.ServerEntry{
		{LanguageID: "python", Command: []string{"/usr/bin/pyright", "--stdio", "--verbose"}},
	}
	args := buildLspArgs(entries)
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "python:pyright,--stdio,--verbose" {
		t.Errorf("got %q, want %q", args[0], "python:pyright,--stdio,--verbose")
	}
}

func TestBuildLspArgs_MultipleServers(t *testing.T) {
	entries := []config.ServerEntry{
		{LanguageID: "go", Command: []string{"gopls"}},
		{LanguageID: "typescript", Command: []string{"typescript-language-server", "--stdio"}},
	}
	args := buildLspArgs(entries)
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestBuildLspArgs_Empty(t *testing.T) {
	args := buildLspArgs(nil)
	if len(args) != 0 {
		t.Errorf("expected 0 args for nil input, got %d", len(args))
	}
}

// --- resolveRulesPath coverage ---

func TestResolveRulesPath_ClaudeProject(t *testing.T) {
	got := resolveRulesPath(1)
	if !strings.HasSuffix(got, "CLAUDE.md") {
		t.Errorf("expected CLAUDE.md suffix, got %q", got)
	}
}

func TestResolveRulesPath_ClaudeDesktopNoRules(t *testing.T) {
	got := resolveRulesPath(3)
	if got != "" {
		t.Errorf("Claude Desktop should return empty rules path, got %q", got)
	}
}

func TestResolveRulesPath_Cursor(t *testing.T) {
	got := resolveRulesPath(4)
	if !strings.HasSuffix(got, "agent-lsp.mdc") {
		t.Errorf("expected agent-lsp.mdc suffix, got %q", got)
	}
	if !strings.Contains(got, ".cursor") {
		t.Errorf("expected .cursor in path, got %q", got)
	}
}

func TestResolveRulesPath_Cline(t *testing.T) {
	got := resolveRulesPath(5)
	if !strings.HasSuffix(got, ".clinerules") {
		t.Errorf("expected .clinerules suffix, got %q", got)
	}
}

func TestResolveRulesPath_Gemini(t *testing.T) {
	got := resolveRulesPath(7)
	if !strings.HasSuffix(got, "GEMINI.md") {
		t.Errorf("expected GEMINI.md suffix, got %q", got)
	}
}

func TestResolveRulesPath_UnknownChoice(t *testing.T) {
	got := resolveRulesPath(999)
	if got != "" {
		t.Errorf("expected empty for unknown choice, got %q", got)
	}
}

// --- writeManagedSection coverage ---

func TestWriteManagedSection_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "CLAUDE.md")
	err := writeManagedSection(path, "test content\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, managedSectionStart) {
		t.Error("missing section start sentinel")
	}
	if !strings.Contains(content, managedSectionEnd) {
		t.Error("missing section end sentinel")
	}
	if !strings.Contains(content, "test content") {
		t.Error("missing managed content")
	}
}

func TestWriteManagedSection_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	initial := "# My project\n\n" + managedSectionStart + "\nold content\n" + managedSectionEnd + "\n\n## Other stuff\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeManagedSection(path, "new content\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if strings.Contains(content, "old content") {
		t.Error("old content should have been replaced")
	}
	if !strings.Contains(content, "new content") {
		t.Error("new content should be present")
	}
	if !strings.Contains(content, "# My project") {
		t.Error("content before managed section should be preserved")
	}
	if !strings.Contains(content, "## Other stuff") {
		t.Error("content after managed section should be preserved")
	}
}

func TestWriteManagedSection_AppendsToFileWithoutSentinels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	existing := "# Existing content\n\nSome notes here.\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeManagedSection(path, "appended\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "# Existing content") {
		t.Error("original content should be preserved")
	}
	if !strings.Contains(content, "appended") {
		t.Error("managed section should be appended")
	}
}

// --- findAsset: empty list ---

func TestFindAsset_EmptyList(t *testing.T) {
	_, err := findAsset(nil, "linux", "amd64")
	if err == nil {
		t.Error("expected error for empty asset list")
	}
}

// --- generateRulesContent (smoke test) ---

func TestGenerateRulesContent_NonEmpty(t *testing.T) {
	content := generateRulesContent()
	if content == "" {
		t.Error("expected non-empty rules content")
	}
	if !strings.Contains(content, "agent-lsp") {
		t.Error("rules content should mention agent-lsp")
	}
	if !strings.Contains(content, "get_change_impact") {
		t.Error("rules content should mention get_change_impact")
	}
}

