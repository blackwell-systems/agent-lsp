// helpers.go contains shared utilities used across all tool handlers:
//
//   - ValidateFilePath: path traversal prevention (resolves symlinks, checks
//     workspace root boundary). Used by every tool that accepts a file_path arg.
//   - WithDocument: convenience wrapper that reads a file from disk, opens it
//     in the LSP server, and calls a callback. Handles the common open-then-query
//     pattern used by navigation and analysis tools.
//   - CreateFileURI / URIToFilePath: file path <-> file:// URI conversion.
//   - CheckInitialized: guard that returns a clear error when the LSP client
//     hasn't been started yet.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gcf "github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
	uriPkg "github.com/blackwell-systems/agent-lsp/internal/uri"
	gcfgo "github.com/blackwell-systems/gcf-go"
)

// ValidateFilePath resolves filePath to a clean absolute path and, when rootDir
// is non-empty, verifies the result is within the workspace root. This prevents
// path traversal attacks (e.g. "../../etc/passwd").
func ValidateFilePath(filePath, rootDir string) (string, error) {
	if filePath == "" {
		return "", errors.New("file_path is required")
	}
	clean, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}
	// L2: Resolve symlinks so in-workspace symlinks to out-of-workspace targets
	// do not bypass the prefix check. EvalSymlinks errors on non-existent paths;
	// fall back to lexical path to allow validation of not-yet-created files.
	if resolved, evalErr := filepath.EvalSymlinks(clean); evalErr == nil {
		clean = resolved
	}
	if rootDir != "" {
		absRoot, _ := filepath.Abs(rootDir)
		if resolvedRoot, evalErr := filepath.EvalSymlinks(absRoot); evalErr == nil {
			absRoot = resolvedRoot
		}
		if clean != absRoot && !strings.HasPrefix(clean, absRoot+string(filepath.Separator)) {
			return "", fmt.Errorf("file path %q is outside workspace root %q", clean, absRoot)
		}
	}
	return clean, nil
}

// WithDocument reads filePath from disk, opens it on the LSP client, then calls cb.
// T is the callback return type. On error, returns zero value of T and the error.
func WithDocument[T any](
	ctx context.Context,
	client *lsp.LSPClient,
	filePath string,
	languageID string,
	cb func(fileURI string) (T, error),
) (T, error) {
	var zero T

	clean, err := ValidateFilePath(filePath, client.RootDir())
	if err != nil {
		return zero, err
	}
	filePath = clean

	content, err := os.ReadFile(filePath)
	if err != nil {
		return zero, fmt.Errorf("reading file %s: %w", filePath, err)
	}

	fileURI := CreateFileURI(filePath)

	if err := client.OpenDocument(ctx, fileURI, string(content), languageID); err != nil {
		return zero, fmt.Errorf("opening document %s: %w", filePath, err)
	}

	return cb(fileURI)
}

// CreateFileURI converts an absolute file path to a file:// URI.
//
// Delegates to lsp.PathToFileURI — the canonical helper that handles
// both POSIX and Windows correctly. The previous implementation here
// did `url.URL{Scheme: "file", Path: filePath}.String()` which on
// Windows produced `file://S:/Source/...` (drive letter promoted to
// the URI authority) instead of the RFC 8089 form `file:///S:/...`.
// Downstream LSP servers then either failed to resolve the URI at all
// or returned references under a totally different normalization,
// silently dropping cross-file callers in find_references and similar.
func CreateFileURI(filePath string) string {
	return lsp.PathToFileURI(filePath)
}

// URIToFilePath converts a file:// URI to an absolute path.
// Delegates to uri.URIToPath — canonical shared implementation (M3).
func URIToFilePath(rawURI string) (string, error) {
	if !strings.HasPrefix(rawURI, "file://") {
		return "", fmt.Errorf("not a file URI: %s", rawURI)
	}
	return uriPkg.URIToPath(rawURI), nil
}

// CheckInitialized returns an error if client is nil.
func CheckInitialized(client *lsp.LSPClient) error {
	if client == nil {
		return errors.New("LSP client not initialized; call start_lsp first")
	}
	return nil
}

// appendHint adds a next-step hint to a tool result's text content.
// The hint is appended as a separate line after the main content.
// Error results and empty hints/content are returned unchanged.
func appendHint(result types.ToolResult, hint string) types.ToolResult {
	if hint == "" || result.IsError || len(result.Content) == 0 || result.Content[0].Text == "" {
		return result
	}
	// Add the hint as a separate content item so it never interferes with
	// JSON parsing of the primary result. Agents see both items; parsers
	// that only read Content[0] are unaffected.
	result.Content = append(result.Content, types.ContentItem{
		Type: "text",
		Text: "Next step: " + hint,
	})
	return result
}

// outputFormatKey is a context key for the output format preference.
type outputFormatKey struct{}

// ContextWithOutputFormat returns a new context with the output format set.
func ContextWithOutputFormat(ctx context.Context, format string) context.Context {
	return context.WithValue(ctx, outputFormatKey{}, format)
}

// OutputFormatFromContext returns the output format from context, defaulting to "json".
func OutputFormatFromContext(ctx context.Context) string {
	if ctx == nil {
		return "json"
	}
	if v, ok := ctx.Value(outputFormatKey{}).(string); ok && v != "" {
		return v
	}
	return "json"
}

// EncodeResult marshals data as JSON or GCF based on the output format in ctx.
// Falls back to JSON if GCF encoding fails or format is unrecognized.
func EncodeResult(ctx context.Context, data any) (types.ToolResult, error) {
	format := OutputFormatFromContext(ctx)
	switch format {
	case "gcf":
		// Graph-profile: if data is already a *gcf.Payload, use graph encoding
		if p, ok := data.(*gcfgo.Payload); ok {
			encoded, err := gcf.EncodeGraph(p)
			if err != nil {
				raw, _ := json.Marshal(data)
				return types.TextResult(string(raw)), nil
			}
			return types.TextResult(encoded), nil
		}
		// Tabular fallback for non-Payload data
		encoded, err := gcf.Encode(data)
		if err != nil {
			// Fall back to JSON on GCF encoding failure
			raw, _ := json.Marshal(data)
			return types.TextResult(string(raw)), nil
		}
		return types.TextResult(encoded), nil
	default:
		raw, err := json.Marshal(data)
		if err != nil {
			return types.ErrorResult(err.Error()), nil
		}
		return types.TextResult(string(raw)), nil
	}
}

// EncodeResultJSON marshals data as JSON regardless of the context output format.
// Use it for any payload the caller must round-trip byte-exact, in particular a
// WorkspaceEdit that gets handed back to apply_edit. GCF's tabular form is a
// summary format: it flattens a TextEdit's range into positional path columns
// (range>end>character, range>end>line, ...) and requires the newText to be
// copied verbatim out of tool output and back into a tool argument. LLM callers
// corrupt that round-trip, transposing the range offsets and truncating a large
// newText, which overwrites the file with a mangled edit. Edits therefore always
// serialize as self-labeling JSON. See issue #12.
func EncodeResultJSON(data any) (types.ToolResult, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return types.ErrorResult(err.Error()), nil
	}
	return types.TextResult(string(raw)), nil
}
