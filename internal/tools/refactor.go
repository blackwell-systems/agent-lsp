// refactor.go implements the refactor tool: run a language server's refactoring
// code action at a position, including the interactive refactorings that ask
// for input before computing their edit (gopls v0.24: add or remove struct
// tags, implement an interface, move a declaration).
//
// Interactive refactorings use the command/resolve form protocol
// (golang/go#76331, microsoft/language-server-protocol#1164). Editors render
// the server's form as a dialog; here the form is returned to the agent, which
// answers by calling the tool again with answers. The protocol is stateless,
// so each call re-resolves from the code action.
//
// The server delivers the final edit through workspace/applyEdit. The tool
// captures it instead of letting it be written, so dry_run can return it as a
// preview, and otherwise applies it and reports diagnostics, as rename_symbol
// does.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// maxEnumOptions caps the lazyEnum options returned with a form.
const maxEnumOptions = 50

// refactorFormField is a form question as presented to the agent.
type refactorFormField struct {
	ID          string              `json:"id"`
	Description string              `json:"description"`
	Kind        string              `json:"kind"`
	Required    bool                `json:"required"`
	Default     any                 `json:"default,omitempty"`
	Error       string              `json:"error,omitempty"`
	Options     []lsp.FormEnumEntry `json:"options,omitempty"`
	Type        json.RawMessage     `json:"type,omitempty"`
}

type refactorAction struct {
	Title string `json:"title"`
	Kind  string `json:"kind,omitempty"`
}

// HandleRefactor lists the refactorings at a position, or runs one.
func HandleRefactor(ctx context.Context, client *lsp.LSPClient, args map[string]any) (types.ToolResult, error) {
	if err := CheckInitialized(client); err != nil {
		return types.ErrorResult(err.Error()), nil
	}
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return types.ErrorResult("file_path is required"), nil
	}
	filePath, err := ValidateFilePath(filePath, client.RootDir())
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid file_path: %s", err)), nil
	}
	line, col, err := ExtractPositionWithPattern(args, filePath)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}
	rng := types.Range{
		Start: types.Position{Line: line - 1, Character: col - 1},
		End:   types.Position{Line: line - 1, Character: col - 1},
	}
	if endLine, _ := toIntOptional(args, "end_line"); endLine > 0 {
		endCol, _ := toIntOptional(args, "end_column")
		if endCol < 1 {
			endCol = 1
		}
		rng.End = types.Position{Line: endLine - 1, Character: endCol - 1}
	}
	languageID, _ := args["language_id"].(string)

	actions, err := WithDocument[[]types.CodeAction](ctx, client, filePath, languageID, func(fileURI string) ([]types.CodeAction, error) {
		return client.GetCodeActions(ctx, fileURI, rng)
	})
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("refactor: %s", err)), nil
	}
	var refactorings []types.CodeAction
	for _, a := range actions {
		if a.Disabled != nil || (a.Command == nil && a.Edit == nil) {
			continue
		}
		if a.Kind != nil && strings.HasPrefix(*a.Kind, "refactor") {
			refactorings = append(refactorings, a)
		}
	}

	want, _ := args["action"].(string)
	if want == "" {
		return listRefactorings(refactorings, filePath, line, col), nil
	}
	action, errResult := pickRefactoring(refactorings, want)
	if errResult != nil {
		return *errResult, nil
	}

	dryRun, _ := args["dry_run"].(bool)
	var edits []any
	switch {
	case action.Command == nil:
		edits = []any{action.Edit}
	default:
		command, cmdArgs := action.Command.Command, action.Command.Arguments
		answers := formAnswers(args)
		if client.SupportsInteractiveResolve() {
			resolved, err := client.ResolveCommand(ctx, command, cmdArgs, answers)
			if err != nil {
				return types.ErrorResult(fmt.Sprintf("refactor: resolving %q: %s", action.Title, err)), nil
			}
			if len(resolved.FormFields) > 0 {
				return refactorForm(ctx, client, action.Title, resolved.FormFields, args), nil
			}
			command, cmdArgs, answers = resolved.Command, resolved.Arguments, resolved.FormAnswers
		}
		captured, err := client.ExecuteCommandCapturingEdits(ctx, command, cmdArgs, answers)
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("refactor: running %q: %s", action.Title, err)), nil
		}
		if action.Edit != nil {
			captured = append([]any{action.Edit}, captured...)
		}
		edits = captured
	}

	var nonEmpty []any
	for _, e := range edits {
		if !isEmptyWorkspaceEdit(e) {
			nonEmpty = append(nonEmpty, e)
		}
	}
	if len(nonEmpty) == 0 {
		return types.TextResult(fmt.Sprintf("%q produced no edit at %s:%d:%d.", action.Title, filePath, line, col)), nil
	}

	locations, fileNames := 0, map[string]bool{}
	for _, e := range nonEmpty {
		_, l, names := summarizeWorkspaceEdit(e)
		locations += l
		for _, n := range names {
			fileNames[n] = true
		}
	}
	names := make([]string, 0, len(fileNames))
	for n := range fileNames {
		names = append(names, n)
	}
	sort.Strings(names)

	if dryRun {
		preview := map[string]any{
			"action":          action.Title,
			"workspace_edits": nonEmpty,
			"preview": map[string]any{
				"files":      len(names),
				"locations":  locations,
				"file_names": names,
				"note":       "dry_run=true: preview only, nothing written. Re-run refactor with the same action and answers, without dry_run, to apply.",
			},
		}
		// Edits must round-trip exactly, so JSON, never GCF (see issue #12).
		encoded, _ := EncodeResultJSON(preview)
		return encoded, nil
	}

	for i, e := range nonEmpty {
		if err := client.ApplyWorkspaceEdit(ctx, e); err != nil {
			return types.ErrorResult(fmt.Sprintf("refactor: applying edit %d of %d: %s", i+1, len(nonEmpty), err)), nil
		}
	}
	errCount, warnCount, verified := getDiagnosticsForFileStatus(ctx, client, filePath)
	summary := fmt.Sprintf("Applied %q: %d location(s) in %d file(s): %s", action.Title, locations, len(names), strings.Join(names, ", "))
	hint := postEditDiagnosticsHint(errCount, warnCount, verified)
	var unverified []string
	for _, e := range nonEmpty {
		unverified = append(unverified, verifyWorkspaceEditChannels(ctx, client, e, filePath)...)
	}
	if len(unverified) > 0 {
		sort.Strings(unverified)
		hint += " Diagnostics channel unverified for edited file(s): " + strings.Join(unverified, ", ") + "."
	}
	return appendHint(types.TextResult(summary), hint), nil
}

func listRefactorings(actions []types.CodeAction, filePath string, line, col int) types.ToolResult {
	if len(actions) == 0 {
		return types.TextResult(fmt.Sprintf("No refactorings available at %s:%d:%d. Some refactorings need a selection: pass end_line/end_column (for example, select struct fields to add tags).", filePath, line, col))
	}
	list := make([]refactorAction, 0, len(actions))
	for _, a := range actions {
		kind := ""
		if a.Kind != nil {
			kind = *a.Kind
		}
		list = append(list, refactorAction{Title: a.Title, Kind: kind})
	}
	encoded, _ := EncodeResultJSON(map[string]any{"refactorings": list})
	return appendHint(encoded, "Call refactor again with action set to one of these titles. If it needs input, the result lists the questions to answer.")
}

// pickRefactoring selects the action whose title matches want: exactly
// (ignoring case), else as the only title containing it.
func pickRefactoring(actions []types.CodeAction, want string) (types.CodeAction, *types.ToolResult) {
	lw := strings.ToLower(strings.TrimSpace(want))
	var partial []types.CodeAction
	for _, a := range actions {
		lt := strings.ToLower(a.Title)
		if lt == lw || (a.Command != nil && a.Command.Command == want) {
			return a, nil
		}
		if strings.Contains(lt, lw) {
			partial = append(partial, a)
		}
	}
	if len(partial) == 1 {
		return partial[0], nil
	}
	titles := make([]string, 0, len(actions))
	for _, a := range actions {
		titles = append(titles, fmt.Sprintf("%q", a.Title))
	}
	msg := fmt.Sprintf("no refactoring matches %q here", want)
	if len(partial) > 1 {
		msg = fmt.Sprintf("%q matches %d refactorings; use the full title", want, len(partial))
	}
	if len(titles) > 0 {
		msg += ". Available: " + strings.Join(titles, ", ")
	}
	r := types.ErrorResult(msg)
	return types.CodeAction{}, &r
}

// formAnswers converts the answers argument ({field id: value}) to form
// answers, ordered by id so requests are deterministic.
func formAnswers(args map[string]any) []lsp.FormAnswer {
	m, _ := args["answers"].(map[string]any)
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]lsp.FormAnswer, 0, len(ids))
	for _, id := range ids {
		out = append(out, lsp.FormAnswer{ID: id, Value: m[id]})
	}
	return out
}

// refactorForm returns the server's questions to the agent. lazyEnum fields
// (a searchable option set such as workspace symbols) get their options
// fetched, filtered by option_queries[field id] when given.
func refactorForm(ctx context.Context, client *lsp.LSPClient, title string, fields []lsp.FormField, args map[string]any) types.ToolResult {
	queries, _ := args["option_queries"].(map[string]any)
	out := make([]refactorFormField, 0, len(fields))
	for _, f := range fields {
		rf := refactorFormField{ID: f.ID, Description: f.Description, Kind: f.Kind(), Required: f.Required, Default: f.Default, Error: f.Error}
		switch rf.Kind {
		case "enum":
			var t struct {
				Entries []lsp.FormEnumEntry `json:"entries"`
			}
			_ = json.Unmarshal(f.Type, &t)
			rf.Options = t.Entries
		case "lazyEnum":
			var t struct {
				Source string          `json:"source"`
				Config json.RawMessage `json:"config"`
			}
			_ = json.Unmarshal(f.Type, &t)
			q, _ := queries[f.ID].(string)
			if opts, err := client.ListEnumOptions(ctx, t.Source, t.Config, q); err == nil {
				if len(opts) > maxEnumOptions {
					opts = opts[:maxEnumOptions]
				}
				rf.Options = opts
			}
		case "string", "bool", "number":
		default:
			// file and list fields carry constraints (existence, extension
			// filters, element type) the agent needs: pass the type through.
			rf.Type = f.Type
		}
		out = append(out, rf)
	}
	encoded, _ := EncodeResultJSON(map[string]any{
		"status": "needs_input",
		"action": title,
		"fields": out,
	})
	return appendHint(encoded, fmt.Sprintf("Call refactor again with action %q and answers: {field id: value}. For a field with options, answer with an option's value; for a searchable field (kind lazyEnum), pass option_queries: {field id: search text} to narrow the options. A field with an error was rejected; correct it and call again.", title))
}
