package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/logging"
	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// ResourceResult is returned by resource read handlers.
type ResourceResult struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType"`
	Text     string `json:"text"`
}

// ResourceTemplate describes a URI template for dynamic resource access.
type ResourceTemplate struct {
	Name        string `json:"name"`
	URITemplate string `json:"uriTemplate"`
	Description string `json:"description"`
}

// HandleDiagnosticsResource handles lsp-diagnostics:// reads.
// If the URI has a non-root path (e.g. lsp-diagnostics:///path/to/file),
// returns diagnostics for that file. If path is empty or "/", returns all.
func HandleDiagnosticsResource(ctx context.Context, client *lsp.LSPClient, uri string) (ResourceResult, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid URI %q: %w", uri, err)
	}

	path := parsed.Path
	if path == "" || path == "/" {
		// Return diagnostics for all open documents.
		if err := client.ReopenAllDocuments(ctx); err != nil {
			logging.Log(logging.LevelWarning, fmt.Sprintf("ReopenAllDocuments: %v", err))
		}

		openDocs := client.GetOpenDocuments()
		uris := make([]string, 0, len(openDocs))
		for _, docURI := range openDocs {
			if strings.HasPrefix(docURI, "file://") {
				uris = append(uris, docURI)
			}
		}

		if err := lsp.WaitForDiagnostics(ctx, client, uris, 10000); err != nil {
			return ResourceResult{}, fmt.Errorf("waiting for diagnostics: %w", err)
		}

		allDiag := client.GetAllDiagnostics()
		result := make(map[string][]types.LSPDiagnostic)
		for _, docURI := range uris {
			diags := allDiag[docURI]
			if diags == nil {
				diags = []types.LSPDiagnostic{}
			}
			result[docURI] = diags
		}

		text, err := json.Marshal(result)
		if err != nil {
			return ResourceResult{}, fmt.Errorf("marshal diagnostics: %w", err)
		}
		return ResourceResult{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(text),
		}, nil
	}

	// Specific file path.
	fileURI := (&url.URL{Scheme: "file", Path: path}).String()
	if err := client.ReopenDocument(ctx, fileURI); err != nil {
		return ResourceResult{}, fmt.Errorf("reopen document %q: %w", fileURI, err)
	}

	if err := lsp.WaitForDiagnostics(ctx, client, []string{fileURI}, 10000); err != nil {
		return ResourceResult{}, fmt.Errorf("waiting for diagnostics: %w", err)
	}

	diags := client.GetDiagnostics(fileURI)
	if diags == nil {
		diags = []types.LSPDiagnostic{}
	}
	result := map[string][]types.LSPDiagnostic{fileURI: diags}
	text, err := json.Marshal(result)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("marshal diagnostics: %w", err)
	}
	return ResourceResult{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(text),
	}, nil
}

// parseResourceQueryParams parses the file path, position, and language ID
// from an lsp-hover:// or lsp-completions:// URI.
// Returns an error if required query params are missing or invalid.
func parseResourceQueryParams(uri string) (filePath string, pos types.Position, languageID string, err error) {
	parsed, pErr := url.Parse(uri)
	if pErr != nil {
		return "", types.Position{}, "", fmt.Errorf("invalid URI %q: %w", uri, pErr)
	}

	filePath = parsed.Path
	q := parsed.Query()

	lineStr := q.Get("line")
	colStr := q.Get("column")
	languageID = q.Get("language_id")

	if lineStr == "" || colStr == "" || languageID == "" {
		return "", types.Position{}, "", fmt.Errorf("URI missing required query params (line, column, language_id)")
	}

	line, lErr := strconv.Atoi(lineStr)
	if lErr != nil {
		return "", types.Position{}, "", fmt.Errorf("invalid line %q: %w", lineStr, lErr)
	}
	col, cErr := strconv.Atoi(colStr)
	if cErr != nil {
		return "", types.Position{}, "", fmt.Errorf("invalid column %q: %w", colStr, cErr)
	}

	// URI params are 1-indexed; LSP is 0-indexed.
	pos = types.Position{Line: line - 1, Character: col - 1}
	return filePath, pos, languageID, nil
}

// HandleHoverResource handles lsp-hover:// reads.
// URI format: lsp-hover:///path/to/file?line=N&column=N&language_id=X
func HandleHoverResource(ctx context.Context, client *lsp.LSPClient, uri string) (ResourceResult, error) {
	filePath, pos, languageID, err := parseResourceQueryParams(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("hover resource: %w", err)
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("read file %q: %w", filePath, err)
	}

	fileURI := (&url.URL{Scheme: "file", Path: filePath}).String()
	if err := client.OpenDocument(ctx, fileURI, string(fileContent), languageID); err != nil {
		return ResourceResult{}, fmt.Errorf("open document %q: %w", fileURI, err)
	}

	hoverText, err := client.GetInfoOnLocation(ctx, fileURI, pos)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("get hover: %w", err)
	}

	return ResourceResult{
		URI:      uri,
		MIMEType: "text/plain",
		Text:     hoverText,
	}, nil
}

// HandleCompletionsResource handles lsp-completions:// reads.
// URI format: lsp-completions:///path/to/file?line=N&column=N&language_id=X
func HandleCompletionsResource(ctx context.Context, client *lsp.LSPClient, uri string) (ResourceResult, error) {
	filePath, pos, languageID, err := parseResourceQueryParams(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("completions resource: %w", err)
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("read file %q: %w", filePath, err)
	}

	fileURI := (&url.URL{Scheme: "file", Path: filePath}).String()
	if err := client.OpenDocument(ctx, fileURI, string(fileContent), languageID); err != nil {
		return ResourceResult{}, fmt.Errorf("open document %q: %w", fileURI, err)
	}

	completions, err := client.GetCompletion(ctx, fileURI, pos)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("get completions: %w", err)
	}

	text, err := json.Marshal(completions)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("marshal completions: %w", err)
	}

	return ResourceResult{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(text),
	}, nil
}

// ResourceTemplates returns the static resource template definitions
// for the MCP server's resources/templates/list response.
func ResourceTemplates() []ResourceTemplate {
	return []ResourceTemplate{
		{
			Name:        "lsp-diagnostics",
			URITemplate: "lsp-diagnostics:///{filePath}",
			Description: "LSP diagnostics for a specific file. Leave filePath empty for all open files.",
		},
		{
			Name:        "lsp-hover",
			URITemplate: "lsp-hover:///{filePath}?line={line}&column={column}&language_id={language_id}",
			Description: "LSP hover information at a specific position in a file.",
		},
		{
			Name:        "lsp-completions",
			URITemplate: "lsp-completions:///{filePath}?line={line}&column={column}&language_id={language_id}",
			Description: "LSP code completions at a specific position in a file.",
		},
	}
}
