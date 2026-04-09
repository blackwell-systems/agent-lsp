package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// lspServerDef describes a known language server.
type lspServerDef struct {
	Language    string   // LSP language ID
	Binary      string   // executable name on PATH
	Args        []string // extra args required to start the server
	DisplayName string   // human-readable server name
}

// knownServers is the canonical mapping from language to LSP server.
// Ordered to prefer the most-used server when multiple serve a language.
var knownServers = []lspServerDef{
	{Language: "go", Binary: "gopls", Args: nil, DisplayName: "gopls"},
	{Language: "typescript", Binary: "typescript-language-server", Args: []string{"--stdio"}, DisplayName: "typescript-language-server"},
	{Language: "javascript", Binary: "typescript-language-server", Args: []string{"--stdio"}, DisplayName: "typescript-language-server"},
	{Language: "python", Binary: "pyright-langserver", Args: []string{"--stdio"}, DisplayName: "pyright"},
	{Language: "rust", Binary: "rust-analyzer", Args: nil, DisplayName: "rust-analyzer"},
	{Language: "java", Binary: "jdtls", Args: nil, DisplayName: "jdtls"},
	{Language: "c", Binary: "clangd", Args: nil, DisplayName: "clangd"},
	{Language: "cpp", Binary: "clangd", Args: nil, DisplayName: "clangd"},
	{Language: "php", Binary: "intelephense", Args: []string{"--stdio"}, DisplayName: "intelephense"},
	{Language: "ruby", Binary: "solargraph", Args: []string{"stdio"}, DisplayName: "solargraph"},
	{Language: "yaml", Binary: "yaml-language-server", Args: []string{"--stdio"}, DisplayName: "yaml-language-server"},
	{Language: "json", Binary: "vscode-json-language-server", Args: []string{"--stdio"}, DisplayName: "vscode-json-language-server"},
	{Language: "dockerfile", Binary: "docker-langserver", Args: []string{"--stdio"}, DisplayName: "docker-langserver"},
	{Language: "csharp", Binary: "csharp-ls", Args: nil, DisplayName: "csharp-ls"},
	{Language: "kotlin", Binary: "kotlin-language-server", Args: nil, DisplayName: "kotlin-language-server"},
	{Language: "lua", Binary: "lua-language-server", Args: nil, DisplayName: "lua-language-server"},
	{Language: "swift", Binary: "sourcekit-lsp", Args: nil, DisplayName: "sourcekit-lsp"},
	{Language: "zig", Binary: "zls", Args: nil, DisplayName: "zls"},
	{Language: "css", Binary: "vscode-css-language-server", Args: []string{"--stdio"}, DisplayName: "vscode-css-language-server"},
	{Language: "html", Binary: "vscode-html-language-server", Args: []string{"--stdio"}, DisplayName: "vscode-html-language-server"},
	{Language: "terraform", Binary: "terraform-ls", Args: []string{"serve"}, DisplayName: "terraform-ls"},
	{Language: "scala", Binary: "metals", Args: nil, DisplayName: "metals"},
}

// rootMarkers maps a project root file to the language it signals.
// Multiple markers can signal the same language (scored additively).
var rootMarkers = map[string]string{
	"go.mod":            "go",
	"go.sum":            "go",
	"package.json":      "typescript",
	"tsconfig.json":     "typescript",
	"jsconfig.json":     "javascript",
	"Cargo.toml":        "rust",
	"Cargo.lock":        "rust",
	"pyproject.toml":    "python",
	"requirements.txt":  "python",
	"setup.py":          "python",
	"Pipfile":           "python",
	"pom.xml":           "java",
	"build.gradle":      "java",
	"build.gradle.kts":  "kotlin",
	"Gemfile":           "ruby",
	"composer.json":     "php",
	"Dockerfile":          "dockerfile",
	"dockerfile":          "dockerfile",
	"settings.gradle.kts": "kotlin",
	"build.zig":           "zig",
	"Package.swift":       "swift",
	"build.sbt":           "scala",
}

// extLanguages maps file extensions to language IDs.
var extLanguages = map[string]string{
	".go":         "go",
	".ts":         "typescript",
	".tsx":        "typescript",
	".mts":        "typescript",
	".cts":        "typescript",
	".js":         "javascript",
	".jsx":        "javascript",
	".mjs":        "javascript",
	".cjs":        "javascript",
	".py":         "python",
	".pyi":        "python",
	".rs":         "rust",
	".java":       "java",
	".c":          "c",
	".h":          "c",
	".cpp":        "cpp",
	".cc":         "cpp",
	".cxx":        "cpp",
	".hpp":        "cpp",
	".hxx":        "cpp",
	".php":        "php",
	".phtml":      "php",
	".rb":         "ruby",
	".yaml":       "yaml",
	".yml":        "yaml",
	".json":       "json",
	".jsonc":      "json",
	".cs":         "csharp",
	".kt":         "kotlin",
	".kts":        "kotlin",
	".lua":        "lua",
	".swift":      "swift",
	".zig":        "zig",
	".css":        "css",
	".scss":       "css",
	".less":       "css",
	".html":       "html",
	".htm":        "html",
	".tf":         "terraform",
	".tfvars":     "terraform",
	".sc":         "scala",
	".scala":      "scala",
}

// skipDirs are directory names that should never be walked.
var skipDirs = map[string]bool{
	"node_modules": true,
	"target":       true,
	"build":        true,
	"dist":         true,
	"vendor":       true,
	".git":         true,
	".svn":         true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	".tox":         true,
}

// DetectedServer is a server found on PATH for a detected workspace language.
type DetectedServer struct {
	Language    string `json:"language"`
	Server      string `json:"server"`
	Path        string `json:"path"`
	ConfigEntry string `json:"config_entry"` // ready-to-use arg for agent-lsp
}

// DetectResult is the response from HandleDetectLspServers.
type DetectResult struct {
	WorkspaceDir       string           `json:"workspace_dir"`
	WorkspaceLanguages []string         `json:"workspace_languages"`
	InstalledServers   []DetectedServer `json:"installed_servers"`
	SuggestedConfig    []string         `json:"suggested_config"`
	NotInstalled       []string         `json:"not_installed,omitempty"`
}

// HandleDetectLspServers scans the workspace for source languages and checks
// PATH for the corresponding LSP server binaries. Returns the detected languages,
// installed servers with their paths, and a suggested_config array ready to paste
// into the agent-lsp MCP server args.
//
// Does not require start_lsp to have been called — the client parameter is unused.
func HandleDetectLspServers(_ context.Context, _ *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	dir, _ := args["workspace_dir"].(string)
	if dir == "" {
		return types.ErrorResult("workspace_dir is required"), nil
	}

	// Score languages by presence in the workspace.
	scores := make(map[string]int)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		name := d.Name()
		if d.IsDir() {
			if skipDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		// Root marker check (high priority: +50 per marker).
		if lang, ok := rootMarkers[name]; ok {
			scores[lang] += 50
		}
		// Extension check.
		if lang, ok := extLanguages[filepath.Ext(name)]; ok {
			scores[lang]++
		}
		return nil
	})
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("scanning workspace: %s", err)), nil
	}

	// Sort detected languages by score descending.
	type langScore struct {
		lang  string
		score int
	}
	var ranked []langScore
	for lang, score := range scores {
		ranked = append(ranked, langScore{lang, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].lang < ranked[j].lang
	})

	workspaceLangs := make([]string, len(ranked))
	for i, ls := range ranked {
		workspaceLangs[i] = ls.lang
	}

	// For each detected language, check PATH for the server binary.
	// Track which binaries we've already located to avoid duplicates
	// (e.g. c and cpp both use clangd).
	seen := make(map[string]bool)
	var installed []DetectedServer
	var notInstalled []string

	for _, ls := range ranked {
		lang := ls.lang
		for _, def := range knownServers {
			if def.Language != lang {
				continue
			}
			if seen[def.Language] {
				break
			}
			seen[def.Language] = true

			binPath, lookErr := exec.LookPath(def.Binary)
			if lookErr != nil {
				notInstalled = append(notInstalled, fmt.Sprintf("%s (%s)", lang, def.Binary))
				break
			}

			entry := buildConfigEntry(def)
			installed = append(installed, DetectedServer{
				Language:    lang,
				Server:      def.Binary,
				Path:        binPath,
				ConfigEntry: entry,
			})
			break
		}
	}

	// Build suggested_config: one entry per installed server, deduped by binary.
	seenBinary := make(map[string]bool)
	var suggested []string
	for _, s := range installed {
		if !seenBinary[s.Server] {
			seenBinary[s.Server] = true
			suggested = append(suggested, s.ConfigEntry)
		}
	}

	result := DetectResult{
		WorkspaceDir:       dir,
		WorkspaceLanguages: workspaceLangs,
		InstalledServers:   installed,
		SuggestedConfig:    suggested,
		NotInstalled:       notInstalled,
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling detect result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// buildConfigEntry returns the agent-lsp args string for a server definition.
// Format: "language:binary" or "language:binary,arg1,arg2"
func buildConfigEntry(def lspServerDef) string {
	if len(def.Args) == 0 {
		return fmt.Sprintf("%s:%s", def.Language, def.Binary)
	}
	return fmt.Sprintf("%s:%s,%s", def.Language, def.Binary, strings.Join(def.Args, ","))
}
