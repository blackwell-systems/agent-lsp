// change_impact.go implements the blast_radius MCP tool for blast-radius
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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
// 16 keeps the gopls request queue saturated without flooding it. gopls has
// limited internal concurrency, so values above ~24 show diminishing returns.
const maxConcurrentRefs = 16

// HandleGetChangeImpact enumerates exported symbols in each changed file via
// GetDocumentSymbols, calls GetReferencesRaw in parallel for each symbol,
// partitions results into test files vs non-test callers, and extracts
// enclosing test function names for test references.
// changeImpactTimeout caps the entire blast_radius operation. Prevents
// indefinite blocking when gopls is slow to index a new workspace.
const changeImpactTimeout = 90 * time.Second

func HandleGetChangeImpact(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Apply overall timeout so the tool never blocks indefinitely.
	ctx, cancel := context.WithTimeout(ctx, changeImpactTimeout)
	defer cancel()

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

	scope := "exported"
	if v, ok := args["scope"].(string); ok && v != "" {
		scope = v
	}

	filter := ""
	if v, ok := args["filter"].(string); ok && v != "" {
		filter = v
	}

	// Phase 1: Collect all exported symbols from all changed files.
	// Only collects top-level exports (functions, types, variables, constants).
	// Struct fields are excluded: they aren't independently callable and their
	// references are noise that inflates the symbol count.
	var allExports []exportedSymbol
	var allDocSymbols []types.DocumentSymbol   // retained for sync-guarded detection
	filesBySymbol := make(map[string]string)   // symbol name -> file path
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
		allDocSymbols = append(allDocSymbols, symbols...)
		for _, sym := range symbols {
			filesBySymbol[sym.Name] = file
		}
		if scope == "all" {
			collectAllSymbols(symbols, file, langID, &allExports, true)
		} else {
			collectExportedSymbols(symbols, file, langID, &allExports, true)
		}
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

	// Per-symbol caller partitioning: each changed symbol gets its own
	// test_callers and non_test_callers lists so agents know which tests
	// cover which specific function/method.
	type symbolWithCallers struct {
		symbolRef
		TestCallers    []symbolRef `json:"test_callers"`
		NonTestCallers []symbolRef `json:"non_test_callers"`
		SyncGuarded    bool        `json:"sync_guarded,omitempty"`
	}
	var symbolsWithCallers []symbolWithCallers

	// Build a set of sync-guarded struct types by scanning the document symbols
	// already fetched in Phase 1. A struct is sync-guarded if it contains a
	// field whose type name includes "Mutex", "RWMutex", "Lock", "synchronized",
	// "Atomic", or "sync.". This is a heuristic across language families.
	syncGuardedTypes := buildSyncGuardedSet(allDocSymbols, filesBySymbol)

	for _, ref := range refResults {
		entry := symbolWithCallers{
			symbolRef: symbolRef{
				Name: ref.Symbol.Name,
				File: ref.Symbol.File,
				Line: ref.Symbol.Line,
			},
			SyncGuarded: isSyncGuardedSymbol(ref.Symbol.Name, syncGuardedTypes),
		}

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
						testRef := symbolRef{
							Name: enclosing.Name,
							File: refPath,
							Line: enclosing.SelectionRange.Start.Line + 1,
						}
						testFunctions = append(testFunctions, testRef)
						entry.TestCallers = append(entry.TestCallers, testRef)
					}
				}
			} else {
				callerRef := symbolRef{
					Name: ref.Symbol.Name,
					File: refPath,
					Line: loc.Range.Start.Line + 1,
				}
				nonTestCallers = append(nonTestCallers, callerRef)
				entry.NonTestCallers = append(entry.NonTestCallers, callerRef)
			}
		}

		changedSymbols = append(changedSymbols, symbolRef{
			Name: ref.Symbol.Name,
			File: ref.Symbol.File,
			Line: ref.Symbol.Line,
		})

		// Apply filter: "untested" keeps only symbols with non-test callers
		// but zero test callers (active in production, no test coverage).
		if filter == "untested" {
			if len(entry.NonTestCallers) == 0 || len(entry.TestCallers) > 0 {
				continue
			}
		}

		symbolsWithCallers = append(symbolsWithCallers, entry)
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
		"affected_symbols": symbolsWithCallers,
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
	impactHint := "Review high-callers symbols before making changes."
	if len(refWarnings) > 0 {
		impactHint = fmt.Sprintf("%d warnings encountered during analysis. %s", len(refWarnings), impactHint)
	}
	return appendHint(types.TextResult(string(data)), impactHint), nil
}

// collectExportedSymbols walks a DocumentSymbol tree and appends exported symbols
// to the provided slice. For Go, only uppercase symbols are exported.
// If recurseIntoChildren is false, struct fields and method children are skipped
// to avoid inflating the symbol count with non-independently-callable members.
func collectExportedSymbols(syms []types.DocumentSymbol, filePath, langID string, out *[]exportedSymbol, recurseIntoChildren bool) {
	// Cache source lines for resolving symbol name positions.
	// gopls returns SelectionRange.Start pointing to the keyword (e.g., "func")
	// rather than the identifier name for functions and methods. We resolve the
	// actual column by finding the symbol name in the source line.
	var sourceLines []string
	if data, err := os.ReadFile(filePath); err == nil {
		sourceLines = strings.Split(string(data), "\n")
	}

	for _, sym := range syms {
		// Skip struct fields (kind 8): they're not independently callable and
		// inflate the symbol count without adding blast-radius value.
		if sym.Kind == 8 {
			continue
		}
		// For Go, check if the symbol is exported. Method names from gopls
		// include the receiver prefix (e.g., "(*Hub).SetSender"), so extract
		// the actual name after the last dot before checking case.
		exportName := sym.Name
		if dotIdx := strings.LastIndex(exportName, "."); dotIdx >= 0 {
			exportName = exportName[dotIdx+1:]
		}
		exported := langID != "go" || (len(exportName) > 0 && exportName[0] >= 'A' && exportName[0] <= 'Z')
		if exported {
			line := sym.SelectionRange.Start.Line
			char := sym.SelectionRange.Start.Character

			// Resolve actual column of the symbol name in the source line.
			// For methods, strip the receiver prefix (e.g., "(*LSPClient).Shutdown" -> "Shutdown").
			searchName := sym.Name
			if dotIdx := strings.LastIndex(searchName, "."); dotIdx >= 0 {
				searchName = searchName[dotIdx+1:]
			}
			if line < len(sourceLines) {
				if col := strings.Index(sourceLines[line], searchName); col >= 0 {
					char = col
				}
			}

			*out = append(*out, exportedSymbol{
				Name:   sym.Name,
				File:   filePath,
				LangID: langID,
				Position: types.Position{
					Line:      line,
					Character: char,
				},
				Line: line + 1,
			})
		}
		if recurseIntoChildren {
			collectExportedSymbols(sym.Children, filePath, langID, out, true)
		}
	}
}

// collectAllSymbols walks a DocumentSymbol tree and appends ALL symbols
// (exported and unexported) to the provided slice. Used when scope="all"
// is requested for dead code detection of internal helpers.
// Struct fields (kind 8) are still excluded.
func collectAllSymbols(syms []types.DocumentSymbol, filePath, langID string, out *[]exportedSymbol, recurseIntoChildren bool) {
	var sourceLines []string
	if data, err := os.ReadFile(filePath); err == nil {
		sourceLines = strings.Split(string(data), "\n")
	}

	for _, sym := range syms {
		if sym.Kind == 8 {
			continue
		}
		line := sym.SelectionRange.Start.Line
		char := sym.SelectionRange.Start.Character

		searchName := sym.Name
		if dotIdx := strings.LastIndex(searchName, "."); dotIdx >= 0 {
			searchName = searchName[dotIdx+1:]
		}
		if line < len(sourceLines) {
			if col := strings.Index(sourceLines[line], searchName); col >= 0 {
				char = col
			}
		}

		*out = append(*out, exportedSymbol{
			Name:   sym.Name,
			File:   filePath,
			LangID: langID,
			Position: types.Position{
				Line:      line,
				Character: char,
			},
			Line: line + 1,
		})
		if recurseIntoChildren {
			collectAllSymbols(sym.Children, filePath, langID, out, true)
		}
	}
}

// perSymbolTimeout caps how long a single reference query can take.
// Prevents one slow symbol from blocking the entire batch. Symbols that
// exceed this are skipped with a warning.
const perSymbolTimeout = 15 * time.Second

// queryReferencesParallel queries GetReferencesRaw for all symbols using a
// worker pool. Checks the persistent reference cache first; only queries the
// language server for cache misses. The caller must ensure the workspace is
// warm before calling (e.g. by doing one blocking GetReferences call first).
func queryReferencesParallel(ctx context.Context, client *lsp.LSPClient, symbols []exportedSymbol) []symbolRefs {
	results := make([]symbolRefs, len(symbols))
	cache := client.RefCache()
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

			// Check cache first.
			if cached := cache.Get(s.File, s.Name, s.Line); cached != nil {
				results[idx] = symbolRefs{Symbol: s, Locs: cached.Locations}
				return
			}

			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Per-symbol timeout prevents one slow query from blocking the batch.
			symCtx, cancel := context.WithTimeout(ctx, perSymbolTimeout)
			defer cancel()

			locs, err := WithDocument[[]types.Location](symCtx, client, s.File, s.LangID, func(fURI string) ([]types.Location, error) {
				return client.GetReferencesRaw(symCtx, fURI, s.Position, false)
			})

			ref := symbolRefs{Symbol: s, Locs: locs}
			if err != nil {
				if symCtx.Err() == context.DeadlineExceeded {
					ref.Warning = fmt.Sprintf("warning: GetReferences timed out for %s in %s after %s", s.Name, s.File, perSymbolTimeout)
				} else {
					ref.Warning = fmt.Sprintf("warning: GetReferences failed for %s in %s: %s", s.Name, s.File, err)
				}
			} else {
				// Cache successful results.
				cache.Put(s.File, s.Name, s.Line, locs)
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

// syncFieldPatterns contains substrings that indicate a field provides
// synchronization. Language-agnostic: covers Go (sync.Mutex, sync.RWMutex),
// Java/Kotlin (ReentrantLock, synchronized), Rust (Mutex), Python (Lock),
// C/C++ (pthread_mutex, std::mutex), and atomic types.
var syncFieldPatterns = []string{
	"Mutex", "RWMutex", "Lock", "Semaphore",
	"atomic", "Atomic",
	"sync.", "pthread_mutex", "std::mutex",
}

// buildSyncGuardedSet scans document symbols and returns a set of type names
// (structs, classes) that contain at least one synchronization field.
// Uses two strategies: (1) check document symbol children if the LSP provides
// them (TypeScript, Java), (2) read the struct's source lines and pattern-match
// for sync primitives (Go/gopls doesn't nest fields as children).
func buildSyncGuardedSet(symbols []types.DocumentSymbol, filesBySymbol map[string]string) map[string]bool {
	guarded := make(map[string]bool)
	for _, sym := range symbols {
		// Check structs/classes (kind 23=struct, 5=class)
		if sym.Kind == 23 || sym.Kind == 5 {
			// Strategy 1: check children (works for TSServer, jdtls, etc.)
			for _, child := range sym.Children {
				if child.Kind == 8 || child.Kind == 7 {
					for _, pattern := range syncFieldPatterns {
						if strings.Contains(child.Name, pattern) || strings.Contains(child.Detail, pattern) {
							guarded[sym.Name] = true
							break
						}
					}
				}
				if guarded[sym.Name] {
					break
				}
			}

			// Strategy 2: read source lines of the struct (for gopls which
			// doesn't nest fields as children).
			if !guarded[sym.Name] {
				if filePath, ok := filesBySymbol[sym.Name]; ok {
					if data, err := os.ReadFile(filePath); err == nil {
						lines := strings.Split(string(data), "\n")
						start := sym.Range.Start.Line
						end := sym.Range.End.Line
						if end >= len(lines) {
							end = len(lines) - 1
						}
						for i := start; i <= end; i++ {
							for _, pattern := range syncFieldPatterns {
								if strings.Contains(lines[i], pattern) {
									guarded[sym.Name] = true
									break
								}
							}
							if guarded[sym.Name] {
								break
							}
						}
					}
				}
			}
		}
		// Recurse into children (nested types)
		for k, v := range buildSyncGuardedSet(sym.Children, filesBySymbol) {
			if v {
				guarded[k] = true
			}
		}
	}
	return guarded
}

// isSyncGuardedSymbol checks if a symbol name refers to a method on a
// sync-guarded type. Go methods from gopls include the receiver prefix
// (e.g., "(*Hub).SetSender"); this extracts the type name and checks
// against the guarded set.
func isSyncGuardedSymbol(name string, guardedTypes map[string]bool) bool {
	// Direct match (the symbol IS a guarded type)
	if guardedTypes[name] {
		return true
	}
	// Method receiver: "(*TypeName).Method" or "TypeName.Method"
	if dotIdx := strings.LastIndex(name, "."); dotIdx >= 0 {
		receiver := name[:dotIdx]
		// Strip pointer receiver syntax: "(*TypeName)" -> "TypeName"
		receiver = strings.TrimPrefix(receiver, "(*")
		receiver = strings.TrimPrefix(receiver, "(")
		receiver = strings.TrimSuffix(receiver, ")")
		if guardedTypes[receiver] {
			return true
		}
	}
	return false
}
