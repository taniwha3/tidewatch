package models

import (
	"testing"
	"time"
)

func TestNewMetric(t *testing.T) {
	deviceID := "test-device-001"
	name := "cpu.temperature"
	value := 45.5

	m := NewMetric(name, value, deviceID)

	if m.Name != name {
		t.Errorf("Expected name %s, got %s", name, m.Name)
	}
	if m.Value != value {
		t.Errorf("Expected value %f, got %f", value, m.Value)
	}
	if m.DeviceID != deviceID {
		t.Errorf("Expected deviceID %s, got %s", deviceID, m.DeviceID)
	}
	if m.TimestampMs == 0 {
		t.Error("Expected non-zero timestamp")
	}
	if m.Tags == nil {
		t.Error("Expected initialized tags map")
	}
}

func TestMetricWithTimestamp(t *testing.T) {
	m := NewMetric("test.metric", 100, "device-001")
	ts := time.Date(2025, 10, 11, 12, 0, 0, 0, time.UTC)

	m.WithTimestamp(ts)

	expected := ts.UnixMilli()
	if m.TimestampMs != expected {
		t.Errorf("Expected timestamp %d, got %d", expected, m.TimestampMs)
	}
}

func TestMetricWithTag(t *testing.T) {
	m := NewMetric("test.metric", 100, "device-001")

	m.WithTag("zone", "cpu0").WithTag("type", "temperature")

	if m.Tags["zone"] != "cpu0" {
		t.Errorf("Expected tag zone=cpu0, got %s", m.Tags["zone"])
	}
	if m.Tags["type"] != "temperature" {
		t.Errorf("Expected tag type=temperature, got %s", m.Tags["type"])
	}
}

func TestMetricChaining(t *testing.T) {
	ts := time.Date(2025, 10, 11, 12, 0, 0, 0, time.UTC)

	m := NewMetric("cpu.temp", 55.5, "device-001").
		WithTimestamp(ts).
		WithTag("core", "0").
		WithTag("zone", "big")

	if m.TimestampMs != ts.UnixMilli() {
		t.Error("Chaining failed: timestamp not set")
	}
	if m.Tags["core"] != "0" || m.Tags["zone"] != "big" {
		t.Error("Chaining failed: tags not set")
	}
}
