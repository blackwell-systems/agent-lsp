// detect_changes.go implements the detect_changes MCP tool. It runs git diff
// to identify changed files, feeds them to the existing get_change_impact
// logic, and returns affected symbols with risk classification.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// gitDiffArgs returns the git diff arguments for the given scope.
func gitDiffArgs(scope string) []string {
	switch scope {
	case "staged":
		return []string{"diff", "--name-only", "--cached"}
	case "committed":
		return []string{"diff", "--name-only", "HEAD~1", "HEAD"}
	default: // "unstaged" or empty
		return []string{"diff", "--name-only"}
	}
}

// filterChangedFiles keeps only files that exist on disk and have a recognized
// language (LanguageIDFromPath returns something other than "plaintext").
func filterChangedFiles(root string, paths []string) []string {
	var out []string
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(root, p)
		}
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		if lsp.LanguageIDFromPath(abs) == "plaintext" {
			continue
		}
		out = append(out, abs)
	}
	return out
}

// classifyRisk assigns a risk level to a symbol based on its non-test callers.
//   - "high": callers from multiple packages
//   - "medium": callers from the same package only
//   - "low": zero non-test callers
func classifyRisk(nonTestCallers []symbolRef, symbolFile string) string {
	if len(nonTestCallers) == 0 {
		return "low"
	}
	symbolDir := filepath.Dir(symbolFile)
	for _, caller := range nonTestCallers {
		if filepath.Dir(caller.File) != symbolDir {
			return "high"
		}
	}
	return "medium"
}

// HandleDetectChanges runs git diff for the given scope, filters the results,
// delegates to HandleGetChangeImpact, and enriches the response with per-symbol
// risk classification.
func HandleDetectChanges(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Parse workspace_root; fall back to client.RootDir().
	workspaceRoot, _ := args["workspace_root"].(string)
	if workspaceRoot == "" {
		workspaceRoot = client.RootDir()
	}
	if workspaceRoot == "" {
		return types.ErrorResult("workspace_root is required when no LSP root is set"), nil
	}

	// Parse scope (default "unstaged").
	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "unstaged"
	}
	if scope != "unstaged" && scope != "staged" && scope != "committed" {
		return types.ErrorResult(fmt.Sprintf("invalid scope %q: must be unstaged, staged, or committed", scope)), nil
	}

	// Run git diff.
	gitArgs := gitDiffArgs(scope)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = workspaceRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return types.ErrorResult(fmt.Sprintf("git diff failed: %s: %s", err, stderr.String())), nil
	}

	// Parse output into file list.
	raw := strings.TrimSpace(stdout.String())
	if raw == "" {
		return types.TextResult(`{"changed_files":[],"affected_symbols":[],"summary":"No changed files detected."}`), nil
	}
	lines := strings.Split(raw, "\n")
	filtered := filterChangedFiles(workspaceRoot, lines)
	if len(filtered) == 0 {
		return types.TextResult(`{"changed_files":[],"affected_symbols":[],"summary":"No recognized source files among changed files."}`), nil
	}

	// Delegate to get_change_impact.
	impactArgs := map[string]any{
		"changed_files": toAnySlice(filtered),
	}
	impactResult, err := HandleGetChangeImpact(ctx, client, impactArgs)
	if err != nil {
		return impactResult, err
	}
	if impactResult.IsError {
		return impactResult, nil
	}

	// Parse the impact result to enrich with risk classification.
	var impactData map[string]any
	if len(impactResult.Content) > 0 {
		if jsonErr := json.Unmarshal([]byte(impactResult.Content[0].Text), &impactData); jsonErr != nil {
			// If we can't parse, return the raw impact result.
			return impactResult, nil
		}
	}

	// Enrich each changed symbol with a risk field.
	enrichSymbols(impactData)

	// Add changed_files to the response.
	impactData["changed_files"] = filtered
	impactData["scope"] = scope

	data, err := json.Marshal(impactData)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling response: %s", err)), nil
	}
	return appendHint(types.TextResult(string(data)), "Review high-risk symbols before committing. Use get_change_impact on specific files for detailed analysis."), nil
}

// enrichSymbols adds a "risk" field to each entry in changed_symbols based on
// which non_test_callers reference that symbol.
func enrichSymbols(impactData map[string]any) {
	changedSymbols, _ := impactData["changed_symbols"].([]any)
	nonTestCallers, _ := impactData["non_test_callers"].([]any)

	// Index non-test callers by symbol name.
	callersBySymbol := map[string][]symbolRef{}
	for _, raw := range nonTestCallers {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := c["name"].(string)
		file, _ := c["file"].(string)
		line, _ := c["line"].(float64)
		callersBySymbol[name] = append(callersBySymbol[name], symbolRef{
			Name: name,
			File: file,
			Line: int(line),
		})
	}

	for _, raw := range changedSymbols {
		sym, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := sym["name"].(string)
		file, _ := sym["file"].(string)
		callers := callersBySymbol[name]
		sym["risk"] = classifyRisk(callers, file)
	}
}

// toAnySlice converts a []string to []any for use in args maps.
func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
