package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Device     DeviceConfig     `yaml:"device"`
	Storage    StorageConfig    `yaml:"storage"`
	Remote     RemoteConfig     `yaml:"remote"`
	Monitoring MonitoringConfig `yaml:"monitoring"`
	Metrics    []MetricConfig   `yaml:"metrics"`
}

// DeviceConfig contains device identification
type DeviceConfig struct {
	ID string `yaml:"id"`
}

// StorageConfig contains local storage settings
type StorageConfig struct {
	Path string `yaml:"path"`
}

// RemoteConfig contains remote endpoint settings
type RemoteConfig struct {
	URL               string `yaml:"url"`
	Enabled           bool   `yaml:"enabled"`
	UploadIntervalStr string `yaml:"upload_interval"`
	AuthToken         string `yaml:"auth_token"` // Bearer token for authentication
}

// MonitoringConfig contains monitoring and health check settings
type MonitoringConfig struct {
	ClockSkewURL string `yaml:"clock_skew_url"` // URL for clock skew detection (e.g., http://localhost:8428/health)
}

// UploadInterval parses the upload interval string to time.Duration
// Returns default of 30s if not configured
func (r *RemoteConfig) UploadInterval() time.Duration {
	if r.UploadIntervalStr == "" {
		return 30 * time.Second
	}
	duration, err := time.ParseDuration(r.UploadIntervalStr)
	if err != nil {
		return 30 * time.Second
	}
	return duration
}

// MetricConfig defines a metric to collect
type MetricConfig struct {
	Name     string `yaml:"name"`
	Interval string `yaml:"interval"`
	Enabled  bool   `yaml:"enabled"`
}

// IntervalDuration parses the interval string to time.Duration
func (m *MetricConfig) IntervalDuration() (time.Duration, error) {
	return time.ParseDuration(m.Interval)
}

// Load reads and parses a YAML configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Device.ID == "" {
		return fmt.Errorf("device.id is required")
	}
	if c.Storage.Path == "" {
		return fmt.Errorf("storage.path is required")
	}
	if c.Remote.Enabled && c.Remote.URL == "" {
		return fmt.Errorf("remote.url is required when remote is enabled")
	}

	// Validate metric intervals
	for _, m := range c.Metrics {
		if m.Enabled {
			if _, err := m.IntervalDuration(); err != nil {
				return fmt.Errorf("invalid interval for metric %s: %w", m.Name, err)
			}
		}
	}

	return nil
}

// EnabledMetrics returns only the enabled metrics
func (c *Config) EnabledMetrics() []MetricConfig {
	var enabled []MetricConfig
	for _, m := range c.Metrics {
		if m.Enabled {
			enabled = append(enabled, m)
		}
	}
	return enabled
}
