package models

import "time"

// Metric represents a single metric data point
type Metric struct {
	TimestampMs int64             // Unix timestamp in milliseconds
	Name        string            // Metric name (e.g., "cpu.temperature")
	Value       float64           // Numeric value
	DeviceID    string            // Device identifier
	Tags        map[string]string // Optional tags for dimensions
}

// NewMetric creates a new metric with the current timestamp
func NewMetric(name string, value float64, deviceID string) *Metric {
	return &Metric{
		TimestampMs: time.Now().UnixMilli(),
		Name:        name,
		Value:       value,
		DeviceID:    deviceID,
		Tags:        make(map[string]string),
	}
}

// WithTimestamp sets a specific timestamp
func (m *Metric) WithTimestamp(ts time.Time) *Metric {
	m.TimestampMs = ts.UnixMilli()
	return m
}

// WithTag adds a tag to the metric
func (m *Metric) WithTag(key, value string) *Metric {
	if m.Tags == nil {
		m.Tags = make(map[string]string)
	}
	m.Tags[key] = value
	return m
}
