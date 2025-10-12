package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		logLevel      Level
		shouldWrite   bool
	}{
		{
			name: "json format info level",
			config: Config{
				Level:  LevelInfo,
				Format: FormatJSON,
			},
			logLevel:    LevelInfo,
			shouldWrite: true,
		},
		{
			name: "console format debug level",
			config: Config{
				Level:  LevelDebug,
				Format: FormatConsole,
			},
			logLevel:    LevelInfo,
			shouldWrite: true,
		},
		{
			name: "console format warn level",
			config: Config{
				Level:  LevelWarn,
				Format: FormatConsole,
			},
			logLevel:    LevelWarn,
			shouldWrite: true,
		},
		{
			name: "console format error level",
			config: Config{
				Level:  LevelError,
				Format: FormatConsole,
			},
			logLevel:    LevelError,
			shouldWrite: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.config.Output = &buf
			logger := New(tt.config)
			if logger == nil {
				t.Fatal("New() returned nil")
			}

			// Test that logger can write at appropriate level
			switch tt.logLevel {
			case LevelDebug:
				logger.Debug("test message")
			case LevelInfo:
				logger.Info("test message")
			case LevelWarn:
				logger.Warn("test message")
			case LevelError:
				logger.Error("test message")
			}

			if tt.shouldWrite && buf.Len() == 0 {
				t.Error("Logger did not write any output")
			}
		})
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: &buf,
	}
	logger := New(cfg)

	logger.Info("test message",
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	)

	// Parse JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// Check required fields
	if logEntry["msg"] != "test message" {
		t.Errorf("Expected msg='test message', got %v", logEntry["msg"])
	}
	if logEntry["level"] != "INFO" {
		t.Errorf("Expected level='INFO', got %v", logEntry["level"])
	}
	if logEntry["key1"] != "value1" {
		t.Errorf("Expected key1='value1', got %v", logEntry["key1"])
	}
	if logEntry["key2"] != float64(42) {
		t.Errorf("Expected key2=42, got %v", logEntry["key2"])
	}
}

func TestConsoleFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  LevelInfo,
		Format: FormatConsole,
		Output: &buf,
	}
	logger := New(cfg)

	logger.Info("test message",
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	)

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Console output missing message: %s", output)
	}
	if !strings.Contains(output, "INFO") {
		t.Errorf("Console output missing level: %s", output)
	}
	if !strings.Contains(output, "key1=value1") {
		t.Errorf("Console output missing key1: %s", output)
	}
	if !strings.Contains(output, "key2=42") {
		t.Errorf("Console output missing key2: %s", output)
	}
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name          string
		level         Level
		logFunc       func(*slog.Logger)
		shouldAppear  bool
	}{
		{
			name:  "debug message at info level",
			level: LevelInfo,
			logFunc: func(l *slog.Logger) {
				l.Debug("debug message")
			},
			shouldAppear: false,
		},
		{
			name:  "info message at info level",
			level: LevelInfo,
			logFunc: func(l *slog.Logger) {
				l.Info("info message")
			},
			shouldAppear: true,
		},
		{
			name:  "warn message at info level",
			level: LevelInfo,
			logFunc: func(l *slog.Logger) {
				l.Warn("warn message")
			},
			shouldAppear: true,
		},
		{
			name:  "error message at info level",
			level: LevelInfo,
			logFunc: func(l *slog.Logger) {
				l.Error("error message")
			},
			shouldAppear: true,
		},
		{
			name:  "info message at error level",
			level: LevelError,
			logFunc: func(l *slog.Logger) {
				l.Info("info message")
			},
			shouldAppear: false,
		},
		{
			name:  "error message at error level",
			level: LevelError,
			logFunc: func(l *slog.Logger) {
				l.Error("error message")
			},
			shouldAppear: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := Config{
				Level:  tt.level,
				Format: FormatConsole,
				Output: &buf,
			}
			logger := New(cfg)

			tt.logFunc(logger)

			hasOutput := buf.Len() > 0
			if hasOutput != tt.shouldAppear {
				t.Errorf("Expected shouldAppear=%v, got hasOutput=%v. Output: %s",
					tt.shouldAppear, hasOutput, buf.String())
			}
		})
	}
}

func TestCollectionAttrs(t *testing.T) {
	attrs := CollectionAttrs("cpu", 42, 123, "session-123")

	if len(attrs) != 4 {
		t.Errorf("Expected 4 attributes, got %d", len(attrs))
	}

	// Convert to map for easier checking
	attrMap := make(map[string]slog.Value)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr.Value
	}

	// Check attribute names and values
	if val, ok := attrMap["collector"]; !ok || val.String() != "cpu" {
		t.Errorf("collector: expected 'cpu', got %v", val)
	}
	if _, ok := attrMap["count"]; !ok {
		t.Error("count attribute missing")
	}
	if val, ok := attrMap["duration_ms"]; !ok || val.Int64() != int64(123) {
		t.Errorf("duration_ms: expected 123, got %v", val)
	}
	if val, ok := attrMap["session_id"]; !ok || val.String() != "session-123" {
		t.Errorf("session_id: expected 'session-123', got %v", val)
	}
}

func TestUploadAttrs(t *testing.T) {
	attrs := UploadAttrs("batch-123", 5, 2, 1000, 200, 1024, 2048)

	if len(attrs) != 7 {
		t.Errorf("Expected 7 attributes, got %d", len(attrs))
	}

	// Convert to map for easier checking
	attrMap := make(map[string]slog.Value)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr.Value
	}

	// Check each attribute
	if val, ok := attrMap["batch_id"]; !ok || val.String() != "batch-123" {
		t.Errorf("batch_id: expected 'batch-123', got %v", val)
	}
	if _, ok := attrMap["chunk_index"]; !ok {
		t.Error("chunk_index attribute missing")
	}
	if _, ok := attrMap["attempt"]; !ok {
		t.Error("attempt attribute missing")
	}
	if val, ok := attrMap["backoff_ms"]; !ok || val.Int64() != int64(1000) {
		t.Errorf("backoff_ms: expected 1000, got %v", val)
	}
	if _, ok := attrMap["http_status"]; !ok {
		t.Error("http_status attribute missing")
	}
	if val, ok := attrMap["bytes_sent"]; !ok || val.Int64() != int64(1024) {
		t.Errorf("bytes_sent: expected 1024, got %v", val)
	}
	if val, ok := attrMap["bytes_rcvd"]; !ok || val.Int64() != int64(2048) {
		t.Errorf("bytes_rcvd: expected 2048, got %v", val)
	}
}

func TestRetryAttrs(t *testing.T) {
	testErr := errors.New("test error")
	attrs := RetryAttrs(3, 5000, testErr)

	if len(attrs) < 3 {
		t.Errorf("Expected at least 3 attributes, got %d", len(attrs))
	}

	// Check that error attributes are present
	foundError := false
	foundErrorType := false
	for _, attr := range attrs {
		if attr.Key == "error" {
			foundError = true
			if attr.Value.Any() != "test error" {
				t.Errorf("Expected error='test error', got %v", attr.Value.Any())
			}
		}
		if attr.Key == "error_type" {
			foundErrorType = true
		}
	}

	if !foundError {
		t.Error("Error attribute not found")
	}
	if !foundErrorType {
		t.Error("Error type attribute not found")
	}
}

func TestErrorAttrs(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		testErr := errors.New("test error")
		attrs := ErrorAttrs(testErr)

		if len(attrs) != 2 {
			t.Errorf("Expected 2 attributes, got %d", len(attrs))
		}
	})

	t.Run("nil error", func(t *testing.T) {
		attrs := ErrorAttrs(nil)

		if attrs != nil {
			t.Errorf("Expected nil for nil error, got %v", attrs)
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  LevelDebug,
		Format: FormatJSON,
		Output: &buf,
	}
	logger := New(cfg)

	t.Run("LogCollection", func(t *testing.T) {
		buf.Reset()
		LogCollection(logger, "cpu", 10, 50, "session-1")

		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if logEntry["collector"] != "cpu" {
			t.Errorf("Expected collector='cpu', got %v", logEntry["collector"])
		}
		if logEntry["count"] != float64(10) {
			t.Errorf("Expected count=10, got %v", logEntry["count"])
		}
	})

	t.Run("LogCollectionError", func(t *testing.T) {
		buf.Reset()
		testErr := errors.New("collection failed")
		LogCollectionError(logger, "cpu", "session-1", testErr)

		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if logEntry["level"] != "ERROR" {
			t.Errorf("Expected level='ERROR', got %v", logEntry["level"])
		}
		if logEntry["error"] != "collection failed" {
			t.Errorf("Expected error='collection failed', got %v", logEntry["error"])
		}
	})

	t.Run("LogUpload", func(t *testing.T) {
		buf.Reset()
		LogUpload(logger, "batch-1", 0, 1, 0, 200, 1024, 512)

		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if logEntry["batch_id"] != "batch-1" {
			t.Errorf("Expected batch_id='batch-1', got %v", logEntry["batch_id"])
		}
		if logEntry["http_status"] != float64(200) {
			t.Errorf("Expected http_status=200, got %v", logEntry["http_status"])
		}
	})

	t.Run("LogUploadError", func(t *testing.T) {
		buf.Reset()
		testErr := errors.New("upload failed")
		LogUploadError(logger, "batch-1", 0, 1, testErr)

		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if logEntry["level"] != "ERROR" {
			t.Errorf("Expected level='ERROR', got %v", logEntry["level"])
		}
	})

	t.Run("LogRetry", func(t *testing.T) {
		buf.Reset()
		testErr := errors.New("retry error")
		LogRetry(logger, 2, 5000, testErr)

		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if logEntry["level"] != "WARN" {
			t.Errorf("Expected level='WARN', got %v", logEntry["level"])
		}
		if logEntry["attempt"] != float64(2) {
			t.Errorf("Expected attempt=2, got %v", logEntry["attempt"])
		}
	})
}

func TestSetDefault(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: &buf,
	}
	logger := New(cfg)
	SetDefault(logger)

	// Test that default logger was set
	defaultLogger := Default()
	if defaultLogger == nil {
		t.Error("Default logger is nil")
	}

	// Use slog default (which should now be our logger)
	slog.Info("test from default")

	if buf.Len() == 0 {
		t.Error("Default logger did not write output")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != LevelInfo {
		t.Errorf("Expected default level=info, got %v", cfg.Level)
	}
	if cfg.Format != FormatConsole {
		t.Errorf("Expected default format=console, got %v", cfg.Format)
	}
	if cfg.Output == nil {
		t.Error("Expected default output to be set")
	}
}
