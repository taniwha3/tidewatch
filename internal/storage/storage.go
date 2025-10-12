package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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
	// Check schema version and migrate if needed
	if err := migrateSchema(db); err != nil {
		return fmt.Errorf("failed to migrate schema: %w", err)
	}
	return nil
}

// migrateSchema handles schema versioning and migrations
func migrateSchema(db *sql.DB) error {
	// Create schema_version table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	// Get current version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	// Apply migrations
	migrations := []struct {
		version int
		sql     string
	}{
		{
			version: 1,
			sql: `
				-- M1 schema
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
			`,
		},
		{
			version: 2,
			sql: `
				-- M2 enhancements
				ALTER TABLE metrics ADD COLUMN uploaded INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE metrics ADD COLUMN priority INTEGER NOT NULL DEFAULT 1;
				ALTER TABLE metrics ADD COLUMN session_id TEXT;
				ALTER TABLE metrics ADD COLUMN dedup_key TEXT;
				ALTER TABLE metrics ADD COLUMN tags_json TEXT;

				-- Create unique index on dedup_key for idempotency
				CREATE UNIQUE INDEX IF NOT EXISTS idx_dedup_key ON metrics(dedup_key);

				-- Index for faster queries on name+device+time
				CREATE INDEX IF NOT EXISTS idx_name_dev_time ON metrics(metric_name, device_id, timestamp_ms);

				-- Index for upload tracking
				CREATE INDEX IF NOT EXISTS idx_uploaded ON metrics(uploaded, timestamp_ms);

				-- Upload checkpoints table for chunked uploads
				CREATE TABLE IF NOT EXISTS upload_checkpoints (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					batch_id TEXT NOT NULL,
					chunk_index INTEGER NOT NULL,
					uploaded_at INTEGER NOT NULL,
					metric_count INTEGER NOT NULL,
					success INTEGER NOT NULL DEFAULT 1,
					UNIQUE(batch_id, chunk_index)
				);

				CREATE INDEX IF NOT EXISTS idx_checkpoint_batch ON upload_checkpoints(batch_id);
			`,
		},
	}

	for _, migration := range migrations {
		if currentVersion < migration.version {
			// Execute migration in transaction
			tx, err := db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.version, err)
			}

			if _, err := tx.Exec(migration.sql); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to apply migration %d: %w", migration.version, err)
			}

			// Record migration
			if _, err := tx.Exec("INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
				migration.version, time.Now().Unix()); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to record migration %d: %w", migration.version, err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit migration %d: %w", migration.version, err)
			}
		}
	}

	return nil
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	return fmt.Sprintf("session_%d_%d", os.Getpid(), time.Now().UnixNano())
}

// generateDedupKey creates a unique deduplication key for a metric
// Format: sha256(name|timestamp_ms|device_id|canonical_tags)
func generateDedupKey(metric *models.Metric) string {
	// Sort tags canonically
	keys := make([]string, 0, len(metric.Tags))
	for k := range metric.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build canonical string
	var parts []string
	parts = append(parts, metric.Name)
	parts = append(parts, fmt.Sprintf("%d", metric.TimestampMs))
	parts = append(parts, metric.DeviceID)

	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, metric.Tags[k]))
	}

	data := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// serializeTags converts tags map to JSON
func serializeTags(tags map[string]string) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Store saves a single metric with deduplication
func (s *SQLiteStorage) Store(ctx context.Context, metric *models.Metric) error {
	return s.StoreBatch(ctx, []*models.Metric{metric})
}

// StoreBatch saves multiple metrics in a single transaction with deduplication
func (s *SQLiteStorage) StoreBatch(ctx context.Context, metrics []*models.Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	sessionID := generateSessionID()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe to call even after commit

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metrics (
			timestamp_ms, metric_name, metric_value, device_id,
			uploaded, priority, session_id, dedup_key, tags_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (dedup_key) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, metric := range metrics {
		dedupKey := generateDedupKey(metric)
		tagsJSON, err := serializeTags(metric.Tags)
		if err != nil {
			return fmt.Errorf("failed to serialize tags: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			metric.TimestampMs,
			metric.Name,
			metric.Value,
			metric.DeviceID,
			0,          // uploaded = false
			1,          // priority = normal
			sessionID,
			dedupKey,
			tagsJSON,
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
	query := "SELECT id, timestamp_ms, metric_name, metric_value, device_id, tags_json FROM metrics WHERE 1=1"
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
		var tagsJSON sql.NullString
		var id int64

		err := rows.Scan(
			&id,
			&m.TimestampMs,
			&m.Name,
			&m.Value,
			&m.DeviceID,
			&tagsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Deserialize tags if present
		if tagsJSON.Valid && tagsJSON.String != "" {
			if err := json.Unmarshal([]byte(tagsJSON.String), &m.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
			}
		}

		metrics = append(metrics, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return metrics, nil
}

// QueryUnuploaded retrieves metrics that haven't been uploaded yet
func (s *SQLiteStorage) QueryUnuploaded(ctx context.Context, limit int) ([]*models.Metric, error) {
	query := `
		SELECT id, timestamp_ms, metric_name, metric_value, device_id, tags_json
		FROM metrics
		WHERE uploaded = 0
		ORDER BY priority DESC, timestamp_ms ASC
	`
	args := []interface{}{}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query unuploaded metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*models.Metric
	for rows.Next() {
		m := &models.Metric{
			Tags: make(map[string]string),
		}
		var tagsJSON sql.NullString
		var id int64

		err := rows.Scan(
			&id,
			&m.TimestampMs,
			&m.Name,
			&m.Value,
			&m.DeviceID,
			&tagsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Store ID in tags for tracking
		if m.Tags == nil {
			m.Tags = make(map[string]string)
		}
		m.Tags["_storage_id"] = fmt.Sprintf("%d", id)

		// Deserialize tags if present
		if tagsJSON.Valid && tagsJSON.String != "" {
			var storedTags map[string]string
			if err := json.Unmarshal([]byte(tagsJSON.String), &storedTags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
			}
			for k, v := range storedTags {
				m.Tags[k] = v
			}
		}

		metrics = append(metrics, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return metrics, nil
}

// MarkUploaded marks metrics as uploaded
func (s *SQLiteStorage) MarkUploaded(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("UPDATE metrics SET uploaded = 1 WHERE id IN (%s)", strings.Join(placeholders, ","))
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to mark metrics as uploaded: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetPendingCount returns the count of unuploaded metrics
func (s *SQLiteStorage) GetPendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM metrics WHERE uploaded = 0").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending metrics: %w", err)
	}
	return count, nil
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

// CheckpointWAL performs a WAL checkpoint to reclaim space
func (s *SQLiteStorage) CheckpointWAL(ctx context.Context) error {
	// Use Exec to avoid the 2-vs-3 value scan issue mentioned in engineering review
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		return fmt.Errorf("WAL checkpoint failed: %w", err)
	}
	return nil
}

// GetWALSize returns the size of the WAL file in bytes
func (s *SQLiteStorage) GetWALSize() (int64, error) {
	// Get database path from connection
	var seq int
	var name, path string
	err := s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path)
	if err != nil {
		return 0, fmt.Errorf("failed to get database path: %w", err)
	}

	// WAL file is database_path + "-wal"
	walPath := path + "-wal"
	info, err := os.Stat(walPath)
	if os.IsNotExist(err) {
		return 0, nil // No WAL file yet
	}
	if err != nil {
		return 0, fmt.Errorf("failed to stat WAL file: %w", err)
	}

	return info.Size(), nil
}

// Close closes the database connection
func (s *SQLiteStorage) Close() error {
	if s.db != nil {
		// Checkpoint WAL before closing (using Exec as per engineering review)
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
