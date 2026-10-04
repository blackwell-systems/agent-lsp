// tools_replace.go defines MCP tool registration for replace_in_files:
// multi-file find-and-replace with a dry-run/selective-apply protocol,
// .gitignore-aware scanning, and LSP-synced writes.
package main

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/audit"
	"github.com/blackwell-systems/agent-lsp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ReplaceInFilesArgs are the arguments for replace_in_files.
type ReplaceInFilesArgs struct {
	// Required: needle, repl and dry_run force the caller to state intent;
	// everything else is optional (omitempty keeps them out of the generated
	// JSON Schema "required" list, so strict clients like the pi MCP adapter
	// do not demand them, and toolArgsToMap drops zero values so the handler
	// defaults — Mode "literal", ExpectedCount -1 — apply).
	Needle           string   `json:"needle" jsonschema:"Text or regular expression to find (required)"`
	Repl             string   `json:"repl" jsonschema:"Replacement text (may be empty to delete)"`
	DryRun           bool     `json:"dry_run" jsonschema:"Preview every occurrence as a diff with a selectable id without changing anything. Call with true first; re-issue with false to apply"`
	Mode             string   `json:"mode,omitempty" jsonschema:"How to interpret needle: literal (default) or regex (Go RE2 syntax; use (?s) for multi-line)"`
	RelativePath     string   `json:"relative_path,omitempty" jsonschema:"Optional file or directory (workspace-root relative) restricting the scan"`
	PathsIncludeGlob string   `json:"paths_include_glob,omitempty" jsonschema:"Optional comma-separated include globs, e.g. src/**/*.mqh (matched against root-relative paths; .gitignore is always honored on top)"`
	PathsExcludeGlob string   `json:"paths_exclude_glob,omitempty" jsonschema:"Optional comma-separated exclude globs"`
	OccurrenceIds    []string `json:"occurrence_ids,omitempty" jsonschema:"Apply only these occurrence ids from the dry-run; if any id is unknown or stale (file changed since the dry-run) NOTHING is changed"`
	ExpectedCount    *int     `json:"expected_count,omitempty" jsonschema:"If >= 0, refuse to apply unless the match count equals this number (0 = require zero matches)"`
}

// filesLineRe extracts the machine-readable "Files: a, b" line the handler
// appends to apply-mode results, for the audit record.
var filesLineRe = regexp.MustCompile(`(?m)^Files: (.*)$`)

func registerReplaceInFilesTool(d toolDeps) {
	addToolWithPhaseCheck(d, &mcp.Tool{
		Name: "replace_in_files",
		Description: "Find and replace text across multiple files in one call. " +
			"Two modes: literal (default) and regex (Go RE2; use (?s) for multi-line patterns; in regex mode repl may reference capture groups as $1 or ${name}). " +
			"Protocol: (1) call with dry_run=true to preview every occurrence with a per-occurrence id; " +
			"(2) re-issue with dry_run=false to apply all of them, or pass occurrence_ids to apply a chosen subset. " +
			"If any id is unknown or stale, NOTHING is changed. " +
			"Scanning respects .gitignore (plus .git/.agent-lsp hard skips) and skips binary/oversized files; " +
			"narrow scope with relative_path or paths_include_glob/paths_exclude_glob; expected_count refuses surprising counts. " +
			"Writes go through the LSP client so textDocument/didChange keeps the server index in sync; BOM/CRLF outside edited ranges are preserved. " +
			"For symbol renames use rename_symbol instead; for coordinated multi-occurrence edits (e.g. repeated call sites across files) this is the tool.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Replace In Files",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ReplaceInFilesArgs) (*mcp.CallToolResult, any, error) {
		startTime := time.Now()
		client := d.cs.get()
		if client == nil {
			// Workspace-level tool with no file argument, so clientForFileWithAutoInit
			// cannot help; auto-init from the server's own working directory instead
			// (the MCP server is typically launched from the project root). If that
			// also fails, HandleReplaceInFiles returns its actionable start_lsp hint.
			client = d.autoInitForWorkspace(ctx)
		}
		r, err := tools.HandleReplaceInFiles(ctx, client, toolArgsToMap(args))

		// Audit: record the invocation with the affected files parsed from
		// the machine-readable "Files:" line of the result text.
		var files []string
		success := err == nil && !r.IsError
		if len(r.Content) > 0 {
			if m := filesLineRe.FindStringSubmatch(r.Content[0].Text); m != nil {
				if err := json.Unmarshal([]byte(m[1]), &files); err != nil {
					// Legacy comma-separated fallback.
					for _, f := range strings.Split(m[1], ", ") {
						if f = strings.TrimSpace(f); f != "" {
							files = append(files, f)
						}
					}
				}
			}
		}
		d.auditLogger.Log(audit.Record{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Tool:      "replace_in_files",
			Files:     files,
			EditSummary: &audit.EditSummary{
				Mode:           "replace-in-files",
				OldTextPreview: audit.Truncate(args.Needle, 200),
				NewTextPreview: audit.Truncate(args.Repl, 200),
				Apply:          !args.DryRun,
			},
			Success:    success,
			DurationMs: time.Since(startTime).Milliseconds(),
		})

		return makeCallToolResult(r), nil, err
	})
}
