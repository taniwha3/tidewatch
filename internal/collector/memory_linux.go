//go:build linux

package collector

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/taniwha3/thugshells/internal/models"
)

// collect implements Linux-specific memory collection using /proc/meminfo
func (c *MemoryCollector) collect(ctx context.Context) ([]*models.Metric, error) {
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
