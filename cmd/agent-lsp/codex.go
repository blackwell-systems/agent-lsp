package main

// codex.go configures agent-lsp for OpenAI Codex (CLI, IDE extension) and the
// ChatGPT desktop app, which share Codex's MCP configuration: a TOML file at
// $CODEX_HOME/config.toml (default ~/.codex/config.toml), or a project's
// .codex/config.toml. Issue #67.
//
// The server is one table:
//
//	[mcp_servers.agent-lsp]
//	command = "agent-lsp"
//	args = ["go:gopls"]
//
// Every other target writes JSON. Rather than decode and re-encode the TOML,
// which would drop the user's comments and reorder their settings, the file is
// edited as text: the agent-lsp table (and any sub-tables such as
// [mcp_servers.agent-lsp.env]) is removed and a fresh one appended, and every
// other line is kept byte for byte.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// codexServerName is the key agent-lsp registers under mcp_servers.
const codexServerName = "agent-lsp"

// codexHome returns Codex's configuration directory: $CODEX_HOME, or ~/.codex.
func codexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

// codexCLI runs the codex CLI; a variable so tests never touch a real Codex
// configuration.
var codexCLI = func(args ...string) error {
	path, err := exec.LookPath("codex")
	if err != nil {
		return err
	}
	return exec.Command(path, args...).Run()
}

// addCodexUserScope registers agent-lsp with `codex mcp add`, replacing an
// existing entry. It returns an error when the CLI is unavailable or fails.
func addCodexUserScope(lspArgs []string) error {
	_ = codexCLI("mcp", "remove", codexServerName) // absent is fine
	args := append([]string{"mcp", "add", codexServerName, "--", "agent-lsp"}, lspArgs...)
	return codexCLI(args...)
}

// tomlString encodes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// codexServerTable renders the agent-lsp table.
func codexServerTable(lspArgs []string) string {
	quoted := make([]string, len(lspArgs))
	for i, a := range lspArgs {
		quoted[i] = tomlString(a)
	}
	return fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nargs = [%s]\n",
		codexServerName, tomlString("agent-lsp"), strings.Join(quoted, ", "))
}

// isCodexServerHeader reports whether a TOML table header line names the
// agent-lsp server table or one of its sub-tables, in bare or quoted form:
// [mcp_servers.agent-lsp], [mcp_servers."agent-lsp"], [mcp_servers.agent-lsp.env].
func isCodexServerHeader(line string) bool {
	t := strings.TrimSpace(line)
	if i := strings.Index(t, "#"); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	if !strings.HasPrefix(t, "[") || strings.HasPrefix(t, "[[") || !strings.HasSuffix(t, "]") {
		return false
	}
	name := strings.TrimSpace(t[1 : len(t)-1])
	for _, form := range []string{"mcp_servers." + codexServerName, `mcp_servers."` + codexServerName + `"`} {
		if name == form || strings.HasPrefix(name, form+".") {
			return true
		}
	}
	return false
}

// isTOMLHeader reports whether a line starts a table or array-of-tables.
func isTOMLHeader(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "[")
}

// removeCodexServerTables removes the agent-lsp table and its sub-tables from
// TOML text, keeping every other line as it was. A table runs from its header
// to the next header. It reports whether anything was removed.
func removeCodexServerTables(content string) (string, bool) {
	lines := strings.SplitAfter(content, "\n")
	var out []string
	removed, inTable := false, false
	for _, line := range lines {
		if isTOMLHeader(line) {
			inTable = isCodexServerHeader(line)
			if inTable {
				removed = true
				continue
			}
		}
		if !inTable {
			out = append(out, line)
		}
	}
	return strings.Join(out, ""), removed
}

// writeCodexConfig sets the agent-lsp table in the Codex config at path,
// creating the file if needed and keeping everything else in it unchanged.
func writeCodexConfig(path string, lspArgs []string) error {
	var content string
	mode := os.FileMode(0o644)
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
	} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", path, err)
	}

	content, _ = removeCodexServerTables(content)
	content = strings.TrimRight(content, "\n")
	if content != "" {
		content += "\n\n"
	}
	content += codexServerTable(lspArgs)

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return fmt.Errorf("failed to write config to %s: %w", path, err)
	}
	return nil
}

// cleanCodexConfig removes the agent-lsp table from the Codex config at path.
// Returns (removed, skipped) like cleanMCPConfig.
func cleanCodexConfig(path string, dryRun bool) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 1
	}
	updated, removed := removeCodexServerTables(string(data))
	if !removed {
		return 0, 1
	}
	if dryRun {
		fmt.Printf("[dry-run] Would remove [mcp_servers.%s] from %s\n", codexServerName, path)
		return 1, 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, 1
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", path, err)
		return 0, 1
	}
	return 1, 0
}
