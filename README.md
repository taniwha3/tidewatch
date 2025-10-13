# Thugshells Metrics Collector

A lightweight, high-performance metrics collection system designed for Orange Pi devices running Belabox IRL streaming software. Collects comprehensive system metrics, stores them locally in SQLite, and uploads to VictoriaMetrics.

## Features

- **Lightweight**: Pure Go implementation, single binary deployment (<10 MB)
- **Reliable**: SQLite WAL mode with ARM SBC tuning, duplicate detection, automatic retry
- **Comprehensive**: 8+ system metrics (CPU, memory, disk, network, temperature)
- **Observable**: Built-in meta-metrics, health monitoring, clock skew detection
- **Production-ready**: Health endpoints, structured logging, security hardening
- **Tested**: Comprehensive unit and integration tests (100+ tests, all passing)
- **Cross-platform**: Runs on macOS (development) and Linux ARM64 (production)

## Milestone 1: Complete ✅

- ✅ 2 metrics: CPU temperature + SRT packet loss (mock)
- ✅ SQLite storage with WAL mode
- ✅ HTTP POST to remote endpoint
- ✅ YAML configuration
- ✅ Cross-compilation for ARM64
- ✅ Systemd service with auto-restart
- ✅ Install script for deployment

## Milestone 2: Complete ✅

- ✅ **System Metrics**: CPU usage, memory, disk I/O, network traffic
- ✅ **VictoriaMetrics Integration**: JSONL import, PromQL queries
- ✅ **Upload Reliability**: Chunked uploads, jittered backoff, duplicate prevention
- ✅ **Health Monitoring**: Graduated status (ok/degraded/error), HTTP endpoints
- ✅ **Meta-Metrics**: Observability of the collector itself
- ✅ **Clock Skew Detection**: Time synchronization monitoring
- ✅ **Structured Logging**: JSON/console formats with contextual fields

## Project Structure

```
thugshells/
├── cmd/
│   ├── metrics-collector/    # Main collector binary
│   └── metrics-receiver/      # Simple HTTP receiver for testing
├── internal/
│   ├── models/                # Metric data structures
│   ├── config/                # YAML configuration
│   ├── collector/             # Metric collectors (system, mock SRT)
│   ├── storage/               # SQLite storage layer
│   └── uploader/              # HTTP uploader
├── configs/                   # Sample configurations
├── scripts/                   # Build and install scripts
├── systemd/                   # Systemd service file
├── docs/                      # Documentation
│   └── belabox-integration.md # Belabox integration notes
├── MILESTONE-1.md             # Milestone 1 specification
└── PRD.md                     # Product requirements document
```

## Quick Start with VictoriaMetrics (Milestone 2)

### 1. Start VictoriaMetrics

```bash
docker compose up -d victoria
```

VictoriaMetrics UI: http://localhost:8428/vmui

### 2. Build and Run Collector

```bash
# Build
./scripts/build.sh

# Run with default config (sends to VictoriaMetrics)
./bin/metrics-collector-darwin -config configs/config.yaml
```

### 3. Query Metrics

```bash
# Using VictoriaMetrics UI: http://localhost:8428/vmui

# Or using curl:
curl -s 'http://localhost:8428/api/v1/query?query=cpu_usage_percent' | jq .

# List all metrics:
curl -s 'http://localhost:8428/api/v1/label/__name__/values' | jq .
```

### 4. Check Health

```bash
curl http://localhost:9100/health | jq .
```

## Quick Start (Milestone 1 - Simple Testing)

### 1. Build

```bash
./scripts/build.sh
```

### 2. Run Simple Receiver (in terminal 1)

```bash
./bin/metrics-receiver-darwin -port 9090
```

### 3. Run Collector (in terminal 2)

```bash
./bin/metrics-collector-darwin -config configs/config.yaml
```

### 4. Query SQLite Directly

```bash
sqlite3 /var/lib/belabox-metrics/metrics.db \
  "SELECT metric_name, metric_value FROM metrics ORDER BY timestamp_ms DESC LIMIT 10"
```

## Deployment to Orange Pi

### 1. Build for ARM64

```bash
./scripts/build.sh
```

### 2. Copy to Orange Pi

```bash
scp -r bin configs scripts systemd user@orangepi:/tmp/thugshells
```

### 3. Install on Orange Pi

```bash
ssh user@orangepi
cd /tmp/thugshells
sudo ./scripts/install.sh bin/metrics-collector-linux-arm64
```

### 4. Verify Installation

```bash
# Check service status
sudo systemctl status metrics-collector

# Watch logs
sudo journalctl -u metrics-collector -f

# Query metrics
sudo sqlite3 /var/lib/belabox-metrics/metrics.db \
  "SELECT datetime(timestamp_ms/1000, 'unixepoch') as time,
          metric_name, metric_value
   FROM metrics
   ORDER BY timestamp_ms DESC
   LIMIT 20"
```

## Configuration

Configuration file: `/etc/belabox-metrics/config.yaml`

```yaml
device:
  id: belabox-001                          # Unique device identifier

storage:
  path: /var/lib/belabox-metrics/metrics.db  # SQLite database path

remote:
  url: http://example.com/api/metrics      # Remote endpoint URL
  enabled: true                            # Enable remote uploads
  upload_interval: 30s                     # Upload interval

metrics:
  - name: cpu.temperature
    interval: 30s                          # Collection interval
    enabled: true

  - name: srt.packet_loss
    interval: 5s
    enabled: true
```

## Testing

### Run All Tests

```bash
go test ./... -v
```

### Run Tests with Coverage

```bash
go test ./... -cover
```

### Test Summary

- **Models**: 4/4 tests passing
- **Config**: 7/7 tests passing
- **Collectors**: 8/8 tests passing
- **Storage**: 19/19 tests passing
- **Uploader**: 16/16 tests passing
- **Total**: 54/54 tests passing ✅

## Metrics Collected

### System Metrics (Milestone 2)

1. **CPU Usage** (`cpu_usage_percent`, `cpu_core_usage_percent`)
   - Overall and per-core CPU usage with delta calculation
   - Wraparound detection, first-sample skip
   - Mock implementation on macOS

2. **Memory** (`memory_used_bytes`, `memory_available_bytes`, `memory_total_bytes`)
   - Canonical used calculation: MemTotal - MemAvailable
   - Swap metrics: `memory_swap_used_bytes`, `memory_swap_total_bytes`
   - Mock implementation on macOS

3. **Disk I/O** (`disk_read_ops_total`, `disk_write_ops_total`, `disk_read_bytes_total`, `disk_write_bytes_total`)
   - Reads/writes in ops/s and bytes/s
   - Time metrics: `disk_read_time_ms_total`, `disk_write_time_ms_total`
   - Per-device metrics with whole-device filtering (no partitions)
   - Sector→byte conversion (512 bytes per sector per kernel docs)

4. **Network** (`network_rx_bytes_total`, `network_tx_bytes_total`, `network_rx_packets_total`, `network_tx_packets_total`)
   - Per-interface traffic counters
   - Error counters: `network_rx_errors_total`, `network_tx_errors_total`
   - Wraparound detection
   - Cardinality guard: max 32 interfaces (prevents explosion)
   - Excludes: lo, docker*, veth*, br-*, wlan.*mon, virbr.*, etc.
   - Mock implementation on macOS

5. **Temperature** (`cpu_temperature_celsius`)
   - Reads from `/sys/class/thermal/thermal_zone*/temp`
   - Per-zone metrics with zone name tags
   - Mock implementation on macOS

### Meta-Metrics (Observability)

6. **Collection Metrics**
   - `collector_metrics_collected_total`: Total metrics collected
   - `collector_metrics_failed_total`: Collection failures
   - `collector_collection_duration_seconds`: Collection time (p50, p95, p99)

7. **Upload Metrics**
   - `uploader_metrics_uploaded_total`: Total metrics uploaded
   - `uploader_upload_failures_total`: Upload failures
   - `uploader_upload_duration_seconds`: Upload time (p50, p95, p99)

8. **Storage Metrics**
   - `storage_database_size_bytes`: SQLite DB size
   - `storage_wal_size_bytes`: WAL file size
   - `storage_metrics_pending_upload`: Pending upload count

9. **Time Synchronization**
   - `time_skew_ms`: Clock skew relative to server (positive = local ahead)
   - Separate URL check every 5 minutes
   - Warns if skew > 2 seconds

### Milestone 3+ (Planned)

- Real SRT stats from server-side SRTLA receiver
- Encoder metrics (from journald logs)
- HDMI input metrics (via v4l2-ctl)
- Load averages, system uptime

## Architecture

```
┌─────────────────────┐
│  Metric Collectors  │  <- cpu.temperature, srt.packet_loss
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│   SQLite Storage    │  <- Local buffer (WAL mode)
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│   HTTP Uploader     │  -> POST to remote endpoint
└─────────────────────┘
```

### Key Design Decisions

1. **Go**: Fast, easy cross-compilation, single binary
2. **SQLite**: Reliable embedded storage with WAL mode
3. **Collectors**: Pluggable interface for different metric sources
4. **Storage-first**: Always store locally, upload asynchronously
5. **No dependencies**: Pure Go implementation (no CGO)

## Performance

- **Binary size**: ~9.5 MB (darwin), ~9.3 MB (linux-arm64)
- **Memory usage**: <50 MB typical
- **CPU usage**: <5% on Orange Pi 5+ (RK3588)
- **Storage**: ~1 KB per metric (with indexes)
- **Throughput**: 1000+ metrics/second store rate

## Troubleshooting

### Collector not starting

```bash
# Check config validity
./bin/metrics-collector-darwin -config test-config.yaml -version

# Check logs
sudo journalctl -u metrics-collector --no-pager -n 50
```

### Metrics not uploading

```bash
# Check receiver is accessible
curl http://localhost:9090/health

# Check collector logs for upload errors
sudo journalctl -u metrics-collector -f | grep upload
```

### Database locked errors

```bash
# Check WAL mode is enabled
sqlite3 /var/lib/belabox-metrics/metrics.db "PRAGMA journal_mode"
# Should return: wal

# Checkpoint WAL if needed
sqlite3 /var/lib/belabox-metrics/metrics.db "PRAGMA wal_checkpoint(TRUNCATE)"
```

## Development

### Prerequisites

- Go 1.24+
- SQLite (for testing)

### Adding a New Collector

1. Implement the `Collector` interface in `internal/collector/`:

```go
type Collector interface {
    Name() string
    Collect(ctx context.Context) ([]*models.Metric, error)
}
```

2. Register in `cmd/metrics-collector/main.go`:

```go
case "your.metric":
    coll = collector.NewYourCollector(cfg.Device.ID)
```

3. Add to config:

```yaml
metrics:
  - name: your.metric
    interval: 30s
    enabled: true
```

## Documentation

- **[Health Monitoring](docs/health-monitoring.md)** - Health endpoints, status levels, alerting
- **[VictoriaMetrics Setup](docs/victoriametrics-setup.md)** - Installation, querying, PromQL examples
- **[Belabox Integration](docs/belabox-integration.md)** - Orange Pi deployment notes
- **[Milestone 1 Spec](MILESTONE-1.md)** - M1 requirements and acceptance criteria
- **[Milestone 2 Spec](MILESTONE-2.md)** - M2 requirements and acceptance criteria (in progress)
- **[Product Requirements](PRD.md)** - Full product roadmap

## Future Milestones

**Milestone 3**: Real SRT stats, encoder metrics, alerting
**Milestone 4**: Priority queue, backfill, data retention

## License

[Add license information]

## Authors

- Tani Aura <111664369+taniwha3@users.noreply.github.com>

## Contributing

[Add contributing guidelines]
