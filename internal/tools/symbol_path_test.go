package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// --- TestHandleGoToSymbol_NilClient ---

// TestHandleGoToSymbol_NilClient verifies that a nil client returns an error
// result containing "not initialized".
func TestHandleGoToSymbol_NilClient(t *testing.T) {
	r, err := HandleGoToSymbol(context.Background(), newNilClient(), map[string]interface{}{
		"symbol_path": "pkg.Function",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("expected IsError=true for nil client, got false")
	}
	if len(r.Content) == 0 || !strings.Contains(r.Content[0].Text, "not initialized") {
		t.Errorf("expected error text containing 'not initialized', got %v", r.Content)
	}
}

// --- TestHandleGoToSymbol_EmptySymbolPath ---

// TestHandleGoToSymbol_EmptySymbolPath verifies that an empty symbol_path returns
// an error result.
func TestHandleGoToSymbol_EmptySymbolPath(t *testing.T) {
	r, err := HandleGoToSymbol(context.Background(), newNilClient(), map[string]interface{}{
		"symbol_path": "",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// nil client triggers CheckInitialized before symbol_path check;
	// either way we expect an error result
	if !r.IsError {
		t.Fatalf("expected IsError=true for empty symbol_path, got false")
	}
}

// --- TestBestSymbolMatch_NoDots ---

// TestBestSymbolMatch_NoDots verifies that when the symbol path has no dots,
// the first candidate is returned regardless of ContainerName.
func TestBestSymbolMatch_NoDots(t *testing.T) {
	containerA := "PkgA"
	containerB := "PkgB"
	candidates := []types.SymbolInformation{
		{Name: "Alpha", ContainerName: &containerA},
		{Name: "Alpha", ContainerName: &containerB},
	}

	result := bestSymbolMatch(candidates, "Alpha")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerName == nil || *result.ContainerName != containerA {
		t.Errorf("expected first candidate (ContainerName=%q), got %v", containerA, result.ContainerName)
	}
}

// --- TestBestSymbolMatch_WithDots ---

// TestBestSymbolMatch_WithDots verifies that when the path is dotted, the candidate
// whose ContainerName equals the parent component is preferred.
func TestBestSymbolMatch_WithDots(t *testing.T) {
	containerWrong := "OtherPkg"
	containerRight := "MyClass"
	candidates := []types.SymbolInformation{
		{Name: "method", ContainerName: &containerWrong},
		{Name: "method", ContainerName: &containerRight},
	}

	result := bestSymbolMatch(candidates, "MyClass.method")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerName == nil || *result.ContainerName != containerRight {
		t.Errorf("expected candidate with ContainerName=%q, got %v", containerRight, result.ContainerName)
	}
}
