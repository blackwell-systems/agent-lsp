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

// serverKey returns the identity a ServerEntry is merged on: its LanguageID,
// or the first extension when LanguageID is empty. An entry with neither yields
// "", which never matches for override purposes (such entries always append).
func serverKey(e ServerEntry) string {
	if e.LanguageID != "" {
		return e.LanguageID
	}
	if len(e.Extensions) > 0 {
		return e.Extensions[0]
	}
	return ""
}

// MergeConfigs overlays override onto base, keyed by language identifier (see
// serverKey). An override entry whose key matches a base entry replaces that
// entry in place; an override entry with a new key is appended; base entries
// with no matching override are kept. Base order is preserved, with appended
// override entries following in their original order. This backs --merge-config:
// auto-detected servers form the base, and the user's config file overrides or
// extends them. Either argument may be nil.
func MergeConfigs(base, override *Config) *Config {
	merged := &Config{}
	keyToIndex := make(map[string]int)

	if base != nil {
		for _, e := range base.Servers {
			if k := serverKey(e); k != "" {
				keyToIndex[k] = len(merged.Servers)
			}
			merged.Servers = append(merged.Servers, e)
		}
	}

	if override != nil {
		for _, e := range override.Servers {
			k := serverKey(e)
			if idx, ok := keyToIndex[k]; ok && k != "" {
				merged.Servers[idx] = e // override in place
				continue
			}
			if k != "" {
				keyToIndex[k] = len(merged.Servers)
			}
			merged.Servers = append(merged.Servers, e)
		}
	}

	return merged
}
