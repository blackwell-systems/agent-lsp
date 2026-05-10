package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// doctorCapabilityEntry maps one LSP capability key to the MCP tools it enables.
type doctorCapabilityEntry struct {
	capability string
	tools      []string
}

// doctorCapabilityMap is a local copy of internal/tools/capabilities.go's
// capabilityToolMap — kept here to avoid exporting internal state.
var doctorCapabilityMap = []doctorCapabilityEntry{
	{"hoverProvider", []string{"get_info_on_location"}},
	{"completionProvider", []string{"get_completions"}},
	{"signatureHelpProvider", []string{"get_signature_help"}},
	{"definitionProvider", []string{"go_to_definition"}},
	{"typeDefinitionProvider", []string{"go_to_type_definition"}},
	{"implementationProvider", []string{"go_to_implementation"}},
	{"declarationProvider", []string{"go_to_declaration"}},
	{"referencesProvider", []string{"get_references"}},
	{"documentSymbolProvider", []string{"get_document_symbols"}},
	{"workspaceSymbolProvider", []string{"get_workspace_symbols"}},
	{"documentFormattingProvider", []string{"format_document"}},
	{"documentRangeFormattingProvider", []string{"format_range"}},
	{"renameProvider", []string{"rename_symbol", "prepare_rename"}},
	{"codeActionProvider", []string{"get_code_actions"}},
	{"semanticTokensProvider", []string{"get_semantic_tokens"}},
	{"callHierarchyProvider", []string{"call_hierarchy"}},
	{"typeHierarchyProvider", []string{"type_hierarchy"}},
	{"inlayHintProvider", []string{"get_inlay_hints"}},
	{"diagnosticProvider", []string{"get_diagnostics"}},
}

// alwaysAvailableDoctorTools are tools that do not require a server capability.
var alwaysAvailableDoctorTools = []string{
	"start_lsp",
	"restart_lsp_server",
	"open_document",
	"close_document",
	"did_change_watched_files",
	"apply_edit",
	"execute_command",
	"set_log_level",
	"detect_lsp_servers",
}

// DoctorResult holds the diagnostic result for one language server.
type DoctorResult struct {
	LanguageID    string
	Binary        string
	Status        string // "ok" | "failed"
	Error         string // non-empty when Status=="failed"
	ServerName    string
	ServerVersion string
	Capabilities  []string // sorted capability keys
	Tools         []string // MCP tool names enabled
}

// buildDoctorEntries is a no-op identity helper that makes the server list
// construction testable without calling os.Exit.
func buildDoctorEntries(entries []config.ServerEntry) []config.ServerEntry {
	return entries
}

// probeServer starts the LSP server briefly, queries its info and capabilities,
// then shuts it down. Returns a DoctorResult.
func probeServer(entry config.ServerEntry) DoctorResult {
	binary := ""
	if len(entry.Command) > 0 {
		binary = entry.Command[0]
	}

	result := DoctorResult{
		LanguageID: entry.LanguageID,
		Binary:     binary,
	}

	tmpDir, err := os.MkdirTemp("", "agent-lsp-doctor-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("create temp dir: %v", err)
		return result
	}
	defer os.RemoveAll(tmpDir)

	if len(entry.Command) == 0 {
		result.Status = "failed"
		result.Error = "server entry has no command"
		return result
	}

	client := lsp.NewLSPClient(entry.Command[0], entry.Command[1:])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, tmpDir); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		// Best-effort shutdown.
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = client.Shutdown(shutCtx)
		return result
	}

	name, version := client.GetServerInfo()
	caps := client.GetCapabilities()

	// Derive capability keys that are present.
	var capKeys []string
	var toolNames []string
	for _, entry := range doctorCapabilityMap {
		if doctorHasCapability(caps, entry.capability) {
			capKeys = append(capKeys, entry.capability)
			toolNames = append(toolNames, entry.tools...)
		}
	}
	sort.Strings(capKeys)
	sort.Strings(toolNames)

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	_ = client.Shutdown(shutCtx)

	result.Status = "ok"
	result.ServerName = name
	result.ServerVersion = version
	result.Capabilities = capKeys
	result.Tools = toolNames
	return result
}

// doctorHasCapability checks whether a capability key is present and truthy.
func doctorHasCapability(caps map[string]any, key string) bool {
	v, ok := caps[key]
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return v != nil
}

// printDoctorReport prints the human-readable diagnostic report to stdout.
func printDoctorReport(results []DoctorResult) {
	fmt.Println("agent-lsp doctor")
	fmt.Println()

	for _, r := range results {
		binary := r.Binary
		if binary == "" {
			binary = "(unknown)"
		}
		fmt.Printf("● %s (%s)\n", r.LanguageID, binary)
		fmt.Printf("  Status:  %s\n", r.Status)

		if r.Status == "failed" {
			fmt.Printf("  Error:   %s\n", r.Error)
		} else {
			serverLabel := r.ServerName
			if r.ServerVersion != "" {
				serverLabel += " v" + r.ServerVersion
			}
			fmt.Printf("  Server:  %s\n", serverLabel)

			fmt.Printf("  Capabilities (%d):\n", len(r.Capabilities))
			for _, cap := range r.Capabilities {
				// Find the tools for this capability.
				var capTools []string
				for _, entry := range doctorCapabilityMap {
					if entry.capability == cap {
						capTools = entry.tools
						break
					}
				}
				fmt.Printf("    %-35s → %s\n", cap, joinStrings(capTools))
			}

			fmt.Printf("  Always-available tools:\n")
			fmt.Printf("    %s\n", joinStrings(alwaysAvailableDoctorTools))
		}
		fmt.Println()
	}

	ok := 0
	failed := 0
	for _, r := range results {
		if r.Status == "ok" {
			ok++
		} else {
			failed++
		}
	}
	fmt.Printf("Summary: %d ok, %d failed\n", ok, failed)
}

// joinStrings joins a slice of strings with ", ".
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// runDoctor is the entry point for `agent-lsp doctor`.
// args is os.Args[2:] (all args after "doctor").
// Does not return — uses os.Exit for fatal conditions.
func runDoctor(args []string) {
	var entries []config.ServerEntry

	if len(args) == 0 {
		cfg, err := config.AutodetectServers()
		if err != nil {
			fmt.Println("No language servers found in PATH.")
			fmt.Println("Install at least one (e.g. `go install golang.org/x/tools/gopls@latest`)")
			fmt.Println("then run `agent-lsp doctor` again.")
			fmt.Println("Or pass explicit server specs: agent-lsp doctor go:gopls typescript:typescript-language-server,--stdio")
			os.Exit(1)
		}
		entries = buildDoctorEntries(cfg.Servers)
	} else {
		parsed, err := config.ParseArgs(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		if parsed.IsSingleServer {
			entries = buildDoctorEntries([]config.ServerEntry{
				{
					LanguageID: parsed.LanguageID,
					Command:    append([]string{parsed.ServerPath}, parsed.ServerArgs...),
				},
			})
		} else {
			entries = buildDoctorEntries(parsed.Config.Servers)
		}
	}

	var results []DoctorResult
	for _, entry := range entries {
		result := probeServer(entry)
		results = append(results, result)
	}

	printDoctorReport(results)

	for _, r := range results {
		if r.Status == "failed" {
			os.Exit(1)
		}
	}
}
