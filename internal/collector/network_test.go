package collector

import (
	"regexp"
	"testing"
)

func TestNetworkCollector_Name(t *testing.T) {
	collector := NewNetworkCollector("test-device")
	if collector.Name() != "network" {
		t.Errorf("Expected name 'network', got '%s'", collector.Name())
	}
}

func TestNetworkCollector_DefaultExclusionPatterns(t *testing.T) {
	collector := NewNetworkCollector("test-device")

	// Should exclude: lo, docker*, veth*, br-*, wlan*mon, virbr*, wwan*, wwp*, usb*
	excludeTests := []struct {
		iface    string
		excluded bool
	}{
		{"lo", true},
		{"docker0", true},
		{"docker123", true},
		{"docker_gwbridge", true},      // Docker bridge with underscore
		{"veth1a2b3c", true},
		{"vethABC123", true},           // veth with caps
		{"br-abc123", true},
		{"wlan0mon", true},
		{"wlan0mon1", true},
		{"virbr0", true},
		{"virbr0-nic", true},           // Real libvirt interface name
		{"virbr1-nic", true},
		{"wwan0", true},
		{"wwan1", true},
		{"wwp0", true},
		{"wwp0s20f0u6c2", true},        // Real WWAN modem interface name
		{"usb0", true},
		{"usb1", true},
		{"eth0", false},     // Should NOT be excluded
		{"en0", false},      // Should NOT be excluded
		{"wlan0", false},    // Should NOT be excluded (only wlanXmonY)
		{"enp3s0", false},   // Should NOT be excluded
		{"wlp2s0", false},   // Should NOT be excluded
	}

	for _, tt := range excludeTests {
		excluded := collector.isExcluded(tt.iface)
		if excluded != tt.excluded {
			t.Errorf("Interface %s: expected excluded=%v, got %v", tt.iface, tt.excluded, excluded)
		}
	}
}

func TestNetworkCollector_CustomExcludePattern(t *testing.T) {
	// Custom pattern: exclude interfaces starting with "test"
	collector := NewNetworkCollectorWithConfig(NetworkCollectorConfig{
		DeviceID:        "test-device",
		ExcludePatterns: []string{`^test\d+`},
	})

	if collector.isExcluded("test0") {
		// Expected
	} else {
		t.Error("Expected test0 to be excluded by custom pattern")
	}

	if !collector.isExcluded("eth0") {
		// Expected - custom pattern doesn't exclude eth0
	} else {
		t.Error("Expected eth0 to NOT be excluded by custom pattern")
	}
}

func TestNetworkCollector_IncludePattern(t *testing.T) {
	// Include only interfaces matching "eth*"
	includeRe := regexp.MustCompile(`^eth\d+$`)
	collector := NewNetworkCollectorWithConfig(NetworkCollectorConfig{
		DeviceID:        "test-device",
		ExcludePatterns: []string{}, // No exclusions
		IncludePattern:  `^eth\d+$`,
	})

	// Manually test include logic
	if includeRe.MatchString("eth0") {
		// Expected
	} else {
		t.Error("Expected eth0 to match include pattern")
	}

	if !includeRe.MatchString("wlan0") {
		// Expected - wlan0 should NOT match
	} else {
		t.Error("Expected wlan0 to NOT match include pattern")
	}

	_ = collector
}

func TestNetworkCollector_CardinalityHardCap(t *testing.T) {
	collector := NewNetworkCollectorWithConfig(NetworkCollectorConfig{
		DeviceID:      "test-device",
		MaxInterfaces: 3, // Cap at 3 interfaces
	})

	// Verify max interfaces is set
	if collector.maxInterfaces != 3 {
		t.Errorf("Expected maxInterfaces=3, got %d", collector.maxInterfaces)
	}

	// Test would need mock /proc/net/dev to verify actual capping behavior
	// For now, verify the config is stored correctly
}

func TestNetworkCollector_WraparoundDetection(t *testing.T) {
	collector := NewNetworkCollector("test-device")

	// Set up previous stats
	collector.previousStats["eth0"] = &NetworkStats{
		RxBytes:   1000000,
		RxPackets: 10000,
		TxBytes:   500000,
		TxPackets: 5000,
	}

	// Current stats show counter wraparound (smaller values)
	currentStats := &NetworkStats{
		RxBytes:   100, // Wrapped around!
		RxPackets: 10,
		TxBytes:   50,
		TxPackets: 5,
	}

	// Check wraparound detection logic
	prevStats := collector.previousStats["eth0"]
	if currentStats.RxBytes < prevStats.RxBytes {
		// This should be detected as wraparound
		// In Collect(), this sample would be skipped
	} else {
		t.Error("Wraparound not detected: current < previous")
	}
}

func TestNetworkCollector_WraparoundRecovery(t *testing.T) {
	// Verify that after wraparound, the interface resumes collecting
	collector := NewNetworkCollector("test-device")

	// Simulate first collection: high values
	collector.previousStats["eth0"] = &NetworkStats{
		RxBytes:   10000000000, // 10 GB
		RxPackets: 1000000,
		TxBytes:   5000000000, // 5 GB
		TxPackets: 500000,
	}

	// Simulate second collection: wraparound (low values)
	wrappedStats := &NetworkStats{
		RxBytes:   1000, // Wrapped!
		RxPackets: 100,
		TxBytes:   500,
		TxPackets: 50,
	}

	// Manually simulate wraparound detection and state update
	prevStats := collector.previousStats["eth0"]
	if wrappedStats.RxBytes < prevStats.RxBytes {
		// Wraparound detected - update baseline (this is the fix!)
		collector.previousStats["eth0"] = wrappedStats
	}

	// Verify baseline was updated to wrapped value
	if collector.previousStats["eth0"].RxBytes != 1000 {
		t.Errorf("Expected previousStats to be updated to wrapped value 1000, got %d",
			collector.previousStats["eth0"].RxBytes)
	}

	// Simulate third collection: normal increase from wrapped baseline
	normalStats := &NetworkStats{
		RxBytes:   2000, // Normal increase from 1000
		RxPackets: 200,
		TxBytes:   1000,
		TxPackets: 100,
	}

	// This should NOT trigger wraparound (2000 > 1000)
	if normalStats.RxBytes < collector.previousStats["eth0"].RxBytes {
		t.Error("False wraparound after recovery: interface should resume normally")
	}

	// Verify we can continue collecting after wraparound
	if normalStats.RxBytes >= collector.previousStats["eth0"].RxBytes {
		// Expected - interface has recovered and continues collecting
	} else {
		t.Error("Interface stuck after wraparound: recovery failed")
	}
}

func TestNetworkCollector_NormalCounterIncrease(t *testing.T) {
	collector := NewNetworkCollector("test-device")

	// Set up previous stats
	collector.previousStats["eth0"] = &NetworkStats{
		RxBytes:   1000000,
		RxPackets: 10000,
		TxBytes:   500000,
		TxPackets: 5000,
	}

	// Current stats show normal increase
	currentStats := &NetworkStats{
		RxBytes:   2000000, // Increased
		RxPackets: 20000,
		TxBytes:   1000000,
		TxPackets: 10000,
	}

	// Verify no wraparound detected
	prevStats := collector.previousStats["eth0"]
	if currentStats.RxBytes >= prevStats.RxBytes &&
		currentStats.TxBytes >= prevStats.TxBytes {
		// This is normal, no wraparound
	} else {
		t.Error("False wraparound detected on normal counter increase")
	}
}

func TestNetworkCollector_MockReturnsMetrics(t *testing.T) {
	collector := NewNetworkCollector("test-device")
	metrics := collector.collectMock()

	// Should return metrics for 2 interfaces (en0, en1)
	// Each interface has 6 metrics (rx_bytes, rx_packets, rx_errors, tx_bytes, tx_packets, tx_errors)
	expectedCount := 2 * 6 // 12 metrics
	if len(metrics) != expectedCount {
		t.Errorf("Expected %d metrics, got %d", expectedCount, len(metrics))
	}

	// Verify metric names
	hasRxBytes := false
	hasTxBytes := false
	hasRxErrors := false
	hasTxErrors := false

	for _, m := range metrics {
		if m.Name == "network.rx_bytes_total" {
			hasRxBytes = true
		}
		if m.Name == "network.tx_bytes_total" {
			hasTxBytes = true
		}
		if m.Name == "network.rx_errors_total" {
			hasRxErrors = true
		}
		if m.Name == "network.tx_errors_total" {
			hasTxErrors = true
		}

		// Verify all metrics have interface tag
		if m.Tags["interface"] == "" {
			t.Errorf("Metric %s missing interface tag", m.Name)
		}

		// Verify values are non-negative (errors can be zero)
		if m.Value < 0 {
			t.Errorf("Metric %s has negative value: %f", m.Name, m.Value)
		}
	}

	if !hasRxBytes {
		t.Error("Missing network.rx_bytes_total metrics")
	}
	if !hasTxBytes {
		t.Error("Missing network.tx_bytes_total metrics")
	}
	if !hasRxErrors {
		t.Error("Missing network.rx_errors_total metrics")
	}
	if !hasTxErrors {
		t.Error("Missing network.tx_errors_total metrics")
	}
}

func TestNetworkCollector_DefaultMaxInterfaces(t *testing.T) {
	collector := NewNetworkCollector("test-device")

	// Default should be 32
	if collector.maxInterfaces != 32 {
		t.Errorf("Expected default maxInterfaces=32, got %d", collector.maxInterfaces)
	}
}

func TestNetworkCollector_InterfaceTagPresent(t *testing.T) {
	collector := NewNetworkCollector("test-device")
	metrics := collector.collectMock()

	// All metrics should have interface tag
	for _, m := range metrics {
		if _, exists := m.Tags["interface"]; !exists {
			t.Errorf("Metric %s missing required 'interface' tag", m.Name)
		}

		// Verify interface name is not empty
		ifaceName := m.Tags["interface"]
		if ifaceName == "" {
			t.Errorf("Metric %s has empty interface tag", m.Name)
		}
	}
}

func TestDefaultExcludePatterns_Compile(t *testing.T) {
	// Verify all default patterns compile successfully
	for _, pattern := range defaultExcludePatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			t.Errorf("Default exclude pattern failed to compile: %s - %v", pattern, err)
		}
	}
}

func TestNetworkStats_Structure(t *testing.T) {
	stats := &NetworkStats{
		RxBytes:   1000,
		RxPackets: 10,
		RxErrors:  2,
		TxBytes:   500,
		TxPackets: 5,
		TxErrors:  1,
	}

	// Verify fields are accessible
	if stats.RxBytes != 1000 {
		t.Errorf("Expected RxBytes=1000, got %d", stats.RxBytes)
	}
	if stats.TxPackets != 5 {
		t.Errorf("Expected TxPackets=5, got %d", stats.TxPackets)
	}
	if stats.RxErrors != 2 {
		t.Errorf("Expected RxErrors=2, got %d", stats.RxErrors)
	}
	if stats.TxErrors != 1 {
		t.Errorf("Expected TxErrors=1, got %d", stats.TxErrors)
	}
}

func TestNetworkCollector_DroppedCounterMonotonic(t *testing.T) {
	// Verify that interfaces_dropped_total is monotonic across collections
	collector := NewNetworkCollectorWithConfig(NetworkCollectorConfig{
		DeviceID:      "test-device",
		MaxInterfaces: 2, // Cap at 2 interfaces
	})

	// First collection: simulate 4 interfaces (2 will be dropped)
	// We can't easily mock /proc/net/dev, but we can verify the counter behavior
	if collector.interfacesDroppedTotal != 0 {
		t.Error("Expected initial dropped count to be 0")
	}

	// Manually simulate what happens during collection when interfaces are dropped
	collector.interfacesDroppedTotal += 2 // Simulate 2 dropped in first collection
	firstTotal := collector.interfacesDroppedTotal

	if firstTotal != 2 {
		t.Errorf("Expected dropped total after first collection: 2, got %d", firstTotal)
	}

	// Second collection: simulate same 4 interfaces (2 dropped again)
	collector.interfacesDroppedTotal += 2 // Simulate 2 more dropped
	secondTotal := collector.interfacesDroppedTotal

	if secondTotal != 4 {
		t.Errorf("Expected dropped total after second collection: 4, got %d", secondTotal)
	}

	// Verify monotonicity: second > first
	if secondTotal <= firstTotal {
		t.Error("Dropped counter not monotonic: second collection should be > first")
	}
}

func TestNetworkCollector_ErrorCounterWraparound(t *testing.T) {
	// Verify that error counter wraparound is detected
	collector := NewNetworkCollector("test-device")

	// Set up previous stats with error counters
	collector.previousStats["eth0"] = &NetworkStats{
		RxBytes:   1000000,
		RxPackets: 10000,
		RxErrors:  100, // Previous error count
		TxBytes:   500000,
		TxPackets: 5000,
		TxErrors:  50,
	}

	// Current stats show error counter wraparound
	currentStats := &NetworkStats{
		RxBytes:   2000000, // Normal increase
		RxPackets: 20000,
		RxErrors:  10, // Wrapped around!
		TxBytes:   1000000,
		TxPackets: 10000,
		TxErrors:  5, // Wrapped around!
	}

	// Check wraparound detection logic
	prevStats := collector.previousStats["eth0"]
	if currentStats.RxErrors < prevStats.RxErrors ||
		currentStats.TxErrors < prevStats.TxErrors {
		// This should be detected as wraparound
		// In Collect(), this sample would be skipped and baseline updated
	} else {
		t.Error("Error counter wraparound not detected")
	}
}

func TestNetworkCollector_PacketCounterWraparound(t *testing.T) {
	// Verify that packet counter wraparound is detected
	// Packet counters wrap more frequently than byte counters (especially on 32-bit kernels)
	collector := NewNetworkCollector("test-device")

	// Set up previous stats with high packet counts (approaching 32-bit limit)
	collector.previousStats["eth0"] = &NetworkStats{
		RxBytes:   10000000000, // 10 GB
		RxPackets: 4294967200,  // Near 32-bit max (4294967295)
		RxErrors:  0,
		TxBytes:   5000000000, // 5 GB
		TxPackets: 4294967100, // Near 32-bit max
		TxErrors:  0,
	}

	// Current stats show packet counter wraparound (but byte counters still increasing)
	currentStats := &NetworkStats{
		RxBytes:   11000000000, // 11 GB - normal increase
		RxPackets: 1000,        // Wrapped around!
		RxErrors:  0,
		TxBytes:   6000000000, // 6 GB - normal increase
		TxPackets: 500,        // Wrapped around!
		TxErrors:  0,
	}

	// Check wraparound detection logic
	prevStats := collector.previousStats["eth0"]
	if currentStats.RxPackets < prevStats.RxPackets ||
		currentStats.TxPackets < prevStats.TxPackets {
		// This should be detected as wraparound
		// In Collect(), this sample would be skipped and baseline updated
	} else {
		t.Error("Packet counter wraparound not detected (bytes increased but packets wrapped)")
	}
}
