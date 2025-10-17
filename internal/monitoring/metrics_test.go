package monitoring

import (
	"context"
	"testing"
	"time"
)

func TestNewMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	if mc.deviceID != "test-device" {
		t.Errorf("Expected deviceID 'test-device', got '%s'", mc.deviceID)
	}

	if mc.collectorMetricsCollected == nil {
		t.Error("collectorMetricsCollected should be initialized")
	}

	if mc.collectorMetricsFailed == nil {
		t.Error("collectorMetricsFailed should be initialized")
	}

	if mc.histogramMaxSamples != 100 {
		t.Errorf("Expected histogramMaxSamples 100, got %d", mc.histogramMaxSamples)
	}
}

func TestRecordCollectionSuccess(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	// Record first success
	mc.RecordCollectionSuccess("cpu", 5, 100*time.Millisecond)

	if mc.collectorMetricsCollected["cpu"] != 5 {
		t.Errorf("Expected 5 collected metrics, got %d", mc.collectorMetricsCollected["cpu"])
	}

	if len(mc.collectorDurations["cpu"]) != 1 {
		t.Errorf("Expected 1 duration sample, got %d", len(mc.collectorDurations["cpu"]))
	}

	if mc.collectorDurations["cpu"][0] != 0.1 {
		t.Errorf("Expected duration 0.1s, got %f", mc.collectorDurations["cpu"][0])
	}

	// Record second success
	mc.RecordCollectionSuccess("cpu", 3, 150*time.Millisecond)

	if mc.collectorMetricsCollected["cpu"] != 8 {
		t.Errorf("Expected 8 collected metrics, got %d", mc.collectorMetricsCollected["cpu"])
	}

	if len(mc.collectorDurations["cpu"]) != 2 {
		t.Errorf("Expected 2 duration samples, got %d", len(mc.collectorDurations["cpu"]))
	}
}

func TestRecordCollectionFailure(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	// Record failures
	mc.RecordCollectionFailure("memory")
	mc.RecordCollectionFailure("memory")
	mc.RecordCollectionFailure("disk")

	if mc.collectorMetricsFailed["memory"] != 2 {
		t.Errorf("Expected 2 failed collections for memory, got %d", mc.collectorMetricsFailed["memory"])
	}

	if mc.collectorMetricsFailed["disk"] != 1 {
		t.Errorf("Expected 1 failed collection for disk, got %d", mc.collectorMetricsFailed["disk"])
	}
}

func TestRecordUploadSuccess(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	// Record uploads
	mc.RecordUploadSuccess(100, 500*time.Millisecond)
	mc.RecordUploadSuccess(50, 300*time.Millisecond)

	if mc.uploaderMetricsUploaded != 150 {
		t.Errorf("Expected 150 uploaded metrics, got %d", mc.uploaderMetricsUploaded)
	}

	if len(mc.uploaderDurations) != 2 {
		t.Errorf("Expected 2 duration samples, got %d", len(mc.uploaderDurations))
	}

	if mc.uploaderDurations[0] != 0.5 {
		t.Errorf("Expected first duration 0.5s, got %f", mc.uploaderDurations[0])
	}

	if mc.uploaderDurations[1] != 0.3 {
		t.Errorf("Expected second duration 0.3s, got %f", mc.uploaderDurations[1])
	}
}

func TestRecordUploadFailure(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	mc.RecordUploadFailure()
	mc.RecordUploadFailure()
	mc.RecordUploadFailure()

	if mc.uploaderFailuresTotal != 3 {
		t.Errorf("Expected 3 upload failures, got %d", mc.uploaderFailuresTotal)
	}
}

func TestUpdateStorageMetrics(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	mc.UpdateStorageMetrics(1024*1024, 64*1024, 500)

	if mc.storageDatabaseSizeBytes != 1024*1024 {
		t.Errorf("Expected DB size 1048576, got %d", mc.storageDatabaseSizeBytes)
	}

	if mc.storageWALSizeBytes != 64*1024 {
		t.Errorf("Expected WAL size 65536, got %d", mc.storageWALSizeBytes)
	}

	if mc.storagePendingUpload != 500 {
		t.Errorf("Expected 500 pending, got %d", mc.storagePendingUpload)
	}
}

func TestUpdateTimeSkew(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	mc.UpdateTimeSkew(1500)

	if mc.timeSkewMs != 1500 {
		t.Errorf("Expected time skew 1500ms, got %d", mc.timeSkewMs)
	}

	mc.UpdateTimeSkew(-2000)

	if mc.timeSkewMs != -2000 {
		t.Errorf("Expected time skew -2000ms, got %d", mc.timeSkewMs)
	}
}

func TestCollectMetrics(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	// Record some activity
	mc.RecordCollectionSuccess("cpu", 10, 100*time.Millisecond)
	mc.RecordCollectionSuccess("cpu", 5, 150*time.Millisecond)
	mc.RecordCollectionFailure("memory")
	mc.RecordUploadSuccess(100, 500*time.Millisecond)
	mc.RecordUploadFailure()
	mc.UpdateStorageMetrics(1024*1024, 64*1024, 200)
	mc.UpdateTimeSkew(1200)

	// Collect metrics
	ctx := context.Background()
	metrics, err := mc.CollectMetrics(ctx)

	if err != nil {
		t.Fatalf("CollectMetrics failed: %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected metrics, got none")
	}

	// Verify device ID is set
	for _, m := range metrics {
		if m.DeviceID != "test-device" {
			t.Errorf("Expected deviceID 'test-device', got '%s'", m.DeviceID)
		}
	}

	// Check for expected metric names
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.Name] = true
	}

	expectedMetrics := []string{
		"collector.metrics_collected_total",
		"collector.metrics_failed_total",
		"collector.collection_duration_seconds_p50",
		"collector.collection_duration_seconds_p95",
		"collector.collection_duration_seconds_p99",
		"uploader.metrics_uploaded_total",
		"uploader.upload_failures_total",
		"uploader.upload_duration_seconds_p50",
		"uploader.upload_duration_seconds_p95",
		"uploader.upload_duration_seconds_p99",
		"storage.database_size_bytes",
		"storage.wal_size_bytes",
		"storage.metrics_pending_upload",
		"time.skew_ms",
	}

	for _, expected := range expectedMetrics {
		if !metricNames[expected] {
			t.Errorf("Missing expected metric: %s", expected)
		}
	}

	// Verify some specific values
	for _, m := range metrics {
		switch m.Name {
		case "collector.metrics_collected_total":
			if m.Tags["collector"] == "cpu" && m.Value != 15 {
				t.Errorf("Expected CPU collected 15, got %f", m.Value)
			}
		case "collector.metrics_failed_total":
			if m.Tags["collector"] == "memory" && m.Value != 1 {
				t.Errorf("Expected memory failed 1, got %f", m.Value)
			}
		case "uploader.metrics_uploaded_total":
			if m.Value != 100 {
				t.Errorf("Expected uploaded 100, got %f", m.Value)
			}
		case "uploader.upload_failures_total":
			if m.Value != 1 {
				t.Errorf("Expected upload failures 1, got %f", m.Value)
			}
		case "storage.database_size_bytes":
			if m.Value != 1024*1024 {
				t.Errorf("Expected DB size 1048576, got %f", m.Value)
			}
		case "storage.wal_size_bytes":
			if m.Value != 64*1024 {
				t.Errorf("Expected WAL size 65536, got %f", m.Value)
			}
		case "storage.metrics_pending_upload":
			if m.Value != 200 {
				t.Errorf("Expected pending 200, got %f", m.Value)
			}
		case "time.skew_ms":
			if m.Value != 1200 {
				t.Errorf("Expected skew 1200ms, got %f", m.Value)
			}
		}
	}
}

func TestCalculatePercentiles(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		p50    float64
		p95    float64
		p99    float64
	}{
		{
			name:   "empty",
			values: []float64{},
			p50:    0,
			p95:    0,
			p99:    0,
		},
		{
			name:   "single value",
			values: []float64{1.0},
			p50:    1.0,
			p95:    1.0,
			p99:    1.0,
		},
		{
			name:   "two values",
			values: []float64{1.0, 2.0},
			p50:    1.0,
			p95:    2.0,
			p99:    2.0,
		},
		{
			name:   "sorted values",
			values: []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0},
			p50:    5.0,
			p95:    10.0,
			p99:    10.0,
		},
		{
			name:   "unsorted values",
			values: []float64{10.0, 1.0, 5.0, 3.0, 8.0, 2.0, 7.0, 4.0, 9.0, 6.0},
			p50:    5.0,
			p95:    10.0,
			p99:    10.0,
		},
		{
			name: "100 values (0-99)",
			values: func() []float64 {
				vals := make([]float64, 100)
				for i := 0; i < 100; i++ {
					vals[i] = float64(i)
				}
				return vals
			}(),
			p50: 49.0, // Nearest-rank: int(100*0.50 - 0.5) = 49
			p95: 94.0, // Nearest-rank: int(100*0.95 - 0.5) = 94
			p99: 98.0, // Nearest-rank: int(100*0.99 - 0.5) = 98
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p50, p95, p99 := calculatePercentiles(tt.values)

			if p50 != tt.p50 {
				t.Errorf("p50: expected %f, got %f", tt.p50, p50)
			}
			if p95 != tt.p95 {
				t.Errorf("p95: expected %f, got %f", tt.p95, p95)
			}
			if p99 != tt.p99 {
				t.Errorf("p99: expected %f, got %f", tt.p99, p99)
			}
		})
	}
}

func TestHistogramSampleLimit(t *testing.T) {
	mc := NewMetricsCollector("test-device")
	mc.histogramMaxSamples = 10 // Set low limit for testing

	// Record more samples than the limit
	for i := 0; i < 20; i++ {
		mc.RecordCollectionSuccess("cpu", 1, time.Duration(i+1)*time.Millisecond)
	}

	// Should only keep last 10 samples
	if len(mc.collectorDurations["cpu"]) != 10 {
		t.Errorf("Expected 10 duration samples (trimmed), got %d", len(mc.collectorDurations["cpu"]))
	}

	// Verify they're the most recent samples (11-20ms)
	// The oldest should be 11ms (0.011s)
	oldest := mc.collectorDurations["cpu"][0]
	if oldest < 0.010 || oldest > 0.012 {
		t.Errorf("Expected oldest sample around 0.011s, got %f", oldest)
	}

	// Test uploader duration limit
	for i := 0; i < 20; i++ {
		mc.RecordUploadSuccess(1, time.Duration(i+1)*time.Millisecond)
	}

	if len(mc.uploaderDurations) != 10 {
		t.Errorf("Expected 10 uploader duration samples (trimmed), got %d", len(mc.uploaderDurations))
	}
}

func TestGetCollectorStats(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	mc.RecordCollectionSuccess("cpu", 10, 100*time.Millisecond)
	mc.RecordCollectionFailure("memory")
	mc.RecordUploadSuccess(50, 200*time.Millisecond)
	mc.RecordUploadFailure()
	mc.UpdateStorageMetrics(1024, 512, 100)
	mc.UpdateTimeSkew(500)

	stats := mc.GetCollectorStats()

	collected := stats["collected"].(map[string]int64)
	if collected["cpu"] != 10 {
		t.Errorf("Expected CPU collected 10, got %d", collected["cpu"])
	}

	failed := stats["failed"].(map[string]int64)
	if failed["memory"] != 1 {
		t.Errorf("Expected memory failed 1, got %d", failed["memory"])
	}

	if stats["uploaded"].(int64) != 50 {
		t.Errorf("Expected uploaded 50, got %d", stats["uploaded"].(int64))
	}

	if stats["upload_failures"].(int64) != 1 {
		t.Errorf("Expected upload failures 1, got %d", stats["upload_failures"].(int64))
	}

	if stats["db_size_bytes"].(int64) != 1024 {
		t.Errorf("Expected DB size 1024, got %d", stats["db_size_bytes"].(int64))
	}

	if stats["wal_size_bytes"].(int64) != 512 {
		t.Errorf("Expected WAL size 512, got %d", stats["wal_size_bytes"].(int64))
	}

	if stats["pending_upload"].(int64) != 100 {
		t.Errorf("Expected pending 100, got %d", stats["pending_upload"].(int64))
	}

	if stats["time_skew_ms"].(int64) != 500 {
		t.Errorf("Expected skew 500ms, got %d", stats["time_skew_ms"].(int64))
	}
}

func TestConcurrentAccess(t *testing.T) {
	mc := NewMetricsCollector("test-device")

	// Simulate concurrent access
	done := make(chan bool)

	// Goroutine 1: Record collections
	go func() {
		for i := 0; i < 100; i++ {
			mc.RecordCollectionSuccess("cpu", 1, time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: Record uploads
	go func() {
		for i := 0; i < 100; i++ {
			mc.RecordUploadSuccess(1, time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Read metrics
	go func() {
		for i := 0; i < 10; i++ {
			_, err := mc.CollectMetrics(context.Background())
			if err != nil {
				t.Errorf("CollectMetrics failed during concurrent access: %v", err)
			}
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// Verify final counts
	if mc.collectorMetricsCollected["cpu"] != 100 {
		t.Errorf("Expected 100 CPU metrics collected, got %d", mc.collectorMetricsCollected["cpu"])
	}

	if mc.uploaderMetricsUploaded != 100 {
		t.Errorf("Expected 100 metrics uploaded, got %d", mc.uploaderMetricsUploaded)
	}
}
