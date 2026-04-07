package extensions

import (
	"regexp"
	"sync"

	"github.com/blackwell-systems/lsp-mcp-go/internal/logging"
	"github.com/blackwell-systems/lsp-mcp-go/internal/types"
)

// sanitizeLanguageID removes any character that is not alphanumeric or a hyphen.
var sanitizeRE = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// factories is the compile-time registry of extension constructors.
// Extensions register themselves by calling RegisterFactory in their init()
// functions. No concurrent writes occur after package initialisation.
//
// Design note: unlike the TypeScript implementation which supports runtime
// extension loading, Go uses compile-time factory registration via init().
// This is a deliberate architectural choice: it eliminates dynamic plugin
// loading complexity and leverages Go's type system for safety. The trade-off
// is that adding a new extension requires recompilation and a new binary.
var (
	factoriesMu sync.RWMutex
	factories   = map[string]func() types.Extension{}
)

// RegisterFactory registers an extension factory for a language ID.
// Called from extension packages' init() functions.
func RegisterFactory(languageID string, factory func() types.Extension) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[languageID] = factory
}

// ExtensionRegistry manages per-language extension plugins.
// Thread-safe via sync.RWMutex.
type ExtensionRegistry struct {
	mu         sync.RWMutex
	extensions map[string]types.Extension
}

// NewRegistry creates an empty ExtensionRegistry.
func NewRegistry() *ExtensionRegistry {
	return &ExtensionRegistry{
		extensions: map[string]types.Extension{},
	}
}

// Activate loads and registers an extension for the given language ID.
// If no extension exists for the language, returns nil (not an error).
// Language IDs are sanitized to alphanumeric + hyphens only.
func (r *ExtensionRegistry) Activate(languageID string) error {
	safe := sanitizeRE.ReplaceAllString(languageID, "")

	factoriesMu.RLock()
	factory, ok := factories[safe]
	factoriesMu.RUnlock()

	if !ok {
		logging.Log(logging.LevelDebug, "no extension factory registered for language: "+safe)
		return nil
	}

	ext := factory()

	r.mu.Lock()
	r.extensions[safe] = ext
	r.mu.Unlock()

	logging.Log(logging.LevelInfo, "activated extension for language: "+safe)
	return nil
}

// deactivate removes an extension from the registry.
func (r *ExtensionRegistry) deactivate(languageID string) {
	safe := sanitizeRE.ReplaceAllString(languageID, "")

	r.mu.Lock()
	delete(r.extensions, safe)
	r.mu.Unlock()

	logging.Log(logging.LevelInfo, "deactivated extension for language: "+safe)
}

// ToolHandlers returns the merged map of all tool handlers across active extensions.
// Keys are prefixed with "<languageID>." to avoid conflicts.
func (r *ExtensionRegistry) ToolHandlers() map[string]types.ToolHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := map[string]types.ToolHandler{}
	for langID, ext := range r.extensions {
		for name, handler := range ext.ToolHandlers() {
			result[langID+"."+name] = handler
		}
	}
	return result
}

// ResourceHandlers returns the merged map of all resource handlers across active extensions.
// Keys are prefixed with "<languageID>." to avoid conflicts.
func (r *ExtensionRegistry) ResourceHandlers() map[string]types.ResourceHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := map[string]types.ResourceHandler{}
	for langID, ext := range r.extensions {
		for name, handler := range ext.ResourceHandlers() {
			result[langID+"."+name] = handler
		}
	}
	return result
}

// SubscriptionHandlers returns the merged map of all subscription resource handlers
// across active extensions. Keys are prefixed with "<languageID>." to avoid conflicts.
func (r *ExtensionRegistry) SubscriptionHandlers() map[string]types.ResourceHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := map[string]types.ResourceHandler{}
	for langID, ext := range r.extensions {
		for name, handler := range ext.SubscriptionHandlers() {
			result[langID+"."+name] = handler
		}
	}
	return result
}

// PromptHandlers returns the merged map of all prompt handlers across active extensions.
// Keys are prefixed with "<languageID>." to avoid conflicts.
func (r *ExtensionRegistry) PromptHandlers() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := map[string]interface{}{}
	for langID, ext := range r.extensions {
		for name, handler := range ext.PromptHandlers() {
			result[langID+"."+name] = handler
		}
	}
	return result
}
