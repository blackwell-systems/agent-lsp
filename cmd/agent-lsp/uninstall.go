package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/skills"
)

// runUninstall is the entry point for `agent-lsp uninstall`.
// It removes all agent-lsp configs, skills, and caches.
const uninstallUsage = `Usage: agent-lsp uninstall [--dry-run]

Remove agent-lsp's entries from AI tool MCP configs, the skills it installed,
its managed rules sections, and its caches. The binary itself is not removed.

Options:
  --dry-run    show what would be removed without changing anything
  -h, --help   show this help and exit without changing anything
`

// parseUninstallArgs parses uninstall's arguments. Unknown arguments are an
// error rather than ignored: uninstall deletes files, and "uninstall --help"
// used to fall through and perform a real uninstall.
func parseUninstallArgs(args []string) (dryRun bool, err error) {
	for _, a := range args {
		switch a {
		case "--dry-run":
			dryRun = true
		case "-h", "--help", "help":
			return false, errHelpRequested
		default:
			return false, fmt.Errorf("unknown argument %q", a)
		}
	}
	return dryRun, nil
}

func runUninstall(args []string) {
	dryRun, err := parseUninstallArgs(args)
	if errors.Is(err, errHelpRequested) {
		fmt.Print(uninstallUsage)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-lsp uninstall: %v\n\n%s", err, uninstallUsage)
		os.Exit(2)
	}

	removed := 0
	skipped := 0

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not determine home directory: %v\n", err)
		os.Exit(1)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not determine working directory: %v\n", err)
		os.Exit(1)
	}

	// Step 1: MCP config cleanup.
	mcpPaths := []string{
		filepath.Join(cwd, ".mcp.json"),
		filepath.Join(homeDir, ".claude.json"),         // Claude Code user scope
		filepath.Join(homeDir, ".claude", ".mcp.json"), // written by init before v0.24.1; never read by Claude Code
		filepath.Join(homeDir, ".config", "mcp", "mcp.json"),
		filepath.Join(cwd, ".cursor", "mcp.json"),
		filepath.Join(cwd, ".vscode", "cline_mcp_settings.json"),
		filepath.Join(cwd, ".gemini", "settings.json"),
	}

	for _, p := range mcpPaths {
		r, s := cleanMCPConfig(p, dryRun)
		removed += r
		skipped += s
	}
	// Claude Code may rewrite ~/.claude.json from memory while running, so
	// also remove the user-scope entry through its CLI when it is installed.
	if !dryRun {
		_ = claudeCLI("mcp", "remove", "--scope", "user", "agent-lsp")
	}

	// Step 2: Skill directory cleanup (all managed destinations).
	skillsDests := []string{
		filepath.Join(homeDir, ".claude", "skills"),
		filepath.Join(homeDir, ".pi", "agent", "skills"),
		filepath.Join(homeDir, ".cursor", "skills"),
		filepath.Join(homeDir, ".config", "gemini-cli", "skills"),
		filepath.Join(cwd, ".agents", "skills"),
	}
	for _, dir := range skillsDests {
		r, s := cleanSkillDirs(dir, dryRun)
		removed += r
		skipped += s
	}

	// Step 3: Managed rules-section cleanup (Claude Code and Pi context files).
	rulesPaths := managedRulesPaths()
	for _, p := range rulesPaths {
		r, s := cleanManagedSection(p, dryRun)
		removed += r
		skipped += s
	}

	// Step 4: Cache directory cleanup.
	cachePaths := []string{
		filepath.Join(homeDir, ".agent-lsp", "cache"),
		filepath.Join(cwd, ".agent-lsp", "cache.db.gz"),
	}
	for _, p := range cachePaths {
		r, s := cleanPath(p, dryRun)
		removed += r
		skipped += s
	}

	fmt.Printf("Removed %d items. Skipped %d items (not found).\n", removed, skipped)
	fmt.Println("To remove the binary: rm $(which agent-lsp)")
}

// cleanMCPConfig reads a JSON config file, removes the "agent-lsp" and "lsp"
// keys from mcpServers, and writes the result back. Returns (removed, skipped).
func cleanMCPConfig(path string, dryRun bool) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 1
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse %s: %v\n", path, err)
		return 0, 1
	}

	serversRaw, ok := raw["mcpServers"]
	if !ok {
		return 0, 1
	}
	servers, ok := serversRaw.(map[string]any)
	if !ok {
		return 0, 1
	}

	keysToRemove := []string{"agent-lsp", "lsp"}
	removedCount := 0
	simulatedServers := len(servers)
	for _, key := range keysToRemove {
		if _, exists := servers[key]; exists {
			if dryRun {
				fmt.Printf("[dry-run] Would remove key %q from mcpServers in %s\n", key, path)
				simulatedServers--
			} else {
				delete(servers, key)
			}
			removedCount++
		}
	}

	// A config that holds nothing of user value (an empty mcpServers map
	// and no other top-level keys) is a husk init likely created: remove it,
	// whether or not this run removed a key from it. In dry-run the simulated
	// post-removal server count decides, so a config that would become empty
	// after the reported key removals is previewed as deleted too.
	effectiveServers := len(servers)
	if dryRun {
		effectiveServers = simulatedServers
	}
	emptyHusk := effectiveServers == 0 && len(raw) == 1
	if removedCount == 0 && !emptyHusk {
		return 0, 1
	}

	if emptyHusk {
		if dryRun {
			fmt.Printf("[dry-run] Would remove empty config file %s\n", path)
		} else if err := os.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not remove empty config %s: %v\n", path, err)
			// The file stays on disk with its original MCP entries, so report
			// it as skipped rather than removed. (PR #36 review follow-up)
			return 0, 1
		}
		if removedCount == 0 {
			removedCount++
		}
		return removedCount, 0
	}

	if !dryRun {
		out, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not marshal %s: %v\n", path, err)
			return 0, 1
		}
		out = append(out, '\n')
		if err := os.WriteFile(path, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", path, err)
			return 0, 1
		}
	}

	return removedCount, 0
}

// cleanSkillDirs removes the managed skill directories from the skills
// directory. A directory counts as managed only when its name matches the
// skill set embedded in this binary (skills.Names): the lsp- prefix alone does
// not establish ownership, and a user-owned skill sharing the prefix must
// survive uninstall. Returns (removed, skipped).
func cleanSkillDirs(skillsDir string, dryRun bool) (int, int) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return 0, 1
	}

	managed := make(map[string]bool)
	for _, name := range skills.Names() {
		managed[name] = true
	}

	removedCount := 0
	for _, e := range entries {
		if e.IsDir() && managed[e.Name()] {
			p := filepath.Join(skillsDir, e.Name())
			if dryRun {
				fmt.Printf("[dry-run] Would remove skill directory %s\n", p)
			} else {
				if err := os.RemoveAll(p); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", p, err)
					continue
				}
			}
			removedCount++
		}
	}

	if removedCount == 0 {
		return 0, 1
	}
	return removedCount, 0
}

// managedRulesPaths lists every rules file init can write a managed section to.
// It is derived from init's own resolveRulesPath so the two cannot drift: a
// hand-maintained list here covered only 3 of the 8 files init writes, leaving
// the managed section behind in CLAUDE.md, GEMINI.md, .clinerules,
// .windsurfrules, and .cursor/rules/agent-lsp.mdc. Choices resolveRulesPath
// does not know return "", so scanning past the current menu is harmless.
func managedRulesPaths() []string {
	seen := make(map[string]bool)
	var paths []string
	for choice := 1; choice <= 64; choice++ {
		if p := resolveRulesPath(choice); p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// managedSentinels lists the sentinel pairs used by managed sections written by
// `agent-lsp init`. Both pairs are handled so sections written by older versions
// are still removed.
var managedSentinels = [][2]string{
	{"<!-- agent-lsp:rules:start -->", "<!-- agent-lsp:rules:end -->"},
	{"<!-- agent-lsp:skills:start -->", "<!-- agent-lsp:skills:end -->"},
}

// cleanManagedSection removes every managed section between sentinel comment
// pairs from the given file: a file may hold both the rules and skills
// sentinel pairs, and all of them belong to agent-lsp. Returns (removed,
// skipped).
func cleanManagedSection(path string, dryRun bool) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 1
	}

	content := string(data)
	newContent := content
	removedCount := 0
	for _, pair := range managedSentinels {
		for {
			s := strings.Index(newContent, pair[0])
			if s == -1 {
				break
			}
			e := strings.Index(newContent, pair[1])
			if e == -1 || e < s {
				break
			}
			endIdx := e + len(pair[1])
			// Also remove a trailing newline if present.
			if endIdx < len(newContent) && newContent[endIdx] == '\n' {
				endIdx++
			}
			newContent = newContent[:s] + newContent[endIdx:]
			removedCount++
		}
	}
	if removedCount == 0 {
		return 0, 1
	}

	if dryRun {
		fmt.Printf("[dry-run] Would remove %d managed section(s) from %s\n", removedCount, path)
		// Report the simulated end state: if nothing user-owned would remain,
		// the file would be deleted after the section removals.
		if strings.TrimSpace(newContent) == "" {
			fmt.Printf("[dry-run] Would remove empty rules file %s\n", path)
		}
		return removedCount, 0
	}

	// If the file holds nothing but the removed sections, remove the file
	// init likely created.
	if strings.TrimSpace(newContent) == "" {
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not remove empty rules file %s: %v\n", path, err)
			return 0, 1
		}
		return removedCount, 0
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", path, err)
		return 0, 1
	}

	return removedCount, 0
}

// cleanPath removes a file or directory. Returns (removed, skipped).
func cleanPath(path string, dryRun bool) (int, int) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, 1
	}

	if dryRun {
		fmt.Printf("[dry-run] Would remove %s\n", path)
		return 1, 0
	}

	if err := os.RemoveAll(path); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", path, err)
		return 0, 1
	}
	return 1, 0
}
