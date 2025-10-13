//go:build darwin

package collector

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/shirou/gopsutil/v3/net"
	"github.com/taniwha3/thugshells/internal/models"
)

// collect implements macOS-specific network collection using gopsutil
func (c *NetworkCollector) collect(ctx context.Context) ([]*models.Metric, error) {
	// Get all network interface stats
	ioCounters, err := net.IOCountersWithContext(ctx, true) // pernic=true for per-interface stats
	if err != nil {
		return nil, fmt.Errorf("failed to get network IO counters: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	currentStats := make(map[string]*NetworkStats)
	var metrics []*models.Metric

	for _, io := range ioCounters {
		iface := io.Name

		// Check exclusions
		if c.isExcluded(iface) {
			continue
		}

		// Check include pattern if set
		if c.includePattern != nil && !c.includePattern.MatchString(iface) {
			continue
		}

		// Cardinality guard: enforce hard cap on interface count
		if len(currentStats) >= c.maxInterfaces {
			// Skip this interface, increment drop counter
			atomic.AddUint64(&c.interfacesDroppedTotal, 1)
			continue
		}

		// Parse current stats
		current := &NetworkStats{
			RxBytes:   io.BytesRecv,
			RxPackets: io.PacketsRecv,
			RxErrors:  io.Errin,
			TxBytes:   io.BytesSent,
			TxPackets: io.PacketsSent,
			TxErrors:  io.Errout,
		}
		currentStats[iface] = current

		// Check for counter wraparound
		if prev, exists := c.previousStats[iface]; exists {
			if current.RxBytes < prev.RxBytes ||
				current.TxBytes < prev.TxBytes ||
				current.RxPackets < prev.RxPackets ||
				current.TxPackets < prev.TxPackets ||
				current.RxErrors < prev.RxErrors ||
				current.TxErrors < prev.TxErrors {
				// Wraparound detected - update baseline and skip this sample
				c.previousStats[iface] = current
				continue
			}
		}

		// RX bytes (counter)
		metrics = append(metrics,
			models.NewMetric("network.rx_bytes_total", float64(current.RxBytes), c.deviceID).
				WithTag("interface", iface))

		// TX bytes (counter)
		metrics = append(metrics,
			models.NewMetric("network.tx_bytes_total", float64(current.TxBytes), c.deviceID).
				WithTag("interface", iface))

		// RX packets (counter)
		metrics = append(metrics,
			models.NewMetric("network.rx_packets_total", float64(current.RxPackets), c.deviceID).
				WithTag("interface", iface))

		// TX packets (counter)
		metrics = append(metrics,
			models.NewMetric("network.tx_packets_total", float64(current.TxPackets), c.deviceID).
				WithTag("interface", iface))

		// RX errors (counter)
		metrics = append(metrics,
			models.NewMetric("network.rx_errors_total", float64(current.RxErrors), c.deviceID).
				WithTag("interface", iface))

		// TX errors (counter)
		metrics = append(metrics,
			models.NewMetric("network.tx_errors_total", float64(current.TxErrors), c.deviceID).
				WithTag("interface", iface))
	}

	// Emit meta-metric for dropped interfaces (cardinality guard)
	droppedTotal := atomic.LoadUint64(&c.interfacesDroppedTotal)
	if droppedTotal > 0 {
		metrics = append(metrics,
			models.NewMetric("network.interfaces_dropped_total", float64(droppedTotal), c.deviceID))
	}

	// Update cache for next collection
	c.previousStats = currentStats

	return metrics, nil
}
