package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	gcf "github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	gcfgo "github.com/blackwell-systems/gcf-go"
)

// concurrentEntryPatterns are source code patterns that indicate a concurrent
// entry point, organized by language family. Used by cross_concurrent to detect
// when a caller is inside a goroutine, thread, or async task.
var concurrentEntryPatterns = []string{
	"go func(", "go func (", // Go goroutines
	"new Thread(", "Thread.start(", "ExecutorService.", "CompletableFuture.", // Java
	"Task.Run(", "Task.Factory.", "new Thread(", "ThreadPool.", // C#
	"pthread_create(", "std::thread(", "std::async(", // C/C++
	"thread::spawn(", "tokio::spawn(", // Rust
	"DispatchQueue.", "Task {", "Task.detached", // Swift
	"threading.Thread(", "asyncio.create_task(", "asyncio.ensure_future(", // Python
	"new Worker(", "setTimeout(", "setInterval(", "Promise(", // JS/TS
}

// callHierarchyResult is the JSON shape returned by HandleCallHierarchy.
type callHierarchyResult struct {
	Items    []types.CallHierarchyItem         `json:"items"`
	Incoming []types.CallHierarchyIncomingCall `json:"incoming,omitempty"`
	Outgoing []types.CallHierarchyOutgoingCall `json:"outgoing,omitempty"`
	// ConcurrentCallers lists callers that cross a concurrent boundary
	// (goroutine, thread spawn, async task). Only populated when
	// cross_concurrent=true.
	ConcurrentCallers []concurrentCaller `json:"concurrent_callers,omitempty"`
}

// concurrentCaller annotates a call hierarchy incoming call with the
// concurrent entry pattern that was detected at the call site.
type concurrentCaller struct {
	Caller  types.CallHierarchyItem `json:"caller"`
	Pattern string                  `json:"pattern"`
	File    string                  `json:"file"`
	Line    int                     `json:"line"`
}

// HandleCallHierarchy resolves call hierarchy for the symbol at the given position.
// The direction argument controls which calls are returned:
//   - "incoming" -- callers of the function
//   - "outgoing" -- callees of the function
//   - "both"     -- both callers and callees (default when omitted or empty)
func HandleCallHierarchy(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	line, col, err := extractPosition(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	direction := "both"
	if d, ok := args["direction"].(string); ok && d != "" {
		direction = strings.ToLower(d)
	}

	crossConcurrent := false
	if cc, ok := args["cross_concurrent"].(bool); ok {
		crossConcurrent = cc
	}
	switch direction {
	case "incoming", "outgoing", "both":
		// valid
	default:
		return types.ErrorResult(fmt.Sprintf("invalid direction %q; must be \"incoming\", \"outgoing\", or \"both\"", direction)), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	items, wErr := WithDocument[[]types.CallHierarchyItem](ctx, client, filePath, languageID, func(fileURI string) ([]types.CallHierarchyItem, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.PrepareCallHierarchy(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("find_callers (prepare): %s", wErr)), nil
	}

	if len(items) == 0 {
		return types.TextResult(fmt.Sprintf("No call hierarchy item found at %s:%d:%d", filePath, line, col)), nil
	}

	result := callHierarchyResult{Items: items}

	for _, item := range items {
		if direction == "incoming" || direction == "both" {
			calls, callErr := client.GetIncomingCalls(ctx, item)
			if callErr != nil {
				return types.ErrorResult(fmt.Sprintf("find_callers (incoming): %s", callErr)), nil
			}
			result.Incoming = append(result.Incoming, calls...)
		}
		if direction == "outgoing" || direction == "both" {
			calls, callErr := client.GetOutgoingCalls(ctx, item)
			if callErr != nil {
				return types.ErrorResult(fmt.Sprintf("find_callers (outgoing): %s", callErr)), nil
			}
			result.Outgoing = append(result.Outgoing, calls...)
		}
	}

	// Cross-concurrent boundary detection: for each incoming caller, read the
	// source line at the call site and check if it's inside a concurrent entry
	// pattern (go func, Thread.start, spawn, etc.).
	if crossConcurrent && len(result.Incoming) > 0 {
		for _, call := range result.Incoming {
			fp, err := URIToFilePath(call.From.URI)
			if err != nil {
				continue
			}
			pattern := detectConcurrentPattern(fp, call.From.Range.Start.Line)
			if pattern != "" {
				result.ConcurrentCallers = append(result.ConcurrentCallers, concurrentCaller{
					Caller:  call.From,
					Pattern: pattern,
					File:    fp,
					Line:    call.From.Range.Start.Line + 1,
				})
			}
		}
	}

	hint := "Use blast_radius for a full blast-radius analysis."
	if crossConcurrent && len(result.ConcurrentCallers) > 0 {
		hint = fmt.Sprintf("%d caller(s) cross concurrent boundaries. These callers run in separate goroutines/threads. %s", len(result.ConcurrentCallers), hint)
	}
	if OutputFormatFromContext(ctx) == "gcf" {
		payload := buildCallHierarchyPayload(result, filePath)
		encoded, encErr := EncodeResult(ctx, payload)
		if encErr != nil {
			return types.ErrorResult(fmt.Sprintf("marshaling call hierarchy result: %s", encErr)), nil
		}
		return appendHint(encoded, hint), nil
	}
	encoded, encErr := EncodeResult(ctx, result)
	if encErr != nil {
		return types.ErrorResult(fmt.Sprintf("marshaling call hierarchy result: %s", encErr)), nil
	}
	return appendHint(encoded, hint), nil
}

func buildCallHierarchyPayload(result callHierarchyResult, filePath string) *gcfgo.Payload {
	var symbols []gcfgo.Symbol
	var edges []gcfgo.Edge

	// Target items (distance 0)
	for _, item := range result.Items {
		fp, _ := URIToFilePath(item.URI)
		if fp == "" {
			fp = filePath
		}
		symbols = append(symbols, gcfgo.Symbol{
			QualifiedName: gcf.QualifiedName(fp, item.Name),
			Kind:          gcf.MapSymbolKind(item.Kind),
			Score:         1.0,
			Provenance:    "lsp_resolved",
			Distance:      0,
		})
	}

	// Incoming callers (distance 1)
	for i, call := range result.Incoming {
		fp, _ := URIToFilePath(call.From.URI)
		qn := gcf.QualifiedName(fp, call.From.Name)
		score := max(0.1, 0.9-float64(i)*0.05)
		symbols = append(symbols, gcfgo.Symbol{
			QualifiedName: qn,
			Kind:          gcf.MapSymbolKind(call.From.Kind),
			Score:         score,
			Provenance:    "lsp_resolved",
			Distance:      1,
		})
		// Edge: caller calls target
		if len(result.Items) > 0 {
			targetFP, _ := URIToFilePath(result.Items[0].URI)
			if targetFP == "" {
				targetFP = filePath
			}
			edges = append(edges, gcfgo.Edge{
				Source:   qn,
				Target:   gcf.QualifiedName(targetFP, result.Items[0].Name),
				EdgeType: "calls",
			})
		}
	}

	// Outgoing callees (distance 1)
	for i, call := range result.Outgoing {
		fp, _ := URIToFilePath(call.To.URI)
		qn := gcf.QualifiedName(fp, call.To.Name)
		score := max(0.1, 0.8-float64(i)*0.05)
		symbols = append(symbols, gcfgo.Symbol{
			QualifiedName: qn,
			Kind:          gcf.MapSymbolKind(call.To.Kind),
			Score:         score,
			Provenance:    "lsp_resolved",
			Distance:      1,
		})
		if len(result.Items) > 0 {
			targetFP, _ := URIToFilePath(result.Items[0].URI)
			if targetFP == "" {
				targetFP = filePath
			}
			edges = append(edges, gcfgo.Edge{
				Source:   gcf.QualifiedName(targetFP, result.Items[0].Name),
				Target:   qn,
				EdgeType: "calls",
			})
		}
	}

	return gcf.BuildGraphPayload("find_callers", symbols, edges)
}

// detectConcurrentPattern reads the source file and checks lines around the
// given line number (0-indexed) for concurrent entry patterns. Returns the
// matched pattern string or empty if none found.
func detectConcurrentPattern(filePath string, line int) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")

	// Check a window of 5 lines above the call site (concurrent entry
	// patterns like "go func() {" appear before the actual call).
	start := line - 5
	if start < 0 {
		start = 0
	}
	end := line + 1
	if end > len(lines) {
		end = len(lines)
	}

	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		for _, pattern := range concurrentEntryPatterns {
			if strings.Contains(trimmed, pattern) {
				return pattern
			}
		}
	}
	return ""
}
