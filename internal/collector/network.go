package collector

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/taniwha3/thugshells/internal/models"
)

// Default interface exclusion patterns (high-cardinality ephemeral interfaces)
var defaultExcludePatterns = []string{
	`^lo$`,           // Loopback
	`^docker.*`,      // Docker bridges (docker0, docker_gwbridge, etc.)
	`^veth.*`,        // Virtual ethernet pairs (veth1a2b3c, vethABC123, etc.)
	`^br-.*`,         // Linux bridges (br-abc123, etc.)
	`^wlan\d+mon.*`,  // Wireless monitor interfaces (wlan0mon, wlan0mon1, etc.)
	`^virbr.*`,       // libvirt bridges (virbr0, virbr0-nic, etc.)
	`^wwan.*`,        // Wireless WAN (wwan0, etc.)
	`^wwp.*`,         // Wireless WAN alternative (wwp0s20f0u6c2, etc.)
	`^usb.*`,         // USB network (usb0, usb1, etc.)
}

// NetworkStats represents network interface statistics
type NetworkStats struct {
	RxBytes   uint64
	RxPackets uint64
	RxErrors  uint64
	TxBytes   uint64
	TxPackets uint64
	TxErrors  uint64
}

// NetworkCollector collects network interface metrics with filtering and cardinality protection
type NetworkCollector struct {
	deviceID              string
	excludePatterns       []*regexp.Regexp
	includePattern        *regexp.Regexp
	maxInterfaces         int
	mu                    sync.Mutex
	previousStats         map[string]*NetworkStats // Interface -> previous stats for wraparound detection
	interfacesDroppedTotal uint64                    // Monotonic counter of dropped interfaces across all collections
}

// NetworkCollectorConfig configures the network collector
type NetworkCollectorConfig struct {
	DeviceID        string
	ExcludePatterns []string // Regex patterns to exclude (empty = use defaults)
	IncludePattern  string   // Regex pattern to include (empty = match all)
	MaxInterfaces   int      // Hard cap on interface count (default 32)
}

// NewNetworkCollector creates a new network collector with default settings
func NewNetworkCollector(deviceID string) *NetworkCollector {
	return NewNetworkCollectorWithConfig(NetworkCollectorConfig{
		DeviceID:      deviceID,
		MaxInterfaces: 32,
	})
}

// NewNetworkCollectorWithConfig creates a new network collector with custom configuration
func NewNetworkCollectorWithConfig(cfg NetworkCollectorConfig) *NetworkCollector {
	c := &NetworkCollector{
		deviceID:      cfg.DeviceID,
		maxInterfaces: cfg.MaxInterfaces,
		previousStats: make(map[string]*NetworkStats),
	}

	if c.maxInterfaces <= 0 {
		c.maxInterfaces = 32 // Default
	}

	// Set up exclude patterns
	excludeList := cfg.ExcludePatterns
	if len(excludeList) == 0 {
		excludeList = defaultExcludePatterns
	}

	for _, pattern := range excludeList {
		if re, err := regexp.Compile(pattern); err == nil {
			c.excludePatterns = append(c.excludePatterns, re)
		}
	}

	// Set up optional include pattern
	if cfg.IncludePattern != "" {
		if re, err := regexp.Compile(cfg.IncludePattern); err == nil {
			c.includePattern = re
		}
	}

	return c
}

// Name returns the collector name
func (c *NetworkCollector) Name() string {
	return "network"
}

// Collect gathers network interface metrics
func (c *NetworkCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	if runtime.GOOS == "darwin" {
		// Mock network metrics on macOS for development
		return c.collectMock(), nil
	}

	// Read /proc/net/dev
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/net/dev: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var metrics []*models.Metric
	interfacesProcessed := 0
	interfacesDropped := 0

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		// Skip header lines
		if i < 2 {
			continue
		}

		// Parse line: "  eth0: 123456 ..."
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		ifaceName := strings.TrimSpace(parts[0])
		if ifaceName == "" {
			continue
		}

		// Apply exclusion filters
		if c.isExcluded(ifaceName) {
			continue
		}

		// Apply optional inclusion filter
		if c.includePattern != nil && !c.includePattern.MatchString(ifaceName) {
			continue
		}

		// Cardinality guard: enforce max interfaces
		if interfacesProcessed >= c.maxInterfaces {
			interfacesDropped++
			c.interfacesDroppedTotal++ // Increment monotonic total
			continue
		}

		// Parse stats
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}

		// /proc/net/dev format (bytes packets errs drop fifo frame compressed multicast)
		// RX: fields 0-7
		// TX: fields 8-15
		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
		rxErrors, _ := strconv.ParseUint(fields[2], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[9], 10, 64)
		txErrors, _ := strconv.ParseUint(fields[10], 10, 64)

		currentStats := &NetworkStats{
			RxBytes:   rxBytes,
			RxPackets: rxPackets,
			RxErrors:  rxErrors,
			TxBytes:   txBytes,
			TxPackets: txPackets,
			TxErrors:  txErrors,
		}

		// Check for wraparound (counters are uint64, can theoretically wrap)
		// Packet counters wrap more frequently than byte counters (especially if kernel exposes as 32-bit)
		if prevStats, exists := c.previousStats[ifaceName]; exists {
			if currentStats.RxBytes < prevStats.RxBytes ||
				currentStats.RxPackets < prevStats.RxPackets ||
				currentStats.RxErrors < prevStats.RxErrors ||
				currentStats.TxBytes < prevStats.TxBytes ||
				currentStats.TxPackets < prevStats.TxPackets ||
				currentStats.TxErrors < prevStats.TxErrors {
				// Wraparound detected, skip this sample but update baseline
				// so the next collection can resume (otherwise interface is stuck forever)
				c.previousStats[ifaceName] = currentStats
				continue
			}
		}

		// Store current stats for next collection
		c.previousStats[ifaceName] = currentStats

		// Create metrics (counters)
		metrics = append(metrics,
			models.NewMetric("network.rx_bytes_total", float64(rxBytes), c.deviceID).
				WithTag("interface", ifaceName))

		metrics = append(metrics,
			models.NewMetric("network.rx_packets_total", float64(rxPackets), c.deviceID).
				WithTag("interface", ifaceName))

		metrics = append(metrics,
			models.NewMetric("network.rx_errors_total", float64(rxErrors), c.deviceID).
				WithTag("interface", ifaceName))

		metrics = append(metrics,
			models.NewMetric("network.tx_bytes_total", float64(txBytes), c.deviceID).
				WithTag("interface", ifaceName))

		metrics = append(metrics,
			models.NewMetric("network.tx_packets_total", float64(txPackets), c.deviceID).
				WithTag("interface", ifaceName))

		metrics = append(metrics,
			models.NewMetric("network.tx_errors_total", float64(txErrors), c.deviceID).
				WithTag("interface", ifaceName))

		interfacesProcessed++
	}

	// Emit cardinality metric (monotonic total across all collections)
	if c.interfacesDroppedTotal > 0 {
		metrics = append(metrics,
			models.NewMetric("network.interfaces_dropped_total", float64(c.interfacesDroppedTotal), c.deviceID))
	}

	return metrics, nil
}

// isExcluded checks if an interface should be excluded
func (c *NetworkCollector) isExcluded(ifaceName string) bool {
	for _, pattern := range c.excludePatterns {
		if pattern.MatchString(ifaceName) {
			return true
		}
	}
	return false
}

// collectMock returns mock network metrics for macOS development
func (c *NetworkCollector) collectMock() []*models.Metric {
	// Mock: 2 interfaces (en0, en1) with realistic traffic
	interfaces := []string{"en0", "en1"}
	metrics := []*models.Metric{}

	for _, iface := range interfaces {
		// Mock some traffic (GB range)
		rxBytes := uint64(10 * 1024 * 1024 * 1024) // 10 GB
		txBytes := uint64(5 * 1024 * 1024 * 1024)  // 5 GB
		rxPackets := uint64(1000000)
		txPackets := uint64(500000)
		rxErrors := uint64(0) // Mock: no errors
		txErrors := uint64(0)

		metrics = append(metrics,
			models.NewMetric("network.rx_bytes_total", float64(rxBytes), c.deviceID).
				WithTag("interface", iface))

		metrics = append(metrics,
			models.NewMetric("network.rx_packets_total", float64(rxPackets), c.deviceID).
				WithTag("interface", iface))

		metrics = append(metrics,
			models.NewMetric("network.rx_errors_total", float64(rxErrors), c.deviceID).
				WithTag("interface", iface))

		metrics = append(metrics,
			models.NewMetric("network.tx_bytes_total", float64(txBytes), c.deviceID).
				WithTag("interface", iface))

		metrics = append(metrics,
			models.NewMetric("network.tx_packets_total", float64(txPackets), c.deviceID).
				WithTag("interface", iface))

		metrics = append(metrics,
			models.NewMetric("network.tx_errors_total", float64(txErrors), c.deviceID).
				WithTag("interface", iface))
	}

	return metrics
}
