package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/skills"
)

type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// runInit is the entry point for `agent-lsp init`.
// args is os.Args[2:] (all args after "init").
// Does not return — uses os.Exit for fatal conditions.
// errHelpRequested reports that a subcommand was asked for its usage. Callers
// print the usage and exit without doing any work.
var errHelpRequested = errors.New("help requested")

const initUsage = `Usage: agent-lsp init [--non-interactive] [--with-skills]

Detect installed language servers, write the MCP config for the chosen AI tool,
and add skill awareness rules to that tool's rules file.

Options:
  --non-interactive   use all detected servers and configure Claude Code for the
                      current directory (.mcp.json) without prompting
  --with-skills       also install the bundled skills for the chosen tool
  -h, --help          show this help and exit without changing anything
`

type initOptions struct {
	nonInteractive bool
	withSkills     bool
}

// parseInitArgs parses init's arguments. Unknown arguments are an error rather
// than ignored: init writes to the user's config files, so a mistyped or
// unsupported flag (including --help, before it was handled) must never fall
// through to a real run.
func parseInitArgs(args []string) (initOptions, error) {
	var o initOptions
	for _, a := range args {
		switch a {
		case "--non-interactive":
			o.nonInteractive = true
		case "--with-skills":
			o.withSkills = true
		case "-h", "--help", "help":
			return o, errHelpRequested
		default:
			return o, fmt.Errorf("unknown argument %q", a)
		}
	}
	return o, nil
}

func runInit(args []string) {
	opts, err := parseInitArgs(args)
	if errors.Is(err, errHelpRequested) {
		fmt.Print(initUsage)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-lsp init: %v\n\n%s", err, initUsage)
		os.Exit(2)
	}
	nonInteractive := opts.nonInteractive
	withSkills := opts.withSkills

	// Step 2: Detect installed language servers.
	cfg, err := config.AutodetectServers()
	if err != nil {
		fmt.Println("No language servers found in PATH.")
		fmt.Println("Install at least one (e.g. `go install golang.org/x/tools/gopls@latest`)")
		fmt.Println("then run `agent-lsp init` again.")
		os.Exit(1)
	}

	// Single shared stdin reader for all prompts. Creating a new reader per
	// prompt loses buffered input when stdin is piped or scripted.
	reader := bufio.NewReader(os.Stdin)

	// Step 3: Present servers and ask which to include.
	selected := cfg.Servers
	if !nonInteractive {
		fmt.Println("Detected language servers:")
		for i, entry := range cfg.Servers {
			fmt.Printf("  %d. %-12s %s\n", i+1, entry.LanguageID, filepath.Base(entry.Command[0]))
		}
		fmt.Print("Include all detected servers? [Y/n]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)
		if strings.EqualFold(answer, "n") {
			var kept []config.ServerEntry
			for _, entry := range cfg.Servers {
				fmt.Printf("Include %s (%s)? [y/N]: ", entry.LanguageID, filepath.Base(entry.Command[0]))
				a2, _ := reader.ReadString('\n')
				a2 = strings.TrimSpace(a2)
				if strings.EqualFold(a2, "y") {
					kept = append(kept, entry)
				}
			}
			if len(kept) == 0 {
				fmt.Println("No servers selected. Exiting.")
				os.Exit(1)
			}
			selected = kept
		}
	}

	// Step 4: Choose AI tool target.
	choice := 1
	customPath := ""
	if !nonInteractive {
		fmt.Println("Which AI tool to configure?")
		fmt.Println("  1. Claude Code  (project .mcp.json in current directory)")
		fmt.Println("  2. Claude Code  (global ~/.claude/.mcp.json)")
		fmt.Println("  3. Claude Desktop")
		fmt.Println("  4. Cursor       (.cursor/mcp.json in current directory)")
		fmt.Println("  5. Cline/VS Code (.vscode/cline_mcp_settings.json in current directory)")
		fmt.Println("  6. Windsurf     (~/.codeium/windsurf/mcp_config.json)")
		fmt.Println("  7. Gemini CLI   (project .gemini/settings.json in current directory)")
		fmt.Println("  8. Custom path")
		fmt.Println("  9. Pi           (project .mcp.json in current directory)")
		fmt.Println(" 10. Pi           (global ~/.config/mcp/mcp.json)")
		fmt.Print("Choice [1-10]: ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		n := 0
		if len(line) == 1 && line[0] >= '1' && line[0] <= '8' {
			n = int(line[0] - '0')
		}
		if line == "9" {
			n = 9
		}
		if line == "10" {
			n = 10
		}
		if n >= 1 && n <= 10 {
			choice = n
		}
		if choice == 8 {
			fmt.Print("Config file path: ")
			cp, _ := reader.ReadString('\n')
			customPath = strings.TrimSpace(cp)
		}
	}

	// Step 5: Resolve target file path.
	targetPath, err := resolveTargetPath(choice, customPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error resolving target path: %v\n", err)
		os.Exit(1)
	}

	// Step 6: Build lsp args.
	lspArgs := buildLspArgs(selected)

	// Step 7: Merge or create the config file.
	if err := writeOrMergeConfig(targetPath, lspArgs); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		os.Exit(1)
	}

	// Step 8: Write provider-specific rules file for skill awareness.
	rulesPath := resolveRulesPath(choice)
	if rulesPath != "" {
		isClaudeCode := choice == 1 || choice == 2
		rulesContent := generateRulesContent(selectRulesTarget(isClaudeCode, isPiChoice(choice)))
		if isClaudeCode {
			// Claude Code: inject managed section into CLAUDE.md.
			if err := writeManagedSection(rulesPath, rulesContent); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not write rules to %s: %v\n", rulesPath, err)
			} else {
				fmt.Printf("Wrote skill awareness rules to: %s\n", rulesPath)
			}
		} else {
			// Other providers: use managed section to preserve existing content.
			if err := writeManagedSection(rulesPath, rulesContent); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not write rules to %s: %v\n", rulesPath, err)
			} else {
				fmt.Printf("Wrote skill awareness rules to: %s\n", rulesPath)
			}
		}
	}

	// Step 9: Install embedded skills when requested.
	if withSkills {
		skillsDest := resolveSkillsDest(choice)
		if skillsDest == "" {
			fmt.Println("--with-skills: no known skills directory for this target; install manually with skills/install.sh --dest")
		} else {
			n, err := skills.Install(skillsDest)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not install skills to %s: %v\n", skillsDest, err)
			} else {
				fmt.Printf("Installed %d skills to: %s\n", n, skillsDest)
			}
		}
	}

	// Step 10: Print result and next step.
	isPi := isPiChoice(choice)
	data, err := os.ReadFile(targetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading written config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote MCP config to: %s\n\n", targetPath)
	fmt.Println("Config written:")
	fmt.Println(string(data))
	if isPi {
		fmt.Println("Pi loads MCP servers through the pi-mcp-adapter package.")
		fmt.Println("If it is not installed yet, run: pi install npm:pi-mcp-adapter")
	}
	fmt.Println("Next: restart your AI tool to pick up the new MCP server.")
}

// resolveTargetPath returns the absolute path for the given target choice.
func resolveTargetPath(choice int, customPath string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get working directory: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w", err)
	}

	switch choice {
	case 1:
		return filepath.Join(cwd, ".mcp.json"), nil
	case 2:
		return filepath.Join(homeDir, ".claude", ".mcp.json"), nil
	case 3:
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
		case "windows":
			return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json"), nil
		default:
			return filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json"), nil
		}
	case 4:
		return filepath.Join(cwd, ".cursor", "mcp.json"), nil
	case 5:
		return filepath.Join(cwd, ".vscode", "cline_mcp_settings.json"), nil
	case 6:
		return filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"), nil
	case 7:
		return filepath.Join(cwd, ".gemini", "settings.json"), nil
	case 9:
		return filepath.Join(cwd, ".mcp.json"), nil
	case 10:
		return filepath.Join(homeDir, ".config", "mcp", "mcp.json"), nil
	case 8:
		if strings.HasPrefix(customPath, "~/") {
			customPath = homeDir + "/" + customPath[2:]
		}
		return customPath, nil
	default:
		return filepath.Join(cwd, ".mcp.json"), nil
	}
}

// buildLspArgs converts a slice of config.ServerEntry into args strings for the MCP config.
func buildLspArgs(entries []config.ServerEntry) []string {
	args := make([]string, 0, len(entries))
	for _, entry := range entries {
		base := filepath.Base(entry.Command[0])
		arg := entry.LanguageID + ":" + base
		if len(entry.Command) > 1 {
			arg += "," + strings.Join(entry.Command[1:], ",")
		}
		args = append(args, arg)
	}
	return args
}

// writeOrMergeConfig reads an existing config at path (if any), sets/overwrites
// the "agent-lsp" key in mcpServers, and writes the result back.
func writeOrMergeConfig(path string, lspArgs []string) error {
	var cfg mcpConfig

	if data, err := os.ReadFile(path); err == nil {
		// File exists — unmarshal it.
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse existing config at %s: %w", path, err)
		}
		if cfg.MCPServers == nil {
			cfg.MCPServers = make(map[string]mcpServerEntry)
		}
	} else {
		// File does not exist — create parent directories.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", path, err)
		}
		cfg = mcpConfig{
			MCPServers: make(map[string]mcpServerEntry),
		}
	}

	// Remove legacy "lsp" key if present (renamed to "agent-lsp" in v0.12.0)
	delete(cfg.MCPServers, "lsp")

	cfg.MCPServers["agent-lsp"] = mcpServerEntry{
		Type:    "stdio",
		Command: "agent-lsp",
		Args:    lspArgs,
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("failed to write config to %s: %w", path, err)
	}

	return nil
}

const (
	managedSectionStart = "<!-- agent-lsp:rules:start -->"
	managedSectionEnd   = "<!-- agent-lsp:rules:end -->"
)

// resolveRulesPath returns the provider-specific rules file path for the given
// init choice. Returns empty string for providers that don't support rules files.
func resolveRulesPath(choice int) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	homeDir, _ := os.UserHomeDir()

	switch choice {
	case 1:
		return filepath.Join(cwd, "CLAUDE.md")
	case 2:
		return filepath.Join(homeDir, ".claude", "CLAUDE.md")
	case 3:
		return "" // Claude Desktop: no rules file, uses Instructions only
	case 4:
		return filepath.Join(cwd, ".cursor", "rules", "agent-lsp.mdc")
	case 5:
		return filepath.Join(cwd, ".clinerules")
	case 6:
		return filepath.Join(homeDir, ".windsurfrules")
	case 7:
		return filepath.Join(cwd, "GEMINI.md")
	case 9:
		// Pi project context file: AGENTS.md (CLAUDE.md is the fallback Pi accepts).
		return filepath.Join(cwd, "AGENTS.md")
	case 10:
		// Pi user instructions: agent-directory AGENTS.md.
		return filepath.Join(homeDir, ".pi", "agent", "AGENTS.md")
	default:
		return ""
	}
}

// rulesTarget selects how the rules content should point the agent at the
// skill workflows, based on the provider's actual skill delivery mechanism.
type rulesTarget int

const (
	rulesTargetGeneric rulesTarget = iota
	rulesTargetClaudeCode
	rulesTargetPi
)

func selectRulesTarget(isClaudeCode, isPi bool) rulesTarget {
	switch {
	case isClaudeCode:
		return rulesTargetClaudeCode
	case isPi:
		return rulesTargetPi
	default:
		return rulesTargetGeneric
	}
}

func isPiChoice(choice int) bool {
	return choice == 9 || choice == 10
}

// resolveSkillsDest returns the skills directory for the given init choice,
// or "" when the provider has no verified skills directory. Project-level
// targets use the tool-agnostic AgentSkills location so the skill set can be
// versioned with the repository; user-level targets use each provider's own
// directory. All destinations mirror skills/install.sh documentation.
func resolveSkillsDest(choice int) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	homeDir, _ := os.UserHomeDir()

	switch choice {
	case 1, 2:
		return filepath.Join(homeDir, ".claude", "skills")
	case 4:
		return filepath.Join(homeDir, ".cursor", "skills")
	case 7:
		return filepath.Join(homeDir, ".config", "gemini-cli", "skills")
	case 9:
		return filepath.Join(cwd, ".agents", "skills")
	case 10:
		return filepath.Join(homeDir, ".pi", "agent", "skills")
	default:
		return ""
	}
}

// generateRulesContent builds the skill awareness rules from embedded SKILL.md
// files. The rulesTarget tunes provider-specific guidance: Claude Code gets
// stronger enforcement language against its built-in tools, and Pi gets the
// slash-command route registered by pi-mcp-adapter instead of raw prompts/get.
func generateRulesContent(target ...rulesTarget) string {
	t := rulesTargetGeneric
	if len(target) > 0 {
		t = target[0]
	}
	claude := t == rulesTargetClaudeCode
	var b strings.Builder
	b.WriteString("## agent-lsp Skills\n\n")
	fmt.Fprintf(&b, "agent-lsp provides %d code intelligence tools and %d workflow skills.\n", advertisedToolCount, advertisedSkillCount)
	b.WriteString("Prefer these tools over text search for code intelligence tasks.\n\n")
	b.WriteString("**Before editing code:** call `blast_radius` for blast-radius analysis.\n")
	b.WriteString("**Before applying edits:** call `preview_edit` to preview the diagnostic delta.\n")
	b.WriteString("**After any change:** call `get_diagnostics`, then `run_build` and `run_tests`.\n\n")
	if claude {
		b.WriteString("**Task-to-tool mapping (use these instead of Read/Grep for code):**\n\n")
		b.WriteString("| Task | Use this | Not this |\n")
		b.WriteString("|------|----------|----------|\n")
		b.WriteString("| See file structure | `list_symbols` | `Read` + manual scanning |\n")
		b.WriteString("| Find a symbol by name | `find_symbol` | `Grep` across files |\n")
		b.WriteString("| Find all usages | `find_references` | `Grep` for the name |\n")
		b.WriteString("| Understand a symbol | `inspect_symbol` | `Read` the file |\n")
		b.WriteString("| What calls this function | `find_callers` | `Grep` for the name |\n")
		b.WriteString("| Replace a function body | `replace_symbol_body` | `Edit` with text matching |\n")
		b.WriteString("| Delete unused symbol | `safe_delete_symbol` | `Edit` to remove lines |\n\n")
	}
	b.WriteString("| Skill | Description |\n")
	b.WriteString("|-------|-------------|\n")

	for _, meta := range loadSkills() {
		desc := meta.Description
		// Truncate long descriptions for the table.
		if len(desc) > 120 {
			desc = desc[:117] + "..."
		}
		fmt.Fprintf(&b, "| `/%s` | %s |\n", meta.Name, desc)
	}

	switch t {
	case rulesTargetPi:
		// pi-mcp-adapter registers MCP prompts as Pi slash commands, and the
		// server exposes activate_skill for phase enforcement.
		b.WriteString("\nLoad full workflow instructions with `/mcp__agent-lsp__<skill>` slash commands (e.g. `/mcp__agent-lsp__lsp-refactor`), or call the `activate_skill` tool with a skill name to enable phase enforcement.\n")
	default:
		b.WriteString("\nCall `prompts/get` with any skill name for full workflow instructions.\n")
	}
	return b.String()
}

// writeManagedSection inserts or replaces a managed section in an existing
// file (e.g., CLAUDE.md). Content between sentinel comments is replaced;
// content outside the sentinels is preserved.
func writeManagedSection(path, content string) error {
	managed := managedSectionStart + "\n" + content + managedSectionEnd + "\n"

	existing, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist: create it with just the managed section.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(managed), 0o644)
	}

	text := string(existing)
	startIdx := strings.Index(text, managedSectionStart)
	endIdx := strings.Index(text, managedSectionEnd)

	if startIdx >= 0 && endIdx >= 0 {
		// Replace existing managed section.
		result := text[:startIdx] + managed + text[endIdx+len(managedSectionEnd):]
		// Trim any trailing double newlines from the replacement.
		result = strings.TrimRight(result, "\n") + "\n"
		return os.WriteFile(path, []byte(result), 0o644)
	}

	// No existing section: append.
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n" + managed
	return os.WriteFile(path, []byte(text), 0o644)
}
