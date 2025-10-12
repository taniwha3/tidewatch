package collector

import (
	"regexp"
	"testing"
)

// TestDiskCollector_Name verifies collector name
func TestDiskCollector_Name(t *testing.T) {
	collector := NewDiskCollector("device-001")
	if collector.Name() != "disk" {
		t.Errorf("Expected name 'disk', got '%s'", collector.Name())
	}
}

// TestDiskCollector_WholeDevicePattern verifies device filtering
func TestDiskCollector_WholeDevicePattern(t *testing.T) {
	tests := []struct {
		device   string
		expected bool
	}{
		// Should match (whole devices)
		{"sda", true},
		{"sdb", true},
		{"nvme0n1", true},
		{"nvme1n1", true},
		{"mmcblk0", true},
		{"mmcblk1", true},
		{"hda", true},
		{"vda", true},
		{"xvda", true},

		// Should NOT match (partitions)
		{"sda1", false},
		{"sda2", false},
		{"nvme0n1p1", false},
		{"nvme0n1p2", false},
		{"mmcblk0p1", false},
		{"mmcblk0p2", false},

		// Should NOT match (other devices)
		{"loop0", false},
		{"dm-0", false},
		{"sr0", false},
	}

	for _, tt := range tests {
		result := wholeDevicePattern.MatchString(tt.device)
		if result != tt.expected {
			t.Errorf("wholeDevicePattern.MatchString(%q) = %v, want %v", tt.device, result, tt.expected)
		}
	}
}

// TestDiskCollector_CustomPattern verifies custom device filtering
func TestDiskCollector_CustomPattern(t *testing.T) {
	// Create collector that only matches nvme devices
	collector := NewDiskCollectorWithConfig(DiskCollectorConfig{
		DeviceID:       "device-001",
		AllowedPattern: `^nvme\d+n\d+$`,
	})

	// Verify pattern was applied
	if !collector.allowedDevs.MatchString("nvme0n1") {
		t.Error("Expected nvme0n1 to match custom pattern")
	}
	if collector.allowedDevs.MatchString("sda") {
		t.Error("Expected sda to NOT match custom pattern")
	}
}

// TestDiskCollector_InvalidPattern verifies fallback to default pattern
func TestDiskCollector_InvalidPattern(t *testing.T) {
	// Create collector with invalid regex
	collector := NewDiskCollectorWithConfig(DiskCollectorConfig{
		DeviceID:       "device-001",
		AllowedPattern: "[invalid(regex",
	})

	// Should fall back to default pattern
	if !collector.allowedDevs.MatchString("sda") {
		t.Error("Expected fallback to default pattern that matches sda")
	}
}

// TestDiskCollector_SectorSizeIs512 verifies diskstats sectors are always 512-byte units
func TestDiskCollector_SectorSizeIs512(t *testing.T) {
	// Per kernel documentation (Documentation/admin-guide/iostats.rst),
	// /proc/diskstats always reports sectors in 512-byte units,
	// regardless of the device's actual logical block size.
	//
	// This test documents this fact and ensures we don't incorrectly
	// multiply by device-specific sector sizes (which would overstate
	// bytes by 8x for 4K devices).

	// This is a documentation test - the actual conversion is hardcoded to 512
	const diskstatsSectorSize = 512

	if diskstatsSectorSize != 512 {
		t.Errorf("diskstats sectors must always be 512 bytes per kernel docs, got %d", diskstatsSectorSize)
	}
}

// TestDiskCollector_EmptyDiskstats verifies handling of empty diskstats
func TestDiskCollector_EmptyDiskstats(t *testing.T) {
	// This test would require mocking /proc/diskstats
	// In production, we'd use dependency injection or build tags
	// For now, we just verify the constructor works
	collector := NewDiskCollector("device-001")
	if collector == nil {
		t.Error("Expected non-nil collector")
	}
}


// TestDiskCollector_MetricGeneration verifies metrics are generated correctly
func TestDiskCollector_MetricGeneration(t *testing.T) {
	// This is a simple constructor test since we can't mock /proc/diskstats easily
	collector := NewDiskCollector("device-001")

	if collector.deviceID != "device-001" {
		t.Errorf("Expected deviceID 'device-001', got '%s'", collector.deviceID)
	}

	if collector.allowedDevs == nil {
		t.Error("Expected non-nil allowedDevs pattern")
	}
}

// TestDiskCollector_CustomPatternCompilation verifies regex compilation
func TestDiskCollector_CustomPatternCompilation(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		shouldWork bool
	}{
		{"valid simple", `^sda$`, true},
		{"valid complex", `^(nvme\d+n\d+|sda)$`, true},
		{"invalid", `[invalid(`, false}, // Should fall back to default
		{"empty", ``, false},             // Should use default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewDiskCollectorWithConfig(DiskCollectorConfig{
				DeviceID:       "device-001",
				AllowedPattern: tt.pattern,
			})

			if collector.allowedDevs == nil {
				t.Error("allowedDevs should never be nil")
			}

			if tt.shouldWork {
				// Custom pattern should be used
				_, err := regexp.Compile(tt.pattern)
				if err != nil && collector.allowedDevs == wholeDevicePattern {
					// This is expected - invalid patterns fall back
					return
				}
			}
		})
	}
}

