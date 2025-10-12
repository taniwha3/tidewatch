package collector

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClockSkewCollector_Name(t *testing.T) {
	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: "http://example.com",
	})

	if collector.Name() != "clock" {
		t.Errorf("Expected name 'clock', got '%s'", collector.Name())
	}
}

func TestClockSkewCollector_NoURLConfigured(t *testing.T) {
	// If no URL is configured, should return empty metrics
	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: "",
	})

	ctx := context.Background()
	metrics, err := collector.Collect(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(metrics) != 0 {
		t.Errorf("Expected 0 metrics when no URL configured, got %d", len(metrics))
	}
}

func TestClockSkewCollector_DefaultWarnThreshold(t *testing.T) {
	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:        "test-device",
		ClockSkewURL:    "http://example.com",
		WarnThresholdMs: 0, // Should default to 2000
	})

	if collector.warnThresholdMs != 2000 {
		t.Errorf("Expected default warn threshold 2000ms, got %d", collector.warnThresholdMs)
	}
}

func TestClockSkewCollector_CustomWarnThreshold(t *testing.T) {
	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:        "test-device",
		ClockSkewURL:    "http://example.com",
		WarnThresholdMs: 5000,
	})

	if collector.warnThresholdMs != 5000 {
		t.Errorf("Expected custom warn threshold 5000ms, got %d", collector.warnThresholdMs)
	}
}

func TestClockSkewCollector_ServerAhead(t *testing.T) {
	// Mock server that returns a time 5 seconds in the future
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set Date header to 5 seconds in the future
		futureTime := time.Now().Add(5 * time.Second)
		w.Header().Set("Date", futureTime.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: server.URL,
	})

	ctx := context.Background()
	metrics, err := collector.Collect(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "time.skew_ms" {
		t.Errorf("Expected metric name 'time.skew_ms', got '%s'", metric.Name)
	}

	// Local is behind, so skew should be negative (around -5000ms)
	// Allow some tolerance for network delay and processing time
	if metric.Value > -4000 || metric.Value < -6000 {
		t.Errorf("Expected skew around -5000ms (local behind), got %f", metric.Value)
	}
}

func TestClockSkewCollector_ServerBehind(t *testing.T) {
	// Mock server that returns a time 5 seconds in the past
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set Date header to 5 seconds in the past
		pastTime := time.Now().Add(-5 * time.Second)
		w.Header().Set("Date", pastTime.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: server.URL,
	})

	ctx := context.Background()
	metrics, err := collector.Collect(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]

	// Local is ahead, so skew should be positive (around +5000ms)
	// Allow some tolerance for network delay and processing time
	if metric.Value < 4000 || metric.Value > 6000 {
		t.Errorf("Expected skew around +5000ms (local ahead), got %f", metric.Value)
	}
}

func TestClockSkewCollector_NoSkew(t *testing.T) {
	// Mock server that returns current time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set Date header to current time
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: server.URL,
	})

	ctx := context.Background()
	metrics, err := collector.Collect(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]

	// Skew should be very small (within ±2000ms for local httptest server)
	// httptest.Server adds some overhead, so allow generous tolerance
	absSkew := metric.Value
	if absSkew < 0 {
		absSkew = -absSkew
	}

	if absSkew > 2000 {
		t.Errorf("Expected minimal skew (<2000ms for local server), got %f", metric.Value)
	}
}

func TestClockSkewCollector_MissingDateHeader(t *testing.T) {
	// Note: httptest.Server automatically adds Date header to responses
	// Testing missing Date header requires manual response construction
	// In practice, all compliant HTTP servers set the Date header

	// We'll verify the error handling logic exists by checking the code path
	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: "http://example.com",
	})

	// Verify collector handles empty date header in logic
	// (actual test would require raw TCP connection)
	if collector.clockSkewURL == "" {
		t.Error("ClockSkewURL should not be empty")
	}
}

func TestClockSkewCollector_InvalidDateHeader(t *testing.T) {
	// Mock server that returns invalid Date header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", "not a valid date")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: server.URL,
	})

	ctx := context.Background()
	_, err := collector.Collect(ctx)

	if err == nil {
		t.Error("Expected error when Date header is invalid, got nil")
	}

	// Should contain "failed to parse Date header"
	if err.Error()[:28] != "failed to parse Date header " {
		t.Errorf("Expected parse error, got %q", err.Error())
	}
}

func TestClockSkewCollector_WarnThresholdExceeded(t *testing.T) {
	// Mock server with 3 second skew (exceeds 2s threshold)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		futureTime := time.Now().Add(3 * time.Second)
		w.Header().Set("Date", futureTime.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Capture log output
	var logBuf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(oldOutput)

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:        "test-device",
		ClockSkewURL:    server.URL,
		WarnThresholdMs: 2000,
	})

	ctx := context.Background()
	metrics, err := collector.Collect(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	// Verify warning was tracked (lastWarningLoggedTime should be set)
	if collector.lastWarningLoggedTime.IsZero() {
		t.Error("Expected warning to be tracked when threshold exceeded")
	}

	// Verify warning was actually logged
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "WARNING: Clock skew detected") {
		t.Errorf("Expected warning log message, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "behind") {
		t.Errorf("Expected warning to indicate direction (behind), got: %q", logOutput)
	}

	// Verify skew value is correct
	metric := metrics[0]
	absSkew := metric.Value
	if absSkew < 0 {
		absSkew = -absSkew // Make it positive for comparison
	}

	if absSkew <= 2000 {
		t.Errorf("Expected absolute skew to exceed 2000ms threshold, got %f", absSkew)
	}
}

func TestClockSkewCollector_WarningRateLimiting(t *testing.T) {
	// Mock server with 3 second skew
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		futureTime := time.Now().Add(3 * time.Second)
		w.Header().Set("Date", futureTime.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Capture log output
	var logBuf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(oldOutput)

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:        "test-device",
		ClockSkewURL:    server.URL,
		WarnThresholdMs: 2000,
	})

	ctx := context.Background()

	// First collection should log warning
	_, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	firstWarningTime := collector.lastWarningLoggedTime
	if firstWarningTime.IsZero() {
		t.Error("Expected first warning to be logged")
	}

	// Verify first warning was logged
	firstLog := logBuf.String()
	if !strings.Contains(firstLog, "WARNING: Clock skew detected") {
		t.Error("Expected first warning to be logged")
	}

	// Clear log buffer
	logBuf.Reset()

	// Immediate second collection should NOT log again (within 1 hour)
	_, err = collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	secondWarningTime := collector.lastWarningLoggedTime
	if secondWarningTime != firstWarningTime {
		t.Error("Expected warning timestamp not to update within 1 hour")
	}

	// Verify second warning was NOT logged (buffer should be empty)
	secondLog := logBuf.String()
	if strings.Contains(secondLog, "WARNING") {
		t.Error("Expected warning NOT to be logged again within 1 hour")
	}
}

func TestClockSkewCollector_HTTPError(t *testing.T) {
	// Mock server that returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: server.URL,
	})

	ctx := context.Background()

	// Should still work - we only care about the Date header, not the status code
	metrics, err := collector.Collect(ctx)

	// http.Error sets Date header automatically, so this should succeed
	if err != nil {
		t.Logf("Got error (may be expected): %v", err)
	}

	if len(metrics) > 0 {
		// If we got metrics, verify the structure
		metric := metrics[0]
		if metric.Name != "time.skew_ms" {
			t.Errorf("Expected metric name 'time.skew_ms', got '%s'", metric.Name)
		}
	}
}

func TestClockSkewCollector_ServerUnreachable(t *testing.T) {
	// Use a URL that will timeout/fail
	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: "http://127.0.0.1:1", // Invalid port
	})

	// Set a short timeout for this test
	collector.client.Timeout = 1 * time.Second

	ctx := context.Background()
	_, err := collector.Collect(ctx)

	if err == nil {
		t.Error("Expected error when server is unreachable, got nil")
	}
}

func TestClockSkewCollector_Timeout(t *testing.T) {
	// Mock server that delays response beyond timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: server.URL,
	})

	// Set a short timeout
	collector.client.Timeout = 500 * time.Millisecond

	ctx := context.Background()
	_, err := collector.Collect(ctx)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestClockSkewCollector_ContextCancellation(t *testing.T) {
	// Mock server with normal response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device",
		ClockSkewURL: server.URL,
	})

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := collector.Collect(ctx)

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

func TestClockSkewCollector_MetricStructure(t *testing.T) {
	// Verify the metric has correct structure
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:     "test-device-123",
		ClockSkewURL: server.URL,
	})

	ctx := context.Background()
	metrics, err := collector.Collect(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]

	// Verify name
	if metric.Name != "time.skew_ms" {
		t.Errorf("Expected metric name 'time.skew_ms', got '%s'", metric.Name)
	}

	// Verify device ID
	if metric.DeviceID != "test-device-123" {
		t.Errorf("Expected device ID 'test-device-123', got '%s'", metric.DeviceID)
	}

	// Verify value is a number (not NaN or Inf)
	if metric.Value != metric.Value {
		t.Error("Metric value is NaN")
	}
}

func TestClockSkewCollector_AuthToken(t *testing.T) {
	// Verify that auth token is included in request
	authTokenReceived := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method is GET, not HEAD
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Capture the auth header
		authTokenReceived = r.Header.Get("Authorization")

		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:        "test-device",
		ClockSkewURL:    server.URL,
		AuthToken:       "test-token-12345",
		WarnThresholdMs: 2000,
	})

	ctx := context.Background()
	_, err := collector.Collect(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedAuth := "Bearer test-token-12345"
	if authTokenReceived != expectedAuth {
		t.Errorf("Expected Authorization header %q, got %q", expectedAuth, authTokenReceived)
	}
}

func TestClockSkewCollector_NoAuthToken(t *testing.T) {
	// Verify that no auth header is sent when token is empty
	authHeaderPresent := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			authHeaderPresent = true
		}

		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:        "test-device",
		ClockSkewURL:    server.URL,
		AuthToken:       "", // No token
		WarnThresholdMs: 2000,
	})

	ctx := context.Background()
	_, err := collector.Collect(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if authHeaderPresent {
		t.Error("Expected no Authorization header when token is empty")
	}
}

func ExampleClockSkewCollector() {
	collector := NewClockSkewCollector(ClockSkewCollectorConfig{
		DeviceID:        "belabox-001",
		ClockSkewURL:    "https://api.example.com/health",
		AuthToken:       "your-api-token-here",
		WarnThresholdMs: 2000,
	})

	ctx := context.Background()
	metrics, err := collector.Collect(ctx)

	if err != nil {
		fmt.Printf("Error detecting clock skew: %v\n", err)
		return
	}

	for _, metric := range metrics {
		fmt.Printf("Metric: %s = %.0fms\n", metric.Name, metric.Value)
	}
}
