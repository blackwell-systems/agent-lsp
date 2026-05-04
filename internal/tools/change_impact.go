// change_impact.go implements the get_change_impact MCP tool for blast-radius
// analysis. Given a list of changed files, it:
//
//  1. Opens each file and retrieves its exported symbols (GetDocumentSymbols).
//  2. Warms the workspace with one blocking reference query.
//  3. Calls GetReferencesRaw for each exported symbol IN PARALLEL.
//  4. Partitions callers into test files vs non-test callers.
//  5. Extracts enclosing test function names for test references.
//
// The result tells the agent which code paths are affected by the change,
// enabling informed decisions about whether to proceed with an edit or halt
// due to excessive blast radius.
//
// Optionally, include_transitive follows one additional level of indirection:
// for each non-test caller, find its callers too.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

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

// symbolRef is a reference to a named symbol at a file location.
type symbolRef struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// exportedSymbol holds a symbol and its position for batch reference queries.
type exportedSymbol struct {
	Name     string
	File     string
	LangID   string
	Position types.Position
	Line     int // 1-indexed for output
}

// symbolRefs holds the references found for a single symbol.
type symbolRefs struct {
	Symbol  exportedSymbol
	Locs    []types.Location
	Warning string
}

// maxConcurrentRefs is the worker pool size for parallel reference queries.
const maxConcurrentRefs = 8

// HandleGetChangeImpact enumerates exported symbols in each changed file via
// GetDocumentSymbols, calls GetReferencesRaw in parallel for each symbol,
// partitions results into test files vs non-test callers, and extracts
// enclosing test function names for test references.
func HandleGetChangeImpact(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Decode changed_files (arrives as []any from JSON).
	rawFiles, ok := args["changed_files"].([]any)
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

	// Phase 1: Collect all exported symbols from all changed files.
	// Only collects top-level exports (functions, types, variables, constants).
	// Struct fields are excluded: they aren't independently callable and their
	// references are noise that inflates the symbol count.
	var allExports []exportedSymbol
	var warnings []string

	for _, file := range changedFiles {
		langID := lsp.LanguageIDFromPath(file)
		symbols, err := WithDocument[[]types.DocumentSymbol](ctx, client, file, langID, func(fURI string) ([]types.DocumentSymbol, error) {
			return client.GetDocumentSymbols(ctx, fURI)
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("warning: could not get symbols for %s: %s", file, err))
			continue
		}
		collectExportedSymbols(symbols, file, langID, &allExports, false)
	}

	// Phase 1.5: Warmup. The first reference query on a cold workspace forces
	// the language server to complete its full package/module load. Subsequent
	// queries are fast. We do one blocking query (with full WaitForFileIndexed)
	// on the first symbol to absorb the cold-start cost. The result is kept
	// and used in Phase 2 so we don't re-query the same symbol.
	var warmupResult *symbolRefs
	if len(allExports) > 0 {
		first := allExports[0]
		locs, err := WithDocument[[]types.Location](ctx, client, first.File, first.LangID, func(fURI string) ([]types.Location, error) {
			return client.GetReferences(ctx, fURI, first.Position, false)
		})
		ref := symbolRefs{Symbol: first, Locs: locs}
		if err != nil {
			ref.Warning = fmt.Sprintf("warning: GetReferences failed for %s in %s: %s", first.Name, first.File, err)
		}
		warmupResult = &ref
	}

	// Phase 2: Query references for remaining symbols in parallel.
	// Skip the first symbol (already queried in warmup).
	remaining := allExports
	if len(remaining) > 0 {
		remaining = remaining[1:]
	}
	refResults := queryReferencesParallel(ctx, client, remaining)

	// Prepend the warmup result so all symbols are in order.
	if warmupResult != nil {
		refResults = append([]symbolRefs{*warmupResult}, refResults...)
	}

	// Phase 3: Partition results into test vs non-test callers.
	var changedSymbols []symbolRef
	testFilesSet := map[string]bool{}
	testFuncSet := map[string]bool{} // dedup key: "file:name"
	var testFunctions []symbolRef
	var nonTestCallers []symbolRef
	var refWarnings []string

	// Cache for test file symbols to avoid redundant GetDocumentSymbols calls.
	testSymbolCache := &sync.Map{}

	for _, ref := range refResults {
		changedSymbols = append(changedSymbols, symbolRef{
			Name: ref.Symbol.Name,
			File: ref.Symbol.File,
			Line: ref.Symbol.Line,
		})

		if ref.Warning != "" {
			refWarnings = append(refWarnings, ref.Warning)
		}

		for _, loc := range ref.Locs {
			refPath, err := URIToFilePath(loc.URI)
			if err != nil {
				continue
			}
			if isTestFile(refPath) {
				testFilesSet[refPath] = true
				enclosing := findEnclosingTestFunction(ctx, client, testSymbolCache, refPath, loc.Range.Start.Line)
				if enclosing != nil {
					key := fmt.Sprintf("%s:%s", refPath, enclosing.Name)
					if !testFuncSet[key] {
						testFuncSet[key] = true
						testFunctions = append(testFunctions, symbolRef{
							Name: enclosing.Name,
							File: refPath,
							Line: enclosing.SelectionRange.Start.Line + 1,
						})
					}
				}
			} else {
				nonTestCallers = append(nonTestCallers, symbolRef{
					Name: ref.Symbol.Name,
					File: refPath,
					Line: loc.Range.Start.Line + 1,
				})
			}
		}
	}

	// Phase 3.5: Transitive references (if requested).
	// Batched and parallelized like Phase 2.
	if includeTransitive && len(nonTestCallers) > 0 {
		var transitiveSymbols []exportedSymbol
		for _, caller := range nonTestCallers {
			transitiveSymbols = append(transitiveSymbols, exportedSymbol{
				Name:   caller.Name,
				File:   caller.File,
				LangID: lsp.LanguageIDFromPath(caller.File),
				Position: types.Position{
					Line:      caller.Line - 1, // convert back to 0-indexed
					Character: 0,
				},
				Line: caller.Line,
			})
		}
		transitiveResults := queryReferencesParallel(ctx, client, transitiveSymbols)
		for _, ref := range transitiveResults {
			for _, loc := range ref.Locs {
				tPath, tErr := URIToFilePath(loc.URI)
				if tErr != nil {
					continue
				}
				if isTestFile(tPath) {
					testFilesSet[tPath] = true
				}
			}
		}
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

	response := map[string]any{
		"changed_symbols":  changedSymbols,
		"test_files":       testFiles,
		"test_functions":   testFunctions,
		"non_test_callers": nonTestCallers,
		"summary":          summary,
		"warnings":         refWarnings,
	}

	data, err := json.Marshal(response)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling response: %s", err)), nil
	}
	return types.TextResult(string(data)), nil
}

// collectExportedSymbols walks a DocumentSymbol tree and appends exported symbols
// to the provided slice. For Go, only uppercase symbols are exported.
// If recurseIntoChildren is false, struct fields and method children are skipped
// to avoid inflating the symbol count with non-independently-callable members.
func collectExportedSymbols(syms []types.DocumentSymbol, filePath, langID string, out *[]exportedSymbol, recurseIntoChildren bool) {
	for _, sym := range syms {
		exported := langID != "go" || (len(sym.Name) > 0 && sym.Name[0] >= 'A' && sym.Name[0] <= 'Z')
		if exported {
			*out = append(*out, exportedSymbol{
				Name:   sym.Name,
				File:   filePath,
				LangID: langID,
				Position: types.Position{
					Line:      sym.SelectionRange.Start.Line,
					Character: sym.SelectionRange.Start.Character,
				},
				Line: sym.SelectionRange.Start.Line + 1,
			})
		}
		if recurseIntoChildren {
			collectExportedSymbols(sym.Children, filePath, langID, out, true)
		}
	}
}

// queryReferencesParallel queries GetReferencesRaw for all symbols using a
// worker pool. The caller must ensure the workspace is warm before calling
// (e.g. by doing one blocking GetReferences call first).
func queryReferencesParallel(ctx context.Context, client *lsp.LSPClient, symbols []exportedSymbol) []symbolRefs {
	results := make([]symbolRefs, len(symbols))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentRefs)

	for i, sym := range symbols {
		wg.Add(1)
		go func(idx int, s exportedSymbol) {
			defer wg.Done()

			// Respect context cancellation.
			if ctx.Err() != nil {
				results[idx] = symbolRefs{Symbol: s, Warning: "context cancelled"}
				return
			}

			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			locs, err := WithDocument[[]types.Location](ctx, client, s.File, s.LangID, func(fURI string) ([]types.Location, error) {
				return client.GetReferencesRaw(ctx, fURI, s.Position, false)
			})

			ref := symbolRefs{Symbol: s, Locs: locs}
			if err != nil {
				ref.Warning = fmt.Sprintf("warning: GetReferences failed for %s in %s: %s", s.Name, s.File, err)
			}
			results[idx] = ref
		}(i, sym)
	}

	wg.Wait()
	return results
}

// findEnclosingTestFunction finds the enclosing test function for a reference
// in a test file, with caching to avoid redundant GetDocumentSymbols calls.
func findEnclosingTestFunction(ctx context.Context, client *lsp.LSPClient, cache *sync.Map, refPath string, line int) *types.DocumentSymbol {
	// Check cache first.
	if cached, ok := cache.Load(refPath); ok {
		if syms, ok := cached.([]types.DocumentSymbol); ok {
			return findEnclosingSymbol(syms, line)
		}
		return nil
	}

	// Query and cache.
	syms, err := WithDocument[[]types.DocumentSymbol](ctx, client, refPath, lsp.LanguageIDFromPath(refPath), func(fURI string) ([]types.DocumentSymbol, error) {
		return client.GetDocumentSymbols(ctx, fURI)
	})
	if err != nil {
		cache.Store(refPath, []types.DocumentSymbol{})
		return nil
	}
	cache.Store(refPath, syms)
	return findEnclosingSymbol(syms, line)
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
