package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
	_ "modernc.org/sqlite"
)

// Storage is the interface for metric storage
type Storage interface {
	// Store saves a single metric
	Store(ctx context.Context, metric *models.Metric) error

	// StoreBatch saves multiple metrics in a single transaction
	StoreBatch(ctx context.Context, metrics []*models.Metric) error

	// Query retrieves metrics within a time range
	Query(ctx context.Context, opts QueryOptions) ([]*models.Metric, error)

	// Close closes the storage connection
	Close() error
}

// QueryOptions defines options for querying metrics
type QueryOptions struct {
	StartMs   int64  // Start timestamp in milliseconds (inclusive)
	EndMs     int64  // End timestamp in milliseconds (inclusive)
	DeviceID  string // Filter by device ID (empty = all devices)
	MetricName string // Filter by metric name (empty = all metrics)
	Limit     int    // Maximum number of results (0 = no limit)
}

// SQLiteStorage implements Storage using SQLite
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage creates a new SQLite storage instance
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	// Open database with SQLite-specific connection parameters
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite doesn't benefit from multiple connections
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Apply SQLite tuning PRAGMAs for ARM SBC
	pragmas := []string{
		"PRAGMA journal_mode=WAL",       // Write-ahead logging for better concurrency
		"PRAGMA synchronous=NORMAL",     // Balance between performance and safety
		"PRAGMA busy_timeout=10000",     // Wait up to 10s for locks
		"PRAGMA temp_store=MEMORY",      // Use memory for temp tables
		"PRAGMA cache_size=-64000",      // 64MB cache (negative = KB)
		"PRAGMA mmap_size=268435456",    // 256MB memory-mapped I/O
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma %s: %w", pragma, err)
		}
	}

	// Initialize schema
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteStorage{db: db}, nil
}

// initSchema creates the database tables and indexes
func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp_ms INTEGER NOT NULL,
		metric_name TEXT NOT NULL,
		metric_value REAL,
		device_id TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_timestamp ON metrics(timestamp_ms);
	CREATE INDEX IF NOT EXISTS idx_name_time ON metrics(metric_name, timestamp_ms);
	CREATE INDEX IF NOT EXISTS idx_device_time ON metrics(device_id, timestamp_ms);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Store saves a single metric
func (s *SQLiteStorage) Store(ctx context.Context, metric *models.Metric) error {
	query := `
	INSERT INTO metrics (timestamp_ms, metric_name, metric_value, device_id)
	VALUES (?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		metric.TimestampMs,
		metric.Name,
		metric.Value,
		metric.DeviceID,
	)

	if err != nil {
		return fmt.Errorf("failed to store metric: %w", err)
	}

	return nil
}

// StoreBatch saves multiple metrics in a single transaction
func (s *SQLiteStorage) StoreBatch(ctx context.Context, metrics []*models.Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe to call even after commit

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metrics (timestamp_ms, metric_name, metric_value, device_id)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, metric := range metrics {
		_, err := stmt.ExecContext(ctx,
			metric.TimestampMs,
			metric.Name,
			metric.Value,
			metric.DeviceID,
		)
		if err != nil {
			return fmt.Errorf("failed to insert metric: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Query retrieves metrics within a time range
func (s *SQLiteStorage) Query(ctx context.Context, opts QueryOptions) ([]*models.Metric, error) {
	query := "SELECT timestamp_ms, metric_name, metric_value, device_id FROM metrics WHERE 1=1"
	args := []interface{}{}

	if opts.StartMs > 0 {
		query += " AND timestamp_ms >= ?"
		args = append(args, opts.StartMs)
	}

	if opts.EndMs > 0 {
		query += " AND timestamp_ms <= ?"
		args = append(args, opts.EndMs)
	}

	if opts.DeviceID != "" {
		query += " AND device_id = ?"
		args = append(args, opts.DeviceID)
	}

	if opts.MetricName != "" {
		query += " AND metric_name = ?"
		args = append(args, opts.MetricName)
	}

	query += " ORDER BY timestamp_ms ASC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*models.Metric
	for rows.Next() {
		m := &models.Metric{
			Tags: make(map[string]string),
		}

		err := rows.Scan(
			&m.TimestampMs,
			&m.Name,
			&m.Value,
			&m.DeviceID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		metrics = append(metrics, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return metrics, nil
}

// Count returns the total number of metrics in storage
func (s *SQLiteStorage) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM metrics").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count metrics: %w", err)
	}
	return count, nil
}

// DeleteBefore deletes metrics older than the specified timestamp
func (s *SQLiteStorage) DeleteBefore(ctx context.Context, timestampMs int64) (int64, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM metrics WHERE timestamp_ms < ?", timestampMs)
	if err != nil {
		return 0, fmt.Errorf("failed to delete metrics: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return deleted, nil
}

// Vacuum reclaims unused database space
func (s *SQLiteStorage) Vacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
	}
	return nil
}

// GetOldestTimestamp returns the timestamp of the oldest metric
func (s *SQLiteStorage) GetOldestTimestamp(ctx context.Context) (int64, error) {
	var ts sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT MIN(timestamp_ms) FROM metrics").Scan(&ts)
	if err != nil {
		return 0, fmt.Errorf("failed to get oldest timestamp: %w", err)
	}
	if !ts.Valid {
		return 0, nil // No metrics
	}
	return ts.Int64, nil
}

// GetNewestTimestamp returns the timestamp of the newest metric
func (s *SQLiteStorage) GetNewestTimestamp(ctx context.Context) (int64, error) {
	var ts sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT MAX(timestamp_ms) FROM metrics").Scan(&ts)
	if err != nil {
		return 0, fmt.Errorf("failed to get newest timestamp: %w", err)
	}
	if !ts.Valid {
		return 0, nil // No metrics
	}
	return ts.Int64, nil
}

// GetMetricStats returns statistics about stored metrics
func (s *SQLiteStorage) GetMetricStats(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT metric_name, COUNT(*) as count
		FROM metrics
		GROUP BY metric_name
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query metric stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}
		stats[name] = count
	}

	return stats, nil
}

// Close closes the database connection
func (s *SQLiteStorage) Close() error {
	if s.db != nil {
		// Checkpoint WAL before closing
		s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		return s.db.Close()
	}
	return nil
}

// DBSize returns the size of the database in bytes
func (s *SQLiteStorage) DBSize() (int64, error) {
	var pageCount, pageSize int64
	err := s.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	if err != nil {
		return 0, fmt.Errorf("failed to get page count: %w", err)
	}
	err = s.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if err != nil {
		return 0, fmt.Errorf("failed to get page size: %w", err)
	}
	return pageCount * pageSize, nil
}

// GetTimeRange returns the time range of stored metrics
func (s *SQLiteStorage) GetTimeRange(ctx context.Context) (startTime, endTime time.Time, err error) {
	oldest, err := s.GetOldestTimestamp(ctx)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	newest, err := s.GetNewestTimestamp(ctx)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	if oldest == 0 || newest == 0 {
		return time.Time{}, time.Time{}, nil // No metrics
	}

	return time.UnixMilli(oldest), time.UnixMilli(newest), nil
}
