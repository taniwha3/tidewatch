//go:build darwin

package collector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/taniwha3/thugshells/internal/models"
)

// collect implements macOS-specific CPU collection using gopsutil
func (c *CPUCollector) collect(ctx context.Context) ([]*models.Metric, error) {
	var metrics []*models.Metric

	// Get overall CPU usage (average across all cores)
	// Use 1 second interval for accurate measurement
	percentages, err := cpu.PercentWithContext(ctx, 1*time.Second, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU percentage: %w", err)
	}

	if len(percentages) > 0 {
		m := models.NewMetric("cpu.usage_percent", percentages[0], c.deviceID)
		metrics = append(metrics, m)
	}

	// Get per-core CPU usage
	perCorePercents, err := cpu.PercentWithContext(ctx, 1*time.Second, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get per-core CPU percentage: %w", err)
	}

	for i, corePercent := range perCorePercents {
		m := models.NewMetric("cpu.core_usage_percent", corePercent, c.deviceID).
			WithTag("core", strconv.Itoa(i))
		metrics = append(metrics, m)
	}

	return metrics, nil
}
