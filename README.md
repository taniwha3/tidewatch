# Thugshells Metrics Collector

A lightweight, high-performance metrics collection system designed for Orange Pi devices running Belabox IRL streaming software. Collects system metrics, stores them locally in SQLite, and uploads to a remote endpoint.

## Features

- **Lightweight**: Pure Go implementation, single binary deployment
- **Reliable**: SQLite WAL mode with ARM SBC tuning for embedded devices
- **Flexible**: YAML configuration with per-metric collection intervals
- **Tested**: Comprehensive unit and integration tests (50+ tests, all passing)
- **Cross-platform**: Runs on macOS (development) and Linux ARM64 (production)

## Milestone 1: Complete ✅

Milestone 1 implements a minimal viable pipeline with:

- ✅ 2 metrics: CPU temperature + SRT packet loss (mock)
- ✅ SQLite storage with WAL mode
- ✅ HTTP POST to remote endpoint
- ✅ YAML configuration
- ✅ Cross-compilation for ARM64
- ✅ Systemd service with auto-restart
- ✅ Install script for deployment

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

## Quick Start (macOS Development)

### 1. Build

```bash
./scripts/build.sh
```

### 2. Run Receiver (in terminal 1)

```bash
./bin/metrics-receiver-darwin -port 9090
```

### 3. Run Collector (in terminal 2)

```bash
# Create test config
mkdir -p test-data
cat > test-config.yaml <<EOF
device:
  id: test-device-001

storage:
  path: ./test-data/metrics.db

remote:
  url: http://localhost:9090/api/metrics
  enabled: true
  upload_interval: 10s

metrics:
  - name: cpu.temperature
    interval: 5s
    enabled: true

  - name: srt.packet_loss
    interval: 3s
    enabled: true
EOF

# Run collector
./bin/metrics-collector-darwin -config test-config.yaml
```

### 4. Query Metrics

```bash
sqlite3 test-data/metrics.db "SELECT * FROM metrics ORDER BY timestamp_ms DESC LIMIT 10"
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

### Milestone 1

1. **cpu.temperature** (real on Linux, mock on macOS)
   - Reads from `/sys/class/thermal/thermal_zone0/temp`
   - Interval: 30s
   - Unit: degrees Celsius

2. **srt.packet_loss_pct** (mock in M1, real in M2)
   - Mock: random 0-5% packet loss
   - Interval: 5s
   - Unit: percentage (0-100)

### Milestone 2+ (Planned)

- Real SRT stats from server-side SRTLA receiver
- Encoder metrics (from journald logs)
- HDMI input metrics (via v4l2-ctl)
- Network metrics (bandwidth, latency)
- System metrics (CPU usage, memory, disk)

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

## Future Milestones

See `PRD.md` and `MILESTONE-1.md` for detailed roadmap.

**Milestone 2**: Server-side SRT stats, encoder metrics, retry logic
**Milestone 3**: VictoriaMetrics integration, Grafana dashboards
**Milestone 4**: Priority queue, backfill, data retention

## License

[Add license information]

## Authors

- Tani Aura <111664369+taniwha3@users.noreply.github.com>

## Contributing

[Add contributing guidelines]
