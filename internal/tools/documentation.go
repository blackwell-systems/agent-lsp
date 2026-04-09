package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// DocResult is the JSON-serializable response returned by HandleGetSymbolDocumentation.
type DocResult struct {
	Symbol    string `json:"symbol"`
	Language  string `json:"language"`
	Source    string `json:"source"`    // "toolchain" or "error"
	Doc       string `json:"doc"`       // full documentation text (ANSI-stripped)
	Signature string `json:"signature"` // extracted signature line, may be empty
	Error     string `json:"error,omitempty"` // set when Source == "error"
}

// reANSI matches ANSI escape codes for stripping from command output.
var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// docDispatcher holds the command and args template for a language's doc tool.
type docDispatcher struct {
	cmd  string
	args []string // template; "{pkg_or_symbol}" and "{symbol}" are replaced at runtime
}

// docDispatchers maps language IDs to their documentation commands.
var docDispatchers = map[string]docDispatcher{
	"go": {
		cmd:  "go",
		args: []string{"doc", "{pkg_or_symbol}"},
	},
	"rust": {
		cmd:  "cargo",
		args: []string{"doc", "--no-deps", "--message-format", "short"},
	},
	"python": {
		cmd:  "python3",
		args: []string{"-m", "pydoc", "{symbol}"},
	},
}

// findGoMod walks up from dir looking for go.mod and returns (moduleRoot, moduleName).
func findGoMod(dir string) (root string, moduleName string) {
	current := dir
	for {
		gomod := filepath.Join(current, "go.mod")
		data, err := os.ReadFile(gomod)
		if err == nil {
			// Parse module name from go.mod
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					moduleName = strings.TrimSpace(strings.TrimPrefix(line, "module "))
					return current, moduleName
				}
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", ""
}

// buildGoArgs constructs the argument list for go doc given symbol and optional file_path.
func buildGoArgs(symbol, filePath string) []string {
	if filePath == "" {
		return []string{"doc", symbol}
	}
	dir := filepath.Dir(filePath)
	modRoot, modName := findGoMod(dir)
	if modRoot == "" || modName == "" {
		// No go.mod found; fall back to plain symbol
		return []string{"doc", symbol}
	}
	rel, err := filepath.Rel(modRoot, dir)
	if err != nil || rel == "." {
		return []string{"doc", symbol}
	}
	pkgPath := modName + "/" + rel
	return []string{"doc", pkgPath, symbol}
}

// extractSignature extracts the first signature-like line from doc output for a given language.
func extractSignature(language, output string) string {
	switch language {
	case "go":
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "func ") ||
				strings.HasPrefix(trimmed, "type ") ||
				strings.HasPrefix(trimmed, "var ") ||
				strings.HasPrefix(trimmed, "const ") {
				return trimmed
			}
		}
	case "python":
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				return trimmed
			}
		}
	case "rust":
		// cargo doc output is HTML-oriented; leave empty
	}
	return ""
}

// HandleGetSymbolDocumentation is the MCP handler for get_symbol_documentation.
// It dispatches to per-language toolchain commands to fetch canonical documentation.
// No *lsp.LSPClient parameter — runs shell commands independently of any LSP session.
func HandleGetSymbolDocumentation(ctx context.Context, args map[string]interface{}) (types.ToolResult, error) {
	symbol, _ := args["symbol"].(string)
	languageID, _ := args["language_id"].(string)
	filePath, _ := args["file_path"].(string)
	format, _ := args["format"].(string)

	if symbol == "" {
		return types.ErrorResult("symbol is required"), nil
	}
	if languageID == "" {
		return types.ErrorResult("language_id is required"), nil
	}

	// Check for explicitly unsupported languages
	switch languageID {
	case "typescript", "javascript":
		result := DocResult{
			Symbol:   symbol,
			Language: languageID,
			Source:   "error",
			Error:    fmt.Sprintf("unsupported language: %s", languageID),
		}
		data, err := json.Marshal(result)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("marshal error: %v", err)), nil
		}
		return types.TextResult(string(data)), nil
	}

	dispatcher, ok := docDispatchers[languageID]
	if !ok {
		result := DocResult{
			Symbol:   symbol,
			Language: languageID,
			Source:   "error",
			Error:    fmt.Sprintf("unsupported language: %s", languageID),
		}
		data, err := json.Marshal(result)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("marshal error: %v", err)), nil
		}
		return types.TextResult(string(data)), nil
	}

	// Build command args
	var cmdArgs []string
	if languageID == "go" {
		cmdArgs = buildGoArgs(symbol, filePath)
	} else {
		for _, a := range dispatcher.args {
			a = strings.ReplaceAll(a, "{pkg_or_symbol}", symbol)
			a = strings.ReplaceAll(a, "{symbol}", symbol)
			cmdArgs = append(cmdArgs, a)
		}
	}

	// Determine working directory
	cmdDir := ""
	if filePath != "" {
		dir := filepath.Dir(filePath)
		modRoot, _ := findGoMod(dir)
		if modRoot != "" {
			cmdDir = modRoot
		} else {
			cmdDir = dir
		}
	}

	// Run command with 10-second timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*1e9) // 10 seconds in nanoseconds
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, dispatcher.cmd, cmdArgs...)
	if cmdDir != "" {
		cmd.Dir = cmdDir
	}

	raw, execErr := cmd.CombinedOutput()
	rawStr := reANSI.ReplaceAllString(string(raw), "")

	if execErr != nil {
		result := DocResult{
			Symbol:   symbol,
			Language: languageID,
			Source:   "error",
			Error:    strings.TrimSpace(rawStr),
		}
		if result.Error == "" {
			result.Error = execErr.Error()
		}
		data, err := json.Marshal(result)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("marshal error: %v", err)), nil
		}
		return types.TextResult(string(data)), nil
	}

	sig := extractSignature(languageID, rawStr)

	// Apply format wrapping to signature
	formattedSig := sig
	if format == "markdown" && sig != "" {
		formattedSig = fmt.Sprintf("```%s\n%s\n```", languageID, sig)
	}

	result := DocResult{
		Symbol:    symbol,
		Language:  languageID,
		Source:    "toolchain",
		Doc:       strings.TrimSpace(rawStr),
		Signature: formattedSig,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshal error: %v", err)), nil
	}
	return types.TextResult(string(data)), nil
}
