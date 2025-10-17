package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/taniwha3/tidewatch/internal/models"
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
	StartMs    int64  // Start timestamp in milliseconds (inclusive)
	EndMs      int64  // End timestamp in milliseconds (inclusive)
	DeviceID   string // Filter by device ID (empty = all devices)
	MetricName string // Filter by metric name (empty = all metrics)
	Limit      int    // Maximum number of results (0 = no limit)
}

// SQLiteStorage implements Storage using SQLite
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage creates a new SQLite storage instance
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	// Create parent directory if it doesn't exist
	// Skip directory creation for SQLite URI paths (e.g., "file:path?mode=ro")
	// to preserve URI support and avoid creating directories named "file:..."
	if !strings.HasPrefix(dbPath, "file:") {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

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
		"PRAGMA journal_mode=WAL",    // Write-ahead logging for better concurrency
		"PRAGMA synchronous=NORMAL",  // Balance between performance and safety
		"PRAGMA busy_timeout=10000",  // Wait up to 10s for locks
		"PRAGMA temp_store=MEMORY",   // Use memory for temp tables
		"PRAGMA cache_size=-64000",   // 64MB cache (negative = KB)
		"PRAGMA mmap_size=268435456", // 256MB memory-mapped I/O
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
		{
			version: 3,
			sql: `
				-- Backfill dedup_key for existing rows
				-- For legacy metrics without tags, generate dedup key from name|timestamp|device
				-- NOTE: This migration is executed via migrateV3Backfill() due to custom logic needs
			`,
		},
		{
			version: 4,
			sql: `
				-- M2: Add support for string metrics
				ALTER TABLE metrics ADD COLUMN value_text TEXT;
				ALTER TABLE metrics ADD COLUMN value_type INTEGER NOT NULL DEFAULT 0;

				-- M2: Create sessions table for session tracking
				CREATE TABLE IF NOT EXISTS sessions (
					id TEXT PRIMARY KEY,
					start_time INTEGER NOT NULL,
					end_time INTEGER,
					status TEXT,
					metadata TEXT
				);

				CREATE INDEX IF NOT EXISTS idx_session_start ON sessions(start_time);
			`,
		},
		{
			version: 5,
			sql: `
				-- M2: Regenerate all dedup_keys to include value_type
				-- This ensures deduplication continues to work after adding value_type field
				-- NOTE: This migration is executed via migrateV5RegenerateDedupKeys() due to custom logic needs
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

			// V3 migration needs custom backfill logic
			if migration.version == 3 {
				if err := migrateV3Backfill(tx); err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to backfill dedup_key in migration 3: %w", err)
				}
			}

			// V5 migration regenerates dedup_keys with new format including value_type
			if migration.version == 5 {
				if err := migrateV5RegenerateDedupKeys(tx); err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to regenerate dedup_keys in migration 5: %w", err)
				}
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

// migrateV5RegenerateDedupKeys regenerates all dedup_keys to include value_type
// This fixes backwards compatibility after adding value_type to the dedup key format
func migrateV5RegenerateDedupKeys(tx *sql.Tx) error {
	// Query ALL metrics to regenerate their dedup_keys with the new format
	rows, err := tx.Query(`
		SELECT id, timestamp_ms, metric_name, device_id, value_type, tags_json
		FROM metrics
	`)
	if err != nil {
		return fmt.Errorf("failed to query metrics for dedup_key regeneration: %w", err)
	}
	defer rows.Close()

	// Prepare update statement
	stmt, err := tx.Prepare("UPDATE metrics SET dedup_key = ? WHERE id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}
	defer stmt.Close()

	// Process each metric and regenerate dedup_key
	count := 0
	for rows.Next() {
		var id int64
		var timestampMs int64
		var metricName, deviceID string
		var valueType int
		var tagsJSON sql.NullString

		if err := rows.Scan(&id, &timestampMs, &metricName, &deviceID, &valueType, &tagsJSON); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		// Reconstruct metric for dedup key generation
		metric := &models.Metric{
			Name:        metricName,
			TimestampMs: timestampMs,
			DeviceID:    deviceID,
			ValueType:   models.ValueType(valueType),
			Tags:        make(map[string]string),
		}

		// Deserialize tags if present
		if tagsJSON.Valid && tagsJSON.String != "" {
			if err := json.Unmarshal([]byte(tagsJSON.String), &metric.Tags); err != nil {
				return fmt.Errorf("failed to unmarshal tags for metric %d: %w", id, err)
			}
		}

		// Generate NEW dedup key using current format (includes value_type)
		dedupKey := generateDedupKey(metric)

		// Update the row
		if _, err := stmt.Exec(dedupKey, id); err != nil {
			return fmt.Errorf("failed to update dedup_key for metric %d: %w", id, err)
		}

		count++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// Log migration progress (will be visible if run with logging enabled)
	fmt.Printf("Migration v5: Regenerated dedup_keys for %d metrics\n", count)

	return nil
}

// migrateV3Backfill backfills dedup_key for existing metrics from v1/v2
func migrateV3Backfill(tx *sql.Tx) error {
	// Query all metrics with NULL dedup_key
	rows, err := tx.Query(`
		SELECT id, timestamp_ms, metric_name, device_id, tags_json
		FROM metrics
		WHERE dedup_key IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to query metrics for backfill: %w", err)
	}
	defer rows.Close()

	// Prepare update statement
	stmt, err := tx.Prepare("UPDATE metrics SET dedup_key = ? WHERE id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}
	defer stmt.Close()

	// Process each legacy metric
	for rows.Next() {
		var id int64
		var timestampMs int64
		var metricName, deviceID string
		var tagsJSON sql.NullString

		if err := rows.Scan(&id, &timestampMs, &metricName, &deviceID, &tagsJSON); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		// Reconstruct metric for dedup key generation
		metric := &models.Metric{
			Name:        metricName,
			TimestampMs: timestampMs,
			DeviceID:    deviceID,
			Tags:        make(map[string]string),
		}

		// Deserialize tags if present
		if tagsJSON.Valid && tagsJSON.String != "" {
			if err := json.Unmarshal([]byte(tagsJSON.String), &metric.Tags); err != nil {
				return fmt.Errorf("failed to unmarshal tags for metric %d: %w", id, err)
			}
		}

		// Generate dedup key using the fixed algorithm
		dedupKey := generateDedupKey(metric)

		// Update the row
		if _, err := stmt.Exec(dedupKey, id); err != nil {
			return fmt.Errorf("failed to update dedup_key for metric %d: %w", id, err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// Now create the unique index (after backfill is complete)
	if _, err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_dedup_key ON metrics(dedup_key)"); err != nil {
		return fmt.Errorf("failed to create unique index: %w", err)
	}

	return nil
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	return fmt.Sprintf("session_%d_%d", os.Getpid(), time.Now().UnixNano())
}

// generateDedupKey creates a unique deduplication key for a metric
// Uses JSON encoding to avoid delimiter collisions
// Format: sha256(json({name, timestamp_ms, device_id, tags, value_type}))
// Including value_type prevents collisions when a metric changes type (e.g., gauge→error string)
func generateDedupKey(metric *models.Metric) string {
	// Create a canonical structure for hashing
	// Using a struct ensures consistent field ordering
	type dedupData struct {
		Name        string            `json:"name"`
		TimestampMs int64             `json:"timestamp_ms"`
		DeviceID    string            `json:"device_id"`
		Tags        map[string]string `json:"tags"`
		ValueType   models.ValueType  `json:"value_type"` // Prevents type-change collisions
	}

	// Sort tags into a new map for canonical ordering
	sortedTags := make(map[string]string, len(metric.Tags))
	tagKeys := make([]string, 0, len(metric.Tags))
	for k := range metric.Tags {
		tagKeys = append(tagKeys, k)
	}
	sort.Strings(tagKeys)

	for _, k := range tagKeys {
		sortedTags[k] = metric.Tags[k]
	}

	data := dedupData{
		Name:        metric.Name,
		TimestampMs: metric.TimestampMs,
		DeviceID:    metric.DeviceID,
		Tags:        sortedTags,
		ValueType:   metric.ValueType,
	}

	// Marshal to JSON (which handles escaping properly)
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		// This should never happen with valid metric data, but fall back to a unique key
		// based on timestamp and a random component if it does
		jsonBytes = []byte(fmt.Sprintf("fallback_%d_%d", metric.TimestampMs, time.Now().UnixNano()))
	}

	// Hash the JSON representation
	hash := sha256.Sum256(jsonBytes)
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
			timestamp_ms, metric_name, metric_value, value_text, value_type,
			device_id, uploaded, priority, session_id, dedup_key, tags_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			metric.ValueText,
			int(metric.ValueType),
			metric.DeviceID,
			0, // uploaded = false
			1, // priority = normal
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
	query := "SELECT id, timestamp_ms, metric_name, metric_value, value_text, value_type, device_id, tags_json FROM metrics WHERE 1=1"
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
		var valueText sql.NullString
		var valueType int
		var id int64

		err := rows.Scan(
			&id,
			&m.TimestampMs,
			&m.Name,
			&m.Value,
			&valueText,
			&valueType,
			&m.DeviceID,
			&tagsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Set value_text and value_type
		if valueText.Valid {
			m.ValueText = valueText.String
		}
		m.ValueType = models.ValueType(valueType)

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
// Only returns numeric metrics (value_type=0) since VictoriaMetrics doesn't accept string metrics
// String metrics remain in SQLite for local event processing
func (s *SQLiteStorage) QueryUnuploaded(ctx context.Context, limit int) ([]*models.Metric, error) {
	query := `
		SELECT id, timestamp_ms, metric_name, metric_value, value_text, value_type, device_id, tags_json
		FROM metrics
		WHERE uploaded = 0 AND value_type = 0
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
		var valueText sql.NullString
		var valueType int
		var id int64

		err := rows.Scan(
			&id,
			&m.TimestampMs,
			&m.Name,
			&m.Value,
			&valueText,
			&valueType,
			&m.DeviceID,
			&tagsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Set value_text and value_type
		if valueText.Valid {
			m.ValueText = valueText.String
		}
		m.ValueType = models.ValueType(valueType)

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

// GetPendingCount returns the count of unuploaded numeric metrics
// Only counts value_type=0 (numeric) since string metrics are not uploaded to VictoriaMetrics
// This prevents string metrics from inflating the pending count and triggering false health degradation
func (s *SQLiteStorage) GetPendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM metrics WHERE uploaded = 0 AND value_type = 0").Scan(&count)
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

// normalizeDBPath strips SQLite URI components from a database path.
// Handles paths returned by PRAGMA database_list which may include:
// - file: prefix (e.g., "file:/var/lib/db.db")
// - Query parameters (e.g., "file:/var/lib/db.db?_busy_timeout=5000")
// - Multiple slashes (e.g., "file:///var/lib/db.db")
//
// Returns a clean filesystem path suitable for use with os.Stat.
// UNC paths (file://hostname/path) are preserved as //hostname/path.
func normalizeDBPath(dbPath string) string {
	path := dbPath

	// Handle SQLite URI format (file:...)
	if strings.HasPrefix(path, "file:") {
		// Strip "file:" prefix
		path = strings.TrimPrefix(path, "file:")

		// Strip any query parameters (everything after ?)
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}

		// Handle SQLite URI formats:
		// - file:///path (three slashes) -> /path (absolute Unix path)
		// - file://host/path (two slashes + host) -> //host/path (UNC path - keep both slashes!)
		// - file:/path (one slash) -> /path (absolute path)
		// - file:path (no slash) -> path (relative path)
		if strings.HasPrefix(path, "///") {
			// file:///absolute/path -> /absolute/path
			path = path[2:] // Remove two slashes, keep the third as leading /
		}
		// DO NOT strip slashes from UNC paths (//hostname/path)
		// They need to remain as //hostname/path for os.Stat to work correctly
	}

	return path
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

	// Normalize the path to strip URI components (file: prefix, query params)
	// This handles SQLite URIs like file:/path/db.db?_busy_timeout=5000
	normalizedPath := normalizeDBPath(path)

	// WAL file is database_path + "-wal"
	walPath := normalizedPath + "-wal"
	info, err := os.Stat(walPath)
	if os.IsNotExist(err) {
		return 0, nil // No WAL file yet
	}
	if err != nil {
		return 0, fmt.Errorf("failed to stat WAL file: %w", err)
	}

	return info.Size(), nil
}

// StartWALCheckpointRoutine starts a background goroutine that periodically
// checkpoints the WAL to prevent unbounded growth. It checkpoints:
// - Every hour (configurable via checkpointInterval)
// - When WAL size exceeds maxWALSize (default 64 MB), checked every 30 seconds
// Call the returned cancel function to stop the routine gracefully.
func (s *SQLiteStorage) StartWALCheckpointRoutine(ctx context.Context, logger *slog.Logger, checkpointInterval time.Duration, maxWALSize int64) context.CancelFunc {
	return s.startWALCheckpointRoutineWithSizeInterval(ctx, logger, checkpointInterval, maxWALSize, 30*time.Second)
}

// startWALCheckpointRoutineWithSizeInterval is the internal implementation that allows
// configuring the size check interval (useful for testing)
func (s *SQLiteStorage) startWALCheckpointRoutineWithSizeInterval(ctx context.Context, logger *slog.Logger, checkpointInterval time.Duration, maxWALSize int64, sizeCheckInterval time.Duration) context.CancelFunc {
	if checkpointInterval == 0 {
		checkpointInterval = 1 * time.Hour
	}
	if maxWALSize == 0 {
		maxWALSize = 64 * 1024 * 1024 // 64 MB default
	}
	if sizeCheckInterval == 0 {
		sizeCheckInterval = 30 * time.Second
	}

	routineCtx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(checkpointInterval)
		defer ticker.Stop()

		logger.Info("WAL checkpoint routine started",
			slog.Duration("interval", checkpointInterval),
			slog.Int64("max_wal_size_mb", maxWALSize/(1024*1024)),
			slog.Duration("size_check_interval", sizeCheckInterval),
		)

		// Check WAL size immediately on startup (before first ticker fires)
		// This handles cases where the process starts with an already-large WAL
		// (e.g., after a crash or unusually busy run)
		walSize, err := s.GetWALSize()
		if err != nil {
			logger.Error("Failed to get initial WAL size",
				slog.Any("error", err),
			)
		} else if walSize > maxWALSize {
			logger.Warn("WAL size exceeds threshold on startup, triggering immediate checkpoint",
				slog.Int64("wal_size_mb", walSize/(1024*1024)),
				slog.Int64("threshold_mb", maxWALSize/(1024*1024)),
			)
			s.performCheckpoint(logger, "startup-size-triggered")
		}

		// Create a separate ticker for size checks
		// This ensures we react quickly to WAL growth without waiting for periodic checkpoint
		sizeTicker := time.NewTicker(sizeCheckInterval)
		defer sizeTicker.Stop()

		for {
			select {
			case <-routineCtx.Done():
				logger.Info("WAL checkpoint routine stopping")
				return
			case <-ticker.C:
				// Periodic checkpoint (hourly by default)
				s.performCheckpoint(logger, "periodic")
			case <-sizeTicker.C:
				// Check WAL size and trigger emergency checkpoint if needed
				walSize, err := s.GetWALSize()
				if err != nil {
					logger.Error("Failed to get WAL size",
						slog.Any("error", err),
					)
					continue
				}

				if walSize > maxWALSize {
					logger.Warn("WAL size exceeds threshold, triggering emergency checkpoint",
						slog.Int64("wal_size_mb", walSize/(1024*1024)),
						slog.Int64("threshold_mb", maxWALSize/(1024*1024)),
					)
					s.performCheckpoint(logger, "size-triggered")
				}
			}
		}
	}()

	return cancel
}

// performCheckpoint executes a WAL checkpoint and logs the results
func (s *SQLiteStorage) performCheckpoint(logger *slog.Logger, reason string) {
	// Get WAL size before checkpoint
	walSizeBefore, err := s.GetWALSize()
	if err != nil {
		logger.Error("Failed to get WAL size before checkpoint",
			slog.String("reason", reason),
			slog.Any("error", err),
		)
		return
	}

	// Perform checkpoint
	startTime := time.Now()
	ctx := context.Background()
	err = s.CheckpointWAL(ctx)
	duration := time.Since(startTime)

	if err != nil {
		logger.Error("WAL checkpoint failed",
			slog.String("reason", reason),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.Any("error", err),
		)
		return
	}

	// Get WAL size after checkpoint
	walSizeAfter, err := s.GetWALSize()
	if err != nil {
		logger.Warn("Failed to get WAL size after checkpoint",
			slog.String("reason", reason),
			slog.Any("error", err),
		)
		walSizeAfter = 0
	}

	bytesReclaimed := walSizeBefore - walSizeAfter
	if bytesReclaimed < 0 {
		bytesReclaimed = 0
	}

	logger.Info("WAL checkpoint completed",
		slog.String("reason", reason),
		slog.Int64("duration_ms", duration.Milliseconds()),
		slog.Int64("wal_size_before_mb", walSizeBefore/(1024*1024)),
		slog.Int64("wal_size_after_mb", walSizeAfter/(1024*1024)),
		slog.Int64("bytes_reclaimed_kb", bytesReclaimed/1024),
	)
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
