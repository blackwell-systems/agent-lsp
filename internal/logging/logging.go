package logging

import (
	"fmt"
	"os"
	"sync"
)

// Level constants matching the MCP logging/message severity levels.
// All eight levels are valid inputs for set_log_level; only Debug/Info/Warning/Error/Critical
// are emitted internally. Notice/Alert/Emergency are accepted but never self-generated.
const (
	LevelDebug     = "debug"
	LevelInfo      = "info"
	LevelNotice    = "notice"
	LevelWarning   = "warning"
	LevelError     = "error"
	LevelCritical  = "critical"
	LevelAlert     = "alert"
	LevelEmergency = "emergency"
)

// logLevelPriority maps level names to numeric priorities for comparison.
var logLevelPriority = map[string]int{
	LevelDebug:     0,
	LevelInfo:      1,
	LevelNotice:    2,
	LevelWarning:   3,
	LevelError:     4,
	LevelCritical:  5,
	LevelAlert:     6,
	LevelEmergency: 7,
}

// logState holds the mutable logging state protected by mu.
var (
	mu                sync.RWMutex
	currentLevel      = LevelInfo
	mcpServer         interface{}
	serverInitialized bool
)

// initWarning holds a warning message generated during init() that could not
// be emitted to stderr at that time (e.g., invalid LOG_LEVEL). It is flushed
// to stderr on the first Log() call, before the message filter is applied.
var initWarning string

// serverSender is an interface satisfied by *mcp.ServerSession for Log calls.
// We use interface{} to avoid a hard dependency on the mcp package here.
type logSender interface {
	LogMessage(level, logger, message string) error
}

func init() {
	// Level initialization is performed by SetLevelFromEnv(), called
	// explicitly at binary startup. This init() is intentionally a no-op.
}

// SetServer stores a reference to the MCP server notification sender.
// Called by server.go (Wave 2) after the server is created.
func SetServer(sender interface{}) {
	mu.Lock()
	defer mu.Unlock()
	mcpServer = sender
}

// MarkServerInitialized signals that the MCP server is ready to receive
// logging/message notifications. Called by server.go after server.Run starts.
func MarkServerInitialized() {
	mu.Lock()
	defer mu.Unlock()
	serverInitialized = true
}

// SetLevel changes the minimum log level. Valid values are the Level* constants.
// Messages below this level are suppressed.
func SetLevel(level string) {
	if _, ok := logLevelPriority[level]; !ok {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
}

// SetLevelFromEnv reads LOG_LEVEL from the environment and applies it
// via SetLevel. Call this explicitly at binary startup instead of relying
// on init(). Emits a warning to stderr if LOG_LEVEL is set to an
// unrecognized value.
func SetLevelFromEnv() {
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		if _, ok := logLevelPriority[envLevel]; ok {
			SetLevel(envLevel)
		} else {
			fmt.Fprintf(os.Stderr, "agent-lsp: invalid LOG_LEVEL %q, defaulting to \"info\"\n", envLevel)
		}
	}
}

// Log emits a log message at the given level.
// Before the MCP server is initialized, messages go to stderr.
// After initialization, messages route through MCP logging/message notifications.
func Log(level, message string) {
	mu.Lock()
	w := initWarning
	initWarning = ""
	mu.Unlock()

	mu.RLock()
	minLevel := currentLevel
	initialized := serverInitialized
	sender := mcpServer
	mu.RUnlock()

	if w != "" {
		fmt.Fprint(os.Stderr, w)
	}

	// Filter by level.
	msgPriority, ok := logLevelPriority[level]
	if !ok {
		// Unknown level: treat as info.
		msgPriority = logLevelPriority[LevelInfo]
	}
	minPriority, ok := logLevelPriority[minLevel]
	if !ok {
		minPriority = logLevelPriority[LevelInfo]
	}
	if msgPriority < minPriority {
		return
	}

	formatted := fmt.Sprintf("[%s] %s", level, message)

	if initialized && sender != nil {
		if ls, ok := sender.(logSender); ok {
			_ = ls.LogMessage(level, "agent-lsp", message)
			return
		}
	}

	// Fallback: write to stderr.
	fmt.Fprintln(os.Stderr, formatted)
}
