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
	Logging    LoggingConfig    `yaml:"logging"`
	Metrics    []MetricConfig   `yaml:"metrics"`
}

// DeviceConfig contains device identification
type DeviceConfig struct {
	ID string `yaml:"id"`
}

// StorageConfig contains local storage settings
type StorageConfig struct {
	Path                      string `yaml:"path"`
	WALCheckpointIntervalStr  string `yaml:"wal_checkpoint_interval"`  // How often to checkpoint WAL (default: 1h)
	WALCheckpointSizeMB       int    `yaml:"wal_checkpoint_size_mb"`   // Checkpoint when WAL exceeds this size (default: 64)
}

// WALCheckpointInterval parses the checkpoint interval string to time.Duration
// Returns default of 1 hour if not configured or if duration is non-positive
// Non-positive durations are rejected to prevent panic in time.NewTicker
func (s *StorageConfig) WALCheckpointInterval() time.Duration {
	if s.WALCheckpointIntervalStr == "" {
		return 1 * time.Hour
	}
	duration, err := time.ParseDuration(s.WALCheckpointIntervalStr)
	if err != nil {
		return 1 * time.Hour
	}
	// Guard against non-positive intervals to prevent panic in time.NewTicker
	if duration <= 0 {
		return 1 * time.Hour
	}
	return duration
}

// WALCheckpointSizeBytes returns the checkpoint size threshold in bytes
// Returns default of 64 MB if not configured
func (s *StorageConfig) WALCheckpointSizeBytes() int64 {
	if s.WALCheckpointSizeMB <= 0 {
		return 64 * 1024 * 1024 // Default: 64 MB
	}
	return int64(s.WALCheckpointSizeMB) * 1024 * 1024
}

// RetryConfig contains retry settings for uploads
type RetryConfig struct {
	Enabled            *bool   `yaml:"enabled"`        // Pointer to distinguish "not set" from "explicitly false"
	MaxAttempts        int     `yaml:"max_attempts"`
	InitialBackoffStr  string  `yaml:"initial_backoff"`
	MaxBackoffStr      string  `yaml:"max_backoff"`
	BackoffMultiplier  float64 `yaml:"backoff_multiplier"`
	JitterPercent      *int    `yaml:"jitter_percent"` // Pointer to distinguish "not set" from "explicitly 0"
}

// InitialBackoff parses the initial backoff string to time.Duration
func (r *RetryConfig) InitialBackoff() time.Duration {
	if r.InitialBackoffStr == "" {
		return 1 * time.Second
	}
	duration, err := time.ParseDuration(r.InitialBackoffStr)
	if err != nil {
		return 1 * time.Second
	}
	return duration
}

// MaxBackoff parses the max backoff string to time.Duration
func (r *RetryConfig) MaxBackoff() time.Duration {
	if r.MaxBackoffStr == "" {
		return 30 * time.Second
	}
	duration, err := time.ParseDuration(r.MaxBackoffStr)
	if err != nil {
		return 30 * time.Second
	}
	return duration
}

// RemoteConfig contains remote endpoint settings
type RemoteConfig struct {
	URL               string       `yaml:"url"`
	Enabled           bool         `yaml:"enabled"`
	UploadIntervalStr string       `yaml:"upload_interval"`
	AuthToken         string       `yaml:"auth_token"` // Bearer token for authentication
	BatchSize         int          `yaml:"batch_size"` // Max metrics per batch query (default: 2500)
	ChunkSize         int          `yaml:"chunk_size"` // Metrics per chunk upload (default: 50)
	Retry             RetryConfig  `yaml:"retry"`      // Retry configuration
}

// GetBatchSize returns the batch size or default
func (r *RemoteConfig) GetBatchSize() int {
	if r.BatchSize <= 0 {
		return 2500 // Default
	}
	return r.BatchSize
}

// GetChunkSize returns the chunk size or default
func (r *RemoteConfig) GetChunkSize() int {
	if r.ChunkSize <= 0 {
		return 50 // Default
	}
	return r.ChunkSize
}

// MonitoringConfig contains monitoring and health check settings
type MonitoringConfig struct {
	ClockSkewURL               string `yaml:"clock_skew_url"`                 // URL for clock skew detection (e.g., http://localhost:8428/health)
	ClockSkewCheckInterval     string `yaml:"clock_skew_check_interval"`      // How often to check clock skew (default: 5m)
	ClockSkewWarnThresholdMs   int    `yaml:"clock_skew_warn_threshold_ms"`   // Warn when skew exceeds this (default: 2000ms)
	HealthAddress              string `yaml:"health_address"`                 // Address for health endpoint server (e.g., ":9100")
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error (default: info)
	Format string `yaml:"format"` // json, console (default: console)
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
