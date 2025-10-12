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

