package haskell

import (
	"github.com/blackwell-systems/agent-lsp/internal/extensions"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func init() {
	extensions.RegisterFactory("haskell", func() types.Extension { return &HaskellExtension{} })
}

// HaskellExtension is the per-language extension for Haskell.
// Tool and resource handlers are stubs; a full implementation would wire in
// HLS-specific operations here.
type HaskellExtension struct{}

func (h *HaskellExtension) ToolHandlers() map[string]types.ToolHandler         { return nil }
func (h *HaskellExtension) ResourceHandlers() map[string]types.ResourceHandler { return nil }
func (h *HaskellExtension) SubscriptionHandlers() map[string]types.ResourceHandler { return nil }
func (h *HaskellExtension) PromptHandlers() map[string]interface{}              { return nil }
