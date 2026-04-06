package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// ResourceResult is returned by resource read handlers.
type ResourceResult struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType"`
	Text     string `json:"text"`
}

// ResourceEntry describes a single available MCP resource.
type ResourceEntry struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Subscribe   bool   `json:"subscribe"`
	Template    bool   `json:"template,omitempty"`
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
	fileURI := "file://" + path
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

// HandleHoverResource handles lsp-hover:// reads.
// URI format: lsp-hover:///path/to/file?line=N&column=N&language_id=X
func HandleHoverResource(ctx context.Context, client *lsp.LSPClient, uri string) (ResourceResult, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid URI %q: %w", uri, err)
	}

	filePath := parsed.Path
	q := parsed.Query()

	lineStr := q.Get("line")
	colStr := q.Get("column")
	languageID := q.Get("language_id")

	if lineStr == "" || colStr == "" || languageID == "" {
		return ResourceResult{}, fmt.Errorf("hover URI missing required query params (line, column, language_id)")
	}

	line, err := strconv.Atoi(lineStr)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid line %q: %w", lineStr, err)
	}
	col, err := strconv.Atoi(colStr)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid column %q: %w", colStr, err)
	}

	// URI params are 1-indexed; LSP is 0-indexed.
	pos := types.Position{Line: line - 1, Character: col - 1}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("read file %q: %w", filePath, err)
	}

	fileURI := "file://" + filePath
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
	parsed, err := url.Parse(uri)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid URI %q: %w", uri, err)
	}

	filePath := parsed.Path
	q := parsed.Query()

	lineStr := q.Get("line")
	colStr := q.Get("column")
	languageID := q.Get("language_id")

	if lineStr == "" || colStr == "" || languageID == "" {
		return ResourceResult{}, fmt.Errorf("completions URI missing required query params (line, column, language_id)")
	}

	line, err := strconv.Atoi(lineStr)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid line %q: %w", lineStr, err)
	}
	col, err := strconv.Atoi(colStr)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("invalid column %q: %w", colStr, err)
	}

	// URI params are 1-indexed; LSP is 0-indexed.
	pos := types.Position{Line: line - 1, Character: col - 1}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return ResourceResult{}, fmt.Errorf("read file %q: %w", filePath, err)
	}

	fileURI := "file://" + filePath
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

// generateResourceList returns the dynamic list of available resources
// based on currently open documents. Returns nil if client is nil.
func generateResourceList(client *lsp.LSPClient) []ResourceEntry {
	if client == nil {
		return nil
	}

	entries := []ResourceEntry{
		{
			URI:         "lsp-diagnostics://",
			Name:        "All Diagnostics",
			Description: "LSP diagnostics for all open documents",
			Subscribe:   true,
		},
	}

	for _, docURI := range client.GetOpenDocuments() {
		if !strings.HasPrefix(docURI, "file://") {
			continue
		}
		// Strip the file:// prefix to get the absolute path.
		filePath := strings.TrimPrefix(docURI, "file://")
		baseName := filepath.Base(filePath)

		entries = append(entries,
			ResourceEntry{
				URI:         "lsp-diagnostics://" + filePath,
				Name:        "Diagnostics: " + baseName,
				Description: "LSP diagnostics for " + filePath,
				Subscribe:   true,
			},
			ResourceEntry{
				URI:         fmt.Sprintf("lsp-hover://%s?line={line}&column={column}&language_id={language_id}", filePath),
				Name:        "Hover: " + baseName,
				Description: "LSP hover information for " + filePath,
				Template:    true,
			},
			ResourceEntry{
				URI:         fmt.Sprintf("lsp-completions://%s?line={line}&column={column}&language_id={language_id}", filePath),
				Name:        "Completions: " + baseName,
				Description: "LSP completions for " + filePath,
				Template:    true,
			},
		)
	}

	return entries
}

// resourceTemplates returns the static resource template definitions
// for the MCP server's resources/templates/list response.
func resourceTemplates() []ResourceTemplate {
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
