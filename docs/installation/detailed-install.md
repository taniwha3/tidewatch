# Detailed Installation Guide

Comprehensive installation guide for Tidewatch on ARM devices.

## Table of Contents

- [System Requirements](#system-requirements)
- [Architecture Selection](#architecture-selection)
- [Pre-Installation](#pre-installation)
- [Installation Methods](#installation-methods)
- [Configuration](#configuration)
- [VictoriaMetrics Setup](#victoriametrics-setup)
- [Security Considerations](#security-considerations)
- [Post-Installation](#post-installation)

## System Requirements

### Minimum Requirements

- **CPU**: ARM Cortex-A7 or newer (1 core minimum)
- **RAM**: 64MB available memory
- **Storage**: 50MB for installation + variable for metrics storage
- **OS**: Debian Bookworm (12) or Ubuntu 22.04+

### Recommended Requirements

- **CPU**: ARM Cortex-A53 or newer (2+ cores)
- **RAM**: 128MB available memory
- **Storage**: 500MB+ for long-term metrics storage
- **OS**: Latest stable Debian or Ubuntu

### Tested Platforms

**x86_64 Systems:**
- Standard PC/Server hardware (Intel/AMD processors)
- Virtual machines (VMware, VirtualBox, KVM)
- Cloud instances (AWS EC2, Google Compute, Azure)

**ARM Systems:**
- Orange Pi Zero 2W (H618, arm64)
- Orange Pi 3 LTS (H6, arm64)
- Raspberry Pi 4 (BCM2711, arm64)
- Raspberry Pi 3 (BCM2837, armhf)
- Raspberry Pi Zero 2 W (BCM2710A1, armhf)

## Architecture Selection

### Determine Your Architecture

```bash
dpkg --print-architecture
```

### Understanding Architectures

**amd64** (x86_64, 64-bit Intel/AMD)
- Standard PC and server hardware
- Intel and AMD processors
- Virtual machines and cloud instances
- Most common architecture

**arm64** (aarch64, 64-bit ARM)
- Modern ARM devices (2015+)
- Better performance and memory support
- Raspberry Pi 4, Orange Pi, etc.
- Recommended for new ARM deployments

**armhf** (ARM Hard Float, 32-bit ARM)
- Older ARM devices
- ARMv7 with hardware floating point
- Raspberry Pi 3 and older
- Use if your device doesn't support 64-bit

### Kernel Check

Verify your kernel architecture:

```bash
uname -m
```

Expected output:
- `x86_64` → use amd64 package
- `aarch64` → use arm64 package
- `armv7l` → use armhf package

## Pre-Installation

### 1. Update System

```bash
sudo apt update
sudo apt upgrade -y
```

### 2. Install Dependencies

Dependencies are installed automatically, but you can pre-install them:

```bash
sudo apt install -y systemd adduser ca-certificates
```

Optional but recommended:

```bash
sudo apt install -y sqlite3  # For manual database inspection
```

### 3. Check Systemd

Tidewatch requires systemd:

```bash
systemctl --version
```

Should show systemd version 240 or newer.

### 4. Free Up Resources

If running on a memory-constrained device:

```bash
# Check memory
free -h

# Stop unnecessary services
sudo systemctl disable --now bluetooth
sudo systemctl disable --now avahi-daemon
```

## Installation Methods

### Method 1: Direct Installation (Recommended)

Download and install in one step:

```bash
# Detect architecture
ARCH=$(dpkg --print-architecture)

# Set version (replace with latest)
VERSION="3.0.0-1"

# Download
wget https://github.com/taniwha3/tidewatch/releases/download/v${VERSION}/tidewatch_${VERSION}_${ARCH}.deb

# Install
sudo apt install ./tidewatch_${VERSION}_${ARCH}.deb
```

### Method 2: Verify Before Install

Download, verify checksums and GPG signature, then install:

```bash
ARCH=$(dpkg --print-architecture)
VERSION="3.0.0-1"

# Download package and verification files
wget https://github.com/taniwha3/tidewatch/releases/download/v${VERSION}/tidewatch_${VERSION}_${ARCH}.deb
wget https://github.com/taniwha3/tidewatch/releases/download/v${VERSION}/tidewatch_${VERSION}_${ARCH}.deb.sha256
wget https://github.com/taniwha3/tidewatch/releases/download/v${VERSION}/tidewatch_${VERSION}_${ARCH}.deb.asc
wget https://github.com/taniwha3/tidewatch/releases/download/v${VERSION}/tidewatch-signing-key.asc

# Import GPG key
gpg --import tidewatch-signing-key.asc

# Verify signature
gpg --verify tidewatch_${VERSION}_${ARCH}.deb.asc tidewatch_${VERSION}_${ARCH}.deb

# Verify checksum
sha256sum -c tidewatch_${VERSION}_${ARCH}.deb.sha256

# Install
sudo apt install ./tidewatch_${VERSION}_${ARCH}.deb
```

### Method 3: From GitHub Releases Web UI

1. Visit https://github.com/taniwha3/tidewatch/releases
2. Download the `.deb` file for your architecture
3. Transfer to your device (scp, USB, etc.)
4. Install: `sudo apt install ./tidewatch_*.deb`

## Configuration

### Configuration File

Location: `/etc/tidewatch/config.yaml`

Permissions: `640` (owner: root, group: tidewatch)

### Minimal Configuration

```yaml
device:
  id: my-orangepi-01

remote:
  url: http://192.168.1.100:8428/api/v1/import
  enabled: true
  upload_interval: 30s

storage:
  path: /var/lib/tidewatch/metrics.db

logging:
  level: info
  format: console
```

### Production Configuration

```yaml
device:
  id: prod-edge-device-01
  location: datacenter-a
  tags:
    environment: production
    rack: r42

remote:
  url: https://victoria.example.com/api/v1/import
  enabled: true
  upload_interval: 30s
  batch_size: 1000
  chunk_size: 100
  timeout: 30s
  retry_attempts: 3
  retry_delay: 5s

storage:
  path: /var/lib/tidewatch/metrics.db
  max_age: 168h  # 7 days
  cleanup_interval: 1h

collectors:
  cpu:
    enabled: true
    interval: 15s
  memory:
    enabled: true
    interval: 15s
  disk:
    enabled: true
    interval: 60s
    paths:
      - /
      - /data
  network:
    enabled: true
    interval: 30s
    interfaces:
      - eth0
      - wlan0
  thermal:
    enabled: true
    interval: 30s
    zones:
      - /sys/class/thermal/thermal_zone0/temp

health:
  enabled: true
  port: 8080

logging:
  level: info
  format: json
```

### Configuration Options Reference

See example configs:
- Development: `/usr/share/doc/tidewatch/examples/config.dev.yaml`
- Production: `/usr/share/doc/tidewatch/examples/config.prod.yaml`

### Applying Configuration Changes

After editing `/etc/tidewatch/config.yaml`:

```bash
# Validate syntax (optional, using python)
python3 -c 'import yaml; yaml.safe_load(open("/etc/tidewatch/config.yaml"))'

# Restart service
sudo systemctl restart tidewatch

# Check if it started successfully
sudo systemctl status tidewatch

# Watch logs for errors
sudo journalctl -u tidewatch -f
```

## VictoriaMetrics Setup

### Option 1: Cloud/Remote VictoriaMetrics

If you have an existing VictoriaMetrics instance:

```yaml
remote:
  url: https://your-vm-instance.com/api/v1/import
  enabled: true
```

### Option 2: Local VictoriaMetrics (Same Device)

Install VictoriaMetrics on the same device:

```bash
# Download VictoriaMetrics (arm64 example)
wget https://github.com/VictoriaMetrics/VictoriaMetrics/releases/download/v1.93.0/victoria-metrics-linux-arm64-v1.93.0.tar.gz
tar xvf victoria-metrics-linux-arm64-v1.93.0.tar.gz

# Create systemd service
sudo tee /etc/systemd/system/victoriametrics.service > /dev/null <<EOF
[Unit]
Description=VictoriaMetrics
After=network.target

[Service]
Type=simple
User=victoria
ExecStart=/usr/local/bin/victoria-metrics-prod \\
  -storageDataPath=/var/lib/victoria-metrics \\
  -httpListenAddr=:8428 \\
  -retentionPeriod=12
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

# Create user and directories
sudo useradd -r -s /bin/false victoria
sudo mkdir -p /var/lib/victoria-metrics
sudo chown victoria:victoria /var/lib/victoria-metrics

# Install binary
sudo mv victoria-metrics-prod /usr/local/bin/

# Start service
sudo systemctl daemon-reload
sudo systemctl enable --now victoriametrics
```

Configure Tidewatch to use local VictoriaMetrics:

```yaml
remote:
  url: http://localhost:8428/api/v1/import
  enabled: true
```

### Option 3: Remote Collector Hub

Use a central collector device on your network:

```yaml
remote:
  url: http://192.168.1.50:8428/api/v1/import
  enabled: true
```

## Security Considerations

### File Permissions

Installed automatically by package:

```
/etc/tidewatch/config.yaml         - 640 root:tidewatch
/var/lib/tidewatch/                - 750 tidewatch:tidewatch
/var/lib/tidewatch/metrics.db      - 640 tidewatch:tidewatch
/var/lib/tidewatch/tidewatch.lock  - 644 tidewatch:tidewatch
```

### Systemd Security Features

The service runs with extensive hardening:

- **User Isolation**: Runs as dedicated `tidewatch` user (no login shell)
- **Filesystem Protection**: Read-only system, isolated /tmp
- **Network Restrictions**: Only AF_INET, AF_INET6, AF_UNIX sockets
- **Capability Dropping**: No new privileges, restricted syscalls
- **Resource Limits**: CPU quota 20%, Memory max 200M

View full security settings:

```bash
systemd-analyze security tidewatch
```

### Network Security

If VictoriaMetrics uses HTTPS:

```yaml
remote:
  url: https://metrics.example.com/api/v1/import
```

Tidewatch validates TLS certificates using system CA bundle.

For self-signed certificates, add to system trust:

```bash
sudo cp your-ca.crt /usr/local/share/ca-certificates/
sudo update-ca-certificates
```

### Firewall Configuration

Tidewatch only needs outbound HTTPS (if using remote metrics):

```bash
# Allow outbound to VictoriaMetrics
sudo ufw allow out to 192.168.1.100 port 8428 proto tcp
```

If using health endpoint:

```bash
# Allow health checks from monitoring system
sudo ufw allow from 192.168.1.0/24 to any port 8080 proto tcp
```

## Post-Installation

### 1. Verify Installation

```bash
# Check service status
sudo systemctl status tidewatch

# View recent logs
sudo journalctl -u tidewatch -n 50

# Check database
sudo sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) FROM metrics;"

# Test health endpoint (if enabled)
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

### 2. Monitor Resource Usage

```bash
# Check memory usage
sudo systemctl status tidewatch | grep Memory

# Check CPU usage
top -bn1 | grep tidewatch

# Check disk usage
du -h /var/lib/tidewatch/
```

### 3. Set Up Log Rotation

Journald handles log rotation by default. To customize:

```bash
sudo mkdir -p /etc/systemd/journald.conf.d/
sudo tee /etc/systemd/journald.conf.d/tidewatch.conf > /dev/null <<EOF
[Journal]
SystemMaxUse=100M
SystemMaxFileSize=10M
EOF

sudo systemctl restart systemd-journald
```

### 4. Enable Watchdog Monitoring

The systemd watchdog is enabled by default (60s timeout).

To verify it's working:

```bash
# Check watchdog status
sudo systemctl show tidewatch | grep Watchdog

# Test watchdog by sending SIGSTOP (service will restart automatically)
sudo systemctl kill -s STOP tidewatch

# Wait 60+ seconds, then check status
sudo systemctl status tidewatch
```

### 5. Test Metrics Collection

Query VictoriaMetrics for metrics:

```bash
# Get CPU metrics
curl -G 'http://your-vm:8428/api/v1/query' \
  --data-urlencode 'query=cpu_usage_percent{device_id="your-device-id"}'

# Get memory metrics
curl -G 'http://your-vm:8428/api/v1/query' \
  --data-urlencode 'query=memory_usage_percent{device_id="your-device-id"}'
```

Or use VictoriaMetrics UI:

```
http://your-vm:8428/vmui
```

## Upgrading

### Standard Upgrade

```bash
# Download new version
ARCH=$(dpkg --print-architecture)
VERSION="3.1.0-1"
wget https://github.com/taniwha3/tidewatch/releases/download/v${VERSION}/tidewatch_${VERSION}_${ARCH}.deb

# Install (will preserve config)
sudo apt install ./tidewatch_${VERSION}_${ARCH}.deb

# Verify upgrade
tidewatch -version
sudo systemctl status tidewatch
```

### Configuration During Upgrade

If the config file was modified:

```
Configuration file '/etc/tidewatch/config.yaml'
 ==> Modified (by you or by a script) since installation.
 ==> Package distributor has shipped an updated version.
   What would you like to do about it ?  Your options are:
    Y or I  : install the package maintainer's version
    N or O  : keep your currently-installed version
      D     : show the differences between the versions
      Z     : start a shell to examine the situation
 The default action is to keep your current version.
*** config.yaml (Y/I/N/O/D/Z) [default=N] ?
```

Recommendation: Choose `N` to keep your config, then manually review changes.

### Database Migrations

Database migrations run automatically on service start after upgrade.

Check migration status:

```bash
sudo journalctl -u tidewatch -n 100 | grep -i migration
```

## Uninstallation

### Remove Package (Keep Data)

```bash
sudo apt remove tidewatch
```

This keeps:
- `/etc/tidewatch/config.yaml`
- `/var/lib/tidewatch/metrics.db`
- System user and group

### Complete Removal (Purge)

```bash
sudo apt purge tidewatch
```

This removes everything, including data and configuration.

### Manual Cleanup

If needed:

```bash
sudo rm -rf /var/lib/tidewatch
sudo rm -rf /etc/tidewatch
sudo deluser tidewatch
sudo delgroup tidewatch
```

## Troubleshooting

For common issues, see [troubleshooting.md](./troubleshooting.md).

Quick diagnostics:

```bash
# Check service status
sudo systemctl status tidewatch

# View full logs
sudo journalctl -u tidewatch --no-pager

# Check configuration syntax
sudo -u tidewatch tidewatch -config /etc/tidewatch/config.yaml 2>&1 | head

# Check file permissions
ls -la /etc/tidewatch/
ls -la /var/lib/tidewatch/

# Check network connectivity
curl -v http://your-victoriametrics:8428/
```

## Next Steps

- Configure custom collectors
- Set up Grafana dashboards
- Configure alerts in VictoriaMetrics
- Explore advanced configuration options
- Join the community discussions

## Additional Resources

- GitHub Repository: https://github.com/taniwha3/tidewatch
- Issue Tracker: https://github.com/taniwha3/tidewatch/issues
- VictoriaMetrics Docs: https://docs.victoriametrics.com/
