# Milestone 2: Production Reliability + VictoriaMetrics

**Timeline:** 3-4 days (Days 3-6)
**Status:** In Progress
**Goal:** Fix critical bugs, add system metrics, integrate VictoriaMetrics, production-grade reliability
**Last Updated:** 2025-10-12

---

## Overview

Build on M1's end-to-end pipeline to create a production-ready monitoring system with:
- No duplicate uploads (fix P1 bug with deduplication key)
- Complete system health metrics with proper delta calculations
- VictoriaMetrics TSDB integration with chunked uploads
- Retry logic with jittered exponential backoff
- Health monitoring with graduated status levels
- Structured logging with comprehensive context
- Clock skew detection and WAL management
- Security hardening

All tested on macOS, ready to deploy to Orange Pi when hardware becomes available.

---

## Changes from Original M2 Plan

### Critical Additions from Technical Review

1. **Deduplication Key** - Add `dedup_key` column with unique index to prevent duplicates even on crashes/retries
2. **Partial-Success Uploads** - Chunked batches with per-chunk tracking for VM partial acks
3. **Jittered Backoff** - Add ±20% jitter to prevent thundering herd
4. **CPU Usage Delta Calculation** - Two-read strategy with cached counters for accurate percentages, handle wraparound
5. **Network Interface Filtering** - Exclude loopback/virtual interfaces, configurable regex includes/excludes
6. **Disk I/O Units** - Convert sectors to bytes (per-device sector size from sysfs: 512 for SATA, 4096 for NVMe/eMMC), expose both ops/s and bytes/s
7. **Clock Skew Detection** - Compare with VM Date header, expose `time.skew_ms` in meta-metrics, warn on >2s
8. **Active WAL Checkpoint** - Hourly `PRAGMA wal_checkpoint(TRUNCATE)` + size-based triggers (>64MB)
9. **Graduated Health Status** - Three-tier: ok/degraded/error with clear rollup rules and thresholds
10. **Enhanced Logging Fields** - Add session_id, batch_id, attempt, backoff_ms, http_status, bytes sent/rcvd

### Additional Improvements

- New index: `idx_name_dev_time` for faster queries
- Security hardening in systemd (NoNewPrivileges, ProtectSystem, MemoryMax=200M, CPUQuota=20%, etc.)
- Expanded test coverage (duplicate-proofing, partial-ack, clock skew, WAL growth, counter wraparound)
- Config additions (include_ifaces, exclude_ifaces with regex defaults)
- Better documentation (PromQL examples, health status meanings, per-zone temps)
- Minified JSON tags with canonical key order for dedup_key consistency
- Token file permissions (0600), non-root systemd user
- Watchdog integration for automatic restart on health failures

---

## Scope

### IN Scope ✅

**1. [P1] Fix Duplicate Upload Bug (Enhanced)**
- Upgrade SQLite schema to production version
- Add `uploaded`, `priority`, `session_id` fields
- **Add `dedup_key` with unique index** - sha256(name|ts|device|tags) prevents duplicates on crashes/retries
- Add `upload_checkpoints` table with batch tracking
- Track upload cursor/checkpoint per chunk
- Query only unuploaded metrics
- Mark metrics as uploaded after success
- **Handle partial success** - chunk-based tracking when VM accepts subset
- Comprehensive tests for no duplicates

**2. VictoriaMetrics Integration (Enhanced)**
- Deploy VictoriaMetrics single-node via Docker Compose
- Switch upload format from simple JSON to VictoriaMetrics JSONL import format
- Support `/api/v1/import` endpoint
- **Chunked uploads** - 50 metrics per chunk (~128-256 KB gzip) for better retry granularity
- Add gzip compression for uploads
- **Ordered by timestamp** within chunks for better compression
- Keep simple receiver for quick local testing
- Document VictoriaMetrics setup with PromQL examples

**3. Retry Logic & Error Handling (Enhanced)**
- Exponential backoff for failed uploads (5s, 15s, 45s, ...)
- **Add ±20% jitter** to prevent thundering herd on many devices
- Max retry attempts (configurable, default 3)
- Track failed upload attempts per chunk
- Graceful degradation on persistent failures
- Error wrapping with context
- Comprehensive error logging with retry metadata

**4. System Metrics Expansion (Enhanced)**
- **CPU usage** - Overall + per-core from `/proc/stat` with **delta calculation** (two-read, cached counters, wraparound handling)
- **Memory usage** - Used, available, swap from `/proc/meminfo`
- **Disk usage + I/O** - From `/proc/diskstats` with **sector→byte conversion** (per-device sector size: 512 for SATA, 4096 for NVMe/eMMC), expose ops/s and bytes/s
- **Network traffic** - Bytes TX/RX from `/proc/net/dev` with **interface filtering** (exclude lo, docker*, veth*, br-*)
- **All RK3588 thermal zones** - SoC, big cores, small cores, GPU, NPU (per-zone + max)
- Load averages (1m, 5m, 15m)
- System uptime

**5. Health Check Endpoint (Enhanced)**
- HTTP server on `:9100/health`
- **Graduated status calculation**: ok/degraded/error with clear thresholds
- Returns JSON with:
  - **Overall status** (ok if all healthy, degraded if 1+ collector error or high pending, error if critical)
  - Uptime (monotonic seconds)
  - Last successful collection timestamp per collector (RFC3339)
  - Last successful upload timestamp (RFC3339)
  - Database size
  - **WAL size**
  - Current error state per component
  - **Time skew** info
- Liveness probe: `/health/live` (200 if process running)
- Readiness probe: `/health/ready` (200 only if status=ok)

**6. Meta-Monitoring (Collector Self-Metrics) (Enhanced)**
Priority metrics to implement:
- `collector.metrics_collected_total` (counter per collector)
- `collector.metrics_failed_total` (counter per collector)
- `collector.collection_duration_seconds` (histogram per collector)
- `uploader.metrics_uploaded_total` (counter)
- `uploader.upload_failures_total` (counter)
- `uploader.upload_duration_seconds` (histogram)
- `uploader.partial_success_total` (counter) - **NEW**
- `storage.database_size_bytes` (gauge)
- `storage.wal_size_bytes` (gauge) - **NEW**
- `storage.metrics_pending_upload` (gauge)
- `time.skew_ms` (gauge) - **NEW**

Document for future:
- `collector.memory_usage_bytes`
- `collector.cpu_usage_percent`
- `storage.query_duration_seconds`
- `storage.write_duration_seconds`

**7. Structured Logging (Enhanced)**
- Migrate from `log` to `log/slog` (Go 1.22+)
- JSON output format for production
- Console output format for development
- Log levels: DEBUG, INFO, WARN, ERROR
- **Enhanced contextual fields**:
  - Collection: collector_name, count, duration_ms, session_id
  - Upload: **batch_id, chunk_index, attempt, backoff_ms**, count, endpoint, **http_status, bytes_sent, bytes_rcvd**, duration_ms
  - Error: error, error_type, stack (if panic)
- Configurable log level via config file

**8. Clock Skew Detection (NEW)**
- Compare local time with VictoriaMetrics Date header on startup
- Expose `time.skew_ms` in meta-metrics
- Log warning if skew > 2 seconds
- Periodic rechecking (configurable interval)

**9. Active WAL Management (NEW)**
- Hourly `PRAGMA wal_checkpoint(TRUNCATE)` background routine
- Size-based triggering when WAL > 64 MB
- Log checkpoint operations with size info
- Expose `storage.wal_size_bytes` in meta-metrics

**10. Security Hardening (NEW)**
- Non-root systemd user (`metrics:metrics`)
- NoNewPrivileges=true
- ProtectSystem=strict, ProtectHome=true
- PrivateTmp=true
- MemoryMax=200M, CPUQuota=20%
- RestrictAddressFamilies, RestrictNamespaces
- Token file permissions 0600
- Watchdog integration (60s)

### OUT of Scope ❌

**Deferred to Milestone 3:**
- Belabox/encoder metrics (need hardware access)
- HDMI input metrics (need hardware access)
- Server-side SRT stats (need receiver access)
- Priority queue implementation (P0/P1/P2/P3)
- Backfill after network recovery
- Data retention and rotation
- Grafana dashboard setup (VictoriaMetrics query only)
- TLS certificate pinning (basic TLS yes, pinning later)

---

## Success Criteria

Milestone 2 is complete when:

1. ✅ No duplicate uploads in 30-minute soak test (verified with dedup_key)
2. ✅ Same batch retried twice → 0 new rows (unique constraint test)
3. ✅ VictoriaMetrics ingesting metrics successfully
4. ✅ Can query metrics from VictoriaMetrics UI with PromQL
5. ✅ 8+ system metrics collecting reliably
6. ✅ CPU usage with proper delta calculation (no first-sample, wraparound handling)
7. ✅ Network metrics exclude lo, docker*, veth*, br-*
8. ✅ Disk I/O in bytes (not sectors)
9. ✅ Upload retries work with jittered backoff (tested with network simulation)
10. ✅ Partial success handled correctly (VM accepts 25/50 → only 25 marked uploaded)
11. ✅ Health endpoint returns graduated status (ok/degraded/error)
12. ✅ Clock skew detected and exposed in meta-metrics
13. ✅ WAL checkpoint runs and keeps WAL < 64 MB
14. ✅ Meta-metrics visible in VictoriaMetrics
15. ✅ Structured logs in JSON format with all required fields
16. ✅ All tests pass (80+ unit tests expected)
17. ✅ Resource usage <5% CPU, <150MB RAM
18. ✅ Docker Compose brings up full stack easily
19. ✅ Security hardening in place (non-root, protections, limits)
20. ✅ Documentation complete (health semantics, PromQL examples, per-zone temps)

---

## Technical Specification

### Database Schema (Production Version v2)

```sql
-- Production schema with upload tracking + deduplication

CREATE TABLE IF NOT EXISTS metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_ms INTEGER NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value REAL,
    value_text TEXT,              -- For string metrics
    value_type INTEGER DEFAULT 0, -- 0=real, 1=text
    tags TEXT,                     -- Minified JSON, canonical key order
    session_id TEXT,
    device_id TEXT,
    uploaded INTEGER DEFAULT 0,
    priority INTEGER DEFAULT 2,   -- 0=P0, 1=P1, 2=P2, 3=P3
    dedup_key TEXT NOT NULL        -- sha256(name|ts_ms|device|tags)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_timestamp ON metrics(timestamp_ms);
CREATE INDEX IF NOT EXISTS idx_metric_session ON metrics(metric_name, session_id, timestamp_ms);
-- Uploader hot path: include id for deterministic chunking and avoid starvation
CREATE INDEX IF NOT EXISTS idx_uploaded ON metrics(uploaded, priority, timestamp_ms, id);
CREATE INDEX IF NOT EXISTS idx_name_dev_time ON metrics(metric_name, device_id, timestamp_ms);

-- Unique constraint prevents duplicates even on crash/retry
CREATE UNIQUE INDEX IF NOT EXISTS ux_metrics_dedup ON metrics(dedup_key);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    start_time INTEGER NOT NULL,
    end_time INTEGER,
    status TEXT,  -- active, completed, failed
    metadata TEXT -- JSON
);

CREATE TABLE IF NOT EXISTS upload_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id TEXT NOT NULL,       -- UUID for batch tracking
    chunk_index INTEGER NOT NULL, -- 0-based chunk number
    last_uploaded_id INTEGER,
    last_uploaded_timestamp INTEGER,
    upload_time INTEGER,
    metrics_count INTEGER         -- Metrics in this chunk
);

CREATE TABLE IF NOT EXISTS schema_version (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version INTEGER NOT NULL,
    applied_at TEXT NOT NULL
);
```

### SQLite Connection Pool & Pragmas

```go
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, err
    }

    // ARM SBC optimization: single connection prevents lock contention
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(1)
    db.SetConnMaxLifetime(0)  // Reuse connection indefinitely

    // Essential WAL pragmas for ARM SBC
    pragmas := []string{
        "PRAGMA journal_mode=WAL",           // Concurrent reads + crash safety
        "PRAGMA synchronous=NORMAL",         // Balanced durability (1-2 tx at risk on power loss)
        "PRAGMA busy_timeout=5000",          // Wait up to 5s on SQLITE_BUSY before returning error
        "PRAGMA temp_store=MEMORY",          // Reduce SSD wear on temp tables
        "PRAGMA mmap_size=268435456",        // 256 MB mmap for faster reads (adjust if RAM constrained)
        "PRAGMA cache_size=-65536",          // 64 MB cache (negative = KB)
    }

    for _, pragma := range pragmas {
        if _, err := db.Exec(pragma); err != nil {
            db.Close()
            return nil, fmt.Errorf("failed to set pragma %s: %w", pragma, err)
        }
    }

    return &SQLiteStorage{db: db, dbPath: dbPath}, nil
}
```

**Why these settings:**
- **MaxOpenConns=1**: Single writer model avoids SQLITE_BUSY errors on ARM SBCs
- **WAL mode**: Enables concurrent readers while writer works, crash-safe
- **synchronous=NORMAL**: 1-2 uncommitted transactions at risk on power loss (acceptable for metrics)
- **busy_timeout=5000ms**: Retry lock acquisition for 5s before failing (handles brief contention)
- **temp_store=MEMORY**: Reduces eMMC/SD card wear from temp tables/sorts
- **mmap_size=256MB**: Memory-mapped I/O for faster reads (set to 128MB if RAM < 2GB)
- **cache_size=64MB**: Page cache size (adjust based on available RAM)

### Deduplication Key Generation

```go
// CanonicalizeTags returns a canonical string representation of tags
// and the sorted key order. This ensures consistent hashing across
// all metric creation paths.
func CanonicalizeTags(tags map[string]string) (string, []string) {
    if len(tags) == 0 {
        return "", nil
    }

    keys := make([]string, 0, len(tags))
    for k := range tags {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    var buf strings.Builder
    for i, k := range keys {
        if i > 0 {
            buf.WriteByte(',')
        }
        buf.WriteString(k)
        buf.WriteByte('=')
        buf.WriteString(tags[k])
    }

    return buf.String(), keys
}

func (m *Metric) GenerateDedupKey() string {
    // Canonical representation - MUST use canonical tag order
    // to ensure consistent hashing across all creation paths
    //
    // Key components:
    // 1. name - metric identifier
    // 2. timestamp_ms - temporal uniqueness
    // 3. device_id - source device
    // 4. canonical_tags - sorted key=value pairs
    // 5. value_type - prevents collisions when metric changes type (e.g., gauge→string)
    parts := []string{
        m.Name,
        strconv.FormatInt(m.TimestampMs, 10),
        m.DeviceID,
    }

    // Add tags in canonical form
    if len(m.Tags) > 0 {
        canonicalTags, _ := CanonicalizeTags(m.Tags)
        parts = append(parts, canonicalTags)
    }

    // Add value_type to prevent collisions on type changes
    // E.g., a metric that flip-flops between numeric gauge and error string
    parts = append(parts, strconv.Itoa(m.ValueType))

    // Hash - normalize floats (no NaN/Inf before hashing if value is included in key)
    data := strings.Join(parts, "|")
    hash := sha256.Sum256([]byte(data))
    return hex.EncodeToString(hash[:])
}

// NewMetric constructor enforces canonical tag ordering
func NewMetric(name string, value float64, deviceID string) *Metric {
    ts := time.Now().UnixMilli()

    // Timestamp validation: reject if clock is wildly wrong
    // Allow future timestamps up to 5 minutes (clock skew, NTP correction)
    // Reject past timestamps older than 1 hour (prevents replays from stale data)
    now := time.Now().UnixMilli()
    if ts > now+300000 {  // +5 minutes
        log.Warn("Timestamp in far future, clamping to now",
            "original_ts", ts,
            "clamped_ts", now,
            "skew_ms", ts-now)
        ts = now
    } else if ts < now-3600000 {  // -1 hour
        log.Warn("Timestamp in far past, clamping to now",
            "original_ts", ts,
            "clamped_ts", now,
            "skew_ms", now-ts)
        ts = now
    }

    return &Metric{
        Name:        name,
        Value:       value,
        DeviceID:    deviceID,
        TimestampMs: ts,
        Tags:        make(map[string]string),
    }
}

// WithTag adds a tag and regenerates dedup_key with canonical ordering
func (m *Metric) WithTag(key, value string) *Metric {
    m.Tags[key] = value
    // Dedup key will be generated just before storage
    return m
}

// Finalize generates the dedup_key - call before storing
func (m *Metric) Finalize() {
    m.DedupKey = m.GenerateDedupKey()
}
```

### Migration Strategy

```go
// GetSchemaVersion returns current version (1=M1, 2=M2)
func GetSchemaVersion(db *sql.DB) (int, error)

// Migrate runs all pending migrations
func Migrate(ctx context.Context, db *sql.DB) error

// Migration v1→v2:
// 1. Create schema_version table if missing
// 2. ALTER TABLE metrics ADD COLUMN for new fields
// 3. CREATE new tables (sessions, upload_checkpoints)
// 4. CREATE new indexes
// 5. Backfill dedup_key for existing rows
// 6. UPDATE schema_version to 2
```

### VictoriaMetrics JSONL Format

```jsonl
{"metric":{"__name__":"cpu_temperature_celsius","device_id":"belabox-001"},"values":[52.3],"timestamps":[1697040000000]}
{"metric":{"__name__":"cpu_usage_percent","device_id":"belabox-001","core":"0"},"values":[45.2],"timestamps":[1697040000123]}
{"metric":{"__name__":"memory_used_bytes","device_id":"belabox-001"},"values":[1073741824],"timestamps":[1697040000456]}
{"metric":{"__name__":"disk_read_bytes_total","device_id":"belabox-001","device":"sda"},"values":[1048576000],"timestamps":[1697040000789]}
```

**Key points:**
- One JSON object per line (JSONL)
- **Metric names are PromQL-safe**: dots replaced with underscores, conform to `[a-zA-Z_:][a-zA-Z0-9_:]*`
- Metric name in `__name__` label
- Tags as additional labels in `metric` object
- Unix millisecond timestamps
- Values as arrays (supports multiple points, but we send 1 per line)
- Ordered by timestamp_ms ASC within chunk for better compression

**CRITICAL:** Metric names **MUST** be PromQL-safe in VictoriaMetrics. Prometheus/PromQL does not allow dots (`.`) in metric names - they cause parsing failures and break queries. All uploaded metrics MUST use underscores.

**Important:** Metric name sanitization happens **only in the upload path** (JSONL builder). The on-disk SQLite `metric_name` field keeps the original dotted names to avoid migration churn. This is a mandatory presentation-layer transformation for PromQL compatibility.

### Metric Name Sanitization (PromQL Compatibility - MANDATORY)

```go
// PromQL-safe metric names: [a-zA-Z_:][a-zA-Z0-9_:]*
var metricNameRE = regexp.MustCompile(`[^a-zA-Z0-9_:]`)

// sanitizeMetricName converts internal metric names to PromQL-safe format
// Applied ONLY in the upload path (JSONL builder), not in storage
func sanitizeMetricName(name string) string {
    // Replace dots with underscores (cpu.temperature → cpu_temperature)
    name = strings.ReplaceAll(name, ".", "_")

    // Remove any remaining invalid characters
    name = metricNameRE.ReplaceAllString(name, "_")

    // Optional: map known metric families to unit-suffixed names
    // This ensures Prometheus/PromQL best practices
    name = normalizeUnitSuffix(name)

    return name
}

// normalizeUnitSuffix ensures metrics follow Prometheus naming conventions
func normalizeUnitSuffix(name string) string {
    // Add unit suffixes if missing
    switch {
    case strings.HasPrefix(name, "cpu_temperature") && !strings.HasSuffix(name, "_celsius"):
        return name + "_celsius"
    case strings.HasPrefix(name, "thermal_") && !strings.HasSuffix(name, "_celsius"):
        return name + "_celsius"
    case strings.Contains(name, "_bytes") || strings.Contains(name, "_ops"):
        // Already has unit suffix
        return name
    }
    return name
}

// buildJSONL constructs VictoriaMetrics JSONL with sanitized metric names
func buildJSONL(metrics []*models.Metric) ([]byte, error) {
    var buf bytes.Buffer

    for _, m := range metrics {
        // IMPORTANT: Skip string metrics (value_type=1)
        // VictoriaMetrics only accepts numeric samples
        // String metrics should be transformed into limited-cardinality labels
        // or stored as local events/logs
        if m.ValueType == 1 {
            log.Debug("Skipping string metric (not sent to VM)",
                "metric_name", m.Name,
                "value_text", m.ValueText)
            continue
        }

        // Sanitize metric name for PromQL compatibility
        safeName := sanitizeMetricName(m.Name)

        // Build JSONL line
        line := map[string]interface{}{
            "metric": map[string]string{
                "__name__":  safeName,
                "device_id": m.DeviceID,
                // ... add tags
            },
            "values":     []float64{m.Value},
            "timestamps": []int64{m.TimestampMs},
        }

        jsonLine, _ := json.Marshal(line)
        buf.Write(jsonLine)
        buf.WriteByte('\n')
    }

    return buf.Bytes(), nil
}
```

**Why this is MANDATORY:**
- **PromQL will FAIL to parse** metric names with dots - this is not optional
- Grafana queries will break with syntax errors
- VictoriaMetrics may accept dots on ingest but PromQL queries will fail
- This is a blocking production issue - queries like `rate(cpu.usage_percent[5m])` will not work
- Unit suffixes (`_bytes`, `_celsius`, `_total`, `_percent`) follow Prometheus naming conventions
- Enforcing `[a-zA-Z_:][a-zA-Z0-9_:]*` regex ensures compatibility with all Prom tooling

**Percent vs Ratio Convention:**
- **DECISION**: Use `_percent` suffix with values 0-100 (not ratios 0-1)
- **Rationale**: Matches industry practice for SBC/embedded metrics; easier human readability
- **Examples**: `cpu_usage_percent` (45.2 = 45.2%), `packet_loss_percent` (0.5 = 0.5%)
- **Consistency**: All percentage metrics use this convention across collectors
- **PromQL adjustments**: Divide by 100 if ratio needed: `cpu_usage_percent / 100`

**Examples:**
- `cpu.temperature` → `cpu_temperature_celsius`
- `cpu.usage_percent` → `cpu_usage_percent`
- `memory.used_bytes` → `memory_used_bytes`
- `disk.read_bytes_total` → `disk_read_bytes_total`
- `network.tx_bytes_total` → `network_tx_bytes_total`
- `thermal.zone_temp` → `thermal_zone_temp_celsius`

**Storage remains unchanged:**
- SQLite `metric_name` field keeps original dotted names (for readability)
- No migration required
- Only the upload/JSONL builder applies sanitization
- Queries in SQLite still use dotted names

**Enforcement:**
- CI check: Parse all JSONL output, reject any `__name__` with dots
- Test: `TestMetricNameSanitization` verifies regex compliance
- All PromQL examples in docs use underscores only (no dots)

### Chunked Upload Strategy

```go
// Query unuploaded metrics with deterministic ordering
// SQL: SELECT * FROM metrics
//      WHERE uploaded = 0
//      ORDER BY priority ASC, timestamp_ms ASC, id ASC
//      LIMIT 2500
// Covered by index: idx_uploaded(uploaded, priority, timestamp_ms, id)
unuploaded := store.GetUnuploadedMetrics(ctx, limit=2500)

// Split into chunks of ~50 metrics each
// Target: 128-256 KB gzipped payload per chunk
chunks := splitIntoChunks(unuploaded, chunkSize=50)

batchID := uuid.New().String()

for chunkIndex, chunk := range chunks {
    // Build JSONL payload, sort by timestamp ASC
    sort.Slice(chunk, func(i, j int) bool {
        return chunk[i].TimestampMs < chunk[j].TimestampMs
    })

    jsonl := buildJSONL(chunk)
    gzipped := gzipCompress(jsonl, gzip.BestSpeed)  // Level 1-2 for ARM SBC efficiency

    // Upload chunk with proper headers
    req, err := http.NewRequest("POST", vmURL, bytes.NewReader(gzipped))
    if err != nil {
        log.Error("Failed to create upload request", "error", err)
        continue
    }
    req.Header.Set("Content-Type", "application/x-ndjson")
    req.Header.Set("Content-Encoding", "gzip")
    req.Header.Set("User-Agent", "belabox-metrics/"+version)
    req.Header.Set("X-Device-ID", cfg.Device.ID)

    resp, err := httpClient.Do(req)
    if err != nil {
        // Transport error - retry with jittered backoff
        continue
    }
    defer resp.Body.Close()

    // HTTP status code semantics for retry decisions
    // Retryable: 429 (rate limit), 503 (unavailable), 5xx (server error), EOF, timeout
    // Non-retryable: 400 (bad request), 401 (unauthorized), 403 (forbidden), 404 (not found)

    switch {
    case resp.StatusCode == 429 || resp.StatusCode == 503:
        // Rate limited or temporarily unavailable - respect Retry-After if present
        retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
        log.Warn("Rate limited or unavailable, retrying",
            "status", resp.StatusCode,
            "retry_after", retryAfter,
            "chunk_index", chunkIndex)
        // Use max(retryAfter, jitteredBackoff) for next attempt
        continue

    case resp.StatusCode >= 500:
        // Server error - retryable
        log.Warn("Server error, retrying",
            "status", resp.StatusCode,
            "chunk_index", chunkIndex)
        continue

    case resp.StatusCode >= 400 && resp.StatusCode < 500:
        // Client error (except 429) - non-retryable
        log.Error("Client error, not retrying",
            "status", resp.StatusCode,
            "chunk_index", chunkIndex,
            "response", readBody(resp))
        break  // Don't retry 4xx except 429
    }

    // Simplified partial success handling
    // Strategy: 2xx = mark ENTIRE chunk as uploaded
    // Rely on dedup_key unique index to prevent duplicates on replay
    // Keep chunks small (50 metrics) to bound blast radius on failure
    //
    // Why simplified:
    // - VictoriaMetrics /api/v1/import rarely provides per-line acceptance detail
    // - Parsing "accepted count" adds complexity and state tracking
    // - dedup_key unique constraint ensures replays don't create duplicates
    // - Small chunks mean retries are cheap and bounded
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        // 2xx = success for entire chunk
        // Mark all metrics in chunk as uploaded
        allChunkIDs := make([]int64, len(chunk))
        for i, m := range chunk {
            allChunkIDs[i] = m.ID
        }
        store.MarkUploaded(ctx, allChunkIDs, batchID, chunkIndex)

        // Save checkpoint
        store.SaveCheckpoint(ctx, Checkpoint{
            BatchID: batchID,
            ChunkIndex: chunkIndex,
            LastUploadedID: chunk[len(chunk)-1].ID,
            LastUploadedTimestamp: chunk[len(chunk)-1].TimestampMs,
            UploadTime: time.Now().UnixMilli(),
            MetricsCount: len(chunk),
        })

        // Log
        log.Info("Upload chunk succeeded",
            "batch_id", batchID,
            "chunk_index", chunkIndex,
            "count", len(chunk),
            "bytes_sent", len(gzipped))
    }
}
```

```go
// MarkUploaded sets uploaded=1 for metrics, respecting SQLite variable limits
// SQLite default: 999 variables; we use ≤500 IDs per batch for safety
func (s *SQLiteStorage) MarkUploaded(ctx context.Context, ids []int64, batchID string, chunkIndex int) error {
    const batchSize = 500

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Process in batches of ≤500 IDs
    for i := 0; i < len(ids); i += batchSize {
        end := i + batchSize
        if end > len(ids) {
            end = len(ids)
        }
        batch := ids[i:end]

        // Build placeholders: UPDATE ... WHERE id IN (?,?,?...)
        placeholders := make([]string, len(batch))
        args := make([]interface{}, len(batch)+2)
        for j := range batch {
            placeholders[j] = "?"
            args[j] = batch[j]
        }
        args[len(batch)] = batchID
        args[len(batch)+1] = chunkIndex

        query := fmt.Sprintf(
            "UPDATE metrics SET uploaded = 1, batch_id = ?, chunk_index = ? WHERE id IN (%s)",
            strings.Join(placeholders, ","))

        if _, err := tx.ExecContext(ctx, query, args...); err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

**Benefits:**
- Smaller retry units (50 metrics vs 2500)
- Partial success tracking
- Better compression with sorted timestamps
- Progress checkpointing

### HTTP Transport Configuration

```go
// NewHTTPClient creates a client optimized for ARM SBC deployment
// with connection reuse to avoid TIME_WAIT churn
func NewHTTPClient(cfg *config.RemoteConfig) *http.Client {
    tr := &http.Transport{
        // Connection pooling to reuse connections
        MaxIdleConns:        8,
        MaxIdleConnsPerHost: 8,
        IdleConnTimeout:     90 * time.Second,

        // Disable built-in compression (we gzip the body ourselves)
        DisableCompression:  true,

        // Timeouts for connection establishment
        DialContext: (&net.Dialer{
            Timeout:   10 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,

        // TLS handshake timeout
        TLSHandshakeTimeout: 10 * time.Second,

        // Response header timeout
        ResponseHeaderTimeout: 30 * time.Second,

        // Expect continue timeout
        ExpectContinueTimeout: 1 * time.Second,
    }

    return &http.Client{
        Transport: tr,
        Timeout:   cfg.Timeout,  // Overall request timeout
    }
}

// gzipCompress compresses payload with BestSpeed level
// for optimal CPU/energy tradeoff on ARM SBCs
func gzipCompress(data []byte) ([]byte, error) {
    var buf bytes.Buffer

    // BestSpeed (level 1-2) reduces CPU usage significantly
    // with minimal increase in compressed size (~5-10%)
    gw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
    if err != nil {
        return nil, err
    }

    if _, err := gw.Write(data); err != nil {
        gw.Close()
        return nil, err
    }

    if err := gw.Close(); err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}
```

**Why these settings:**
- **Connection pooling**: Reduces TIME_WAIT socket churn on resource-constrained SBCs
- **DisableCompression**: We control gzip level ourselves for ARM optimization
- **BestSpeed gzip**: 10-20% lower CPU usage vs DefaultCompression, ~5-10% larger payloads
- **Idle timeout 90s**: Balances connection reuse with server keep-alive limits
- **MaxIdleConns 8**: Sufficient for single-threaded uploader, low memory overhead

### Retry Logic with Jittered Backoff

```go
type RetryConfig struct {
    Enabled           bool
    MaxAttempts       int
    InitialBackoff    time.Duration
    MaxBackoff        time.Duration
    BackoffMultiplier int
    JitterPercent     int  // ±20%
}

func init() {
    // Seed rand once at process start for jitter randomness
    rand.Seed(time.Now().UnixNano())
}

func calculateBackoff(attempt int, cfg RetryConfig) time.Duration {
    base := cfg.InitialBackoff

    // Exponential growth
    for i := 1; i < attempt; i++ {
        base = base * time.Duration(cfg.BackoffMultiplier)
        if base > cfg.MaxBackoff {
            base = cfg.MaxBackoff
            break
        }
    }

    // Add jitter: ±20%
    // rand.Seed() called in init() ensures different jitter across process restarts
    jitterRange := float64(base) * (float64(cfg.JitterPercent) / 100.0)
    jitter := (rand.Float64()*2 - 1) * jitterRange  // -20% to +20%

    return base + time.Duration(jitter)
}

// parseRetryAfter parses Retry-After header (seconds or HTTP-date)
// Returns 0 if header is missing or invalid
func parseRetryAfter(header string) time.Duration {
    if header == "" {
        return 0
    }

    // Try parsing as integer seconds first
    if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
        return time.Duration(secs) * time.Second
    }

    // Try parsing as HTTP-date (RFC1123, RFC850, ANSI C)
    for _, layout := range []string{time.RFC1123, time.RFC850, time.ANSIC} {
        if t, err := time.Parse(layout, header); err == nil {
            delta := time.Until(t)
            if delta > 0 {
                return delta
            }
            return 0  // Date in past
        }
    }

    return 0  // Invalid format
}
```

**Example backoffs with jitter:**
- Attempt 1: 5s ± 20% = 4.0-6.0s
- Attempt 2: 15s ± 20% = 12.0-18.0s
- Attempt 3: 45s ± 20% = 36.0-54.0s

**Why jitter?**
Prevents thundering herd when many devices lose connectivity simultaneously and retry at same intervals.

### CPU Usage Collector (Delta Calculation)

```go
type CPUCollector struct {
    deviceID     string
    lastStats    map[string]cpuStat  // Cache per core
    lastReadTime time.Time
}

type cpuStat struct {
    user    uint64
    nice    uint64
    system  uint64
    idle    uint64
    iowait  uint64
    irq     uint64
    softirq uint64
    total   uint64
}

func (c *CPUCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
    // Read /proc/stat
    currentStats, err := readProcStat()
    if err != nil {
        return nil, err
    }

    currentTime := time.Now()

    // If no previous stats, save and return nil (skip first sample)
    if c.lastStats == nil {
        c.lastStats = currentStats
        c.lastReadTime = currentTime
        return nil, nil  // No metrics yet
    }

    var metrics []*models.Metric

    // Track aggregate CPU for overall system usage
    var aggregateDeltaTotal uint64
    var aggregateDeltaActive uint64

    // Calculate deltas for each core
    for core, current := range currentStats {
        last, exists := c.lastStats[core]
        if !exists {
            continue  // New core appeared?
        }

        // Check for counter wraparound
        if current.total < last.total {
            log.Warn("CPU counter wraparound detected, skipping sample", "core", core)
            continue  // Skip this sample
        }

        deltaTotal := current.total - last.total
        if deltaTotal == 0 {
            continue  // No time passed, avoid division by zero
        }

        deltaActive := (current.total - current.idle) - (last.total - last.idle)
        usagePercent := (float64(deltaActive) / float64(deltaTotal)) * 100.0

        // Per-core metric
        m := models.NewMetric("cpu.usage_percent", usagePercent, c.deviceID).
            WithTag("core", core)
        metrics = append(metrics, m)

        // Accumulate for aggregate (skip the "cpu" line if present)
        if core != "cpu" {
            aggregateDeltaTotal += deltaTotal
            aggregateDeltaActive += deltaActive
        }
    }

    // Add overall CPU usage metric (aggregate of all cores)
    if aggregateDeltaTotal > 0 {
        overallUsage := (float64(aggregateDeltaActive) / float64(aggregateDeltaTotal)) * 100.0
        overallMetric := models.NewMetric("cpu.usage_percent", overallUsage, c.deviceID).
            WithTag("core", "all")
        metrics = append(metrics, overallMetric)
    }

    // Update cache
    c.lastStats = currentStats
    c.lastReadTime = currentTime

    return metrics, nil
}

func readProcStat() (map[string]cpuStat, error) {
    data, err := os.ReadFile("/proc/stat")
    if err != nil {
        return nil, err
    }

    stats := make(map[string]cpuStat)

    for _, line := range strings.Split(string(data), "\n") {
        if !strings.HasPrefix(line, "cpu") {
            continue
        }

        fields := strings.Fields(line)
        if len(fields) < 8 {
            continue
        }

        core := fields[0]  // "cpu", "cpu0", "cpu1", ...

        user, _ := strconv.ParseUint(fields[1], 10, 64)
        nice, _ := strconv.ParseUint(fields[2], 10, 64)
        system, _ := strconv.ParseUint(fields[3], 10, 64)
        idle, _ := strconv.ParseUint(fields[4], 10, 64)
        iowait, _ := strconv.ParseUint(fields[5], 10, 64)
        irq, _ := strconv.ParseUint(fields[6], 10, 64)
        softirq, _ := strconv.ParseUint(fields[7], 10, 64)

        total := user + nice + system + idle + iowait + irq + softirq

        stats[core] = cpuStat{
            user:    user,
            nice:    nice,
            system:  system,
            idle:    idle,
            iowait:  iowait,
            irq:     irq,
            softirq: softirq,
            total:   total,
        }
    }

    return stats, nil
}
```

**Key points:**
- Two reads required for accurate delta
- First sample skipped (no previous to compare)
- Wraparound detection (skip sample if current < last)
- Per-core tracking in separate metrics
- Division by zero protection

### Memory Collector (Canonical Used Calculation)

```go
type MemoryCollector struct {
    deviceID string
}

type memInfo struct {
    total       uint64  // MemTotal
    available   uint64  // MemAvailable
    swapTotal   uint64  // SwapTotal
    swapFree    uint64  // SwapFree
}

// usedBytes returns canonical "used memory" calculation
// per Linux kernel documentation: MemTotal - MemAvailable
// This accounts for buffers/cache that can be reclaimed
func (m memInfo) usedBytes() uint64 {
    return m.total - m.available
}

func (m memInfo) swapUsedBytes() uint64 {
    return m.swapTotal - m.swapFree
}

func (c *MemoryCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
    data, err := os.ReadFile("/proc/meminfo")
    if err != nil {
        return nil, err
    }

    info := parseMeminfo(data)

    metrics := []*models.Metric{
        // Used memory (single source of truth)
        models.NewMetric("memory.used_bytes", float64(info.usedBytes()), c.deviceID),

        // Available memory (for capacity planning)
        models.NewMetric("memory.available_bytes", float64(info.available), c.deviceID),

        // Swap usage
        models.NewMetric("memory.swap_used_bytes", float64(info.swapUsedBytes()), c.deviceID),

        // Total capacities (for percentage calculations in queries)
        models.NewMetric("memory.total_bytes", float64(info.total), c.deviceID),
        models.NewMetric("memory.swap_total_bytes", float64(info.swapTotal), c.deviceID),
    }

    return metrics, nil
}

func parseMeminfo(data []byte) memInfo {
    var info memInfo

    for _, line := range strings.Split(string(data), "\n") {
        fields := strings.Fields(line)
        if len(fields) < 2 {
            continue
        }

        key := strings.TrimSuffix(fields[0], ":")
        valueKB, _ := strconv.ParseUint(fields[1], 10, 64)
        valueBytes := valueKB * 1024

        switch key {
        case "MemTotal":
            info.total = valueBytes
        case "MemAvailable":
            info.available = valueBytes
        case "SwapTotal":
            info.swapTotal = valueBytes
        case "SwapFree":
            info.swapFree = valueBytes
        }
    }

    return info
}
```

**Key points:**
- **Canonical used calculation**: `MemTotal - MemAvailable` per Linux kernel docs
- **No redundant metrics**: Avoid exporting both "used" and "free" (causes dashboard math errors)
- **Unit test required**: Verify used calculation matches kernel semantics
- **MemAvailable**: Best estimate of memory available without swapping (includes reclaimable buffers/cache)

### Network Collector (Interface Filtering + Wraparound)

```go
type NetworkCollector struct {
    deviceID        string
    includePatterns []*regexp.Regexp
    excludePatterns []*regexp.Regexp
    lastStats       map[string]netStat  // Cache for wraparound detection
    maxInterfaces   int                  // Hard cap to prevent cardinality explosion
    droppedCount    int64                // Count of interfaces dropped due to limit
}

type netStat struct {
    rxBytes   uint64
    txBytes   uint64
    rxPackets uint64
    txPackets uint64
    rxErrors  uint64
    txErrors  uint64
}

func NewNetworkCollector(deviceID string, includes, excludes []string, maxInterfaces int) (*NetworkCollector, error) {
    c := &NetworkCollector{
        deviceID:      deviceID,
        maxInterfaces: maxInterfaces,
    }

    for _, pattern := range includes {
        re, err := regexp.Compile(pattern)
        if err != nil {
            return nil, fmt.Errorf("invalid include pattern %s: %w", pattern, err)
        }
        c.includePatterns = append(c.includePatterns, re)
    }

    for _, pattern := range excludes {
        re, err := regexp.Compile(pattern)
        if err != nil {
            return nil, fmt.Errorf("invalid exclude pattern %s: %w", pattern, err)
        }
        c.excludePatterns = append(c.excludePatterns, re)
    }

    return c, nil
}

func (c *NetworkCollector) shouldInclude(iface string) bool {
    // Check exclusions first
    for _, pattern := range c.excludePatterns {
        if pattern.MatchString(iface) {
            return false
        }
    }

    // If include list is empty, include all (except excluded)
    if len(c.includePatterns) == 0 {
        return true
    }

    // Check includes
    for _, pattern := range c.includePatterns {
        if pattern.MatchString(iface) {
            return true
        }
    }

    return false
}

func (c *NetworkCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
    data, err := os.ReadFile("/proc/net/dev")
    if err != nil {
        return nil, err
    }

    currentStats := make(map[string]netStat)
    var metrics []*models.Metric

    for _, line := range strings.Split(string(data), "\n") {
        if !strings.Contains(line, ":") {
            continue  // Skip header lines
        }

        parts := strings.Split(line, ":")
        iface := strings.TrimSpace(parts[0])

        if !c.shouldInclude(iface) {
            continue
        }

        // Cardinality guard: enforce hard cap on interface count
        if len(currentStats) >= c.maxInterfaces {
            // Skip this interface, increment drop counter
            atomic.AddInt64(&c.droppedCount, 1)
            log.Warn("Interface limit reached, dropping interface from collection",
                "interface", iface,
                "limit", c.maxInterfaces,
                "dropped_total", atomic.LoadInt64(&c.droppedCount))
            continue
        }

        fields := strings.Fields(parts[1])
        if len(fields) < 16 {
            continue
        }

        // Parse current stats
        current := netStat{
            rxBytes:   parseUint64(fields[0]),
            rxPackets: parseUint64(fields[1]),
            rxErrors:  parseUint64(fields[2]),
            txBytes:   parseUint64(fields[8]),
            txPackets: parseUint64(fields[9]),
            txErrors:  parseUint64(fields[10]),
        }
        currentStats[iface] = current

        // Check for counter wraparound (modern kernels use 64-bit, but guard anyway)
        if c.lastStats != nil {
            if last, exists := c.lastStats[iface]; exists {
                if current.rxBytes < last.rxBytes || current.txBytes < last.txBytes {
                    log.Warn("Network counter wraparound detected, skipping sample",
                        "interface", iface,
                        "rx_bytes_delta", int64(current.rxBytes)-int64(last.rxBytes),
                        "tx_bytes_delta", int64(current.txBytes)-int64(last.txBytes))
                    continue  // Skip this sample
                }
            }
        }

        // RX bytes (counter)
        metrics = append(metrics,
            models.NewMetric("network.rx_bytes_total", float64(current.rxBytes), c.deviceID).
                WithTag("interface", iface))

        // TX bytes (counter)
        metrics = append(metrics,
            models.NewMetric("network.tx_bytes_total", float64(current.txBytes), c.deviceID).
                WithTag("interface", iface))

        // RX packets (counter)
        metrics = append(metrics,
            models.NewMetric("network.rx_packets_total", float64(current.rxPackets), c.deviceID).
                WithTag("interface", iface))

        // TX packets (counter)
        metrics = append(metrics,
            models.NewMetric("network.tx_packets_total", float64(current.txPackets), c.deviceID).
                WithTag("interface", iface))

        // RX errors (counter)
        metrics = append(metrics,
            models.NewMetric("network.rx_errors_total", float64(current.rxErrors), c.deviceID).
                WithTag("interface", iface))

        // TX errors (counter)
        metrics = append(metrics,
            models.NewMetric("network.tx_errors_total", float64(current.txErrors), c.deviceID).
                WithTag("interface", iface))
    }

    // Emit meta-metric for dropped interfaces (cardinality guard)
    droppedTotal := atomic.LoadInt64(&c.droppedCount)
    if droppedTotal > 0 {
        metrics = append(metrics,
            models.NewMetric("network.interfaces_dropped_total", float64(droppedTotal), c.deviceID))
    }

    // Update cache for next collection
    c.lastStats = currentStats

    return metrics, nil
}

func parseUint64(s string) uint64 {
    v, _ := strconv.ParseUint(s, 10, 64)
    return v
}
```

**Key points:**
- **Counter wraparound detection**: Similar to CPU collector, checks if current < last
- Modern Linux kernels use 64-bit counters, but guard anyway for older kernels
- Caches previous stats per interface
- Skips sample on wraparound (logs warning with deltas)
- All metrics are counters (`_total` suffix) for rate() queries in PromQL
- **Cardinality guard**: Hard cap on interface count (default 32) to prevent label explosion
  - Emits `network.interfaces_dropped_total` counter when limit is hit
  - Use this metric to alert on cardinality issues

**Default exclusions:**
- `lo` - loopback
- `docker.*` - Docker bridge networks
- `veth.*` - Docker virtual ethernet
- `br-.*` - Linux bridges
- `wlan.*mon` - Wireless monitor mode interfaces
- `virbr.*` - Virtual bridge interfaces (libvirt)

### Disk I/O Collector (Sector→Byte Conversion)

```go
// Regex to match whole block devices (not partitions)
// Matches: sda, sdb, nvme0n1, nvme1n1, mmcblk0, etc.
// Skips: sda1, nvme0n1p1, mmcblk0p1, etc.
var wholeDevicePattern = regexp.MustCompile(`^(sd[a-z]+|nvme\d+n\d+|mmcblk\d+)$`)

// Per-device sector size cache (NVMe/eMMC may use 4096, SATA uses 512)
var sectorSizeCache sync.Map // device name -> int64 bytes

// getSectorSize reads logical block size for a device
// Modern NVMe uses 4096-byte sectors, SATA/SCSI use 512
// Caches results to avoid repeated sysfs reads
func getSectorSize(device string) int64 {
    // Check cache first
    if cached, ok := sectorSizeCache.Load(device); ok {
        return cached.(int64)
    }

    // Read from sysfs
    path := filepath.Join("/sys/block", device, "queue", "logical_block_size")
    data, err := os.ReadFile(path)
    if err != nil {
        // Fallback to 512 (standard for SATA/SCSI)
        sectorSizeCache.Store(device, int64(512))
        return 512
    }

    size, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
    if err != nil || size <= 0 {
        // Invalid value, use 512 fallback
        sectorSizeCache.Store(device, int64(512))
        return 512
    }

    // Cache and return
    sectorSizeCache.Store(device, size)
    return size
}

type DiskCollector struct {
    deviceID    string
    allowedDevs *regexp.Regexp  // Optional: user-configurable filter
}

func NewDiskCollector(deviceID string, allowedPattern string) (*DiskCollector, error) {
    c := &DiskCollector{deviceID: deviceID}

    if allowedPattern != "" {
        re, err := regexp.Compile(allowedPattern)
        if err != nil {
            return nil, fmt.Errorf("invalid disk device pattern: %w", err)
        }
        c.allowedDevs = re
    } else {
        // Use default whole-device pattern
        c.allowedDevs = wholeDevicePattern
    }

    return c, nil
}

func (c *DiskCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
    data, err := os.ReadFile("/proc/diskstats")
    if err != nil {
        return nil, err
    }

    var metrics []*models.Metric

    for _, line := range strings.Split(string(data), "\n") {
        fields := strings.Fields(line)
        if len(fields) < 14 {
            continue
        }

        device := fields[2]

        // Filter to whole devices only (skip partitions)
        // Handles nvme0n1 (whole device) vs nvme0n1p1 (partition)
        if !c.allowedDevs.MatchString(device) {
            continue
        }

        // Reads completed (field 3)
        readsCompleted, _ := strconv.ParseUint(fields[3], 10, 64)
        metrics = append(metrics,
            models.NewMetric("disk.read_ops_total", float64(readsCompleted), c.deviceID).
                WithTag("device", device))

        // Get per-device sector size (512 for SATA, 4096 for NVMe)
        sectorSize := getSectorSize(device)

        // Sectors read (field 5)
        sectorsRead, _ := strconv.ParseUint(fields[5], 10, 64)
        readBytes := sectorsRead * uint64(sectorSize)
        metrics = append(metrics,
            models.NewMetric("disk.read_bytes_total", float64(readBytes), c.deviceID).
                WithTag("device", device))

        // Writes completed (field 7)
        writesCompleted, _ := strconv.ParseUint(fields[7], 10, 64)
        metrics = append(metrics,
            models.NewMetric("disk.write_ops_total", float64(writesCompleted), c.deviceID).
                WithTag("device", device))

        // Sectors written (field 9)
        sectorsWritten, _ := strconv.ParseUint(fields[9], 10, 64)
        writeBytes := sectorsWritten * uint64(sectorSize)
        metrics = append(metrics,
            models.NewMetric("disk.write_bytes_total", float64(writeBytes), c.deviceID).
                WithTag("device", device))

        // I/O time in milliseconds (field 12)
        ioTimeMs, _ := strconv.ParseUint(fields[12], 10, 64)
        metrics = append(metrics,
            models.NewMetric("disk.io_time_ms_total", float64(ioTimeMs), c.deviceID).
                WithTag("device", device))

        // Weighted I/O time (field 13) - accounts for concurrent I/O
        weightedIOTime, _ := strconv.ParseUint(fields[13], 10, 64)
        metrics = append(metrics,
            models.NewMetric("disk.weighted_io_time_ms_total", float64(weightedIOTime), c.deviceID).
                WithTag("device", device))
    }

    return metrics, nil
}
```

**Key points:**
- **Per-device sector size detection**: Reads `/sys/block/<dev>/queue/logical_block_size`
- NVMe/eMMC may use 4096-byte sectors, SATA/SCSI use 512-byte sectors
- Cached per device to avoid repeated sysfs reads
- Exposes both operations (ops_total) and bytes (bytes_total)
- **Regex allow-list for whole devices**: Correctly handles nvme0n1 (whole) vs nvme0n1p1 (partition)
- **Configurable**: Allow user override via config for specialized block devices
- Include weighted I/O time for concurrency awareness
- Pattern matches: `sda`, `nvme0n1`, `mmcblk0` (whole devices)
- Pattern skips: `sda1`, `nvme0n1p1`, `mmcblk0p1` (partitions)

**Why per-device sector size matters:**
- Prevents silent under/over-reporting on mixed storage
- Orange Pi 5+ may have both eMMC (often 4096) and NVMe (varies)
- 8x difference between 512 and 4096 would cause massive metric errors
- Fallback to 512 ensures backward compatibility

### Clock Skew Detection

```go
func detectClockSkew(clockSkewURL string) (time.Duration, error) {
    client := &http.Client{Timeout: 5 * time.Second}

    // Use GET to dedicated health endpoint (not HEAD to ingest URL)
    // Many ingest endpoints don't support HEAD or return proxy time
    // VictoriaMetrics: /health returns reliable Date headers
    req, err := http.NewRequest("GET", clockSkewURL, nil)
    if err != nil {
        return 0, err
    }

    localBefore := time.Now()
    resp, err := client.Do(req)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()

    localAfter := time.Now()

    // Parse Date header
    dateStr := resp.Header.Get("Date")
    if dateStr == "" {
        return 0, fmt.Errorf("no Date header in response")
    }

    serverTime, err := http.ParseTime(dateStr)
    if err != nil {
        return 0, fmt.Errorf("failed to parse Date header: %w", err)
    }

    // Estimate local time at moment of server response
    localEstimate := localBefore.Add(localAfter.Sub(localBefore) / 2)

    // Calculate skew (positive = local ahead, negative = local behind)
    skew := localEstimate.Sub(serverTime)

    return skew, nil
}

// In main startup:
func startClockSkewMonitoring(ctx context.Context, cfg *config.Config, store *storage.SQLiteStorage) {
    checkInterval := cfg.Monitoring.ClockSkewCheckInterval
    warnThreshold := cfg.Monitoring.ClockSkewWarnThresholdMs

    ticker := time.NewTicker(checkInterval)
    defer ticker.Stop()

    // Check immediately on startup
    checkAndRecordSkew(cfg, store, warnThreshold)

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            checkAndRecordSkew(cfg, store, warnThreshold)
        }
    }
}

func checkAndRecordSkew(cfg *config.Config, store *storage.SQLiteStorage, warnThreshold int) {
    // Use dedicated clock_skew_url (not ingest URL)
    skew, err := detectClockSkew(cfg.Monitoring.ClockSkewURL)
    if err != nil {
        log.Warn("Failed to detect clock skew", "error", err,
            "clock_skew_url", cfg.Monitoring.ClockSkewURL)
        return
    }

    skewMs := skew.Milliseconds()

    // Log both URLs for diagnostics (ingest vs clock-check)
    log.Info("Clock skew measured",
        "skew_ms", skewMs,
        "clock_skew_url", cfg.Monitoring.ClockSkewURL,
        "ingest_url", cfg.Remote.URL)

    if abs(skewMs) > int64(warnThreshold) {
        log.Warn("Large clock skew detected",
            "skew_ms", skewMs,
            "threshold_ms", warnThreshold,
            "clock_skew_url", cfg.Monitoring.ClockSkewURL)
    }

    // Store as meta-metric
    metric := models.NewMetric("time.skew_ms", float64(skewMs), cfg.Device.ID)
    store.Store(context.Background(), metric)
}

func abs(n int64) int64 {
    if n < 0 {
        return -n
    }
    return n
}
```

**Why it matters:**
- VictoriaMetrics trusts client timestamps
- Skewed clocks create jagged/out-of-order time series
- Warn threshold: 2 seconds (configurable)
- Check every 5 minutes (configurable)

**Important notes:**
- If VM is behind a proxy/load balancer, the Date header may reflect proxy time, not VM time
- Log the server hostname/IP alongside skew warnings to aid diagnosis
- Large skew may indicate NTP issues on the Orange Pi - sync with `ntpdate` or `systemd-timesyncd`

### Active WAL Checkpoint Management

```go
func (s *SQLiteStorage) startWALCheckpointRoutine(ctx context.Context, interval time.Duration, sizeLimitMB int64) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    log.Info("WAL checkpoint routine started",
        "interval", interval,
        "size_limit_mb", sizeLimitMB)

    for {
        select {
        case <-ctx.Done():
            // Final checkpoint on shutdown
            log.Info("Running final WAL checkpoint before shutdown")
            s.checkpointWAL(sizeLimitMB)
            return
        case <-ticker.C:
            s.checkpointWAL(sizeLimitMB)
        }
    }
}

func (s *SQLiteStorage) checkpointWAL(sizeLimitMB int64) {
    walPath := s.dbPath + "-wal"

    // Check if WAL exists
    info, err := os.Stat(walPath)
    if err != nil {
        if os.IsNotExist(err) {
            return  // No WAL file
        }
        log.Error("Failed to stat WAL file", "error", err)
        return
    }

    walSizeBytes := info.Size()
    walSizeMB := walSizeBytes / (1024 * 1024)

    log.Debug("WAL size check",
        "wal_size_mb", walSizeMB,
        "wal_size_bytes", walSizeBytes,
        "limit_mb", sizeLimitMB)

    // Checkpoint if WAL exceeds limit
    if walSizeMB >= sizeLimitMB {
        log.Info("WAL size exceeds threshold, running checkpoint",
            "wal_size_mb", walSizeMB,
            "wal_size_bytes", walSizeBytes,
            "limit_mb", sizeLimitMB)

        start := time.Now()

        // Run TRUNCATE checkpoint - reclaims disk space
        // SQLite returns 3 values: (busy, log_frames, checkpointed_frames)
        var busy, logFrames, ckptFrames int32
        err := s.db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &ckptFrames)
        if err != nil {
            log.Error("WAL checkpoint failed", "error", err)
            return
        }

        duration := time.Since(start)

        // Check new size
        newInfo, err := os.Stat(walPath)
        var newSizeMB int64
        var newSizeBytes int64
        if err == nil {
            newSizeBytes = newInfo.Size()
            newSizeMB = newSizeBytes / (1024 * 1024)
        }

        // Calculate space reclaimed
        bytesReclaimed := walSizeBytes - newSizeBytes
        reductionPercent := 0.0
        if walSizeBytes > 0 {
            reductionPercent = (float64(bytesReclaimed) / float64(walSizeBytes)) * 100.0
        }

        log.Info("WAL checkpoint completed",
            "duration_ms", duration.Milliseconds(),
            "old_size_mb", walSizeMB,
            "new_size_mb", newSizeMB,
            "bytes_reclaimed", bytesReclaimed,
            "reduction_percent", fmt.Sprintf("%.1f", reductionPercent),
            "busy", busy,
            "log_frames", logFrames,
            "frames_checkpointed", ckptFrames)

        // Emit meta-metrics for observability
        s.emitMetaMetric("storage.wal_checkpoint_duration_ms", float64(duration.Milliseconds()))
        s.emitMetaMetric("storage.wal_bytes_reclaimed", float64(bytesReclaimed))
    }
}

// Expose WAL size in meta-metrics
func (s *SQLiteStorage) GetWALSize() (int64, error) {
    walPath := s.dbPath + "-wal"
    info, err := os.Stat(walPath)
    if err != nil {
        if os.IsNotExist(err) {
            return 0, nil
        }
        return 0, err
    }
    return info.Size(), nil
}
```

**Triggers:**
- Hourly timer
- WAL size > 64 MB
- On shutdown (final checkpoint)

**Why TRUNCATE mode:**
- Reclaims disk space
- Resets WAL to minimal size
- More aggressive than PASSIVE or FULL

### Graduated Health Status

```go
type HealthStatus string

const (
    StatusOK       HealthStatus = "ok"
    StatusDegraded HealthStatus = "degraded"
    StatusError    HealthStatus = "error"
)

type HealthResponse struct {
    Status     HealthStatus           `json:"status"`
    Version    string                 `json:"version"`
    Uptime     int64                  `json:"uptime_seconds"`
    Timestamp  string                 `json:"timestamp"`
    Collectors map[string]CollectorStatus `json:"collectors"`
    Uploader   UploaderStatus         `json:"uploader"`
    Storage    StorageStatus          `json:"storage"`
    Time       TimeStatus             `json:"time"`
}

type CollectorStatus struct {
    Status         string     `json:"status"`
    LastCollection string     `json:"last_collection"`  // RFC3339
    MetricsCollected int64    `json:"metrics_collected"`
    LastError      *string    `json:"last_error"`
}

type UploaderStatus struct {
    Status              string `json:"status"`
    LastUpload          string `json:"last_upload"`  // RFC3339
    MetricsUploaded     int64  `json:"metrics_uploaded"`
    UploadFailures      int64  `json:"upload_failures"`
    PendingMetrics      int64  `json:"pending_metrics"`
    RetryQueueSize      int    `json:"retry_queue_size"`
}

type StorageStatus struct {
    DatabaseSizeBytes   int64  `json:"database_size_bytes"`
    WALSizeBytes        int64  `json:"wal_size_bytes"`
    MetricsPendingUpload int64 `json:"metrics_pending_upload"`
    OldestMetricTimestamp string `json:"oldest_metric_timestamp"`  // RFC3339
}

type TimeStatus struct {
    Local      string `json:"local"`       // RFC3339
    SkewMs     int64  `json:"skew_ms"`
    NTPSynced  bool   `json:"ntp_synced"`  // Future: check NTP sync status
}

func calculateOverallStatus(
    collectors map[string]CollectorStatus,
    uploader UploaderStatus,
    storage StorageStatus,
    cfg *config.HealthConfig,
) HealthStatus {
    now := time.Now()

    // ERROR conditions (critical failures)

    // 1. No successful uploads for > 10 minutes AND high pending
    if uploader.LastUpload != "" {
        lastUpload, _ := time.Parse(time.RFC3339, uploader.LastUpload)
        if now.Sub(lastUpload) > time.Duration(cfg.ErrorThreshold.UploadAgeSeconds)*time.Second &&
           storage.MetricsPendingUpload > cfg.ErrorThreshold.PendingMetrics {
            return StatusError
        }
    }

    // 2. All collectors failing
    errorCount := 0
    for _, c := range collectors {
        if c.Status == "error" {
            errorCount++
        }
    }
    if errorCount == len(collectors) && len(collectors) > 0 {
        return StatusError
    }

    // DEGRADED conditions (partial failures or concerning trends)

    // 1. At least one collector failing
    if errorCount > 0 {
        return StatusDegraded
    }

    // 2. No recent uploads (but not critical yet)
    if uploader.LastUpload != "" {
        lastUpload, _ := time.Parse(time.RFC3339, uploader.LastUpload)
        uploadInterval := time.Duration(cfg.DegradedThreshold.UploadAgeSeconds) * time.Second
        if now.Sub(lastUpload) > uploadInterval {
            return StatusDegraded
        }
    }

    // 3. High pending count (but not critical)
    if storage.MetricsPendingUpload > cfg.DegradedThreshold.PendingMetrics {
        return StatusDegraded
    }

    // OK - everything healthy
    return StatusOK
}
```

**Status meanings:**

**OK:**
- All collectors succeeded in last cycle
- Uploads within 2× configured interval (e.g., 60s for 30s interval)
- Pending metrics < 5000

**Degraded:**
- ≥1 collector in error state
- No upload in 2×-10× interval (60s - 600s)
- Pending metrics 5000-10000
- Still functional but needs attention

**Error:**
- All collectors failing
- No upload in >10 minutes AND pending >10000
- Critical failure requiring immediate intervention

### Enhanced Structured Logging Fields

```go
// Collection logs
log.Info("Metrics collected",
    "collector", collectorName,
    "count", len(metrics),
    "duration_ms", duration.Milliseconds(),
    "session_id", sessionID)

// Upload logs
log.Info("Upload chunk succeeded",
    "batch_id", batchID,
    "chunk_index", chunkIndex,
    "attempt", attemptNumber,
    "count", len(chunk),
    "endpoint", cfg.Remote.URL,
    "http_status", resp.StatusCode,
    "bytes_sent", len(compressedPayload),
    "bytes_rcvd", len(responseBody),
    "duration_ms", duration.Milliseconds())

// Retry logs
log.Warn("Upload chunk failed, retrying",
    "batch_id", batchID,
    "chunk_index", chunkIndex,
    "attempt", attemptNumber,
    "backoff_ms", backoff.Milliseconds(),
    "error", err.Error(),
    "http_status", resp.StatusCode)

// Error logs
log.Error("Collection failed",
    "collector", collectorName,
    "error", err.Error(),
    "error_type", classifyError(err))
```

**JSON output example:**
```json
{"time":"2025-10-13T14:30:00.123456Z","level":"INFO","msg":"Metrics collected","collector":"cpu.usage","count":8,"duration_ms":12.3,"session_id":"550e8400-e29b-41d4-a716-446655440000"}
{"time":"2025-10-13T14:30:00.456789Z","level":"INFO","msg":"Upload chunk succeeded","batch_id":"660f9511-f3ac-52e5-b827-557766551111","chunk_index":0,"attempt":1,"count":50,"endpoint":"http://localhost:8428/api/v1/import","http_status":200,"bytes_sent":8192,"bytes_rcvd":45,"duration_ms":234.5}
```

### Docker Compose Stack

**File:** `docker/docker-compose.yml`

```yaml
version: '3.8'

services:
  victoriametrics:
    # Pin to specific version for reproducibility (update quarterly)
    image: victoriametrics/victoria-metrics:v1.97.1
    container_name: victoriametrics
    ports:
      - "8428:8428"
    volumes:
      - vm-data:/victoria-metrics-data
    command:
      - '--storageDataPath=/victoria-metrics-data'
      - '--httpListenAddr=:8428'
      - '--retentionPeriod=30d'
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8428/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  metrics-receiver:
    build:
      context: ..
      dockerfile: docker/Dockerfile.receiver
    container_name: metrics-receiver
    ports:
      - "9090:9090"
    environment:
      - PORT=9090
      - VERBOSE=true
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

volumes:
  vm-data:
```

**File:** `docker/Dockerfile.receiver`

```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o metrics-receiver cmd/metrics-receiver/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates wget
WORKDIR /root/
COPY --from=builder /app/metrics-receiver .

EXPOSE 9090
CMD ["./metrics-receiver"]
```

### Updated Systemd Service (Security Hardening)

**File:** `systemd/metrics-collector.service`

```ini
[Unit]
Description=Belabox Metrics Collector
Documentation=https://github.com/taniwha3/thugshells
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=metrics
Group=metrics
ExecStart=/usr/local/bin/metrics-collector -config /etc/belabox-metrics/config.yaml

# Restart policy
Restart=always
RestartSec=10s
StartLimitInterval=300s
StartLimitBurst=5

# Resource limits
MemoryMax=200M
MemoryHigh=150M
CPUQuota=20%

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
RestrictRealtime=true
RestrictNamespaces=true
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM

# Capability restrictions
# We only need network access and file I/O - no special capabilities required
CapabilityBoundingSet=
AmbientCapabilities=

# Read-write paths
# ProtectSystem=strict makes / read-only; explicitly allow our data/config paths
ReadWritePaths=/var/lib/belabox-metrics
ReadOnlyPaths=/etc/belabox-metrics
# Collectors need read access to /proc and /sys for system metrics
# These are allowed by default with ProtectSystem=strict (doesn't block /proc//sys)

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=metrics-collector

# Watchdog (restart if unhealthy for 60s)
WatchdogSec=60s

[Install]
WantedBy=multi-user.target
```

**Installation notes:**
- Create `metrics` user/group: `sudo useradd -r -s /bin/false metrics`
- Set permissions: `sudo chown -R metrics:metrics /var/lib/belabox-metrics`
- Token file: `sudo chmod 600 /etc/belabox-metrics/api-token`

### Configuration

**File:** `configs/config.yaml`

```yaml
device:
  id: belabox-001

storage:
  path: /var/lib/belabox-metrics/metrics.db
  wal_checkpoint_interval: 1h
  wal_checkpoint_size_mb: 64

remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
  upload_interval: 30s
  timeout: 30s
  compression: true
  batch_size: 2500       # Total metrics per batch
  chunk_size: 50         # Metrics per chunk

  retry:
    enabled: true
    max_attempts: 3
    initial_backoff: 5s
    max_backoff: 60s
    backoff_multiplier: 3
    jitter_percent: 20   # ±20%

health:
  enabled: true
  port: 9100
  path: /health

  # Health status semantics (SLO thresholds):
  #
  # OK: All collectors healthy, uploads within 2× interval, pending < 5000
  # DEGRADED: ≥1 collector error OR no upload in 2×-10× interval OR pending 5000-10000
  # ERROR: All collectors failing OR no upload >10min AND pending >10000
  #
  # Adjust thresholds based on your network conditions and upload interval.
  # For slow LTE links, consider increasing upload_age_seconds thresholds.

  degraded_threshold:
    pending_metrics: 5000
    upload_age_seconds: 60  # 2× upload_interval (e.g., 30s interval → 60s threshold)

  error_threshold:
    pending_metrics: 10000
    upload_age_seconds: 600  # 10 minutes - critical failure threshold

logging:
  level: info              # debug, info, warn, error
  format: json             # json, console
  output: stdout           # stdout, stderr, or file path

monitoring:
  clock_skew_check_interval: 5m
  clock_skew_warn_threshold_ms: 2000
  # IMPORTANT: Use dedicated health endpoint for clock checks, not ingest URL
  # Many ingest endpoints don't support HEAD or return proxy time
  # VictoriaMetrics: use /health endpoint for reliable Date headers
  clock_skew_url: "http://localhost:8428/health"  # Separate from ingest URL

metrics:
  # System metrics
  - name: cpu.temperature
    interval: 30s
    enabled: true
    # Collects max across all zones + per-zone metrics:
    #   thermal.zone_temp{zone="soc-thermal"}
    #   thermal.zone_temp{zone="bigcore0-thermal"}
    #   etc.

  - name: cpu.usage
    interval: 10s
    enabled: true
    # Delta calculation, per-core + overall

  - name: memory.usage
    interval: 30s
    enabled: true

  - name: disk.io
    interval: 30s
    enabled: true
    # Sector→byte conversion, ops/s + bytes/s
    config:
      # Optional: override default whole-device pattern
      # Default: ^(sd[a-z]+|nvme\d+n\d+|mmcblk\d+)$
      allowed_devices: ""  # Empty = use default

  - name: network.traffic
    interval: 10s
    enabled: true
    config:
      include_ifaces: []    # Empty = all (except excluded)
      # Exclude loopback, virtual, transient, and modem interfaces
      exclude_ifaces: [
        "lo",           # Loopback
        "docker.*",     # Docker bridges
        "veth.*",       # Docker virtual ethernet
        "br-.*",        # Linux bridges
        "wlan.*mon",    # Wireless monitor mode
        "virbr.*",      # Libvirt virtual bridges
        "wwan.*",       # WWAN modem interfaces
        "wwp.*",        # WWAN point-to-point
        "enx[0-9a-f]{12}",  # USB Ethernet (random MAC-based names)
        "usb.*"         # USB network interfaces
      ]
      # Cardinality guard: hard cap on interface count (prevents label explosion)
      max_interfaces: 32    # Default 32, increase cautiously (each interface = 6 metrics)

  # Mock streaming metrics (real in M3)
  - name: srt.packet_loss
    interval: 5s
    enabled: true
```

### Systemd Watchdog Integration

```go
import (
    "time"
    "github.com/coreos/go-systemd/v22/daemon"
)

// startWatchdogPinger sends periodic WATCHDOG=1 notifications to systemd
// Must be called after systemd activation (after socket/service readiness)
func startWatchdogPinger(ctx context.Context) {
    // Query systemd for watchdog interval
    interval, err := daemon.SdWatchdogEnabled(false)
    if err != nil || interval == 0 {
        log.Info("Systemd watchdog not enabled or failed to query")
        return
    }

    // Ping at half the watchdog interval (systemd best practice)
    pingInterval := interval / 2
    log.Info("Systemd watchdog enabled",
        "watchdog_interval", interval,
        "ping_interval", pingInterval)

    ticker := time.NewTicker(pingInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            log.Info("Watchdog pinger stopping")
            return
        case <-ticker.C:
            // Send WATCHDOG=1 to systemd
            sent, err := daemon.SdNotify(false, daemon.SdNotifyWatchdog)
            if err != nil {
                log.Warn("Failed to send watchdog ping", "error", err)
            } else if !sent {
                log.Debug("Watchdog ping not sent (systemd not listening)")
            }
        }
    }
}

// In main():
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // ... storage, collectors, uploader setup ...

    // Start watchdog pinger in background
    go startWatchdogPinger(ctx)

    // Signal systemd that we're ready
    daemon.SdNotify(false, daemon.SdNotifyReady)

    // ... main loop ...
}
```

**Why this matters:**
- `WatchdogSec=60s` in systemd unit requires periodic `WATCHDOG=1` notifications
- Without pings, systemd will kill and restart the process after 60s
- Ping at half the interval (30s) to ensure timely delivery
- Use `SdWatchdogEnabled()` to detect if watchdog is active (works in both systemd and non-systemd environments)

### Single-Process Guard (Prevent Double-Run)

```go
import (
    "os"
    "syscall"
)

// AcquireProcessLock ensures only one instance runs
// Takes a non-blocking flock on the database path
func AcquireProcessLock(dbPath string) (*os.File, error) {
    lockPath := dbPath + ".lock"

    // Open (or create) lock file
    lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
    if err != nil {
        return nil, fmt.Errorf("failed to open lock file: %w", err)
    }

    // Try to acquire exclusive, non-blocking lock
    err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
    if err != nil {
        lockFile.Close()
        if err == syscall.EWOULDBLOCK {
            return nil, fmt.Errorf("another instance is already running (lock held on %s)", lockPath)
        }
        return nil, fmt.Errorf("failed to acquire lock: %w", err)
    }

    // Write PID to lock file for debugging
    lockFile.Truncate(0)
    lockFile.Seek(0, 0)
    fmt.Fprintf(lockFile, "%d\n", os.Getpid())
    lockFile.Sync()

    return lockFile, nil
}

// Usage in main():
// lockFile, err := AcquireProcessLock(cfg.Storage.Path)
// if err != nil {
//     log.Fatal("Failed to acquire process lock", "error", err)
// }
// defer lockFile.Close()  // Releases flock on exit
```

**Why this matters:**
- Prevents accidental double-runs even if systemd is bypassed
- Uses flock (file locking) which is released automatically on process exit
- Non-blocking check - fails immediately if another instance holds lock
- Writes PID for debugging (identify which process holds lock)
- Complementary to systemd's single-instance guarantees

### Chunked Upload with Byte-Size Limit

```go
const (
    DefaultChunkMetrics = 50       // Target metrics per chunk
    MaxChunkSizeBytes   = 256 * 1024  // 256 KB max gzipped payload
)

// splitIntoChunks splits metrics into chunks with byte-size limit
func splitIntoChunks(metrics []*models.Metric, targetCount int) [][]*models.Metric {
    if len(metrics) == 0 {
        return nil
    }

    var chunks [][]*models.Metric
    currentChunk := make([]*models.Metric, 0, targetCount)
    currentSizeEstimate := 0

    for _, m := range metrics {
        // Rough estimate: 100 bytes per metric (pre-compression)
        metricSize := 100 + len(m.Name) + len(m.DeviceID)
        for k, v := range m.Tags {
            metricSize += len(k) + len(v)
        }

        // Check if adding this metric would exceed byte limit
        // We estimate post-gzip size as ~40% of uncompressed
        estimatedGzipSize := int(float64(currentSizeEstimate+metricSize) * 0.4)

        if len(currentChunk) >= targetCount || estimatedGzipSize > MaxChunkSizeBytes {
            // Flush current chunk
            if len(currentChunk) > 0 {
                chunks = append(chunks, currentChunk)
                currentChunk = make([]*models.Metric, 0, targetCount)
                currentSizeEstimate = 0
            }
        }

        currentChunk = append(currentChunk, m)
        currentSizeEstimate += metricSize
    }

    // Flush remaining
    if len(currentChunk) > 0 {
        chunks = append(chunks, currentChunk)
    }

    return chunks
}
```

**Why byte-size limits:**
- Steady upload latency on slow links
- Better fairness when bandwidth is constrained
- Prevents oversized payloads from single chunks with many high-cardinality tags
- Gzip compression ratio varies (40-60%) - estimate conservatively
- Complements metric-count chunking (50 metrics OR 256 KB, whichever first)

---

## Implementation Plan

### Prerequisites

**Required:**
- Go 1.22+ (pinned across Dockerfile, CI, go.mod)
- SQLite 3.35+ (for WAL improvements)
- Docker + Docker Compose (for VictoriaMetrics test stack)

**go.mod example:**
```go
module github.com/taniwha3/thugshells

go 1.22

require (
    github.com/mattn/go-sqlite3 v1.14.18
    github.com/google/uuid v1.5.0
    github.com/coreos/go-systemd/v22 v22.5.0
)
```

### Day 1: Database + Upload Fix (7-9 hours)

**Task 1: Database Schema Migration (2-3h)**
- Create `internal/storage/migration.go`
- Implement `GetSchemaVersion()` and `Migrate()`
- Migration v1→v2:
  - Add new columns (value_text, value_type, tags, session_id, uploaded, priority, dedup_key)
  - Create sessions table
  - Create upload_checkpoints table
  - Create new indexes including unique index on dedup_key
  - Backfill dedup_key for existing rows
- Unit tests: v1→v2 migration, idempotency, error cases

**Task 2: Dedup Key Generation (1h)**
- Add `GenerateDedupKey()` method to `models.Metric`
- Canonical ordering of tags
- SHA256 hashing
- Unit tests: consistency, collision resistance

**Task 3: Chunked Upload Strategy (2-3h)**
- Modify upload loop to use chunks (50 metrics each)
- Generate batch_id (UUID) for tracking
- Sort by timestamp within chunk
- JSONL building + gzip compression (BestSpeed for ARM efficiency)
- Configure HTTP transport with connection pooling
- Target 128-256 KB gzipped payload per chunk
- Unit tests: chunking logic, sorting, compression

**Task 4: Partial Success Handling (1-2h)**
- Parse VM response for accepted count
- Mark only accepted metrics as uploaded
- Save checkpoint per successful chunk
- Increment partial_success counter
- Unit tests: partial ack scenarios

**Task 5: Jittered Backoff (1h)**
- Implement `calculateBackoff()` with jitter
- ±20% jitter calculation
- Unit tests: backoff values, jitter range

### Day 2: Collectors (7-9 hours)

**Task 6: CPU Delta Collector (2-3h)**
- Create `internal/collector/cpu.go`
- Implement two-read strategy with cached counters
- Per-core delta calculation
- Wraparound detection and handling
- Skip first sample (no previous to compare)
- Mock implementation for macOS
- Unit tests: delta calculation, wraparound, first-sample skip

**Task 7: Memory Collector (1h)**
- Create `internal/collector/memory.go`
- Parse `/proc/meminfo` for MemTotal, MemAvailable, SwapTotal, SwapFree
- Calculate used = MemTotal - MemAvailable (Linux-recommended)
- Export: memory.used_bytes, memory.available_bytes, memory.swap_used_bytes
- Avoid redundant metrics that cause dashboard confusion
- Mock for macOS
- Unit tests: parsing logic, used calculation

**Task 8: Disk I/O Collector (1-2h)**
- Create `internal/collector/disk.go`
- Parse `/proc/diskstats`
- Sector→byte conversion (per-device from sysfs: 512 for SATA, 4096 for NVMe/eMMC)
- Expose ops/s and bytes/s
- Skip partitions
- Mock for macOS
- Unit tests: sector conversion, parsing, partition filtering

**Task 9: Network Collector (2-3h)**
- Create `internal/collector/network.go`
- Parse `/proc/net/dev`
- Regex-based interface filtering
- Default exclusions: lo, docker*, veth*, br-*
- Configurable includes/excludes
- Mock for macOS
- Unit tests: filtering logic, regex patterns, parsing

**Task 10: Clock Skew Detection (1h)**
- Create `internal/monitoring/clock.go`
- Implement `detectClockSkew()` using VM Date header
- Periodic checking routine
- Warn on >2s skew
- Expose time.skew_ms metric
- Unit tests: skew calculation, warning threshold

### Day 3: Health + Monitoring (6-8 hours)

**Task 11: Graduated Health Status (2-3h)**
- Create `internal/health/health.go`
- Implement status calculation with ok/degraded/error rules
- Per-component status tracking
- `/health` endpoint with full JSON
- `/health/live` liveness probe
- `/health/ready` readiness probe
- Integrate with main collector
- Unit tests: status calculation, thresholds, rollup logic

**Task 12: Meta-Monitoring (2h)**
- Create `internal/monitoring/metrics.go`
- Implement counters: metrics_collected, metrics_failed, metrics_uploaded, upload_failures, partial_success
- Implement gauges: database_size, wal_size, pending_upload, time.skew_ms
- Implement histograms: collection_duration, upload_duration
- Send meta-metrics to storage/VM
- Unit tests: counter increments, gauge updates, histogram recordings

**Task 13: Enhanced Logging (1-2h)**
- Migrate from `log` to `log/slog`
- Create JSON formatter
- Create console formatter
- Add all required contextual fields
- Configuration for level and format
- Update all log statements throughout codebase
- Unit tests: formatter output, field presence

**Task 14: WAL Checkpoint Routine (1h)**
- Add `startWALCheckpointRoutine()` to storage
- Hourly ticker + size-based triggering
- Checkpoint when WAL > 64 MB
- Expose wal_size in meta-metrics
- Final checkpoint on shutdown
- Unit tests: checkpoint triggers, size checking

### Day 4: VictoriaMetrics + Testing (6-8 hours)

**Task 15: VictoriaMetrics Integration (2h)**
- Create `internal/uploader/victoriametrics.go`
- JSONL formatter for VM format
- Labels mapping (__name__, device_id, tags)
- Test with local VM instance
- Integration test: end-to-end ingestion

**Task 16: Docker Setup (1h)**
- Create `docker/docker-compose.yml`
- Create `docker/Dockerfile.receiver`
- Create `docker/README.md` with setup guide
- Test local deployment
- Add PromQL sanity query examples

**Task 17: Expanded Test Coverage (2-3h)**
- Duplicate-proofing test (same batch retried → no new rows)
- Partial-ack test (VM accepts 25/50 → only 25 marked)
- Clock skew test (mock VM with skewed time)
- WAL growth test (insert many → checkpoint → size reduced)
- Counter wraparound test (CPU stats wrap → skip sample)
- Integration test: 30-minute soak, no duplicates

**Task 18: Documentation (1-2h)**
- Update README.md with M2 features
- Create `docs/health-monitoring.md` (status meanings, thresholds)
- Create `docs/victoriametrics-setup.md` (setup, queries, troubleshooting)
- Add PromQL query examples to VictoriaMetrics docs
- Update config.yaml with inline comments for new options
- Document chunk sizing rationale (128-256 KB target)
- Update MILESTONE-2.md acceptance checklist

---

## Testing Strategy

### Unit Tests (80+ expected)

**Storage (15+ tests):**
- Schema migration v1→v2
- Dedup_key prevents duplicates
- MarkUploaded with various metric IDs
- GetUnuploadedMetrics with filters
- Checkpoint save/retrieve
- Pending count calculation
- WAL size retrieval
- Checkpoint routine triggers

**Uploader (18+ tests):**
- Chunking logic (2500 → 50 chunks)
- Jittered backoff calculation (±20%)
- JSONL formatting
- Gzip compression
- Partial success handling
- VM endpoint integration
- Retry with backoff

**Collectors (30+ tests):**
- CPU delta calculation (two-read, wraparound, aggregate "all" core)
- Memory parsing (canonical used calculation)
- **Disk per-device sector size** (NVMe 4096, SATA 512, eMMC variants)
- Network interface filtering (regex)
- **Network counter wraparound** detection
- Thermal zones (all RK3588)
- Mock implementations for macOS

**Health (10+ tests):**
- Status calculation (ok/degraded/error)
- Threshold enforcement
- Component rollup
- JSON response format
- Liveness/readiness probes

**Monitoring (8+ tests):**
- Clock skew detection
- Meta-metric collection
- Counter increments
- Gauge updates
- Histogram recordings

**Models (6+ tests):**
- Dedup_key generation
- Tag canonicalization
- Hash consistency
- **Metric name sanitization** (dots→underscores, PromQL regex)
- **Unit suffix normalization** (_bytes, _celsius, _total)

**Uploader (20+ tests):**
- **HTTP retry code semantics** (table-driven: 429/503/5xx→retry, 4xx→fail)
- **Retry-After header parsing** (seconds, HTTP-date formats)
- **Byte-size chunk limits** (256 KB max, split oversized chunks)

### Integration Tests (21+ scenarios)

**No Duplicate Uploads:**
```go
func TestNoDuplicateUploads(t *testing.T) {
    // Setup: storage with dedup_key
    // Insert 100 metrics
    // Run upload loop for 30 minutes
    // Track all uploaded metric IDs
    // Verify: no ID appears twice in VM
    // Verify: dedup_key unique constraint enforced
}
```

**Partial Success:**
```go
func TestPartialSuccessUpload(t *testing.T) {
    // Mock VM that accepts 25 of 50 metrics
    // Upload 50-metric chunk
    // Verify: only 25 marked as uploaded
    // Verify: remaining 25 retried
    // Verify: partial_success counter incremented
}
```

**Partial Success Fallback:**
```go
func TestPartialSuccessFallback(t *testing.T) {
    // Mock VM that returns 200 without acceptance details
    // Upload 50-metric chunk
    // Verify: entire chunk marked as uploaded (fallback to 2xx = success)
    // Verify: no partial_success counter increment
}
```

**Transport Soak Test:**
```go
func TestTransportSoak(t *testing.T) {
    // Run collector for 60 minutes with VM restarts
    // Simulate VM restart at 20min and 40min marks
    // Verify: no duplicates across restarts
    // Verify: chunks resume from last successful chunk
    // Verify: MaxAttempts respected for failed chunks
    // Verify: connection reuse (check TIME_WAIT sockets)
}
```

**Clock Skew:**
```go
func TestClockSkewDetection(t *testing.T) {
    // Mock VM with +10s skewed Date header
    // Run clock skew detection
    // Verify: skew_ms = -10000 (we're behind)
    // Verify: warning logged with server hostname
    // Verify: time.skew_ms metric created
}
```

**Proxy Clock Skew:**
```go
func TestProxyClockSkew(t *testing.T) {
    // Mock VM behind proxy with ±10s Date header
    // Verify: skew measured against proxy time
    // Verify: health status degraded only if skew > threshold
    // Verify: logs include server hostname for diagnosis
}
```

**WAL Growth:**
```go
func TestWALCheckpoint(t *testing.T) {
    // Insert 100k metrics (grow WAL to >64 MB)
    // Trigger checkpoint
    // Verify: WAL size reduced significantly
    // Verify: data integrity maintained
    // Verify: checkpoint meta-metrics emitted (duration, bytes_reclaimed)
}
```

**Counter Wraparound:**
```go
func TestCPUCounterWraparound(t *testing.T) {
    // Mock CPU stats near uint64 max
    // Next sample wraps around to low value
    // Verify: sample skipped (no negative delta)
    // Verify: next valid sample works correctly
}
```

**High-Cardinality Interface Guard:**
```go
func TestNetworkInterfaceCardinality(t *testing.T) {
    // Simulate 1000 ephemeral interface names (wlan0mon1, wlan0mon2, ...)
    // Verify: exclusion filter catches them
    // Verify: label cardinality remains bounded
    // Verify: no metrics for excluded interfaces
}
```

**Metric Name Sanitization (PromQL Safety):**
```go
func TestMetricNameSanitization(t *testing.T) {
    // Create metrics with dotted names: "cpu.usage.percent", "network.rx.bytes"
    // Run through sanitizeMetricName()
    // Verify: dots converted to underscores in upload payload
    // Verify: storage keeps original dotted names
    // Verify: PromQL regex enforced: [a-zA-Z_:][a-zA-Z0-9_:]*
    // Test edge cases: leading numbers, special chars, multiple dots
}
```

**Chunk Replay with Dedup Key:**
```go
func TestChunkReplayDedup(t *testing.T) {
    // Upload 50-metric chunk, get 200 response
    // Mark all metrics as uploaded
    // Simulate collector restart (clear state)
    // Query same metrics again, attempt upload
    // Verify: VictoriaMetrics dedup_key prevents duplicates
    // Verify: unique constraint on (dedup_key) in SQLite
    // Verify: value_type changes force new dedup_key
}
```

**WAL Checkpoint Growth Test:**
```go
func TestWALCheckpointGrowthPrevention(t *testing.T) {
    // Insert 200k metrics without checkpointing (grow WAL to >128 MB)
    // Verify: automatic checkpoint triggered at 64 MB threshold
    // Verify: WAL truncated, space reclaimed
    // Verify: meta-metrics emitted: wal_checkpoint_duration_ms, wal_bytes_reclaimed
    // Verify: frames_checkpointed logged
    // Verify: no data loss after checkpoint
}
```

**Interface Cardinality Hard Cap:**
```go
func TestInterfaceCardinalityHardCap(t *testing.T) {
    // Create mock /proc/net/dev with 100 interfaces
    // Set max_interfaces = 32
    // Run network collector
    // Verify: exactly 32 interfaces collected (6 metrics each = 192 total)
    // Verify: remaining 68 interfaces dropped
    // Verify: network.interfaces_dropped_total = 68
    // Verify: warnings logged with interface names
}
```

**Timestamp Validation:**
```go
func TestTimestampValidation(t *testing.T) {
    // Create metric with timestamp in far future (+10 minutes)
    // Verify: timestamp clamped to now, warning logged
    // Create metric with timestamp in far past (-2 hours)
    // Verify: timestamp clamped to now, warning logged
    // Create metric with valid timestamp (within ±5 minutes)
    // Verify: timestamp accepted as-is
}
```

**Clock Skew URL Configuration:**
```go
func TestClockSkewSeparateURL(t *testing.T) {
    // Configure separate clock_skew_url (e.g., /health)
    // Configure remote.url (e.g., /api/v1/import)
    // Run clock skew detection
    // Verify: GET request sent to clock_skew_url, not ingest URL
    // Verify: logs include both URLs for diagnostics
    // Verify: no POST attempts to clock check endpoint
}
```

**Chunk Atomicity (5xx forces entire chunk retry):**
```go
func TestChunkAtomicRetry(t *testing.T) {
    // Mock VM that returns 500 for first attempt, 200 for retry
    // Upload 50-metric chunk
    // Verify: no subset marked uploaded after 500 response
    // Verify: entire chunk retried
    // Verify: all 50 metrics marked uploaded only after 200 response
    // Verify: dedup_key prevents duplicates if chunk sent twice
}
```

**String Metric Filtering:**
```go
func TestStringMetricsNotSentToVM(t *testing.T) {
    // Create metrics with value_type=0 (numeric) and value_type=1 (string)
    // Pass through buildJSONL()
    // Verify: only numeric metrics in JSONL output
    // Verify: string metrics skipped with debug log
    // Verify: string metrics still stored in SQLite (for local events)
}
```

**Retry-After Header Parsing:**
```go
func TestRetryAfterParsing(t *testing.T) {
    // Test case 1: "30" (integer seconds)
    // Verify: parseRetryAfter() returns 30s
    // Test case 2: "Wed, 21 Oct 2025 07:28:00 GMT" (HTTP-date)
    // Verify: parseRetryAfter() returns time.Until(date)
    // Test case 3: "" (missing header)
    // Verify: parseRetryAfter() returns 0
    // Test case 4: "invalid"
    // Verify: parseRetryAfter() returns 0
}
```

**SQLite Connection Pool Settings:**
```go
func TestSQLiteConnectionPool(t *testing.T) {
    // Create NewSQLiteStorage()
    // Verify: db.Stats().MaxOpenConnections == 1
    // Verify: PRAGMA journal_mode returns "wal"
    // Verify: PRAGMA synchronous returns 1 (NORMAL)
    // Verify: concurrent reads work (no SQLITE_BUSY)
    // Verify: write blocks concurrent write (single writer model)
}
```

**Index Coverage on Uploader Hot Path:**
```go
func TestUploaderIndexCoverage(t *testing.T) {
    // Insert 100k metrics with random priority/timestamp
    // Run EXPLAIN QUERY PLAN on GetUnuploadedMetrics()
    // Verify: uses idx_uploaded (no SCAN TABLE)
    // Verify: deterministic ordering by (priority, timestamp_ms, id)
    // Measure query latency
    // Verify: <10ms on ARM SBC target (or <1ms on dev machine)
}
```

### Manual Testing (macOS)

```bash
# 1. Start VictoriaMetrics
cd docker
docker-compose up -d

# 2. Build collector
cd ..
./scripts/build.sh

# 3. Run collector
./bin/metrics-collector-darwin -config configs/config.yaml

# 4. Verify VictoriaMetrics ingestion
# Open http://localhost:8428
# Query: {device_id="belabox-001"}
# Verify metrics appearing

# 5. Check health endpoint
curl http://localhost:9100/health | jq

# 6. Test retry logic
docker stop victoriametrics
# Watch logs for retry attempts with jittered backoff
docker start victoriametrics
# Verify successful upload after reconnect

# 7. Resource usage check
ps aux | grep metrics-collector
# Verify <5% CPU, <150MB RAM

# 8. 30-minute soak test
# Let run for 30 minutes
# Query VM for duplicate timestamps
# Verify no duplicates

# 9. Clock skew check
# Check logs for "Clock skew measured"
# Verify skew_ms is reasonable (<1000ms typically)

# 10. WAL size check
ls -lh /var/lib/belabox-metrics/metrics.db-wal
# Should stay <64 MB due to checkpoints
```

---

## Acceptance Checklist

### Database & Storage
- [ ] Schema migration from M1 to M2 succeeds
- [ ] `dedup_key` field added with unique index
- [ ] Same metrics retried → UNIQUE constraint error (no duplicates)
- [ ] `uploaded` field added and indexed
- [ ] `upload_checkpoints` table created with batch tracking
- [ ] `MarkUploaded()` updates metrics correctly
- [ ] `GetUnuploadedMetrics()` returns only unuploaded
- [ ] Checkpoint tracking persists across restarts
- [ ] WAL checkpoint routine runs hourly
- [ ] WAL size stays <64 MB under load

### Upload Fix
- [ ] No duplicate uploads in 30-minute test
- [ ] Upload loop queries only `uploaded=0`
- [ ] Metrics marked as uploaded after success
- [ ] Checkpoint advances correctly per chunk
- [ ] Chunking: 2500 metrics → 50-metric chunks
- [ ] Chunks sorted by timestamp ASC
- [ ] Gzip compression applied
- [ ] Partial success handled (VM accepts 25/50 → only 25 marked)

### Retry Logic
- [ ] Jittered backoff calculates correctly (±20%)
- [ ] Failed uploads retry with proper delays
- [ ] Max attempts respected (3 attempts)
- [ ] Eventual success after retries
- [ ] Backoff logged with attempt number

### System Metrics
- [ ] CPU usage collecting with delta calculation
- [ ] First sample skipped (no previous to compare)
- [ ] Counter wraparound detected and handled
- [ ] Per-core + overall CPU metrics
- [ ] Memory usage collecting
- [ ] Disk I/O collecting with sector→byte conversion
- [ ] Disk ops/s and bytes/s both exposed
- [ ] Network traffic collecting with interface filtering
- [ ] lo, docker*, veth*, br-* excluded by default
- [ ] All thermal zones collecting (SoC, cores, GPU, NPU)
- [ ] Load averages collecting
- [ ] System uptime collecting

### VictoriaMetrics
- [ ] Docker Compose starts VictoriaMetrics
- [ ] Metrics ingested successfully
- [ ] Can query metrics from UI with PromQL
- [ ] JSONL format correct (__name__, labels, values, timestamps)
- [ ] Gzip compression works
- [ ] Timestamps preserved correctly (milliseconds)
- [ ] Labels include device_id and tags

### Health & Monitoring
- [ ] Health endpoint responds on :9100
- [ ] `/health` returns full status JSON
- [ ] `/health/live` returns liveness (200)
- [ ] `/health/ready` returns readiness (200 only if ok)
- [ ] Status calculation: ok/degraded/error
- [ ] Degraded when 1+ collector fails
- [ ] Degraded when pending >5000
- [ ] Error when no upload >10min AND pending >10000
- [ ] Collector statuses accurate (with RFC3339 timestamps)
- [ ] Uploader status accurate
- [ ] Storage status includes WAL size
- [ ] Time status includes skew_ms
- [ ] Meta-metrics collecting
- [ ] Meta-metrics visible in VictoriaMetrics

### Clock Skew
- [ ] Clock skew detected on startup
- [ ] Periodic rechecking (5min interval)
- [ ] Warning logged when skew >2s
- [ ] time.skew_ms exposed in meta-metrics
- [ ] time.skew_ms visible in health endpoint

### Logging
- [ ] Structured logging with `log/slog`
- [ ] JSON format works
- [ ] Console format works for development
- [ ] Log levels configurable
- [ ] Collection logs include: collector, count, duration_ms, session_id
- [ ] Upload logs include: batch_id, chunk_index, attempt, backoff_ms, http_status, bytes_sent, bytes_rcvd
- [ ] Retry logs include: attempt, backoff_ms, error
- [ ] No sensitive data in logs

### Security
- [ ] Systemd runs as non-root `metrics` user
- [ ] NoNewPrivileges=true
- [ ] ProtectSystem=strict, ProtectHome=true
- [ ] MemoryMax=200M, CPUQuota=20%
- [ ] RestrictAddressFamilies, RestrictNamespaces
- [ ] Token file permissions 0600
- [ ] Watchdog integration (60s)

### Testing
- [ ] All unit tests pass (80+)
- [ ] Integration tests pass
- [ ] No duplicate uploads verified
- [ ] Partial success verified
- [ ] Retry logic verified
- [ ] Clock skew verified
- [ ] WAL growth verified
- [ ] Counter wraparound verified
- [ ] Resource usage <5% CPU, <150MB RAM

### Documentation
- [ ] MILESTONE-2.md complete and updated
- [ ] README updated with M2 features
- [ ] Docker setup documented in `docker/README.md`
- [ ] VictoriaMetrics setup documented
- [ ] Health monitoring documented (status meanings, thresholds)
- [ ] Config examples include all new options
- [ ] Per-zone temperature documented
- [ ] PromQL sanity queries provided

---

## PromQL Query Examples

**For use with VictoriaMetrics UI (http://localhost:8428) or Grafana**

### Network Throughput

```promql
# Network upload rate (bytes per second) over last minute
rate(network_tx_bytes_total{device_id="belabox-001"}[1m])

# Network download rate
rate(network.rx_bytes_total{device_id="belabox-001"}[1m])

# Total network throughput (upload + download)
sum(rate(network_tx_bytes_total{device_id="belabox-001"}[1m])) +
sum(rate(network.rx_bytes_total{device_id="belabox-001"}[1m]))
```

### CPU Usage

```promql
# Overall CPU usage (aggregated across all cores)
cpu.usage_percent{device_id="belabox-001", core="all"}

# Per-core CPU usage
cpu.usage_percent{device_id="belabox-001", core!="all"}

# Average CPU usage across all cores
avg(cpu.usage_percent{device_id="belabox-001", core!="all"})

# Max CPU usage (hottest core)
max(cpu.usage_percent{device_id="belabox-001", core!="all"})
```

### Memory Usage

```promql
# Memory usage percentage
(memory.used_bytes{device_id="belabox-001"} / memory.total_bytes{device_id="belabox-001"}) * 100

# Available memory in MB
memory.available_bytes{device_id="belabox-001"} / 1024 / 1024

# Swap usage
memory.swap_used_bytes{device_id="belabox-001"} / 1024 / 1024
```

### Disk I/O

```promql
# Disk read rate (bytes per second)
rate(disk_read_bytes_total{device_id="belabox-001"}[1m])

# Disk write rate
rate(disk.write_bytes_total{device_id="belabox-001"}[1m])

# Disk IOPS (reads + writes)
rate(disk_read_ops_total{device_id="belabox-001"}[1m]) +
rate(disk.write_ops_total{device_id="belabox-001"}[1m])
```

### Thermal Monitoring

```promql
# Max temperature across all thermal zones (RK3588)
max_over_time(thermal.zone_temp_c{device_id="belabox-001"}[5m])

# Temperature by zone
thermal.zone_temp_c{device_id="belabox-001"}

# Check if any zone exceeds 85°C
max(thermal.zone_temp_c{device_id="belabox-001"}) > 85
```

### Meta-Metrics (Collector Health)

```promql
# Metrics collected per second
rate(collector.metrics_collected_total{device_id="belabox-001"}[1m])

# Upload success rate
rate(uploader.metrics_uploaded_total{device_id="belabox-001"}[5m]) /
(rate(uploader.metrics_uploaded_total{device_id="belabox-001"}[5m]) +
 rate(uploader.upload_failures_total{device_id="belabox-001"}[5m]))

# Pending metrics (backlog)
storage.metrics_pending_upload{device_id="belabox-001"}

# Database size growth rate
deriv(storage.database_size_bytes{device_id="belabox-001"}[1h])

# Time skew (clock drift)
time.skew_ms{device_id="belabox-001"}
```

### Alerting Queries

```promql
# High CPU usage alert (>90% for 5 minutes)
cpu.usage_percent{device_id="belabox-001", core="all"} > 90

# Low memory alert (<100 MB available)
memory.available_bytes{device_id="belabox-001"} < 100*1024*1024

# Thermal throttling risk (>85°C)
max(thermal.zone_temp_c{device_id="belabox-001"}) > 85

# Upload failures (>10% failure rate)
rate(uploader.upload_failures_total{device_id="belabox-001"}[5m]) /
(rate(uploader.metrics_uploaded_total{device_id="belabox-001"}[5m]) +
 rate(uploader.upload_failures_total{device_id="belabox-001"}[5m])) > 0.1

# Large clock skew (>2 seconds)
abs(time.skew_ms{device_id="belabox-001"}) > 2000
```

### Dashboard Queries

```promql
# Overall system health score (0-100)
100 - (
  (cpu.usage_percent{device_id="belabox-001", core="all"} * 0.3) +
  ((memory.used_bytes / memory.total_bytes) * 100 * 0.3) +
  ((max(thermal.zone_temp_c) / 100) * 100 * 0.2) +
  ((storage.metrics_pending_upload / 10000) * 100 * 0.2)
)

# Metrics ingestion rate (metrics per minute)
sum(rate(collector.metrics_collected_total{device_id="belabox-001"}[1m])) * 60
```

---

## Troubleshooting Guide

### Duplicate Metrics Still Appearing

**Check:**
1. Schema migration completed: `sqlite3 metrics.db "SELECT version FROM schema_version"`
2. `dedup_key` column exists: `sqlite3 metrics.db "PRAGMA table_info(metrics)"`
3. Unique index exists: `sqlite3 metrics.db "SELECT * FROM sqlite_master WHERE type='index' AND name='ux_metrics_dedup'"`
4. Metrics have dedup_key populated: `sqlite3 metrics.db "SELECT COUNT(*) FROM metrics WHERE dedup_key IS NULL"`

**Solution:**
- Re-run migration if needed
- Check logs for unique constraint violations (expected on retry)

### WAL Growing Without Bound

**Check:**
1. Checkpoint routine running: Check logs for "WAL checkpoint"
2. WAL size: `ls -lh /var/lib/belabox-metrics/metrics.db-wal`
3. Checkpoint config: `wal_checkpoint_interval` and `wal_checkpoint_size_mb`

**Solution:**
- Manual checkpoint: `sqlite3 metrics.db "PRAGMA wal_checkpoint(TRUNCATE)"`
- Reduce checkpoint interval or size threshold

### CPU Usage Incorrect (>100% or negative)

**Check:**
1. Delta calculation being used (not single-read)
2. First sample being skipped
3. Wraparound detection working

**Debug:**
```bash
# Check CPU collector logs
journalctl -u metrics-collector | grep "cpu.usage"

# Verify counter values aren't wrapping
cat /proc/stat | grep "^cpu "
```

**Solution:**
- Verify two-read implementation
- Check wraparound handling logic

### Network Metrics Include Unwanted Interfaces

**Check config:**
```yaml
metrics:
  - name: network.traffic
    config:
      exclude_ifaces: ["lo", "docker.*", "veth.*", "br-.*", "your-pattern"]
```

### Clock Skew Warning

**Check:**
```bash
# Compare local time with VM
curl -I http://localhost:8428 | grep Date
date -u
```

**Solution:**
- Sync system clock with NTP: `sudo ntpdate pool.ntp.org`
- Or: `sudo systemctl restart systemd-timesyncd`

### VictoriaMetrics Not Receiving Metrics

**Check:**
1. VM running: `docker ps | grep victoriametrics`
2. Endpoint accessible: `curl http://localhost:8428/health`
3. Collector logs for upload errors
4. JSONL format with manual test:
   ```bash
   echo '{"metric":{"__name__":"test","device_id":"test"},"values":[42],"timestamps":[1697040000000]}' | \
   gzip | \
   curl -X POST http://localhost:8428/api/v1/import \
     -H "Content-Encoding: gzip" \
     --data-binary @-
   ```

### Health Endpoint Returns Wrong Status

**Check:**
1. Thresholds in config: `degraded_threshold` and `error_threshold`
2. Component statuses in `/health` JSON
3. Time since last upload
4. Pending metrics count

**Adjust thresholds if needed:**
```yaml
health:
  degraded_threshold:
    pending_metrics: 10000    # Increase if false positives
    upload_age_seconds: 120   # Increase if slow network
```

---

## Next Steps After Completion

Once Milestone 2 is complete, move to Milestone 3:
- Belabox/encoder metrics (journald parsing)
- HDMI input metrics (v4l2-ctl)
- Server-side SRT stats (when receiver available)
- Priority queue (P0/P1/P2/P3 upload ordering)
- Backfill after network recovery
- Data retention and rotation policies
- Grafana dashboard creation
- Deploy to Orange Pi for real hardware testing
- TLS certificate pinning

---

## Estimated Timeline

**Realistic timeline with full polish:**

- **Day 1:** Database + Upload Fix (7-9h)
- **Day 2:** Collectors (7-9h)
- **Day 3:** Health + Monitoring (6-8h)
- **Day 4:** VictoriaMetrics + Testing (6-8h)

**Total: 26-34 hours = 3-4 days**

This accounts for:
- Implementation
- Comprehensive testing
- Documentation
- Debugging/iteration
- Integration testing

**Acceptable given:**
- 10 major redlines addressed
- Additional security/ops improvements
- Significantly more robust than original plan
- Production-ready quality

---

## Engineering Review: Production Readiness Assessment

**Review Date:** 2025-10-12
**Verdict:** Strong direction with surgical fixes needed before merge

### Overall Assessment

The M2 design demonstrates production-grade thinking: dedup key, chunked JSONL to VictoriaMetrics, jittered backoff, graduated health monitoring, and WAL management are exactly what this pipeline needs for SBC deployments. With the fixes below, this will be production-credible on Orange Pi hardware.

---

## 🔴 Merge-Blocking Issues (Fix Before Landing)

### 1. HTTP Upload: Wrong Call Shape + Missing Headers

**Problem:** Some code uses `httpClient.Post(vmURL, gzipped)` which:
- Has wrong signature (won't compile)
- Misses required headers: `Content-Type`, `Content-Encoding`, `Authorization`
- VictoriaMetrics won't accept gzipped JSONL without proper headers

**Impact:** Uploads will fail with 400/415 errors

**Fix Required:**
```go
req, err := http.NewRequest("POST", cfg.Remote.URL, bytes.NewReader(gzipped))
if err != nil { return err }

req.Header.Set("Content-Type", "application/json")
req.Header.Set("Content-Encoding", "gzip")
if cfg.Remote.AuthToken != "" {
    req.Header.Set("Authorization", "Bearer "+cfg.Remote.AuthToken)
}

resp, err := httpClient.Do(req)
```

**Notes:**
- Keep `Transport.DisableCompression=true` (you control gzip manually)
- `Content-Encoding: gzip` tells server body is compressed
- Aligns code with docs that already promise "gzip compression"

---

### 2. SQLite WAL Checkpoint: PRAGMA Result Handling

**Problem:** One variant scans only 2 integers from `PRAGMA wal_checkpoint(TRUNCATE)`:
```go
var busy, log int
db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &log)
```

But SQLite returns **3 values**: `(busy, log, checkpointed)`. This scan will fail.

**Impact:** WAL checkpoint silently fails, WAL grows unbounded on disk

**Fix Required (Option A - Simplest):**
```go
// Prefer Exec to avoid scan breakage across SQLite builds
if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
    log.Error("WAL checkpoint failed", "error", err)
}
```

**Fix Required (Option B - Full scan):**
```go
var busy, logFrames, ckptFrames int
if err := s.db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &ckptFrames); err != nil {
    log.Error("WAL checkpoint failed", "error", err)
    return
}
```

**Decision:** Use Option A for consistency. Keep size-triggered checkpoint (≥64 MB) and hourly cadence. Emit meta-metrics: `storage.wal_checkpoint_duration_ms`, `storage.wal_bytes_reclaimed`.

---

### 3. Disk I/O Units: Unify on Per-Device Sector Size

**Problem:** Two implementations exist:
- Constant 512 bytes/sector (wrong for NVMe/eMMC)
- Reads `/sys/block/<dev>/queue/logical_block_size` (correct)

On Orange Pi 5+ hardware:
- NVMe typically uses 4096-byte sectors
- eMMC often uses 4096-byte sectors
- SATA uses 512-byte sectors

Using constant 512 causes **up to 8× error** in bytes counters.

**Impact:** `disk.read_bytes`, `disk.write_bytes` metrics will be wildly incorrect on NVMe/eMMC

**Fix Required:**
```go
var sectorSizeCache sync.Map

func sectorSizeOf(dev string) int64 {
    if v, ok := sectorSizeCache.Load(dev); ok {
        return v.(int64)
    }

    p := filepath.Join("/sys/block", dev, "queue", "logical_block_size")
    if b, err := os.ReadFile(p); err == nil {
        if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil && n > 0 {
            sectorSizeCache.Store(dev, n)
            return n
        }
    }

    // Fallback for old kernels or weird devices
    sectorSizeCache.Store(dev, int64(512))
    return 512
}
```

**Additional Requirements:**
- Filter to **whole devices only**: `nvme0n1`, `mmcblk0`, `sda` (not partitions like `nvme0n1p1`, `sda1`)
- Use regex: `^(nvme\d+n\d+|mmcblk\d+|sd[a-z]+|vd[a-z]+)$`
- Retain both ops counters AND bytes counters (both valuable)

---

### 4. JSONL Metric Name Sanitization: Make it Consistent End-to-End

**Problem:** Spec says "store dotted names in SQLite, sanitize to PromQL-safe on upload only", but some examples show dotted names in JSONL output.

**Impact:**
- PromQL queries will be awkward: `"cpu.temperature"` instead of `cpu_temperature_celsius`
- Inconsistent with Prometheus naming conventions

**Fix Required:**

**Storage (SQLite):** Use dotted names
```
cpu.temperature
memory.used_bytes
disk.read_bytes
```

**Upload (JSONL to VictoriaMetrics):** Sanitize to PromQL-safe
```go
func sanitizeMetricName(name string) string {
    // dots → underscores
    safe := strings.ReplaceAll(name, ".", "_")

    // Add unit suffixes if missing
    if strings.Contains(name, "temperature") && !strings.HasSuffix(safe, "_celsius") {
        safe += "_celsius"
    }
    if strings.Contains(name, "bytes") && !strings.HasSuffix(safe, "_bytes") {
        safe += "_bytes"
    }

    // Counters must end with _total
    if isCounter(name) && !strings.HasSuffix(safe, "_total") {
        safe += "_total"
    }

    return safe
}
```

**Result in JSONL:**
```json
{"metric": {"__name__": "cpu_temperature_celsius", "zone": "cpu-thermal"}, ...}
{"metric": {"__name__": "memory_used_bytes", "device_id": "..."}, ...}
{"metric": {"__name__": "disk_read_bytes_total", "device": "nvme0n1"}, ...}
{"metric": {"__name__": "network_tx_bytes_total", "interface": "eth0"}, ...}
```

**Critical:** This happens **only in JSONL builder**, not in storage layer.

---

### 5. Clock Skew Check: Reuse Auth + Allow Custom Endpoint

**Problem:** Current implementation:
- Bare `HEAD /` may fail if VM requires auth (returns 401/403)
- Some proxies return 405 for HEAD on import paths
- Hardcoded to VM root endpoint

**Impact:** Clock skew check fails unnecessarily, logs spam with warnings

**Fix Required:**
```go
// In config
type MonitoringConfig struct {
    SkewURL           string        `yaml:"skew_url"`              // Default: http://vm:8428/health
    SkewWarnThreshold time.Duration `yaml:"skew_warn_threshold"`   // Default: 2s
}

// In clock skew checker
func checkClockSkew(cfg Config) error {
    url := cfg.Monitoring.SkewURL
    if url == "" {
        url = cfg.Remote.URL // fallback to VM import URL
    }

    req, err := http.NewRequest("HEAD", url, nil)
    if err != nil {
        return err
    }

    // Reuse same auth as uploads
    if cfg.Remote.AuthToken != "" {
        req.Header.Set("Authorization", "Bearer "+cfg.Remote.AuthToken)
    }

    resp, err := httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    serverDate := resp.Header.Get("Date")
    // ... compute skew from serverDate vs time.Now() ...

    // Expose as metric
    recordMetric("time.skew_ms", skewMs, nil)

    if skewMs > cfg.Monitoring.SkewWarnThreshold.Milliseconds() {
        log.Warn("Clock skew detected", "skew_ms", skewMs)
    }

    return nil
}
```

**Notes:**
- Allow `skew_url` override in config for proxied/load-balanced setups
- Default to `/health` endpoint (safer than `/api/v1/import`)
- Emit `time.skew_ms` as gauge in meta-metrics

---

## 🟠 Important Correctness & Resilience

### A. Network Collector: Counters vs Deltas

**Current Status:** Correctly export raw counters from `/proc/net/dev` and recommend `rate()` in PromQL queries.

**Inconsistency:** One variant added wraparound detection with cached last sample, another doesn't.

**Decision:**
- **Export raw counters** (correct approach for TSDB)
- **Wraparound guard is optional** on 64-bit kernels but harmless
- **Keep wraparound detection** if you have the cache (better diagnostics)
- **Log and skip** absurd negative deltas when detected

**Example:**
```go
// In network collector
lastRx, ok := cache[iface]
if ok && currentRx < lastRx {
    log.Warn("Network counter wrapped",
        "interface", iface,
        "last", lastRx,
        "current", currentRx)
    // Don't emit point, wait for next sample
    continue
}
cache[iface] = currentRx
recordMetric("network.rx_bytes", float64(currentRx), tags)
```

**PromQL Query (client-side rate):**
```promql
rate(network_rx_bytes_total{device_id="belabox-001"}[30s]) * 8 / 1e6  # Mbps
```

---

### B. CPU Usage Delta Calculation

**Current Implementation:** Two-read strategy with first-sample skip (correct).

**Optimization Opportunity:**
- Use aggregate `cpu` line directly instead of summing cores
- Cheaper and avoids drift if cores hot-plug
- Still keep per-core metrics for detailed analysis

**Recommended Structure:**
```go
// Emit both aggregate and per-core
recordMetric("cpu.usage_percent", overallPct, map[string]string{
    "core": "all",
})

for i, corePct := range perCorePcts {
    recordMetric("cpu.usage_percent", corePct, map[string]string{
        "core": strconv.Itoa(i),
    })
}
```

**Optional Enhancement:** Add `iowait` percentage for better saturation insight (no extra reads needed, it's in `/proc/stat`).

---

### C. Deduplication Key: Scope Validation

**Current Design:**
```
dedup_key = sha256(name|ts_ms|device|canonical_tags)
```

**Critical:** Do NOT include `session_id` in dedup key. If you did, you'd re-admit duplicates after restarts.

**Validation:** Current design is correct. The unique index on `dedup_key` guarantees:
- Idempotency across crashes/retries
- Same point never inserted twice
- Works even after process restarts

**Good.** Keep it.

---

### D. Chunk Sizing: Byte-Cap Enforcement

**Current Plan:** Target ~128-256 KB/chunk after gzip (~40% compression).

**Problem:** Cardinality spikes could create pathological chunks (e.g., 10k unique device tags).

**Fix Required:**
```go
const MaxChunkSizeBytes = 256 * 1024 // 256 KB hard limit

func buildChunks(metrics []Metric) []Chunk {
    chunks := []Chunk{}
    currentChunk := []Metric{}

    for _, m := range metrics {
        currentChunk = append(currentChunk, m)

        if len(currentChunk) >= 50 {
            jsonl := buildJSONL(currentChunk)
            gzipped := gzip(jsonl)

            if len(gzipped) > MaxChunkSizeBytes {
                // Bisect: split currentChunk in half, retry
                log.Warn("Chunk too large, splitting",
                    "size", len(gzipped),
                    "metrics", len(currentChunk))
                // ... split and retry ...
            } else {
                chunks = append(chunks, Chunk{Data: gzipped})
                currentChunk = []Metric{}
            }
        }
    }

    // Handle remainder
    if len(currentChunk) > 0 {
        chunks = append(chunks, buildFinalChunk(currentChunk))
    }

    return chunks
}
```

**Benefit:** Keeps P99 upload latency predictable on weak uplinks (3G/4G with ~1 Mbps).

---

### E. Backoff + Rate Limiting: Honor Retry-After

**Current Plan:** Exponential backoff with ±20% jitter (good).

**Enhancement Needed:** Honor `Retry-After` header on 429/503.

**Fix Required:**
```go
func backoffDuration(attempt int, resp *http.Response) time.Duration {
    base := time.Duration(5) * time.Second * time.Duration(math.Pow(3, float64(attempt)))
    jitter := time.Duration(rand.Float64()*0.4-0.2) * base  // ±20%
    backoff := base + jitter

    if resp != nil && (resp.StatusCode == 429 || resp.StatusCode == 503) {
        if ra := resp.Header.Get("Retry-After"); ra != "" {
            if secs, err := strconv.Atoi(ra); err == nil {
                retryAfter := time.Duration(secs) * time.Second
                if retryAfter > backoff {
                    log.Info("Honoring Retry-After",
                        "retry_after_sec", secs,
                        "calculated_backoff_sec", backoff.Seconds())
                    return retryAfter
                }
            }
        }
    }

    return backoff
}
```

**Log Context:** Add `retry_after_sec` to upload logs when honored.

---

### F. SQLite Pragmas: Add at Database Open

**Current:** Schema creates tables/indexes.

**Missing:** Performance and durability pragmas.

**Fix Required:**
```go
func openDB(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, err
    }

    // One-time pragmas for all connections
    pragmas := []string{
        "PRAGMA journal_mode=WAL",           // Enable WAL mode
        "PRAGMA synchronous=NORMAL",         // Good durability/throughput trade
        "PRAGMA temp_store=MEMORY",          // Faster temp tables
        "PRAGMA busy_timeout=5000",          // 5s wait instead of immediate SQLITE_BUSY
        "PRAGMA cache_size=-64000",          // ~64 MB cache (negative = KB)
    }

    for _, pragma := range pragmas {
        if _, err := db.Exec(pragma); err != nil {
            return nil, fmt.Errorf("%s: %w", pragma, err)
        }
    }

    return db, nil
}
```

**Impact:** Materially reduces I/O spikes on Orange Pi under concurrent collect + upload.

**Meta-Metrics:** Already planned `storage.wal_size_bytes` gauge (good).

---

### G. Health Semantics: Calibrate to Config

**Current Plan:** Tie `degraded`/`error` to `upload_age_seconds` and `pending_metrics` thresholds (good).

**Enhancement:** Make thresholds **relative to `remote.upload_interval`** so they stay correct if ops changes the interval.

**Fix Required:**
```go
func calculateHealthStatus(state State, cfg Config) HealthStatus {
    uploadInterval := cfg.Remote.UploadInterval.Seconds()

    degradedAge := uploadInterval * 2    // 2× interval
    errorAge := uploadInterval * 20       // 20× interval

    degradedPending := 5000
    errorPending := 20000

    // Use state.LastUploadTime, state.PendingCount
    age := time.Since(state.LastUploadTime).Seconds()

    if age > errorAge || state.PendingCount > errorPending {
        return HealthError
    }
    if age > degradedAge || state.PendingCount > degradedPending {
        return HealthDegraded
    }
    return HealthOK
}
```

**Config Comments:** Already imply this, just enforce at runtime.

---

## 🟡 Security & Ops Polish

### Systemd Hardening

**Current Hardening (Good):**
- `NoNewPrivileges=true`
- `ProtectSystem=strict`
- `CPUQuota=20%`
- `MemoryMax=200M`
- `User=metrics` (non-root)

**Verification Needed:**
- Confirm `/proc/{pid}/stat` reads work with `ProtectProc=` (you didn't set it, which is fine)
- Confirm token file (`/etc/metrics-collector/token`) is readable under `ReadOnlyPaths`
- Token file must be `chmod 0600`, owned by `metrics` user

**Pre-Deployment Checklist:**
```bash
# Create metrics user
sudo useradd -r -s /bin/false metrics

# Set token permissions
sudo install -m 0600 -o metrics -g metrics token /etc/metrics-collector/token

# Test service can start
sudo systemctl start metrics-collector
sudo systemctl status metrics-collector

# Verify no permission errors
sudo journalctl -u metrics-collector | grep -i "permission denied"
```

---

### Single-Process Guard

**Current Plan:** `flock` on PID file alongside systemd (belt-and-suspenders, good).

**Implementation:**
```go
func acquireLock(path string) (*os.File, error) {
    f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
    if err != nil {
        return nil, err
    }

    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
        f.Close()
        return nil, fmt.Errorf("another instance running: %w", err)
    }

    // Write PID
    f.Truncate(0)
    fmt.Fprintf(f, "%d\n", os.Getpid())

    return f, nil
}

// In main()
lockFile, err := acquireLock("/var/run/metrics-collector.lock")
if err != nil {
    log.Fatal("Failed to acquire lock", "error", err)
}
defer lockFile.Close()
```

**Keep it.** Works well with systemd's `Restart=on-failure`.

---

### Health Endpoint: Fail-Open Ready Check

**Current:** `/health` returns ok/degraded/error immediately.

**Enhancement for Orchestrators:** Add `/health/ready` that:
- Returns 503 until first successful upload completes
- Then returns same status as `/health`

**Use Case:** Load balancers, Kubernetes readiness probes.

**Implementation:**
```go
var firstUploadComplete atomic.Bool

func healthReadyHandler(w http.ResponseWriter, r *http.Request) {
    if !firstUploadComplete.Load() {
        http.Error(w, "waiting for first upload", http.StatusServiceUnavailable)
        return
    }

    // Otherwise same as /health
    healthHandler(w, r)
}

// In upload success path
func markUploadSuccess() {
    firstUploadComplete.Store(true)
}
```

**Optional but useful for production deploys.**

---

## PromQL Examples (Drop-In Queries)

These assume upload-path sanitization (underscores + `_total` for counters):

### CPU Overall Usage (%)
```promql
avg_over_time(cpu_usage_percent{device_id="belabox-001", core="all"}[1m])
```

### Per-Interface Throughput (Mbps)
```promql
(rate(network_tx_bytes_total{device_id="belabox-001", interface=~"^(wwan|usb).*"}[30s]) * 8) / 1e6
```

### Disk Write Bandwidth (MB/s)
```promql
rate(disk_write_bytes_total{device_id="belabox-001"}[1m]) / 1e6
```

### Pending Backlog Watch
```promql
storage_metrics_pending_upload{device_id="belabox-001"}
```

### Clock Skew Watch
```promql
time_skew_ms{device_id="belabox-001"}
```

### Temperature Alert (RK3588)
```promql
max(cpu_temperature_celsius{device_id="belabox-001"}) > 80
```

**Note:** All names sanitized at upload time per fix #4.

---

## Test Cases to Add (High Value)

These complement existing unit/integration tests:

### 1. HTTP Upload Headers Test
```go
func TestUploadSetsRequiredHeaders(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
        assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))
        assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
        w.WriteHeader(204)
    }))
    defer server.Close()

    // ... upload test metrics to server.URL ...
}
```

### 2. WAL Checkpoint Reduces Size
```go
func TestWALCheckpointReducesSize(t *testing.T) {
    db := openTestDB(t)

    // Insert 100k metrics to grow WAL
    for i := 0; i < 100000; i++ {
        insertMetric(db, testMetric(i))
    }

    walSizeBefore := getWALSize(db)
    assert.Greater(t, walSizeBefore, 64*1024*1024) // >64 MB

    // Trigger checkpoint
    _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
    assert.NoError(t, err)

    walSizeAfter := getWALSize(db)
    assert.Less(t, walSizeAfter, walSizeBefore/2) // At least 50% reduction
}
```

### 3. Disk Sector Size Correctness
```go
func TestDiskBytesUseCorrectSectorSize(t *testing.T) {
    // Mock sysfs
    mockSysfs := map[string]string{
        "/sys/block/nvme0n1/queue/logical_block_size": "4096",
        "/sys/block/sda/queue/logical_block_size":      "512",
    }

    collector := &DiskCollector{sysfsReader: mockSysfsReader(mockSysfs)}

    // Read /proc/diskstats with known sector counts
    metrics := collector.Collect()

    nvmeMetric := findMetric(metrics, "disk.read_bytes", "nvme0n1")
    assert.Equal(t, 10*4096, nvmeMetric.Value) // 10 sectors × 4096

    sdaMetric := findMetric(metrics, "disk.read_bytes", "sda")
    assert.Equal(t, 10*512, sdaMetric.Value)   // 10 sectors × 512
}
```

### 4. Name Sanitization (Storage vs Upload)
```go
func TestMetricNameSanitization(t *testing.T) {
    // Store with dotted name
    db := openTestDB(t)
    m := Metric{Name: "cpu.temperature", Value: 65.0}
    insertMetric(db, m)

    // Retrieve from DB (should still be dotted)
    stored := queryMetric(db, 1)
    assert.Equal(t, "cpu.temperature", stored.Name)

    // Build JSONL for upload (should be sanitized)
    jsonl := buildJSONL([]Metric{stored})
    assert.Contains(t, jsonl, `"__name__":"cpu_temperature_celsius"`)
    assert.NotContains(t, jsonl, "cpu.temperature")
}
```

### 5. Retry-After Honored
```go
func TestBackoffHonorsRetryAfter(t *testing.T) {
    attempt := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if attempt == 0 {
            w.Header().Set("Retry-After", "10")
            w.WriteHeader(429)
        } else {
            w.WriteHeader(204)
        }
        attempt++
    }))
    defer server.Close()

    start := time.Now()
    uploader := NewUploader(Config{Remote: RemoteConfig{URL: server.URL}})
    err := uploader.Upload(testMetrics)
    elapsed := time.Since(start)

    assert.NoError(t, err)
    assert.GreaterOrEqual(t, elapsed.Seconds(), 10.0) // Waited at least 10s
}
```

### 6. Clock Skew Detection with Auth
```go
func TestClockSkewUsesAuth(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
        w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
        w.WriteHeader(200)
    }))
    defer server.Close()

    cfg := Config{
        Remote:     RemoteConfig{AuthToken: "test-token"},
        Monitoring: MonitoringConfig{SkewURL: server.URL},
    }

    err := checkClockSkew(cfg)
    assert.NoError(t, err)
}
```

---

## Ready-to-Merge Checklist (M2)

**Code Changes:**
- [ ] HTTP requests set `Content-Type`, `Content-Encoding: gzip`, and `Authorization` headers
- [ ] HTTP upload uses `http.NewRequest` + `httpClient.Do` (correct signature)
- [ ] WAL checkpoint uses `Exec` (no scan) or scans all 3 return values consistently
- [ ] Disk I/O uses per-device `logical_block_size` from sysfs with 512 fallback
- [ ] Disk device filter excludes partitions (only whole devices)
- [ ] JSONL output uses sanitized names (`_` not `.`, `_total` for counters, unit suffixes)
- [ ] Storage layer keeps dotted names (sanitization only at upload)
- [ ] Clock skew check reuses auth token from config
- [ ] Clock skew endpoint configurable (`skew_url` in config)
- [ ] Backoff honors `Retry-After` header when present (max with calculated backoff)
- [ ] SQLite pragmas added at open: `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`
- [ ] Health thresholds computed as functions of `upload_interval` (2× for degraded, 20× for error)

**Testing:**
- [ ] Test: HTTP upload sets all required headers
- [ ] Test: WAL checkpoint reduces WAL file size after burst inserts
- [ ] Test: Disk bytes correct for 4096-byte and 512-byte sector sizes
- [ ] Test: Metric names sanitized in JSONL but dotted in storage
- [ ] Test: Retry-After honored and backoff extended appropriately
- [ ] Test: Clock skew check includes Authorization header

**Documentation:**
- [ ] PromQL examples updated with sanitized names (`_total`, `_celsius`, etc.)
- [ ] Config examples show `skew_url` option
- [ ] Troubleshooting section mentions checking headers with `tcpdump`/`curl -v`

**Pre-Deploy Verification:**
- [ ] Compile succeeds on Linux arm64 (cross-compile or on Pi)
- [ ] Systemd service starts without permission errors
- [ ] Token file permissions: `0600`, owned by `metrics` user
- [ ] First upload to VictoriaMetrics succeeds (check VM logs)
- [ ] PromQL queries return expected data with sanitized names
- [ ] Health endpoint returns `ok` after successful upload
- [ ] WAL file size < 100 MB after 24h of operation

**Acceptance Criteria (from M2 scope):**
- [ ] No duplicate metrics uploaded (even across crashes/retries)
- [ ] CPU usage shows accurate percentages (verified against `top`)
- [ ] Network bytes counters increase monotonically (no negative deltas)
- [ ] Disk bytes match expected order of magnitude for known I/O
- [ ] VictoriaMetrics import succeeds with chunked JSONL
- [ ] Upload retries with backoff on transient failures
- [ ] Health status reflects actual system state (ok/degraded/error)

---

## Suggested Implementation PRs

If you'd like, I can draft focused PRs for:

1. **HTTP Upload Headers** - Fix signature + add required headers (10 min)
2. **WAL Checkpoint** - Uniform `Exec` approach (5 min)
3. **Disk Sector Size** - Per-device sysfs read with cache (15 min)
4. **Name Sanitization** - Consistent upload-time sanitization (20 min)
5. **Clock Skew Auth** - Reuse token + configurable endpoint (10 min)

These are surgical fixes to the core upload/collection paths. The rest of the M2 plan is solid and can proceed in parallel.

---

## Summary

**Verdict:** Strong architectural direction. With these 5 merge-blocking fixes (HTTP headers, WAL checkpoint, disk sector size, name sanitization, clock skew auth) plus the correctness enhancements (chunk size cap, Retry-After, SQLite pragmas), M2 will be production-credible on Orange Pi hardware.

**Estimated Fix Time:** 2-3 hours for all merge-blockers + tests.

**Recommendation:** Land the fixes, run the test suite, deploy to a dev Orange Pi for 24h soak test, then promote to production fleet.
