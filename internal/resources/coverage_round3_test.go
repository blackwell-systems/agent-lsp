package resources

import (
	"testing"
)

// --- parseResourceQueryParams ---

func TestParseResourceQueryParams_Valid(t *testing.T) {
	uri := "lsp-hover:///project/main.go?line=10&column=5&language_id=go"
	filePath, pos, langID, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filePath != "/project/main.go" {
		t.Errorf("filePath=%q, want /project/main.go", filePath)
	}
	// 1-indexed to 0-indexed conversion
	if pos.Line != 9 || pos.Character != 4 {
		t.Errorf("pos: got %d:%d, want 9:4", pos.Line, pos.Character)
	}
	if langID != "go" {
		t.Errorf("languageID=%q, want go", langID)
	}
}

func TestParseResourceQueryParams_MissingLine(t *testing.T) {
	uri := "lsp-hover:///project/main.go?column=5&language_id=go"
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for missing line param")
	}
}

func TestParseResourceQueryParams_MissingColumn(t *testing.T) {
	uri := "lsp-hover:///project/main.go?line=10&language_id=go"
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for missing column param")
	}
}

func TestParseResourceQueryParams_MissingLanguageID(t *testing.T) {
	uri := "lsp-hover:///project/main.go?line=10&column=5"
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for missing language_id param")
	}
}

func TestParseResourceQueryParams_InvalidLine(t *testing.T) {
	uri := "lsp-hover:///project/main.go?line=abc&column=5&language_id=go"
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for non-numeric line")
	}
}

func TestParseResourceQueryParams_InvalidColumn(t *testing.T) {
	uri := "lsp-hover:///project/main.go?line=10&column=xyz&language_id=go"
	_, _, _, err := parseResourceQueryParams(uri)
	if err == nil {
		t.Error("expected error for non-numeric column")
	}
}

func TestParseResourceQueryParams_InvalidURI(t *testing.T) {
	_, _, _, err := parseResourceQueryParams("://invalid\x00uri")
	if err == nil {
		t.Error("expected error for invalid URI")
	}
}

// --- ResourceTemplates ---

func TestResourceTemplates_Count(t *testing.T) {
	templates := ResourceTemplates()
	if len(templates) != 3 {
		t.Errorf("expected 3 templates, got %d", len(templates))
	}
}

func TestResourceTemplates_Names(t *testing.T) {
	templates := ResourceTemplates()
	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
	}
	expected := []string{"lsp-diagnostics", "lsp-hover", "lsp-completions"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected template %q not found", name)
		}
	}
}
