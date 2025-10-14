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
- Confirm token file (`/etc/tidewatch/token`) is readable under `ReadOnlyPaths`
- Token file must be `chmod 0600`, owned by `metrics` user

**Pre-Deployment Checklist:**
```bash
# Create metrics user
sudo useradd -r -s /bin/false metrics

# Set token permissions
sudo install -m 0600 -o metrics -g metrics token /etc/tidewatch/token

# Test service can start
sudo systemctl start tidewatch
sudo systemctl status tidewatch

# Verify no permission errors
sudo journalctl -u tidewatch | grep -i "permission denied"
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
lockFile, err := acquireLock("/var/run/tidewatch.lock")
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

