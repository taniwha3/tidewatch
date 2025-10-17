package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/taniwha3/tidewatch/internal/models"
)

// MetricsCollector manages meta-monitoring metrics about the collector itself
type MetricsCollector struct {
	mu       sync.RWMutex
	deviceID string

	// Collector metrics
	collectorMetricsCollected map[string]int64     // collector name -> count
	collectorMetricsFailed    map[string]int64     // collector name -> count
	collectorDurations        map[string][]float64 // collector name -> recent durations (for histogram)

	// Uploader metrics
	uploaderMetricsUploaded int64
	uploaderFailuresTotal   int64
	uploaderDurations       []float64 // recent durations (for histogram)

	// Storage metrics
	storageDatabaseSizeBytes int64
	storageWALSizeBytes      int64
	storagePendingUpload     int64

	// Time metrics
	timeSkewMs int64

	// Configuration
	histogramMaxSamples int // Maximum number of duration samples to keep
}

// NewMetricsCollector creates a new meta-metrics collector
func NewMetricsCollector(deviceID string) *MetricsCollector {
	return &MetricsCollector{
		deviceID:                  deviceID,
		collectorMetricsCollected: make(map[string]int64),
		collectorMetricsFailed:    make(map[string]int64),
		collectorDurations:        make(map[string][]float64),
		uploaderDurations:         make([]float64, 0, 100),
		histogramMaxSamples:       100, // Keep last 100 samples for histogram calculation
	}
}

// RecordCollectionSuccess records a successful collection
func (m *MetricsCollector) RecordCollectionSuccess(collectorName string, count int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectorMetricsCollected[collectorName] += int64(count)

	// Record duration for histogram
	durationSeconds := duration.Seconds()
	durations := m.collectorDurations[collectorName]
	durations = append(durations, durationSeconds)

	// Keep only recent samples
	if len(durations) > m.histogramMaxSamples {
		durations = durations[len(durations)-m.histogramMaxSamples:]
	}
	m.collectorDurations[collectorName] = durations
}

// RecordCollectionFailure records a failed collection
func (m *MetricsCollector) RecordCollectionFailure(collectorName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectorMetricsFailed[collectorName]++
}

// RecordUploadSuccess records a successful upload
func (m *MetricsCollector) RecordUploadSuccess(count int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.uploaderMetricsUploaded += int64(count)

	// Record duration for histogram
	durationSeconds := duration.Seconds()
	m.uploaderDurations = append(m.uploaderDurations, durationSeconds)

	// Keep only recent samples
	if len(m.uploaderDurations) > m.histogramMaxSamples {
		m.uploaderDurations = m.uploaderDurations[len(m.uploaderDurations)-m.histogramMaxSamples:]
	}
}

// RecordUploadFailure records a failed upload
func (m *MetricsCollector) RecordUploadFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.uploaderFailuresTotal++
}

// UpdateStorageMetrics updates storage-related metrics
func (m *MetricsCollector) UpdateStorageMetrics(dbSize, walSize, pendingCount int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.storageDatabaseSizeBytes = dbSize
	m.storageWALSizeBytes = walSize
	m.storagePendingUpload = pendingCount
}

// UpdateTimeSkew updates the clock skew metric
func (m *MetricsCollector) UpdateTimeSkew(skewMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.timeSkewMs = skewMs
}

// CollectMetrics generates meta-metrics as models.Metric instances
func (m *MetricsCollector) CollectMetrics(ctx context.Context) ([]*models.Metric, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	metrics := make([]*models.Metric, 0, 50)

	// Collector metrics
	for collectorName, count := range m.collectorMetricsCollected {
		metrics = append(metrics, &models.Metric{
			Name:        "collector.metrics_collected_total",
			TimestampMs: now.UnixMilli(),
			Value:       float64(count),
			ValueType:   models.ValueTypeNumeric,
			DeviceID:    m.deviceID,
			Tags: map[string]string{
				"collector": collectorName,
			},
		})
	}

	for collectorName, count := range m.collectorMetricsFailed {
		metrics = append(metrics, &models.Metric{
			Name:        "collector.metrics_failed_total",
			TimestampMs: now.UnixMilli(),
			Value:       float64(count),
			ValueType:   models.ValueTypeNumeric,
			DeviceID:    m.deviceID,
			Tags: map[string]string{
				"collector": collectorName,
			},
		})
	}

	// Collector duration histograms (p50, p95, p99)
	for collectorName, durations := range m.collectorDurations {
		if len(durations) > 0 {
			p50, p95, p99 := calculatePercentiles(durations)

			metrics = append(metrics,
				&models.Metric{
					Name:        "collector.collection_duration_seconds_p50",
					TimestampMs: now.UnixMilli(),
					Value:       p50,
					ValueType:   models.ValueTypeNumeric,
					DeviceID:    m.deviceID,
					Tags: map[string]string{
						"collector": collectorName,
					},
				},
				&models.Metric{
					Name:        "collector.collection_duration_seconds_p95",
					TimestampMs: now.UnixMilli(),
					Value:       p95,
					ValueType:   models.ValueTypeNumeric,
					DeviceID:    m.deviceID,
					Tags: map[string]string{
						"collector": collectorName,
					},
				},
				&models.Metric{
					Name:        "collector.collection_duration_seconds_p99",
					TimestampMs: now.UnixMilli(),
					Value:       p99,
					ValueType:   models.ValueTypeNumeric,
					DeviceID:    m.deviceID,
					Tags: map[string]string{
						"collector": collectorName,
					},
				},
			)
		}
	}

	// Uploader metrics
	metrics = append(metrics,
		&models.Metric{
			Name:        "uploader.metrics_uploaded_total",
			TimestampMs: now.UnixMilli(),
			Value:       float64(m.uploaderMetricsUploaded),
			ValueType:   models.ValueTypeNumeric,
			DeviceID:    m.deviceID,
			Tags:        make(map[string]string),
		},
		&models.Metric{
			Name:        "uploader.upload_failures_total",
			TimestampMs: now.UnixMilli(),
			Value:       float64(m.uploaderFailuresTotal),
			ValueType:   models.ValueTypeNumeric,
			DeviceID:    m.deviceID,
			Tags:        make(map[string]string),
		},
	)

	// Uploader duration histogram
	if len(m.uploaderDurations) > 0 {
		p50, p95, p99 := calculatePercentiles(m.uploaderDurations)

		metrics = append(metrics,
			&models.Metric{
				Name:        "uploader.upload_duration_seconds_p50",
				TimestampMs: now.UnixMilli(),
				Value:       p50,
				ValueType:   models.ValueTypeNumeric,
				DeviceID:    m.deviceID,
				Tags:        make(map[string]string),
			},
			&models.Metric{
				Name:        "uploader.upload_duration_seconds_p95",
				TimestampMs: now.UnixMilli(),
				Value:       p95,
				ValueType:   models.ValueTypeNumeric,
				DeviceID:    m.deviceID,
				Tags:        make(map[string]string),
			},
			&models.Metric{
				Name:        "uploader.upload_duration_seconds_p99",
				TimestampMs: now.UnixMilli(),
				Value:       p99,
				ValueType:   models.ValueTypeNumeric,
				DeviceID:    m.deviceID,
				Tags:        make(map[string]string),
			},
		)
	}

	// Storage metrics
	metrics = append(metrics,
		&models.Metric{
			Name:        "storage.database_size_bytes",
			TimestampMs: now.UnixMilli(),
			Value:       float64(m.storageDatabaseSizeBytes),
			ValueType:   models.ValueTypeNumeric,
			DeviceID:    m.deviceID,
			Tags:        make(map[string]string),
		},
		&models.Metric{
			Name:        "storage.wal_size_bytes",
			TimestampMs: now.UnixMilli(),
			Value:       float64(m.storageWALSizeBytes),
			ValueType:   models.ValueTypeNumeric,
			DeviceID:    m.deviceID,
			Tags:        make(map[string]string),
		},
		&models.Metric{
			Name:        "storage.metrics_pending_upload",
			TimestampMs: now.UnixMilli(),
			Value:       float64(m.storagePendingUpload),
			ValueType:   models.ValueTypeNumeric,
			DeviceID:    m.deviceID,
			Tags:        make(map[string]string),
		},
	)

	// Time metrics
	metrics = append(metrics, &models.Metric{
		Name:        "time.skew_ms",
		TimestampMs: now.UnixMilli(),
		Value:       float64(m.timeSkewMs),
		ValueType:   models.ValueTypeNumeric,
		DeviceID:    m.deviceID,
		Tags:        make(map[string]string),
	})

	return metrics, nil
}

// calculatePercentiles calculates p50, p95, p99 from a slice of values
// Uses nearest-rank method for percentile calculation
func calculatePercentiles(values []float64) (p50, p95, p99 float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}

	// Make a copy and sort
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple bubble sort for small arrays (max 100 samples)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate percentile indices using nearest-rank method
	// For percentile P and N values: index = ceil(P * N) - 1
	// We use int(P * N - 0.5) which is equivalent for our use case
	n := len(sorted)

	idx50 := int(float64(n)*0.50 - 0.5)
	idx95 := int(float64(n)*0.95 - 0.5)
	idx99 := int(float64(n)*0.99 - 0.5)

	// Clamp to valid indices [0, n-1]
	if idx50 < 0 {
		idx50 = 0
	}
	if idx50 >= n {
		idx50 = n - 1
	}

	if idx95 < 0 {
		idx95 = 0
	}
	if idx95 >= n {
		idx95 = n - 1
	}

	if idx99 < 0 {
		idx99 = 0
	}
	if idx99 >= n {
		idx99 = n - 1
	}

	return sorted[idx50], sorted[idx95], sorted[idx99]
}

// GetCollectorStats returns current collector statistics for debugging
func (m *MetricsCollector) GetCollectorStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	collectedCopy := make(map[string]int64)
	for k, v := range m.collectorMetricsCollected {
		collectedCopy[k] = v
	}

	failedCopy := make(map[string]int64)
	for k, v := range m.collectorMetricsFailed {
		failedCopy[k] = v
	}

	return map[string]interface{}{
		"collected":       collectedCopy,
		"failed":          failedCopy,
		"uploaded":        m.uploaderMetricsUploaded,
		"upload_failures": m.uploaderFailuresTotal,
		"db_size_bytes":   m.storageDatabaseSizeBytes,
		"wal_size_bytes":  m.storageWALSizeBytes,
		"pending_upload":  m.storagePendingUpload,
		"time_skew_ms":    m.timeSkewMs,
	}
}
