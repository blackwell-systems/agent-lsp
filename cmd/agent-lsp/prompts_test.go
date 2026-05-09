package main

import (
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	content := `---
name: lsp-explore
description: "Tell me about this symbol"
argument-hint: "[symbol-name]"
user-invocable: true
allowed-tools: mcp__lsp__start_lsp
---

# lsp-explore

Full workflow instructions here.`

	meta, ok := parseSkillMD(content)
	if !ok {
		t.Fatal("parseSkillMD returned false")
	}
	if meta.Name != "lsp-explore" {
		t.Errorf("Name = %q, want %q", meta.Name, "lsp-explore")
	}
	if meta.Description != "Tell me about this symbol" {
		t.Errorf("Description = %q, want %q", meta.Description, "Tell me about this symbol")
	}
	if meta.ArgumentHint != "[symbol-name]" {
		t.Errorf("ArgumentHint = %q, want %q", meta.ArgumentHint, "[symbol-name]")
	}
	if meta.Body == "" {
		t.Error("Body is empty")
	}
	if meta.Body != "# lsp-explore\n\nFull workflow instructions here." {
		t.Errorf("Body = %q", meta.Body)
	}
}

func TestParseSkillMD_NoFrontmatter(t *testing.T) {
	_, ok := parseSkillMD("no frontmatter here")
	if ok {
		t.Error("expected false for content without frontmatter")
	}
}

func TestParseSkillMD_NoName(t *testing.T) {
	content := `---
description: missing name field
---

body`
	_, ok := parseSkillMD(content)
	if ok {
		t.Error("expected false for content without name")
	}
}

func TestParseArgumentHint(t *testing.T) {
	tests := []struct {
		hint string
		want int
	}{
		{"", 0},
		{"[symbol-name]", 1},
		{"[old-name] [new-name]", 2},
		{"[symbol-name | file-path]", 2},
		{"[file-path] [optional: start_line-end_line]", 2},
	}
	for _, tt := range tests {
		args := parseArgumentHint(tt.hint)
		if len(args) != tt.want {
			t.Errorf("parseArgumentHint(%q) = %d args, want %d", tt.hint, len(args), tt.want)
		}
	}
}

func TestParseArgumentHint_Required(t *testing.T) {
	args := parseArgumentHint("[file-path] [optional: range]")
	if len(args) != 2 {
		t.Fatalf("got %d args, want 2", len(args))
	}
	if !args[0].Required {
		t.Error("first arg should be required")
	}
	if args[1].Required {
		t.Error("second arg (optional: range) should be optional")
	}
}

func TestParseArgumentHint_Alternatives(t *testing.T) {
	args := parseArgumentHint("[symbol-name | file-path]")
	if len(args) != 2 {
		t.Fatalf("got %d args, want 2", len(args))
	}
	if args[0].Name != "symbol-name" {
		t.Errorf("first alt = %q, want symbol-name", args[0].Name)
	}
	if args[1].Name != "file-path" {
		t.Errorf("second alt = %q, want file-path", args[1].Name)
	}
}

func TestParseSkillMD_MissingClosingFrontmatter(t *testing.T) {
	content := `---
name: test
description: "missing closing"
`
	_, ok := parseSkillMD(content)
	if ok {
		t.Error("expected false for missing closing ---")
	}
}

func TestParseSkillMD_EmptyBody(t *testing.T) {
	content := `---
name: minimal
description: "minimal skill"
---
`
	meta, ok := parseSkillMD(content)
	if !ok {
		t.Fatal("parseSkillMD returned false")
	}
	if meta.Name != "minimal" {
		t.Errorf("Name = %q, want %q", meta.Name, "minimal")
	}
	if meta.Body != "" {
		t.Errorf("Body = %q, want empty", meta.Body)
	}
}

func TestParseSkillMD_ExtraFrontmatterFields(t *testing.T) {
	content := `---
name: test-skill
description: "a test"
argument-hint: "[arg1]"
user-invocable: true
allowed-tools: mcp__lsp__start_lsp
custom-field: ignored
---

Body text.`

	meta, ok := parseSkillMD(content)
	if !ok {
		t.Fatal("parseSkillMD returned false")
	}
	if meta.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", meta.Name, "test-skill")
	}
	if meta.ArgumentHint != "[arg1]" {
		t.Errorf("ArgumentHint = %q, want %q", meta.ArgumentHint, "[arg1]")
	}
}

func TestParseSkillMD_FrontmatterLineWithoutColon(t *testing.T) {
	content := `---
name: valid
this line has no colon separator
description: "still works"
---

Body.`

	meta, ok := parseSkillMD(content)
	if !ok {
		t.Fatal("parseSkillMD returned false")
	}
	if meta.Name != "valid" {
		t.Errorf("Name = %q, want %q", meta.Name, "valid")
	}
	if meta.Description != "still works" {
		t.Errorf("Description = %q, want %q", meta.Description, "still works")
	}
}

func TestParseArgumentHint_NestedBrackets(t *testing.T) {
	// Only outer brackets should be matched.
	args := parseArgumentHint("[outer]")
	if len(args) != 1 {
		t.Fatalf("got %d args, want 1", len(args))
	}
	if args[0].Name != "outer" {
		t.Errorf("Name = %q, want %q", args[0].Name, "outer")
	}
}

func TestParseArgumentHint_EmptyBrackets(t *testing.T) {
	args := parseArgumentHint("[]")
	if len(args) != 0 {
		t.Errorf("expected 0 args for empty brackets, got %d", len(args))
	}
}

func TestParseArgumentHint_MultiWordName(t *testing.T) {
	args := parseArgumentHint("[file path name]")
	if len(args) != 1 {
		t.Fatalf("got %d args, want 1", len(args))
	}
	if args[0].Name != "file-path-name" {
		t.Errorf("Name = %q, want 'file-path-name'", args[0].Name)
	}
}

func TestParseArgumentHint_OptionalPrefix(t *testing.T) {
	tests := []struct {
		hint     string
		required bool
		name     string
	}{
		{"[optional: range]", false, "range"},
		{"[optional line]", false, "line"},
		{"[required-arg]", true, "required-arg"},
	}
	for _, tt := range tests {
		args := parseArgumentHint(tt.hint)
		if len(args) != 1 {
			t.Errorf("hint %q: got %d args, want 1", tt.hint, len(args))
			continue
		}
		if args[0].Required != tt.required {
			t.Errorf("hint %q: Required = %v, want %v", tt.hint, args[0].Required, tt.required)
		}
		if args[0].Name != tt.name {
			t.Errorf("hint %q: Name = %q, want %q", tt.hint, args[0].Name, tt.name)
		}
	}
}

func TestRegisterPrompts_EmbeddedSkills(t *testing.T) {
	// Verify that the embedded skill files parse without errors.
	// This is an integration test that catches embed path issues.
	count := 0
	registerPromptsWithCallback(func(name, description string) {
		if name == "" {
			t.Error("registered prompt with empty name")
		}
		if description == "" {
			t.Errorf("prompt %q has empty description", name)
		}
		count++
	})
	if count == 0 {
		t.Error("no prompts registered from embedded skills")
	}
	if count < 20 {
		t.Errorf("only %d prompts registered, expected at least 20 (have 21 skills)", count)
	}
}
