package logging

// Level constants matching the MCP logging/message severity levels.
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

// Log emits a log message at the given level.
// Before the MCP server is initialized, messages go to stderr.
// After initialization, messages route through MCP logging/message notifications.
// Stub body — implemented in logging_impl.go (Wave 2, Agent E).
func Log(level, message string) {}

// SetLevel changes the minimum log level. Valid values are the Level* constants.
// Messages below this level are suppressed.
// Stub body — implemented in logging_impl.go (Wave 2, Agent E).
func SetLevel(level string) {}

// SetServer stores a reference to the MCP server notification sender.
// Called by server.go (Wave 2) after the server is created.
// Stub body — implemented in logging_impl.go (Wave 2, Agent E).
func SetServer(sender interface{}) {}

// MarkServerInitialized signals that the MCP server is ready to receive
// logging/message notifications. Called by server.go after server.Run starts.
// Stub body — implemented in logging_impl.go (Wave 2, Agent E).
func MarkServerInitialized() {}
