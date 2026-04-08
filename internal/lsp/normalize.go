package lsp

import (
	"bytes"
	"encoding/json"

	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// NormalizeDocumentSymbols converts a raw LSP documentSymbol response
// (DocumentSymbol[] | SymbolInformation[]) into []types.DocumentSymbol.
func NormalizeDocumentSymbols(raw json.RawMessage) ([]types.DocumentSymbol, error) {
	if raw == nil || string(raw) == "null" {
		return []types.DocumentSymbol{}, nil
	}

	// Probe the array to determine the variant.
	var probe []json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	if len(probe) == 0 {
		return []types.DocumentSymbol{}, nil
	}

	// Discriminate: DocumentSymbol has "selectionRange"; SymbolInformation does not.
	var disc struct {
		SelectionRange *json.RawMessage `json:"selectionRange"`
	}
	if err := json.Unmarshal(probe[0], &disc); err != nil {
		return nil, err
	}

	if disc.SelectionRange != nil {
		// Server returned DocumentSymbol[].
		var syms []types.DocumentSymbol
		if err := json.Unmarshal(raw, &syms); err != nil {
			return nil, err
		}
		return syms, nil
	}

	// Server returned SymbolInformation[]. Two-pass tree reconstruction.
	var infos []types.SymbolInformation
	if err := json.Unmarshal(raw, &infos); err != nil {
		return nil, err
	}

	// Pass 1: create DocumentSymbol for each item and build name map.
	nameMap := make(map[string]*types.DocumentSymbol, len(infos))
	symPtrs := make([]*types.DocumentSymbol, len(infos))
	for i, info := range infos {
		ds := &types.DocumentSymbol{
			Name:           info.Name,
			Kind:           info.Kind,
			Tags:           info.Tags,
			Range:          info.Location.Range,
			SelectionRange: info.Location.Range,
		}
		symPtrs[i] = ds
		nameMap[info.Name] = ds // last write wins on duplicates
	}

	// Pass 2: attach children to parents; collect roots.
	var roots []types.DocumentSymbol
	for i, info := range infos {
		ds := symPtrs[i]
		if info.ContainerName != nil && *info.ContainerName != "" {
			if parent, ok := nameMap[*info.ContainerName]; ok {
				parent.Children = append(parent.Children, *ds)
				continue
			}
		}
		roots = append(roots, *ds)
	}

	if roots == nil {
		return []types.DocumentSymbol{}, nil
	}
	return roots, nil
}

// NormalizeCompletion converts a raw LSP completion response
// (CompletionList | CompletionItem[]) into types.CompletionList.
func NormalizeCompletion(raw json.RawMessage) (types.CompletionList, error) {
	if raw == nil || string(raw) == "null" {
		return types.CompletionList{Items: []types.CompletionItem{}}, nil
	}

	// Discriminate: CompletionList has an "items" field.
	var probe struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &probe); err == nil && probe.Items != nil {
		// Server returned a CompletionList.
		var cl struct {
			IsIncomplete bool                   `json:"isIncomplete"`
			Items        []types.CompletionItem `json:"items"`
		}
		if err := json.Unmarshal(raw, &cl); err != nil {
			return types.CompletionList{}, err
		}
		if cl.Items == nil {
			cl.Items = []types.CompletionItem{}
		}
		return types.CompletionList{IsIncomplete: cl.IsIncomplete, Items: cl.Items}, nil
	}

	// Server returned CompletionItem[].
	var items []types.CompletionItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return types.CompletionList{}, err
	}
	if items == nil {
		items = []types.CompletionItem{}
	}
	return types.CompletionList{IsIncomplete: false, Items: items}, nil
}

// NormalizeCodeActions converts a raw LSP codeAction response
// ((Command | CodeAction)[]) into []types.CodeAction.
func NormalizeCodeActions(raw json.RawMessage) ([]types.CodeAction, error) {
	if raw == nil || string(raw) == "null" {
		return []types.CodeAction{}, nil
	}

	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, err
	}

	out := make([]types.CodeAction, 0, len(elements))
	for _, elem := range elements {
		var disc struct {
			Title   string          `json:"title"`
			Command json.RawMessage `json:"command"`
			Kind    *string         `json:"kind"`
		}
		if err := json.Unmarshal(elem, &disc); err != nil {
			continue
		}

		// Discriminate: if "command" is a JSON string (not an object), this is a
		// bare Command. The reliable check is whether the first non-whitespace byte
		// is a double-quote character.
		if len(disc.Command) > 0 && bytes.TrimSpace(disc.Command)[0] == '"' {
			// Bare Command — synthesize a CodeAction.
			var cmd types.Command
			if err := json.Unmarshal(elem, &cmd); err != nil {
				continue
			}
			out = append(out, types.CodeAction{Title: cmd.Title, Command: &cmd})
		} else {
			// CodeAction (command field is absent, null, or an object).
			var ca types.CodeAction
			if err := json.Unmarshal(elem, &ca); err != nil {
				continue
			}
			out = append(out, ca)
		}
	}

	return out, nil
}
