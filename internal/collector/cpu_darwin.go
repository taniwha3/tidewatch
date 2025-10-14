//go:build darwin

package collector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/taniwha3/tidewatch/internal/models"
)

// collect implements macOS-specific CPU collection using gopsutil
func (c *CPUCollector) collect(ctx context.Context) ([]*models.Metric, error) {
	var metrics []*models.Metric

	// Get per-core CPU usage with a single 1-second sample
	// This avoids double-blocking (calling PercentWithContext twice)
	perCorePercents, err := cpu.PercentWithContext(ctx, 1*time.Second, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get per-core CPU percentage: %w", err)
	}

	// Calculate aggregate CPU usage as average of all cores
	var totalUsage float64
	for i, corePercent := range perCorePercents {
		totalUsage += corePercent

		// Per-core metric
		m := models.NewMetric("cpu.core_usage_percent", corePercent, c.deviceID).
			WithTag("core", strconv.Itoa(i))
		metrics = append(metrics, m)
	}

	// Overall CPU usage (average across all cores)
	if len(perCorePercents) > 0 {
		avgUsage := totalUsage / float64(len(perCorePercents))
		m := models.NewMetric("cpu.usage_percent", avgUsage, c.deviceID)
		metrics = append(metrics, m)
	}

	return metrics, nil
}
