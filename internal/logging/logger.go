package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Format represents the log output format
type Format string

const (
	FormatJSON    Format = "json"
	FormatConsole Format = "console"
)

// Level represents log levels
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Config holds logging configuration
type Config struct {
	Level  Level
	Format Format
	Output io.Writer // defaults to os.Stdout if nil
}

// DefaultConfig returns a default logging configuration
func DefaultConfig() Config {
	return Config{
		Level:  LevelInfo,
		Format: FormatConsole,
		Output: os.Stdout,
	}
}

var defaultLogger *slog.Logger

func init() {
	// Initialize with default console logger
	cfg := DefaultConfig()
	defaultLogger = New(cfg)
}

// New creates a new structured logger with the given configuration
func New(cfg Config) *slog.Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	level := parseLevel(cfg.Level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch cfg.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(cfg.Output, opts)
	default:
		handler = slog.NewTextHandler(cfg.Output, opts)
	}

	return slog.New(handler)
}

// parseLevel converts a Level string to slog.Level
func parseLevel(level Level) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetDefault sets the default logger for the package
func SetDefault(logger *slog.Logger) {
	defaultLogger = logger
	slog.SetDefault(logger)
}

// Default returns the default logger
func Default() *slog.Logger {
	return defaultLogger
}

// Context keys for logging
type contextKey string

const (
	// ContextKeyCollector is the context key for collector name
	ContextKeyCollector contextKey = "collector"
	// ContextKeyBatchID is the context key for batch ID
	ContextKeyBatchID contextKey = "batch_id"
	// ContextKeySessionID is the context key for session ID
	ContextKeySessionID contextKey = "session_id"
)

// WithCollector adds collector name to context
func WithCollector(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, ContextKeyCollector, name)
}

// WithBatchID adds batch ID to context
func WithBatchID(ctx context.Context, batchID string) context.Context {
	return context.WithValue(ctx, ContextKeyBatchID, batchID)
}

// WithSessionID adds session ID to context
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, ContextKeySessionID, sessionID)
}

// CollectionAttrs returns common attributes for collection logging
func CollectionAttrs(collector string, count int, durationMs int64, sessionID string) []slog.Attr {
	return []slog.Attr{
		slog.String("collector", collector),
		slog.Int("count", count),
		slog.Int64("duration_ms", durationMs),
		slog.String("session_id", sessionID),
	}
}

// UploadAttrs returns common attributes for upload logging
func UploadAttrs(batchID string, chunkIndex, attempt int, backoffMs int64, httpStatus int, bytesSent, bytesRcvd int64) []slog.Attr {
	return []slog.Attr{
		slog.String("batch_id", batchID),
		slog.Int("chunk_index", chunkIndex),
		slog.Int("attempt", attempt),
		slog.Int64("backoff_ms", backoffMs),
		slog.Int("http_status", httpStatus),
		slog.Int64("bytes_sent", bytesSent),
		slog.Int64("bytes_rcvd", bytesRcvd),
	}
}

// RetryAttrs returns common attributes for retry logging
func RetryAttrs(attempt int, backoffMs int64, err error) []slog.Attr {
	attrs := []slog.Attr{
		slog.Int("attempt", attempt),
		slog.Int64("backoff_ms", backoffMs),
	}
	if err != nil {
		attrs = append(attrs,
			slog.String("error", err.Error()),
			slog.String("error_type", errorType(err)),
		)
	}
	return attrs
}

// ErrorAttrs returns common attributes for error logging
func ErrorAttrs(err error) []slog.Attr {
	if err == nil {
		return nil
	}
	return []slog.Attr{
		slog.String("error", err.Error()),
		slog.String("error_type", errorType(err)),
	}
}

// errorType attempts to determine the type of error
func errorType(err error) string {
	if err == nil {
		return ""
	}
	// Try to get the concrete type name
	return fmt.Sprintf("%T", err)
}

// Helper functions for common logging patterns

// LogCollection logs a collection event with standard fields
func LogCollection(logger *slog.Logger, collector string, count int, durationMs int64, sessionID string) {
	logger.LogAttrs(context.Background(), slog.LevelInfo, "Collection completed",
		CollectionAttrs(collector, count, durationMs, sessionID)...)
}

// LogCollectionError logs a collection error with standard fields
func LogCollectionError(logger *slog.Logger, collector string, sessionID string, err error) {
	attrs := []slog.Attr{
		slog.String("collector", collector),
		slog.String("session_id", sessionID),
	}
	attrs = append(attrs, ErrorAttrs(err)...)
	logger.LogAttrs(context.Background(), slog.LevelError, "Collection failed", attrs...)
}

// LogUpload logs an upload event with standard fields
func LogUpload(logger *slog.Logger, batchID string, chunkIndex, attempt int, backoffMs int64, httpStatus int, bytesSent, bytesRcvd int64) {
	logger.LogAttrs(context.Background(), slog.LevelInfo, "Upload completed",
		UploadAttrs(batchID, chunkIndex, attempt, backoffMs, httpStatus, bytesSent, bytesRcvd)...)
}

// LogUploadError logs an upload error with standard fields
func LogUploadError(logger *slog.Logger, batchID string, chunkIndex, attempt int, err error) {
	attrs := []slog.Attr{
		slog.String("batch_id", batchID),
		slog.Int("chunk_index", chunkIndex),
		slog.Int("attempt", attempt),
	}
	attrs = append(attrs, ErrorAttrs(err)...)
	logger.LogAttrs(context.Background(), slog.LevelError, "Upload failed", attrs...)
}

// LogRetry logs a retry attempt with standard fields
func LogRetry(logger *slog.Logger, attempt int, backoffMs int64, err error) {
	logger.LogAttrs(context.Background(), slog.LevelWarn, "Retrying after failure",
		RetryAttrs(attempt, backoffMs, err)...)
}
