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

// SystemCollector collects system metrics (CPU temperature, usage, memory)
type SystemCollector struct {
	deviceID string
}

// NewSystemCollector creates a new system metrics collector
func NewSystemCollector(deviceID string) *SystemCollector {
	return &SystemCollector{
		deviceID: deviceID,
	}
}

// Name returns the collector name
func (c *SystemCollector) Name() string {
	return "system"
}

// Collect gathers system metrics
func (c *SystemCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	var metrics []*models.Metric

	// CPU Temperature (real on Linux, mock on macOS)
	temp, err := c.getCPUTemperature()
	if err == nil {
		m := models.NewMetric("cpu.temperature", temp, c.deviceID)
		metrics = append(metrics, m)
	}

	// Bonus: All thermal zones (if available)
	if runtime.GOOS == "linux" {
		zones, _ := c.getAllThermalZones()
		metrics = append(metrics, zones...)
	}

	return metrics, nil
}

// getCPUTemperature reads CPU temperature
// Real on Linux (from /sys/class/thermal), mock on macOS
func (c *SystemCollector) getCPUTemperature() (float64, error) {
	if runtime.GOOS == "darwin" {
		// Mock temperature on macOS
		return 45.0 + float64(os.Getpid()%10), nil
	}

	// Real temperature on Linux (Orange Pi / RK3588)
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, fmt.Errorf("failed to read temperature: %w", err)
	}

	millideg, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse temperature: %w", err)
	}

	return float64(millideg) / 1000.0, nil
}

// getAllThermalZones reads all thermal zones (Linux only, bonus feature)
func (c *SystemCollector) getAllThermalZones() ([]*models.Metric, error) {
	var metrics []*models.Metric

	entries, err := os.ReadDir("/sys/class/thermal")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}

		zonePath := "/sys/class/thermal/" + entry.Name()

		// Read zone type
		typeData, err := os.ReadFile(zonePath + "/type")
		if err != nil {
			continue
		}
		zoneType := strings.TrimSpace(string(typeData))

		// Read temperature
		tempData, err := os.ReadFile(zonePath + "/temp")
		if err != nil {
			continue
		}

		millideg, err := strconv.ParseInt(strings.TrimSpace(string(tempData)), 10, 64)
		if err != nil {
			continue
		}

		temp := float64(millideg) / 1000.0

		m := models.NewMetric("thermal.zone_temp", temp, c.deviceID).
			WithTag("zone", zoneType).
			WithTag("zone_number", entry.Name())

		metrics = append(metrics, m)
	}

	return metrics, nil
}
