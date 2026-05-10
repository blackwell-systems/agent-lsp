package tools

import (
	"context"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// fuzzyPositionFallback retries a position-based lookup using workspace symbol
// candidates when the direct lookup returns empty results.
//
// It extracts a symbol name from hover at (line, col), searches workspace symbols
// for that name, and retries lookupFn at each candidate position. Returns the first
// non-empty result set, or an empty slice if all fallbacks also return empty.
//
// line and col are 1-indexed (tool convention); converted internally to 0-indexed.
func fuzzyPositionFallback(
	ctx context.Context,
	client *lsp.LSPClient,
	fileURI string,
	line, col int,
	lookupFn func(pos types.Position) ([]types.Location, error),
) ([]types.Location, error) {
	hoverPos := types.Position{Line: line - 1, Character: col - 1}
	hoverText, err := client.GetInfoOnLocation(ctx, fileURI, hoverPos)
	if err != nil || hoverText == "" {
		logging.Log(logging.LevelDebug, "fuzzyFallback: no hover text, skipping")
		return []types.Location{}, nil
	}

	symbolName := extractSymbolName(hoverText)
	if symbolName == "" {
		logging.Log(logging.LevelDebug, "fuzzyFallback: could not extract symbol name from hover")
		return []types.Location{}, nil
	}

	logging.Log(logging.LevelDebug, "fuzzyFallback: searching workspace symbols for "+symbolName)

	syms, symErr := client.GetWorkspaceSymbols(ctx, symbolName)
	if symErr != nil || len(syms) == 0 {
		return []types.Location{}, nil
	}

	for _, sym := range syms {
		if sym.Location.URI == "" {
			continue
		}
		candidatePos := types.Position{
			Line:      sym.Location.Range.Start.Line,
			Character: sym.Location.Range.Start.Character,
		}
		results, lErr := lookupFn(candidatePos)
		if lErr == nil && len(results) > 0 {
			logging.Log(logging.LevelDebug, "fuzzyFallback: found results via workspace symbol candidate")
			return results, nil
		}
	}

	return []types.Location{}, nil
}

// extractSymbolName parses a short identifier from hover text.
// Hover text from gopls typically starts with the symbol signature.
// We extract the first identifier-like token.
func extractSymbolName(hover string) string {
	hover = strings.TrimSpace(hover)
	if strings.HasPrefix(hover, "```") {
		lines := strings.SplitN(hover, "\n", 3)
		if len(lines) >= 2 {
			hover = lines[1]
		}
	}
	var sb strings.Builder
	for _, r := range hover {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else if sb.Len() > 0 {
			break
		}
	}
	return sb.String()
}
