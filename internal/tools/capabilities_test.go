package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHandleGetServerCapabilities_NilClient(t *testing.T) {
	r, err := HandleGetServerCapabilities(context.Background(), newNilClient(), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client")
	}
}

// TestHasCapabilityInMap verifies the capability map lookup logic.
func TestHasCapabilityInMap(t *testing.T) {
	caps := map[string]interface{}{
		"hoverProvider":      true,
		"completionProvider": map[string]interface{}{"triggerCharacters": []string{"."}},
		"renameProvider":     false,
		"absentKey":          nil,
	}

	cases := []struct {
		key  string
		want bool
	}{
		{"hoverProvider", true},                // bool true
		{"completionProvider", true},           // non-nil object
		{"renameProvider", false},              // bool false
		{"absentKey", false},                   // nil value (absent)
		{"notPresent", false},                  // key missing entirely
	}

	for _, tc := range cases {
		got := hasCapabilityInMap(caps, tc.key)
		if got != tc.want {
			t.Errorf("hasCapabilityInMap(%q): want %v, got %v", tc.key, tc.want, got)
		}
	}
}

// TestAlwaysAvailableToolsNotInCapabilityMap verifies no tool appears in both
// alwaysAvailableTools and capabilityToolMap (would cause duplicates in output).
func TestAlwaysAvailableToolsNotInCapabilityMap(t *testing.T) {
	always := make(map[string]bool, len(alwaysAvailableTools))
	for _, tool := range alwaysAvailableTools {
		always[tool] = true
	}
	for _, entry := range capabilityToolMap {
		for _, tool := range entry.tools {
			if always[tool] {
				t.Errorf("tool %q appears in both alwaysAvailableTools and capabilityToolMap", tool)
			}
		}
	}
}

// TestCapabilityToolMapNoDuplicates verifies no tool name appears twice in capabilityToolMap.
func TestCapabilityToolMapNoDuplicates(t *testing.T) {
	seen := make(map[string]string)
	for _, entry := range capabilityToolMap {
		for _, tool := range entry.tools {
			if prev, ok := seen[tool]; ok {
				t.Errorf("tool %q appears under both %q and %q", tool, prev, entry.capability)
			}
			seen[tool] = entry.capability
		}
	}
}

// TestServerCapabilitiesResultShape verifies the JSON output contains expected
// top-level fields when capabilities are empty (server returned no capabilities).
func TestServerCapabilitiesResultShape(t *testing.T) {
	result := ServerCapabilitiesResult{
		ServerName:       "gopls",
		ServerVersion:    "v0.15.0",
		SupportedTools:   []string{"start_lsp"},
		UnsupportedTools: []string{"type_hierarchy"},
		Capabilities:     map[string]interface{}{},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"server_name", "server_version", "supported_tools", "unsupported_tools", "capabilities"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing field %q in JSON output", field)
		}
	}
}
