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

