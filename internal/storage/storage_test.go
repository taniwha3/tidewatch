package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*SQLiteStorage, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}

	cleanup := func() {
		storage.Close()
		os.RemoveAll(tmpDir)
	}

	return storage, dbPath, cleanup
}

func TestNewSQLiteStorage(t *testing.T) {
	storage, dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	if storage == nil {
		t.Fatal("Expected non-nil storage")
	}

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file not created at %s", dbPath)
	}

	// Verify WAL mode is enabled
	var journalMode string
	err := storage.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("Failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("Expected journal_mode=wal, got %s", journalMode)
	}
}

func TestStore_SingleMetric(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	metric := models.NewMetric("cpu.temperature", 52.3, "test-device")

	err := storage.Store(ctx, metric)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify stored
	count, err := storage.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 metric, got %d", count)
	}
}

func TestStoreBatch(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(1 * time.Second)),
		models.NewMetric("srt.packet_loss_pct", 0.5, "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}

	err := storage.StoreBatch(ctx, metrics)
	if err != nil {
		t.Fatalf("StoreBatch failed: %v", err)
	}

	count, err := storage.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 metrics, got %d", count)
	}
}

func TestStoreBatch_EmptySlice(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	err := storage.StoreBatch(ctx, []*models.Metric{})
	if err != nil {
		t.Errorf("StoreBatch with empty slice should not error: %v", err)
	}

	count, err := storage.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 metrics, got %d", count)
	}
}

func TestQuery_AllMetrics(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Store test data
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(1 * time.Second)),
		models.NewMetric("srt.packet_loss_pct", 0.5, "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	// Query all
	results, err := storage.Query(ctx, QueryOptions{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 metrics, got %d", len(results))
	}
}

func TestQuery_TimeRange(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Store metrics across time range
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now.Add(-10 * time.Second)),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 52.0, "device-001").WithTimestamp(now.Add(10 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	// Query middle metric only
	results, err := storage.Query(ctx, QueryOptions{
		StartMs: now.Add(-5 * time.Second).UnixMilli(),
		EndMs:   now.Add(5 * time.Second).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 metric in range, got %d", len(results))
	}

	if len(results) > 0 && results[0].Value != 51.0 {
		t.Errorf("Expected value 51.0, got %.1f", results[0].Value)
	}
}

func TestQuery_FilterByMetricName(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(1 * time.Second)),
		models.NewMetric("srt.packet_loss_pct", 0.5, "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	// Query only cpu.temperature
	results, err := storage.Query(ctx, QueryOptions{
		MetricName: "cpu.temperature",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 cpu.temperature metrics, got %d", len(results))
	}

	for _, m := range results {
		if m.Name != "cpu.temperature" {
			t.Errorf("Expected metric name cpu.temperature, got %s", m.Name)
		}
	}
}

func TestQuery_FilterByDeviceID(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-002").WithTimestamp(now.Add(1 * time.Second)),
		models.NewMetric("cpu.temperature", 52.0, "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	// Query only device-001
	results, err := storage.Query(ctx, QueryOptions{
		DeviceID: "device-001",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 metrics for device-001, got %d", len(results))
	}

	for _, m := range results {
		if m.DeviceID != "device-001" {
			t.Errorf("Expected device ID device-001, got %s", m.DeviceID)
		}
	}
}

func TestQuery_Limit(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(1 * time.Second)),
		models.NewMetric("cpu.temperature", 52.0, "device-001").WithTimestamp(now.Add(2 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	// Query with limit
	results, err := storage.Query(ctx, QueryOptions{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 metrics (limit), got %d", len(results))
	}
}

func TestQuery_OrderByTimestamp(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Insert out of order
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 52.0, "device-001").WithTimestamp(now.Add(10 * time.Second)),
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(5 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	// Query should return in ascending timestamp order
	results, err := storage.Query(ctx, QueryOptions{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 metrics, got %d", len(results))
	}

	// Verify ascending order
	if results[0].Value != 50.0 || results[1].Value != 51.0 || results[2].Value != 52.0 {
		t.Errorf("Metrics not in ascending timestamp order: %.1f, %.1f, %.1f",
			results[0].Value, results[1].Value, results[2].Value)
	}
}

func TestDeleteBefore(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Store metrics across time
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now.Add(-10 * time.Second)),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 52.0, "device-001").WithTimestamp(now.Add(10 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	// Delete metrics older than now
	deleted, err := storage.DeleteBefore(ctx, now.UnixMilli())
	if err != nil {
		t.Fatalf("DeleteBefore failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 deleted metric, got %d", deleted)
	}

	// Verify remaining count
	count, err := storage.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 remaining metrics, got %d", count)
	}
}

func TestGetOldestTimestamp(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Empty database
	oldest, err := storage.GetOldestTimestamp(ctx)
	if err != nil {
		t.Fatalf("GetOldestTimestamp failed: %v", err)
	}
	if oldest != 0 {
		t.Errorf("Expected 0 for empty database, got %d", oldest)
	}

	// Add metrics
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now.Add(10 * time.Second)),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now),
	}
	storage.StoreBatch(ctx, metrics)

	oldest, err = storage.GetOldestTimestamp(ctx)
	if err != nil {
		t.Fatalf("GetOldestTimestamp failed: %v", err)
	}

	expectedOldest := now.UnixMilli()
	if oldest != expectedOldest {
		t.Errorf("Expected oldest %d, got %d", expectedOldest, oldest)
	}
}

func TestGetNewestTimestamp(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Empty database
	newest, err := storage.GetNewestTimestamp(ctx)
	if err != nil {
		t.Fatalf("GetNewestTimestamp failed: %v", err)
	}
	if newest != 0 {
		t.Errorf("Expected 0 for empty database, got %d", newest)
	}

	// Add metrics
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(10 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	newest, err = storage.GetNewestTimestamp(ctx)
	if err != nil {
		t.Fatalf("GetNewestTimestamp failed: %v", err)
	}

	expectedNewest := now.Add(10 * time.Second).UnixMilli()
	if newest != expectedNewest {
		t.Errorf("Expected newest %d, got %d", expectedNewest, newest)
	}
}

func TestGetMetricStats(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(1 * time.Second)),
		models.NewMetric("cpu.temperature", 52.0, "device-001").WithTimestamp(now.Add(2 * time.Second)),
		models.NewMetric("srt.packet_loss_pct", 0.5, "device-001").WithTimestamp(now.Add(3 * time.Second)),
		models.NewMetric("srt.packet_loss_pct", 1.0, "device-001").WithTimestamp(now.Add(4 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	stats, err := storage.GetMetricStats(ctx)
	if err != nil {
		t.Fatalf("GetMetricStats failed: %v", err)
	}

	if len(stats) != 2 {
		t.Errorf("Expected 2 metric types, got %d", len(stats))
	}

	if stats["cpu.temperature"] != 3 {
		t.Errorf("Expected 3 cpu.temperature metrics, got %d", stats["cpu.temperature"])
	}

	if stats["srt.packet_loss_pct"] != 2 {
		t.Errorf("Expected 2 srt.packet_loss_pct metrics, got %d", stats["srt.packet_loss_pct"])
	}
}

func TestGetTimeRange(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Empty database
	start, end, err := storage.GetTimeRange(ctx)
	if err != nil {
		t.Fatalf("GetTimeRange failed: %v", err)
	}
	if !start.IsZero() || !end.IsZero() {
		t.Errorf("Expected zero times for empty database, got %v, %v", start, end)
	}

	// Add metrics
	now := time.Now()
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001").WithTimestamp(now),
		models.NewMetric("cpu.temperature", 51.0, "device-001").WithTimestamp(now.Add(10 * time.Second)),
	}
	storage.StoreBatch(ctx, metrics)

	start, end, err = storage.GetTimeRange(ctx)
	if err != nil {
		t.Fatalf("GetTimeRange failed: %v", err)
	}

	// Check range (allow for millisecond precision differences)
	if start.Unix() != now.Unix() {
		t.Errorf("Expected start time %v, got %v", now, start)
	}
	if end.Unix() != now.Add(10*time.Second).Unix() {
		t.Errorf("Expected end time %v, got %v", now.Add(10*time.Second), end)
	}
}

func TestDBSize(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	size, err := storage.DBSize()
	if err != nil {
		t.Fatalf("DBSize failed: %v", err)
	}

	if size <= 0 {
		t.Errorf("Expected positive database size, got %d", size)
	}
}

func TestVacuum(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Add and delete metrics to create fragmentation
	metrics := []*models.Metric{
		models.NewMetric("cpu.temperature", 50.0, "device-001"),
		models.NewMetric("cpu.temperature", 51.0, "device-001"),
	}
	storage.StoreBatch(ctx, metrics)
	storage.DeleteBefore(ctx, time.Now().UnixMilli())

	// Vacuum should succeed
	err := storage.Vacuum(ctx)
	if err != nil {
		t.Errorf("Vacuum failed: %v", err)
	}
}

func TestClose(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	err := storage.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify database is closed (operations should fail)
	ctx := context.Background()
	_, err = storage.Count(ctx)
	if err == nil {
		t.Error("Expected error after close, got nil")
	}
}

func TestConcurrentWrites(t *testing.T) {
	storage, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Write metrics concurrently with unique timestamps
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			metric := models.NewMetric("cpu.temperature", float64(50+id), "device-001").
				WithTimestamp(now.Add(time.Duration(id) * time.Second))
			storage.Store(ctx, metric)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all metrics were stored
	count, err := storage.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 10 {
		t.Errorf("Expected 10 metrics from concurrent writes, got %d", count)
	}
}

func BenchmarkStore(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	metric := models.NewMetric("cpu.temperature", 52.3, "device-001")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.Store(ctx, metric)
	}
}

func BenchmarkStoreBatch(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	metrics := make([]*models.Metric, 100)
	for i := range metrics {
		metrics[i] = models.NewMetric("cpu.temperature", float64(50+i), "device-001")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.StoreBatch(ctx, metrics)
	}
}

func BenchmarkQuery(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Populate database
	metrics := make([]*models.Metric, 1000)
	for i := range metrics {
		metrics[i] = models.NewMetric("cpu.temperature", float64(50+i%10), "device-001")
	}
	storage.StoreBatch(ctx, metrics)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.Query(ctx, QueryOptions{Limit: 100})
	}
}
