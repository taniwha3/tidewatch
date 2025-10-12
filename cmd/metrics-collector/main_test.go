package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/taniwha3/thugshells/internal/logging"
	"github.com/taniwha3/thugshells/internal/models"
	"github.com/taniwha3/thugshells/internal/storage"
	"github.com/taniwha3/thugshells/internal/uploader"
)

// testLogger creates a logger for testing that writes to a buffer
func testLogger() *slog.Logger {
	var buf bytes.Buffer
	return logging.New(logging.Config{
		Level:  logging.LevelDebug,
		Format: logging.FormatConsole,
		Output: &buf,
	})
}

// mockUploader implements a simple mock uploader for testing
type mockUploader struct {
	uploadedMetrics []*models.Metric
	shouldFail      bool
}

func (m *mockUploader) Upload(ctx context.Context, metrics []*models.Metric) error {
	if m.shouldFail {
		return &uploader.RetryableError{Err: fmt.Errorf("mock upload failure")}
	}
	m.uploadedMetrics = append(m.uploadedMetrics, metrics...)
	return nil
}

func (m *mockUploader) UploadBatch(ctx context.Context, batches [][]*models.Metric) error {
	for _, batch := range batches {
		if err := m.Upload(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockUploader) Close() error {
	return nil
}

func TestUploadMetrics_MarksMetricsAsUploaded(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store some test metrics
	now := time.Now()
	testMetrics := []*models.Metric{
		{
			Name:        "test.metric1",
			TimestampMs: now.UnixMilli(),
			Value:       42.0,
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		},
		{
			Name:        "test.metric2",
			TimestampMs: now.UnixMilli() + 1000,
			Value:       43.0,
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		},
		{
			Name:        "test.metric3",
			TimestampMs: now.UnixMilli() + 2000,
			Value:       44.0,
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		},
	}

	if err := store.StoreBatch(ctx, testMetrics); err != nil {
		t.Fatalf("Failed to store metrics: %v", err)
	}

	// Verify all metrics are pending
	pendingBefore, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingBefore != 3 {
		t.Errorf("Expected 3 pending metrics, got %d", pendingBefore)
	}

	// Create mock uploader
	mockUpload := &mockUploader{}

	// Upload metrics
	logger := testLogger()
	if _, err := uploadMetrics(ctx, store, mockUpload, logger); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify metrics were uploaded
	if len(mockUpload.uploadedMetrics) != 3 {
		t.Errorf("Expected 3 metrics uploaded, got %d", len(mockUpload.uploadedMetrics))
	}

	// Verify metrics are marked as uploaded (pending count should be 0)
	pendingAfter, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingAfter != 0 {
		t.Errorf("Expected 0 pending metrics after upload, got %d", pendingAfter)
	}

	// Verify second upload attempt returns no metrics
	mockUpload.uploadedMetrics = nil
	if _, err := uploadMetrics(ctx, store, mockUpload, logger); err != nil {
		t.Fatalf("Second upload failed: %v", err)
	}
	if len(mockUpload.uploadedMetrics) != 0 {
		t.Errorf("Expected 0 metrics on second upload, got %d", len(mockUpload.uploadedMetrics))
	}
}

func TestUploadMetrics_DoesNotMarkOnFailure(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store some test metrics
	now := time.Now()
	testMetrics := []*models.Metric{
		{
			Name:        "test.metric1",
			TimestampMs: now.UnixMilli(),
			Value:       42.0,
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		},
	}

	if err := store.StoreBatch(ctx, testMetrics); err != nil {
		t.Fatalf("Failed to store metrics: %v", err)
	}

	// Create failing mock uploader
	mockUpload := &mockUploader{shouldFail: true}

	// Attempt upload (should fail)
	logger := testLogger()
	if _, err := uploadMetrics(ctx, store, mockUpload, logger); err == nil {
		t.Fatal("Expected upload to fail, but it succeeded")
	}

	// Verify metrics are still pending
	pendingAfter, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingAfter != 1 {
		t.Errorf("Expected 1 pending metric after failed upload, got %d", pendingAfter)
	}
}

func TestUploadMetrics_BatchLimit(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store 3000 test metrics (more than batch size of 2500)
	now := time.Now()
	var testMetrics []*models.Metric
	for i := 0; i < 3000; i++ {
		testMetrics = append(testMetrics, &models.Metric{
			Name:        "test.metric",
			TimestampMs: now.UnixMilli() + int64(i*1000),
			Value:       float64(i),
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		})
	}

	if err := store.StoreBatch(ctx, testMetrics); err != nil {
		t.Fatalf("Failed to store metrics: %v", err)
	}

	// Verify all metrics are pending
	pendingBefore, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingBefore != 3000 {
		t.Errorf("Expected 3000 pending metrics, got %d", pendingBefore)
	}

	// Create mock uploader
	mockUpload := &mockUploader{}

	// First upload should only upload 2500 (batch limit)
	logger := testLogger()
	if _, err := uploadMetrics(ctx, store, mockUpload, logger); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if len(mockUpload.uploadedMetrics) != 2500 {
		t.Errorf("Expected 2500 metrics uploaded in first batch, got %d", len(mockUpload.uploadedMetrics))
	}

	// Verify 500 metrics are still pending
	pendingAfter, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingAfter != 500 {
		t.Errorf("Expected 500 pending metrics after first upload, got %d", pendingAfter)
	}

	// Second upload should upload remaining 500
	mockUpload.uploadedMetrics = nil
	if _, err := uploadMetrics(ctx, store, mockUpload, logger); err != nil {
		t.Fatalf("Second upload failed: %v", err)
	}

	if len(mockUpload.uploadedMetrics) != 500 {
		t.Errorf("Expected 500 metrics uploaded in second batch, got %d", len(mockUpload.uploadedMetrics))
	}

	// Verify no metrics are pending
	pendingFinal, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingFinal != 0 {
		t.Errorf("Expected 0 pending metrics after all uploads, got %d", pendingFinal)
	}
}

func TestUploadMetrics_StringMetricsRemainInStorage(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store 2500 string metrics followed by 100 numeric metrics
	// This tests that QueryUnuploaded only returns numeric metrics
	now := time.Now()
	var testMetrics []*models.Metric

	// Add 2500 string metrics (should NOT be queried for upload)
	for i := 0; i < 2500; i++ {
		testMetrics = append(testMetrics, &models.Metric{
			Name:        "test.string_metric",
			TimestampMs: now.UnixMilli() + int64(i*1000),
			Value:       0, // String metrics use ValueText, not Value
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeString,
			ValueText:   fmt.Sprintf("string_value_%d", i),
		})
	}

	// Add 100 numeric metrics (should be uploaded)
	for i := 0; i < 100; i++ {
		testMetrics = append(testMetrics, &models.Metric{
			Name:        "test.numeric_metric",
			TimestampMs: now.UnixMilli() + int64((2500+i)*1000),
			Value:       float64(i),
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		})
	}

	if err := store.StoreBatch(ctx, testMetrics); err != nil {
		t.Fatalf("Failed to store metrics: %v", err)
	}

	// GetPendingCount only counts unuploaded NUMERIC metrics (value_type=0)
	// This prevents string metrics from inflating pending count and causing health issues
	pendingBefore, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingBefore != 100 {
		t.Errorf("Expected 100 pending numeric metrics, got %d", pendingBefore)
	}

	// Create mock uploader
	mockUpload := &mockUploader{}

	// Upload should only process 100 numeric metrics (string metrics filtered by QueryUnuploaded)
	logger := testLogger()
	count, err := uploadMetrics(ctx, store, mockUpload, logger)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Return value should be 100 (only numeric metrics)
	if count != 100 {
		t.Errorf("Expected upload count of 100 (numeric metrics only), got %d", count)
	}

	// Mock uploader should have received only 100 numeric metrics
	if len(mockUpload.uploadedMetrics) != 100 {
		t.Errorf("Expected 100 numeric metrics uploaded, got %d", len(mockUpload.uploadedMetrics))
	}

	// Verify all uploaded metrics are numeric
	for i, m := range mockUpload.uploadedMetrics {
		if m.ValueType != models.ValueTypeNumeric {
			t.Errorf("Metric %d has wrong type: expected numeric (0), got %d", i, m.ValueType)
		}
	}

	// Critical: String metrics remain in SQLite with uploaded=0 for local processing
	// GetPendingCount now returns 0 because it only counts numeric metrics (value_type=0)
	// This prevents string metrics from triggering health degradation
	pendingAfter, err := store.GetPendingCount(ctx)
	if err != nil {
		t.Fatalf("Failed to get pending count: %v", err)
	}
	if pendingAfter != 0 {
		t.Errorf("Expected 0 pending numeric metrics (all uploaded), got %d", pendingAfter)
	}

	// Verify second upload finds no numeric metrics (but string metrics still present)
	mockUpload.uploadedMetrics = nil
	count2, err := uploadMetrics(ctx, store, mockUpload, logger)
	if err != nil {
		t.Fatalf("Second upload failed: %v", err)
	}

	if count2 != 0 {
		t.Errorf("Expected 0 metrics on second upload (all numeric already uploaded), got %d", count2)
	}
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
