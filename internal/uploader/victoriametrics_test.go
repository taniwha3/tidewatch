package uploader

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taniwha3/tidewatch/internal/models"
)

// TestBuildVMJSONL_SingleMetric verifies basic JSONL structure for a single metric
func TestBuildVMJSONL_SingleMetric(t *testing.T) {
	now := time.Now()
	metric := models.NewMetric("cpu.temperature", 45.5, "device-001").
		WithTimestamp(now)

	result, _, err := BuildVMJSONL([]*models.Metric{metric})
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	// Should produce one line ending with newline
	lines := bytes.Split(result, []byte("\n"))
	if len(lines) != 2 { // Last split is empty after final \n
		t.Fatalf("Expected 2 splits (1 line + empty), got %d", len(lines))
	}

	// Parse the JSON line
	var vmMetric VMMetric
	if err := json.Unmarshal(lines[0], &vmMetric); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify structure
	if vmMetric.Metric["__name__"] != "cpu_temperature_celsius" {
		t.Errorf("Expected sanitized name 'cpu_temperature_celsius', got '%s'", vmMetric.Metric["__name__"])
	}
	if vmMetric.Metric["device_id"] != "device-001" {
		t.Errorf("Expected device_id 'device-001', got '%s'", vmMetric.Metric["device_id"])
	}
	if len(vmMetric.Values) != 1 || vmMetric.Values[0] != 45.5 {
		t.Errorf("Expected values [45.5], got %v", vmMetric.Values)
	}
	if len(vmMetric.Timestamps) != 1 || vmMetric.Timestamps[0] != now.UnixMilli() {
		t.Errorf("Expected timestamps [%d], got %v", now.UnixMilli(), vmMetric.Timestamps)
	}
}

// TestBuildVMJSONL_MultipleMetrics verifies each metric gets its own line
func TestBuildVMJSONL_MultipleMetrics(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
		models.NewMetric("memory.bytes.used", 1024.0, "device-001").WithTimestamp(now.Add(time.Second)),
		models.NewMetric("network.tx.total", 5000.0, "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}

	result, _, err := BuildVMJSONL(metrics)
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	lines := bytes.Split(result, []byte("\n"))
	if len(lines) != 4 { // 3 metrics + empty after final \n
		t.Fatalf("Expected 4 splits (3 lines + empty), got %d", len(lines))
	}

	// Verify each line is valid JSON
	expectedNames := []string{"cpu_temperature_celsius", "memory_bytes_used_bytes", "network_tx_total"}
	for i, expected := range expectedNames {
		var vmMetric VMMetric
		if err := json.Unmarshal(lines[i], &vmMetric); err != nil {
			t.Fatalf("Failed to parse line %d: %v", i, err)
		}
		if vmMetric.Metric["__name__"] != expected {
			t.Errorf("Line %d: expected name '%s', got '%s'", i, expected, vmMetric.Metric["__name__"])
		}
	}
}

// TestSanitizeMetricName_DotReplacement verifies dots become underscores
func TestSanitizeMetricName_DotReplacement(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"cpu.temperature", "cpu_temperature_celsius"},
		{"network.tx.bytes", "network_tx_bytes_total"}, // tx is a counter keyword
		{"disk.read.total", "disk_read_total"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		result := sanitizeMetricName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeMetricName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestSanitizeMetricName_TemperatureSuffix verifies _celsius is added
func TestSanitizeMetricName_TemperatureSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"cpu.temperature", "cpu_temperature_celsius"},
		{"thermal.zone.temperature", "thermal_zone_temperature_celsius"},
		{"temperature", "temperature_celsius"},
		// Regression test: "temp" should also trigger _celsius suffix when it's a separate word
		{"thermal.zone_temp", "thermal_zone_temp_celsius"},
		{"cpu.temp", "cpu_temp_celsius"},
		{"ambient.temp", "ambient_temp_celsius"},
		{"temp", "temp_celsius"},
		{"temp.sensor", "temp_sensor_celsius"},
		// Should not double-add
		{"cpu_temperature_celsius", "cpu_temperature_celsius"},
		{"thermal_zone_temp_celsius", "thermal_zone_temp_celsius"},
		// Regression test: words containing "temp" should NOT trigger _celsius
		{"login.attempts", "login_attempts"},
		{"http.attempt", "http_attempt"},
		{"service.login.attempts", "service_login_attempts"},
		{"contempt.score", "contempt_score"},
		{"temptation.level", "temptation_level"},
		{"contemporary.metric", "contemporary_metric"},
		// Regression test: "template" and "tempest" should NOT trigger _celsius
		{"render.template_duration", "render_template_duration"},
		{"http.template_latency", "http_template_latency"},
		{"template.render.time", "template_render_time"},
		{"weather.tempest.severity", "weather_tempest_severity"},
		{"tempest.wind.speed", "tempest_wind_speed"},
	}

	for _, tt := range tests {
		result := sanitizeMetricName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeMetricName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestSanitizeMetricName_BytesSuffix verifies _bytes is added
func TestSanitizeMetricName_BytesSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"memory.byte.used", "memory_byte_used_bytes"},
		// Regression test for bug: metrics with ".bytes." in name should get _bytes suffix
		{"memory.bytes.used", "memory_bytes_used_bytes"},
		{"disk.bytes.read", "disk_bytes_read_total"}, // "read" is counter keyword - counters only get _total
		{"network.bytes.available", "network_bytes_available_bytes"},
		// Already has bytes but tx is a counter
		{"network.tx.bytes", "network_tx_bytes_total"},
		// Has bytes but write is a counter keyword
		{"disk.bytes.write", "disk_bytes_write_total"}, // "write" is counter - only gets _total
		// Should not double-add
		{"memory_bytes", "memory_bytes"},
		{"disk_read_bytes", "disk_read_bytes_total"}, // "read" is counter keyword - only gets _total
	}

	for _, tt := range tests {
		result := sanitizeMetricName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeMetricName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestSanitizeMetricName_CounterSuffix verifies _total is added for counters
func TestSanitizeMetricName_CounterSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"network.tx.total", "network_tx_total"},
		{"requests.count", "requests_count_total"},
		{"bytes.sent", "bytes_sent_total"},
		{"packets.received", "packets_received_total"},
		{"disk.read.operations", "disk_read_operations_total"},
		{"errors.total", "errors_total"},
		// Should not double-add
		{"requests_total", "requests_total"},
		// Non-counters
		{"cpu.temperature", "cpu_temperature_celsius"},
		{"memory.available", "memory_available"},
	}

	for _, tt := range tests {
		result := sanitizeMetricName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeMetricName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestIsCounter verifies counter detection heuristic
func TestIsCounter(t *testing.T) {
	tests := []struct {
		name      string
		isCounter bool
	}{
		{"network.tx.total", true},
		{"requests.count", true},
		{"bytes.sent", true},
		{"packets.received", true},
		{"errors.failed", true},
		{"requests.success", true},
		{"api.request", true},
		{"http.response", true},
		{"cpu.temperature", false},
		{"memory.available", false},
		{"disk.usage.percent", false},
	}

	for _, tt := range tests {
		result := isCounter(tt.name)
		if result != tt.isCounter {
			t.Errorf("isCounter(%q) = %v, want %v", tt.name, result, tt.isCounter)
		}
	}
}

// TestSanitizeMetricName_CounterWithBytesTotal is a regression test for the bug where
// counter metrics whose names already end in _total still fell through to byte-suffix logic,
// resulting in double suffixes like network_rx_bytes_total_bytes.
// This breaks PromQL queries that expect network_rx_bytes_total.
func TestSanitizeMetricName_CounterWithBytesTotal(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		issue    string
	}{
		{
			"network.rx_bytes_total",
			"network_rx_bytes_total",
			"should not add _bytes suffix to counter already ending in _total",
		},
		{
			"network.tx_bytes_total",
			"network_tx_bytes_total",
			"should not add _bytes suffix to counter already ending in _total",
		},
		{
			"disk.read_bytes_total",
			"disk_read_bytes_total",
			"should not add _bytes suffix to counter already ending in _total",
		},
		{
			"disk.write_bytes_total",
			"disk_write_bytes_total",
			"should not add _bytes suffix to counter already ending in _total",
		},
		{
			"network.rx.bytes.total",
			"network_rx_bytes_total",
			"dots should become underscores, but no double suffix",
		},
		{
			// Counter without _total but with bytes - should only get _total
			"network.rx.bytes",
			"network_rx_bytes_total",
			"counter with bytes should only get _total, not _bytes",
		},
		{
			// Already has both _bytes and _total
			"network_rx_bytes_total",
			"network_rx_bytes_total",
			"should be idempotent - no changes needed",
		},
	}

	for _, tt := range tests {
		result := sanitizeMetricName(tt.input)
		if result != tt.expected {
			t.Errorf("%s: sanitizeMetricName(%q) = %q, want %q", tt.issue, tt.input, result, tt.expected)
		}
	}
}

// TestBuildVMJSONL_WithTags verifies tags are included and sorted
func TestBuildVMJSONL_WithTags(t *testing.T) {
	now := time.Now()
	metric := models.NewMetric("cpu.temperature", 45.5, "device-001").
		WithTimestamp(now).
		WithTag("zone", "thermal0").
		WithTag("cpu", "0").
		WithTag("host", "server1")

	result, _, err := BuildVMJSONL([]*models.Metric{metric})
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	lines := bytes.Split(result, []byte("\n"))
	var vmMetric VMMetric
	if err := json.Unmarshal(lines[0], &vmMetric); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify tags are present
	if vmMetric.Metric["zone"] != "thermal0" {
		t.Errorf("Expected zone tag 'thermal0', got '%s'", vmMetric.Metric["zone"])
	}
	if vmMetric.Metric["cpu"] != "0" {
		t.Errorf("Expected cpu tag '0', got '%s'", vmMetric.Metric["cpu"])
	}
	if vmMetric.Metric["host"] != "server1" {
		t.Errorf("Expected host tag 'server1', got '%s'", vmMetric.Metric["host"])
	}

	// Verify tag count (3 user tags + __name__ + device_id)
	if len(vmMetric.Metric) != 5 {
		t.Errorf("Expected 5 labels, got %d: %v", len(vmMetric.Metric), vmMetric.Metric)
	}
}

// TestBuildVMJSONL_FiltersStorageTags verifies internal tags are excluded
func TestBuildVMJSONL_FiltersStorageTags(t *testing.T) {
	now := time.Now()
	metric := models.NewMetric("cpu.temperature", 45.5, "device-001").
		WithTimestamp(now).
		WithTag("_storage_id", "12345").
		WithTag("zone", "thermal0")

	result, _, err := BuildVMJSONL([]*models.Metric{metric})
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	lines := bytes.Split(result, []byte("\n"))
	var vmMetric VMMetric
	if err := json.Unmarshal(lines[0], &vmMetric); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Should not include _storage_id
	if _, exists := vmMetric.Metric["_storage_id"]; exists {
		t.Errorf("Internal tag _storage_id should be filtered out")
	}

	// Should include zone
	if vmMetric.Metric["zone"] != "thermal0" {
		t.Errorf("Expected zone tag 'thermal0', got '%s'", vmMetric.Metric["zone"])
	}

	// Should have: __name__, device_id, zone (3 labels)
	if len(vmMetric.Metric) != 3 {
		t.Errorf("Expected 3 labels, got %d: %v", len(vmMetric.Metric), vmMetric.Metric)
	}
}

// TestBuildVMJSONL_EmptyTags verifies handling of metrics without tags
func TestBuildVMJSONL_EmptyTags(t *testing.T) {
	now := time.Now()
	metric := models.NewMetric("cpu.temperature", 45.5, "device-001").
		WithTimestamp(now)

	result, _, err := BuildVMJSONL([]*models.Metric{metric})
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	lines := bytes.Split(result, []byte("\n"))
	var vmMetric VMMetric
	if err := json.Unmarshal(lines[0], &vmMetric); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Should have exactly: __name__ and device_id
	if len(vmMetric.Metric) != 2 {
		t.Errorf("Expected 2 labels, got %d: %v", len(vmMetric.Metric), vmMetric.Metric)
	}
	if vmMetric.Metric["__name__"] != "cpu_temperature_celsius" {
		t.Errorf("Expected __name__, got '%s'", vmMetric.Metric["__name__"])
	}
	if vmMetric.Metric["device_id"] != "device-001" {
		t.Errorf("Expected device_id, got '%s'", vmMetric.Metric["device_id"])
	}
}

// TestBuildVMJSONL_EmptyInput verifies handling of empty metric slice
func TestBuildVMJSONL_EmptyInput(t *testing.T) {
	result, _, err := BuildVMJSONL([]*models.Metric{})
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty output for empty input, got %d bytes", len(result))
	}
}

// TestCompressGzip verifies gzip compression works
func TestCompressGzip(t *testing.T) {
	input := []byte("Hello, World! This is test data for compression.")

	compressed, err := CompressGzip(input)
	if err != nil {
		t.Fatalf("CompressGzip failed: %v", err)
	}

	// Should be smaller (or at least different)
	if len(compressed) == 0 {
		t.Errorf("Compressed data is empty")
	}

	// Verify we can decompress
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	if !bytes.Equal(decompressed, input) {
		t.Errorf("Decompressed data doesn't match input:\nGot:  %s\nWant: %s", decompressed, input)
	}
}

// TestBuildChunks_SingleChunk verifies small batches fit in one chunk
func TestBuildChunks_SingleChunk(t *testing.T) {
	now := time.Now()
	metrics := make([]*models.Metric, 10)
	for i := 0; i < 10; i++ {
		metrics[i] = models.NewMetric("cpu.temperature", float64(40+i), "device-001").
			WithTimestamp(now.Add(time.Duration(i) * time.Second))
	}

	chunks, err := BuildChunks(metrics, 50)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk for 10 metrics, got %d", len(chunks))
	}

	chunk := chunks[0]
	if len(chunk.Metrics) != 10 {
		t.Errorf("Expected 10 metrics in chunk, got %d", len(chunk.Metrics))
	}
	if len(chunk.CompressedData) == 0 {
		t.Errorf("CompressedData is empty")
	}
	if chunk.Size != len(chunk.CompressedData) {
		t.Errorf("Size mismatch: Size=%d, len(CompressedData)=%d", chunk.Size, len(chunk.CompressedData))
	}
}

// TestBuildChunks_MultipleChunks verifies chunking splits correctly
func TestBuildChunks_MultipleChunks(t *testing.T) {
	now := time.Now()
	metrics := make([]*models.Metric, 100)
	for i := 0; i < 100; i++ {
		metrics[i] = models.NewMetric("cpu.temperature", float64(40+i), "device-001").
			WithTimestamp(now.Add(time.Duration(i) * time.Second))
	}

	chunks, err := BuildChunks(metrics, 30)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// 100 metrics / 30 per chunk = 4 chunks (30, 30, 30, 10)
	if len(chunks) != 4 {
		t.Fatalf("Expected 4 chunks, got %d", len(chunks))
	}

	// Verify sizes
	if len(chunks[0].Metrics) != 30 {
		t.Errorf("Chunk 0: expected 30 metrics, got %d", len(chunks[0].Metrics))
	}
	if len(chunks[3].Metrics) != 10 {
		t.Errorf("Chunk 3: expected 10 metrics, got %d", len(chunks[3].Metrics))
	}

	// Verify all chunks have compressed data
	for i, chunk := range chunks {
		if len(chunk.CompressedData) == 0 {
			t.Errorf("Chunk %d has no compressed data", i)
		}
		if chunk.Size != len(chunk.CompressedData) {
			t.Errorf("Chunk %d size mismatch", i)
		}
	}
}

// TestBuildChunks_SortsByTimestamp verifies metrics are sorted before chunking
func TestBuildChunks_SortsByTimestamp(t *testing.T) {
	now := time.Now()
	// Create metrics in reverse chronological order
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.0, "device-001").WithTimestamp(now.Add(5 * time.Second)),
		models.NewMetric("cpu.temperature", 44.0, "device-001").WithTimestamp(now.Add(3 * time.Second)),
		models.NewMetric("cpu.temperature", 43.0, "device-001").WithTimestamp(now.Add(1 * time.Second)),
	}

	chunks, err := BuildChunks(metrics, 50)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// Should have 1 chunk with metrics sorted by timestamp
	chunk := chunks[0]
	if chunk.Metrics[0].TimestampMs > chunk.Metrics[1].TimestampMs {
		t.Errorf("Metrics not sorted: %d > %d", chunk.Metrics[0].TimestampMs, chunk.Metrics[1].TimestampMs)
	}
	if chunk.Metrics[1].TimestampMs > chunk.Metrics[2].TimestampMs {
		t.Errorf("Metrics not sorted: %d > %d", chunk.Metrics[1].TimestampMs, chunk.Metrics[2].TimestampMs)
	}
}

// TestBuildChunks_DefaultChunkSize verifies default of 50 is used
func TestBuildChunks_DefaultChunkSize(t *testing.T) {
	now := time.Now()
	metrics := make([]*models.Metric, 75)
	for i := 0; i < 75; i++ {
		metrics[i] = models.NewMetric("cpu.temperature", float64(40+i), "device-001").
			WithTimestamp(now.Add(time.Duration(i) * time.Second))
	}

	// Pass 0 or negative to trigger default
	chunks, err := BuildChunks(metrics, 0)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// 75 metrics / 50 per chunk = 2 chunks (50, 25)
	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks with default size, got %d", len(chunks))
	}
	if len(chunks[0].Metrics) != 50 {
		t.Errorf("Expected 50 metrics in first chunk, got %d", len(chunks[0].Metrics))
	}
	if len(chunks[1].Metrics) != 25 {
		t.Errorf("Expected 25 metrics in second chunk, got %d", len(chunks[1].Metrics))
	}
}

// TestBuildChunks_SizeLimit verifies 256KB hard limit enforcement
func TestBuildChunks_SizeLimit(t *testing.T) {
	now := time.Now()

	// Create metrics with large, unique tag values to exceed size limit
	// Each metric needs different data to prevent compression
	metrics := make([]*models.Metric, 100)
	for i := 0; i < 100; i++ {
		// Create unique, incompressible data for each metric
		largeTag := ""
		for j := 0; j < 15000; j++ {
			// Use metric index and position to create unique patterns
			largeTag += string(rune('a' + ((i*j + j) % 26)))
		}

		metrics[i] = models.NewMetric("cpu.temperature", float64(40+i), "device-001").
			WithTimestamp(now.Add(time.Duration(i)*time.Second)).
			WithTag("large_tag", largeTag).
			WithTag("metric_id", fmt.Sprintf("metric-%d", i))
	}

	// Should automatically bisect to stay under 256KB
	chunks, err := BuildChunks(metrics, 50)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// Should have split into multiple chunks
	if len(chunks) <= 1 {
		t.Errorf("Expected multiple chunks due to size limit, got %d", len(chunks))
	}

	// Verify all chunks are under 256KB
	const MaxChunkSizeBytes = 256 * 1024
	for i, chunk := range chunks {
		if chunk.Size > MaxChunkSizeBytes {
			t.Errorf("Chunk %d exceeds size limit: %d bytes > %d bytes", i, chunk.Size, MaxChunkSizeBytes)
		}
	}
}

// TestBuildChunks_EmptyInput verifies handling of empty metric slice
func TestBuildChunks_EmptyInput(t *testing.T) {
	chunks, err := BuildChunks([]*models.Metric{}, 50)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty input, got %d", len(chunks))
	}
}

// TestBuildVMJSONL_FiltersStringMetrics verifies string metrics are excluded
func TestBuildVMJSONL_FiltersStringMetrics(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
		models.NewStringMetric("system.status", "healthy", "device-001").WithTimestamp(now.Add(time.Second)),
		models.NewMetric("memory.used", 1024.0, "device-001").WithTimestamp(now.Add(2 * time.Second)),
		models.NewStringMetric("error.message", "connection timeout", "device-001").WithTimestamp(now.Add(3 * time.Second)),
	}

	result, _, err := BuildVMJSONL(metrics)
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	// Should only have 2 lines (2 numeric metrics), not 4
	lines := bytes.Split(result, []byte("\n"))
	if len(lines) != 3 { // 2 metrics + empty after final \n
		t.Fatalf("Expected 3 splits (2 lines + empty), got %d", len(lines))
	}

	// Verify first line is numeric metric
	var vmMetric1 VMMetric
	if err := json.Unmarshal(lines[0], &vmMetric1); err != nil {
		t.Fatalf("Failed to parse line 0: %v", err)
	}
	if vmMetric1.Metric["__name__"] != "cpu_temperature_celsius" {
		t.Errorf("Expected cpu_temperature_celsius, got '%s'", vmMetric1.Metric["__name__"])
	}

	// Verify second line is numeric metric
	var vmMetric2 VMMetric
	if err := json.Unmarshal(lines[1], &vmMetric2); err != nil {
		t.Fatalf("Failed to parse line 1: %v", err)
	}
	if vmMetric2.Metric["__name__"] != "memory_used" {
		t.Errorf("Expected memory_used, got '%s'", vmMetric2.Metric["__name__"])
	}
}

// TestBuildVMJSONL_AllStringMetrics verifies empty output when all metrics are strings
func TestBuildVMJSONL_AllStringMetrics(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewStringMetric("system.status", "healthy", "device-001").WithTimestamp(now),
		models.NewStringMetric("error.message", "none", "device-001").WithTimestamp(now.Add(time.Second)),
	}

	result, _, err := BuildVMJSONL(metrics)
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	// Should be empty since all metrics are strings
	if len(result) != 0 {
		t.Errorf("Expected empty output for all string metrics, got %d bytes: %s", len(result), string(result))
	}
}

// TestBuildChunks_FiltersStringMetrics verifies string metrics don't create chunks
func TestBuildChunks_FiltersStringMetrics(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").WithTimestamp(now),
		models.NewStringMetric("system.status", "healthy", "device-001").WithTimestamp(now.Add(time.Second)),
		models.NewMetric("memory.used", 1024.0, "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}

	chunks, err := BuildChunks(metrics, 50)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// Should have 1 chunk with only 2 numeric metrics
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	// Chunk.Metrics contains all 3 original metrics, but JSONL should only have 2
	if len(chunk.Metrics) != 3 {
		t.Errorf("Expected 3 original metrics in chunk.Metrics, got %d", len(chunk.Metrics))
	}

	// Verify JSONL only has numeric metrics
	lines := bytes.Split(chunk.JSONLData, []byte("\n"))
	if len(lines) != 3 { // 2 numeric + empty
		t.Errorf("Expected 3 JSONL lines (2 numeric + empty), got %d", len(lines))
	}
}

// TestBuildChunks_SkipsEmptyChunks verifies chunks with only string metrics are skipped
func TestBuildChunks_SkipsEmptyChunks(t *testing.T) {
	now := time.Now()
	// Create 3 chunks worth of metrics: all strings, mixed, all numeric
	metrics := []*models.Metric{
		// First 50: all strings (should be skipped)
		models.NewStringMetric("error.message.0", "error 0", "device-001").WithTimestamp(now),
		models.NewStringMetric("error.message.1", "error 1", "device-001").WithTimestamp(now.Add(time.Second)),
		models.NewStringMetric("error.message.2", "error 2", "device-001").WithTimestamp(now.Add(2 * time.Second)),

		// Next 50: mixed (should create 1 chunk)
		models.NewMetric("cpu.temperature.0", 45.0, "device-001").WithTimestamp(now.Add(3 * time.Second)),
		models.NewStringMetric("status.0", "ok", "device-001").WithTimestamp(now.Add(4 * time.Second)),
		models.NewMetric("cpu.temperature.1", 46.0, "device-001").WithTimestamp(now.Add(5 * time.Second)),

		// Last 50: all numeric (should create 1 chunk)
		models.NewMetric("memory.used.0", 1024.0, "device-001").WithTimestamp(now.Add(6 * time.Second)),
		models.NewMetric("memory.used.1", 2048.0, "device-001").WithTimestamp(now.Add(7 * time.Second)),
	}

	chunks, err := BuildChunks(metrics, 3) // Small chunk size to force multiple chunks
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// Should have 2 chunks (all-string chunk skipped, mixed chunk, all-numeric chunk)
	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks (empty chunk skipped), got %d", len(chunks))
	}

	// First chunk should have mixed metrics (2 numeric out of 3 total)
	if len(chunks[0].Metrics) != 3 {
		t.Errorf("Chunk 0: expected 3 original metrics, got %d", len(chunks[0].Metrics))
	}
	lines0 := bytes.Split(chunks[0].JSONLData, []byte("\n"))
	if len(lines0) != 3 { // 2 numeric + empty
		t.Errorf("Chunk 0: expected 3 JSONL lines (2 numeric + empty), got %d", len(lines0))
	}

	// Second chunk should have all numeric (2 out of 2)
	if len(chunks[1].Metrics) != 2 {
		t.Errorf("Chunk 1: expected 2 metrics, got %d", len(chunks[1].Metrics))
	}
	lines1 := bytes.Split(chunks[1].JSONLData, []byte("\n"))
	if len(lines1) != 3 { // 2 numeric + empty
		t.Errorf("Chunk 1: expected 3 JSONL lines (2 numeric + empty), got %d", len(lines1))
	}
}

// TestBuildChunks_AllStringMetricsReturnsEmpty verifies all-string batch returns no chunks
func TestBuildChunks_AllStringMetricsReturnsEmpty(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewStringMetric("error.message", "error 1", "device-001").WithTimestamp(now),
		models.NewStringMetric("status", "degraded", "device-001").WithTimestamp(now.Add(time.Second)),
		models.NewStringMetric("log.level", "warn", "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}

	chunks, err := BuildChunks(metrics, 50)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// Should return empty slice (no chunks created for all-string metrics)
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for all-string metrics, got %d", len(chunks))
	}
}

// TestBuildVMJSONL_TracksIncludedIDs verifies that IncludedIDs contains only numeric metrics
func TestBuildVMJSONL_TracksIncludedIDs(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 45.5, "device-001").
			WithTimestamp(now).
			WithTag("_storage_id", "100"),
		models.NewStringMetric("system.status", "healthy", "device-001").
			WithTimestamp(now.Add(time.Second)).
			WithTag("_storage_id", "101"),
		models.NewMetric("memory.used", 1024.0, "device-001").
			WithTimestamp(now.Add(2*time.Second)).
			WithTag("_storage_id", "102"),
	}

	_, includedIDs, err := BuildVMJSONL(metrics)
	if err != nil {
		t.Fatalf("BuildVMJSONL failed: %v", err)
	}

	// Should only include IDs for numeric metrics (100, 102), not string metric (101)
	if len(includedIDs) != 2 {
		t.Fatalf("Expected 2 included IDs, got %d", len(includedIDs))
	}

	if includedIDs[0] != 100 {
		t.Errorf("Expected first ID to be 100, got %d", includedIDs[0])
	}
	if includedIDs[1] != 102 {
		t.Errorf("Expected second ID to be 102, got %d", includedIDs[1])
	}
}

// TestBuildChunks_TracksIncludedIDsAcrossChunks verifies IDs are tracked across multiple chunks
func TestBuildChunks_TracksIncludedIDsAcrossChunks(t *testing.T) {
	now := time.Now()
	metrics := []*models.Metric{
		// First chunk: 2 numeric
		models.NewMetric("metric1", 1.0, "device-001").WithTimestamp(now).WithTag("_storage_id", "1"),
		models.NewMetric("metric2", 2.0, "device-001").WithTimestamp(now.Add(time.Second)).WithTag("_storage_id", "2"),
		// String metric (should be skipped)
		models.NewStringMetric("status1", "ok", "device-001").WithTimestamp(now.Add(2*time.Second)).WithTag("_storage_id", "3"),
		// Second chunk: 2 numeric
		models.NewMetric("metric3", 3.0, "device-001").WithTimestamp(now.Add(3*time.Second)).WithTag("_storage_id", "4"),
		models.NewMetric("metric4", 4.0, "device-001").WithTimestamp(now.Add(4*time.Second)).WithTag("_storage_id", "5"),
	}

	chunks, err := BuildChunks(metrics, 3) // Small chunk size to force multiple chunks
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}

	// Should have 2 chunks (first chunk has 2 numeric + 1 string, second has 2 numeric)
	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks, got %d", len(chunks))
	}

	// First chunk should have IDs 1, 2 (not 3 because it's a string)
	if len(chunks[0].IncludedIDs) != 2 {
		t.Errorf("Chunk 0: expected 2 included IDs, got %d", len(chunks[0].IncludedIDs))
	}
	if chunks[0].IncludedIDs[0] != 1 || chunks[0].IncludedIDs[1] != 2 {
		t.Errorf("Chunk 0: unexpected IDs %v", chunks[0].IncludedIDs)
	}

	// Second chunk should have IDs 4, 5
	if len(chunks[1].IncludedIDs) != 2 {
		t.Errorf("Chunk 1: expected 2 included IDs, got %d", len(chunks[1].IncludedIDs))
	}
	if chunks[1].IncludedIDs[0] != 4 || chunks[1].IncludedIDs[1] != 5 {
		t.Errorf("Chunk 1: unexpected IDs %v", chunks[1].IncludedIDs)
	}
}

// TestUploadAndGetIDs_ReturnsOnlyNumericIDs verifies UploadAndGetIDs excludes string metrics
func TestUploadAndGetIDs_ReturnsOnlyNumericIDs(t *testing.T) {
	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	uploader := NewHTTPUploader(server.URL, "test-device")

	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temp", 45.0, "test-device").
			WithTimestamp(now).
			WithTag("_storage_id", "10"),
		models.NewStringMetric("status", "ok", "test-device").
			WithTimestamp(now.Add(time.Second)).
			WithTag("_storage_id", "11"),
		models.NewMetric("memory.used", 1024.0, "test-device").
			WithTimestamp(now.Add(2*time.Second)).
			WithTag("_storage_id", "12"),
	}

	ctx := context.Background()
	uploadedIDs, err := uploader.UploadAndGetIDs(ctx, metrics)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Should only return IDs for numeric metrics (10, 12), not string (11)
	if len(uploadedIDs) != 2 {
		t.Fatalf("Expected 2 uploaded IDs, got %d: %v", len(uploadedIDs), uploadedIDs)
	}

	if uploadedIDs[0] != 10 || uploadedIDs[1] != 12 {
		t.Errorf("Expected IDs [10, 12], got %v", uploadedIDs)
	}
}
