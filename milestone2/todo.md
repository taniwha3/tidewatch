# Milestone 2 TODO Checklist

This comprehensive checklist covers all tasks required to complete Milestone 2.

## Day 1: Database + Upload Fix (7-9 hours)

### Database Schema Migration (2-3h) ✅ COMPLETE
- [x] Create `internal/storage/migration.go` (already exists in storage.go)
- [x] Implement `GetSchemaVersion()` function
- [x] Implement `Migrate()` function
- [x] Migration v4: Add new columns (value_text, value_type for string metrics)
- [x] Migration v4: Create sessions table
- [x] Migration v2: Create upload_checkpoints table (already done)
- [x] Migration v2: Create new indexes including unique index on dedup_key (already done)
- [x] Migration v5: Regenerate ALL dedup_keys with new format (backwards compatibility fix)
- [x] Unit tests: v1→v5 migrations
- [x] Unit tests: Migration idempotency
- [x] Unit tests: Migration error cases

### Dedup Key Generation (1h) ✅ COMPLETE
- [x] Add `GenerateDedupKey()` method to `models.Metric` (already exists in storage.go)
- [x] Implement canonical tag ordering for consistent hashing
- [x] Implement SHA256 hashing for dedup key
- [x] Include ValueType in dedup key to prevent type-change collisions
- [x] Unit tests: Dedup key consistency
- [x] Unit tests: Collision resistance
- [x] Unit tests: Value type changes force new dedup key
- [x] Unit tests: Migration v5 regenerates dedup keys correctly

### Chunked Upload Strategy (2-3h) ✅ COMPLETE
- [x] Modify upload loop to use chunks (50 metrics each)
- [x] Generate batch_id (UUID) for tracking (in uploader.go)
- [x] Sort metrics by timestamp within chunk
- [x] Implement JSONL building for VictoriaMetrics format
- [x] Add gzip compression (BestSpeed for ARM efficiency)
- [x] Configure HTTP transport with connection pooling (MaxIdleConns=10)
- [x] Target 128-256 KB gzipped payload per chunk
- [x] Implement byte-size limiting (256 KB max with automatic bisecting)
- [x] Unit tests: Chunking logic
- [x] Unit tests: Timestamp sorting
- [x] Unit tests: Gzip compression
- [x] Unit tests: Byte-size limits

### Partial Success Handling (1-2h)
- [ ] Simplified strategy: 2xx = mark entire chunk as uploaded
- [ ] Mark only accepted metrics as uploaded when VM provides details
- [ ] Save checkpoint per successful chunk
- [ ] Increment partial_success counter
- [ ] Unit tests: Partial ack scenarios
- [ ] Unit tests: Fallback to full-chunk success on 2xx

### Jittered Backoff (1h)
- [ ] Implement `calculateBackoff()` with exponential backoff
- [ ] Add ±20% jitter calculation
- [ ] Seed random number generator for jitter
- [ ] Implement `parseRetryAfter()` for HTTP Retry-After header
- [ ] Unit tests: Backoff values
- [ ] Unit tests: Jitter range (±20%)
- [ ] Unit tests: Retry-After parsing (seconds and HTTP-date)

## Day 2: Collectors (7-9 hours)

### CPU Delta Collector (2-3h)
- [ ] Create `internal/collector/cpu.go`
- [ ] Implement two-read strategy with cached counters
- [ ] Per-core delta calculation
- [ ] Aggregate "all" core metric calculation
- [ ] Wraparound detection and handling
- [ ] Skip first sample (no previous to compare)
- [ ] Division by zero protection
- [ ] Mock implementation for macOS
- [ ] Unit tests: Delta calculation
- [ ] Unit tests: Wraparound handling
- [ ] Unit tests: First-sample skip
- [ ] Unit tests: Aggregate calculation

### Memory Collector (1h)
- [ ] Create `internal/collector/memory.go`
- [ ] Parse `/proc/meminfo` for MemTotal, MemAvailable, SwapTotal, SwapFree
- [ ] Implement canonical used calculation (MemTotal - MemAvailable)
- [ ] Export memory.used_bytes, memory.available_bytes, memory.swap_used_bytes
- [ ] Export memory.total_bytes, memory.swap_total_bytes (for percentage calculations)
- [ ] Mock for macOS
- [ ] Unit tests: Parsing logic
- [ ] Unit tests: Canonical used calculation

### Disk I/O Collector (1-2h)
- [ ] Create `internal/collector/disk.go`
- [ ] Parse `/proc/diskstats`
- [ ] Implement per-device sector size detection from sysfs
- [ ] Cache sector sizes (512 for SATA, 4096 for NVMe/eMMC)
- [ ] Sector→byte conversion for read/write bytes
- [ ] Expose ops/s and bytes/s
- [ ] Whole-device regex pattern (skip partitions)
- [ ] Configurable device pattern override
- [ ] Mock for macOS
- [ ] Unit tests: Sector size detection
- [ ] Unit tests: Sector→byte conversion
- [ ] Unit tests: Parsing logic
- [ ] Unit tests: Partition filtering (nvme0n1 vs nvme0n1p1)

### Network Collector (2-3h)
- [ ] Create `internal/collector/network.go`
- [ ] Parse `/proc/net/dev`
- [ ] Regex-based interface filtering
- [ ] Default exclusions: lo, docker*, veth*, br-*, wlan.*mon, virbr.*, wwan.*, wwp.*, usb.*
- [ ] Configurable includes/excludes
- [ ] Counter wraparound detection
- [ ] Cardinality guard: hard cap on interface count (default 32)
- [ ] Emit network.interfaces_dropped_total when cap hit
- [ ] Mock for macOS
- [ ] Unit tests: Filtering logic
- [ ] Unit tests: Regex patterns
- [ ] Unit tests: Parsing
- [ ] Unit tests: Wraparound detection
- [ ] Unit tests: Cardinality hard cap

### Clock Skew Detection (1h)
- [ ] Create `internal/monitoring/clock.go`
- [ ] Implement `detectClockSkew()` using Date header
- [ ] Use separate clock_skew_url (not ingest URL)
- [ ] Periodic checking routine (5min interval)
- [ ] Warn on >2s skew
- [ ] Expose time.skew_ms metric
- [ ] Log both clock_skew_url and ingest_url for diagnostics
- [ ] Unit tests: Skew calculation
- [ ] Unit tests: Warning threshold

## Day 3: Health + Monitoring (6-8 hours)

### Graduated Health Status (2-3h)
- [ ] Create `internal/health/health.go`
- [ ] Implement status calculation with ok/degraded/error rules
- [ ] Per-component status tracking (collectors, uploader, storage, time)
- [ ] `/health` endpoint with full JSON
- [ ] `/health/live` liveness probe
- [ ] `/health/ready` readiness probe (200 only if ok)
- [ ] Integrate with main collector
- [ ] OK rules: All collectors healthy, uploads within 2× interval, pending < 5000
- [ ] Degraded rules: ≥1 collector error OR no upload 2×-10× interval OR pending 5000-10000
- [ ] Error rules: All collectors failing OR no upload >10min AND pending >10000
- [ ] Unit tests: Status calculation
- [ ] Unit tests: Thresholds
- [ ] Unit tests: Component rollup logic
- [ ] Unit tests: JSON response format
- [ ] Unit tests: Liveness/readiness probes

### Meta-Monitoring (2h)
- [ ] Create `internal/monitoring/metrics.go`
- [ ] Implement collector.metrics_collected_total counter
- [ ] Implement collector.metrics_failed_total counter
- [ ] Implement collector.collection_duration_seconds histogram
- [ ] Implement uploader.metrics_uploaded_total counter
- [ ] Implement uploader.upload_failures_total counter
- [ ] Implement uploader.upload_duration_seconds histogram
- [ ] Implement uploader.partial_success_total counter
- [ ] Implement storage.database_size_bytes gauge
- [ ] Implement storage.wal_size_bytes gauge
- [ ] Implement storage.metrics_pending_upload gauge
- [ ] Implement time.skew_ms gauge
- [ ] Send meta-metrics to storage/VM
- [ ] Unit tests: Counter increments
- [ ] Unit tests: Gauge updates
- [ ] Unit tests: Histogram recordings

### Enhanced Logging (1-2h)
- [ ] Migrate from `log` to `log/slog`
- [ ] Create JSON formatter
- [ ] Create console formatter for development
- [ ] Add collection contextual fields: collector_name, count, duration_ms, session_id
- [ ] Add upload contextual fields: batch_id, chunk_index, attempt, backoff_ms, http_status, bytes_sent, bytes_rcvd, duration_ms
- [ ] Add retry contextual fields: attempt, backoff_ms, error, error_type
- [ ] Add error contextual fields: error, error_type, stack (if panic)
- [ ] Configuration for level (debug, info, warn, error)
- [ ] Configuration for format (json, console)
- [ ] Update all log statements throughout codebase
- [ ] Unit tests: JSON formatter output
- [ ] Unit tests: Console formatter output
- [ ] Unit tests: Required field presence

### WAL Checkpoint Routine (1h)
- [ ] Add `startWALCheckpointRoutine()` to storage
- [ ] Hourly ticker
- [ ] Size-based triggering (WAL > 64 MB)
- [ ] Implement `checkpointWAL()` with TRUNCATE mode
- [ ] Expose wal_size in meta-metrics
- [ ] Final checkpoint on shutdown
- [ ] Log checkpoint operations with size info
- [ ] Emit storage.wal_checkpoint_duration_ms metric
- [ ] Emit storage.wal_bytes_reclaimed metric
- [ ] Unit tests: Checkpoint triggers
- [ ] Unit tests: Size checking
- [ ] Unit tests: Shutdown checkpoint

## Day 4: VictoriaMetrics + Testing (6-8 hours)

### VictoriaMetrics Integration (2h)
- [ ] Create `internal/uploader/victoriametrics.go`
- [ ] JSONL formatter for VM format
- [ ] Metric name sanitization (dots→underscores for PromQL compatibility)
- [ ] Unit suffix normalization (_bytes, _celsius, _total, _percent)
- [ ] Labels mapping (__name__, device_id, tags)
- [ ] Skip string metrics (value_type=1) in JSONL
- [ ] Test with local VM instance
- [ ] Integration test: End-to-end ingestion
- [ ] Unit tests: JSONL format correctness
- [ ] Unit tests: Metric name sanitization (PromQL regex)
- [ ] Unit tests: String metric filtering

### Docker Setup (1h)
- [ ] Create `docker/docker-compose.yml`
- [ ] Pin VictoriaMetrics version (v1.97.1 or later)
- [ ] Configure VM ports (8428)
- [ ] Configure retention period (30d)
- [ ] Add VM healthcheck
- [ ] Configure logging (json-file driver, 10m max-size)
- [ ] Create `docker/Dockerfile.receiver`
- [ ] Create `docker/README.md` with setup guide
- [ ] Test local deployment
- [ ] Add PromQL sanity query examples

### Expanded Test Coverage (2-3h)
- [ ] Test: No duplicate uploads (same batch retried → no new rows)
- [ ] Test: Partial success (VM accepts 25/50 → only 25 marked)
- [ ] Test: Partial success fallback (200 without details → full chunk marked)
- [ ] Test: Transport soak (60min with VM restarts)
- [ ] Test: Clock skew detection (mock VM with skewed time)
- [ ] Test: Proxy clock skew (Date header from proxy)
- [ ] Test: WAL growth (insert many → checkpoint → size reduced)
- [ ] Test: Counter wraparound (CPU stats wrap → skip sample)
- [ ] Test: High-cardinality interface guard
- [ ] Test: Metric name sanitization (dots→underscores)
- [ ] Test: Chunk replay with dedup key
- [ ] Test: WAL checkpoint growth prevention
- [ ] Test: Interface cardinality hard cap (32 limit)
- [ ] Test: Timestamp validation (far future/past clamping)
- [ ] Test: Clock skew separate URL configuration
- [ ] Test: Chunk atomicity (5xx forces entire chunk retry)
- [ ] Test: String metrics not sent to VM
- [ ] Test: Retry-After header parsing
- [ ] Test: SQLite connection pool settings
- [ ] Test: Index coverage on uploader hot path
- [ ] Integration test: 30-minute soak, no duplicates

### Documentation (1-2h)
- [ ] Update README.md with M2 features
- [ ] Create `docs/health-monitoring.md` (status meanings, thresholds)
- [ ] Create `docs/victoriametrics-setup.md` (setup, queries, troubleshooting)
- [ ] Add PromQL query examples to VictoriaMetrics docs
- [ ] Update config.yaml with inline comments for new options
- [ ] Document chunk sizing rationale (128-256 KB target)
- [ ] Document health status semantics (ok/degraded/error)
- [ ] Document per-zone temperature metrics
- [ ] Document clock skew URL configuration
- [ ] Update MILESTONE-2.md acceptance checklist

## Security & Operations

### Security Hardening
- [ ] Create `metrics` user and group (`useradd -r -s /bin/false metrics`)
- [ ] Update systemd/metrics-collector.service with security hardening
- [ ] Set User=metrics, Group=metrics
- [ ] Add NoNewPrivileges=true
- [ ] Add ProtectSystem=strict, ProtectHome=true
- [ ] Add PrivateTmp=true
- [ ] Add resource limits: MemoryMax=200M, CPUQuota=20%
- [ ] Add RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
- [ ] Add RestrictNamespaces=true
- [ ] Add ReadWritePaths=/var/lib/belabox-metrics
- [ ] Add ReadOnlyPaths=/etc/belabox-metrics
- [ ] Set permissions: `chown -R metrics:metrics /var/lib/belabox-metrics`
- [ ] Set token file permissions: `chmod 600 /etc/belabox-metrics/api-token`

### Systemd Integration
- [ ] Add WatchdogSec=60s to systemd unit
- [ ] Implement `startWatchdogPinger()` in Go
- [ ] Use coreos/go-systemd for watchdog notifications
- [ ] Ping at half the watchdog interval (30s)
- [ ] Send WATCHDOG=1 to systemd
- [ ] Test watchdog kills and restarts on hang

### Process Locking
- [ ] Implement `AcquireProcessLock()` using flock
- [ ] Non-blocking lock on database path
- [ ] Write PID to lock file for debugging
- [ ] Release lock on process exit

### Configuration
- [ ] Update configs/config.yaml with all M2 options
- [ ] Add storage.wal_checkpoint_interval (1h)
- [ ] Add storage.wal_checkpoint_size_mb (64)
- [ ] Add remote.batch_size (2500)
- [ ] Add remote.chunk_size (50)
- [ ] Add remote.retry configuration (enabled, max_attempts, backoffs, jitter)
- [ ] Add health.degraded_threshold and health.error_threshold
- [ ] Add monitoring.clock_skew_check_interval (5m)
- [ ] Add monitoring.clock_skew_warn_threshold_ms (2000)
- [ ] Add monitoring.clock_skew_url (separate from ingest URL)
- [ ] Add network.max_interfaces (32)
- [ ] Add disk.allowed_devices pattern

## Final Acceptance Checklist

### Database & Storage
- [x] Schema migration from M1 to M2 succeeds (migrations v4, v5)
- [x] dedup_key field added with unique index
- [x] Value type field added (value_text, value_type)
- [x] Sessions table created
- [x] Same metrics retried → UNIQUE constraint error (no duplicates)
- [x] Dedup keys regenerated with new format (backwards compatible)
- [x] uploaded field added and indexed (already done in v2)
- [x] upload_checkpoints table created with batch tracking (already done in v2)
- [x] MarkUploaded() updates metrics correctly
- [x] GetUnuploadedMetrics() returns only unuploaded
- [ ] Checkpoint tracking persists across restarts
- [x] WAL checkpoint routine implemented (CheckpointWAL, GetWALSize)
- [ ] WAL size stays <64 MB under load (needs background routine)

### Upload Fix
- [ ] No duplicate uploads in 30-minute test
- [ ] Upload loop queries only uploaded=0
- [ ] Metrics marked as uploaded after success
- [ ] Checkpoint advances correctly per chunk
- [ ] Chunking: 2500 metrics → 50-metric chunks
- [ ] Chunks sorted by timestamp ASC
- [ ] Gzip compression applied (BestSpeed)
- [ ] Partial success handled (VM accepts 25/50 → only 25 marked)

### Retry Logic
- [ ] Jittered backoff calculates correctly (±20%)
- [ ] Failed uploads retry with proper delays
- [ ] Max attempts respected (3 attempts)
- [ ] Eventual success after retries
- [ ] Backoff logged with attempt number
- [ ] Retry-After header parsed and respected

### System Metrics
- [ ] CPU usage collecting with delta calculation
- [ ] First sample skipped (no previous to compare)
- [ ] Counter wraparound detected and handled
- [ ] Per-core + overall CPU metrics
- [ ] Memory usage collecting (canonical used calculation)
- [ ] Disk I/O collecting with per-device sector→byte conversion
- [ ] Disk ops/s and bytes/s both exposed
- [ ] Network traffic collecting with interface filtering
- [ ] lo, docker*, veth*, br-*, wlan.*mon, virbr.*, wwan.*, usb.* excluded by default
- [ ] Network counter wraparound detected
- [ ] Network cardinality hard cap (32 interfaces)
- [ ] All thermal zones collecting (SoC, cores, GPU, NPU)
- [ ] Load averages collecting
- [ ] System uptime collecting

### VictoriaMetrics
- [ ] Docker Compose starts VictoriaMetrics
- [ ] Metrics ingested successfully
- [ ] Can query metrics from UI with PromQL
- [ ] JSONL format correct (__name__, labels, values, timestamps)
- [ ] Metric names sanitized (dots→underscores for PromQL)
- [ ] Unit suffixes normalized (_bytes, _celsius, _total, _percent)
- [ ] String metrics filtered out (not sent to VM)
- [ ] Gzip compression works
- [ ] Timestamps preserved correctly (milliseconds)
- [ ] Labels include device_id and tags

### Health & Monitoring
- [ ] Health endpoint responds on :9100
- [ ] /health returns full status JSON
- [ ] /health/live returns liveness (200)
- [ ] /health/ready returns readiness (200 only if ok)
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
- [ ] Separate clock_skew_url used (not ingest URL)

### Logging
- [ ] Structured logging with log/slog
- [ ] JSON format works
- [ ] Console format works for development
- [ ] Log levels configurable (debug, info, warn, error)
- [ ] Collection logs include: collector, count, duration_ms, session_id
- [ ] Upload logs include: batch_id, chunk_index, attempt, backoff_ms, http_status, bytes_sent, bytes_rcvd
- [ ] Retry logs include: attempt, backoff_ms, error
- [ ] No sensitive data in logs

### Security
- [ ] Systemd runs as non-root metrics user
- [ ] NoNewPrivileges=true
- [ ] ProtectSystem=strict, ProtectHome=true
- [ ] MemoryMax=200M, CPUQuota=20%
- [ ] RestrictAddressFamilies, RestrictNamespaces
- [ ] Token file permissions 0600
- [ ] Watchdog integration (60s)
- [ ] Process lock prevents double-run

### Testing
- [ ] All unit tests pass (80+)
- [ ] Integration tests pass (21+)
- [ ] No duplicate uploads verified
- [ ] Partial success verified
- [ ] Retry logic verified
- [ ] Clock skew verified
- [ ] WAL growth verified
- [ ] Counter wraparound verified
- [ ] Resource usage <5% CPU, <150MB RAM
- [ ] 30-minute soak test passes

### Documentation
- [ ] MILESTONE-2.md complete and updated
- [ ] README updated with M2 features
- [ ] Docker setup documented in docker/README.md
- [ ] VictoriaMetrics setup documented
- [ ] Health monitoring documented (status meanings, thresholds)
- [ ] Config examples include all new options
- [ ] Per-zone temperature documented
- [ ] PromQL sanity queries provided
- [ ] Clock skew URL configuration documented
- [ ] Chunk sizing rationale documented

## Success Criteria (Final Verification)

1. ✅ No duplicate uploads in 30-minute soak test (verified with dedup_key)
2. ✅ Same batch retried twice → 0 new rows (unique constraint test)
3. ✅ VictoriaMetrics ingesting metrics successfully
4. ✅ Can query metrics from VictoriaMetrics UI with PromQL
5. ✅ 8+ system metrics collecting reliably
6. ✅ CPU usage with proper delta calculation (no first-sample, wraparound handling)
7. ✅ Network metrics exclude lo, docker*, veth*, br-*
8. ✅ Disk I/O in bytes (not sectors) with per-device sector size
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

**Estimated Timeline:** 3-4 days (26-34 hours)

- Day 1: Database + Upload Fix (7-9h)
- Day 2: Collectors (7-9h)
- Day 3: Health + Monitoring (6-8h)
- Day 4: VictoriaMetrics + Testing (6-8h)
