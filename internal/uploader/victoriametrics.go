package uploader

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/taniwha3/thugshells/internal/models"
)

// VictoriaMetrics JSONL format implementation
// See: https://docs.victoriametrics.com/#how-to-import-time-series-data

// VMMetric represents a single metric in VictoriaMetrics import format
type VMMetric struct {
	Metric     map[string]string `json:"metric"`
	Values     []float64         `json:"values"`
	Timestamps []int64           `json:"timestamps"`
}

// sanitizeMetricName converts metric names to PromQL-safe format
// Per engineering review: dots -> underscores, add unit suffixes, _total for counters
func sanitizeMetricName(name string) string {
	// Replace dots with underscores
	safe := strings.ReplaceAll(name, ".", "_")

	// Counters should end with _total (highest priority - counters don't need unit suffixes)
	// Heuristic: metrics with "total", "count", or metrics that are cumulative
	if isCounter(name) && !strings.HasSuffix(safe, "_total") {
		safe += "_total"
		return safe
	}

	// Add unit suffixes if missing (only for non-counters)
	if strings.Contains(name, "temperature") && !strings.HasSuffix(safe, "_celsius") {
		safe += "_celsius"
	}

	// Handle byte/bytes metrics - check for both "bytes" and "byte"
	if (strings.Contains(name, "bytes") || strings.Contains(name, "byte")) && !strings.HasSuffix(safe, "_bytes") {
		safe += "_bytes"
	}

	return safe
}

// isCounter determines if a metric name represents a counter
func isCounter(name string) bool {
	// Common counter patterns
	counterKeywords := []string{
		"total", "count", "sent", "received", "tx", "rx",
		"read", "write", "uploaded", "downloaded", "failed",
		"success", "error", "request", "response",
	}

	lowerName := strings.ToLower(name)
	for _, keyword := range counterKeywords {
		if strings.Contains(lowerName, keyword) {
			return true
		}
	}

	return false
}

// BuildVMJSONL converts metrics to VictoriaMetrics JSONL format
// Each line is a separate JSON object
func BuildVMJSONL(metrics []*models.Metric) ([]byte, error) {
	if len(metrics) == 0 {
		return []byte{}, nil
	}

	var buf bytes.Buffer

	for _, m := range metrics {
		// Build metric labels (tags)
		labels := make(map[string]string)

		// Add __name__ (required)
		labels["__name__"] = sanitizeMetricName(m.Name)

		// Add device_id
		if m.DeviceID != "" {
			labels["device_id"] = m.DeviceID
		}

		// Add user tags (sorted for consistency)
		if m.Tags != nil {
			keys := make([]string, 0, len(m.Tags))
			for k := range m.Tags {
				// Skip internal storage tags
				if k == "_storage_id" {
					continue
				}
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				labels[k] = m.Tags[k]
			}
		}

		// Create VM metric structure
		vmMetric := VMMetric{
			Metric:     labels,
			Values:     []float64{m.Value},
			Timestamps: []int64{m.TimestampMs},
		}

		// Marshal to JSON
		line, err := json.Marshal(vmMetric)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metric %s: %w", m.Name, err)
		}

		// Write line + newline
		buf.Write(line)
		buf.WriteByte('\n')
	}

	return buf.Bytes(), nil
}

// CompressGzip compresses data using gzip
func CompressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)
	if _, err := gzipWriter.Write(data); err != nil {
		return nil, fmt.Errorf("gzip write failed: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}

// Chunk represents a batch of metrics ready for upload
type Chunk struct {
	Metrics      []*models.Metric
	JSONLData    []byte
	CompressedData []byte
	Size         int // Size in bytes after compression
}

// BuildChunks splits metrics into chunks and compresses them
// Per engineering review: 50 metrics per chunk, ~128-256 KB target, hard cap at 256 KB
func BuildChunks(metrics []*models.Metric, chunkSize int) ([]*Chunk, error) {
	if chunkSize <= 0 {
		chunkSize = 50 // Default
	}

	const MaxChunkSizeBytes = 256 * 1024 // 256 KB hard limit

	var chunks []*Chunk

	// Sort by timestamp for better compression
	sortedMetrics := make([]*models.Metric, len(metrics))
	copy(sortedMetrics, metrics)
	sort.Slice(sortedMetrics, func(i, j int) bool {
		return sortedMetrics[i].TimestampMs < sortedMetrics[j].TimestampMs
	})

	for i := 0; i < len(sortedMetrics); i += chunkSize {
		end := i + chunkSize
		if end > len(sortedMetrics) {
			end = len(sortedMetrics)
		}

		chunkMetrics := sortedMetrics[i:end]

		// Build JSONL
		jsonlData, err := BuildVMJSONL(chunkMetrics)
		if err != nil {
			return nil, fmt.Errorf("failed to build JSONL for chunk: %w", err)
		}

		// Compress
		compressed, err := CompressGzip(jsonlData)
		if err != nil {
			return nil, fmt.Errorf("failed to compress chunk: %w", err)
		}

		// Check size limit
		if len(compressed) > MaxChunkSizeBytes {
			// Bisect and retry if chunk is too large
			if len(chunkMetrics) <= 1 {
				// Can't split further, this single metric is too large
				return nil, fmt.Errorf("single metric exceeds size limit: %d bytes", len(compressed))
			}

			// Split in half and retry
			halfSize := chunkSize / 2
			if halfSize < 1 {
				halfSize = 1
			}

			// Recursively process this range with smaller chunk size
			subMetrics := sortedMetrics[i:end]
			subChunks, err := BuildChunks(subMetrics, halfSize)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, subChunks...)
			continue
		}

		chunk := &Chunk{
			Metrics:        chunkMetrics,
			JSONLData:      jsonlData,
			CompressedData: compressed,
			Size:           len(compressed),
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
