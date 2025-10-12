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
		MaxRetries: 3,
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
		MaxRetries: 3,
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
		MaxRetries: 3,
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
		MaxRetries: 3,
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
		MaxRetries: 4,
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
		MaxRetries: 2,
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
		MaxRetries: 2,
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

// Helper function
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
