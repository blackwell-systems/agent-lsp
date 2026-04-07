package extensions

import (
	"testing"

	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// resetFactories clears the factory map between tests to avoid cross-test pollution.
func resetFactories() {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories = map[string]func() types.Extension{}
}

// mockExtension is a test double for types.Extension.
type mockExtension struct {
	toolHandlers     map[string]types.ToolHandler
	resourceHandlers map[string]types.ResourceHandler
}

func (m *mockExtension) ToolHandlers() map[string]types.ToolHandler {
	return m.toolHandlers
}

func (m *mockExtension) ResourceHandlers() map[string]types.ResourceHandler {
	return m.resourceHandlers
}

func (m *mockExtension) SubscriptionHandlers() map[string]types.ResourceHandler {
	return nil
}

func (m *mockExtension) PromptHandlers() map[string]interface{} {
	return nil
}

// TestNewRegistry_Empty verifies that a freshly created registry has no extensions.
func TestNewRegistry_Empty(t *testing.T) {
	r := NewRegistry()
	if len(r.extensions) != 0 {
		t.Fatalf("expected empty registry, got %d extensions", len(r.extensions))
	}
	if len(r.ToolHandlers()) != 0 {
		t.Fatalf("expected no tool handlers in empty registry")
	}
	if len(r.ResourceHandlers()) != 0 {
		t.Fatalf("expected no resource handlers in empty registry")
	}
}

// TestRegistry_Activate_UnknownLanguage verifies that activating an unregistered language
// returns nil (not an error) and adds no extension to the registry.
func TestRegistry_Activate_UnknownLanguage(t *testing.T) {
	resetFactories()
	r := NewRegistry()

	if err := r.Activate("cobol"); err != nil {
		t.Fatalf("expected nil error for unknown language, got: %v", err)
	}
	if len(r.extensions) != 0 {
		t.Fatalf("expected no extension to be added for unknown language")
	}
}

// TestRegistry_Activate_KnownLanguage verifies that Activate installs an extension when
// a factory has been registered for that language ID.
func TestRegistry_Activate_KnownLanguage(t *testing.T) {
	resetFactories()
	RegisterFactory("go", func() types.Extension {
		return &mockExtension{}
	})

	r := NewRegistry()
	if err := r.Activate("go"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.extensions["go"]; !ok {
		t.Fatal("expected extension to be registered under 'go'")
	}
}

// TestRegistry_ToolHandlers_Prefixed verifies that tool handler keys are prefixed with
// the language ID followed by a dot.
func TestRegistry_ToolHandlers_Prefixed(t *testing.T) {
	resetFactories()

	handler := func(ctx interface{}, args map[string]interface{}) (types.ToolResult, error) {
		return types.TextResult("ok"), nil
	}
	RegisterFactory("rust", func() types.Extension {
		return &mockExtension{
			toolHandlers: map[string]types.ToolHandler{
				"check": handler,
			},
		}
	})

	r := NewRegistry()
	if err := r.Activate("rust"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handlers := r.ToolHandlers()
	if _, ok := handlers["rust.check"]; !ok {
		t.Fatalf("expected key 'rust.check', got keys: %v", keys(handlers))
	}
	if len(handlers) != 1 {
		t.Fatalf("expected exactly 1 handler, got %d", len(handlers))
	}
}

// keys is a small helper to format map keys for test error messages.
func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
