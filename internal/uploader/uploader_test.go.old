package uploader

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
)

func TestNewHTTPUploader(t *testing.T) {
	uploader := NewHTTPUploader("http://example.com/api/metrics", "test-device")

	if uploader == nil {
		t.Fatal("Expected non-nil uploader")
	}

	if uploader.GetURL() != "http://example.com/api/metrics" {
		t.Errorf("Expected URL http://example.com/api/metrics, got %s", uploader.GetURL())
	}

	if uploader.GetDeviceID() != "test-device" {
		t.Errorf("Expected device ID test-device, got %s", uploader.GetDeviceID())
	}
}

func TestUpload_Success(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Read and parse payload
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var payload MetricsPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("Failed to unmarshal payload: %v", err)
		}

		// Verify payload
		if payload.DeviceID != "test-device" {
			t.Errorf("Expected device_id test-device, got %s", payload.DeviceID)
		}

		if len(payload.Metrics) != 2 {
			t.Errorf("Expected 2 metrics, got %d", len(payload.Metrics))
		}

		if payload.Metrics[0].Name != "cpu.temperature" {
			t.Errorf("Expected metric name cpu.temperature, got %s", payload.Metrics[0].Name)
		}

		// Send success response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{
			Success:  true,
			Received: len(payload.Metrics),
		})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
		models.NewMetric("srt.packet_loss_pct", 0.5, "test-device"),
	}

	err := uploader.Upload(ctx, metrics)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
}

func TestUpload_EmptyMetrics(t *testing.T) {
	uploader := NewHTTPUploader("http://example.com/api/metrics", "test-device")
	ctx := context.Background()

	// Should not error on empty metrics
	err := uploader.Upload(ctx, []*models.Metric{})
	if err != nil {
		t.Errorf("Upload with empty metrics should not error: %v", err)
	}
}

func TestUpload_ServerError(t *testing.T) {
	// Mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
	}

	err := uploader.Upload(ctx, metrics)
	if err == nil {
		t.Error("Expected error on server error, got nil")
	}
}

func TestUpload_BadRequest(t *testing.T) {
	// Mock server that returns 400
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad request"))
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
	}

	err := uploader.Upload(ctx, metrics)
	if err == nil {
		t.Error("Expected error on bad request, got nil")
	}
}

func TestUpload_JSONErrorResponse(t *testing.T) {
	// Mock server that returns JSON error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   "Validation failed",
		})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
	}

	err := uploader.Upload(ctx, metrics)
	if err == nil {
		t.Error("Expected error on JSON error response, got nil")
	}
}

func TestUpload_NetworkError(t *testing.T) {
	// Use invalid URL to trigger network error
	uploader := NewHTTPUploader("http://localhost:0/api/metrics", "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
	}

	err := uploader.Upload(ctx, metrics)
	if err == nil {
		t.Error("Expected error on network failure, got nil")
	}
}

func TestUpload_ContextCancellation(t *testing.T) {
	// Mock server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
	}

	err := uploader.Upload(ctx, metrics)
	if err == nil {
		t.Error("Expected error on context cancellation, got nil")
	}
}

func TestUpload_PayloadFormat(t *testing.T) {
	var receivedPayload MetricsPayload

	// Mock server that captures payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{Success: true, Received: len(receivedPayload.Metrics)})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device-001")
	ctx := context.Background()

	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device-001").WithTimestamp(now),
		models.NewMetric("srt.packet_loss_pct", 1.5, "test-device-001").WithTimestamp(now),
	}

	err := uploader.Upload(ctx, metrics)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify payload structure
	if receivedPayload.DeviceID != "test-device-001" {
		t.Errorf("Expected device_id test-device-001, got %s", receivedPayload.DeviceID)
	}

	if len(receivedPayload.Metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(receivedPayload.Metrics))
	}

	// Verify first metric
	if receivedPayload.Metrics[0].Name != "cpu.temperature" {
		t.Errorf("Expected metric name cpu.temperature, got %s", receivedPayload.Metrics[0].Name)
	}

	if receivedPayload.Metrics[0].Value != 52.3 {
		t.Errorf("Expected value 52.3, got %.1f", receivedPayload.Metrics[0].Value)
	}

	// Verify timestamp format (should be RFC3339Nano)
	_, err = time.Parse(time.RFC3339Nano, receivedPayload.Metrics[0].Timestamp)
	if err != nil {
		t.Errorf("Invalid timestamp format: %v", err)
	}
}

func TestUpload_UserAgent(t *testing.T) {
	var userAgent string

	// Mock server that captures User-Agent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Response{Success: true})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
	}

	uploader.Upload(ctx, metrics)

	if userAgent != "thugshells-metrics-collector/1.0" {
		t.Errorf("Expected User-Agent thugshells-metrics-collector/1.0, got %s", userAgent)
	}
}

func TestUploadBatch(t *testing.T) {
	uploadCount := 0

	// Mock server that counts uploads
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{Success: true})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	batches := [][]*models.Metric{
		{models.NewMetric("cpu.temperature", 50.0, "test-device")},
		{models.NewMetric("cpu.temperature", 51.0, "test-device")},
		{models.NewMetric("cpu.temperature", 52.0, "test-device")},
	}

	err := uploader.UploadBatch(ctx, batches)
	if err != nil {
		t.Fatalf("UploadBatch failed: %v", err)
	}

	if uploadCount != 3 {
		t.Errorf("Expected 3 uploads, got %d", uploadCount)
	}
}

func TestUploadBatch_ErrorStopsUploads(t *testing.T) {
	uploadCount := 0

	// Mock server that fails on second upload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		if uploadCount == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Response{Success: true})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	batches := [][]*models.Metric{
		{models.NewMetric("cpu.temperature", 50.0, "test-device")},
		{models.NewMetric("cpu.temperature", 51.0, "test-device")},
		{models.NewMetric("cpu.temperature", 52.0, "test-device")},
	}

	err := uploader.UploadBatch(ctx, batches)
	if err == nil {
		t.Error("Expected error on failed upload, got nil")
	}

	// Should stop after second upload
	if uploadCount > 2 {
		t.Errorf("Expected uploads to stop after error, got %d uploads", uploadCount)
	}
}

func TestSetTimeout(t *testing.T) {
	uploader := NewHTTPUploader("http://example.com/api/metrics", "test-device")

	// Set custom timeout
	uploader.SetTimeout(5 * time.Second)

	if uploader.client.Timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", uploader.client.Timeout)
	}
}

func TestClose(t *testing.T) {
	uploader := NewHTTPUploader("http://example.com/api/metrics", "test-device")

	err := uploader.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestUpload_NonJSONResponse(t *testing.T) {
	// Mock server that returns non-JSON success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
	}

	// Should succeed even if response is not JSON
	err := uploader.Upload(ctx, metrics)
	if err != nil {
		t.Errorf("Upload with non-JSON response should succeed: %v", err)
	}
}

func TestUpload_LargePayload(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{Success: true})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	// Create large batch
	metrics := make([]*models.Metric, 1000)
	for i := range metrics {
		metrics[i] = models.NewMetric("cpu.temperature", float64(50+i%10), "test-device")
	}

	err := uploader.Upload(ctx, metrics)
	if err != nil {
		t.Fatalf("Upload of large payload failed: %v", err)
	}
}

func BenchmarkUpload(b *testing.B) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Response{Success: true})
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")
	ctx := context.Background()

	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.3, "test-device"),
		models.NewMetric("srt.packet_loss_pct", 0.5, "test-device"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		uploader.Upload(ctx, metrics)
	}
}
