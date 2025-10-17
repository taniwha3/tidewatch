package storage

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/taniwha3/tidewatch/internal/models"
)

func TestStartWALCheckpointRoutine_PeriodicCheckpoint(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create a logger that writes to buffer
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Start checkpoint routine with very short interval for testing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checkpointInterval := 100 * time.Millisecond
	maxWALSize := int64(1024 * 1024) // 1 MB

	stopCheckpoint := store.StartWALCheckpointRoutine(ctx, logger, checkpointInterval, maxWALSize)
	defer stopCheckpoint()

	// Wait for at least one checkpoint to occur
	time.Sleep(250 * time.Millisecond)

	// Verify routine started message in logs
	logs := logBuf.String()
	if !contains(logs, "WAL checkpoint routine started") {
		t.Error("Expected routine started message in logs")
	}

	// Stop the routine
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Verify stopping message
	logs = logBuf.String()
	if !contains(logs, "WAL checkpoint routine stopping") {
		t.Error("Expected routine stopping message in logs")
	}
}

func TestStartWALCheckpointRoutine_SizeTriggered(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create logger
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Start checkpoint routine with very low threshold and fast size check interval
	checkpointCtx, checkpointCancel := context.WithCancel(context.Background())
	defer checkpointCancel()

	checkpointInterval := 10 * time.Hour       // Very long periodic interval (won't fire)
	maxWALSize := int64(1024)                  // 1 KB threshold (very low for testing)
	sizeCheckInterval := 50 * time.Millisecond // Fast size checks for testing

	// Use internal method to set fast size check interval
	stopCheckpoint := store.startWALCheckpointRoutineWithSizeInterval(
		checkpointCtx, logger, checkpointInterval, maxWALSize, sizeCheckInterval)
	defer stopCheckpoint()

	// Wait for routine to start
	time.Sleep(25 * time.Millisecond)

	// Now insert many metrics to grow the WAL AFTER the routine is running
	// This simulates WAL growth during operation
	for i := 0; i < 1000; i++ {
		metrics := make([]*models.Metric, 10)
		for j := 0; j < 10; j++ {
			metrics[j] = &models.Metric{
				Name:        "test.metric",
				TimestampMs: time.Now().UnixMilli() + int64(i*10+j),
				Value:       float64(i*10 + j),
				DeviceID:    "test-device",
				ValueType:   models.ValueTypeNumeric,
			}
		}
		if err := store.StoreBatch(ctx, metrics); err != nil {
			t.Fatalf("Failed to store metrics: %v", err)
		}
	}

	// Get WAL size after inserts
	walSizeBefore, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size: %v", err)
	}

	t.Logf("WAL size after inserts: %d bytes", walSizeBefore)

	// Wait for size check ticker to fire and trigger checkpoint
	// We configured 50ms size check interval, so wait 150ms to be safe
	time.Sleep(150 * time.Millisecond)

	// Check if we got a size-triggered checkpoint warning
	logs := logBuf.String()
	if walSizeBefore > maxWALSize && !contains(logs, "size-triggered") {
		t.Errorf("Expected size-triggered checkpoint (WAL was %d bytes, threshold %d bytes)", walSizeBefore, maxWALSize)
		t.Logf("Logs:\n%s", logs)
	}

	// Verify WAL was actually reduced
	walSizeAfter, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size after checkpoint: %v", err)
	}
	t.Logf("WAL size after checkpoint: %d bytes", walSizeAfter)

	if walSizeBefore > maxWALSize && walSizeAfter >= walSizeBefore {
		t.Errorf("Expected WAL size to be reduced: before=%d, after=%d", walSizeBefore, walSizeAfter)
	}
}

func TestStartWALCheckpointRoutine_StartupSizeCheck(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Insert many metrics to grow the WAL
	for i := 0; i < 1000; i++ {
		metrics := make([]*models.Metric, 10)
		for j := 0; j < 10; j++ {
			metrics[j] = &models.Metric{
				Name:        "test.metric",
				TimestampMs: time.Now().UnixMilli() + int64(i*10+j),
				Value:       float64(i*10 + j),
				DeviceID:    "test-device",
				ValueType:   models.ValueTypeNumeric,
			}
		}
		if err := store.StoreBatch(ctx, metrics); err != nil {
			t.Fatalf("Failed to store metrics: %v", err)
		}
	}

	// Get WAL size after inserts (should be large)
	walSizeBefore, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size: %v", err)
	}

	t.Logf("WAL size before starting routine: %d bytes", walSizeBefore)

	// Create logger to capture logs
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Start checkpoint routine with very low threshold
	// This should trigger an IMMEDIATE checkpoint on startup
	checkpointCtx, checkpointCancel := context.WithCancel(context.Background())
	defer checkpointCancel()

	checkpointInterval := 10 * time.Hour // Very long interval (won't fire during test)
	maxWALSize := int64(1024)            // 1 KB threshold (very low for testing)

	stopCheckpoint := store.StartWALCheckpointRoutine(checkpointCtx, logger, checkpointInterval, maxWALSize)
	defer stopCheckpoint()

	// Wait briefly for startup checkpoint to complete
	time.Sleep(100 * time.Millisecond)

	// Check logs for startup-triggered checkpoint
	logs := logBuf.String()
	if walSizeBefore > maxWALSize {
		if !contains(logs, "startup-size-triggered") {
			t.Errorf("Expected startup-size-triggered checkpoint in logs (WAL was %d bytes, threshold %d bytes)", walSizeBefore, maxWALSize)
		}
		if !contains(logs, "WAL checkpoint completed") {
			t.Error("Expected checkpoint completion message in logs")
		}
	}

	// Verify WAL size was reduced
	walSizeAfter, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size after startup checkpoint: %v", err)
	}

	t.Logf("WAL size after startup checkpoint: %d bytes", walSizeAfter)

	// WAL should be significantly reduced
	if walSizeBefore > maxWALSize && walSizeAfter >= walSizeBefore {
		t.Errorf("Expected WAL size to be reduced after startup checkpoint: before=%d, after=%d", walSizeBefore, walSizeAfter)
	}

	store.Close()
}

func TestPerformCheckpoint(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert some metrics to create WAL
	metrics := []*models.Metric{
		{
			Name:        "test.metric",
			TimestampMs: time.Now().UnixMilli(),
			Value:       42.0,
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		},
	}
	if err := store.StoreBatch(ctx, metrics); err != nil {
		t.Fatalf("Failed to store metrics: %v", err)
	}

	// Create logger
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Perform checkpoint
	store.performCheckpoint(logger, "test")

	// Verify checkpoint completed message
	logs := logBuf.String()
	if !contains(logs, "WAL checkpoint completed") {
		t.Error("Expected checkpoint completed message in logs")
	}
	if !contains(logs, "reason=test") {
		t.Error("Expected reason=test in logs")
	}
}

func TestCheckpointWALReducesWALSize(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert many metrics to grow WAL
	for i := 0; i < 100; i++ {
		metrics := make([]*models.Metric, 10)
		for j := 0; j < 10; j++ {
			metrics[j] = &models.Metric{
				Name:        "test.metric",
				TimestampMs: time.Now().UnixMilli() + int64(i*10+j),
				Value:       float64(i*10 + j),
				DeviceID:    "test-device",
				ValueType:   models.ValueTypeNumeric,
			}
		}
		if err := store.StoreBatch(ctx, metrics); err != nil {
			t.Fatalf("Failed to store metrics: %v", err)
		}
	}

	// Get WAL size before checkpoint
	walSizeBefore, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size before: %v", err)
	}

	t.Logf("WAL size before checkpoint: %d bytes", walSizeBefore)

	// Perform checkpoint
	if err := store.CheckpointWAL(ctx); err != nil {
		t.Fatalf("Checkpoint failed: %v", err)
	}

	// Get WAL size after checkpoint
	walSizeAfter, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size after: %v", err)
	}

	t.Logf("WAL size after checkpoint: %d bytes", walSizeAfter)

	// WAL size should be reduced (or at least not increased)
	if walSizeAfter > walSizeBefore {
		t.Errorf("WAL size increased after checkpoint: before=%d, after=%d", walSizeBefore, walSizeAfter)
	}
}

func TestCheckpointOnShutdown(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Insert metrics
	metrics := []*models.Metric{
		{
			Name:        "test.metric",
			TimestampMs: time.Now().UnixMilli(),
			Value:       42.0,
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		},
	}
	if err := store.StoreBatch(ctx, metrics); err != nil {
		t.Fatalf("Failed to store metrics: %v", err)
	}

	// Get WAL size before close
	walSizeBefore, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size: %v", err)
	}

	t.Logf("WAL size before close: %d bytes", walSizeBefore)

	// Close storage (should checkpoint)
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close storage: %v", err)
	}

	// Reopen database to check WAL size
	store2, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer store2.Close()

	walSizeAfter, err := store2.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size after reopen: %v", err)
	}

	t.Logf("WAL size after reopen: %d bytes", walSizeAfter)

	// WAL should be small after checkpoint on close
	// Note: It may not be exactly 0 due to SQLite internals
	if walSizeAfter > walSizeBefore {
		t.Errorf("WAL size should not increase after close: before=%d, after=%d", walSizeBefore, walSizeAfter)
	}
}

func TestGetWALSize(t *testing.T) {
	// Create temporary database
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Get WAL size (should be 0 or small for new database)
	walSize, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size: %v", err)
	}

	// WAL size should be non-negative
	if walSize < 0 {
		t.Errorf("WAL size should be non-negative: %d", walSize)
	}

	t.Logf("Initial WAL size: %d bytes", walSize)

	// Insert some data
	ctx := context.Background()
	metrics := []*models.Metric{
		{
			Name:        "test.metric",
			TimestampMs: time.Now().UnixMilli(),
			Value:       42.0,
			DeviceID:    "test-device",
			ValueType:   models.ValueTypeNumeric,
		},
	}
	if err := store.StoreBatch(ctx, metrics); err != nil {
		t.Fatalf("Failed to store metrics: %v", err)
	}

	// Get WAL size again
	walSizeAfter, err := store.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size after insert: %v", err)
	}

	t.Logf("WAL size after insert: %d bytes", walSizeAfter)

	// WAL size may increase or stay the same depending on SQLite behavior
	if walSizeAfter < 0 {
		t.Errorf("WAL size after insert should be non-negative: %d", walSizeAfter)
	}
}

func TestWALSizeNonExistent(t *testing.T) {
	// Create storage but immediately close it
	dbPath := t.TempDir() + "/test.db"
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	store.Close()

	// Remove WAL file if it exists
	walPath := dbPath + "-wal"
	os.Remove(walPath)

	// Reopen database
	store2, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer store2.Close()

	// Get WAL size (should be 0 for non-existent WAL)
	walSize, err := store2.GetWALSize()
	if err != nil {
		t.Fatalf("Failed to get WAL size: %v", err)
	}

	if walSize != 0 {
		t.Logf("Note: WAL size is %d (may be created immediately by SQLite)", walSize)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
