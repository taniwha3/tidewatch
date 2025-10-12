package uploader

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
)

// TestBuildVMJSONL_SingleMetric verifies basic JSONL structure for a single metric
func TestBuildVMJSONL_SingleMetric(t *testing.T) {
	now := time.Now()
	metric := models.NewMetric("cpu.temperature", 45.5, "device-001").
		WithTimestamp(now)

	result, err := BuildVMJSONL([]*models.Metric{metric})
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

	result, err := BuildVMJSONL(metrics)
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
		// Should not double-add
		{"cpu_temperature_celsius", "cpu_temperature_celsius"},
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

// TestBuildVMJSONL_WithTags verifies tags are included and sorted
func TestBuildVMJSONL_WithTags(t *testing.T) {
	now := time.Now()
	metric := models.NewMetric("cpu.temperature", 45.5, "device-001").
		WithTimestamp(now).
		WithTag("zone", "thermal0").
		WithTag("cpu", "0").
		WithTag("host", "server1")

	result, err := BuildVMJSONL([]*models.Metric{metric})
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

	result, err := BuildVMJSONL([]*models.Metric{metric})
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

	result, err := BuildVMJSONL([]*models.Metric{metric})
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
	result, err := BuildVMJSONL([]*models.Metric{})
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
			WithTimestamp(now.Add(time.Duration(i) * time.Second)).
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
