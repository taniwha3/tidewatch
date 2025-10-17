package uploader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/taniwha3/tidewatch/internal/models"
)

// Uploader is the interface for uploading metrics to remote endpoints
type Uploader interface {
	// Upload sends metrics to the remote endpoint
	Upload(ctx context.Context, metrics []*models.Metric) error

	// UploadBatch sends multiple batches of metrics
	UploadBatch(ctx context.Context, batches [][]*models.Metric) error

	// Close closes the uploader and releases resources
	Close() error
}

// HTTPUploader implements Uploader using HTTP POST to VictoriaMetrics
type HTTPUploader struct {
	url               string
	deviceID          string
	authToken         string
	client            *http.Client
	maxRetries        int
	retryDelay        time.Duration
	maxBackoff        time.Duration
	backoffMultiplier float64
	jitterPercent     int
	chunkSize         int
	rng               *rand.Rand   // Per-uploader RNG for jitter to prevent thundering herd
	rngMu             sync.Mutex   // Protects rng for concurrent access
}

// HTTPUploaderConfig configures the HTTP uploader
type HTTPUploaderConfig struct {
	URL               string
	DeviceID          string
	AuthToken         string        // Optional bearer token
	Timeout           time.Duration // Default: 30s
	MaxRetries        *int          // Default: 3. Use nil for default, &0 for explicitly 0 (no retries)
	RetryDelay        time.Duration // Base delay for exponential backoff, default: 1s
	MaxBackoff        time.Duration // Maximum backoff delay, default: 30s
	BackoffMultiplier float64       // Backoff multiplier for exponential backoff, default: 2.0
	JitterPercent     *int          // Jitter percentage (0-100), default: 20. Use nil for default, &0 for explicitly 0
	ChunkSize         int           // Metrics per chunk, default: 50
}

// NewHTTPUploader creates a new HTTP uploader with default settings
func NewHTTPUploader(url, deviceID string) *HTTPUploader {
	maxRetries := 3
	jitterPercent := 20
	return NewHTTPUploaderWithConfig(HTTPUploaderConfig{
		URL:           url,
		DeviceID:      deviceID,
		Timeout:       30 * time.Second,
		MaxRetries:    &maxRetries,
		RetryDelay:    1 * time.Second,
		JitterPercent: &jitterPercent,
		ChunkSize:     50,
	})
}

// NewHTTPUploaderWithConfig creates a new HTTP uploader with custom configuration
// MaxRetries: nil = use default (3), &0 = explicitly 0 (no retries), &N = N retries
// JitterPercent: nil = use default (20%), &0 = explicitly 0% (no jitter), &N = N%
func NewHTTPUploaderWithConfig(cfg HTTPUploaderConfig) *HTTPUploader {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// MaxRetries: nil means use default, otherwise honor the value (even if 0)
	maxRetries := 3 // default
	if cfg.MaxRetries != nil {
		maxRetries = *cfg.MaxRetries
	}

	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 1 * time.Second
	}

	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}

	backoffMultiplier := cfg.BackoffMultiplier
	if backoffMultiplier == 0 {
		backoffMultiplier = 2.0
	}

	// JitterPercent: nil means use default (20 to prevent thundering herd), otherwise honor the value (even if 0)
	jitterPercent := 20 // default
	if cfg.JitterPercent != nil {
		jitterPercent = *cfg.JitterPercent
	}

	chunkSize := cfg.ChunkSize
	if chunkSize == 0 {
		chunkSize = 50
	}

	return &HTTPUploader{
		url:               cfg.URL,
		deviceID:          cfg.DeviceID,
		authToken:         cfg.AuthToken,
		maxRetries:        maxRetries,
		retryDelay:        retryDelay,
		maxBackoff:        maxBackoff,
		backoffMultiplier: backoffMultiplier,
		jitterPercent:     jitterPercent,
		chunkSize:         chunkSize,
		rng:               rand.New(rand.NewSource(time.Now().UnixNano())),
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Upload sends metrics to VictoriaMetrics with chunking, compression, and retry
func (u *HTTPUploader) Upload(ctx context.Context, metrics []*models.Metric) error {
	_, err := u.UploadAndGetIDs(ctx, metrics)
	return err
}

// UploadAndGetIDs sends metrics to VictoriaMetrics and returns the storage IDs of metrics actually uploaded
// String metrics are filtered out and their IDs are NOT included in the returned slice
func (u *HTTPUploader) UploadAndGetIDs(ctx context.Context, metrics []*models.Metric) ([]int64, error) {
	if len(metrics) == 0 {
		return nil, nil
	}

	// Build chunks (includes JSONL formatting and gzip compression)
	chunks, err := BuildChunks(metrics, u.chunkSize)
	if err != nil {
		return nil, fmt.Errorf("failed to build chunks: %w", err)
	}

	// Collect all included IDs from chunks
	var allIncludedIDs []int64
	for _, chunk := range chunks {
		allIncludedIDs = append(allIncludedIDs, chunk.IncludedIDs...)
	}

	// Upload each chunk with retry
	for i, chunk := range chunks {
		if err := u.uploadChunkWithRetry(ctx, chunk, i); err != nil {
			return nil, fmt.Errorf("failed to upload chunk %d/%d: %w", i+1, len(chunks), err)
		}
	}

	return allIncludedIDs, nil
}

// uploadChunkWithRetry uploads a single chunk with exponential backoff retry
func (u *HTTPUploader) uploadChunkWithRetry(ctx context.Context, chunk *Chunk, chunkIndex int) error {
	var lastErr error

	for attempt := 0; attempt <= u.maxRetries; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := u.uploadChunk(ctx, chunk, chunkIndex, attempt)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if we should retry
		if !isRetryable(err) {
			return fmt.Errorf("non-retryable error: %w", err)
		}

		// Don't sleep after the last attempt
		if attempt < u.maxRetries {
			delay := u.calculateBackoff(attempt, err)
			select {
			case <-time.After(delay):
				// Continue to next attempt
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", u.maxRetries, lastErr)
}

// uploadChunk uploads a single chunk to VictoriaMetrics
func (u *HTTPUploader) uploadChunk(ctx context.Context, chunk *Chunk, chunkIndex, attempt int) error {
	// Create request with compressed data
	req, err := http.NewRequestWithContext(ctx, "POST", u.url, bytes.NewReader(chunk.CompressedData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers per engineering review
	req.Header.Set("Content-Type", "application/x-ndjson") // JSONL / newline-delimited JSON
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("User-Agent", "tidewatch/1.0")
	req.Header.Set("X-Device-ID", u.deviceID)

	// Add authorization if configured
	if u.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.authToken)
	}

	// Add debug/tracking headers
	req.Header.Set("X-Chunk-Index", strconv.Itoa(chunkIndex))
	req.Header.Set("X-Chunk-Metrics", strconv.Itoa(len(chunk.Metrics)))
	req.Header.Set("X-Attempt", strconv.Itoa(attempt))

	// Send request
	resp, err := u.client.Do(req)
	if err != nil {
		return &RetryableError{Err: err}
	}
	defer resp.Body.Close()

	// Read response body (limited to prevent memory issues)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	// M2 Simplified Strategy: 2xx = entire chunk succeeded
	// Future enhancement: Parse VM response for partial success details
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil // Success - entire chunk uploaded
	}

	// Handle specific status codes
	switch resp.StatusCode {
	case http.StatusBadRequest: // 400
		return &NonRetryableError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("bad request: %s", string(respBody)),
		}
	case http.StatusUnauthorized: // 401
		return &NonRetryableError{
			StatusCode: resp.StatusCode,
			Message:    "unauthorized - check auth token",
		}
	case http.StatusTooManyRequests: // 429
		return &RateLimitError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		// 500, 502, 503, 504 - server errors are retryable
		return &RetryableError{
			Err: fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody)),
		}
	default:
		// Other errors are retryable by default
		return &RetryableError{
			Err: fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody)),
		}
	}
}

// UploadBatch sends multiple batches of metrics
func (u *HTTPUploader) UploadBatch(ctx context.Context, batches [][]*models.Metric) error {
	for _, batch := range batches {
		if err := u.Upload(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the uploader and releases resources
func (u *HTTPUploader) Close() error {
	u.client.CloseIdleConnections()
	return nil
}

// SetTimeout sets the HTTP client timeout
func (u *HTTPUploader) SetTimeout(timeout time.Duration) {
	u.client.Timeout = timeout
}

// GetURL returns the configured upload URL
func (u *HTTPUploader) GetURL() string {
	return u.url
}

// GetDeviceID returns the configured device ID
func (u *HTTPUploader) GetDeviceID() string {
	return u.deviceID
}

// Error types for retry logic

// RetryableError indicates an error that should be retried
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error: %v", e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NonRetryableError indicates an error that should not be retried
type NonRetryableError struct {
	StatusCode int
	Message    string
}

func (e *NonRetryableError) Error() string {
	return fmt.Sprintf("non-retryable error (status %d): %s", e.StatusCode, e.Message)
}

// RateLimitError indicates rate limiting with optional Retry-After
type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited (status %d): retry after %v", e.StatusCode, e.RetryAfter)
	}
	return fmt.Sprintf("rate limited (status %d)", e.StatusCode)
}

// isRetryable checks if an error should be retried
func isRetryable(err error) bool {
	// Check for explicit non-retryable errors
	var nonRetryable *NonRetryableError
	if errors.As(err, &nonRetryable) {
		return false
	}

	// Rate limits and retryable errors should be retried
	return true
}

// calculateBackoff calculates exponential backoff with jitter
func (u *HTTPUploader) calculateBackoff(attempt int, err error) time.Duration {
	// Check for Retry-After header in rate limit errors
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) && rateLimitErr.RetryAfter > 0 {
		// Add jitter to Retry-After
		u.rngMu.Lock()
		jitter := time.Duration(u.rng.Float64() * float64(time.Second))
		u.rngMu.Unlock()
		return rateLimitErr.RetryAfter + jitter
	}

	// Exponential backoff: baseDelay * multiplier^attempt
	backoff := float64(u.retryDelay) * math.Pow(u.backoffMultiplier, float64(attempt))

	// Cap at configured max backoff
	if backoff > float64(u.maxBackoff) {
		backoff = float64(u.maxBackoff)
	}

	// Add jitter using configured percentage (convert to fraction)
	jitterFraction := float64(u.jitterPercent) / 100.0
	u.rngMu.Lock()
	jitter := backoff * jitterFraction * (u.rng.Float64()*2 - 1)
	u.rngMu.Unlock()
	backoff += jitter

	return time.Duration(backoff)
}

// parseRetryAfter parses the Retry-After header (seconds or HTTP date)
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}

	// Try parsing as seconds
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := time.Parse(time.RFC1123, header); err == nil {
		duration := time.Until(t)
		if duration > 0 {
			return duration
		}
	}

	return 0
}
