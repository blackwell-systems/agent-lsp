package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// crossRepoRef represents a single reference found in a cross-repo lookup,
// annotated with the consumer repo it belongs to.
type crossRepoRef struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Repo   string `json:"repo"`
}

// repoForFile returns the first consumer root that is a prefix of filePath.
// Returns "primary" when no root matches.
func repoForFile(filePath string, consumerRoots []string) string {
	for _, root := range consumerRoots {
		if strings.HasPrefix(filePath, root) {
			return root
		}
	}
	return "primary"
}

// HandleGetCrossRepoReferences handles the get_cross_repo_references MCP tool.
// It adds each consumer_root as a workspace folder, calls GetReferences on the
// symbol, then partitions results by which consumer_root prefix they belong to.
func HandleGetCrossRepoReferences(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	// Decode symbol_file (required).
	symbolFile, ok := args["symbol_file"].(string)
	if !ok || symbolFile == "" {
		return types.ErrorResult("symbol_file is required"), nil
	}

	// Decode consumer_roots (required, non-empty) — validated before CheckInitialized
	// so arg errors are reported regardless of client state.
	rawRoots, ok := args["consumer_roots"].([]any)
	if !ok || len(rawRoots) == 0 {
		return types.ErrorResult("consumer_roots must be non-empty; use find_references for single-repo lookup"), nil
	}
	consumerRoots := make([]string, 0, len(rawRoots))
	for _, r := range rawRoots {
		s, ok := r.(string)
		if !ok || s == "" {
			continue
		}
		consumerRoots = append(consumerRoots, s)
	}
	if len(consumerRoots) == 0 {
		return types.ErrorResult("consumer_roots must be non-empty; use find_references for single-repo lookup"), nil
	}

	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Decode line and column (required, 1-indexed).
	line, col, err := extractPosition(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Decode language_id (optional, default "plaintext").
	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	// Add each consumer root as a workspace folder. Continue even if some fail.
	var warnings []string
	for _, root := range consumerRoots {
		if err := client.AddWorkspaceFolder(ctx, root); err != nil {
			warnings = append(warnings, fmt.Sprintf("add_workspace_folder %q: %s", root, err))
		}
	}

	// Call GetReferences via WithDocument.
	locs, wErr := WithDocument[[]types.Location](ctx, client, symbolFile, languageID, func(fURI string) ([]types.Location, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetReferences(ctx, fURI, pos, false)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("find_references: %s", wErr)), nil
	}

	// Try to get the symbol name via hover (separate WithDocument call).
	symbolName := "unknown"
	hoverResult, hErr := WithDocument[string](ctx, client, symbolFile, languageID, func(fURI string) (string, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetInfoOnLocation(ctx, fURI, pos)
	})
	if hErr == nil && hoverResult != "" {
		// Extract first word from hover text as the symbol name.
		word := strings.FieldsFunc(hoverResult, func(r rune) bool {
			return r == ' ' || r == '\n' || r == '\t' || r == '(' || r == ')' || r == '\r'
		})
		if len(word) > 0 {
			symbolName = word[0]
		}
	}

	// Partition locations by consumer root.
	refs := make([]crossRepoRef, 0, len(locs))
	for _, loc := range locs {
		fp, err := URIToFilePath(loc.URI)
		if err != nil {
			continue
		}
		refs = append(refs, crossRepoRef{
			File:   fp,
			Line:   loc.Range.Start.Line + 1,
			Column: loc.Range.Start.Character + 1,
			Repo:   repoForFile(fp, consumerRoots),
		})
	}

	// Count unique repos.
	repoSet := make(map[string]struct{})
	for _, ref := range refs {
		repoSet[ref.Repo] = struct{}{}
	}

	// Build response.
	response := map[string]any{
		"symbol":     symbolName,
		"references": refs,
		"summary":    fmt.Sprintf("Found %d references across %d repos.", len(refs), len(repoSet)),
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	data, mErr := json.Marshal(response)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshal response: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}
