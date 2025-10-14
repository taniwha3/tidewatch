# VictoriaMetrics Setup Guide

This guide covers setting up VictoriaMetrics to receive and store metrics from the tidewatch metrics collector.

## Quick Start with Docker Compose

The easiest way to get started is using the provided Docker Compose configuration:

```bash
# Start VictoriaMetrics
docker compose up -d victoria

# Check status
docker compose ps

# View logs
docker compose logs -f victoria

# Stop VictoriaMetrics
docker compose down
```

**Endpoints after startup:**
- Metrics import API: `http://localhost:8428/api/v1/import`
- Query API: `http://localhost:8428/api/v1/query`
- Web UI: `http://localhost:8428/vmui`
- Health check: `http://localhost:8428/health`

## Docker Compose Configuration

The provided `docker-compose.yml` includes:

### VictoriaMetrics Single-Node

```yaml
services:
  victoria:
    image: victoriametrics/victoria-metrics:v1.97.1
    ports:
      - "8428:8428"
    volumes:
      - victoria-data:/victoria-metrics-data
    command:
      - '--storageDataPath=/victoria-metrics-data'
      - '--httpListenAddr=:8428'
      - '--retentionPeriod=30d'
      - '--maxInsertRequestSize=32MB'
    restart: unless-stopped
```

**Key parameters:**
- `retentionPeriod`: 30 days (configurable)
- `maxInsertRequestSize`: 32MB (allows bulk uploads)
- Health check: Built-in via `/health` endpoint

### Optional: Grafana for Visualization

```bash
# Start both VictoriaMetrics and Grafana
docker compose up -d

# Access Grafana at http://localhost:3000
# Default credentials: admin/admin
```

Grafana is pre-configured to use VictoriaMetrics as a data source.

## Metrics Collector Configuration

Configure the metrics collector to send data to VictoriaMetrics:

```yaml
# configs/config.yaml
device:
  id: belabox-001

remote:
  url: http://localhost:8428/api/v1/import  # VictoriaMetrics import endpoint
  enabled: true
  upload_interval: 30s
  batch_size: 2500
  chunk_size: 50

monitoring:
  clock_skew_url: http://localhost:8428/health  # For time synchronization checks
  health_address: ":9100"
```

## Querying Metrics

### Using the Web UI (VMUI)

1. Open `http://localhost:8428/vmui` in your browser
2. Enter a PromQL query (e.g., `cpu_usage_percent`)
3. Click "Execute Query" or press Enter
4. View results in graph or table format

### Using the Query API

```bash
# Query current CPU usage
curl -s 'http://localhost:8428/api/v1/query?query=cpu_usage_percent' | jq .

# Query with time range
curl -s 'http://localhost:8428/api/v1/query_range?query=cpu_usage_percent&start=2025-10-12T00:00:00Z&end=2025-10-12T23:59:59Z&step=1m' | jq .

# List all metrics
curl -s 'http://localhost:8428/api/v1/label/__name__/values' | jq .
```

### Using PromQL

VictoriaMetrics supports PromQL (Prometheus Query Language). Common queries:

```promql
# Current CPU usage by device
cpu_usage_percent{device_id="belabox-001"}

# Memory used percentage
(memory_used_bytes / memory_total_bytes) * 100

# Network traffic rate (bytes per second)
rate(network_rx_bytes_total[5m])

# Disk I/O operations per second
rate(disk_read_ops_total[1m])

# Average CPU usage over 5 minutes
avg_over_time(cpu_usage_percent[5m])

# Peak memory usage in the last hour
max_over_time(memory_used_bytes[1h])
```

## Available Metrics

### System Metrics

| Metric Name | Type | Description | Unit |
|------------|------|-------------|------|
| `cpu_usage_percent` | Gauge | Overall CPU usage | Percent |
| `cpu_core_usage_percent` | Gauge | Per-core CPU usage | Percent |
| `cpu_temperature_celsius` | Gauge | CPU temperature | Celsius |
| `memory_used_bytes` | Gauge | Memory in use | Bytes |
| `memory_available_bytes` | Gauge | Memory available | Bytes |
| `memory_total_bytes` | Gauge | Total memory | Bytes |
| `memory_swap_used_bytes` | Gauge | Swap in use | Bytes |
| `memory_swap_total_bytes` | Gauge | Total swap | Bytes |
| `disk_read_ops_total` | Counter | Disk read operations | Count |
| `disk_write_ops_total` | Counter | Disk write operations | Count |
| `disk_read_bytes_total` | Counter | Bytes read from disk | Bytes |
| `disk_write_bytes_total` | Counter | Bytes written to disk | Bytes |
| `disk_read_time_ms_total` | Counter | Time spent reading | Milliseconds |
| `disk_write_time_ms_total` | Counter | Time spent writing | Milliseconds |
| `disk_io_time_weighted_ms_total` | Counter | Weighted I/O time | Milliseconds |
| `network_rx_bytes_total` | Counter | Network bytes received | Bytes |
| `network_tx_bytes_total` | Counter | Network bytes transmitted | Bytes |
| `network_rx_packets_total` | Counter | Packets received | Count |
| `network_tx_packets_total` | Counter | Packets transmitted | Count |
| `network_rx_errors_total` | Counter | Receive errors | Count |
| `network_tx_errors_total` | Counter | Transmit errors | Count |

### Meta-Metrics (Observability)

| Metric Name | Type | Description |
|------------|------|-------------|
| `collector_metrics_collected_total` | Counter | Total metrics collected |
| `collector_metrics_failed_total` | Counter | Collection failures |
| `collector_collection_duration_seconds` | Histogram | Collection time (p50, p95, p99) |
| `uploader_metrics_uploaded_total` | Counter | Total metrics uploaded |
| `uploader_upload_failures_total` | Counter | Upload failures |
| `uploader_upload_duration_seconds` | Histogram | Upload time (p50, p95, p99) |
| `storage_database_size_bytes` | Gauge | SQLite database size |
| `storage_wal_size_bytes` | Gauge | WAL file size |
| `storage_metrics_pending_upload` | Gauge | Metrics pending upload |
| `time_skew_ms` | Gauge | Clock skew relative to server |

### Label Dimensions

Most metrics include these labels:
- `device_id`: Device identifier (e.g., "belabox-001")
- `core`: CPU core number (for per-core metrics)
- `device`: Disk device name (for disk metrics)
- `interface`: Network interface name (for network metrics)

## Example PromQL Queries

### Performance Monitoring

```promql
# Average CPU usage across all cores
avg(cpu_core_usage_percent{device_id="belabox-001"})

# Memory usage percentage
100 * (memory_used_bytes / memory_total_bytes)

# Network traffic rate (MB/s)
rate(network_rx_bytes_total{interface="eth0"}[5m]) / 1024 / 1024

# Disk I/O throughput (MB/s)
rate(disk_read_bytes_total{device="sda"}[5m]) / 1024 / 1024
```

### Troubleshooting

```promql
# Metrics collection success rate
rate(collector_metrics_collected_total[5m]) /
  (rate(collector_metrics_collected_total[5m]) + rate(collector_metrics_failed_total[5m]))

# Upload lag (how far behind are uploads?)
storage_metrics_pending_upload

# Average upload duration (seconds)
histogram_quantile(0.5, rate(uploader_upload_duration_seconds_bucket[5m]))

# Clock skew detection
abs(time_skew_ms) > 2000
```

### Resource Usage

```promql
# Top 3 busiest CPU cores
topk(3, cpu_core_usage_percent{device_id="belabox-001"})

# Network interfaces with errors
network_rx_errors_total > 0 or network_tx_errors_total > 0

# Disk I/O wait time percentage
rate(disk_io_time_weighted_ms_total[5m]) / 10  # Convert to percentage
```

## Data Retention and Storage

### Configure Retention Period

Edit `docker-compose.yml` to change retention:

```yaml
command:
  - '--retentionPeriod=90d'  # Keep data for 90 days
```

Or pass as environment variable:

```bash
docker run -d \
  -p 8428:8428 \
  -v victoria-data:/victoria-metrics-data \
  victoriametrics/victoria-metrics:v1.97.1 \
  --storageDataPath=/victoria-metrics-data \
  --retentionPeriod=90d
```

### Storage Size Estimation

Rough estimates for storage size:

| Scenario | Metrics/sec | Data Points/day | Storage/day | Storage/30d |
|----------|------------|-----------------|-------------|-------------|
| 1 device, 30s interval | 10 | 28,800 | ~100 KB | ~3 MB |
| 10 devices, 30s interval | 100 | 288,000 | ~1 MB | ~30 MB |
| 100 devices, 10s interval | 1,000 | 8,640,000 | ~30 MB | ~900 MB |

**Note:** VictoriaMetrics uses aggressive compression. Actual storage is typically 50-70% less.

### Checking Storage Usage

```bash
# Check disk usage
docker compose exec victoria du -sh /victoria-metrics-data

# Check metrics count
curl -s 'http://localhost:8428/api/v1/status/tsdb' | jq .
```

## Backup and Restore

### Backup

VictoriaMetrics supports instant snapshots:

```bash
# Create snapshot
curl http://localhost:8428/snapshot/create

# Response: {"status":"ok","snapshot":"20251012-150530-1A2B3C4D"}

# Snapshot location: /victoria-metrics-data/snapshots/20251012-150530-1A2B3C4D
```

Copy snapshot to backup location:

```bash
docker compose exec victoria tar czf /tmp/backup.tar.gz /victoria-metrics-data/snapshots/20251012-150530-1A2B3C4D
docker cp tidewatch-victoria:/tmp/backup.tar.gz ./backup-$(date +%Y%m%d).tar.gz
```

### Restore

```bash
# Stop VictoriaMetrics
docker compose down victoria

# Extract backup to data directory
tar xzf backup-20251012.tar.gz -C /path/to/victoria-data

# Start VictoriaMetrics
docker compose up -d victoria
```

## Production Deployment

### Resource Requirements

Minimum recommended resources:
- **CPU:** 1 core (2+ for >100 devices)
- **RAM:** 512 MB (2GB+ for >1000 devices)
- **Disk:** 10 GB (for 30-day retention, 100 devices)
- **Network:** 1 Mbps (scales with device count)

### Security Hardening

1. **Enable authentication:**

```yaml
environment:
  - VMAUTH_CONFIG=/etc/vmauth.yml
```

2. **Use HTTPS:**

```yaml
command:
  - '--tls'
  - '--tlsCertFile=/certs/server.crt'
  - '--tlsKeyFile=/certs/server.key'
```

3. **Restrict network access:**

```yaml
ports:
  - "127.0.0.1:8428:8428"  # Only localhost
```

4. **Run as non-root:**

```yaml
user: "1000:1000"
```

### High Availability

For production, consider VictoriaMetrics cluster:
- Multiple storage nodes for redundancy
- Separate insert and select nodes
- Load balancer for distribution

See: https://docs.victoriametrics.com/Cluster-VictoriaMetrics.html

## Troubleshooting

### No Metrics Appearing

1. **Check VictoriaMetrics logs:**
   ```bash
   docker compose logs victoria | tail -50
   ```

2. **Verify connectivity:**
   ```bash
   curl http://localhost:8428/health
   ```

3. **Check metrics collector logs:**
   ```bash
   journalctl -u tidewatch -n 100
   ```

4. **Test manual insert:**
   ```bash
   echo '{"metric":{"__name__":"test_metric"},"values":[42],"timestamps":['"$(date +%s)000"']}' | \
     curl -X POST http://localhost:8428/api/v1/import -d @-

   # Query test metric
   curl 'http://localhost:8428/api/v1/query?query=test_metric'
   ```

### High Memory Usage

VictoriaMetrics caches aggressively. If memory is an issue:

```yaml
command:
  - '--memory.allowedPercent=50'  # Limit cache to 50% of available RAM
```

### Slow Queries

1. **Check cardinality:**
   ```bash
   curl -s 'http://localhost:8428/api/v1/status/tsdb' | jq '.data.totalSeries'
   ```

2. **Reduce retention:**
   ```yaml
   - '--retentionPeriod=7d'  # Shorter retention = less data to query
   ```

3. **Add more RAM** - Caching improves query performance

### Disk Space Issues

1. **Check current usage:**
   ```bash
   docker compose exec victoria df -h /victoria-metrics-data
   ```

2. **Reduce retention:**
   ```yaml
   - '--retentionPeriod=7d'
   ```

3. **Force compaction:**
   ```bash
   docker compose restart victoria
   ```

## Migration from Prometheus

VictoriaMetrics is Prometheus-compatible. To migrate:

1. **Remote write from Prometheus:**

```yaml
# prometheus.yml
remote_write:
  - url: http://localhost:8428/api/v1/write
```

2. **Import existing Prometheus data:**

```bash
vmctl prometheus --prom-snapshot /path/to/prometheus/data \
  --vm-addr=http://localhost:8428
```

3. **Update queries** - Most PromQL queries work as-is

## References

- VictoriaMetrics Documentation: https://docs.victoriametrics.com/
- PromQL Guide: https://prometheus.io/docs/prometheus/latest/querying/basics/
- VictoriaMetrics vs Prometheus: https://docs.victoriametrics.com/Articles.html#victoriametrics-vs-prometheus
- MetricsQL Extensions: https://docs.victoriametrics.com/MetricsQL.html
