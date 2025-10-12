package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
	"github.com/taniwha3/thugshells/internal/storage"
	"github.com/taniwha3/thugshells/internal/uploader"
)

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
	if _, err := uploadMetrics(ctx, store, mockUpload); err != nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload); err != nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload); err == nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload); err != nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload); err != nil {
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

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
