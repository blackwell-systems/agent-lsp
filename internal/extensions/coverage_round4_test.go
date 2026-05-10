package extensions

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// fullMockExtension implements all Extension methods with non-nil returns.
type fullMockExtension struct {
	tools         map[string]types.ToolHandler
	resources     map[string]types.ResourceHandler
	subscriptions map[string]types.ResourceHandler
	prompts       map[string]any
}

func (m *fullMockExtension) ToolHandlers() map[string]types.ToolHandler {
	return m.tools
}

func (m *fullMockExtension) ResourceHandlers() map[string]types.ResourceHandler {
	return m.resources
}

func (m *fullMockExtension) SubscriptionHandlers() map[string]types.ResourceHandler {
	return m.subscriptions
}

func (m *fullMockExtension) PromptHandlers() map[string]any {
	return m.prompts
}

func TestRegistry_SubscriptionHandlers_Prefixed(t *testing.T) {
	resetFactories()

	subHandler := func(ctx any, uri string) (any, error) {
		return "subscribed", nil
	}

	RegisterFactory("python", func() types.Extension {
		return &fullMockExtension{
			tools:     map[string]types.ToolHandler{},
			resources: map[string]types.ResourceHandler{},
			subscriptions: map[string]types.ResourceHandler{
				"diagnostics": subHandler,
			},
			prompts: map[string]any{},
		}
	})

	r := NewRegistry()
	if err := r.Activate("python"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handlers := r.SubscriptionHandlers()
	if _, ok := handlers["python.diagnostics"]; !ok {
		t.Fatalf("expected key 'python.diagnostics', got keys: %v", keys(handlers))
	}
	if len(handlers) != 1 {
		t.Fatalf("expected 1 subscription handler, got %d", len(handlers))
	}
}

func TestRegistry_PromptHandlers_Prefixed(t *testing.T) {
	resetFactories()

	RegisterFactory("rust", func() types.Extension {
		return &fullMockExtension{
			tools:         map[string]types.ToolHandler{},
			resources:     map[string]types.ResourceHandler{},
			subscriptions: map[string]types.ResourceHandler{},
			prompts: map[string]any{
				"explain": "explain prompt handler",
			},
		}
	})

	r := NewRegistry()
	if err := r.Activate("rust"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handlers := r.PromptHandlers()
	if _, ok := handlers["rust.explain"]; !ok {
		t.Fatalf("expected key 'rust.explain', got keys: %v", keysAny(handlers))
	}
	if len(handlers) != 1 {
		t.Fatalf("expected 1 prompt handler, got %d", len(handlers))
	}
}

func TestRegistry_EmptySubscriptionAndPromptHandlers(t *testing.T) {
	resetFactories()
	r := NewRegistry()

	if len(r.SubscriptionHandlers()) != 0 {
		t.Error("expected 0 subscription handlers on empty registry")
	}
	if len(r.PromptHandlers()) != 0 {
		t.Error("expected 0 prompt handlers on empty registry")
	}
}

func TestRegistry_MultipleLanguages_MergedHandlers(t *testing.T) {
	resetFactories()

	RegisterFactory("go", func() types.Extension {
		return &fullMockExtension{
			tools:         map[string]types.ToolHandler{},
			resources:     map[string]types.ResourceHandler{},
			subscriptions: map[string]types.ResourceHandler{},
			prompts:       map[string]any{"p1": "go-prompt"},
		}
	})
	RegisterFactory("rust", func() types.Extension {
		return &fullMockExtension{
			tools:         map[string]types.ToolHandler{},
			resources:     map[string]types.ResourceHandler{},
			subscriptions: map[string]types.ResourceHandler{},
			prompts:       map[string]any{"p1": "rust-prompt"},
		}
	})

	r := NewRegistry()
	_ = r.Activate("go")
	_ = r.Activate("rust")

	prompts := r.PromptHandlers()
	if len(prompts) != 2 {
		t.Fatalf("expected 2 merged prompt handlers, got %d", len(prompts))
	}
	if _, ok := prompts["go.p1"]; !ok {
		t.Error("missing go.p1 prompt handler")
	}
	if _, ok := prompts["rust.p1"]; !ok {
		t.Error("missing rust.p1 prompt handler")
	}
}

func TestRegistry_Activate_SanitizesLanguageID(t *testing.T) {
	resetFactories()

	RegisterFactory("typescript", func() types.Extension {
		return &fullMockExtension{
			tools:         map[string]types.ToolHandler{},
			resources:     map[string]types.ResourceHandler{},
			subscriptions: map[string]types.ResourceHandler{},
			prompts:       map[string]any{},
		}
	})

	r := NewRegistry()
	// Inject special characters that should be stripped
	err := r.Activate("type$script!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After sanitization, "type$script!" becomes "typescript"
	if _, ok := r.extensions["typescript"]; !ok {
		t.Error("expected sanitized language ID 'typescript' in registry")
	}
}

func TestRegistry_ResourceHandlers_Prefixed(t *testing.T) {
	resetFactories()

	resHandler := func(ctx any, uri string) (any, error) {
		return "resource data", nil
	}

	RegisterFactory("java", func() types.Extension {
		return &fullMockExtension{
			tools: map[string]types.ToolHandler{},
			resources: map[string]types.ResourceHandler{
				"classpath": resHandler,
			},
			subscriptions: map[string]types.ResourceHandler{},
			prompts:       map[string]any{},
		}
	})

	r := NewRegistry()
	_ = r.Activate("java")

	handlers := r.ResourceHandlers()
	if _, ok := handlers["java.classpath"]; !ok {
		t.Fatalf("expected key 'java.classpath', got keys: %v", keysRes(handlers))
	}
}

func keysAny(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysRes(m map[string]types.ResourceHandler) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
