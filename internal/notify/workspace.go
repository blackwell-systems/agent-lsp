package notify

import (
	"encoding/json"
	"time"
)

// WorkspaceReadySubscriber provides workspace loading state.
type WorkspaceReadySubscriber interface {
	IsWorkspaceLoaded() bool
}

// workspaceReadyPayload is the JSON payload emitted when workspace is ready.
type workspaceReadyPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// SubscribeWorkspaceReady monitors workspace loading state and emits a notification
// when the language server finishes indexing. Returns a stop function to cancel polling.
//
// If the workspace is already loaded, the notification is emitted immediately and
// the returned stop function is a no-op.
//
// Polling stops automatically after 5 minutes if the workspace never becomes ready.
func SubscribeWorkspaceReady(hub *Hub, client WorkspaceReadySubscriber, pollInterval time.Duration) func() {
	payload := workspaceReadyPayload{
		Type:    "workspace_ready",
		Message: "Language server indexing complete",
	}

	emit := func() {
		msg, _ := json.Marshal(payload)
		hub.Send("info", "workspace", string(msg))
	}

	// If already loaded, emit immediately and return no-op.
	if client.IsWorkspaceLoaded() {
		emit()
		return func() {}
	}

	// Poll in a background goroutine.
	stop := make(chan struct{})
	go func() {
		timeout := time.NewTimer(5 * time.Minute)
		defer timeout.Stop()

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-timeout.C:
				return
			case <-ticker.C:
				if client.IsWorkspaceLoaded() {
					emit()
					return
				}
			}
		}
	}()

	return func() {
		select {
		case <-stop:
			// Already closed.
		default:
			close(stop)
		}
	}
}
