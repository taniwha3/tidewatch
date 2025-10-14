# Deployment Guide

This guide covers deploying the metrics collector in production with proper security hardening and operational best practices.

## Prerequisites

- Linux system (systemd-based distribution)
- Go 1.21+ (for building from source)
- Sudo/root access for initial setup
- VictoriaMetrics or compatible TSDB endpoint

## Installation Steps

### 1. Build the Binary

```bash
# Clone and build
git clone https://github.com/taniwha3/thugshells.git
cd tidewatch
go build -o tidewatch cmd/collector/main.go

# Or download pre-built binary
# curl -L https://github.com/taniwha3/thugshells/releases/latest/download/tidewatch -o tidewatch
```

### 2. Create Dedicated User and Group

The metrics collector should run as a dedicated non-root user for security:

```bash
# Create system user (no login shell, no home directory)
sudo useradd -r -s /bin/false -U metrics

# Verify user was created
id metrics
# Output: uid=XXX(metrics) gid=XXX(metrics) groups=XXX(metrics)
```

### 3. Install Binary and Configuration

```bash
# Install binary
sudo install -m 755 -o root -g root tidewatch /usr/local/bin/

# Create directories
sudo mkdir -p /etc/tidewatch
sudo mkdir -p /var/lib/tidewatch

# Set ownership
sudo chown -R metrics:metrics /var/lib/tidewatch
sudo chown root:metrics /etc/tidewatch
sudo chmod 750 /etc/tidewatch

# Copy configuration
sudo cp configs/config.yaml /etc/tidewatch/
sudo chown root:metrics /etc/tidewatch/config.yaml
sudo chmod 640 /etc/tidewatch/config.yaml
```

### 4. Create API Token (if using authentication)

```bash
# Generate a secure random token
openssl rand -base64 32 > /tmp/api-token

# Install with restricted permissions
sudo install -m 600 -o root -g metrics /tmp/api-token /etc/tidewatch/api-token
rm /tmp/api-token

# Update config to reference token file
# Edit /etc/tidewatch/config.yaml:
# remote:
#   auth_token_file: /etc/tidewatch/api-token
```

### 5. Install Systemd Service

```bash
# Copy service file
sudo cp systemd/tidewatch.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Enable service to start on boot
sudo systemctl enable tidewatch

# Start service
sudo systemctl start tidewatch

# Check status
sudo systemctl status tidewatch
```

## Configuration

### Main Configuration File

Edit `/etc/tidewatch/config.yaml`:

```yaml
device_id: "device-001"  # Unique identifier for this device

# Data collection intervals
collection_interval: 30s
upload_interval: 1m

# Remote endpoint
remote:
  url: "https://metrics.example.com/api/v1/import"
  auth_token_file: /etc/tidewatch/api-token  # Optional
  timeout: 30s

  # Upload tuning
  batch_size: 2500  # Max metrics per upload batch
  chunk_size: 50    # Metrics per HTTP chunk

  # Retry configuration
  retry:
    enabled: true
    max_attempts: 3
    initial_backoff: 1s
    max_backoff: 30s
    backoff_multiplier: 2.0
    jitter_percent: 20

# Local storage
storage:
  path: /var/lib/tidewatch/metrics.db
  wal_checkpoint_interval: 1h
  wal_checkpoint_size_mb: 64

# Health monitoring
health:
  listen_addr: ":9100"

# Monitoring
monitoring:
  clock_skew_check_interval: 5m
  clock_skew_warn_threshold_ms: 2000
  clock_skew_url: "https://metrics.example.com"  # For time sync check

# Logging
logging:
  level: info     # debug, info, warn, error
  format: json    # json, console

# Network monitoring
network:
  max_interfaces: 32

# Disk monitoring
disk:
  allowed_devices: "^(sd[a-z]|nvme[0-9]+n[0-9]+|vd[a-z])$"
```

### Service Configuration

The systemd service file at `/etc/systemd/system/tidewatch.service` includes:

**Security Hardening:**
- Runs as non-root `metrics:metrics` user
- `NoNewPrivileges=true` - prevents privilege escalation
- `ProtectSystem=strict` - read-only root filesystem
- `ProtectHome=true` - no access to home directories
- `PrivateTmp=true` - isolated /tmp
- Restricted system call access
- Limited device access
- Network address family restrictions

**Resource Limits:**
- `MemoryMax=200M` - maximum memory usage
- `CPUQuota=20%` - maximum CPU usage

**Watchdog:**
- `WatchdogSec=60s` - systemd will restart if process hangs

## Operational Tasks

### Checking Service Status

```bash
# Service status
sudo systemctl status tidewatch

# Live logs
sudo journalctl -u tidewatch -f

# Recent logs with context
sudo journalctl -u tidewatch -n 100 --no-pager

# Logs from specific time
sudo journalctl -u tidewatch --since "1 hour ago"
```

### Health Checks

```bash
# Full health report
curl http://localhost:9100/health | jq

# Liveness probe (always returns 200 if process is running)
curl http://localhost:9100/health/live

# Readiness probe (returns 200 only if system is healthy)
curl http://localhost:9100/health/ready
```

Health statuses:
- **ok**: All systems operational
- **degraded**: One or more issues detected (e.g., collector failure, elevated backlog)
- **error**: Critical issues (e.g., all collectors failing, no uploads for 10+ minutes with high backlog)

### Restarting Service

```bash
# Graceful restart
sudo systemctl restart tidewatch

# Reload configuration without restart (if supported)
sudo systemctl reload tidewatch

# Stop service
sudo systemctl stop tidewatch
```

### Updating the Service

```bash
# 1. Stop service
sudo systemctl stop tidewatch

# 2. Replace binary
sudo install -m 755 -o root -g root tidewatch /usr/local/bin/

# 3. Start service
sudo systemctl start tidewatch

# 4. Verify
sudo systemctl status tidewatch
```

### Database Maintenance

```bash
# Check database size
sudo ls -lh /var/lib/tidewatch/

# Check WAL size
sudo ls -lh /var/lib/tidewatch/*-wal

# The service automatically checkpoints the WAL hourly and when it exceeds 64MB
```

### Process Locking

The service uses file-based locking (flock) to prevent multiple instances:

```bash
# Check if lock exists
ls -l /var/lib/tidewatch/metrics.db.lock

# Read PID from lock file
cat /var/lib/tidewatch/metrics.db.lock
```

**Important Notes:**
- Lock files persist after the process exits (not removed) to prevent race conditions
- The flock is automatically released when the process exits or crashes
- A stale lock file (with PID from dead process) will not prevent a new instance from starting
- If you see an "already running" error, check if the PID in the lock file is actually running:
  ```bash
  PID=$(cat /var/lib/tidewatch/metrics.db.lock | tr -d '\n')
  ps -p $PID || echo "Process not running - lock is stale but safe to ignore"
  ```
- The persistent lock file prevents inode-based race conditions where two processes could
  lock different inodes during overlapping start/stop cycles

## Monitoring & Alerting

### Key Metrics to Monitor

The service exposes health status at `:9100/health`:

```json
{
  "status": "ok",
  "timestamp": "2024-01-15T10:30:00Z",
  "uptime_seconds": 3600,
  "components": {
    "collector.cpu": {
      "status": "ok",
      "message": "collecting metrics",
      "timestamp": "2024-01-15T10:30:00Z",
      "details": {
        "metrics_collected": 8
      }
    },
    "uploader": {
      "status": "ok",
      "message": "uploading metrics",
      "timestamp": "2024-01-15T10:30:00Z",
      "details": {
        "last_upload_time": "2024-01-15T10:29:50Z",
        "pending_count": 120,
        "time_since_upload_seconds": 10
      }
    },
    "storage": {
      "status": "ok",
      "message": "storage operational",
      "timestamp": "2024-01-15T10:30:00Z",
      "details": {
        "database_size_bytes": 1048576,
        "wal_size_bytes": 4096,
        "pending_count": 120
      }
    },
    "time": {
      "status": "ok",
      "message": "time synchronized",
      "timestamp": "2024-01-15T10:30:00Z",
      "details": {
        "skew_ms": 150
      }
    }
  }
}
```

### Recommended Alerts

**Critical:**
- Health status = `error` for > 5 minutes
- Process not running
- Memory usage > 180MB (90% of limit)
- No successful upload in 15 minutes

**Warning:**
- Health status = `degraded` for > 10 minutes
- Pending metrics > 5000
- Clock skew > 2 seconds
- WAL size > 64MB
- Any collector failing

### Integration with Monitoring Systems

**Prometheus:**
```yaml
scrape_configs:
  - job_name: 'tidewatch-health'
    static_configs:
      - targets: ['device-001:9100']
    metrics_path: '/health'
```

**Systemd monitoring:**
```bash
# Monitor via systemd
sudo systemctl show tidewatch -p ActiveState,SubState,Result,MainPID

# Integration with monitoring tools
systemctl is-active tidewatch || alert
```

## Troubleshooting

### Service Won't Start

```bash
# Check logs
sudo journalctl -u tidewatch -n 50

# Common issues:
# - Permissions on /var/lib/tidewatch
# - Invalid configuration
# - Lock file from crashed process
# - Port 9100 already in use
```

### High Memory Usage

```bash
# Check actual memory usage
sudo systemctl status tidewatch | grep Memory

# If approaching limit:
# 1. Check pending metrics count via health endpoint
# 2. Verify uploads are succeeding
# 3. Consider increasing batch_size to reduce backlog faster
```

### Database Issues

```bash
# Check database integrity
sudo -u metrics sqlite3 /var/lib/tidewatch/metrics.db "PRAGMA integrity_check;"

# Vacuum database (reclaim space)
sudo systemctl stop tidewatch
sudo -u metrics sqlite3 /var/lib/tidewatch/metrics.db "VACUUM;"
sudo systemctl start tidewatch
```

### Clock Skew Warnings

```bash
# Check system time
timedatectl status

# Sync with NTP
sudo timedatectl set-ntp true

# Or use chronyd/ntpd
sudo systemctl restart chronyd
```

### Upload Failures

```bash
# Check logs for upload errors
sudo journalctl -u tidewatch | grep -i upload | grep -i error

# Test connectivity to remote endpoint
curl -I https://metrics.example.com/api/v1/import

# Verify auth token
curl -H "Authorization: Bearer $(cat /etc/tidewatch/api-token)" \
  https://metrics.example.com/api/v1/import
```

## Security Considerations

### File Permissions

```bash
# Binary: root-owned, world-executable
/usr/local/bin/tidewatch      755 root:root

# Configuration: root-owned, group-readable
/etc/tidewatch/                 750 root:metrics
/etc/tidewatch/config.yaml      640 root:metrics

# API token: restricted to service account
/etc/tidewatch/api-token        600 root:metrics

# Data directory: service-owned
/var/lib/tidewatch/             750 metrics:metrics
```

### Network Security

- The service only needs outbound HTTPS to the metrics endpoint
- Inbound traffic only on `localhost:9100` for health checks
- Consider firewall rules to restrict health endpoint access
- Use TLS for all remote communications

### Systemd Hardening

The service file includes extensive hardening:
- No new privileges
- Restricted filesystem access
- Limited system call access
- Network restrictions
- Device access controls

To verify security settings:
```bash
sudo systemd-analyze security tidewatch
```

## Backup and Recovery

### Backing Up Configuration

```bash
# Backup configuration
sudo tar -czf tidewatch-config-$(date +%Y%m%d).tar.gz \
  /etc/tidewatch/

# Restore configuration
sudo tar -xzf tidewatch-config-YYYYMMDD.tar.gz -C /
```

### Database Backup

The SQLite database at `/var/lib/tidewatch/metrics.db` contains queued metrics. If backlog is low, loss is acceptable. For backup:

```bash
# Stop service
sudo systemctl stop tidewatch

# Backup
sudo cp /var/lib/tidewatch/metrics.db \
  /backup/metrics-$(date +%Y%m%d).db

# Start service
sudo systemctl start tidewatch
```

### Disaster Recovery

1. Reinstall from packages/source
2. Restore configuration from backup
3. Regenerate or restore API token
4. Start service - it will create a new database and resume collection

## Performance Tuning

### For High-Volume Devices

```yaml
# Increase batch and chunk sizes
remote:
  batch_size: 5000
  chunk_size: 100

# Reduce collection frequency
collection_interval: 60s
upload_interval: 2m
```

### For Resource-Constrained Devices

```yaml
# Reduce batch size
remote:
  batch_size: 1000
  chunk_size: 25

# Increase upload frequency to prevent backlog
upload_interval: 30s

# Reduce network cardinality tracking
network:
  max_interfaces: 16
```

## See Also

- [Health Monitoring](health-monitoring.md) - Detailed health status documentation
- [VictoriaMetrics Setup](victoriametrics-setup.md) - TSDB configuration
- [Configuration Reference](../configs/config.yaml) - Full configuration options
