package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/lsp-mcp-go/internal/lsp"
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
	if rootDir != "" {
		absRoot, _ := filepath.Abs(rootDir)
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
func CreateFileURI(filePath string) string {
	u := url.URL{
		Scheme: "file",
		Path:   filePath,
	}
	return u.String()
}

// URIToFilePath converts a file:// URI to an absolute path.
func URIToFilePath(uri string) (string, error) {
	if !strings.HasPrefix(uri, "file://") {
		return "", fmt.Errorf("not a file URI: %s", uri)
	}
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parsing URI %s: %w", uri, err)
	}
	return u.Path, nil
}

// CheckInitialized returns an error if client is nil.
func CheckInitialized(client *lsp.LSPClient) error {
	if client == nil {
		return errors.New("LSP client not initialized; call start_lsp first")
	}
	return nil
}
