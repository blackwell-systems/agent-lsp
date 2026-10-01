// analysis.go implements MCP tool handlers for code analysis queries:
// get_diagnostics, inspect_symbol (hover), get_completions,
// get_signature_help, suggest_fixes, list_symbols, and
// find_symbol.
//
// get_diagnostics has special behavior: it reopens the document from disk
// before collecting diagnostics, ensuring results reflect the latest saved
// state rather than stale LSP cache. It waits up to 25 seconds for
// diagnostics to settle (cross-package analysis in Go can be slow).
//
// Push is tried first: any document that delivered a
// textDocument/publishDiagnostics notification is used as-is. For a document
// whose push channel is dead, get_diagnostics issues one LSP 3.17
// textDocument/diagnostic request when the server declared diagnosticProvider
// (a bounded, non-retrying pull), so pull-model servers that never push can
// still report diagnostics. (issue #43)
//
// suggest_fixes filters the returned actions to a concise summary:
// title, kind, and whether a command or workspace edit is attached.
// Full workspace edits are not inlined to keep responses compact.
package tools

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	gcfgo "github.com/blackwell-systems/gcf-go"
)

// HandleGetDiagnostics retrieves LSP diagnostics for a file or all open documents.
func HandleGetDiagnostics(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, _ := args["file_path"].(string)

	var diagMap map[string][]types.LSPDiagnostic
	// queriedURIs is the set of documents whose diagnostics channel we must
	// classify (live vs dead) to decide whether an empty result is trustworthy.
	var queriedURIs []string

	if filePath != "" {
		cleanPath, err := ValidateFilePath(filePath, client.RootDir())
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("invalid file path: %s", err)), nil
		}
		fileURI := CreateFileURI(cleanPath)
		if err := client.ReopenDocument(ctx, fileURI); err != nil {
			return types.ErrorResult(fmt.Sprintf("failed to reopen document: %s", err)), nil
		}
		if err := lsp.WaitForDiagnostics(ctx, client, []string{fileURI}, 25000); err != nil {
			return types.ErrorResult(fmt.Sprintf("waiting for diagnostics: %s", err)), nil
		}
		diags := client.GetDiagnostics(fileURI)
		diagMap = map[string][]types.LSPDiagnostic{fileURI: diags}
		queriedURIs = []string{fileURI}
	} else {
		if err := client.ReopenAllDocuments(ctx); err != nil {
			return types.ErrorResult(fmt.Sprintf("failed to reopen documents: %s", err)), nil
		}
		openURIs := client.GetOpenDocuments()
		if err := lsp.WaitForDiagnostics(ctx, client, openURIs, 25000); err != nil {
			return types.ErrorResult(fmt.Sprintf("waiting for diagnostics: %s", err)), nil
		}
		all := client.GetAllDiagnostics()
		// Filter to only open documents.
		openSet := make(map[string]bool, len(openURIs))
		for _, u := range openURIs {
			openSet[u] = true
		}
		diagMap = make(map[string][]types.LSPDiagnostic)
		for uri, diags := range all {
			if openSet[uri] {
				diagMap[uri] = diags
			}
		}
		// A document that never received a publish is absent from GetOpenDocuments'
		// diagnostics entirely, so the queried set must be the open documents, not
		// the (possibly smaller) key set of diagMap.
		queriedURIs = openURIs
	}

	// Push-first: a document that already received a publishDiagnostics
	// notification is never pulled. For a document whose push channel is dead,
	// issue one textDocument/diagnostic request when the server declared
	// diagnosticProvider, and merge the result into diagMap. (issue #43)
	hasProvider := client.SupportsPullDiagnostics()
	pulledLive, pullAttemptedURIs := pullDiagnosticsForDeadChannels(ctx, client, diagMap, queriedURIs, hasProvider)

	hasErrors := false
	for _, diags := range diagMap {
		if len(diags) > 0 {
			hasErrors = true
			break
		}
	}

	deadURIs, liveCount := classifyDiagnosticsChannel(client, queriedURIs, pulledLive)
	// Split the dead URIs by whether a pull was actually attempted for them:
	// only attempted-and-unanswered documents may be reported as "pull did not
	// respond". Documents skipped because the budget ran out (or because the
	// fallback is disabled) keep the generic wording, which claims no pull was
	// tried.
	attemptedSet := make(map[string]bool, len(pullAttemptedURIs))
	for _, uri := range pullAttemptedURIs {
		attemptedSet[uri] = true
	}
	var pullDead, pullSkipped []string
	for _, uri := range deadURIs {
		if attemptedSet[uri] {
			pullDead = append(pullDead, uri)
		} else {
			pullSkipped = append(pullSkipped, uri)
		}
	}

	// group_by=symbol: group diagnostics under their owning symbol.
	groupBy, _ := args["group_by"].(string)
	if groupBy == "symbol" && filePath != "" {
		result, gErr := groupDiagnosticsBySymbol(ctx, client, filePath, diagMap)
		if gErr == nil {
			encoded, _ := EncodeResult(ctx, result)
			return appendHint(encoded, diagnosticsHint(hasErrors, deadURIs, liveCount, pullDead, pullSkipped)), nil
		}
		// Fall through to ungrouped if symbol grouping fails.
	}

	encoded, _ := EncodeResult(ctx, diagMap)
	return appendHint(encoded, diagnosticsHint(hasErrors, deadURIs, liveCount, pullDead, pullSkipped)), nil
}

// diagnosticsHint builds the next-step hint for a get_diagnostics result.
//
// An empty result is ambiguous: the server may have analyzed the document and
// found nothing (a live, empty publish channel), it may have answered a pull
// request successfully (also verified), or its diagnostics channel may be dead
// (no push ever, and no successful pull). The hint must not claim the file is
// clean in the last case. deadURIs lists queried documents that are not
// verified by either a textDocument/publishDiagnostics notification or a
// successful pull; liveCount is how many queried documents are verified (a
// published empty array counts as live). pullDead lists the dead URIs for which
// a pull was attempted but did not respond; pullSkipped lists dead URIs that
// were never attempted (pull disabled, no provider, or the aggregate budget
// ran out), so the wording never claims a pull was tried for them.
// (issues #43, #44)
func diagnosticsHint(hasErrors bool, deadURIs []string, liveCount int, pullDead, pullSkipped []string) string {
	const (
		fixesHint = "Use suggest_fixes at each error location for quick fixes."
		safeHint  = "No errors. Safe to proceed."
	)
	if hasErrors {
		if len(deadURIs) == 0 {
			return fixesHint
		}
		// Errors elsewhere do not confirm the dead documents: report them too,
		// so an all-documents query with errors in one file still surfaces the
		// unverified ones.
		sorted := append([]string(nil), deadURIs...)
		sort.Strings(sorted)
		return fixesHint + " No diagnostics received for: " + strings.Join(sorted, ", ") + " — those files are not confirmed clean."
	}
	if len(deadURIs) == 0 {
		// Every queried document is verified by a push or a successful pull
		// (or nothing was queried).
		return safeHint
	}
	sorted := append([]string(nil), deadURIs...)
	sort.Strings(sorted)
	sort.Strings(pullDead)
	sort.Strings(pullSkipped)
	joined := strings.Join(sorted, ", ")
	pullDeadList := strings.Join(pullDead, ", ")
	pullSkippedList := strings.Join(pullSkipped, ", ")
	if liveCount == 0 {
		switch {
		case len(pullDead) == 0:
			// No pull was ever attempted for the dead documents (fallback
			// disabled, no provider, or budget exhausted before any pull).
			return "No diagnostics received — the server has not published any for this document; this does not confirm the file is clean."
		case len(pullSkipped) == 0:
			return "No diagnostics received — the server publishes none and its pull diagnostics did not respond; this does not confirm the file is clean."
		default:
			return fmt.Sprintf("No diagnostics received — pull did not respond for %s; no pull was attempted for %s (pull time budget); this does not confirm the file is clean.", pullDeadList, pullSkippedList)
		}
	}
	switch {
	case len(pullDead) == 0:
		return safeHint + " No diagnostics received for: " + joined + " — those files are not confirmed clean."
	case len(pullSkipped) == 0:
		return safeHint + " No diagnostics received for: " + joined + " — the server publishes none and its pull diagnostics did not respond; those files are not confirmed clean."
	default:
		return safeHint + " No diagnostics received for: " + joined + " — pull did not respond for " + pullDeadList + "; no pull was attempted for " + pullSkippedList + " (pull time budget); those files are not confirmed clean."
	}
}

// classifyDiagnosticsChannel splits queried URIs into those that are not
// verified by any diagnostics channel (dead) and a count of those that are
// (live). A URI is verified when it received a publishDiagnostics notification
// (including a published empty array) or when a pull request answered
// successfully (pullLive). A dead channel makes an empty result unverifiable.
// (issues #43, #44)
func classifyDiagnosticsChannel(client *lsp.LSPClient, uris []string, pullLive map[string]bool) (deadURIs []string, liveCount int) {
	for _, uri := range uris {
		if client.HasPublishedDiagnostics(uri) || pullLive[uri] {
			liveCount++
		} else {
			deadURIs = append(deadURIs, uri)
		}
	}
	return deadURIs, liveCount
}

// pullDiagnosticsEnabled reports whether the pull fallback is enabled. The
// default is ENABLED: the one server whose pull method hung and wedged the
// process (observed with mql-lsp-server v2.4.2, upstream
// davalillo/mql-language-server#91) fixed it in v2.5.0 — verified end-to-end
// (textDocument/diagnostic answers, the server survives subsequent requests,
// and the MQL Tier-2 harness passes with the fallback on) — and the CI pin now
// references that release. It can be forced off via
// AGENT_LSP_PULL_DIAGNOSTICS=0 (0/false/no/off) for servers whose pull
// implementation is still broken. (issue #43)
var pullDiagnosticsEnabled = func() bool {
	switch strings.ToLower(os.Getenv("AGENT_LSP_PULL_DIAGNOSTICS")) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// shouldAttemptPull reports whether get_diagnostics should issue a
// textDocument/diagnostic request for one document. Pull is attempted only
// when the pull fallback is enabled, the server declared diagnosticProvider,
// and the push channel for that document is dead: push-first means a document
// that already received a publishDiagnostics notification is never pulled.
// (issue #43)
func shouldAttemptPull(hasProvider, pushLive bool) bool {
	return hasProvider && !pushLive && pullDiagnosticsEnabled()
}

// pullDiagnosticsForDeadChannels attempts a single textDocument/diagnostic
// request for each queried document whose push channel is dead, when the server
// declares diagnosticProvider. A successful pull replaces the (empty or absent)
// cached push entry in diagMap and is recorded in the returned set of
// pull-verified URIs. A pull that errors or times out leaves the channel dead;
// it is never retried and never run for a document that already received a
// push. (issue #43)
//
// pullBudget bounds the aggregate time spent pulling across documents: once
// exceeded, remaining dead documents are skipped for this query (left dead;
// the next query can pull them). Without it, an all-documents query against a
// server whose pull method hangs (davalillo/mql-language-server#91) with N
// open dead documents would block for N x 10s. (issue #43)
const pullTimeBudget = 15 * time.Second

func pullDiagnosticsForDeadChannels(ctx context.Context, client *lsp.LSPClient, diagMap map[string][]types.LSPDiagnostic, queriedURIs []string, hasProvider bool) (pulledLive map[string]bool, attemptedURIs []string) {
	pulledLive = make(map[string]bool)
	if !hasProvider {
		return pulledLive, nil
	}
	// Bound every pull by the remaining aggregate budget, not only by a
	// pre-check: a pull issued at 14.9s would otherwise still run for its full
	// 10s request timeout, overshooting the budget.
	budgetCtx, cancel := context.WithTimeout(ctx, pullTimeBudget)
	defer cancel()
	for _, uri := range queriedURIs {
		if !shouldAttemptPull(hasProvider, client.HasPublishedDiagnostics(uri)) {
			continue
		}
		if budgetCtx.Err() != nil {
			logging.Log(logging.LevelInfo, fmt.Sprintf("pull diagnostics: aggregate budget %s exhausted; skipping remaining dead documents", pullTimeBudget))
			break
		}
		attemptedURIs = append(attemptedURIs, uri)
		pulled, err := client.PullDiagnostics(budgetCtx, uri)
		if err != nil {
			// Timeout, protocol error, or no full report: the pull verified
			// nothing, and the caller never retries. But a publish may have
			// arrived while the pull was pending; the classifier then treats
			// the channel as live, so the result must carry those pushed
			// diagnostics rather than the empty entry collected before.
			if client.HasPublishedDiagnostics(uri) {
				diagMap[uri] = client.GetDiagnostics(uri)
			}
			continue
		}
		if client.HasPublishedDiagnostics(uri) {
			// A publish arrived while the pull was pending; the push is
			// authoritative for this document, so use it and do not count the
			// channel as pull-verified.
			diagMap[uri] = client.GetDiagnostics(uri)
			continue
		}
		diagMap[uri] = pulled
		pulledLive[uri] = true
	}
	return pulledLive, attemptedURIs
}

// symbolDiagGroup groups diagnostics under a named symbol.
type symbolDiagGroup struct {
	Name        string                `json:"name"`
	Kind        string                `json:"kind"`
	Line        int                   `json:"line"`
	Diagnostics []types.LSPDiagnostic `json:"diagnostics"`
}

// groupedDiagnostics is the response format for group_by=symbol.
type groupedDiagnostics struct {
	Symbols   []symbolDiagGroup     `json:"symbols"`
	Ungrouped []types.LSPDiagnostic `json:"ungrouped"`
}

// groupDiagnosticsBySymbol assigns each diagnostic to its owning symbol
// based on range containment. Diagnostics outside any symbol range go
// into the ungrouped list.
func groupDiagnosticsBySymbol(ctx context.Context, client *lsp.LSPClient, filePath string, diagMap map[string][]types.LSPDiagnostic) (*groupedDiagnostics, error) {
	fileURI := CreateFileURI(filePath)
	symbols, err := client.GetDocumentSymbols(ctx, fileURI)
	if err != nil {
		return nil, err
	}

	diags := diagMap[fileURI]
	if len(diags) == 0 {
		return &groupedDiagnostics{}, nil
	}

	// Build a flat list of symbols with their ranges for containment checks.
	type flatSymbol struct {
		name      string
		kind      string
		startLine int
		endLine   int
	}
	var flat []flatSymbol
	var flatten func(syms []types.DocumentSymbol, prefix string)
	flatten = func(syms []types.DocumentSymbol, prefix string) {
		for _, s := range syms {
			name := s.Name
			if prefix != "" {
				name = prefix + "." + name
			}
			flat = append(flat, flatSymbol{
				name:      name,
				kind:      symbolKindName(int(s.Kind)),
				startLine: s.Range.Start.Line,
				endLine:   s.Range.End.Line,
			})
			if len(s.Children) > 0 {
				flatten(s.Children, name)
			}
		}
	}
	flatten(symbols, "")

	// Assign each diagnostic to the innermost containing symbol.
	grouped := make(map[string]*symbolDiagGroup)
	var ungrouped []types.LSPDiagnostic

	for _, d := range diags {
		line := d.Range.Start.Line
		var bestMatch *flatSymbol
		for i := range flat {
			s := &flat[i]
			if line >= s.startLine && line <= s.endLine {
				if bestMatch == nil || (s.endLine-s.startLine) < (bestMatch.endLine-bestMatch.startLine) {
					bestMatch = s
				}
			}
		}
		if bestMatch != nil {
			g, ok := grouped[bestMatch.name]
			if !ok {
				g = &symbolDiagGroup{
					Name: bestMatch.name,
					Kind: bestMatch.kind,
					Line: bestMatch.startLine + 1,
				}
				grouped[bestMatch.name] = g
			}
			g.Diagnostics = append(g.Diagnostics, d)
		} else {
			ungrouped = append(ungrouped, d)
		}
	}

	result := &groupedDiagnostics{Ungrouped: ungrouped}
	for _, g := range grouped {
		result.Symbols = append(result.Symbols, *g)
	}
	return result, nil
}

// HandleGetInfoOnLocation retrieves hover information at a source location.
func HandleGetInfoOnLocation(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	line, col, err := ExtractPositionWithPattern(args, filePath)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = client.LanguageIDForFile(filePath)
	}

	result, wErr := WithDocument[string](ctx, client, filePath, languageID, func(fileURI string) (string, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetInfoOnLocation(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("inspect_symbol: %s", wErr)), nil
	}
	return appendHint(types.TextResult(result), "Use find_references to find all usages of this symbol."), nil
}

// HandleGetCompletions retrieves completion suggestions at a source location.
func HandleGetCompletions(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = client.LanguageIDForFile(filePath)
	}

	result, wErr := WithDocument[types.CompletionList](ctx, client, filePath, languageID, func(fileURI string) (types.CompletionList, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetCompletion(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_completions: %s", wErr)), nil
	}

	return EncodeResult(ctx, result)
}

// HandleGetSignatureHelp retrieves signature help at a source location.
func HandleGetSignatureHelp(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = client.LanguageIDForFile(filePath)
	}

	result, wErr := WithDocument[any](ctx, client, filePath, languageID, func(fileURI string) (any, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.GetSignatureHelp(ctx, fileURI, pos)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("get_signature_help: %s", wErr)), nil
	}

	return EncodeResult(ctx, result)
}

// HandleGetCodeActions retrieves code actions for a range in a document.
func HandleGetCodeActions(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	rng, err := extractRange(args)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = client.LanguageIDForFile(filePath)
	}

	result, wErr := WithDocument[[]types.CodeAction](ctx, client, filePath, languageID, func(fileURI string) ([]types.CodeAction, error) {
		return client.GetCodeActions(ctx, fileURI, rng)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("suggest_fixes: %s", wErr)), nil
	}

	encoded, _ := EncodeResult(ctx, result)
	return appendHint(encoded, "Use execute_command to apply a code action."), nil
}

// HandleGetDocumentSymbols retrieves the symbols defined in a document.
func HandleGetDocumentSymbols(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = client.LanguageIDForFile(filePath)
	}

	format, _ := args["format"].(string)

	result, wErr := WithDocument[[]types.DocumentSymbol](ctx, client, filePath, languageID, func(fileURI string) ([]types.DocumentSymbol, error) {
		return client.GetDocumentSymbols(ctx, fileURI)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("list_symbols: %s", wErr)), nil
	}

	shifted := make([]types.DocumentSymbol, len(result))
	for i, s := range result {
		shifted[i] = shiftDocumentSymbol(s)
	}

	docSymbolHint := "Use blast_radius with this file to analyze blast radius."
	if format == "outline" {
		return AppendTokenMeta(appendHint(types.TextResult(renderOutline(shifted, 0)), docSymbolHint), filePath), nil
	}

	if OutputFormatFromContext(ctx) == "gcf" {
		payload := buildDocumentSymbolsPayload(shifted, filePath)
		encodedResult, eErr := EncodeResult(ctx, payload)
		if eErr != nil {
			return encodedResult, eErr
		}
		return AppendTokenMeta(appendHint(encodedResult, docSymbolHint), filePath), nil
	}
	encodedResult, eErr := EncodeResult(ctx, shifted)
	if eErr != nil {
		return encodedResult, eErr
	}
	return AppendTokenMeta(appendHint(encodedResult, docSymbolHint), filePath), nil
}

// workspaceSymbolEnriched is a SymbolInformation with an optional hover field.
type workspaceSymbolEnriched struct {
	types.SymbolInformation
	Hover string `json:"hover,omitempty"`
}

// workspaceSymbolsResponse is the structured response for find_symbol.
// symbols contains all matches (name/kind/location only). enriched contains
// the hover-enriched window defined by offset and limit. pagination describes
// the current window position.
type workspaceSymbolsResponse struct {
	Total      int                        `json:"total"`
	Symbols    []types.SymbolInformation  `json:"symbols"`
	Enriched   []workspaceSymbolEnriched  `json:"enriched,omitempty"`
	Pagination *workspaceSymbolPagination `json:"pagination,omitempty"`
}

type workspaceSymbolPagination struct {
	Offset int  `json:"offset"`
	Limit  int  `json:"limit"`
	More   bool `json:"more"`
}

// encodeWorkspaceSymbolsResult encodes a find_symbol result honoring the
// active output format (GCF graph payload vs JSON).
func encodeWorkspaceSymbolsResult(ctx context.Context, symbols []types.SymbolInformation) (types.ToolResult, error) {
	if OutputFormatFromContext(ctx) == "gcf" {
		return EncodeResult(ctx, buildWorkspaceSymbolsPayload(symbols))
	}
	return EncodeResult(ctx, symbols)
}

// HandleGetWorkspaceSymbols searches for symbols across the workspace.
//
// detail_level controls enrichment:
//   - "basic" (or empty): returns all matching symbols with name/kind/location only.
//   - "hover" (default when limit/offset used): returns all symbols in symbols[],
//     plus hover-enriched results for the offset..offset+limit window in enriched[].
//
// limit (default 3) and offset (default 0) control the enrichment window.
// The AI can paginate: read symbols[] to see all results, use offset to step
// through enriched detail windows without re-running the workspace search.
func HandleGetWorkspaceSymbols(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}
	return HandleGetWorkspaceSymbolsMulti(ctx, []*lsp.LSPClient{client}, args)
}

// HandleGetWorkspaceSymbolsMulti is the multi-server fan-out of find_symbol
// (issue #2): server-less workspace queries must not be bound to the default
// server, which in auto-detect mode may not own the workspace's documents
// (e.g. clangd answering 0 for MQL symbols). The query runs against every
// initialized client; each symbol is tagged with the server that produced it
// ("server" field, empty when a single server answers). Single-client sets
// behave byte-identically to HandleGetWorkspaceSymbols.
func HandleGetWorkspaceSymbolsMulti(ctx context.Context, clients []*lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if len(clients) == 0 || (len(clients) == 1 && clients[0] == nil) {
		return types.ErrorResult("LSP client not initialized; call start_lsp first"), nil
	}

	query, _ := args["query"].(string)
	detailLevel, _ := args["detail_level"].(string)
	limit := 3
	if v, ok := toIntOpt(args, "limit"); ok && v > 0 {
		limit = v
	}
	offset := 0
	if v, ok := toIntOpt(args, "offset"); ok && v >= 0 {
		offset = v
	}

	// Fan out; tag each symbol with its producing server (stable order:
	// client list order, then server answer order).
	type serverSymbol struct {
		sym    types.SymbolInformation
		client *lsp.LSPClient
	}
	var merged []serverSymbol
	var firstErr error
	errored := 0
	anyDeclared := false
	for clientIdx, client := range clients {
		if client == nil || !client.IsInitialized() {
			continue
		}
		if client.HasCapability("workspaceSymbolProvider") {
			anyDeclared = true
		}
		syms, err := client.GetWorkspaceSymbols(ctx, query)
		if err != nil {
			errored++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		serverName, _ := client.GetServerInfo()
		if serverName == "" {
			serverName = "server-" + itoa(clientIdx)
		}
		for _, s := range syms {
			if len(clients) > 1 {
				s.Server = serverName
			}
			merged = append(merged, serverSymbol{sym: s, client: client})
		}
	}
	if len(merged) == 0 && firstErr != nil && errored == countInitialized(clients) {
		return types.ErrorResult(fmt.Sprintf("find_symbol: %s", firstErr)), nil
	}

	symbols := make([]types.SymbolInformation, len(merged))
	for i, ms := range merged {
		symbols[i] = ms.sym
	}

	defaultClient := clients[0]
	for _, c := range clients {
		if c != nil && c.IsInitialized() {
			defaultClient = c
			break
		}
	}

	wsSymHint := "Use inspect_symbol on a symbol for type details."
	// An empty result has distinct causes; qualify it instead of presenting
	// it as an authoritative "not found". The cause note is appended AFTER
	// the detail-specific encoding below — not via an early return — so the
	// response keeps its established envelope (workspaceSymbolsResponse with
	// total/symbols for hover detail) even when empty. (issue #42;
	// CodeRabbit #51 thread 1)
	emptyCause := ""
	if len(symbols) == 0 {
		if !anyDeclared {
			emptyCause = "No matches. The server does not declare the workspaceSymbolProvider capability — workspace symbol search is unavailable for this language server."
			if len(clients) > 1 {
				emptyCause = "No matches. None of the connected servers declare the workspaceSymbolProvider capability — workspace symbol search is unavailable."
			}
		} else if note := noteIndexCoverage(ctx, defaultClient); note != "" {
			emptyCause = "No matches. Note: " + note + "."
		}
	}
	if detailLevel == "basic" || detailLevel == "" {
		encoded, _ := encodeWorkspaceSymbolsResult(ctx, symbols)
		return appendHint(encoded, emptyHint(emptyCause, wsSymHint)), nil
	}

	// Enrich the offset..offset+limit window with hover info, using the
	// server that owns each symbol.
	resp := workspaceSymbolsResponse{
		Total:   len(symbols),
		Symbols: symbols,
	}

	if start, end, pg := symbolPaginationWindow(len(symbols), offset, limit); pg != nil {
		resp.Pagination = pg
		window := merged[start:end]
		enriched := make([]workspaceSymbolEnriched, len(window))
		for i, ms := range window {
			enriched[i] = workspaceSymbolEnriched{SymbolInformation: ms.sym}
			filePath, pErr := URIToFilePath(ms.sym.Location.URI)
			if pErr == nil {
				pos := types.Position{
					Line:      ms.sym.Location.Range.Start.Line + 1,
					Character: ms.sym.Location.Range.Start.Character + 1,
				}
				hoverText, hErr := WithDocument[string](ctx, ms.client, filePath, "", func(uri string) (string, error) {
					return ms.client.GetInfoOnLocation(ctx, uri, pos)
				})
				if hErr == nil && hoverText != "" {
					enriched[i].Hover = hoverText
				}
			}
		}
		resp.Enriched = enriched
	}

	encoded, _ := EncodeResult(ctx, resp)
	return appendHint(encoded, emptyHint(emptyCause, wsSymHint)), nil
}

// emptyHint picks the hint for a (possibly empty) result: the empty-cause
// note wins when present, otherwise the generic next-step hint.
func emptyHint(cause, fallback string) string {
	if cause != "" {
		return cause
	}
	return fallback
}

// countInitialized counts non-nil initialized clients in the set.
func countInitialized(clients []*lsp.LSPClient) int {
	n := 0
	for _, c := range clients {
		if c != nil && c.IsInitialized() {
			n++
		}
	}
	return n
}

// itoa is a minimal integer formatter for server-N labels.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// toIntOpt reads an integer argument without error — returns (value, true) if present and valid.
func toIntOpt(args map[string]any, key string) (int, bool) {
	v, err := toInt(args, key)
	return v, err == nil
}

// symbolPaginationWindow computes the enrichment window [start, end) and pagination
// metadata for a result set of size total. Returns nil pagination when offset is
// out of bounds. Extracted for testing.
func symbolPaginationWindow(total, offset, limit int) (start, end int, p *workspaceSymbolPagination) {
	if offset >= total || total == 0 {
		return 0, 0, nil
	}
	end = offset + limit
	if end > total {
		end = total
	}
	return offset, end, &workspaceSymbolPagination{
		Offset: offset,
		Limit:  limit,
		More:   end < total,
	}
}

// extractPosition reads line and column from args, validates 1-indexed.
func extractPosition(args map[string]any) (line, col int, err error) {
	line, err = toInt(args, "line")
	if err != nil {
		return 0, 0, fmt.Errorf("line: %w", err)
	}
	if line < 1 {
		return 0, 0, fmt.Errorf("line must be >= 1, got %d", line)
	}

	col, err = toInt(args, "column")
	if err != nil {
		return 0, 0, fmt.Errorf("column: %w", err)
	}
	if col < 1 {
		return 0, 0, fmt.Errorf("column must be >= 1, got %d", col)
	}

	return line, col, nil
}

// extractRange reads start/end line and column from args, validates 1-indexed and ordering.
func extractRange(args map[string]any) (types.Range, error) {
	startLine, err := toInt(args, "start_line")
	if err != nil {
		return types.Range{}, fmt.Errorf("start_line: %w", err)
	}
	if startLine < 1 {
		return types.Range{}, fmt.Errorf("start_line must be >= 1, got %d", startLine)
	}

	startCol, err := toInt(args, "start_column")
	if err != nil {
		return types.Range{}, fmt.Errorf("start_column: %w", err)
	}
	if startCol < 1 {
		return types.Range{}, fmt.Errorf("start_column must be >= 1, got %d", startCol)
	}

	endLine, err := toInt(args, "end_line")
	if err != nil {
		return types.Range{}, fmt.Errorf("end_line: %w", err)
	}
	if endLine < 1 {
		return types.Range{}, fmt.Errorf("end_line must be >= 1, got %d", endLine)
	}

	endCol, err := toInt(args, "end_column")
	if err != nil {
		return types.Range{}, fmt.Errorf("end_column: %w", err)
	}
	if endCol < 1 {
		return types.Range{}, fmt.Errorf("end_column must be >= 1, got %d", endCol)
	}

	// start must not be after end
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		return types.Range{}, fmt.Errorf("start position (%d:%d) must not be after end position (%d:%d)",
			startLine, startCol, endLine, endCol)
	}

	return types.Range{
		Start: types.Position{Line: startLine - 1, Character: startCol - 1},
		End:   types.Position{Line: endLine - 1, Character: endCol - 1},
	}, nil
}

// shiftDocumentSymbol converts all positions in a DocumentSymbol (and its children)
// from 0-based LSP convention to 1-based for MCP tool output.
func shiftDocumentSymbol(s types.DocumentSymbol) types.DocumentSymbol {
	s.Range = shiftRange(s.Range)
	s.SelectionRange = shiftRange(s.SelectionRange)
	for i, c := range s.Children {
		s.Children[i] = shiftDocumentSymbol(c)
	}
	return s
}

func shiftRange(r types.Range) types.Range {
	return types.Range{
		Start: types.Position{Line: r.Start.Line + 1, Character: r.Start.Character + 1},
		End:   types.Position{Line: r.End.Line + 1, Character: r.End.Character + 1},
	}
}

// renderOutline renders a DocumentSymbol tree as compact markdown for LLM consumption.
// Each symbol appears as "name [Kind] :line", indented two spaces per depth level.
// Children are rendered recursively beneath their parent.
func renderOutline(symbols []types.DocumentSymbol, depth int) string {
	var b strings.Builder
	indent := strings.Repeat("  ", depth)
	for _, s := range symbols {
		fmt.Fprintf(&b, "%s%s [%s] :%d\n", indent, s.Name, symbolKindName(int(s.Kind)), s.Range.Start.Line)
		if len(s.Children) > 0 {
			b.WriteString(renderOutline(s.Children, depth+1))
		}
	}
	return b.String()
}

// symbolKindName maps LSP SymbolKind integers to readable names.
func symbolKindName(kind int) string {
	names := map[int]string{
		1: "File", 2: "Module", 3: "Namespace", 4: "Package", 5: "Class",
		6: "Method", 7: "Property", 8: "Field", 9: "Constructor", 10: "Enum",
		11: "Interface", 12: "Function", 13: "Variable", 14: "Constant",
		22: "EnumMember", 23: "Struct", 26: "TypeParameter",
	}
	if n, ok := names[kind]; ok {
		return n
	}
	return fmt.Sprintf("Kind%d", kind)
}

// toInt extracts an integer from args[key]. Handles float64 (JSON default) and int.
func toInt(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("missing required argument %q", key)
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("argument %q must be a number, got %T", key, v)
	}
}

// buildDocumentSymbolsPayload converts document symbols into a flat graph Payload.
func buildDocumentSymbolsPayload(symbols []types.DocumentSymbol, filePath string) *gcfgo.Payload {
	var gcfSymbols []gcfgo.Symbol
	for i, sym := range symbols {
		score := max(0.1, 1.0-float64(i)*0.02)
		gcfSymbols = append(gcfSymbols, gcfgo.Symbol{
			QualifiedName: gcf.QualifiedName(filePath, sym.Name),
			Kind:          gcf.MapSymbolKind(sym.Kind),
			Score:         score,
			Provenance:    "lsp_resolved",
			Distance:      0,
		})
	}
	return gcf.BuildGraphPayload("list_symbols", gcfSymbols, nil)
}

// buildWorkspaceSymbolsPayload converts workspace symbols into a flat graph Payload.
func buildWorkspaceSymbolsPayload(symbols []types.SymbolInformation) *gcfgo.Payload {
	var gcfSymbols []gcfgo.Symbol
	for i, sym := range symbols {
		fp, _ := URIToFilePath(sym.Location.URI)
		score := max(0.1, 1.0-float64(i)*0.02)
		gcfSymbols = append(gcfSymbols, gcfgo.Symbol{
			QualifiedName: gcf.QualifiedName(fp, sym.Name),
			Kind:          gcf.MapSymbolKind(sym.Kind),
			Score:         score,
			Provenance:    "lsp_resolved",
			Distance:      0,
		})
	}
	return gcf.BuildGraphPayload("find_symbol", gcfSymbols, nil)
}
