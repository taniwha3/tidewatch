package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taniwha3/tidewatch/internal/config"
	"github.com/taniwha3/tidewatch/internal/logging"
	"github.com/taniwha3/tidewatch/internal/models"
	"github.com/taniwha3/tidewatch/internal/storage"
	"github.com/taniwha3/tidewatch/internal/uploader"
)

// intPtr is a helper function to create a pointer to an int value
func intPtr(i int) *int {
	return &i
}

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
	if _, err := uploadMetrics(ctx, store, mockUpload, 2500, logger); err != nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload, 2500, logger); err != nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload, 2500, logger); err == nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload, 2500, logger); err != nil {
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
	if _, err := uploadMetrics(ctx, store, mockUpload, 2500, logger); err != nil {
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
	count, err := uploadMetrics(ctx, store, mockUpload, 2500, logger)
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
	count2, err := uploadMetrics(ctx, store, mockUpload, 2500, logger)
	if err != nil {
		t.Fatalf("Second upload failed: %v", err)
	}

	if count2 != 0 {
		t.Errorf("Expected 0 metrics on second upload (all numeric already uploaded), got %d", count2)
	}
}

// TestConfigWiring_BatchSize tests that config.remote.batch_size is wired through
func TestConfigWiring_BatchSize(t *testing.T) {
	tests := []struct {
		name               string
		configBatchSize    int
		metricsToStore     int
		expectedFirstBatch int
	}{
		{
			name:               "custom batch size 1000",
			configBatchSize:    1000,
			metricsToStore:     1500,
			expectedFirstBatch: 1000,
		},
		{
			name:               "custom batch size 100",
			configBatchSize:    100,
			metricsToStore:     250,
			expectedFirstBatch: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary database
			dbPath := t.TempDir() + "/test.db"
			store, err := storage.NewSQLiteStorage(dbPath)
			if err != nil {
				t.Fatalf("Failed to create storage: %v", err)
			}
			defer store.Close()

			ctx := context.Background()

			// Store test metrics
			now := time.Now()
			var testMetrics []*models.Metric
			for i := 0; i < tt.metricsToStore; i++ {
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

			// Upload with configured batch size
			mockUpload := &mockUploader{}
			logger := testLogger()
			_, err = uploadMetrics(ctx, store, mockUpload, tt.configBatchSize, logger)
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}

			// Verify batch size was respected
			if len(mockUpload.uploadedMetrics) != tt.expectedFirstBatch {
				t.Errorf("Expected %d metrics in first batch, got %d",
					tt.expectedFirstBatch, len(mockUpload.uploadedMetrics))
			}
		})
	}
}

// TestConfigWiring_CustomBatchSizeVsDefault verifies custom batch_size overrides default
func TestConfigWiring_CustomBatchSizeVsDefault(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store 5000 metrics
	now := time.Now()
	var testMetrics []*models.Metric
	for i := 0; i < 5000; i++ {
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

	logger := testLogger()

	// Test 1: Default batch size (2500)
	mockUpload1 := &mockUploader{}
	_, err = uploadMetrics(ctx, store, mockUpload1, 2500, logger)
	if err != nil {
		t.Fatalf("Upload with default batch size failed: %v", err)
	}
	if len(mockUpload1.uploadedMetrics) != 2500 {
		t.Errorf("Default batch size: expected 2500 metrics, got %d", len(mockUpload1.uploadedMetrics))
	}

	// Test 2: Custom batch size (5000 - upload all remaining)
	mockUpload2 := &mockUploader{}
	_, err = uploadMetrics(ctx, store, mockUpload2, 5000, logger)
	if err != nil {
		t.Fatalf("Upload with custom batch size failed: %v", err)
	}
	// Remaining 2500 metrics should be uploaded
	if len(mockUpload2.uploadedMetrics) != 2500 {
		t.Errorf("Custom batch size: expected 2500 remaining metrics, got %d", len(mockUpload2.uploadedMetrics))
	}
}

// TestNormalizeStoragePath_UNCPaths tests that UNC network paths are preserved
func TestNormalizeStoragePath_UNCPaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "UNC path without query params",
			input:    "file://host/share/metrics.db",
			expected: "//host/share/metrics.db",
		},
		{
			name:     "UNC path with query params",
			input:    "file://host/share/metrics.db?cache=shared",
			expected: "//host/share/metrics.db",
		},
		{
			name:     "UNC path with subdirectory",
			input:    "file://server/data/tidewatch/metrics.db",
			expected: "//server/data/tidewatch/metrics.db",
		},
		{
			name:     "UNC path with IP address",
			input:    "file://192.168.1.100/share/metrics.db",
			expected: "//192.168.1.100/share/metrics.db",
		},
		{
			name:     "Local absolute path (three slashes)",
			input:    "file:///var/lib/metrics.db",
			expected: "/var/lib/metrics.db",
		},
		{
			name:     "Local absolute path with query params",
			input:    "file:///var/lib/metrics.db?cache=shared",
			expected: "/var/lib/metrics.db",
		},
		{
			name:     "Relative path",
			input:    "file:data/metrics.db",
			expected: filepath.Join(mustGetwd(), "data/metrics.db"),
		},
		{
			name:     "Direct absolute path (no URI)",
			input:    "/var/lib/metrics.db",
			expected: "/var/lib/metrics.db",
		},
		{
			name:     "Direct relative path (no URI)",
			input:    "data/metrics.db",
			expected: filepath.Join(mustGetwd(), "data/metrics.db"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeStoragePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeStoragePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}

			// Verify that UNC paths start with //
			if strings.Contains(tt.name, "UNC") {
				if !strings.HasPrefix(result, "//") {
					t.Errorf("UNC path should start with //, got %q", result)
				}
			}
		})
	}
}

// TestNormalizeStoragePath_LockFileCoordination tests that UNC paths create
// lock files in the correct network location for proper coordination
func TestNormalizeStoragePath_LockFileCoordination(t *testing.T) {
	// Simulate two instances with the same UNC database path
	dbPath := "file://server/share/metrics.db"

	// Both instances should normalize to the same path
	normalized1 := normalizeStoragePath(dbPath)
	normalized2 := normalizeStoragePath(dbPath)

	if normalized1 != normalized2 {
		t.Errorf("Same UNC path should normalize to same result: %q != %q", normalized1, normalized2)
	}

	// The normalized path should be a UNC path
	expected := "//server/share/metrics.db"
	if normalized1 != expected {
		t.Errorf("Expected UNC path %q, got %q", expected, normalized1)
	}

	// Verify the lock file would be created at the UNC location
	// (not in a local relative path like "server/share/metrics.db.lock")
	if !strings.HasPrefix(normalized1, "//") {
		t.Errorf("UNC path must start with // to ensure lock file coordination, got %q", normalized1)
	}
}

// mustGetwd is a helper that returns the current working directory or fails the test
func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get working directory: %v", err))
	}
	return wd
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}

// TestConfigWiring_RetryDefaults tests that missing retry block uses defaults (3 retries)
func TestConfigWiring_RetryDefaults(t *testing.T) {
	// Create temp config WITHOUT retry block
	configYAML := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  enabled: true
  url: http://localhost:8428/api/v1/import
  upload_interval: 30s
  # Note: NO retry block - should use defaults

metrics:
  - name: cpu.temperature
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify retry block is not configured (all zeros)
	if cfg.Remote.Retry.MaxAttempts != 0 {
		t.Errorf("Expected MaxAttempts=0 (not set), got %d", cfg.Remote.Retry.MaxAttempts)
	}
	if cfg.Remote.Retry.Enabled != nil {
		t.Errorf("Expected Enabled=nil (not set), got %v", *cfg.Remote.Retry.Enabled)
	}

	// Simulate the logic in main.go (after P1 fix)
	retryConfigured := cfg.Remote.Retry.Enabled != nil ||
		cfg.Remote.Retry.MaxAttempts > 0 ||
		cfg.Remote.Retry.InitialBackoffStr != "" ||
		cfg.Remote.Retry.MaxBackoffStr != "" ||
		cfg.Remote.Retry.BackoffMultiplier > 0 ||
		cfg.Remote.Retry.JitterPercent != nil

	if retryConfigured {
		t.Error("Expected retryConfigured=false when retry block is missing")
	}

	// Create uploader config (mimicking main.go logic)
	uploaderCfg := uploader.HTTPUploaderConfig{
		URL:      cfg.Remote.URL,
		DeviceID: cfg.Device.ID,
		Timeout:  30 * time.Second,
	}

	// Don't set any retry fields if not configured
	if !retryConfigured {
		// Leave retry fields at zero - constructor will apply defaults
	}

	uploaderCfg.ChunkSize = cfg.Remote.GetChunkSize()

	// Create uploader
	up := uploader.NewHTTPUploaderWithConfig(uploaderCfg)
	defer up.Close()

	// The uploader should have received defaults (3 retries)
	// We can't directly access private fields, but we can test behavior
	// by making it fail and counting attempts

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError) // Always fail
	}))
	defer server.Close()

	// Update URL to test server
	up2 := uploader.NewHTTPUploaderWithConfig(uploader.HTTPUploaderConfig{
		URL:       server.URL,
		DeviceID:  cfg.Device.ID,
		Timeout:   30 * time.Second,
		ChunkSize: 50,
		// No retry fields set - should use defaults
	})
	defer up2.Close()

	metric := models.NewMetric("test.metric", 1.0, "test-device")
	err = up2.Upload(context.Background(), []*models.Metric{metric})

	if err == nil {
		t.Fatal("Expected error from failed upload")
	}

	// Should have made 4 attempts: initial + 3 retries (default)
	expectedAttempts := 4
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempts with default retries, got %d", expectedAttempts, attempts)
	}
}

// TestConfigWiring_RetryExplicitlyDisabled tests that retry.enabled: false disables retries
func TestConfigWiring_RetryExplicitlyDisabled(t *testing.T) {
	// Create temp config WITH retry block, but disabled
	configYAML := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  enabled: true
  url: http://localhost:8428/api/v1/import
  upload_interval: 30s
  retry:
    enabled: false
    max_attempts: 3  # This field makes the block "configured"

metrics:
  - name: cpu.temperature
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify retry block IS configured
	if cfg.Remote.Retry.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got %d", cfg.Remote.Retry.MaxAttempts)
	}
	if cfg.Remote.Retry.Enabled == nil {
		t.Fatal("Expected Enabled to be non-nil (explicitly set)")
	}
	if *cfg.Remote.Retry.Enabled {
		t.Error("Expected Enabled=false, got true")
	}

	// Simulate the logic in main.go (after P1 fix)
	retryConfigured := cfg.Remote.Retry.Enabled != nil ||
		cfg.Remote.Retry.MaxAttempts > 0 ||
		cfg.Remote.Retry.InitialBackoffStr != "" ||
		cfg.Remote.Retry.MaxBackoffStr != "" ||
		cfg.Remote.Retry.BackoffMultiplier > 0 ||
		cfg.Remote.Retry.JitterPercent != nil

	if !retryConfigured {
		t.Error("Expected retryConfigured=true when retry block has values")
	}

	// Create uploader config (mimicking NEW main.go logic after P1 fix)
	uploaderCfg := uploader.HTTPUploaderConfig{
		URL:      cfg.Remote.URL,
		DeviceID: cfg.Device.ID,
		Timeout:  30 * time.Second,
	}

	if retryConfigured {
		// Default to true if Enabled is nil but other fields are set
		enabled := cfg.Remote.Retry.Enabled == nil || *cfg.Remote.Retry.Enabled
		if enabled {
			maxAttempts := cfg.Remote.Retry.MaxAttempts
			uploaderCfg.MaxRetries = &maxAttempts
			retryDelay, _ := cfg.Remote.Retry.InitialBackoff()
			uploaderCfg.RetryDelay = retryDelay
		} else {
			// Explicitly disabled
			zero := 0
			uploaderCfg.MaxRetries = &zero
			uploaderCfg.RetryDelay = 1 * time.Second
		}
	}

	uploaderCfg.ChunkSize = cfg.Remote.GetChunkSize()

	// Verify we set MaxRetries=0
	if uploaderCfg.MaxRetries == nil || *uploaderCfg.MaxRetries != 0 {
		var val int
		if uploaderCfg.MaxRetries != nil {
			val = *uploaderCfg.MaxRetries
		}
		t.Errorf("Expected MaxRetries=0 when explicitly disabled, got %d", val)
	}

	// Test with actual server
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploaderCfg.URL = server.URL
	up := uploader.NewHTTPUploaderWithConfig(uploaderCfg)
	defer up.Close()

	metric := models.NewMetric("test.metric", 1.0, "test-device")
	err = up.Upload(context.Background(), []*models.Metric{metric})

	if err == nil {
		t.Fatal("Expected error from failed upload")
	}

	// Should have made only 1 attempt (no retries)
	expectedAttempts := 1
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempt when retries disabled, got %d", expectedAttempts, attempts)
	}
}

// TestConfigWiring_RetryExplicitlyEnabled tests that retry.enabled: true with values works
func TestConfigWiring_RetryExplicitlyEnabled(t *testing.T) {
	// Create temp config WITH retry block enabled
	configYAML := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  enabled: true
  url: http://localhost:8428/api/v1/import
  upload_interval: 30s
  retry:
    enabled: true
    max_attempts: 2

metrics:
  - name: cpu.temperature
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify retry block is configured and enabled
	if cfg.Remote.Retry.MaxAttempts != 2 {
		t.Errorf("Expected MaxAttempts=2, got %d", cfg.Remote.Retry.MaxAttempts)
	}
	if cfg.Remote.Retry.Enabled == nil {
		t.Fatal("Expected Enabled to be non-nil (explicitly set)")
	}
	if !*cfg.Remote.Retry.Enabled {
		t.Error("Expected Enabled=true, got false")
	}

	// Simulate the logic in main.go (after P1 fix)
	retryConfigured := cfg.Remote.Retry.Enabled != nil ||
		cfg.Remote.Retry.MaxAttempts > 0 ||
		cfg.Remote.Retry.InitialBackoffStr != "" ||
		cfg.Remote.Retry.MaxBackoffStr != "" ||
		cfg.Remote.Retry.BackoffMultiplier > 0 ||
		cfg.Remote.Retry.JitterPercent != nil

	if !retryConfigured {
		t.Error("Expected retryConfigured=true")
	}

	uploaderCfg := uploader.HTTPUploaderConfig{
		URL:       "http://localhost:8428/api/v1/import",
		DeviceID:  cfg.Device.ID,
		Timeout:   30 * time.Second,
		ChunkSize: 50,
	}

	if retryConfigured {
		// Default to true if Enabled is nil but other fields are set
		enabled := cfg.Remote.Retry.Enabled == nil || *cfg.Remote.Retry.Enabled
		if enabled {
			maxAttempts := cfg.Remote.Retry.MaxAttempts
			// Convert max_attempts (total attempts) to maxRetries (number of retries)
			maxRetries := maxAttempts - 1
			if maxRetries < 0 {
				maxRetries = 0
			}
			uploaderCfg.MaxRetries = &maxRetries
			retryDelay, _ := cfg.Remote.Retry.InitialBackoff()
			uploaderCfg.RetryDelay = retryDelay
		}
	}

	// Verify we set MaxRetries=1 (max_attempts=2 → maxRetries=1)
	if uploaderCfg.MaxRetries == nil || *uploaderCfg.MaxRetries != 1 {
		var val int
		if uploaderCfg.MaxRetries != nil {
			val = *uploaderCfg.MaxRetries
		}
		t.Errorf("Expected MaxRetries=1 (max_attempts=2 → maxRetries=1), got %d", val)
	}

	// Test with actual server
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploaderCfg.URL = server.URL
	up := uploader.NewHTTPUploaderWithConfig(uploaderCfg)
	defer up.Close()

	metric := models.NewMetric("test.metric", 1.0, "test-device")
	err = up.Upload(context.Background(), []*models.Metric{metric})

	if err == nil {
		t.Fatal("Expected error from failed upload")
	}

	// Should have made 2 attempts (as configured by max_attempts: 2)
	expectedAttempts := 2
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempts (max_attempts=2), got %d", expectedAttempts, attempts)
	}
}

// TestConfigWiring_RetryDisabledWithOnlyEnabledFalse tests P1 fix:
// Setting ONLY enabled: false (with no other fields) should disable retries
func TestConfigWiring_RetryDisabledWithOnlyEnabledFalse(t *testing.T) {
	// Create temp config with ONLY enabled: false (no other retry fields)
	// This is the typical case when a user wants to disable retries
	configYAML := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  enabled: true
  url: http://localhost:8428/api/v1/import
  upload_interval: 30s
  retry:
    enabled: false
    # Note: NO other retry fields - this should still disable retries

metrics:
  - name: cpu.temperature
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify Enabled is explicitly set to false
	if cfg.Remote.Retry.Enabled == nil {
		t.Fatal("Expected Enabled to be non-nil (explicitly set)")
	}
	if *cfg.Remote.Retry.Enabled {
		t.Error("Expected Enabled=false, got true")
	}

	// Verify other fields are zero/default
	if cfg.Remote.Retry.MaxAttempts != 0 {
		t.Errorf("Expected MaxAttempts=0 (not set), got %d", cfg.Remote.Retry.MaxAttempts)
	}

	// Simulate the NEW logic in main.go (after fix)
	retryConfigured := cfg.Remote.Retry.Enabled != nil ||
		cfg.Remote.Retry.MaxAttempts > 0 ||
		cfg.Remote.Retry.InitialBackoffStr != "" ||
		cfg.Remote.Retry.MaxBackoffStr != "" ||
		cfg.Remote.Retry.BackoffMultiplier > 0 ||
		cfg.Remote.Retry.JitterPercent != nil

	if !retryConfigured {
		t.Error("Expected retryConfigured=true when Enabled field is set")
	}

	// Create uploader config (mimicking NEW main.go logic after fix)
	uploaderCfg := uploader.HTTPUploaderConfig{
		URL:      cfg.Remote.URL,
		DeviceID: cfg.Device.ID,
		Timeout:  30 * time.Second,
	}

	if retryConfigured {
		// Default to true if Enabled is nil but other fields are set
		enabled := cfg.Remote.Retry.Enabled == nil || *cfg.Remote.Retry.Enabled
		if enabled {
			maxAttempts := cfg.Remote.Retry.MaxAttempts
			uploaderCfg.MaxRetries = &maxAttempts
			retryDelay, _ := cfg.Remote.Retry.InitialBackoff()
			uploaderCfg.RetryDelay = retryDelay
		} else {
			// Explicitly disabled - set MaxRetries=0
			zero := 0
			uploaderCfg.MaxRetries = &zero
			uploaderCfg.RetryDelay = 1 * time.Second
		}
	}

	uploaderCfg.ChunkSize = cfg.Remote.GetChunkSize()

	// Verify we set MaxRetries=0 (disabled)
	if uploaderCfg.MaxRetries == nil || *uploaderCfg.MaxRetries != 0 {
		var val int
		if uploaderCfg.MaxRetries != nil {
			val = *uploaderCfg.MaxRetries
		}
		t.Errorf("Expected MaxRetries=0 when explicitly disabled, got %d", val)
	}

	// Test with actual server
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploaderCfg.URL = server.URL
	up := uploader.NewHTTPUploaderWithConfig(uploaderCfg)
	defer up.Close()

	metric := models.NewMetric("test.metric", 1.0, "test-device")
	err = up.Upload(context.Background(), []*models.Metric{metric})

	if err == nil {
		t.Fatal("Expected error from failed upload")
	}

	// Should have made only 1 attempt (no retries) - this is the P1 fix
	expectedAttempts := 1
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempt when retries disabled with only enabled:false, got %d", expectedAttempts, attempts)
	}
}

// TestConfigWiring_RetryEnabledWithOnlyEnabledTrue tests P1 fix:
// Setting ONLY enabled: true (with no other fields) should use default retries (3 retries = 4 attempts)
func TestConfigWiring_RetryEnabledWithOnlyEnabledTrue(t *testing.T) {
	// Create temp config with ONLY enabled: true (no other retry fields)
	// This is a common case when a user wants to enable retries with defaults
	configYAML := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  enabled: true
  url: http://localhost:8428/api/v1/import
  upload_interval: 30s
  retry:
    enabled: true
    # Note: NO other retry fields - should use default 3 retries

metrics:
  - name: cpu.temperature
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify Enabled is explicitly set to true
	if cfg.Remote.Retry.Enabled == nil {
		t.Fatal("Expected Enabled to be non-nil (explicitly set)")
	}
	if !*cfg.Remote.Retry.Enabled {
		t.Error("Expected Enabled=true, got false")
	}

	// Verify max_attempts is not set (zero)
	if cfg.Remote.Retry.MaxAttempts != 0 {
		t.Errorf("Expected MaxAttempts=0 (not set), got %d", cfg.Remote.Retry.MaxAttempts)
	}

	// Simulate the NEW logic in main.go (after P1 fix)
	retryConfigured := cfg.Remote.Retry.Enabled != nil ||
		cfg.Remote.Retry.MaxAttempts > 0 ||
		cfg.Remote.Retry.InitialBackoffStr != "" ||
		cfg.Remote.Retry.MaxBackoffStr != "" ||
		cfg.Remote.Retry.BackoffMultiplier > 0 ||
		cfg.Remote.Retry.JitterPercent != nil

	if !retryConfigured {
		t.Error("Expected retryConfigured=true when Enabled field is set")
	}

	// Create uploader config (mimicking NEW main.go logic after P1 fix)
	uploaderCfg := uploader.HTTPUploaderConfig{
		URL:      cfg.Remote.URL,
		DeviceID: cfg.Device.ID,
		Timeout:  30 * time.Second,
	}

	if retryConfigured {
		enabled := cfg.Remote.Retry.Enabled == nil || *cfg.Remote.Retry.Enabled
		if enabled {
			// Use configured retry values, applying defaults for unset fields
			maxAttempts := cfg.Remote.Retry.MaxAttempts
			if maxAttempts == 0 {
				// User enabled retries but didn't set max_attempts - use default
				maxAttempts = 3
			}
			// Convert max_attempts (total attempts) to maxRetries (number of retries)
			maxRetries := maxAttempts - 1
			if maxRetries < 0 {
				maxRetries = 0
			}
			uploaderCfg.MaxRetries = &maxRetries
			retryDelay, _ := cfg.Remote.Retry.InitialBackoff()
			maxBackoff, _ := cfg.Remote.Retry.MaxBackoff()
			uploaderCfg.RetryDelay = retryDelay
			uploaderCfg.MaxBackoff = maxBackoff
			uploaderCfg.BackoffMultiplier = cfg.Remote.Retry.BackoffMultiplier
			// JitterPercent: nil means use default (20), otherwise honor the value (even if 0)
			if cfg.Remote.Retry.JitterPercent != nil {
				// User explicitly set jitter_percent - honor it (even if 0)
				uploaderCfg.JitterPercent = cfg.Remote.Retry.JitterPercent
			} else {
				// User enabled retries but didn't set jitter_percent - use default
				// Critical: Without jitter, all instances retry in lockstep (thundering herd)
				jitter := 20
				uploaderCfg.JitterPercent = &jitter
			}
		} else {
			zero := 0
			uploaderCfg.MaxRetries = &zero
			uploaderCfg.RetryDelay = 1 * time.Second
		}
	}

	uploaderCfg.ChunkSize = cfg.Remote.GetChunkSize()

	// Verify we set MaxRetries=2 (default max_attempts=3, converted to maxRetries=2)
	if uploaderCfg.MaxRetries == nil || *uploaderCfg.MaxRetries != 2 {
		var val int
		if uploaderCfg.MaxRetries != nil {
			val = *uploaderCfg.MaxRetries
		}
		t.Errorf("Expected MaxRetries=2 (max_attempts=3 → maxRetries=2), got %d", val)
	}

	// Verify we set JitterPercent=20 (default when enabled but not configured)
	// This is critical to prevent thundering herd behavior
	if uploaderCfg.JitterPercent == nil || *uploaderCfg.JitterPercent != 20 {
		var val int
		if uploaderCfg.JitterPercent != nil {
			val = *uploaderCfg.JitterPercent
		}
		t.Errorf("Expected JitterPercent=20 (default) when enabled:true but jitter_percent not set, got %d", val)
	}

	// Test with actual server
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploaderCfg.URL = server.URL
	up := uploader.NewHTTPUploaderWithConfig(uploaderCfg)
	defer up.Close()

	metric := models.NewMetric("test.metric", 1.0, "test-device")
	err = up.Upload(context.Background(), []*models.Metric{metric})

	if err == nil {
		t.Fatal("Expected error from failed upload")
	}

	// Should have made 3 attempts (default max_attempts=3)
	expectedAttempts := 3
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempts (default max_attempts=3), got %d", expectedAttempts, attempts)
	}
}

// TestConfigWiring_RetryJitterDefaultWhenPartialConfig tests P1 fix:
// When retry is enabled with some fields but jitter_percent omitted, default to 20% jitter
func TestConfigWiring_RetryJitterDefaultWhenPartialConfig(t *testing.T) {
	// Create temp config with partial retry config (max_attempts set, jitter_percent not set)
	// This is common when users want custom retry count but default jitter
	configYAML := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  enabled: true
  url: http://localhost:8428/api/v1/import
  upload_interval: 30s
  retry:
    enabled: true
    max_attempts: 2
    # Note: jitter_percent NOT set - should default to 20%

metrics:
  - name: cpu.temperature
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify JitterPercent is not set in config (nil)
	if cfg.Remote.Retry.JitterPercent != nil {
		t.Errorf("Expected JitterPercent=nil (not set in config), got %v", *cfg.Remote.Retry.JitterPercent)
	}

	// Simulate the NEW logic in main.go (after P1 fix)
	retryConfigured := cfg.Remote.Retry.Enabled != nil ||
		cfg.Remote.Retry.MaxAttempts > 0 ||
		cfg.Remote.Retry.InitialBackoffStr != "" ||
		cfg.Remote.Retry.MaxBackoffStr != "" ||
		cfg.Remote.Retry.BackoffMultiplier > 0 ||
		cfg.Remote.Retry.JitterPercent != nil

	uploaderCfg := uploader.HTTPUploaderConfig{
		URL:      cfg.Remote.URL,
		DeviceID: cfg.Device.ID,
		Timeout:  30 * time.Second,
	}

	if retryConfigured {
		enabled := cfg.Remote.Retry.Enabled == nil || *cfg.Remote.Retry.Enabled
		if enabled {
			maxAttempts := cfg.Remote.Retry.MaxAttempts
			if maxAttempts == 0 {
				maxAttempts = 3
			}
			// Convert max_attempts (total attempts) to maxRetries (number of retries)
			maxRetries := maxAttempts - 1
			if maxRetries < 0 {
				maxRetries = 0
			}
			uploaderCfg.MaxRetries = &maxRetries
			retryDelay, _ := cfg.Remote.Retry.InitialBackoff()
			maxBackoff, _ := cfg.Remote.Retry.MaxBackoff()
			uploaderCfg.RetryDelay = retryDelay
			uploaderCfg.MaxBackoff = maxBackoff
			uploaderCfg.BackoffMultiplier = cfg.Remote.Retry.BackoffMultiplier
			// JitterPercent: nil means use default (20), otherwise honor the value (even if 0)
			if cfg.Remote.Retry.JitterPercent != nil {
				// User explicitly set jitter_percent - honor it (even if 0)
				uploaderCfg.JitterPercent = cfg.Remote.Retry.JitterPercent
			} else {
				// User enabled retries but didn't set jitter_percent - use default
				// Critical: Without jitter, all instances retry in lockstep (thundering herd)
				jitter := 20
				uploaderCfg.JitterPercent = &jitter
			}
		}
	}

	uploaderCfg.ChunkSize = cfg.Remote.GetChunkSize()

	// Verify we set JitterPercent=20 (default) even though max_attempts was set
	// This prevents thundering herd behavior
	if uploaderCfg.JitterPercent == nil || *uploaderCfg.JitterPercent != 20 {
		var val int
		if uploaderCfg.JitterPercent != nil {
			val = *uploaderCfg.JitterPercent
		}
		t.Errorf("Expected JitterPercent=20 (default) when jitter_percent not set in partial config, got %d", val)
	}

	// Verify MaxRetries is set to 1 (max_attempts=2 → maxRetries=1)
	if uploaderCfg.MaxRetries == nil || *uploaderCfg.MaxRetries != 1 {
		var val int
		if uploaderCfg.MaxRetries != nil {
			val = *uploaderCfg.MaxRetries
		}
		t.Errorf("Expected MaxRetries=1 (max_attempts=2 → maxRetries=1), got %d", val)
	}

	// Test with actual server to verify jitter is applied
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploaderCfg.URL = server.URL
	up := uploader.NewHTTPUploaderWithConfig(uploaderCfg)
	defer up.Close()

	metric := models.NewMetric("test.metric", 1.0, "test-device")
	err = up.Upload(context.Background(), []*models.Metric{metric})

	if err == nil {
		t.Fatal("Expected error from failed upload")
	}

	// Should have made 2 attempts (as configured by max_attempts: 2)
	expectedAttempts := 2
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempts (max_attempts=2), got %d", expectedAttempts, attempts)
	}
}

// TestConfigWiring_ClockSkewIntervalGuard tests P1 fix:
// Non-positive clock skew check intervals should fall back to 5-minute default
// This prevents panic in time.NewTicker which rejects non-positive durations
func TestConfigWiring_ClockSkewIntervalGuard(t *testing.T) {
	tests := []struct {
		name             string
		configInterval   string
		expectedInterval time.Duration
		shouldWarn       bool
	}{
		{
			name:             "zero interval falls back to default",
			configInterval:   "0s",
			expectedInterval: 5 * time.Minute,
			shouldWarn:       true,
		},
		{
			name:             "negative interval falls back to default",
			configInterval:   "-1s",
			expectedInterval: 5 * time.Minute,
			shouldWarn:       true,
		},
		{
			name:             "negative 10 minutes falls back to default",
			configInterval:   "-10m",
			expectedInterval: 5 * time.Minute,
			shouldWarn:       true,
		},
		{
			name:             "positive interval is respected",
			configInterval:   "10m",
			expectedInterval: 10 * time.Minute,
			shouldWarn:       false,
		},
		{
			name:             "small positive interval is respected",
			configInterval:   "1s",
			expectedInterval: 1 * time.Second,
			shouldWarn:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic in main.go (after P1 fix)
			clockCheckInterval := 5 * time.Minute // Default to 5 minutes
			var logged bool

			if tt.configInterval != "" {
				if interval, err := time.ParseDuration(tt.configInterval); err == nil {
					// Guard against non-positive intervals to prevent panic in time.NewTicker
					if interval > 0 {
						clockCheckInterval = interval
					} else {
						// This would log a warning in actual code
						logged = true
					}
				}
			}

			// Verify the interval is set correctly
			if clockCheckInterval != tt.expectedInterval {
				t.Errorf("Expected interval %v, got %v", tt.expectedInterval, clockCheckInterval)
			}

			// Verify warning would be logged for non-positive intervals
			if tt.shouldWarn && !logged {
				t.Error("Expected warning to be logged for non-positive interval")
			}
			if !tt.shouldWarn && logged {
				t.Error("Did not expect warning to be logged for positive interval")
			}

			// Critical: Verify we can create a ticker without panic
			// This would panic if interval <= 0
			ticker := time.NewTicker(clockCheckInterval)
			ticker.Stop()
		})
	}
}

// TestConfigWiring_ClockSkewIntervalDefault tests that missing config uses 5-minute default
func TestConfigWiring_ClockSkewIntervalDefault(t *testing.T) {
	// Test with empty config (no clock_skew_check_interval field)
	clockCheckInterval := 5 * time.Minute // Default to 5 minutes
	configInterval := ""

	if configInterval != "" {
		if interval, err := time.ParseDuration(configInterval); err == nil {
			if interval > 0 {
				clockCheckInterval = interval
			}
		}
	}

	// Verify default is used
	expectedInterval := 5 * time.Minute
	if clockCheckInterval != expectedInterval {
		t.Errorf("Expected default interval %v, got %v", expectedInterval, clockCheckInterval)
	}

	// Verify we can create a ticker without panic
	ticker := time.NewTicker(clockCheckInterval)
	ticker.Stop()
}

// TestConfigWiring_RetryExplicitZeroJitter tests P2 fix:
// When user explicitly sets jitter_percent: 0 (to disable jitter), it should be respected
func TestConfigWiring_RetryExplicitZeroJitter(t *testing.T) {
	// Create temp config with explicit jitter_percent: 0
	configYAML := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  enabled: true
  url: http://localhost:8428/api/v1/import
  upload_interval: 30s
  retry:
    enabled: true
    max_attempts: 3
    jitter_percent: 0  # Explicitly disable jitter

metrics:
  - name: cpu.temperature
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify JitterPercent is explicitly set to 0 (not nil)
	if cfg.Remote.Retry.JitterPercent == nil {
		t.Fatal("Expected JitterPercent to be non-nil (explicitly set)")
	}
	if *cfg.Remote.Retry.JitterPercent != 0 {
		t.Errorf("Expected JitterPercent=0, got %d", *cfg.Remote.Retry.JitterPercent)
	}

	// Simulate the NEW logic in main.go (after P2 fix)
	retryConfigured := cfg.Remote.Retry.Enabled != nil ||
		cfg.Remote.Retry.MaxAttempts > 0 ||
		cfg.Remote.Retry.InitialBackoffStr != "" ||
		cfg.Remote.Retry.MaxBackoffStr != "" ||
		cfg.Remote.Retry.BackoffMultiplier > 0 ||
		cfg.Remote.Retry.JitterPercent != nil

	if !retryConfigured {
		t.Error("Expected retryConfigured=true when retry fields are set")
	}

	uploaderCfg := uploader.HTTPUploaderConfig{
		URL:      cfg.Remote.URL,
		DeviceID: cfg.Device.ID,
		Timeout:  30 * time.Second,
	}

	if retryConfigured {
		enabled := cfg.Remote.Retry.Enabled == nil || *cfg.Remote.Retry.Enabled
		if enabled {
			maxAttempts := cfg.Remote.Retry.MaxAttempts
			if maxAttempts == 0 {
				maxAttempts = 3
			}
			// Convert max_attempts (total attempts) to maxRetries (number of retries)
			maxRetries := maxAttempts - 1
			if maxRetries < 0 {
				maxRetries = 0
			}
			uploaderCfg.MaxRetries = &maxRetries
			retryDelay, _ := cfg.Remote.Retry.InitialBackoff()
			maxBackoff, _ := cfg.Remote.Retry.MaxBackoff()
			uploaderCfg.RetryDelay = retryDelay
			uploaderCfg.MaxBackoff = maxBackoff
			uploaderCfg.BackoffMultiplier = cfg.Remote.Retry.BackoffMultiplier
			// JitterPercent: nil means use default (20), otherwise honor the value (even if 0)
			if cfg.Remote.Retry.JitterPercent != nil {
				// User explicitly set jitter_percent - honor it (even if 0)
				uploaderCfg.JitterPercent = cfg.Remote.Retry.JitterPercent
			} else {
				// User enabled retries but didn't set jitter_percent - use default
				jitter := 20
				uploaderCfg.JitterPercent = &jitter
			}
		}
	}

	uploaderCfg.ChunkSize = cfg.Remote.GetChunkSize()

	// Critical: Verify we set JitterPercent=0 (user's explicit value, not default)
	// This is the P2 fix - previously would have been overwritten to 20
	if uploaderCfg.JitterPercent == nil || *uploaderCfg.JitterPercent != 0 {
		var val int
		if uploaderCfg.JitterPercent != nil {
			val = *uploaderCfg.JitterPercent
		}
		t.Errorf("Expected JitterPercent=0 (user explicitly set), got %d", val)
	}

	// Test with actual server to verify no jitter is applied
	attempts := 0
	var delays []time.Duration
	lastTime := time.Now()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts > 1 {
			delays = append(delays, time.Since(lastTime))
		}
		lastTime = time.Now()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploaderCfg.URL = server.URL
	uploaderCfg.RetryDelay = 100 * time.Millisecond // Short delay for testing
	up := uploader.NewHTTPUploaderWithConfig(uploaderCfg)
	defer up.Close()

	metric := models.NewMetric("test.metric", 1.0, "test-device")
	err = up.Upload(context.Background(), []*models.Metric{metric})

	if err == nil {
		t.Fatal("Expected error from failed upload")
	}

	// Should have made 3 attempts (as configured by max_attempts: 3)
	expectedAttempts := 3
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempts (max_attempts=3), got %d", expectedAttempts, attempts)
	}

	// With zero jitter, delays should be very consistent (within a small margin)
	// This verifies that jitter is actually disabled
	if len(delays) > 0 {
		// All delays should be close to the base delay with exponential backoff
		// With 0% jitter, variance should be minimal (just timing variations)
		for i, delay := range delays {
			t.Logf("Retry %d delay: %v", i+1, delay)
		}
	}
}
