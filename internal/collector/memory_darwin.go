//go:build darwin

package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v3/mem"
	"github.com/taniwha3/tidewatch/internal/models"
)

// collect implements macOS-specific memory collection using gopsutil
func (c *MemoryCollector) collect(ctx context.Context) ([]*models.Metric, error) {
	// Get virtual memory stats
	vmStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual memory stats: %w", err)
	}

	// Get swap memory stats
	swapStat, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap memory stats: %w", err)
	}

	metrics := []*models.Metric{
		// Memory metrics in bytes
		models.NewMetric("memory.used_bytes", float64(vmStat.Used), c.deviceID),
		models.NewMetric("memory.available_bytes", float64(vmStat.Available), c.deviceID),
		models.NewMetric("memory.total_bytes", float64(vmStat.Total), c.deviceID),

		// Swap metrics in bytes
		models.NewMetric("memory.swap_used_bytes", float64(swapStat.Used), c.deviceID),
		models.NewMetric("memory.swap_total_bytes", float64(swapStat.Total), c.deviceID),
	}

	return metrics, nil
}
