package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ParseResult holds the outcome of argument parsing.
type ParseResult struct {
	// Single-server fields (backward compat)
	IsSingleServer bool
	LanguageID     string
	ServerPath     string
	ServerArgs     []string

	// Multi-server field
	Config *Config
}

// extensionMap maps language IDs to their file extensions.
// Based on mcp-lsp-bridge convention.
var extensionMap = map[string][]string{
	"go":         {"go"},
	"typescript": {"ts", "tsx"},
	"javascript": {"js", "jsx", "mjs", "cjs"},
	"python":     {"py", "pyw"},
	"rust":       {"rs"},
	"java":       {"java"},
	"cpp":        {"cpp", "cc", "cxx", "c++", "hpp", "hxx"},
	"c":          {"c", "h"},
	"csharp":     {"cs", "csx"},
	"ruby":       {"rb", "rake", "gemspec"},
	"php":        {"php", "phtml"},
	"swift":      {"swift"},
	"kotlin":     {"kt", "kts"},
	"lua":        {"lua"},
	"haskell":    {"hs", "lhs"},
	"ocaml":      {"ml", "mli"},
	"zig":        {"zig"},
	"scala":      {"scala", "sc", "sbt"},
}

// ParseArgs parses command-line arguments into a ParseResult.
// Mode 1 — legacy: lsp-mcp-go <language-id> <binary> [args...]
//
//	Detected when len>=2 AND args[0] contains no ":"
//
// Mode 2 — multi-arg: lsp-mcp-go go:gopls typescript:tsserver,--stdio
//
//	Detected when args[0] contains ":"
//
// Mode 3 — config file: lsp-mcp-go --config /path/to/lsp-mcp.json
func ParseArgs(args []string) (ParseResult, error) {
	if len(args) == 0 {
		return ParseResult{}, fmt.Errorf("usage: lsp-mcp-go <language-id> <binary> [args...]\n" +
			"       lsp-mcp-go go:gopls typescript:typescript-language-server,--stdio\n" +
			"       lsp-mcp-go --config /path/to/lsp-mcp.json")
	}

	// Mode 3: config file
	if args[0] == "--config" {
		if len(args) < 2 {
			return ParseResult{}, fmt.Errorf("--config requires a file path argument")
		}
		cfg, err := LoadConfig(args[1])
		if err != nil {
			return ParseResult{}, fmt.Errorf("load config %s: %w", args[1], err)
		}
		return ParseResult{Config: cfg}, nil
	}

	// Mode 1: legacy (len>=2 AND args[0] contains no ":")
	if len(args) >= 2 && !strings.Contains(args[0], ":") {
		// Validate that the server binary exists.
		if _, err := os.Stat(args[1]); err != nil {
			return ParseResult{}, fmt.Errorf("server binary %q not found: %w", args[1], err)
		}
		return ParseResult{
			IsSingleServer: true,
			LanguageID:     args[0],
			ServerPath:     args[1],
			ServerArgs:     args[2:],
		}, nil
	}

	// Mode 2: multi-arg — each arg is "language-id:binary,arg1,arg2..."
	entries := make([]ServerEntry, 0, len(args))
	for _, arg := range args {
		colonIdx := strings.Index(arg, ":")
		if colonIdx < 0 {
			return ParseResult{}, fmt.Errorf("invalid argument %q: expected format language-id:binary[,args...]", arg)
		}
		languageID := arg[:colonIdx]
		rest := arg[colonIdx+1:]

		parts := strings.Split(rest, ",")
		if len(parts) == 0 || parts[0] == "" {
			return ParseResult{}, fmt.Errorf("invalid argument %q: binary path is empty", arg)
		}

		exts, ok := extensionMap[languageID]
		if !ok {
			// default: use languageID itself as the extension
			exts = []string{languageID}
		}

		entry := ServerEntry{
			Extensions: exts,
			Command:    parts,
			LanguageID: languageID,
		}
		entries = append(entries, entry)
	}

	return ParseResult{Config: &Config{Servers: entries}}, nil
}

// LoadConfig reads and parses a JSON config file.
// Format: {"servers": [{"extensions":["go"],"command":["gopls"]}]}
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}
