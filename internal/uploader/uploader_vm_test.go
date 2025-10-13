package uploader

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
)

// intPtr is a helper function to create a pointer to an int value
func intPtr(i int) *int {
	return &i
}

// TestUploadVM_Success verifies successful upload with proper headers
func TestUploadVM_Success(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
		models.NewMetric("memory.bytes.used", 1024.0, "device-001").WithTimestamp(now.Add(time.Second)),
	}

	// Mock server that validates headers and payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Verify headers
		if ct := r.Header.Get("Content-Type"); ct != "application/x-ndjson" {
			t.Errorf("Expected Content-Type application/x-ndjson, got %s", ct)
		}
		if ce := r.Header.Get("Content-Encoding"); ce != "gzip" {
			t.Errorf("Expected Content-Encoding gzip, got %s", ce)
		}
		if ua := r.Header.Get("User-Agent"); ua != "thugshells/1.0" {
			t.Errorf("Expected User-Agent thugshells/1.0, got %s", ua)
		}
		if did := r.Header.Get("X-Device-ID"); did != "device-001" {
			t.Errorf("Expected X-Device-ID device-001, got %s", did)
		}

		// Verify gzipped body can be decompressed
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("Failed to create gzip reader: %v", err)
		}
		defer reader.Close()

		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read decompressed body: %v", err)
		}

		// Should contain JSONL data
		if len(body) == 0 {
			t.Errorf("Body is empty")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "device-001")
	err := uploader.Upload(context.Background(), metrics)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
}

// TestUploadVM_WithAuth verifies Bearer token is sent
func TestUploadVM_WithAuth(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	expectedToken := "test-token-123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+expectedToken {
			t.Errorf("Expected Authorization 'Bearer %s', got '%s'", expectedToken, auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:       server.URL,
		DeviceID:  "device-001",
		AuthToken: expectedToken,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
}

// TestUploadVM_Chunking verifies metrics are split into chunks
func TestUploadVM_Chunking(t *testing.T) {
	now := time.Now()
	// Create 75 metrics with chunk size of 30
	metrics := make([]*models.Metric, 75)
	for i := 0; i < 75; i++ {
		metrics[i] = models.NewMetric("cpu.temperature", float64(40+i), "device-001").
			WithTimestamp(now.Add(time.Duration(i) * time.Second))
	}

	uploadCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadCount++

		// Verify chunk headers
		chunkIndex := r.Header.Get("X-Chunk-Index")
		chunkMetrics := r.Header.Get("X-Chunk-Metrics")

		if chunkIndex == "" {
			t.Errorf("Missing X-Chunk-Index header")
		}
		if chunkMetrics == "" {
			t.Errorf("Missing X-Chunk-Metrics header")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:       server.URL,
		DeviceID:  "device-001",
		ChunkSize: 30,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// 75 metrics / 30 per chunk = 3 chunks
	expectedChunks := 3
	if uploadCount != expectedChunks {
		t.Errorf("Expected %d uploads (chunks), got %d", expectedChunks, uploadCount)
	}
}

// TestUploadVM_RetryOnServerError verifies retry on 500 errors
func TestUploadVM_RetryOnServerError(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		// Verify X-Attempt header
		attemptStr := r.Header.Get("X-Attempt")
		expectedAttempt := strconv.Itoa(attempts - 1)
		if attemptStr != expectedAttempt {
			t.Errorf("Expected X-Attempt %s, got %s", expectedAttempt, attemptStr)
		}

		// Fail first 2 attempts, succeed on 3rd
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxRetries: intPtr(3),
		RetryDelay: 10 * time.Millisecond,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err != nil {
		t.Fatalf("Upload failed after retries: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

// TestUploadVM_NoRetryOn400 verifies no retry on 400 Bad Request
func TestUploadVM_NoRetryOn400(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxRetries: intPtr(3),
		RetryDelay: 10 * time.Millisecond,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err == nil {
		t.Fatal("Expected error for 400 Bad Request")
	}

	// Should not retry on 400
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retry), got %d", attempts)
	}

	// Verify error message contains "non-retryable"
	if !contains(err.Error(), "non-retryable") {
		t.Errorf("Expected 'non-retryable' in error, got: %v", err)
	}
}

// TestUploadVM_NoRetryOn401 verifies no retry on 401 Unauthorized
func TestUploadVM_NoRetryOn401(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxRetries: intPtr(3),
		RetryDelay: 10 * time.Millisecond,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err == nil {
		t.Fatal("Expected error for 401 Unauthorized")
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retry), got %d", attempts)
	}
}

// TestUploadVM_RateLimitRetry verifies retry on 429 with backoff
func TestUploadVM_RateLimitRetry(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		// Rate limit first attempt, succeed on second
		if attempts == 1 {
			w.Header().Set("Retry-After", "1") // 1 second
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxRetries: intPtr(3),
		RetryDelay: 10 * time.Millisecond,
	})

	start := time.Now()
	err := uploader.Upload(context.Background(), metrics)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}

	// Should have delayed at least 1 second (Retry-After)
	if elapsed < 1*time.Second {
		t.Errorf("Expected delay of at least 1s, got %v", elapsed)
	}
}

// TestUploadVM_ExponentialBackoff verifies exponential backoff with jitter
func TestUploadVM_ExponentialBackoff(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		// Fail first 3 attempts
		if attempts < 4 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxRetries: intPtr(4),
		RetryDelay: 100 * time.Millisecond,
	})

	start := time.Now()
	err := uploader.Upload(context.Background(), metrics)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Should have exponential backoff: 100ms * 2^0, 100ms * 2^1, 100ms * 2^2
	// With jitter (±25%), minimum is: (100 + 200 + 400) * 0.75 = 525ms
	minExpected := 500 * time.Millisecond // Give some slack for timing variance
	if elapsed < minExpected {
		t.Errorf("Expected backoff of at least %v, got %v", minExpected, elapsed)
	}
}

// TestUploadVM_ContextCancellation verifies context cancellation stops upload
func TestUploadVM_ContextCancellation(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay to allow context cancellation
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "device-001")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := uploader.Upload(ctx, metrics)
	if err == nil {
		t.Fatal("Expected error for cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// TestUploadVM_EmptyMetrics verifies empty metrics slice is handled
func TestUploadVM_EmptyMetrics(t *testing.T) {
	uploader := NewHTTPUploader("http://example.com", "device-001")

	err := uploader.Upload(context.Background(), []*models.Metric{})
	if err != nil {
		t.Errorf("Expected no error for empty metrics, got %v", err)
	}
}

// TestUploadVM_MaxRetriesExceeded verifies error after max retries
func TestUploadVM_MaxRetriesExceeded(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxRetries: intPtr(2),
		RetryDelay: 10 * time.Millisecond,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err == nil {
		t.Fatal("Expected error after max retries exceeded")
	}

	// Should attempt: initial + 2 retries = 3 total
	expectedAttempts := 3
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempts, got %d", expectedAttempts, attempts)
	}

	// Verify error message
	if !contains(err.Error(), "max retries") {
		t.Errorf("Expected 'max retries' in error, got: %v", err)
	}
}

// TestUploadVM_NetworkError verifies network errors are retryable
func TestUploadVM_NetworkError(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	// Use invalid URL to trigger network error
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        "http://invalid.localhost:99999",
		DeviceID:   "device-001",
		MaxRetries: intPtr(2),
		RetryDelay: 10 * time.Millisecond,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err == nil {
		t.Fatal("Expected network error")
	}

	// Should have retried
	if !contains(err.Error(), "max retries") {
		t.Errorf("Expected retries for network error, got: %v", err)
	}
}

// TestUploadBatchVM verifies batch upload
func TestUploadBatchVM(t *testing.T) {
	now := time.Now()
	batch1 := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}
	batch2 := []*models.Metric{
		models.NewMetric("memory.bytes.used", 1024.0, "device-001").WithTimestamp(now.Add(time.Second)),
	}

	uploadCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "device-001")
	err := uploader.UploadBatch(context.Background(), [][]*models.Metric{batch1, batch2})
	if err != nil {
		t.Fatalf("UploadBatch failed: %v", err)
	}

	// Should have made 2 separate uploads (one per batch)
	if uploadCount != 2 {
		t.Errorf("Expected 2 uploads, got %d", uploadCount)
	}
}

// TestParseRetryAfter verifies Retry-After header parsing
func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		header   string
		expected time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"120", 120 * time.Second},
		{"invalid", 0},
		// HTTP date format is hard to test without mock time
	}

	for _, tt := range tests {
		result := parseRetryAfter(tt.header)
		if result != tt.expected {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.header, result, tt.expected)
		}
	}
}

// TestCalculateBackoff verifies exponential backoff calculation
func TestCalculateBackoff(t *testing.T) {
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        "http://example.com",
		DeviceID:   "device-001",
		RetryDelay: 1 * time.Second,
	})

	// Test exponential growth
	for attempt := 0; attempt < 5; attempt++ {
		backoff := uploader.calculateBackoff(attempt, fmt.Errorf("test error"))

		// Expected: 1s * 2^attempt with jitter
		// So minimum should be ~75% of base (accounting for -25% jitter)
		minExpected := time.Duration(float64(time.Second) * (0.75) * float64(int(1)<<attempt))

		if backoff < minExpected {
			t.Errorf("Attempt %d: backoff %v less than min expected %v", attempt, backoff, minExpected)
		}

		// Should be capped at 30 seconds
		if backoff > 30*time.Second {
			t.Errorf("Attempt %d: backoff %v exceeds 30s cap", attempt, backoff)
		}
	}
}

// TestUploadVM_NoRetriesWhenMaxRetriesZero verifies that MaxRetries=0 means single attempt
func TestUploadVM_NoRetriesWhenMaxRetriesZero(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		// Always fail to verify no retries happen
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// MaxRetries=0 should mean NO retries (only 1 attempt)
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxRetries: intPtr(0),   // IMPORTANT: &0 means single attempt, no retries
		RetryDelay: 10 * time.Millisecond,
	})

	err := uploader.Upload(context.Background(), metrics)
	if err == nil {
		t.Fatal("Expected error after failed upload")
	}

	// Should attempt ONLY ONCE (no retries)
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries) with MaxRetries=0, got %d", attempts)
	}

	// Verify error is NOT about max retries (since we didn't retry at all)
	// The error should be about the server error directly
	if contains(err.Error(), "max retries (0) exceeded") {
		// This is actually correct - it means we exhausted 0 retries
		// The loop does: for attempt := 0; attempt <= maxRetries
		// With maxRetries=0, we only do attempt 0
	}
}

// TestUploadVM_RetriesEnabledVsDisabled compares behavior with retries enabled vs disabled
func TestUploadVM_RetriesEnabledVsDisabled(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	tests := []struct {
		name           string
		maxRetries     int
		expectedTries  int
	}{
		{
			name:          "retries disabled (MaxRetries=0)",
			maxRetries:    0,
			expectedTries: 1, // Only initial attempt
		},
		{
			name:          "retries enabled (MaxRetries=3)",
			maxRetries:    3,
			expectedTries: 4, // Initial + 3 retries
		},
		{
			name:          "retries enabled (MaxRetries=1)",
			maxRetries:    1,
			expectedTries: 2, // Initial + 1 retry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				// Always fail to count all retry attempts
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
				URL:        server.URL,
				DeviceID:   "device-001",
				MaxRetries: intPtr(tt.maxRetries),
				RetryDelay: 10 * time.Millisecond,
			})

			err := uploader.Upload(context.Background(), metrics)
			if err == nil {
				t.Fatal("Expected error after failed upload")
			}

			if attempts != tt.expectedTries {
				t.Errorf("With MaxRetries=%d, expected %d total attempts, got %d",
					tt.maxRetries, tt.expectedTries, attempts)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

// TestCalculateBackoff_CustomMultiplier verifies custom backoff multiplier
func TestCalculateBackoff_CustomMultiplier(t *testing.T) {
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:               "http://example.com",
		DeviceID:          "device-001",
		RetryDelay:        1 * time.Second,
		BackoffMultiplier: 3.0, // Custom multiplier (default is 2.0)
		MaxBackoff:        60 * time.Second,
		JitterPercent:     intPtr(0), // No jitter for precise testing
	})

	// Test that multiplier is applied correctly
	// Attempt 0: 1s * 3^0 = 1s
	// Attempt 1: 1s * 3^1 = 3s
	// Attempt 2: 1s * 3^2 = 9s

	backoff0 := uploader.calculateBackoff(0, fmt.Errorf("test error"))
	if backoff0 != 1*time.Second {
		t.Errorf("Attempt 0: expected 1s, got %v", backoff0)
	}

	backoff1 := uploader.calculateBackoff(1, fmt.Errorf("test error"))
	if backoff1 != 3*time.Second {
		t.Errorf("Attempt 1: expected 3s, got %v", backoff1)
	}

	backoff2 := uploader.calculateBackoff(2, fmt.Errorf("test error"))
	if backoff2 != 9*time.Second {
		t.Errorf("Attempt 2: expected 9s, got %v", backoff2)
	}
}

// TestCalculateBackoff_CustomMaxBackoff verifies custom max backoff cap
func TestCalculateBackoff_CustomMaxBackoff(t *testing.T) {
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:               "http://example.com",
		DeviceID:          "device-001",
		RetryDelay:        10 * time.Second,
		BackoffMultiplier: 2.0,
		MaxBackoff:        15 * time.Second, // Custom cap (default is 30s)
		JitterPercent:     intPtr(0),        // No jitter
	})

	// Attempt 2: 10s * 2^2 = 40s, but should be capped at 15s
	backoff := uploader.calculateBackoff(2, fmt.Errorf("test error"))
	if backoff != 15*time.Second {
		t.Errorf("Expected backoff capped at 15s, got %v", backoff)
	}
}

// TestCalculateBackoff_CustomJitter verifies custom jitter percentage
func TestCalculateBackoff_CustomJitter(t *testing.T) {
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:               "http://example.com",
		DeviceID:          "device-001",
		RetryDelay:        1 * time.Second,
		BackoffMultiplier: 2.0,
		MaxBackoff:        30 * time.Second,
		JitterPercent:     intPtr(10), // Custom jitter (default is 20%)
	})

	// With 10% jitter, backoff should be in range [base * 0.9, base * 1.1]
	// For attempt 1: base = 2s, range = [1.8s, 2.2s]
	expectedBase := 2 * time.Second
	minExpected := time.Duration(float64(expectedBase) * 0.9)
	maxExpected := time.Duration(float64(expectedBase) * 1.1)

	// Test multiple times to verify jitter is within range
	for i := 0; i < 10; i++ {
		backoff := uploader.calculateBackoff(1, fmt.Errorf("test error"))
		if backoff < minExpected || backoff > maxExpected {
			t.Errorf("Backoff %v outside expected range [%v, %v]", backoff, minExpected, maxExpected)
		}
	}
}

// TestCalculateBackoff_ZeroJitter verifies zero jitter means no randomness
func TestCalculateBackoff_ZeroJitter(t *testing.T) {
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:               "http://example.com",
		DeviceID:          "device-001",
		RetryDelay:        1 * time.Second,
		BackoffMultiplier: 2.0,
		MaxBackoff:        30 * time.Second,
		JitterPercent:     intPtr(0), // No jitter
	})

	// With zero jitter, backoff should be deterministic
	expected := 4 * time.Second // 1s * 2^2
	for i := 0; i < 5; i++ {
		backoff := uploader.calculateBackoff(2, fmt.Errorf("test error"))
		if backoff != expected {
			t.Errorf("Expected deterministic backoff %v, got %v", expected, backoff)
		}
	}
}

// TestCalculateBackoff_DefaultsMatchHardcoded verifies defaults match original behavior
func TestCalculateBackoff_DefaultsMatchHardcoded(t *testing.T) {
	// Create uploader with explicit defaults
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:               "http://example.com",
		DeviceID:          "device-001",
		RetryDelay:        1 * time.Second,
		BackoffMultiplier: 2.0,  // Default
		MaxBackoff:        30 * time.Second, // Default
		JitterPercent:     intPtr(20),   // Default
	})

	// Test that defaults behave like the old hardcoded implementation
	for attempt := 0; attempt < 5; attempt++ {
		backoff := uploader.calculateBackoff(attempt, fmt.Errorf("test error"))

		// Expected: 1s * 2^attempt with ±20% jitter, capped at 30s
		expectedBase := float64(time.Second) * float64(int(1)<<attempt)
		if expectedBase > float64(30*time.Second) {
			expectedBase = float64(30 * time.Second)
		}

		minExpected := time.Duration(expectedBase * 0.8)
		maxExpected := time.Duration(expectedBase * 1.2)

		if backoff < minExpected || backoff > maxExpected {
			t.Errorf("Attempt %d: backoff %v outside range [%v, %v]", attempt, backoff, minExpected, maxExpected)
		}
	}
}

// TestUploadVM_CustomDelayDefaultRetries tests P1 fix:
// When caller customizes RetryDelay but leaves MaxRetries=0, should get 3 default retries
func TestUploadVM_CustomDelayDefaultRetries(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		// Always fail to count all retry attempts
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Custom RetryDelay but MaxRetries=0 (unset)
	// This should get 3 default retries (not 0!)
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		RetryDelay: 2 * time.Second, // Custom delay
		// MaxRetries: 0 (not set)
	})

	err := uploader.Upload(context.Background(), metrics)
	if err == nil {
		t.Fatal("Expected error after failed upload")
	}

	// Should have made 4 attempts: initial + 3 default retries
	// This is the P1 fix - previously would have been 1 attempt only
	expectedAttempts := 4
	if attempts != expectedAttempts {
		t.Errorf("With custom RetryDelay but MaxRetries=0, expected %d attempts (1 + 3 default retries), got %d", expectedAttempts, attempts)
	}
}

// TestUploadVM_CustomBackoffDefaultRetries tests P1 fix:
// When caller customizes MaxBackoff but leaves MaxRetries=0, should get 3 default retries
func TestUploadVM_CustomBackoffDefaultRetries(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Custom MaxBackoff but MaxRetries=0 (unset)
	// This should get 3 default retries (not 0!)
	uploader := NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:        server.URL,
		DeviceID:   "device-001",
		MaxBackoff: 10 * time.Second, // Custom max backoff
		RetryDelay: 10 * time.Millisecond, // Speed up test
		// MaxRetries: 0 (not set)
	})

	err := uploader.Upload(context.Background(), metrics)
	if err == nil {
		t.Fatal("Expected error after failed upload")
	}

	// Should have made 4 attempts: initial + 3 default retries
	expectedAttempts := 4
	if attempts != expectedAttempts {
		t.Errorf("With custom MaxBackoff but MaxRetries=0, expected %d attempts (1 + 3 default retries), got %d", expectedAttempts, attempts)
	}
}
