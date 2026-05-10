package resources

import (
	"strings"
	"testing"
)

// --- parseResourceQueryParams: additional edge cases ---

func TestParseResourceQueryParams_LargeLineColumn(t *testing.T) {
	uri := "lsp-hover:///project/main.go?line=9999&column=500&language_id=go"
	_, pos, _, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1-indexed to 0-indexed
	if pos.Line != 9998 {
		t.Errorf("pos.Line = %d, want 9998", pos.Line)
	}
	if pos.Character != 499 {
		t.Errorf("pos.Character = %d, want 499", pos.Character)
	}
}

func TestParseResourceQueryParams_SpecialCharsInPath(t *testing.T) {
	uri := "lsp-hover:///project/my%20file.go?line=1&column=1&language_id=go"
	filePath, _, _, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filePath != "/project/my file.go" {
		t.Errorf("filePath = %q, want %q", filePath, "/project/my file.go")
	}
}

func TestParseResourceQueryParams_CompletionsScheme(t *testing.T) {
	// Verify it works equally well with the completions scheme
	uri := "lsp-completions:///src/app.ts?line=5&column=3&language_id=typescript"
	filePath, pos, langID, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filePath != "/src/app.ts" {
		t.Errorf("filePath = %q, want %q", filePath, "/src/app.ts")
	}
	if pos.Line != 4 || pos.Character != 2 {
		t.Errorf("pos = %d:%d, want 4:2", pos.Line, pos.Character)
	}
	if langID != "typescript" {
		t.Errorf("langID = %q, want %q", langID, "typescript")
	}
}

func TestParseResourceQueryParams_ExtraQueryParams(t *testing.T) {
	// Extra params should be ignored
	uri := "lsp-hover:///f.go?line=1&column=1&language_id=go&extra=ignored"
	_, _, _, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Error("extra query params should not cause error")
	}
}

func TestParseResourceQueryParams_ZeroLineColumn(t *testing.T) {
	// line=0 and column=0 are valid input (though unusual)
	uri := "lsp-hover:///f.go?line=0&column=0&language_id=go"
	_, pos, _, err := parseResourceQueryParams(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0 - 1 = -1 (the function just does the arithmetic)
	if pos.Line != -1 || pos.Character != -1 {
		t.Errorf("pos = %d:%d, want -1:-1", pos.Line, pos.Character)
	}
}

// --- ResourceTemplates: structural validation ---

func TestResourceTemplates_URITemplatesHaveScheme(t *testing.T) {
	templates := ResourceTemplates()
	for _, tmpl := range templates {
		if !strings.Contains(tmpl.URITemplate, "://") {
			t.Errorf("template %q URI has no scheme: %q", tmpl.Name, tmpl.URITemplate)
		}
	}
}

func TestResourceTemplates_NoDuplicateNames(t *testing.T) {
	templates := ResourceTemplates()
	seen := make(map[string]bool)
	for _, tmpl := range templates {
		if seen[tmpl.Name] {
			t.Errorf("duplicate template name: %q", tmpl.Name)
		}
		seen[tmpl.Name] = true
	}
}

// --- ResourceResult: structure ---

func TestResourceResult_Fields(t *testing.T) {
	r := ResourceResult{
		URI:      "lsp-hover:///foo.go?line=1&column=1&language_id=go",
		MIMEType: "text/plain",
		Text:     "func Foo() error",
	}
	if r.URI == "" || r.MIMEType == "" || r.Text == "" {
		t.Error("expected all fields to be non-empty")
	}
}

func TestResourceResult_JSONMimeType(t *testing.T) {
	r := ResourceResult{
		URI:      "lsp-diagnostics:///foo.go",
		MIMEType: "application/json",
		Text:     `{"file:///foo.go":[]}`,
	}
	if r.MIMEType != "application/json" {
		t.Errorf("expected application/json, got %q", r.MIMEType)
	}
}
