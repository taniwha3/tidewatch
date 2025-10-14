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

