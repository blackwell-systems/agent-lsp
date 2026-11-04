package lsp

// interactive.go implements the client side of the interactive refactoring
// protocol extension (golang/go#76331, microsoft/language-server-protocol#1164),
// which gopls v0.24 uses for refactorings that need input before they can
// compute an edit: which struct tags to add, which interface to implement,
// which file a declaration moves to.
//
// The server describes the input it needs as a form of typed fields in reply
// to "command/resolve"; the client answers and resolves again until the server
// returns no fields, then executes the command. Agents fill these forms
// directly, so the form is passed through to the tool caller rather than
// rendered as a dialog.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// interactiveInputTypes are the form field kinds agent-lsp accepts: all of
// them, since an agent answers each with a JSON value.
var interactiveInputTypes = []string{"string", "file", "bool", "number", "enum", "lazyEnum", "list"}

// FormField is one question in an interactive form. Type is kept raw: its
// "kind" selects the shape (enum entries, file constraints, lazyEnum source,
// list element type), and the caller passes it through.
type FormField struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Type        json.RawMessage `json:"type"`
	Required    bool            `json:"required"`
	Default     any             `json:"default,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// Kind returns the field's input kind ("string", "enum", "lazyEnum", ...).
func (f FormField) Kind() string {
	var t struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(f.Type, &t)
	return t.Kind
}

// FormAnswer answers the FormField with the same ID.
type FormAnswer struct {
	ID    string `json:"id"`
	Value any    `json:"value"`
}

// FormEnumEntry is one option of an enum or lazyEnum field.
type FormEnumEntry struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

// ResolvedCommand is a command after "command/resolve": FormFields is empty
// once the answers are complete and valid, and the command can be executed.
type ResolvedCommand struct {
	Command     string       `json:"command"`
	Arguments   []any        `json:"arguments,omitempty"`
	FormFields  []FormField  `json:"formFields,omitempty"`
	FormAnswers []FormAnswer `json:"formAnswers,omitempty"`
}

// SupportsInteractiveResolve reports whether the server resolves commands
// interactively (experimental.interactiveResolveProvider with kind "command").
func (c *LSPClient) SupportsInteractiveResolve() bool {
	exp, _ := c.getCapabilityRaw("experimental").(map[string]any)
	prov, _ := exp["interactiveResolveProvider"].(map[string]any)
	kinds, _ := prov["kinds"].([]any)
	for _, k := range kinds {
		if k == "command" {
			return true
		}
	}
	return false
}

// ResolveCommand sends "command/resolve" with the answers so far. The reply
// carries the questions still open (with validation errors on rejected
// answers), or no questions once the command is ready to execute.
func (c *LSPClient) ResolveCommand(ctx context.Context, command string, args []any, answers []FormAnswer) (*ResolvedCommand, error) {
	params := map[string]any{"command": command, "arguments": args}
	if len(answers) > 0 {
		params["formAnswers"] = answers
	}
	raw, err := c.sendRequest(ctx, "command/resolve", params)
	if err != nil {
		return nil, err
	}
	var out ResolvedCommand
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("command/resolve: %w", err)
	}
	if out.Command == "" {
		out.Command = command
	}
	return &out, nil
}

// ListEnumOptions sends "interactive/listEnum" for a lazyEnum field: the
// options matching query from the field's source (for example
// "workspace/symbol"). source and config are echoed from the field's type.
func (c *LSPClient) ListEnumOptions(ctx context.Context, source string, config json.RawMessage, query string) ([]FormEnumEntry, error) {
	params := map[string]any{"source": source, "query": query}
	if len(config) > 0 && string(config) != "null" {
		params["config"] = config
	}
	raw, err := c.sendRequest(ctx, "interactive/listEnum", params)
	if err != nil {
		return nil, err
	}
	var entries []FormEnumEntry
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("interactive/listEnum: %w", err)
		}
	}
	return entries, nil
}

// editCapture collects the edits a server sends through workspace/applyEdit
// while a command runs, instead of writing them, so the caller can preview
// them or apply them itself.
type editCapture struct {
	mu    sync.Mutex
	edits []any
}

// ExecuteCommandCapturingEdits runs command with its form answers and returns
// the workspace edits the server asked the client to apply while it ran,
// without applying them. The server is told each edit was applied, since it
// computes the edit and leaves writing it to the client; the caller must
// apply the returned edits (or discard them for a preview).
//
// Captures are serialized: an applyEdit that arrives during one belongs to it.
func (c *LSPClient) ExecuteCommandCapturingEdits(ctx context.Context, command string, args []any, answers []FormAnswer) ([]any, error) {
	c.captureRunMu.Lock()
	defer c.captureRunMu.Unlock()

	capture := &editCapture{}
	c.captureMu.Lock()
	c.capture = capture
	c.captureMu.Unlock()
	defer func() {
		c.captureMu.Lock()
		c.capture = nil
		c.captureMu.Unlock()
	}()

	params := map[string]any{"command": command, "arguments": args}
	if len(answers) > 0 {
		params["formAnswers"] = answers
	}
	if _, err := c.sendRequest(ctx, "workspace/executeCommand", params); err != nil {
		return nil, err
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]any(nil), capture.edits...), nil
}

// captureEdit records edit if a capture is running and reports whether it did.
func (c *LSPClient) captureEdit(edit any) bool {
	c.captureMu.Lock()
	capture := c.capture
	c.captureMu.Unlock()
	if capture == nil {
		return false
	}
	capture.mu.Lock()
	capture.edits = append(capture.edits, edit)
	capture.mu.Unlock()
	return true
}
