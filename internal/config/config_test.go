package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// boolPtr is a helper function to create a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// intPtr is a helper function to create a pointer to an int value
func intPtr(i int) *int {
	return &i
}

func TestLoadConfig(t *testing.T) {
	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8080/api/metrics
  enabled: true

metrics:
  - name: cpu.temperature
    interval: 30s
    enabled: true
  - name: srt.packet_loss
    interval: 5s
    enabled: true
  - name: disabled.metric
    interval: 10s
    enabled: false
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test device config
	if cfg.Device.ID != "test-device-001" {
		t.Errorf("Expected device ID test-device-001, got %s", cfg.Device.ID)
	}

	// Test storage config
	if cfg.Storage.Path != "/tmp/test.db" {
		t.Errorf("Expected storage path /tmp/test.db, got %s", cfg.Storage.Path)
	}

	// Test remote config
	if !cfg.Remote.Enabled {
		t.Error("Expected remote to be enabled")
	}
	if cfg.Remote.URL != "http://localhost:8080/api/metrics" {
		t.Errorf("Expected remote URL http://localhost:8080/api/metrics, got %s", cfg.Remote.URL)
	}

	// Test metrics
	if len(cfg.Metrics) != 3 {
		t.Errorf("Expected 3 metrics, got %d", len(cfg.Metrics))
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: "/tmp/test.db"},
				Remote:  RemoteConfig{URL: "http://localhost", Enabled: true},
				Metrics: []MetricConfig{
					{Name: "test", Interval: "30s", Enabled: true},
				},
			},
			expectError: false,
		},
		{
			name: "missing device ID",
			config: Config{
				Device:  DeviceConfig{ID: ""},
				Storage: StorageConfig{Path: "/tmp/test.db"},
			},
			expectError: true,
			errorMsg:    "device.id is required",
		},
		{
			name: "missing storage path",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: ""},
			},
			expectError: true,
			errorMsg:    "storage.path is required",
		},
		{
			name: "remote enabled but no URL",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: "/tmp/test.db"},
				Remote:  RemoteConfig{URL: "", Enabled: true},
			},
			expectError: true,
			errorMsg:    "remote.url is required",
		},
		{
			name: "invalid interval",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: "/tmp/test.db"},
				Metrics: []MetricConfig{
					{Name: "test", Interval: "invalid", Enabled: true},
				},
			},
			expectError: true,
			errorMsg:    "invalid interval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestMetricConfigInterval(t *testing.T) {
	m := MetricConfig{Name: "test", Interval: "30s", Enabled: true}

	duration, err := m.IntervalDuration()
	if err != nil {
		t.Fatalf("Failed to parse interval: %v", err)
	}

	expected := 30 * time.Second
	if duration != expected {
		t.Errorf("Expected duration %v, got %v", expected, duration)
	}
}

func TestEnabledMetrics(t *testing.T) {
	cfg := Config{
		Metrics: []MetricConfig{
			{Name: "metric1", Interval: "10s", Enabled: true},
			{Name: "metric2", Interval: "20s", Enabled: false},
			{Name: "metric3", Interval: "30s", Enabled: true},
		},
	}

	enabled := cfg.EnabledMetrics()
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled metrics, got %d", len(enabled))
	}

	if enabled[0].Name != "metric1" || enabled[1].Name != "metric3" {
		t.Error("Enabled metrics not filtered correctly")
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

// TestWALCheckpointInterval tests parsing of WAL checkpoint interval configuration
func TestWALCheckpointInterval(t *testing.T) {
	tests := []struct {
		name     string
		config   StorageConfig
		expected time.Duration
	}{
		{
			name: "configured interval",
			config: StorageConfig{
				Path:                     "/tmp/test.db",
				WALCheckpointIntervalStr: "30m",
			},
			expected: 30 * time.Minute,
		},
		{
			name: "default interval when empty",
			config: StorageConfig{
				Path:                     "/tmp/test.db",
				WALCheckpointIntervalStr: "",
			},
			expected: 1 * time.Hour,
		},
		{
			name: "invalid interval uses default",
			config: StorageConfig{
				Path:                     "/tmp/test.db",
				WALCheckpointIntervalStr: "invalid",
			},
			expected: 1 * time.Hour,
		},
		{
			name: "negative interval uses default",
			config: StorageConfig{
				Path:                     "/tmp/test.db",
				WALCheckpointIntervalStr: "-1s",
			},
			expected: 1 * time.Hour,
		},
		{
			name: "zero interval uses default",
			config: StorageConfig{
				Path:                     "/tmp/test.db",
				WALCheckpointIntervalStr: "0s",
			},
			expected: 1 * time.Hour,
		},
		{
			name: "negative 10 minutes uses default",
			config: StorageConfig{
				Path:                     "/tmp/test.db",
				WALCheckpointIntervalStr: "-10m",
			},
			expected: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.config.WALCheckpointInterval()

			// Negative and zero values should now return errors
			if strings.Contains(tt.name, "negative") || strings.Contains(tt.name, "zero") {
				if err == nil {
					t.Error("Expected error for non-positive duration, got none")
				}
				return
			}

			// Invalid values should return error
			if strings.Contains(tt.name, "invalid") {
				if err == nil {
					t.Error("Expected error for invalid duration string, got none")
				}
				return
			}

			// Valid values should not error
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
			// Critical: Verify we can create a ticker without panic
			// This would panic if interval <= 0
			ticker := time.NewTicker(result)
			ticker.Stop()
		})
	}
}

// TestWALCheckpointSizeBytes tests parsing of WAL checkpoint size configuration
func TestWALCheckpointSizeBytes(t *testing.T) {
	tests := []struct {
		name     string
		config   StorageConfig
		expected int64
	}{
		{
			name: "configured size",
			config: StorageConfig{
				Path:                "/tmp/test.db",
				WALCheckpointSizeMB: 128,
			},
			expected: 128 * 1024 * 1024,
		},
		{
			name: "default size when zero",
			config: StorageConfig{
				Path:                "/tmp/test.db",
				WALCheckpointSizeMB: 0,
			},
			expected: 64 * 1024 * 1024, // Default 64 MB
		},
		{
			name: "default size when negative",
			config: StorageConfig{
				Path:                "/tmp/test.db",
				WALCheckpointSizeMB: -1,
			},
			expected: 64 * 1024 * 1024, // Default 64 MB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.WALCheckpointSizeBytes()
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestRetryConfigParsing tests parsing of retry configuration
func TestRetryConfigParsing(t *testing.T) {
	tests := []struct {
		name            string
		config          RetryConfig
		expectedInitial time.Duration
		expectedMax     time.Duration
	}{
		{
			name: "configured backoff values",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       5,
				InitialBackoffStr: "2s",
				MaxBackoffStr:     "60s",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 2 * time.Second,
			expectedMax:     60 * time.Second,
		},
		{
			name: "default values when empty",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       3,
				InitialBackoffStr: "",
				MaxBackoffStr:     "",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 1 * time.Second,
			expectedMax:     30 * time.Second,
		},
		{
			name: "invalid values use defaults",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       3,
				InitialBackoffStr: "invalid",
				MaxBackoffStr:     "invalid",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 1 * time.Second,
			expectedMax:     30 * time.Second,
		},
		{
			name: "negative initial backoff uses default",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       3,
				InitialBackoffStr: "-1s",
				MaxBackoffStr:     "60s",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 1 * time.Second,
			expectedMax:     60 * time.Second,
		},
		{
			name: "negative max backoff uses default",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       3,
				InitialBackoffStr: "2s",
				MaxBackoffStr:     "-30s",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 2 * time.Second,
			expectedMax:     30 * time.Second,
		},
		{
			name: "zero initial backoff uses default",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       3,
				InitialBackoffStr: "0s",
				MaxBackoffStr:     "60s",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 1 * time.Second,
			expectedMax:     60 * time.Second,
		},
		{
			name: "zero max backoff uses default",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       3,
				InitialBackoffStr: "2s",
				MaxBackoffStr:     "0s",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 2 * time.Second,
			expectedMax:     30 * time.Second,
		},
		{
			name: "both negative backoffs use defaults",
			config: RetryConfig{
				Enabled:           boolPtr(true),
				MaxAttempts:       3,
				InitialBackoffStr: "-5s",
				MaxBackoffStr:     "-60s",
				BackoffMultiplier: 2.0,
				JitterPercent:     intPtr(10),
			},
			expectedInitial: 1 * time.Second,
			expectedMax:     30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initial, initialErr := tt.config.InitialBackoff()
			max, maxErr := tt.config.MaxBackoff()

			// Negative, zero, and invalid values should return errors
			expectInitialError := strings.Contains(tt.name, "negative initial") ||
				strings.Contains(tt.name, "zero initial") ||
				strings.Contains(tt.name, "both negative") ||
				strings.Contains(tt.name, "invalid")
			expectMaxError := strings.Contains(tt.name, "negative max") ||
				strings.Contains(tt.name, "zero max") ||
				strings.Contains(tt.name, "both negative") ||
				strings.Contains(tt.name, "invalid")

			if expectInitialError {
				if initialErr == nil {
					t.Error("InitialBackoff: expected error for invalid/non-positive duration, got none")
				}
			} else {
				if initialErr != nil {
					t.Errorf("InitialBackoff: unexpected error: %v", initialErr)
				}
				if initial != tt.expectedInitial {
					t.Errorf("InitialBackoff: expected %v, got %v", tt.expectedInitial, initial)
				}
				// Critical: Verify that returned durations are positive
				if initial <= 0 {
					t.Errorf("InitialBackoff returned non-positive duration: %v", initial)
				}
			}

			if expectMaxError {
				if maxErr == nil {
					t.Error("MaxBackoff: expected error for invalid/non-positive duration, got none")
				}
			} else {
				if maxErr != nil {
					t.Errorf("MaxBackoff: unexpected error: %v", maxErr)
				}
				if max != tt.expectedMax {
					t.Errorf("MaxBackoff: expected %v, got %v", tt.expectedMax, max)
				}
				// Critical: Verify that returned durations are positive
				if max <= 0 {
					t.Errorf("MaxBackoff returned non-positive duration: %v", max)
				}
			}
		})
	}
}

// TestRemoteConfigBatchSize tests batch size configuration
func TestRemoteConfigBatchSize(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
		expected  int
	}{
		{
			name:      "configured batch size",
			batchSize: 5000,
			expected:  5000,
		},
		{
			name:      "default when zero",
			batchSize: 0,
			expected:  2500, // Default
		},
		{
			name:      "default when negative",
			batchSize: -1,
			expected:  2500, // Default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := RemoteConfig{
				BatchSize: tt.batchSize,
			}
			result := cfg.GetBatchSize()
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestRemoteConfigChunkSize tests chunk size configuration
func TestRemoteConfigChunkSize(t *testing.T) {
	tests := []struct {
		name      string
		chunkSize int
		expected  int
	}{
		{
			name:      "configured chunk size",
			chunkSize: 100,
			expected:  100,
		},
		{
			name:      "default when zero",
			chunkSize: 0,
			expected:  50, // Default
		},
		{
			name:      "default when negative",
			chunkSize: -1,
			expected:  50, // Default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := RemoteConfig{
				ChunkSize: tt.chunkSize,
			}
			result := cfg.GetChunkSize()
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestLoadConfigWithAllFields tests loading a complete config with all M2 fields
func TestLoadConfigWithAllFields(t *testing.T) {
	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db
  wal_checkpoint_interval: 30m
  wal_checkpoint_size_mb: 128

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  upload_interval: 60s
  auth_token: test-token-123
  batch_size: 5000
  chunk_size: 100
  retry:
    enabled: true
    max_attempts: 5
    initial_backoff: 2s
    max_backoff: 60s
    backoff_multiplier: 2.0
    jitter_percent: 10

monitoring:
  clock_skew_url: http://localhost:8428/health
  clock_skew_check_interval: 5m
  clock_skew_warn_threshold_ms: 3000
  health_address: ":9100"

logging:
  level: debug
  format: json

metrics:
  - name: cpu.usage
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test WAL checkpoint config
	if cfg.Storage.WALCheckpointIntervalStr != "30m" {
		t.Errorf("Expected WAL interval 30m, got %s", cfg.Storage.WALCheckpointIntervalStr)
	}
	if cfg.Storage.WALCheckpointSizeMB != 128 {
		t.Errorf("Expected WAL size 128, got %d", cfg.Storage.WALCheckpointSizeMB)
	}
	interval, err := cfg.Storage.WALCheckpointInterval()
	if err != nil {
		t.Fatalf("Expected valid interval, got error: %v", err)
	}
	if interval != 30*time.Minute {
		t.Errorf("Expected parsed interval 30m, got %v", interval)
	}
	if cfg.Storage.WALCheckpointSizeBytes() != 128*1024*1024 {
		t.Errorf("Expected parsed size 128MB, got %d", cfg.Storage.WALCheckpointSizeBytes())
	}

	// Test remote config
	if cfg.Remote.AuthToken != "test-token-123" {
		t.Errorf("Expected auth token test-token-123, got %s", cfg.Remote.AuthToken)
	}
	if cfg.Remote.BatchSize != 5000 {
		t.Errorf("Expected batch size 5000, got %d", cfg.Remote.BatchSize)
	}
	if cfg.Remote.ChunkSize != 100 {
		t.Errorf("Expected chunk size 100, got %d", cfg.Remote.ChunkSize)
	}
	if cfg.Remote.GetBatchSize() != 5000 {
		t.Errorf("Expected GetBatchSize() 5000, got %d", cfg.Remote.GetBatchSize())
	}
	if cfg.Remote.GetChunkSize() != 100 {
		t.Errorf("Expected GetChunkSize() 100, got %d", cfg.Remote.GetChunkSize())
	}

	// Test retry config
	if cfg.Remote.Retry.Enabled == nil || !*cfg.Remote.Retry.Enabled {
		t.Error("Expected retry enabled")
	}
	if cfg.Remote.Retry.MaxAttempts != 5 {
		t.Errorf("Expected max attempts 5, got %d", cfg.Remote.Retry.MaxAttempts)
	}
	initialBackoff, err := cfg.Remote.Retry.InitialBackoff()
	if err != nil {
		t.Fatalf("Expected valid initial_backoff, got error: %v", err)
	}
	if initialBackoff != 2*time.Second {
		t.Errorf("Expected initial backoff 2s, got %v", initialBackoff)
	}
	maxBackoff, err := cfg.Remote.Retry.MaxBackoff()
	if err != nil {
		t.Fatalf("Expected valid max_backoff, got error: %v", err)
	}
	if maxBackoff != 60*time.Second {
		t.Errorf("Expected max backoff 60s, got %v", maxBackoff)
	}

	// Test monitoring config
	if cfg.Monitoring.ClockSkewCheckInterval != "5m" {
		t.Errorf("Expected clock skew interval 5m, got %s", cfg.Monitoring.ClockSkewCheckInterval)
	}
	if cfg.Monitoring.ClockSkewWarnThresholdMs != 3000 {
		t.Errorf("Expected clock skew threshold 3000ms, got %d", cfg.Monitoring.ClockSkewWarnThresholdMs)
	}
}

// TestRetryDisabled tests that retry.enabled: false is properly parsed
func TestRetryDisabled(t *testing.T) {
	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  retry:
    enabled: false

metrics:
  - name: cpu.usage
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify retry is explicitly disabled
	if cfg.Remote.Retry.Enabled == nil {
		t.Fatal("Expected retry.enabled to be non-nil (explicitly set)")
	}
	if *cfg.Remote.Retry.Enabled {
		t.Error("Expected retry.enabled to be false")
	}
}

// TestRetryEnabledWithCustomValues tests retry configuration with custom values
func TestRetryEnabledWithCustomValues(t *testing.T) {
	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  retry:
    enabled: true
    max_attempts: 5
    initial_backoff: 2s
    max_backoff: 60s
    backoff_multiplier: 2.0
    jitter_percent: 10

metrics:
  - name: cpu.usage
    interval: 10s
    enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify retry is enabled with custom values
	if cfg.Remote.Retry.Enabled == nil {
		t.Fatal("Expected retry.enabled to be non-nil (explicitly set)")
	}
	if !*cfg.Remote.Retry.Enabled {
		t.Error("Expected retry.enabled to be true")
	}
	if cfg.Remote.Retry.MaxAttempts != 5 {
		t.Errorf("Expected max_attempts 5, got %d", cfg.Remote.Retry.MaxAttempts)
	}
	initialBackoff2, err := cfg.Remote.Retry.InitialBackoff()
	if err != nil {
		t.Fatalf("Expected valid initial_backoff, got error: %v", err)
	}
	if initialBackoff2 != 2*time.Second {
		t.Errorf("Expected initial_backoff 2s, got %v", initialBackoff2)
	}
	maxBackoff2, err := cfg.Remote.Retry.MaxBackoff()
	if err != nil {
		t.Fatalf("Expected valid max_backoff, got error: %v", err)
	}
	if maxBackoff2 != 60*time.Second {
		t.Errorf("Expected max_backoff 60s, got %v", maxBackoff2)
	}
	if cfg.Remote.Retry.BackoffMultiplier != 2.0 {
		t.Errorf("Expected backoff_multiplier 2.0, got %f", cfg.Remote.Retry.BackoffMultiplier)
	}
	if cfg.Remote.Retry.JitterPercent == nil || *cfg.Remote.Retry.JitterPercent != 10 {
		var val int
		if cfg.Remote.Retry.JitterPercent != nil {
			val = *cfg.Remote.Retry.JitterPercent
		}
		t.Errorf("Expected jitter_percent 10, got %d", val)
	}
}

// TestAuthTokenFile tests loading auth token from a file
func TestAuthTokenFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create token file
	tokenPath := filepath.Join(tmpDir, "token.txt")
	tokenContent := "my-secret-token-12345\n"
	if err := os.WriteFile(tokenPath, []byte(tokenContent), 0600); err != nil {
		t.Fatalf("Failed to write token file: %v", err)
	}

	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  auth_token_file: ` + tokenPath + `

metrics:
  - name: cpu.usage
    interval: 10s
    enabled: true
`

	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify token was loaded and trimmed
	expectedToken := "my-secret-token-12345"
	if cfg.Remote.AuthToken != expectedToken {
		t.Errorf("Expected auth token '%s', got '%s'", expectedToken, cfg.Remote.AuthToken)
	}
}

// TestAuthTokenFileMutualExclusivity tests that both auth_token and auth_token_file cannot be specified
func TestAuthTokenFileMutualExclusivity(t *testing.T) {
	tmpDir := t.TempDir()

	// Create token file
	tokenPath := filepath.Join(tmpDir, "token.txt")
	if err := os.WriteFile(tokenPath, []byte("token-from-file"), 0600); err != nil {
		t.Fatalf("Failed to write token file: %v", err)
	}

	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  auth_token: inline-token
  auth_token_file: ` + tokenPath + `

metrics:
  - name: cpu.usage
    interval: 10s
    enabled: true
`

	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Expected error when both auth_token and auth_token_file are specified")
	}

	expectedError := "cannot specify both remote.auth_token and remote.auth_token_file"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// TestAuthTokenFileNotFound tests error when token file doesn't exist
func TestAuthTokenFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  auth_token_file: /nonexistent/token.txt

metrics:
  - name: cpu.usage
    interval: 10s
    enabled: true
`

	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Expected error when token file doesn't exist")
	}
}

// TestAuthTokenFileEmpty tests error when token file is empty
func TestAuthTokenFileEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty token file
	tokenPath := filepath.Join(tmpDir, "token.txt")
	if err := os.WriteFile(tokenPath, []byte(""), 0600); err != nil {
		t.Fatalf("Failed to write token file: %v", err)
	}

	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  auth_token_file: ` + tokenPath + `

metrics:
  - name: cpu.usage
    interval: 10s
    enabled: true
`

	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Expected error when token file is empty")
	}
}

// TestUploadInterval tests parsing of upload interval configuration
func TestUploadInterval(t *testing.T) {
	tests := []struct {
		name     string
		config   RemoteConfig
		expected time.Duration
	}{
		{
			name: "configured interval",
			config: RemoteConfig{
				UploadIntervalStr: "60s",
			},
			expected: 60 * time.Second,
		},
		{
			name: "default interval when empty",
			config: RemoteConfig{
				UploadIntervalStr: "",
			},
			expected: 30 * time.Second,
		},
		{
			name: "invalid interval uses default",
			config: RemoteConfig{
				UploadIntervalStr: "invalid",
			},
			expected: 30 * time.Second,
		},
		{
			name: "negative interval uses default",
			config: RemoteConfig{
				UploadIntervalStr: "-1s",
			},
			expected: 30 * time.Second,
		},
		{
			name: "zero interval uses default",
			config: RemoteConfig{
				UploadIntervalStr: "0s",
			},
			expected: 30 * time.Second,
		},
		{
			name: "negative 30 seconds uses default",
			config: RemoteConfig{
				UploadIntervalStr: "-30s",
			},
			expected: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.config.UploadInterval()

			// Negative, zero, and invalid values should now return errors
			if strings.Contains(tt.name, "negative") || strings.Contains(tt.name, "zero") || strings.Contains(tt.name, "invalid") {
				if err == nil {
					t.Error("Expected error for invalid/non-positive duration, got none")
				}
				return
			}

			// Valid values should not error
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
			// Critical: Verify we can create a ticker without panic
			// This would panic if interval <= 0
			ticker := time.NewTicker(result)
			ticker.Stop()
		})
	}
}

// TestInvalidTimingValuesFailValidation tests that invalid timing values cause config.Load() to fail
func TestInvalidTimingValuesFailValidation(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		yamlContent string
		errorMsg    string
	}{
		{
			name: "negative WAL checkpoint interval",
			yamlContent: `
device:
  id: test-device
storage:
  path: /tmp/test.db
  wal_checkpoint_interval: -1h
remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`,
			errorMsg: "wal_checkpoint_interval must be positive",
		},
		{
			name: "invalid upload interval",
			yamlContent: `
device:
  id: test-device
storage:
  path: /tmp/test.db
remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  upload_interval: not-a-duration
metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`,
			errorMsg: "invalid remote.upload_interval",
		},
		{
			name: "negative initial backoff",
			yamlContent: `
device:
  id: test-device
storage:
  path: /tmp/test.db
remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  retry:
    initial_backoff: -5s
metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`,
			errorMsg: "retry.initial_backoff must be positive",
		},
		{
			name: "zero max backoff",
			yamlContent: `
device:
  id: test-device
storage:
  path: /tmp/test.db
remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  retry:
    max_backoff: 0s
metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`,
			errorMsg: "retry.max_backoff must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tt.name+".yaml")
			if err := os.WriteFile(configPath, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			_, err := Load(configPath)
			if err == nil {
				t.Fatal("Expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
			}
		})
	}
}

// TestInvalidTimingValuesCheckedEvenWhenDisabled tests that invalid timing values
// are caught during validation even when the feature is disabled (e.g., remote.enabled: false).
// This prevents runtime crashes when code calls timing methods before checking enabled flags.
func TestInvalidTimingValuesCheckedEvenWhenDisabled(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		yamlContent string
		errorMsg    string
	}{
		{
			name: "disabled remote with zero upload interval",
			yamlContent: `
device:
  id: test-device
storage:
  path: /tmp/test.db
remote:
  url: http://localhost:8428/api/v1/import
  enabled: false
  upload_interval: 0s
metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`,
			errorMsg: "remote.upload_interval must be positive",
		},
		{
			name: "disabled remote with negative upload interval",
			yamlContent: `
device:
  id: test-device
storage:
  path: /tmp/test.db
remote:
  enabled: false
  upload_interval: -30s
metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`,
			errorMsg: "remote.upload_interval must be positive",
		},
		{
			name: "disabled remote with invalid retry timing",
			yamlContent: `
device:
  id: test-device
storage:
  path: /tmp/test.db
remote:
  enabled: false
  retry:
    initial_backoff: 0s
metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`,
			errorMsg: "retry.initial_backoff must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tt.name+".yaml")
			if err := os.WriteFile(configPath, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			_, err := Load(configPath)
			if err == nil {
				t.Fatal("Expected error but got none - invalid timing value should be caught during validation")
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
			}
		})
	}
}

// TestMetricIntervalValidation tests that zero/negative metric intervals are rejected during validation
func TestMetricIntervalValidation(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		interval    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid interval",
			interval:    "30s",
			expectError: false,
		},
		{
			name:        "zero interval",
			interval:    "0s",
			expectError: true,
			errorMsg:    "interval must be positive",
		},
		{
			name:        "negative interval",
			interval:    "-10s",
			expectError: true,
			errorMsg:    "interval must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true

metrics:
  - name: cpu.usage
    interval: ` + tt.interval + `
    enabled: true
`

			configPath := filepath.Join(tmpDir, tt.name+".yaml")
			if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			_, err := Load(configPath)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestInvalidBackoffMultiplierValidation tests that backoff_multiplier < 1.0
// is rejected during validation to prevent math.Pow from yielding zero/negative delays.
func TestInvalidBackoffMultiplierValidation(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		multiplier  float64
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid multiplier 2.0",
			multiplier:  2.0,
			expectError: false,
		},
		{
			name:        "valid multiplier 1.0 (minimum)",
			multiplier:  1.0,
			expectError: false,
		},
		{
			name:        "valid multiplier 1.5",
			multiplier:  1.5,
			expectError: false,
		},
		{
			name:        "zero multiplier (treated as not configured)",
			multiplier:  0,
			expectError: false,
		},
		{
			name:        "negative multiplier",
			multiplier:  -1.0,
			expectError: true,
			errorMsg:    "backoff_multiplier must be >= 1.0",
		},
		{
			name:        "fractional multiplier less than 1",
			multiplier:  0.5,
			expectError: true,
			errorMsg:    "backoff_multiplier must be >= 1.0",
		},
		{
			name:        "very small positive multiplier",
			multiplier:  0.1,
			expectError: true,
			errorMsg:    "backoff_multiplier must be >= 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  retry:
    backoff_multiplier: ` + strings.TrimSpace(strings.TrimRight(fmt.Sprintf("%v", tt.multiplier), "0")) + `

metrics:
  - name: cpu.usage
    interval: 30s
    enabled: true
`

			configPath := filepath.Join(tmpDir, tt.name+".yaml")
			if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			_, err := Load(configPath)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none - invalid backoff_multiplier should be caught during validation")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
