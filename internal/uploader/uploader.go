package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
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

// HTTPUploader implements Uploader using HTTP POST
type HTTPUploader struct {
	url      string
	deviceID string
	client   *http.Client
}

// NewHTTPUploader creates a new HTTP uploader
func NewHTTPUploader(url, deviceID string) *HTTPUploader {
	return &HTTPUploader{
		url:      url,
		deviceID: deviceID,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// MetricsPayload represents the JSON payload sent to the remote endpoint
type MetricsPayload struct {
	DeviceID  string          `json:"device_id"`
	Timestamp string          `json:"timestamp"`
	Metrics   []MetricPayload `json:"metrics"`
}

// MetricPayload represents a single metric in the JSON payload
type MetricPayload struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Timestamp string  `json:"timestamp"`
}

// Response represents the response from the remote endpoint
type Response struct {
	Success  bool   `json:"success"`
	Received int    `json:"received"`
	Error    string `json:"error,omitempty"`
}

// Upload sends metrics to the remote endpoint
func (u *HTTPUploader) Upload(ctx context.Context, metrics []*models.Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	// Build payload
	payload := MetricsPayload{
		DeviceID:  u.deviceID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Metrics:   make([]MetricPayload, len(metrics)),
	}

	for i, m := range metrics {
		payload.Metrics[i] = MetricPayload{
			Name:      m.Name,
			Value:     m.Value,
			Timestamp: time.UnixMilli(m.TimestampMs).UTC().Format(time.RFC3339Nano),
		}
	}

	// Marshal to JSON
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", u.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "thugshells-metrics-collector/1.0")

	// Send request
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response (optional validation)
	var response Response
	if err := json.Unmarshal(respBody, &response); err != nil {
		// Non-fatal: server may not return JSON
		return nil
	}

	if !response.Success {
		return fmt.Errorf("upload failed: %s", response.Error)
	}

	return nil
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
