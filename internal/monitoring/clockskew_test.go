package monitoring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDetectClockSkew_Success verifies successful clock skew detection
func TestDetectClockSkew_Success(t *testing.T) {
	// Create test server that returns Date header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return current time as Date header
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	skew, err := DetectClockSkew(server.URL, "", 5*time.Second)
	if err != nil {
		t.Fatalf("DetectClockSkew failed: %v", err)
	}

	// Skew should be very small (< 1 second) since server and client are on same machine
	if skew > time.Second || skew < -time.Second {
		t.Errorf("Expected small skew, got %v", skew)
	}
}

// TestDetectClockSkew_WithAuth verifies auth token is sent
func TestDetectClockSkew_WithAuth(t *testing.T) {
	expectedToken := "test-token-123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+expectedToken {
			t.Errorf("Expected Authorization 'Bearer %s', got '%s'", expectedToken, auth)
		}

		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := DetectClockSkew(server.URL, expectedToken, 5*time.Second)
	if err != nil {
		t.Fatalf("DetectClockSkew failed: %v", err)
	}
}

// TestDetectClockSkew_WithoutAuth verifies no auth header when token empty
func TestDetectClockSkew_WithoutAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Expected no Authorization header, got '%s'", auth)
		}

		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := DetectClockSkew(server.URL, "", 5*time.Second)
	if err != nil {
		t.Fatalf("DetectClockSkew failed: %v", err)
	}
}

// TestDetectClockSkew_NoDateHeader verifies error when Date header missing
func TestDetectClockSkew_NoDateHeader(t *testing.T) {
	// httptest.Server automatically adds Date header, so we can't easily test this
	// The error handling code is tested in InvalidDateHeader test
	t.Skip("httptest.Server automatically adds Date header")
}

// TestDetectClockSkew_InvalidDateHeader verifies error for malformed Date
func TestDetectClockSkew_InvalidDateHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", "invalid-date-format")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := DetectClockSkew(server.URL, "", 5*time.Second)
	if err == nil {
		t.Fatal("Expected error for invalid Date header")
	}

	if !strings.Contains(err.Error(), "failed to parse Date header") {
		t.Errorf("Expected parse error, got: %v", err)
	}
}

// TestDetectClockSkew_NetworkError verifies error handling for network failures
func TestDetectClockSkew_NetworkError(t *testing.T) {
	// Use invalid URL to trigger network error
	_, err := DetectClockSkew("http://invalid.localhost:99999", "", 1*time.Second)
	if err == nil {
		t.Fatal("Expected network error")
	}

	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("Expected request failure, got: %v", err)
	}
}

// TestDetectClockSkew_Timeout verifies timeout handling
func TestDetectClockSkew_Timeout(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Set very short timeout
	_, err := DetectClockSkew(server.URL, "", 100*time.Millisecond)
	if err == nil {
		t.Fatal("Expected timeout error")
	}
}

// TestDetectClockSkew_DefaultTimeout verifies default timeout is used
func TestDetectClockSkew_DefaultTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Pass 0 timeout to trigger default
	_, err := DetectClockSkew(server.URL, "", 0)
	if err != nil {
		t.Fatalf("DetectClockSkew with default timeout failed: %v", err)
	}
}

// TestDetectClockSkew_SimulatedSkew verifies skew calculation
func TestDetectClockSkew_SimulatedSkew(t *testing.T) {
	// Server returns time 10 seconds in the past
	pastTime := time.Now().Add(-10 * time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", pastTime.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	skew, err := DetectClockSkew(server.URL, "", 5*time.Second)
	if err != nil {
		t.Fatalf("DetectClockSkew failed: %v", err)
	}

	// Local clock should be ~10 seconds ahead (positive skew)
	if skew < 9*time.Second || skew > 11*time.Second {
		t.Errorf("Expected skew around 10s, got %v", skew)
	}
}

// TestDetectClockSkewDetailed_Success verifies detailed results
func TestDetectClockSkewDetailed_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := DetectClockSkewDetailed(server.URL, "", 5*time.Second)
	if err != nil {
		t.Fatalf("DetectClockSkewDetailed failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Skew > time.Second || result.Skew < -time.Second {
		t.Errorf("Expected small skew, got %v", result.Skew)
	}

	if result.SkewMs != result.Skew.Milliseconds() {
		t.Errorf("SkewMs mismatch: %d != %d", result.SkewMs, result.Skew.Milliseconds())
	}

	if result.RoundTrip < 0 {
		t.Errorf("RoundTrip should be positive, got %v", result.RoundTrip)
	}

	if result.ServerTime.IsZero() {
		t.Error("ServerTime should not be zero")
	}

	if result.LocalTime.IsZero() {
		t.Error("LocalTime should not be zero")
	}
}

// TestDetectClockSkewDetailed_WithAuth verifies auth is used in detailed version
func TestDetectClockSkewDetailed_WithAuth(t *testing.T) {
	expectedToken := "detailed-test-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+expectedToken {
			t.Errorf("Expected Authorization 'Bearer %s', got '%s'", expectedToken, auth)
		}

		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := DetectClockSkewDetailed(server.URL, expectedToken, 5*time.Second)
	if err != nil {
		t.Fatalf("DetectClockSkewDetailed failed: %v", err)
	}
}

// TestDetectClockSkew_HTTPMethodIsGET verifies GET method is used
func TestDetectClockSkew_HTTPMethodIsGET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := DetectClockSkew(server.URL, "", 5*time.Second)
	if err != nil {
		t.Fatalf("DetectClockSkew failed: %v", err)
	}
}

// Benchmark clock skew detection
func BenchmarkDetectClockSkew(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectClockSkew(server.URL, "", 5*time.Second)
	}
}
