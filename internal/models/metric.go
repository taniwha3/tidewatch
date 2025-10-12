package models

import "time"

// ValueType indicates the type of metric value
type ValueType int

const (
	ValueTypeNumeric ValueType = 0 // Numeric gauge/counter
	ValueTypeString  ValueType = 1 // String value (for errors, states, etc.)
)

// Metric represents a single metric data point
type Metric struct {
	TimestampMs int64             // Unix timestamp in milliseconds
	Name        string            // Metric name (e.g., "cpu.temperature")
	Value       float64           // Numeric value (when ValueType = 0)
	ValueText   string            // String value (when ValueType = 1)
	ValueType   ValueType         // Type of value (numeric or string)
	DeviceID    string            // Device identifier
	Tags        map[string]string // Optional tags for dimensions
}

// NewMetric creates a new numeric metric with the current timestamp
func NewMetric(name string, value float64, deviceID string) *Metric {
	return &Metric{
		TimestampMs: time.Now().UnixMilli(),
		Name:        name,
		Value:       value,
		ValueType:   ValueTypeNumeric,
		DeviceID:    deviceID,
		Tags:        make(map[string]string),
	}
}

// NewStringMetric creates a new string metric with the current timestamp
func NewStringMetric(name string, valueText string, deviceID string) *Metric {
	return &Metric{
		TimestampMs: time.Now().UnixMilli(),
		Name:        name,
		ValueText:   valueText,
		ValueType:   ValueTypeString,
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
