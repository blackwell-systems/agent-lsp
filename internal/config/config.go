// Package config holds types and parsing for multi-server configuration.
package config

// ServerEntry describes one language server to launch.
// The format matches cclsp.json: extensions[] + command[].
type ServerEntry struct {
	// Extensions is the list of file extensions this server handles (without dot).
	// e.g. ["go"] or ["ts", "tsx", "js", "jsx"]
	Extensions []string `json:"extensions"`

	// Command is [binary, arg1, arg2, ...].
	// e.g. ["gopls"] or ["typescript-language-server", "--stdio"]
	Command []string `json:"command"`

	// LanguageID is used when opening documents (e.g. "go", "typescript").
	// If empty, inferred from Extensions[0] via the built-in extension map.
	LanguageID string `json:"language_id,omitempty"`
}

// Config is the top-level multi-server configuration.
// File format: {"servers": [{...}, ...]}
type Config struct {
	Servers []ServerEntry `json:"servers"`
}
