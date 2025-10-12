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

