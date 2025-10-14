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

