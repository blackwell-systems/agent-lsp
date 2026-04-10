// Package logging provides a minimal leveled logger for agent-lsp.
// It writes to stderr and is controlled by the LOG_LEVEL environment variable
// (debug, info, warning, error). The default level is info.
// Log is the primary entry point; SetLevelFromEnv configures the level at startup.
package logging
