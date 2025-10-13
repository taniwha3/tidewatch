package collector

import (
	"context"
	"fmt"
	"sync"

	"github.com/taniwha3/thugshells/internal/models"
)

// CPUStats represents CPU time counters from /proc/stat (Linux-specific)
type CPUStats struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

// Total returns total CPU time (excluding guest which is already counted in user)
func (s *CPUStats) Total() uint64 {
	return s.User + s.Nice + s.System + s.Idle + s.IOWait + s.IRQ + s.SoftIRQ + s.Steal
}

// Busy returns busy (non-idle) CPU time
func (s *CPUStats) Busy() uint64 {
	return s.Total() - s.Idle - s.IOWait
}

// CPUCollector collects CPU usage metrics
type CPUCollector struct {
	deviceID    string
	mu          sync.Mutex
	previousCPU map[string]*CPUStats // Core name -> previous stats (Linux only)
	firstSample bool                 // True until we have a baseline (Linux only)
}

// NewCPUCollector creates a new CPU usage collector
func NewCPUCollector(deviceID string) *CPUCollector {
	return &CPUCollector{
		deviceID:    deviceID,
		previousCPU: make(map[string]*CPUStats),
		firstSample: true,
	}
}

// Name returns the collector name
func (c *CPUCollector) Name() string {
	return "cpu"
}

// Collect gathers CPU usage metrics
// Platform-specific implementations in cpu_linux.go and cpu_darwin.go
func (c *CPUCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	return c.collect(ctx)
}

// collectMock returns mock CPU metrics for testing
func (c *CPUCollector) collectMock() []*models.Metric {
	c.mu.Lock()
	defer c.mu.Unlock()

	// First sample returns empty (need baseline)
	if c.firstSample {
		c.firstSample = false
		return []*models.Metric{}
	}

	// Second sample returns mock metrics
	var metrics []*models.Metric

	// Per-core metrics (4 cores)
	for i := 0; i < 4; i++ {
		usage := 45.2 + float64(i)*5.0 // Different usage per core
		m := models.NewMetric("cpu.core_usage_percent", usage, c.deviceID).
			WithTag("core", fmt.Sprintf("%d", i))
		metrics = append(metrics, m)
	}

	// Overall CPU usage (no tags)
	overallMetric := models.NewMetric("cpu.usage_percent", 52.5, c.deviceID)
	metrics = append(metrics, overallMetric)

	return metrics
}
