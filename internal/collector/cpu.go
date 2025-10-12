package collector

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/taniwha3/thugshells/internal/models"
)

// CPUStats represents CPU time counters from /proc/stat
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

// CPUCollector collects CPU usage metrics with delta calculation
type CPUCollector struct {
	deviceID     string
	mu           sync.Mutex
	previousCPU  map[string]*CPUStats // Core name -> previous stats
	firstSample  bool                 // True until we have a baseline
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

// Collect gathers CPU usage metrics using delta calculation
func (c *CPUCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	if runtime.GOOS == "darwin" {
		// Mock CPU usage on macOS for development
		return c.collectMock(), nil
	}

	// Read current CPU stats
	currentStats, err := c.readCPUStats()
	if err != nil {
		return nil, fmt.Errorf("failed to read CPU stats: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If this is the first sample, just cache it and return empty
	// We need a previous sample to calculate deltas
	if c.firstSample {
		c.previousCPU = currentStats
		c.firstSample = false
		return []*models.Metric{}, nil
	}

	// Calculate deltas for each core
	var metrics []*models.Metric

	for coreName, currentStat := range currentStats {
		previousStat, exists := c.previousCPU[coreName]
		if !exists {
			// New core appeared, skip this sample
			continue
		}

		// Calculate deltas
		deltaTotal := currentStat.Total() - previousStat.Total()
		deltaBusy := currentStat.Busy() - previousStat.Busy()

		// Check for wraparound (counters are uint64 but can theoretically wrap)
		if currentStat.Total() < previousStat.Total() {
			// Counter wrapped around, skip this sample
			continue
		}

		// Check for division by zero
		if deltaTotal == 0 {
			// No time elapsed, skip this sample
			continue
		}

		// Calculate usage percentage
		usagePercent := float64(deltaBusy) / float64(deltaTotal) * 100.0

		// Create metric
		if coreName == "cpu" {
			// Aggregate "all" cores metric
			m := models.NewMetric("cpu.usage_percent", usagePercent, c.deviceID)
			metrics = append(metrics, m)
		} else {
			// Per-core metric
			coreNum := strings.TrimPrefix(coreName, "cpu")
			m := models.NewMetric("cpu.core_usage_percent", usagePercent, c.deviceID).
				WithTag("core", coreNum)
			metrics = append(metrics, m)
		}
	}

	// Update previous stats for next collection
	c.previousCPU = currentStats

	return metrics, nil
}

// readCPUStats parses /proc/stat and returns CPU stats per core
func (c *CPUCollector) readCPUStats() (map[string]*CPUStats, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/stat: %w", err)
	}

	stats := make(map[string]*CPUStats)
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue // Invalid line
		}

		coreName := fields[0]

		// Parse CPU time fields (all in USER_HZ units, typically 1/100th of a second)
		stat := &CPUStats{}
		stat.User, _ = strconv.ParseUint(fields[1], 10, 64)
		stat.Nice, _ = strconv.ParseUint(fields[2], 10, 64)
		stat.System, _ = strconv.ParseUint(fields[3], 10, 64)
		stat.Idle, _ = strconv.ParseUint(fields[4], 10, 64)
		stat.IOWait, _ = strconv.ParseUint(fields[5], 10, 64)
		stat.IRQ, _ = strconv.ParseUint(fields[6], 10, 64)
		stat.SoftIRQ, _ = strconv.ParseUint(fields[7], 10, 64)

		// Optional fields (not always present)
		if len(fields) > 8 {
			stat.Steal, _ = strconv.ParseUint(fields[8], 10, 64)
		}
		if len(fields) > 9 {
			stat.Guest, _ = strconv.ParseUint(fields[9], 10, 64)
		}
		if len(fields) > 10 {
			stat.GuestNice, _ = strconv.ParseUint(fields[10], 10, 64)
		}

		stats[coreName] = stat
	}

	return stats, nil
}

// collectMock returns mock CPU usage for macOS development
func (c *CPUCollector) collectMock() []*models.Metric {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Skip first sample even on mock
	if c.firstSample {
		c.firstSample = false
		return []*models.Metric{}
	}

	// Return mock CPU usage (40-60% range)
	usage := 40.0 + float64(len(c.deviceID)%20)

	metrics := []*models.Metric{
		models.NewMetric("cpu.usage_percent", usage, c.deviceID),
	}

	// Add per-core mocks (simulate 4-8 cores)
	numCores := 4 + (len(c.deviceID) % 5)
	for i := 0; i < numCores; i++ {
		coreUsage := usage + float64((i*7)%20) - 10.0 // Vary per core
		m := models.NewMetric("cpu.core_usage_percent", coreUsage, c.deviceID).
			WithTag("core", strconv.Itoa(i))
		metrics = append(metrics, m)
	}

	return metrics
}
