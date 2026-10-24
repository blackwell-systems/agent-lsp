// workspace.go implements MCP tool handlers that modify workspace state:
// rename_symbol, prepare_rename, open_document, close_document, apply_edit,
// format_document, and format_range.
//
// The rename handler includes a fuzzy fallback for imprecise AI positions:
// if the exact line:column produces an empty WorkspaceEdit, it hovers to get
// the symbol name, searches workspace symbols for a match, and retries the
// rename at the resolved definition location. This compensates for agents
// that provide approximate rather than exact cursor positions.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// HandleRenameSymbol renames the symbol at the given location across the workspace.
// When the direct position lookup returns an empty WorkspaceEdit, it falls back to
// workspace symbol search by hover name and retries — handling imprecise AI positions.
func HandleRenameSymbol(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	newName, ok := args["new_name"].(string)
	if !ok || newName == "" {
		return types.ErrorResult("new_name is required"), nil
	}

	line, col, err := ExtractPositionWithPattern(args, filePath)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[any](ctx, client, filePath, languageID, func(fileURI string) (any, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		res, rErr := client.RenameSymbol(ctx, fileURI, pos, newName)
		if rErr != nil {
			return nil, rErr
		}
		if isEmptyWorkspaceEdit(res) {
			logging.Log(logging.LevelDebug, "rename_symbol: empty result at exact position, trying fuzzy fallback")
			res = renameWithFuzzyFallback(ctx, client, fileURI, line, col, newName)
		}
		return res, nil
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("rename_symbol: %s", wErr)), nil
	}

	// Apply glob exclusions if provided.
	if globs := extractStringSlice(args, "exclude_globs"); len(globs) > 0 {
		result = filterWorkspaceEditByGlobs(result, globs)
	}

	if isEmptyWorkspaceEdit(result) {
		return types.TextResult(fmt.Sprintf("No rename edits produced for the symbol at %s:%d:%d (nothing to rename, or the position does not resolve to a renameable symbol).", filePath, line, col)), nil
	}

	files, locations, fileNames := summarizeWorkspaceEdit(result)

	dryRun, _ := args["dry_run"].(bool)
	if dryRun {
		type previewEnvelope struct {
			WorkspaceEdit any `json:"workspace_edit"`
			Preview       struct {
				Files     int      `json:"files"`
				Locations int      `json:"locations"`
				FileNames []string `json:"file_names"`
				Note      string   `json:"note"`
			} `json:"preview"`
		}
		env := previewEnvelope{WorkspaceEdit: result}
		env.Preview.Files = files
		env.Preview.Locations = locations
		env.Preview.FileNames = fileNames
		env.Preview.Note = "dry_run=true: preview only, nothing written. Re-run rename_symbol without dry_run to apply."
		// A preview carries the raw edit for inspection; it must round-trip byte-exact,
		// so always JSON, never GCF's summary tabular form. See issue #12.
		encoded, _ := EncodeResultJSON(env)
		return appendHint(encoded, "Re-run rename_symbol without dry_run to apply. Do not reconstruct this edit into apply_edit; rename_symbol applies it directly."), nil
	}

	// Apply the edit server-side, consistent with the other edit tools
	// (replace_symbol_body, insert_*, safe_delete_symbol). The WorkspaceEdit never
	// leaves the server as data the caller must reconstruct, which is what corrupted
	// files under GCF (transposed range offsets, truncated newText). See issue #12.
	if err := client.ApplyWorkspaceEdit(ctx, result); err != nil {
		return types.ErrorResult(fmt.Sprintf("rename_symbol: applying edit: %s", err)), nil
	}

	errCount, warnCount, verified := getDiagnosticsForFileStatus(ctx, client, filePath)
	summary := fmt.Sprintf("Renamed to %q across %d location(s) in %d file(s): %s",
		newName, locations, files, strings.Join(fileNames, ", "))
	hint := postEditDiagnosticsHint(errCount, warnCount, verified)
	// A rename can edit files other than the requested one, and the counts
	// above cover only filePath. Verify the diagnostics channel of every other
	// edited file, so the result is not presented as verified when a sibling
	// file's channel is dead. (issue #44 review follow-up)
	if unverified := verifyWorkspaceEditChannels(ctx, client, result, filePath); len(unverified) > 0 {
		sort.Strings(unverified)
		hint += " Diagnostics channel unverified for edited file(s): " + strings.Join(unverified, ", ") + "."
	}
	return appendHint(types.TextResult(summary), hint), nil
}

// verifyWorkspaceEditChannels re-checks the diagnostics channel of every file
// touched by the applied workspace edit except requestedFilePath: it drops the
// cached diagnostics, reopens the document to trigger a fresh publication, and
// reports the URIs whose channel still published nothing. A rename result must
// not read as verified when only the requested file's channel answered.
// (issue #44 review follow-up)
//
// All documents are reopened first and then share one wait, so a dead channel
// costs a single editChannelWaitMs regardless of how many files the rename
// touched (waiting per file made a 50-file rename against a server that never
// publishes block for minutes).
func verifyWorkspaceEditChannels(ctx context.Context, client *lsp.LSPClient, edit any, requestedFilePath string) []string {
	if client == nil {
		return nil
	}
	uris := editedURIsToVerify(edit, requestedFilePath, client.RootDir())
	if len(uris) == 0 {
		return nil
	}
	for _, uri := range uris {
		client.ResetDiagnostics(uri)
		_ = client.ReopenDocument(ctx, uri)
	}
	_ = lsp.WaitForDiagnostics(ctx, client, uris, editChannelWaitMs)
	var unverified []string
	for _, uri := range uris {
		if !client.HasPublishedDiagnostics(uri) {
			unverified = append(unverified, uri)
		}
	}
	return unverified
}

// editChannelWaitMs bounds the single shared wait in verifyWorkspaceEditChannels.
// A variable so tests can shorten it.
var editChannelWaitMs = 5000

// editedURIsToVerify returns the edited document URIs whose diagnostics channel
// verifyWorkspaceEditChannels should re-check: every URI in the edit except the
// requested file (already verified by the caller), limited to paths inside the
// workspace root. The URIs come from the language server's edit, and
// ApplyWorkspaceEdit validates only the form it applies (documentChanges wins
// over changes), so an out-of-root URI in the ignored form must not reach
// ReopenDocument, which would read it from disk and send it to the server.
func editedURIsToVerify(edit any, requestedFilePath, rootDir string) []string {
	requestedURI := lsp.NormalizeFileURI(CreateFileURI(requestedFilePath))
	var out []string
	for _, uri := range workspaceEditURIs(edit) {
		if lsp.NormalizeFileURI(uri) == requestedURI {
			continue
		}
		path, err := URIToFilePath(uri)
		if err != nil {
			continue
		}
		if _, err := ValidateFilePath(path, rootDir); err != nil {
			continue
		}
		out = append(out, uri)
	}
	return out
}

// workspaceEditURIs lists the document URIs a WorkspaceEdit touches, handling
// both the "changes" (map[uri][]TextEdit) and "documentChanges"
// ([]TextDocumentEdit) forms. The result is deduplicated and sorted.
func workspaceEditURIs(edit any) []string {
	data, err := json.Marshal(edit)
	if err != nil {
		return nil
	}
	var we struct {
		Changes         map[string]json.RawMessage `json:"changes"`
		DocumentChanges []struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(data, &we); err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var uris []string
	for uri := range we.Changes {
		if !seen[uri] {
			seen[uri] = true
			uris = append(uris, uri)
		}
	}
	for _, dc := range we.DocumentChanges {
		if dc.TextDocument.URI != "" && !seen[dc.TextDocument.URI] {
			seen[dc.TextDocument.URI] = true
			uris = append(uris, dc.TextDocument.URI)
		}
	}
	sort.Strings(uris)
	return uris
}

// summarizeWorkspaceEdit counts the files and edit locations in a WorkspaceEdit,
// handling both the "changes" (map[uri][]TextEdit) and "documentChanges"
// ([]TextDocumentEdit) forms. File names are the URI path basenames, sorted for
// deterministic output. It never surfaces newText, so it is safe to return to
// the caller without a byte-exact round-trip hazard.
func summarizeWorkspaceEdit(result any) (files int, locations int, fileNames []string) {
	data, err := json.Marshal(result)
	if err != nil {
		return 0, 0, nil
	}
	var we struct {
		Changes         map[string][]any `json:"changes"`
		DocumentChanges []struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []any `json:"edits"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(data, &we); err != nil {
		return 0, 0, nil
	}

	seen := map[string]struct{}{}
	add := func(uri string, n int) {
		if _, ok := seen[uri]; !ok {
			seen[uri] = struct{}{}
			fileNames = append(fileNames, uriBasename(uri))
		}
		locations += n
	}
	for uri, edits := range we.Changes {
		add(uri, len(edits))
	}
	for _, dc := range we.DocumentChanges {
		add(dc.TextDocument.URI, len(dc.Edits))
	}
	sort.Strings(fileNames)
	return len(seen), locations, fileNames
}

// uriBasename returns the final path segment of a file URI, falling back to the
// whole string when it has no slash.
func uriBasename(uri string) string {
	if i := strings.LastIndex(uri, "/"); i >= 0 && i+1 < len(uri) {
		return uri[i+1:]
	}
	return uri
}

// renameWithFuzzyFallback retries rename using workspace symbol candidates when the
// direct position lookup returned an empty WorkspaceEdit. Mirrors the pattern used
// by go_to_definition and find_references for position-imprecise AI callers.
func renameWithFuzzyFallback(ctx context.Context, client *lsp.LSPClient, fileURI string, line, col int, newName string) any {
	hoverPos := types.Position{Line: line - 1, Character: col - 1}
	hoverText, err := client.GetInfoOnLocation(ctx, fileURI, hoverPos)
	if err != nil || hoverText == "" {
		return nil
	}

	symbolName := extractSymbolName(hoverText)
	if symbolName == "" {
		return nil
	}

	logging.Log(logging.LevelDebug, "rename fuzzyFallback: searching workspace symbols for "+symbolName)

	syms, symErr := client.GetWorkspaceSymbols(ctx, symbolName)
	if symErr != nil || len(syms) == 0 {
		return nil
	}

	for _, sym := range syms {
		if sym.Location.URI == "" {
			continue
		}
		candidatePos := types.Position{
			Line:      sym.Location.Range.Start.Line,
			Character: sym.Location.Range.Start.Character,
		}
		res, rErr := client.RenameSymbol(ctx, sym.Location.URI, candidatePos, newName)
		if rErr == nil && !isEmptyWorkspaceEdit(res) {
			logging.Log(logging.LevelDebug, "rename fuzzyFallback: found result via workspace symbol candidate")
			return res
		}
	}
	return nil
}

// filterWorkspaceEditByGlobs removes workspace edit entries whose file URI
// matches any of the provided glob patterns. Patterns use filepath.Match syntax
// (e.g. "vendor/**", "**/*_gen.go"). If excludeGlobs is empty, the result is
// returned unchanged. Handles both "changes" (map[string][]TextEdit) and
// "documentChanges" ([]TextDocumentEdit) WorkspaceEdit formats.
func filterWorkspaceEditByGlobs(result any, excludeGlobs []string) any {
	if len(excludeGlobs) == 0 || result == nil {
		return result
	}

	data, err := json.Marshal(result)
	if err != nil {
		return result
	}

	var edit map[string]any
	if err := json.Unmarshal(data, &edit); err != nil {
		return result
	}

	matchesAny := func(fileURI string) bool {
		fp, convErr := URIToFilePath(fileURI)
		if convErr != nil {
			fp = strings.TrimPrefix(fileURI, "file://")
		}
		for _, pattern := range excludeGlobs {
			if matched, _ := filepath.Match(pattern, fp); matched {
				return true
			}
			if matched, _ := filepath.Match(pattern, filepath.Base(fp)); matched {
				return true
			}
		}
		return false
	}

	// Handle "changes" format: map[uri][]TextEdit
	if changes, ok := edit["changes"].(map[string]any); ok {
		filtered := make(map[string]any, len(changes))
		for uri, edits := range changes {
			if !matchesAny(uri) {
				filtered[uri] = edits
			}
		}
		edit["changes"] = filtered
	}

	// Handle "documentChanges" format: []TextDocumentEdit
	if docChanges, ok := edit["documentChanges"].([]any); ok {
		filtered := make([]any, 0, len(docChanges))
		for _, dc := range docChanges {
			dcMap, ok := dc.(map[string]any)
			if !ok {
				filtered = append(filtered, dc)
				continue
			}
			td, _ := dcMap["textDocument"].(map[string]any)
			uri, _ := td["uri"].(string)
			if uri == "" || !matchesAny(uri) {
				filtered = append(filtered, dc)
			}
		}
		edit["documentChanges"] = filtered
	}

	return edit
}

// extractStringSlice extracts a []string from args[key]. Handles both
// []string (typed) and []interface{} (JSON-decoded) input shapes.
func extractStringSlice(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// isEmptyWorkspaceEdit reports whether a RenameSymbol result contains no edits.
// A nil result or one that marshals to "null" or "{}" is considered empty —
// indicating the server found no symbol at the requested position.
func isEmptyWorkspaceEdit(result any) bool {
	if result == nil {
		return true
	}
	data, err := json.Marshal(result)
	if err != nil {
		return true
	}
	s := string(data)
	return s == "null" || s == "{}"
}

// HandlePrepareRename checks whether a rename is valid at the given location.
func HandlePrepareRename(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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
		languageID = "plaintext"
	}

	result, wErr := WithDocument[any](ctx, client, filePath, languageID, func(fileURI string) (any, error) {
		pos := types.Position{Line: line - 1, Character: col - 1}
		return client.PrepareRename(ctx, fileURI, pos)
	})
	if wErr != nil {
		if lsp.IsMethodNotFound(wErr) {
			// The server advertised prepareProvider (or the capability was
			// masked by #37) but answers textDocument/prepareRename itself with
			// JSON-RPC -32601: prepare_rename is genuinely unsupported. Report
			// that in words instead of failing the call; every other request
			// error still propagates.
			return prepareRenameUnsupportedResult(ctx)
		}
		return types.ErrorResult(fmt.Sprintf("prepare_rename: %s", wErr)), nil
	}
	if result == nil {
		return prepareRenameNilResult(ctx, client.GetCapabilities())
	}

	return EncodeResult(ctx, result)
}

// renameSupportsPrepare reports whether the server's renameProvider options
// declare prepareProvider support (map form with prepareProvider: true).
func renameSupportsPrepare(caps map[string]any) bool {
	v, ok := caps["renameProvider"]
	if !ok {
		return false
	}
	opts, ok := v.(map[string]any)
	if !ok {
		return false
	}
	pp, ok := opts["prepareProvider"].(bool)
	return ok && pp
}

// prepareRenameUnsupportedResult explains that the server does not implement
// textDocument/prepareRename at all: either it never declared prepareProvider,
// its declared support is masked (issue #37), or the method itself answers
// JSON-RPC -32601 despite the advertisement.
func prepareRenameUnsupportedResult(ctx context.Context) (types.ToolResult, error) {
	encoded, err := EncodeResult(ctx, map[string]any{
		"supported": false,
		"message":   "prepare_rename is not supported by this language server; rename_symbol may still work.",
	})
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}
	return encoded, nil
}

// prepareRenameNilResult explains a nil prepareRename result: either the
// server does not implement textDocument/prepareRename (capability check
// short-circuits), or it answered null for this position (rename not valid
// here). (issue #39)
//
// Both outcomes are informational, never an ErrorResult: the tool answered
// honestly, it simply has no range to report.
func prepareRenameNilResult(ctx context.Context, caps map[string]any) (types.ToolResult, error) {
	if renameSupportsPrepare(caps) {
		encoded, err := EncodeResult(ctx, map[string]any{
			"supported": true,
			"message":   "The server returned no rename range at this position; a rename here may not be valid.",
		})
		if err != nil {
			return types.ErrorResult(err.Error()), nil
		}
		return appendHint(encoded, "Use rename_symbol with dry_run=true to check before applying."), nil
	}
	return prepareRenameUnsupportedResult(ctx)
}

// HandleFormatDocument formats an entire document.
func HandleFormatDocument(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}

	tabSize := 2
	if v, err := toInt(args, "tab_size"); err != nil && args["tab_size"] != nil {
		return types.ErrorResult(fmt.Sprintf("tab_size: %s", err)), nil
	} else if err == nil {
		tabSize = v
	}

	insertSpaces := true
	if v, ok := args["insert_spaces"].(bool); ok {
		insertSpaces = v
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[[]types.TextEdit](ctx, client, filePath, languageID, func(fileURI string) ([]types.TextEdit, error) {
		return client.FormatDocument(ctx, fileURI, tabSize, insertSpaces)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("format_document: %s", wErr)), nil
	}

	return EncodeResult(ctx, result)
}

// HandleFormatRange formats a range within a document.
func HandleFormatRange(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
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

	tabSize := 2
	if v, tErr := toInt(args, "tab_size"); tErr != nil && args["tab_size"] != nil {
		return types.ErrorResult(fmt.Sprintf("tab_size: %s", tErr)), nil
	} else if tErr == nil {
		tabSize = v
	}

	insertSpaces := true
	if v, ok := args["insert_spaces"].(bool); ok {
		insertSpaces = v
	}

	languageID, _ := args["language_id"].(string)
	if languageID == "" {
		languageID = "plaintext"
	}

	result, wErr := WithDocument[[]types.TextEdit](ctx, client, filePath, languageID, func(fileURI string) ([]types.TextEdit, error) {
		return client.FormatRange(ctx, fileURI, rng, tabSize, insertSpaces)
	})
	if wErr != nil {
		return types.ErrorResult(fmt.Sprintf("format_range: %s", wErr)), nil
	}

	return EncodeResult(ctx, result)
}

// HandleApplyEdit applies a workspace edit.
//
// Two modes:
//   - WorkspaceEdit mode (default): supply workspace_edit with positional changes.
//   - Text-match mode: supply file_path + old_text + new_text. The tool finds
//     old_text in the file (exact match first, then whitespace-normalized line
//     match) and replaces it with new_text without requiring line/column positions.
func HandleApplyEdit(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	// Text-match mode: file_path + old_text + new_text.
	oldText, hasOld := args["old_text"].(string)
	filePath, hasPath := args["file_path"].(string)
	if hasOld && oldText != "" && hasPath && filePath != "" {
		validPath, err := ValidateFilePath(filePath, client.RootDir())
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("apply_edit (text-match): %s", err)), nil
		}
		newText, _ := args["new_text"].(string)
		absPath, absErr := filepath.Abs(validPath)
		if absErr != nil {
			absPath = validPath
		}
		// The full read→find→apply→verify sequence holds the per-file lock so
		// two concurrent text-match applies to the same file cannot both
		// compute their ranges against the same disk snapshot (issue #18).
		result, err := applyTextMatchEdit(ctx, absPath, oldText, newText, func(ctx context.Context, edit any) error {
			return client.ApplyWorkspaceEdit(ctx, edit)
		})
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("apply_edit (text-match): %s", err)), nil
		}
		return result, nil
	}

	// WorkspaceEdit mode.
	edit, ok := args["workspace_edit"]
	if !ok || edit == nil {
		return types.ErrorResult("workspace_edit is required (or supply file_path + old_text + new_text for text-match mode)"), nil
	}

	if err := client.ApplyWorkspaceEdit(ctx, edit); err != nil {
		return types.ErrorResult(fmt.Sprintf("apply_edit: %s", err)), nil
	}
	return types.TextResult("Edit applied successfully"), nil
}

// textMatchApply constructs a WorkspaceEdit by locating oldText in the file at
// filePath and replacing it with newText. It tries an exact byte match first
// (tolerating an old_text written with LF endings against a CRLF region and
// balancing trailing newlines, issue #18); if that fails it falls back to a
// line-by-line whitespace-normalised match (each line compared after
// strings.TrimSpace). The normalised match replaces full lines, so newText
// should include the desired indentation for those lines.
func textMatchApply(filePath, oldText, newText string) (any, error) {
	plan, err := buildTextMatchPlan(filePath, oldText, newText)
	if err != nil {
		return nil, err
	}
	return plan.edit, nil
}

// textMatchFileLocks serializes the text-match apply sequence per absolute
// file path (issue #18 fix 2): without it, two concurrent text-match applies
// to the same file both computed their ranges against the same disk snapshot
// and the second landed at a stale position. Entries are never removed; the
// map is bounded by the number of distinct files edited in the process
// lifetime.
var (
	textMatchFileLocksMu sync.Mutex
	textMatchFileLocks   = map[string]*sync.Mutex{}
)

// lockTextMatchFile acquires the per-file mutex for absPath and returns the
// corresponding unlock function.
func lockTextMatchFile(absPath string) func() {
	textMatchFileLocksMu.Lock()
	mu := textMatchFileLocks[absPath]
	if mu == nil {
		mu = &sync.Mutex{}
		textMatchFileLocks[absPath] = mu
	}
	textMatchFileLocksMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// workspaceEditApplier applies a constructed WorkspaceEdit. HandleApplyEdit
// wires it to LSPClient.ApplyWorkspaceEdit; tests inject a splicing fake.
type workspaceEditApplier func(ctx context.Context, edit any) error

// applyTextMatchEdit runs the full serialized text-match apply sequence for
// one file: read → findText → build edit → apply → post-apply verification
// (issue #18 fixes 2 and 3). The whole sequence holds the per-file lock so a
// concurrent apply to the same file cannot invalidate the computed range
// between the read and the write. The WorkspaceEdit (positional) mode of
// apply_edit is not serialized.
func applyTextMatchEdit(ctx context.Context, filePath, oldText, newText string, applyFn workspaceEditApplier) (types.ToolResult, error) {
	unlock := lockTextMatchFile(filePath)
	defer unlock()

	plan, err := buildTextMatchPlan(filePath, oldText, newText)
	if err != nil {
		return types.ToolResult{}, err
	}
	if err := applyFn(ctx, plan.edit); err != nil {
		return types.ToolResult{}, err
	}

	// Post-apply verification (issue #18 fix 3): re-read the file and confirm
	// the replacement actually landed at the intended site. A precomputed
	// range applied against a stale snapshot used to mis-locate silently.
	after, err := os.ReadFile(filePath)
	if err != nil {
		return types.ToolResult{}, fmt.Errorf("post-apply verification: reading %s: %w", filePath, err)
	}
	if !verifyTextMatchApplied(string(after), plan) {
		return types.ToolResult{}, fmt.Errorf("post-apply verification failed for %s: the replacement is not present at the intended site (the edit may have been computed against a stale file snapshot); re-run apply_edit against the current file contents", filePath)
	}
	return textMatchSuccessResult(plan, string(after)), nil
}

// verifyTextMatchApplied checks that a text-match edit landed where it was
// computed to land: the replacement text is present in the file and occupies
// exactly the byte span the matched span was replaced by, and — when the
// matched text was unique in the pre-edit snapshot — the matched text is gone
// from the file. The site check, not a whole-file absence check, carries the
// concurrency guarantee: old_text may legitimately occur again elsewhere in
// the file (strings.Index replaces the first occurrence).
func verifyTextMatchApplied(after string, plan textMatchPlan) bool {
	siteEnd := plan.startByte + len(plan.replacement)
	if siteEnd > len(after) || after[plan.startByte:siteEnd] != plan.replacement {
		return false
	}
	// When the matched text was unique before the edit, it must not survive
	// anywhere outside the applied site: the replacement may legitimately
	// embed the old text (e.g. "return nil" -> "return nil // guard"), so
	// the absence check excludes the replaced span instead of the whole file.
	if strings.Count(plan.src, plan.spanText) == 1 {
		outside := after[:plan.startByte] + after[siteEnd:]
		if strings.Contains(outside, plan.spanText) {
			return false
		}
	}
	return true
}

// textMatchSuccessResult reports the applied site: file path plus the byte
// range and LSP 0-based line span where new_text now lives (issue #18 fix 3).
// The "Edit applied successfully" prefix is kept so existing callers that
// match on it are unaffected.
func textMatchSuccessResult(plan textMatchPlan, after string) types.ToolResult {
	siteEnd := plan.startByte + len(plan.replacement)
	startLine, _, endLine, _ := byteSpanToLSPRange(after, plan.startByte, siteEnd)
	return types.TextResult(fmt.Sprintf("Edit applied successfully: %s bytes %d-%d (lines %d-%d, LSP 0-based)",
		plan.filePath, plan.startByte, siteEnd, startLine, endLine))
}

// textMatchPlan is the outcome of locating oldText in a file and preparing
// the replacement: the WorkspaceEdit to apply, the exact bytes removed, the
// final replacement bytes (after EOL balancing), and the pre-edit snapshot
// used for post-apply verification.
type textMatchPlan struct {
	filePath    string
	src         string
	startByte   int
	spanText    string
	replacement string
	edit        any
}

// buildTextMatchPlan locates oldText in the file at filePath and prepares the
// replacement WorkspaceEdit. The exact match path is CRLF-tolerant and
// balances trailing newlines (issue #18 fix 4); the whitespace-normalised
// fallback keeps its existing semantics.
func buildTextMatchPlan(filePath, oldText, newText string) (textMatchPlan, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return textMatchPlan{}, fmt.Errorf("reading %s: %w", filePath, err)
	}
	src := string(content)

	var plan textMatchPlan
	if start, end, ok := findTextExact(src, oldText); ok {
		spanText := src[start:end]
		replacement := newText
		if strings.Contains(spanText, "\r\n") {
			// The matched region uses CRLF endings: preserve them in the
			// replacement even when old_text/new_text were written with LF
			// (issue #18 fix 4).
			replacement = toCRLF(newText)
		}
		replacement = balanceTrailingEOL(spanText, replacement)
		plan = textMatchPlan{filePath: filePath, src: src, startByte: start, spanText: spanText, replacement: replacement}
	} else {
		start, end, ok := findTextNormalized(src, oldText)
		if !ok {
			return textMatchPlan{}, fmt.Errorf("old_text not found in %s (tried exact and whitespace-normalised line match)", filePath)
		}
		plan = textMatchPlan{filePath: filePath, src: src, startByte: start, spanText: src[start:end], replacement: newText}
	}

	startLine, startChar, endLine, endChar := byteSpanToLSPRange(src, plan.startByte, plan.startByte+len(plan.spanText))
	// A leading UTF-8 BOM is part of src, so the strings.Index byte offsets
	// and the byteSpanToLSPRange positions agree by construction — the same
	// convention as buildReplaceWorkspaceEdit — and the BOM must not shift
	// the computed range (issue #18 fix 4).
	fileURI := CreateFileURI(filePath)
	plan.edit = map[string]any{
		"changes": map[string]any{
			fileURI: []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": startLine, "character": startChar},
						"end":   map[string]any{"line": endLine, "character": endChar},
					},
					"newText": plan.replacement,
				},
			},
		},
	}
	return plan, nil
}

// findTextExact returns the byte span [start, end) of the first exact
// occurrence of oldText within src, tolerating an old_text written with LF
// line endings that targets a CRLF region (issue #18 fix 4): when the literal
// search fails, the LF endings of oldText are retried as CRLF.
func findTextExact(src, oldText string) (start, end int, found bool) {
	if idx := strings.Index(src, oldText); idx >= 0 {
		return idx, idx + len(oldText), true
	}
	if strings.Contains(oldText, "\n") && strings.Contains(src, "\r\n") {
		crlfOld := strings.ReplaceAll(oldText, "\n", "\r\n")
		if idx := strings.Index(src, crlfOld); idx >= 0 {
			return idx, idx + len(crlfOld), true
		}
	}
	return 0, 0, false
}

// balanceTrailingEOL preserves line structure across a text-match replacement
// (issue #18 fix 4): when the matched span includes a trailing newline that
// the replacement does not provide, the span's EOL is re-attached so the
// following line does not fuse onto the last line of the replacement
// (tail-merge); conversely, when the replacement ends with an EOL the span
// does not include, that EOL is trimmed so no blank line is inserted. A
// mismatched EOL style (CRLF vs LF) on either side is aligned to the span's
// style. An empty replacement is a deletion of the whole matched span and is
// left untouched.
func balanceTrailingEOL(spanText, replacement string) string {
	if replacement == "" {
		return replacement
	}
	var spanEOL string
	switch {
	case strings.HasSuffix(spanText, "\r\n"):
		spanEOL = "\r\n"
	case strings.HasSuffix(spanText, "\n"):
		spanEOL = "\n"
	}
	var replEOL string
	switch {
	case strings.HasSuffix(replacement, "\r\n"):
		replEOL = "\r\n"
	case strings.HasSuffix(replacement, "\n"):
		replEOL = "\n"
	}
	switch {
	case spanEOL == "" && replEOL != "":
		// The span does not consume a newline; the replacement must not
		// insert one.
		return strings.TrimSuffix(replacement, replEOL)
	case spanEOL != "" && replEOL != spanEOL:
		// The span consumes a newline the replacement does not provide (or
		// provides in the wrong style): re-attach the span's EOL.
		if replEOL != "" {
			replacement = strings.TrimSuffix(replacement, replEOL)
		}
		return replacement + spanEOL
	default:
		return replacement
	}
}

// toCRLF converts every LF line ending in s to CRLF (idempotent), preserving
// a CRLF file's line endings in a replacement whose text was written with LF.
func toCRLF(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// findText returns the byte range [start, end) of oldText within src.
// It first tries an exact bytes match (CRLF-tolerant); on failure it attempts
// a line-by-line comparison where each line is stripped of leading/trailing
// whitespace before comparison. The normalised match returns the byte range of
// the full matched lines (including their original indentation) so callers
// replace complete lines.
func findText(src, oldText string) (start, end int, found bool) {
	if s, e, ok := findTextExact(src, oldText); ok {
		return s, e, true
	}
	return findTextNormalized(src, oldText)
}

// findTextNormalized locates oldText by line-by-line whitespace-normalised
// comparison and returns the byte range of the full matched lines (including
// their original indentation and trailing newline, if any).
func findTextNormalized(src, oldText string) (start, end int, found bool) {
	// Whitespace-normalised line match.
	srcLines := strings.Split(src, "\n")
	patLines := strings.Split(oldText, "\n")
	// Drop a trailing empty element caused by a trailing newline in oldText.
	if len(patLines) > 0 && patLines[len(patLines)-1] == "" {
		patLines = patLines[:len(patLines)-1]
	}
	if len(patLines) == 0 || len(patLines) > len(srcLines) {
		return 0, 0, false
	}

	normPat := make([]string, len(patLines))
	for i, l := range patLines {
		normPat[i] = strings.TrimSpace(l)
	}

	for i := 0; i <= len(srcLines)-len(patLines); i++ {
		match := true
		for j, pat := range normPat {
			if strings.TrimSpace(srcLines[i+j]) != pat {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// Compute byte offsets: start at beginning of line i, end after last
		// character of line i+len(patLines)-1 (before its newline, if any).
		byteStart := 0
		for k := 0; k < i; k++ {
			byteStart += len(srcLines[k]) + 1 // +1 for '\n'
		}
		byteEnd := byteStart
		lastIdx := i + len(patLines) - 1
		for k := i; k <= lastIdx; k++ {
			byteEnd += len(srcLines[k])
			if k < len(srcLines)-1 {
				byteEnd++ // '\n'
			}
		}
		return byteStart, byteEnd, true
	}

	return 0, 0, false
}

// HandleExecuteCommand executes a workspace command.
func HandleExecuteCommand(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}

	command, ok := args["command"].(string)
	if !ok || command == "" {
		return types.ErrorResult("command is required"), nil
	}

	var cmdArgs []any
	if v, ok := args["arguments"].([]any); ok {
		cmdArgs = v
	}

	result, err := client.ExecuteCommand(ctx, command, cmdArgs)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("execute_command: %s", err)), nil
	}

	return EncodeResult(ctx, result)
}
