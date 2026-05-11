// explore.go implements the explore_symbol composite tool handler.
// It combines hover info, call hierarchy, document symbols (source), and
// references into a single response for deep-dive symbol exploration.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// exploreResult is the JSON response structure for explore_symbol.
type exploreResult struct {
	TypeInfo         string              `json:"type_info"`
	Source           *exploreSource      `json:"source,omitempty"`
	Callers          []exploreCaller     `json:"callers"`
	CallersCount     int                 `json:"callers_count"`
	References       exploreReferences   `json:"references"`
	TestCallersCount int                 `json:"test_callers_count"`
}

type exploreSource struct {
	SymbolName string `json:"symbol_name"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Source     string `json:"source"`
}

type exploreCaller struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type exploreReferences struct {
	Count    int      `json:"count"`
	TopFiles []string `json:"top_files"`
}

// HandleExploreSymbol orchestrates four LSP queries (hover, call hierarchy,
// document symbols, references) and returns a unified deep-dive result.
func HandleExploreSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	cleanPath, pathErr := ValidateFilePath(filePath, client.RootDir())
	if pathErr != nil {
		return types.ErrorResult(pathErr.Error()), nil
	}

	line, col, posErr := ExtractPositionWithPattern(args, cleanPath)
	if posErr != nil {
		return types.ErrorResult(posErr.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	pos := types.Position{Line: line - 1, Character: col - 1}

	// 1. Hover info (type_info)
	typeInfo, wErr := WithDocument[string](ctx, client, cleanPath, languageID, func(fileURI string) (string, error) {
		return client.GetInfoOnLocation(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("explore_symbol (hover): %s", wErr)), nil
	}

	// 2. Call hierarchy incoming (callers)
	fileURI := CreateFileURI(cleanPath)
	items, _ := client.PrepareCallHierarchy(ctx, fileURI, pos)

	var callers []exploreCaller
	var testCallersCount int
	for _, item := range items {
		incoming, _ := client.GetIncomingCalls(ctx, item)
		for _, call := range incoming {
			fp, fpErr := URIToFilePath(call.From.URI)
			if fpErr != nil {
				fp = call.From.URI
			}
			callers = append(callers, exploreCaller{
				Name: call.From.Name,
				File: fp,
				Line: call.From.Range.Start.Line + 1,
			})
			if isTestFile(fp) {
				testCallersCount++
			}
		}
	}
	callersCount := len(callers)
	// Limit callers to top 10
	if len(callers) > 10 {
		callers = callers[:10]
	}

	// 3. Document symbols + innermost symbol (source)
	var source *exploreSource
	symbols, symErr := client.GetDocumentSymbols(ctx, fileURI)
	if symErr == nil && len(symbols) > 0 {
		sym := findInnermostSymbol(symbols, line-1, col-1)
		if sym != nil {
			content, readErr := os.ReadFile(cleanPath)
			if readErr == nil {
				lines := strings.Split(string(content), "\n")
				start := sym.Range.Start.Line
				end := sym.Range.End.Line
				if start < 0 {
					start = 0
				}
				if end >= len(lines) {
					end = len(lines) - 1
				}
				src := strings.Join(lines[start:end+1], "\n")
				source = &exploreSource{
					SymbolName: sym.Name,
					StartLine:  start + 1,
					EndLine:    end + 1,
					Source:     src,
				}
			}
		}
	}

	// 4. References (count + top files)
	locs, _ := client.GetReferences(ctx, fileURI, pos, false)
	refCount := len(locs)

	// Count references per file and collect top 5
	fileCounts := make(map[string]int)
	for _, loc := range locs {
		fp, fpErr := URIToFilePath(loc.URI)
		if fpErr != nil {
			continue
		}
		fileCounts[fp]++
		if isTestFile(fp) {
			testCallersCount++ // also count test references
		}
	}
	// Deduplicate: test callers already counted from call hierarchy, so
	// reset and recount just from references for accuracy.
	testCallersCount = 0
	for _, loc := range locs {
		fp, fpErr := URIToFilePath(loc.URI)
		if fpErr != nil {
			continue
		}
		if isTestFile(fp) {
			testCallersCount++
		}
	}

	// Sort files by reference count (simple selection of top 5)
	topFiles := topNFiles(fileCounts, 5)

	result := exploreResult{
		TypeInfo:         typeInfo,
		Source:           source,
		Callers:          callers,
		CallersCount:     callersCount,
		References:       exploreReferences{Count: refCount, TopFiles: topFiles},
		TestCallersCount: testCallersCount,
	}

	data, mErr := json.Marshal(result)
	if mErr != nil {
		return types.ErrorResult(fmt.Sprintf("explore_symbol: marshaling result: %s", mErr)), nil
	}
	return types.TextResult(string(data)), nil
}

// topNFiles returns the top N files by reference count.
func topNFiles(fileCounts map[string]int, n int) []string {
	type fileCount struct {
		path  string
		count int
	}
	var sorted []fileCount
	for path, count := range fileCounts {
		sorted = append(sorted, fileCount{path, count})
	}
	// Simple selection sort for small N
	for i := 0; i < len(sorted) && i < n; i++ {
		maxIdx := i
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[maxIdx].count {
				maxIdx = j
			}
		}
		sorted[i], sorted[maxIdx] = sorted[maxIdx], sorted[i]
	}
	limit := n
	if limit > len(sorted) {
		limit = len(sorted)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = sorted[i].path
	}
	return result
}
