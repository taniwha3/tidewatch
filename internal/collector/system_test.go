package collector

import (
	"context"
	"runtime"
	"testing"
)

func TestSystemCollector_Name(t *testing.T) {
	c := NewSystemCollector("test-device")
	if c.Name() != "system" {
		t.Errorf("Expected name 'system', got '%s'", c.Name())
	}
}

func TestSystemCollector_Collect(t *testing.T) {
	c := NewSystemCollector("test-device-001")

	ctx := context.Background()
	metrics, err := c.Collect(ctx)

	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("Expected at least one metric (cpu.temperature)")
	}

	// Check for cpu.temperature metric
	foundCPUTemp := false
	for _, m := range metrics {
		if m.Name == "cpu.temperature" {
			foundCPUTemp = true
			if m.DeviceID != "test-device-001" {
				t.Errorf("Expected deviceID test-device-001, got %s", m.DeviceID)
			}
			if m.Value <= 0 || m.Value > 150 {
				t.Errorf("Unexpected temperature value: %f", m.Value)
			}
			if m.TimestampMs == 0 {
				t.Error("Expected non-zero timestamp")
			}
		}
	}

	if !foundCPUTemp {
		t.Error("cpu.temperature metric not found")
	}
}

func TestSystemCollector_MacOSMock(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	c := NewSystemCollector("test-device")
	temp, err := c.getCPUTemperature()

	if err != nil {
		t.Fatalf("getCPUTemperature failed: %v", err)
	}

	// Mock should return reasonable values
	if temp < 40 || temp > 60 {
		t.Errorf("Mock temperature out of range: %f", temp)
	}
}

func TestSystemCollector_CollectCancellation(t *testing.T) {
	c := NewSystemCollector("test-device")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should still complete quickly even if context is cancelled
	// (Current implementation doesn't check context, but we test the interface)
	_, err := c.Collect(ctx)
	if err != nil {
		t.Errorf("Collect should handle cancelled context gracefully: %v", err)
	}
}
