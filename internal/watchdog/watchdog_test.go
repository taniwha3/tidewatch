package watchdog

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestNewPinger_NoSystemd(t *testing.T) {
	// Clear systemd environment variables
	os.Unsetenv("WATCHDOG_USEC")
	os.Unsetenv("WATCHDOG_PID")
	os.Unsetenv("NOTIFY_SOCKET")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pinger := NewPinger(logger)

	if pinger.IsEnabled() {
		t.Error("Expected watchdog to be disabled without systemd")
	}
}

func TestPinger_Start_NoSystemd(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pinger := &Pinger{
		enabled: false,
		logger:  logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should return immediately without error
	pinger.Start(ctx)

	// Test passes if we get here without hanging
}

func TestPinger_GetInterval(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	testInterval := 30 * time.Second
	pinger := &Pinger{
		enabled:  true,
		interval: testInterval,
		logger:   logger,
	}

	if pinger.GetInterval() != testInterval {
		t.Errorf("Expected interval %v, got %v", testInterval, pinger.GetInterval())
	}
}

func TestIsRunningUnderSystemd_False(t *testing.T) {
	// Clear systemd environment variables
	os.Unsetenv("NOTIFY_SOCKET")
	os.Unsetenv("INVOCATION_ID")

	if IsRunningUnderSystemd() {
		t.Error("Expected IsRunningUnderSystemd to return false without systemd env vars")
	}
}

func TestIsRunningUnderSystemd_NotifySocket(t *testing.T) {
	// Set NOTIFY_SOCKET
	os.Setenv("NOTIFY_SOCKET", "/run/systemd/notify")
	defer os.Unsetenv("NOTIFY_SOCKET")

	if !IsRunningUnderSystemd() {
		t.Error("Expected IsRunningUnderSystemd to return true with NOTIFY_SOCKET")
	}
}

func TestIsRunningUnderSystemd_InvocationID(t *testing.T) {
	// Set INVOCATION_ID
	os.Setenv("INVOCATION_ID", "abc123")
	defer os.Unsetenv("INVOCATION_ID")

	if !IsRunningUnderSystemd() {
		t.Error("Expected IsRunningUnderSystemd to return true with INVOCATION_ID")
	}
}

func TestPinger_NotifyReady_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pinger := &Pinger{
		enabled: false,
		logger:  logger,
	}

	// Should not panic or error
	pinger.NotifyReady()
}

func TestPinger_NotifyStopping_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pinger := &Pinger{
		enabled: false,
		logger:  logger,
	}

	// Should not panic or error
	pinger.NotifyStopping()
}
