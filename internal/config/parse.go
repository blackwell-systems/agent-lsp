package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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

	// HTTP transport fields
	HTTPMode       bool
	HTTPPort       int
	HTTPToken      string
	HTTPListenAddr string // bind address; defaults to 127.0.0.1
	HTTPNoAuth     bool   // explicit opt-in to unauthenticated HTTP mode
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
// Mode 1 — legacy: agent-lsp <language-id> <binary> [args...]
//
//	Detected when len>=2 AND args[0] contains no ":"
//
// Mode 2 — multi-arg: agent-lsp go:gopls typescript:tsserver,--stdio
//
//	Detected when args[0] contains ":"
//
// Mode 3 — config file: agent-lsp --config /path/to/lsp-mcp.json
func ParseArgs(args []string) (ParseResult, error) {
	// Pre-process HTTP flags: --http, --port <N>, --token <S>.
	// These are consumed before the existing positional argument parsing.
	var httpMode bool
	httpPort := 8080
	var httpToken string
	httpListenAddr := "127.0.0.1"
	var httpNoAuth bool
	remainder := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--http":
			httpMode = true
		case "--port":
			if i+1 >= len(args) {
				return ParseResult{}, fmt.Errorf("--port requires a value")
			}
			i++
			p, err := strconv.Atoi(args[i])
			if err != nil {
				return ParseResult{}, fmt.Errorf("--port value %q is not a valid integer", args[i])
			}
			if p < 1 || p > 65535 {
				return ParseResult{}, fmt.Errorf("--port value %d is out of range (1–65535)", p)
			}
			httpPort = p
		case "--listen-addr":
			if i+1 >= len(args) {
				return ParseResult{}, fmt.Errorf("--listen-addr requires a value")
			}
			i++
			httpListenAddr = args[i]
		case "--no-auth":
			httpNoAuth = true
		case "--token":
			if i+1 >= len(args) {
				return ParseResult{}, fmt.Errorf("--token requires a value")
			}
			i++
			httpToken = args[i]
		default:
			remainder = append(remainder, args[i])
		}
	}
	args = remainder

	// Prefer AGENT_LSP_TOKEN env var over --token flag. The env var keeps the
	// credential out of the process list (ps aux, /proc/<pid>/cmdline).
	if ev := os.Getenv("AGENT_LSP_TOKEN"); ev != "" {
		httpToken = ev
	}

	httpResult := func(r ParseResult) ParseResult {
		r.HTTPMode = httpMode
		r.HTTPPort = httpPort
		r.HTTPToken = httpToken
		r.HTTPListenAddr = httpListenAddr
		r.HTTPNoAuth = httpNoAuth
		return r
	}

	if len(args) == 0 || (len(args) == 1 && args[0] == "--auto") {
		cfg, err := AutodetectServers()
		if err != nil {
			return ParseResult{}, fmt.Errorf("auto-detect: %w", err)
		}
		return httpResult(ParseResult{Config: cfg}), nil
	}

	// Mode 3: config file
	if args[0] == "--config" {
		if len(args) < 2 {
			return ParseResult{}, fmt.Errorf("--config requires a file path argument\n" +
				"usage: agent-lsp <language-id> <binary> [args...]\n" +
				"       agent-lsp go:gopls typescript:typescript-language-server,--stdio\n" +
				"       agent-lsp --config /path/to/lsp-mcp.json\n" +
				"       agent-lsp (auto-detect mode)")
		}
		cfg, err := LoadConfig(args[1])
		if err != nil {
			return ParseResult{}, fmt.Errorf("load config %s: %w", args[1], err)
		}
		return httpResult(ParseResult{Config: cfg}), nil
	}

	// Mode 1: legacy (len>=2 AND args[0] contains no ":")
	if len(args) >= 2 && !strings.Contains(args[0], ":") {
		// Validate that the server binary exists.
		if _, err := os.Stat(args[1]); err != nil {
			return ParseResult{}, fmt.Errorf("server binary %q not found: %w", args[1], err)
		}
		return httpResult(ParseResult{
			IsSingleServer: true,
			LanguageID:     args[0],
			ServerPath:     args[1],
			ServerArgs:     args[2:],
		}), nil
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

	return httpResult(ParseResult{Config: &Config{Servers: entries}}), nil
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
