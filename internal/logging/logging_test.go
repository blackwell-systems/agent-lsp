package logging

import (
	"sync"
	"testing"
)

// resetState resets the logging package state between tests.
func resetState() {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = LevelInfo
	mcpServer = nil
	serverInitialized = false
	initWarning = ""
}

func TestSetLevel_Valid(t *testing.T) {
	resetState()

	tests := []struct {
		level string
		want  string
	}{
		{LevelDebug, LevelDebug},
		{LevelWarning, LevelWarning},
		{LevelError, LevelError},
		{LevelCritical, LevelCritical},
		{LevelEmergency, LevelEmergency},
	}

	for _, tt := range tests {
		SetLevel(tt.level)
		mu.RLock()
		got := currentLevel
		mu.RUnlock()
		if got != tt.want {
			t.Errorf("SetLevel(%q): currentLevel = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestSetLevel_Invalid(t *testing.T) {
	resetState()
	SetLevel(LevelWarning) // set to a known valid value first

	SetLevel("garbage")

	mu.RLock()
	got := currentLevel
	mu.RUnlock()
	if got != LevelWarning {
		t.Errorf("SetLevel(invalid) changed level to %q, want %q", got, LevelWarning)
	}
}

func TestSetServer(t *testing.T) {
	resetState()

	type fakeServer struct{}
	s := &fakeServer{}
	SetServer(s)

	mu.RLock()
	got := mcpServer
	mu.RUnlock()
	if got != s {
		t.Error("SetServer did not store the server reference")
	}
}

func TestMarkServerInitialized(t *testing.T) {
	resetState()

	mu.RLock()
	before := serverInitialized
	mu.RUnlock()
	if before {
		t.Fatal("serverInitialized should be false before MarkServerInitialized")
	}

	MarkServerInitialized()

	mu.RLock()
	after := serverInitialized
	mu.RUnlock()
	if !after {
		t.Error("serverInitialized should be true after MarkServerInitialized")
	}
}

// mockSender implements the logSender interface for testing.
type mockSender struct {
	mu       sync.Mutex
	messages []struct{ level, logger, message string }
}

func (m *mockSender) LogMessage(level, logger, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, struct{ level, logger, message string }{level, logger, message})
	return nil
}

func TestLog_RoutesThroughMCPWhenInitialized(t *testing.T) {
	resetState()

	sender := &mockSender{}
	SetServer(sender)
	MarkServerInitialized()
	SetLevel(LevelDebug)

	Log(LevelInfo, "hello from test")

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message routed to sender, got %d", len(sender.messages))
	}
	if sender.messages[0].message != "hello from test" {
		t.Errorf("message = %q, want %q", sender.messages[0].message, "hello from test")
	}
	if sender.messages[0].level != LevelInfo {
		t.Errorf("level = %q, want %q", sender.messages[0].level, LevelInfo)
	}
}

func TestLog_FiltersBelowMinLevel(t *testing.T) {
	resetState()

	sender := &mockSender{}
	SetServer(sender)
	MarkServerInitialized()
	SetLevel(LevelWarning)

	Log(LevelDebug, "should be filtered")
	Log(LevelInfo, "also filtered")

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.messages) != 0 {
		t.Errorf("expected 0 messages (filtered), got %d", len(sender.messages))
	}
}

func TestLog_AllowsAtOrAboveMinLevel(t *testing.T) {
	resetState()

	sender := &mockSender{}
	SetServer(sender)
	MarkServerInitialized()
	SetLevel(LevelWarning)

	Log(LevelWarning, "warning msg")
	Log(LevelError, "error msg")

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sender.messages))
	}
}

func TestLog_UnknownLevelTreatedAsInfo(t *testing.T) {
	resetState()

	sender := &mockSender{}
	SetServer(sender)
	MarkServerInitialized()
	SetLevel(LevelDebug)

	Log("unknown_level", "test msg")

	sender.mu.Lock()
	defer sender.mu.Unlock()
	// unknown level gets priority of info (1), which is >= debug (0), so it should pass
	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message for unknown level, got %d", len(sender.messages))
	}
}

func TestLog_FallsBackToStderrWhenNotInitialized(t *testing.T) {
	resetState()
	SetLevel(LevelDebug)

	// Without server initialized, Log should not panic and should write to stderr.
	// We just verify no panic occurs.
	Log(LevelInfo, "fallback test")
}

func TestLogLevelPriority_Ordering(t *testing.T) {
	ordered := []string{
		LevelDebug,
		LevelInfo,
		LevelNotice,
		LevelWarning,
		LevelError,
		LevelCritical,
		LevelAlert,
		LevelEmergency,
	}

	for i := 0; i < len(ordered)-1; i++ {
		cur := logLevelPriority[ordered[i]]
		next := logLevelPriority[ordered[i+1]]
		if cur >= next {
			t.Errorf("priority(%s)=%d should be < priority(%s)=%d", ordered[i], cur, ordered[i+1], next)
		}
	}
}

func TestSetLevelFromEnv_NoEnvVar(t *testing.T) {
	resetState()
	// With no LOG_LEVEL env var, should remain at default (info).
	t.Setenv("LOG_LEVEL", "")
	SetLevelFromEnv()

	mu.RLock()
	got := currentLevel
	mu.RUnlock()
	if got != LevelInfo {
		t.Errorf("currentLevel = %q, want %q", got, LevelInfo)
	}
}

func TestSetLevelFromEnv_ValidEnvVar(t *testing.T) {
	resetState()
	t.Setenv("LOG_LEVEL", "debug")
	SetLevelFromEnv()

	mu.RLock()
	got := currentLevel
	mu.RUnlock()
	if got != LevelDebug {
		t.Errorf("currentLevel = %q, want %q", got, LevelDebug)
	}
}

func TestSetLevelFromEnv_InvalidEnvVar(t *testing.T) {
	resetState()
	t.Setenv("LOG_LEVEL", "bogus")
	SetLevelFromEnv()

	mu.RLock()
	got := currentLevel
	mu.RUnlock()
	// Invalid value should not change from default
	if got != LevelInfo {
		t.Errorf("currentLevel = %q, want %q (should stay at default)", got, LevelInfo)
	}
}
