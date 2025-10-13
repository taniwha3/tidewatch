package collector

import (
	"context"

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
// Platform-specific implementations in memory_linux.go and memory_darwin.go
func (c *MemoryCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	return c.collect(ctx)
}

// collectMock returns mock memory metrics for testing
func (c *MemoryCollector) collectMock() []*models.Metric {
	// Mock data: 8 GB total, 4 GB used, 4 GB available
	// 2 GB swap total, 500 MB swap used
	const (
		totalBytes     = 8 * 1024 * 1024 * 1024 // 8 GB
		usedBytes      = 4 * 1024 * 1024 * 1024 // 4 GB
		availableBytes = 4 * 1024 * 1024 * 1024 // 4 GB
		swapTotal      = 2 * 1024 * 1024 * 1024 // 2 GB
		swapUsed       = 500 * 1024 * 1024      // 500 MB
	)

	metrics := []*models.Metric{
		models.NewMetric("memory.used_bytes", float64(usedBytes), c.deviceID),
		models.NewMetric("memory.available_bytes", float64(availableBytes), c.deviceID),
		models.NewMetric("memory.total_bytes", float64(totalBytes), c.deviceID),
		models.NewMetric("memory.swap_used_bytes", float64(swapUsed), c.deviceID),
		models.NewMetric("memory.swap_total_bytes", float64(swapTotal), c.deviceID),
	}

	return metrics
}

// Meminfo represents parsed /proc/meminfo data (Linux-specific)
type Meminfo struct {
	MemTotal     uint64 // Total usable RAM
	MemAvailable uint64 // Estimate of available memory
	SwapTotal    uint64 // Total swap space available
	SwapFree     uint64 // Amount of swap space that is currently unused
}
