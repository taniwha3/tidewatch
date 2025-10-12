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

