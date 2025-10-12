package collector

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/taniwha3/thugshells/internal/models"
)

// MemoryCollector collects memory usage metrics
type MemoryCollector struct {
	deviceID string
}

// NewMemoryCollector creates a new memory metrics collector
func NewMemoryCollector(deviceID string) *MemoryCollector {
	return &MemoryCollector{
		deviceID: deviceID,
	}
}

// Name returns the collector name
func (c *MemoryCollector) Name() string {
	return "memory"
}

// Collect gathers memory usage metrics
func (c *MemoryCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	if runtime.GOOS == "darwin" {
		// Mock memory metrics on macOS for development
		return c.collectMock(), nil
	}

	// Read /proc/meminfo
	meminfo, err := c.parseMeminfo()
	if err != nil {
		return nil, fmt.Errorf("failed to parse /proc/meminfo: %w", err)
	}

	// Calculate canonical used (Linux-recommended approach)
	// Used = MemTotal - MemAvailable
	// This accounts for buffers/cache that can be reclaimed
	memUsed := meminfo.MemTotal - meminfo.MemAvailable
	swapUsed := meminfo.SwapTotal - meminfo.SwapFree

	metrics := []*models.Metric{
		// Memory metrics in bytes
		models.NewMetric("memory.used_bytes", float64(memUsed), c.deviceID),
		models.NewMetric("memory.available_bytes", float64(meminfo.MemAvailable), c.deviceID),
		models.NewMetric("memory.total_bytes", float64(meminfo.MemTotal), c.deviceID),

		// Swap metrics in bytes
		models.NewMetric("memory.swap_used_bytes", float64(swapUsed), c.deviceID),
		models.NewMetric("memory.swap_total_bytes", float64(meminfo.SwapTotal), c.deviceID),
	}

	return metrics, nil
}

// Meminfo represents parsed /proc/meminfo data
type Meminfo struct {
	MemTotal      uint64 // Total usable RAM (i.e., physical RAM minus a few reserved bits and the kernel binary code)
	MemAvailable  uint64 // Estimate of available memory (free + reclaimable buffers/cache)
	SwapTotal     uint64 // Total swap space available
	SwapFree      uint64 // Amount of swap space that is currently unused
}

// parseMeminfo reads and parses /proc/meminfo
func (c *MemoryCollector) parseMeminfo() (*Meminfo, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/meminfo: %w", err)
	}

	meminfo := &Meminfo{}
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		// Values in /proc/meminfo are in KB, convert to bytes
		valueBytes := value * 1024

		switch key {
		case "MemTotal":
			meminfo.MemTotal = valueBytes
		case "MemAvailable":
			meminfo.MemAvailable = valueBytes
		case "SwapTotal":
			meminfo.SwapTotal = valueBytes
		case "SwapFree":
			meminfo.SwapFree = valueBytes
		}
	}

	// Validate we got the required fields
	if meminfo.MemTotal == 0 {
		return nil, fmt.Errorf("missing MemTotal in /proc/meminfo")
	}

	return meminfo, nil
}

// collectMock returns mock memory metrics for macOS development
func (c *MemoryCollector) collectMock() []*models.Metric {
	// Mock: 8 GB total, 40% used
	totalBytes := uint64(8 * 1024 * 1024 * 1024) // 8 GB
	usedBytes := uint64(totalBytes * 40 / 100)   // 40% used
	availableBytes := totalBytes - usedBytes

	// Mock: 2 GB swap, 10% used
	swapTotalBytes := uint64(2 * 1024 * 1024 * 1024) // 2 GB
	swapUsedBytes := uint64(swapTotalBytes * 10 / 100) // 10% used

	metrics := []*models.Metric{
		models.NewMetric("memory.used_bytes", float64(usedBytes), c.deviceID),
		models.NewMetric("memory.available_bytes", float64(availableBytes), c.deviceID),
		models.NewMetric("memory.total_bytes", float64(totalBytes), c.deviceID),
		models.NewMetric("memory.swap_used_bytes", float64(swapUsedBytes), c.deviceID),
		models.NewMetric("memory.swap_total_bytes", float64(swapTotalBytes), c.deviceID),
	}

	return metrics
}
