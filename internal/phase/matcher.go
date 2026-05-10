package phase

import "strings"

// MatchToolPattern checks whether a tool name matches a glob pattern.
// Supports trailing wildcard (*) only, which matches the common patterns
// in skill YAML (e.g., "mcp__lsp__simulate_*" matches "mcp__lsp__simulate_edit").
//
// Patterns without wildcards require an exact match.
func MatchToolPattern(pattern, toolName string) bool {
	if pattern == toolName {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(toolName, prefix)
	}
	return false
}

// MatchesAny returns true if toolName matches any pattern in the list.
func MatchesAny(patterns []string, toolName string) bool {
	for _, p := range patterns {
		if MatchToolPattern(p, toolName) {
			return true
		}
	}
	return false
}
