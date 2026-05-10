package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/config"
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
func runInit(args []string) {
	nonInteractive := false
	for _, a := range args {
		if a == "--non-interactive" {
			nonInteractive = true
		}
	}

	// Step 2: Detect installed language servers.
	cfg, err := config.AutodetectServers()
	if err != nil {
		fmt.Println("No language servers found in PATH.")
		fmt.Println("Install at least one (e.g. `go install golang.org/x/tools/gopls@latest`)")
		fmt.Println("then run `agent-lsp init` again.")
		os.Exit(1)
	}

	// Step 3: Present servers and ask which to include.
	selected := cfg.Servers
	if !nonInteractive {
		fmt.Println("Detected language servers:")
		for i, entry := range cfg.Servers {
			fmt.Printf("  %d. %-12s %s\n", i+1, entry.LanguageID, filepath.Base(entry.Command[0]))
		}
		reader := bufio.NewReader(os.Stdin)
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
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Choice [1-8]: ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		n := 0
		if len(line) == 1 && line[0] >= '1' && line[0] <= '8' {
			n = int(line[0] - '0')
		}
		if n >= 1 && n <= 8 {
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
		rulesContent := generateRulesContent()
		if choice == 1 || choice == 2 {
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

	// Step 9: Print result and next step.
	data, err := os.ReadFile(targetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading written config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote MCP config to: %s\n\n", targetPath)
	fmt.Println("Config written:")
	fmt.Println(string(data))
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
// the "lsp" key in mcpServers, and writes the result back.
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

	cfg.MCPServers["lsp"] = mcpServerEntry{
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
	default:
		return ""
	}
}

// generateRulesContent builds the skill awareness rules from embedded SKILL.md
// files. The content is the same for all providers; only the target file differs.
func generateRulesContent() string {
	var b strings.Builder
	b.WriteString("## agent-lsp Skills\n\n")
	b.WriteString("agent-lsp provides 56 code intelligence tools and 22 workflow skills.\n")
	b.WriteString("Prefer LSP tools over Grep/Glob/Read for code navigation.\n\n")
	b.WriteString("**Before editing code:** call `get_change_impact` for blast-radius analysis.\n")
	b.WriteString("**Before applying edits:** call `preview_edit` to preview the diagnostic delta.\n")
	b.WriteString("**After any change:** call `get_diagnostics`, then `run_build` and `run_tests`.\n\n")
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

	b.WriteString("\nCall `prompts/get` with any skill name for full workflow instructions.\n")
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
