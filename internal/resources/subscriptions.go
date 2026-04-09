package resources

import (
	"context"
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// NotifyFunc is called by the subscription handler to send resource update notifications.
// The caller (Wave 2 server.go) provides the actual MCP notification function.
type NotifyFunc func(uri string)

// SubscriptionContext holds state for an active resource subscription.
type SubscriptionContext struct {
	Callback types.DiagnosticUpdateCallback
}

// HandleSubscribeDiagnostics sets up a diagnostic subscription.
// Returns a SubscriptionContext to pass to HandleUnsubscribeDiagnostics.
func HandleSubscribeDiagnostics(
	ctx context.Context,
	client *lsp.LSPClient,
	uri string,
	notify NotifyFunc,
) (*SubscriptionContext, error) {
	// Determine if this is a specific-file or all-files subscription.
	// URI format: lsp-diagnostics:///path/to/file  (specific)
	//             lsp-diagnostics://               (all)
	//             lsp-diagnostics:///              (all)
	targetPath := strings.TrimPrefix(uri, "lsp-diagnostics://")
	if targetPath == "" || targetPath == "/" {
		// All-files subscription: fire notify for any open-file update.
		cb := types.DiagnosticUpdateCallback(func(updatedURI string, _ []types.LSPDiagnostic) {
			if strings.HasPrefix(updatedURI, "file://") {
				notify(updatedURI)
			}
		})
		client.SubscribeToDiagnostics(cb)
		return &SubscriptionContext{Callback: cb}, nil
	}

	// Specific-file subscription: only fire notify when the matching file updates.
	fileURI := "file://" + targetPath
	cb := types.DiagnosticUpdateCallback(func(updatedURI string, _ []types.LSPDiagnostic) {
		if updatedURI == fileURI {
			notify(updatedURI)
		}
	})
	client.SubscribeToDiagnostics(cb)
	return &SubscriptionContext{Callback: cb}, nil
}

// HandleUnsubscribeDiagnostics removes a diagnostic subscription.
func HandleUnsubscribeDiagnostics(
	ctx context.Context,
	client *lsp.LSPClient,
	uri string,
	sub *SubscriptionContext,
) error {
	if sub == nil {
		return nil
	}
	client.UnsubscribeFromDiagnostics(sub.Callback)
	return nil
}
