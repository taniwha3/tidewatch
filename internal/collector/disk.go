package collector

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/taniwha3/thugshells/internal/models"
)

// Regex to match whole block devices (not partitions)
// Matches: sda, sdb, nvme0n1, nvme1n1, mmcblk0, etc.
// Skips: sda1, nvme0n1p1, mmcblk0p1, etc.
var wholeDevicePattern = regexp.MustCompile(`^(sd[a-z]+|nvme\d+n\d+|mmcblk\d+|hd[a-z]+|vd[a-z]+|xvd[a-z]+)$`)

// DiskCollector collects disk I/O metrics from /proc/diskstats
// with per-device sector size detection for accurate byte calculations
type DiskCollector struct {
	deviceID    string
	allowedDevs *regexp.Regexp // Optional: user-configurable filter
}

// DiskCollectorConfig configures the disk collector
type DiskCollectorConfig struct {
	DeviceID       string
	AllowedPattern string // Regex pattern for allowed devices (empty = use default)
}

// NewDiskCollector creates a new disk I/O collector
func NewDiskCollector(deviceID string) *DiskCollector {
	return NewDiskCollectorWithConfig(DiskCollectorConfig{
		DeviceID: deviceID,
	})
}

// NewDiskCollectorWithConfig creates a new disk I/O collector with custom configuration
func NewDiskCollectorWithConfig(cfg DiskCollectorConfig) *DiskCollector {
	c := &DiskCollector{
		deviceID: cfg.DeviceID,
	}

	// Use custom pattern if provided, otherwise use default
	if cfg.AllowedPattern != "" {
		if pattern, err := regexp.Compile(cfg.AllowedPattern); err == nil {
			c.allowedDevs = pattern
		}
	}

	if c.allowedDevs == nil {
		c.allowedDevs = wholeDevicePattern
	}

	return c
}

// Name returns the collector name
func (c *DiskCollector) Name() string {
	return "disk"
}

// Collect gathers disk I/O metrics from /proc/diskstats
func (c *DiskCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/diskstats: %w", err)
	}

	var metrics []*models.Metric

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}

		device := fields[2]

		// Filter to whole devices only (skip partitions)
		if !c.allowedDevs.MatchString(device) {
			continue
		}

		// Field indices from /proc/diskstats format:
		// 0: major, 1: minor, 2: device name
		// 3: reads completed, 4: reads merged, 5: sectors read, 6: time reading (ms)
		// 7: writes completed, 8: writes merged, 9: sectors written, 10: time writing (ms)
		// 11: IOs in progress, 12: time doing I/O (ms), 13: weighted time
		//
		// IMPORTANT: Per kernel documentation (Documentation/admin-guide/iostats.rst),
		// the "sectors" fields are ALWAYS in 512-byte units, regardless of the device's
		// actual logical block size. Do NOT multiply by device sector size.

		// Reads completed (field 3)
		readsCompleted, _ := strconv.ParseUint(fields[3], 10, 64)
		metrics = append(metrics,
			models.NewMetric("disk.read_ops_total", float64(readsCompleted), c.deviceID).
				WithTag("device", device))

		// Sectors read (field 5) -> convert to bytes
		// Sectors are always in 512-byte units per kernel docs
		sectorsRead, _ := strconv.ParseUint(fields[5], 10, 64)
		readBytes := sectorsRead * 512
		metrics = append(metrics,
			models.NewMetric("disk.read_bytes_total", float64(readBytes), c.deviceID).
				WithTag("device", device))

		// Writes completed (field 7)
		writesCompleted, _ := strconv.ParseUint(fields[7], 10, 64)
		metrics = append(metrics,
			models.NewMetric("disk.write_ops_total", float64(writesCompleted), c.deviceID).
				WithTag("device", device))

		// Sectors written (field 9) -> convert to bytes
		// Sectors are always in 512-byte units per kernel docs
		sectorsWritten, _ := strconv.ParseUint(fields[9], 10, 64)
		writeBytes := sectorsWritten * 512
		metrics = append(metrics,
			models.NewMetric("disk.write_bytes_total", float64(writeBytes), c.deviceID).
				WithTag("device", device))

		// Time spent reading (field 6, in milliseconds)
		timeReading, _ := strconv.ParseUint(fields[6], 10, 64)
		metrics = append(metrics,
			models.NewMetric("disk.read_time_ms_total", float64(timeReading), c.deviceID).
				WithTag("device", device))

		// Time spent writing (field 10, in milliseconds)
		timeWriting, _ := strconv.ParseUint(fields[10], 10, 64)
		metrics = append(metrics,
			models.NewMetric("disk.write_time_ms_total", float64(timeWriting), c.deviceID).
				WithTag("device", device))

		// IOs currently in progress (field 11, gauge not counter)
		iosInProgress, _ := strconv.ParseUint(fields[11], 10, 64)
		metrics = append(metrics,
			models.NewMetric("disk.io_in_progress", float64(iosInProgress), c.deviceID).
				WithTag("device", device))

		// Weighted time doing I/O (field 13, in milliseconds)
		// This accounts for parallel operations
		weightedTime, _ := strconv.ParseUint(fields[13], 10, 64)
		metrics = append(metrics,
			models.NewMetric("disk.io_time_weighted_ms_total", float64(weightedTime), c.deviceID).
				WithTag("device", device))
	}

	return metrics, nil
}
