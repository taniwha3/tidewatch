package collector

import (
	"context"
	"testing"
)

func TestMockSRTCollector_Name(t *testing.T) {
	c := NewMockSRTCollector("test-device")
	if c.Name() != "mock_srt" {
		t.Errorf("Expected name 'mock_srt', got '%s'", c.Name())
	}
}

func TestMockSRTCollector_Collect(t *testing.T) {
	c := NewMockSRTCollector("test-device-001")

	ctx := context.Background()
	metrics, err := c.Collect(ctx)

	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	m := metrics[0]
	if m.Name != "srt.packet_loss_pct" {
		t.Errorf("Expected metric name srt.packet_loss_pct, got %s", m.Name)
	}

	if m.DeviceID != "test-device-001" {
		t.Errorf("Expected deviceID test-device-001, got %s", m.DeviceID)
	}

	// Packet loss should be 0-5%
	if m.Value < 0 || m.Value > 5.1 {
		t.Errorf("Unexpected packet loss value: %f", m.Value)
	}

	if m.TimestampMs == 0 {
		t.Error("Expected non-zero timestamp")
	}
}

func TestMockSRTCollector_DeterministicSeed(t *testing.T) {
	// Same device ID should produce same sequence
	c1 := NewMockSRTCollector("device-123")
	c2 := NewMockSRTCollector("device-123")

	ctx := context.Background()

	m1, _ := c1.Collect(ctx)
	m2, _ := c2.Collect(ctx)

	if m1[0].Value != m2[0].Value {
		t.Error("Same device ID should produce same mock values")
	}
}

func TestMockSRTCollector_MultipleCollections(t *testing.T) {
	c := NewMockSRTCollector("test-device")
	ctx := context.Background()

	// Collect multiple times and verify all return valid data
	for i := 0; i < 100; i++ {
		metrics, err := c.Collect(ctx)
		if err != nil {
			t.Fatalf("Collect %d failed: %v", i, err)
		}
		if len(metrics) != 1 {
			t.Fatalf("Collect %d: expected 1 metric", i)
		}
		if metrics[0].Value < 0 || metrics[0].Value > 5.1 {
			t.Errorf("Collect %d: invalid value %f", i, metrics[0].Value)
		}
	}
}
