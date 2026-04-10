package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// isTestFile returns true if the given path looks like a test file.
func isTestFile(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return true
	}
	if strings.Contains(path, ".test.") || strings.Contains(path, ".spec.") {
		return true
	}
	if strings.HasPrefix(filepath.Base(path), "test_") {
		return true
	}
	return false
}

// langIDFromPath maps a file extension to an LSP language ID.
func langIDFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	default:
		return "plaintext"
	}
}

// symbolRef is a reference to a named symbol at a file location.
type symbolRef struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// HandleGetChangeImpact enumerates exported symbols in each changed file via
// GetDocumentSymbols, calls GetReferences for each symbol, partitions results
// into test files vs non-test callers, and extracts enclosing test function
// names for test references.
func HandleGetChangeImpact(ctx context.Context, client *lsp.LSPClient, args map[string]interface{}) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Decode changed_files (arrives as []interface{} from JSON).
	rawFiles, ok := args["changed_files"].([]interface{})
	if !ok || len(rawFiles) == 0 {
		return types.ErrorResult("changed_files is required"), nil
	}
	changedFiles := make([]string, 0, len(rawFiles))
	for _, v := range rawFiles {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		changedFiles = append(changedFiles, s)
	}
	if len(changedFiles) == 0 {
		return types.ErrorResult("changed_files is required"), nil
	}

	includeTransitive := false
	if v, ok := args["include_transitive"].(bool); ok {
		includeTransitive = v
	}

	var changedSymbols []symbolRef
	testFilesSet := map[string]bool{}
	var testFunctions []symbolRef
	var nonTestCallers []symbolRef

	// collectExported walks a DocumentSymbol tree and collects exported symbols.
	// For Go, only symbols whose first character is uppercase are exported.
	// For other languages, all symbols are included.
	var collectExported func(syms []types.DocumentSymbol, filePath, langID string)
	collectExported = func(syms []types.DocumentSymbol, filePath, langID string) {
		for _, sym := range syms {
			exported := langID != "go" || (len(sym.Name) > 0 && sym.Name[0] >= 'A' && sym.Name[0] <= 'Z')
			if exported {
				changedSymbols = append(changedSymbols, symbolRef{
					Name: sym.Name,
					File: filePath,
					Line: sym.SelectionRange.Start.Line + 1,
				})

				// Get references for this exported symbol.
				pos := types.Position{
					Line:      sym.SelectionRange.Start.Line,
					Character: sym.SelectionRange.Start.Character,
				}
				locs, _ := WithDocument[[]types.Location](ctx, client, filePath, langID, func(fURI string) ([]types.Location, error) {
					return client.GetReferences(ctx, fURI, pos, false)
				})

				for _, loc := range locs {
					refPath, err := URIToFilePath(loc.URI)
					if err != nil {
						continue
					}
					if isTestFile(refPath) {
						testFilesSet[refPath] = true
						// Find enclosing function in the test file.
						refSyms, sErr := WithDocument[[]types.DocumentSymbol](ctx, client, refPath, langIDFromPath(refPath), func(fURI string) ([]types.DocumentSymbol, error) {
							return client.GetDocumentSymbols(ctx, fURI)
						})
						if sErr == nil {
							enclosing := findEnclosingSymbol(refSyms, loc.Range.Start.Line)
							if enclosing != nil {
								testFunctions = append(testFunctions, symbolRef{
									Name: enclosing.Name,
									File: refPath,
									Line: enclosing.SelectionRange.Start.Line + 1,
								})
							}
						}
					} else {
						nonTestCallers = append(nonTestCallers, symbolRef{
							Name: sym.Name,
							File: refPath,
							Line: loc.Range.Start.Line + 1,
						})

						// Transitive: find test files that reference this non-test caller.
						if includeTransitive {
							transitivePos := types.Position{
								Line:      loc.Range.Start.Line,
								Character: loc.Range.Start.Character,
							}
							transLocs, _ := WithDocument[[]types.Location](ctx, client, refPath, langIDFromPath(refPath), func(fURI string) ([]types.Location, error) {
								return client.GetReferences(ctx, fURI, transitivePos, false)
							})
							for _, tLoc := range transLocs {
								tPath, tErr := URIToFilePath(tLoc.URI)
								if tErr != nil {
									continue
								}
								if isTestFile(tPath) {
									testFilesSet[tPath] = true
								}
							}
						}
					}
				}
			}
			// Recurse into children regardless of export status.
			collectExported(sym.Children, filePath, langID)
		}
	}

	var warnings []string
	for _, file := range changedFiles {
		langID := langIDFromPath(file)
		symbols, err := WithDocument[[]types.DocumentSymbol](ctx, client, file, langID, func(fURI string) ([]types.DocumentSymbol, error) {
			return client.GetDocumentSymbols(ctx, fURI)
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("warning: could not get symbols for %s: %s", file, err))
			continue
		}
		collectExported(symbols, file, langID)
	}

	// Build testFiles slice from the set.
	testFiles := make([]string, 0, len(testFilesSet))
	for f := range testFilesSet {
		testFiles = append(testFiles, f)
	}

	// Build summary.
	summary := fmt.Sprintf("Found %d changed symbols with %d test references across %d test files.",
		len(changedSymbols), len(testFunctions), len(testFiles))
	if len(warnings) > 0 {
		summary += " Warnings: " + strings.Join(warnings, "; ")
	}

	response := map[string]interface{}{
		"changed_symbols":  changedSymbols,
		"test_files":       testFiles,
		"test_functions":   testFunctions,
		"non_test_callers": nonTestCallers,
		"summary":          summary,
	}

	data, err := json.Marshal(response)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling response: %s", err)), nil
	}
	return types.TextResult(string(data)), nil
}

// findEnclosingSymbol walks a DocumentSymbol tree and returns the smallest symbol
// whose range contains lineNum (0-indexed). Returns nil if none found.
func findEnclosingSymbol(syms []types.DocumentSymbol, lineNum int) *types.DocumentSymbol {
	var best *types.DocumentSymbol
	for i := range syms {
		sym := &syms[i]
		if sym.Range.Start.Line <= lineNum && lineNum <= sym.Range.End.Line {
			size := sym.Range.End.Line - sym.Range.Start.Line
			if best == nil || size < (best.Range.End.Line-best.Range.Start.Line) {
				best = sym
			}
			// Check children for a tighter fit.
			if child := findEnclosingSymbol(sym.Children, lineNum); child != nil {
				childSize := child.Range.End.Line - child.Range.Start.Line
				if best == nil || childSize < (best.Range.End.Line-best.Range.Start.Line) {
					best = child
				}
			}
		}
	}
	return best
}
