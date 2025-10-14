# Health Monitoring

The metrics collector provides comprehensive health monitoring through HTTP endpoints that report the operational status of all system components.

## Health Endpoints

### `/health` - Full Health Status

Returns a complete health report in JSON format with detailed component statuses.

```bash
curl http://localhost:9100/health
```

**Response Format:**
```json
{
  "status": "ok",
  "timestamp": "2025-10-12T20:15:30Z",
  "uptime_seconds": 3600.5,
  "components": {
    "collector.cpu.usage": {
      "status": "ok",
      "message": "collecting metrics",
      "timestamp": "2025-10-12T20:15:29Z",
      "details": {
        "metrics_collected": 8
      }
    },
    "uploader": {
      "status": "ok",
      "message": "uploading metrics",
      "timestamp": "2025-10-12T20:15:30Z",
      "details": {
        "last_upload_time": "2025-10-12T20:15:25Z",
        "pending_count": 150,
        "time_since_upload_seconds": 5
      }
    },
    "storage": {
      "status": "ok",
      "message": "storage operational",
      "timestamp": "2025-10-12T20:15:30Z",
      "details": {
        "database_size_bytes": 10485760,
        "wal_size_bytes": 524288,
        "pending_count": 150
      }
    },
    "time": {
      "status": "ok",
      "message": "time synchronized",
      "timestamp": "2025-10-12T20:15:30Z",
      "details": {
        "skew_ms": 125
      }
    }
  }
}
```

### `/health/live` - Liveness Probe

Simple liveness check - returns 200 if the process is running.

```bash
curl http://localhost:9100/health/live
```

**Response:**
```json
{
  "status": "alive"
}
```

**Use case:** Kubernetes liveness probes, process monitoring

### `/health/ready` - Readiness Probe

Readiness check - returns 200 only if status is "ok", 503 otherwise.

```bash
curl http://localhost:9100/health/ready
```

**Success Response (200):**
```json
{
  "status": "ready"
}
```

**Not Ready Response (503):**
```json
{
  "status": "not_ready",
  "message": "system is not in OK state",
  "current_status": "degraded"
}
```

**Use case:** Kubernetes readiness probes, load balancer health checks

## Health Status Levels

The system reports one of three health statuses:

### `ok` - Healthy Operation
- All collectors functioning normally
- Uploads succeeding within 2× configured interval
- Pending metrics < 5,000
- No component errors

**HTTP Status Code:** 200

### `degraded` - Partial Degradation
Triggered by any of:
- At least 1 collector failing (but not all)
- No successful upload between 2×-10× interval threshold
- Pending metrics between 5,000-10,000
- Clock skew exceeds ±2 seconds
- WAL size exceeds 64 MB

**HTTP Status Code:** 200 (still operational)

### `error` - Critical Failure
Triggered by any of:
- All collectors failing
- Uploader error state
- Storage error state
- No upload in >10 minutes AND pending count >10,000

**HTTP Status Code:** 503

## Health Status Calculation

### Dynamic Thresholds

Health thresholds are automatically derived from the configured upload interval to ensure accurate status reporting:

| Upload Interval | OK Threshold | Degraded Threshold | Error Threshold |
|----------------|--------------|-------------------|-----------------|
| 30s            | 60s (2×)     | 300s (10×)        | 600s (10min)    |
| 1m             | 2m (2×)      | 10m (10×)         | 10m             |
| 5m             | 10m (2×)     | 50m (10×)         | 10m             |

**Note:** The error threshold is always 10 minutes regardless of upload interval, per Milestone 2 specification.

### Component-Specific Rules

#### Collectors
- **OK**: No errors, metrics being collected
- **Degraded**: Individual collector errors (system remains operational with other collectors)
- **Error**: All collectors failing simultaneously

#### Uploader
- **OK**: Successful uploads within 2× interval, pending < 5,000
- **Degraded**:
  - No upload for 2×-10× interval OR
  - Pending metrics 5,000-10,000
- **Error**:
  - Upload failures OR
  - No upload > 10min AND pending >10,000

#### Storage
- **OK**: Database operational, WAL < 64 MB
- **Degraded**: WAL size exceeds 64 MB (checkpoint needed)
- **Error**: Database I/O failures

#### Time Synchronization
- **OK**: Clock skew < 2 seconds
- **Degraded**: Clock skew ≥ 2 seconds (metrics may have incorrect timestamps)
- **Error**: Unable to check clock skew

## Monitoring Integration

### Prometheus/VictoriaMetrics

Health status is also exposed as metrics:

```promql
# Overall system health (0=ok, 1=degraded, 2=error)
system_health_status{device_id="belabox-001"}

# Component-specific health
collector_health_status{device_id="belabox-001", collector="cpu.usage"}
uploader_health_status{device_id="belabox-001"}
storage_health_status{device_id="belabox-001"}
time_health_status{device_id="belabox-001"}
```

### Kubernetes Health Checks

**Liveness Probe:** Detects if process is hung/crashed
```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 9100
  initialDelaySeconds: 10
  periodSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3
```

**Readiness Probe:** Detects if service can accept traffic
```yaml
readinessProbe:
  httpGet:
    path: /health/ready
    port: 9100
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 2
```

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: metrics_collector_health
    rules:
      - alert: MetricsCollectorDegraded
        expr: system_health_status > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Metrics collector {{ $labels.device_id }} is degraded"

      - alert: MetricsCollectorDown
        expr: system_health_status >= 2
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Metrics collector {{ $labels.device_id }} is in error state"

      - alert: MetricsCollectorUploadStalled
        expr: time() - uploader_last_success_timestamp > 600
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Metrics collector {{ $labels.device_id }} has not uploaded for 10+ minutes"
```

## Configuration

Health monitoring is configured in `config.yaml`:

```yaml
monitoring:
  health_address: ":9100"                      # HTTP server bind address
  clock_skew_url: http://localhost:8428/health # URL for clock skew checks
  clock_skew_check_interval: 5m                # How often to check (default: 5m)
  clock_skew_warn_threshold_ms: 2000           # Warn threshold in ms (default: 2000)
```

## Troubleshooting

### Health Status is "degraded"

1. **Check component details:**
   ```bash
   curl -s http://localhost:9100/health | jq '.components'
   ```

2. **Common causes:**
   - Individual collector errors (check logs for specific collector)
   - High pending count (check upload connectivity)
   - Clock skew (verify NTP/time synchronization)
   - Large WAL file (checkpoint will trigger automatically)

3. **Resolution:**
   - Collector errors: Check /proc filesystem permissions (CPU, memory, disk, network)
   - Upload issues: Verify VictoriaMetrics connectivity and credentials
   - Clock skew: Configure NTP/chronyd on the device
   - WAL size: Wait for automatic checkpoint or check disk space

### Health Status is "error"

1. **Check critical components:**
   ```bash
   curl -s http://localhost:9100/health | jq '.components | to_entries | .[] | select(.value.status == "error")'
   ```

2. **Common causes:**
   - All collectors failing (permissions, /proc not mounted)
   - Upload completely failed (network down, VM unreachable)
   - Storage I/O errors (disk full, permissions)
   - Combined: upload stalled >10min with high pending count

3. **Immediate actions:**
   - Check logs: `journalctl -u tidewatch -n 100`
   - Verify disk space: `df -h /var/lib/tidewatch`
   - Test network: `curl http://localhost:8428/health`
   - Check permissions: `ls -la /var/lib/tidewatch`

### Clock Skew Warnings

Clock skew detection runs every 5 minutes and warns if local time differs from server time by >2 seconds.

**Impact:**
- Metrics may have incorrect timestamps
- Time-series queries may be inaccurate
- Alerting may trigger incorrectly

**Resolution:**
```bash
# Check current time sync status
timedatectl status

# Enable NTP (systemd-timesyncd)
sudo timedatectl set-ntp true

# Or install chrony for more accurate sync
sudo apt install chrony
sudo systemctl enable --now chronyd

# Verify sync after 1-2 minutes
timedatectl status
```

## Best Practices

1. **Monitor the `/health` endpoint** - Don't rely solely on liveness/readiness
2. **Set up alerts for "degraded" state** - Catch issues before they become critical
3. **Review component details** - Understand which specific components are unhealthy
4. **Correlate with logs** - Health status + logs provide complete picture
5. **Use dynamic thresholds** - Let the system calculate thresholds based on upload interval
6. **Monitor pending count trends** - Growing pending count indicates upload issues
7. **Check clock skew regularly** - Time synchronization is critical for time-series data
