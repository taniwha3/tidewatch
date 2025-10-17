package collector

import (
	"context"
	"regexp"
	"sync"

	"github.com/taniwha3/tidewatch/internal/models"
)

// Default interface exclusion patterns (high-cardinality ephemeral interfaces)
var defaultExcludePatterns = []string{
	`^lo$`,          // Loopback
	`^docker.*`,     // Docker bridges (docker0, docker_gwbridge, etc.)
	`^veth.*`,       // Virtual ethernet pairs (veth1a2b3c, vethABC123, etc.)
	`^br-.*`,        // Linux bridges (br-abc123, etc.)
	`^wlan\d+mon.*`, // Wireless monitor interfaces (wlan0mon, wlan0mon1, etc.)
	`^virbr.*`,      // libvirt bridges (virbr0, virbr0-nic, etc.)
	`^wwan.*`,       // Wireless WAN (wwan0, etc.)
	`^wwp.*`,        // Wireless WAN alternative (wwp0s20f0u6c2, etc.)
	`^usb.*`,        // USB network (usb0, usb1, etc.)
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
	deviceID               string
	excludePatterns        []*regexp.Regexp
	includePattern         *regexp.Regexp
	maxInterfaces          int
	mu                     sync.Mutex
	previousStats          map[string]*NetworkStats // Interface -> previous stats for wraparound detection
	interfacesDroppedTotal uint64                   // Monotonic counter of dropped interfaces across all collections
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
// Platform-specific implementations in network_linux.go and network_darwin.go
func (c *NetworkCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	return c.collect(ctx)
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

// collectMock returns mock network metrics for testing
func (c *NetworkCollector) collectMock() []*models.Metric {
	var metrics []*models.Metric

	// Mock data for 2 interfaces (en0, en1)
	interfaces := []struct {
		name    string
		rxBytes uint64
		rxPkts  uint64
		rxErrs  uint64
		txBytes uint64
		txPkts  uint64
		txErrs  uint64
	}{
		{"en0", 1000000, 10000, 0, 500000, 5000, 0},
		{"en1", 2000000, 20000, 5, 1000000, 10000, 2},
	}

	for _, iface := range interfaces {
		metrics = append(metrics,
			models.NewMetric("network.rx_bytes_total", float64(iface.rxBytes), c.deviceID).
				WithTag("interface", iface.name),
			models.NewMetric("network.rx_packets_total", float64(iface.rxPkts), c.deviceID).
				WithTag("interface", iface.name),
			models.NewMetric("network.rx_errors_total", float64(iface.rxErrs), c.deviceID).
				WithTag("interface", iface.name),
			models.NewMetric("network.tx_bytes_total", float64(iface.txBytes), c.deviceID).
				WithTag("interface", iface.name),
			models.NewMetric("network.tx_packets_total", float64(iface.txPkts), c.deviceID).
				WithTag("interface", iface.name),
			models.NewMetric("network.tx_errors_total", float64(iface.txErrs), c.deviceID).
				WithTag("interface", iface.name),
		)
	}

	return metrics
}
