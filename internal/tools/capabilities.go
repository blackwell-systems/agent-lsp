package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// capabilityToolEntry maps an LSP capability key to the MCP tools it enables.
type capabilityToolEntry struct {
	capability string
	tools      []string
}

// capabilityToolMap is the canonical mapping from LSP capability keys to
// lsp-mcp-go tool names. Order determines output order in the response.
var capabilityToolMap = []capabilityToolEntry{
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

// alwaysAvailableTools are tools that do not require a server capability —
// they work regardless of what the language server advertises.
var alwaysAvailableTools = []string{
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

// ServerCapabilitiesResult is the response shape for get_server_capabilities.
type ServerCapabilitiesResult struct {
	ServerName      string                 `json:"server_name,omitempty"`
	ServerVersion   string                 `json:"server_version,omitempty"`
	SupportedTools  []string               `json:"supported_tools"`
	UnsupportedTools []string              `json:"unsupported_tools"`
	Capabilities    map[string]interface{} `json:"capabilities"`
}

// HandleGetServerCapabilities returns the language server's capability map
// and classifies every lsp-mcp-go tool as supported or unsupported based on
// what the server advertised during initialization.
//
// This lets the AI skip tools that will return empty results and avoid
// unnecessary LSP round trips for unsupported features.
func HandleGetServerCapabilities(_ context.Context, client *lsp.LSPClient, _ map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	caps := client.GetCapabilities()
	name, version := client.GetServerInfo()

	var supported []string
	var unsupported []string

	// Always-available tools come first.
	supported = append(supported, alwaysAvailableTools...)

	// Capability-gated tools.
	for _, entry := range capabilityToolMap {
		if hasCapabilityInMap(caps, entry.capability) {
			supported = append(supported, entry.tools...)
		} else {
			unsupported = append(unsupported, entry.tools...)
		}
	}

	sort.Strings(supported)
	sort.Strings(unsupported)

	result := ServerCapabilitiesResult{
		ServerName:       name,
		ServerVersion:    version,
		SupportedTools:   supported,
		UnsupportedTools: unsupported,
		Capabilities:     caps,
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling capabilities: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// hasCapabilityInMap checks whether a capability key is present and truthy
// in the given map — mirrors the client's hasCapability logic.
func hasCapabilityInMap(caps map[string]interface{}, key string) bool {
	v, ok := caps[key]
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return v != nil
}
