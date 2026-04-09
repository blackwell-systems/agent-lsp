package config

import (
	"fmt"
	"os/exec"
	"sort"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
)

// ServerSpec describes a language server's binary name and default args.
type ServerSpec struct {
	Binary     string   // e.g. "gopls", "typescript-language-server"
	Args       []string // e.g. ["--stdio"] or nil for no args
	Extensions []string // e.g. ["go"] or ["ts", "tsx"]
}

// knownServers maps language ID to server specification.
// Ordered by popularity (Go, TypeScript, Python first).
var knownServers = map[string]ServerSpec{
	"go":         {Binary: "gopls", Args: nil, Extensions: []string{"go"}},
	"typescript": {Binary: "typescript-language-server", Args: []string{"--stdio"}, Extensions: []string{"ts", "tsx"}},
	"javascript": {Binary: "typescript-language-server", Args: []string{"--stdio"}, Extensions: []string{"js", "jsx", "mjs", "cjs"}},
	"python":     {Binary: "pyright-langserver", Args: []string{"--stdio"}, Extensions: []string{"py", "pyw"}},
	"rust":       {Binary: "rust-analyzer", Args: nil, Extensions: []string{"rs"}},
	"c":          {Binary: "clangd", Args: nil, Extensions: []string{"c", "h"}},
	"cpp":        {Binary: "clangd", Args: nil, Extensions: []string{"cpp", "cc", "cxx", "c++", "hpp", "hxx"}},
	"ruby":       {Binary: "solargraph", Args: []string{"stdio"}, Extensions: []string{"rb", "rake", "gemspec"}},
	"yaml":       {Binary: "yaml-language-server", Args: []string{"--stdio"}, Extensions: []string{"yaml", "yml"}},
	"json":       {Binary: "vscode-json-language-server", Args: []string{"--stdio"}, Extensions: []string{"json"}},
	"dockerfile": {Binary: "docker-langserver", Args: []string{"--stdio"}, Extensions: []string{"dockerfile"}},
	"csharp":     {Binary: "csharp-ls", Args: nil, Extensions: []string{"cs", "csx"}},
	"java":       {Binary: "jdtls", Args: nil, Extensions: []string{"java"}},
	"kotlin":     {Binary: "kotlin-language-server", Args: nil, Extensions: []string{"kt", "kts"}},
	"php":        {Binary: "intelephense", Args: []string{"--stdio"}, Extensions: []string{"php", "phtml"}},
}

// AutodetectServers scans PATH for known language server binaries
// and returns a Config struct with all discovered servers.
// Logs INFO for each found server and WARN for each missing server.
func AutodetectServers() (*Config, error) {
	var servers []ServerEntry

	// Extract and sort keys for deterministic iteration
	languages := make([]string, 0, len(knownServers))
	for lang := range knownServers {
		languages = append(languages, lang)
	}
	sort.Strings(languages)

	// Scan PATH for each known language server
	for _, lang := range languages {
		spec := knownServers[lang]
		path, err := exec.LookPath(spec.Binary)
		if err != nil {
			// Server not found in PATH
			logging.Log(logging.LevelWarning, fmt.Sprintf("language server not found: %s (for %s)", spec.Binary, lang))
			continue
		}

		// Build command array: [binary, arg1, arg2, ...]
		command := []string{path}
		if spec.Args != nil {
			command = append(command, spec.Args...)
		}

		// Create ServerEntry
		entry := ServerEntry{
			Extensions: spec.Extensions,
			Command:    command,
			LanguageID: lang,
		}
		servers = append(servers, entry)

		logging.Log(logging.LevelInfo, fmt.Sprintf("auto-detected %s: %s", lang, spec.Binary))
	}

	// Return error if no servers were found
	if len(servers) == 0 {
		return nil, fmt.Errorf("no language servers found in PATH")
	}

	return &Config{Servers: servers}, nil
}
