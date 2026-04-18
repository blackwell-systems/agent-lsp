package main

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/config"
)

func TestDoctorCapabilityMap_NotEmpty(t *testing.T) {
	if len(doctorCapabilityMap) == 0 {
		t.Fatal("doctorCapabilityMap must not be empty")
	}
}

func TestPrintDoctorReport_NoResults(t *testing.T) {
	// Smoke test: printDoctorReport must not panic with empty slice.
	printDoctorReport(nil)
}

func TestPrintDoctorReport_FailedServer(t *testing.T) {
	results := []DoctorResult{
		{
			LanguageID: "go",
			Binary:     "gopls",
			Status:     "failed",
			Error:      "executable not found",
		},
	}
	// Must not panic.
	printDoctorReport(results)
}

func TestPrintDoctorReport_OkServer(t *testing.T) {
	results := []DoctorResult{
		{
			LanguageID:    "go",
			Binary:        "gopls",
			Status:        "ok",
			ServerName:    "gopls",
			ServerVersion: "0.17.1",
			Capabilities:  []string{"hoverProvider", "definitionProvider"},
			Tools:         []string{"get_info_on_location", "go_to_definition"},
		},
	}
	// Must not panic.
	printDoctorReport(results)
}

func TestRunDoctor_NilAutodetectGraceful(t *testing.T) {
	// When no servers are detectable, runDoctor must print a helpful message
	// and not panic. We cannot call runDoctor() directly (it calls os.Exit),
	// so test buildDoctorEntries helper instead.
	entries := buildDoctorEntries([]config.ServerEntry{})
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty input, got %d", len(entries))
	}
}

func TestDoctorHasCapability_TrueValues(t *testing.T) {
	caps := map[string]interface{}{
		"hoverProvider":      true,
		"completionProvider": map[string]interface{}{"triggerCharacters": []string{"."}},
		"missingProvider":    nil,
	}

	if !doctorHasCapability(caps, "hoverProvider") {
		t.Error("expected hoverProvider to be present")
	}
	if !doctorHasCapability(caps, "completionProvider") {
		t.Error("expected completionProvider (object) to be present")
	}
	if doctorHasCapability(caps, "missingProvider") {
		t.Error("expected nil-valued missingProvider to be absent")
	}
	if doctorHasCapability(caps, "nonexistent") {
		t.Error("expected nonexistent key to be absent")
	}
}

func TestDoctorHasCapability_FalseValue(t *testing.T) {
	caps := map[string]interface{}{
		"referencesProvider": false,
	}
	if doctorHasCapability(caps, "referencesProvider") {
		t.Error("expected false bool to be treated as absent")
	}
}

func TestBuildDoctorEntries_Identity(t *testing.T) {
	input := []config.ServerEntry{
		{LanguageID: "go", Command: []string{"gopls"}},
		{LanguageID: "typescript", Command: []string{"typescript-language-server", "--stdio"}},
	}
	output := buildDoctorEntries(input)
	if len(output) != len(input) {
		t.Fatalf("expected %d entries, got %d", len(input), len(output))
	}
	for i := range input {
		if output[i].LanguageID != input[i].LanguageID {
			t.Errorf("entry %d: expected LanguageID %q, got %q", i, input[i].LanguageID, output[i].LanguageID)
		}
	}
}

func TestJoinStrings(t *testing.T) {
	cases := []struct {
		input    []string
		expected string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b", "c"}, "a, b, c"},
	}
	for _, c := range cases {
		got := joinStrings(c.input)
		if got != c.expected {
			t.Errorf("joinStrings(%v) = %q, want %q", c.input, got, c.expected)
		}
	}
}
