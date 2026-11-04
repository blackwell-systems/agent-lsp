package main

import (
	"reflect"
	"testing"
)

// Each tool's direction must accept its own values: a shared enum for every
// field named "direction" made the type_hierarchy schema reject supertypes
// and subtypes, the only values its handler accepts.
func TestSchemaEnums_PerTool(t *testing.T) {
	cases := []struct {
		name  string
		enum  []any
		field string
	}{
		{"type_hierarchy", generateFixedSchema[TypeHierarchyArgs]().Properties["direction"].Enum, "direction"},
		{"call_hierarchy", generateFixedSchema[CallHierarchyArgs]().Properties["direction"].Enum, "direction"},
		{"set_log_level", generateFixedSchema[SetLogLevelArgs]().Properties["level"].Enum, "level"},
		{"find_symbol", generateFixedSchema[GetWorkspaceSymbolsArgs]().Properties["detail_level"].Enum, "detail_level"},
	}
	want := map[string][]any{
		"type_hierarchy": {"supertypes", "subtypes", "both"},
		"call_hierarchy": {"incoming", "outgoing", "both"},
		"set_log_level":  {"emergency", "alert", "critical", "error", "warning", "notice", "info", "debug"},
		"find_symbol":    {"basic", "hover"},
	}
	for _, c := range cases {
		if !reflect.DeepEqual(c.enum, want[c.name]) {
			t.Errorf("%s %s enum = %v, want %v", c.name, c.field, c.enum, want[c.name])
		}
	}
}
