package notify

import (
	"encoding/json"
	"time"
)

// HealthChecker provides language server process liveness state.
type HealthChecker interface {
	IsAlive() bool
}

// healthPayload is the JSON payload emitted for process health changes.
type healthPayload struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// SubscribeHealth monitors language server process health and emits notifications
// on crash or recovery. Returns a stop function to cancel polling.
//
// The checker is assumed to start in the alive state. Notifications are emitted
// only on state transitions (alive->dead or dead->alive).
func SubscribeHealth(hub *Hub, checker HealthChecker, pollInterval time.Duration) func() {
	stop := make(chan struct{})

	go func() {
		alive := true // assume initially alive

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				current := checker.IsAlive()
				if alive && !current {
					// Crashed.
					p := healthPayload{
						Type:    "process_health",
						Status:  "crashed",
						Message: "Language server process exited unexpectedly",
					}
					msg, _ := json.Marshal(p)
					hub.Send("error", "health", string(msg))
				} else if !alive && current {
					// Recovered.
					p := healthPayload{
						Type:    "process_health",
						Status:  "recovered",
						Message: "Language server process recovered",
					}
					msg, _ := json.Marshal(p)
					hub.Send("info", "health", string(msg))
				}
				alive = current
			}
		}
	}()

	return func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
}
