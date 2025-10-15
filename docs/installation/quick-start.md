# Quick Start Installation Guide

Get Tidewatch up and running in under 5 minutes.

## Prerequisites

- Linux system (x86_64, ARM-based devices like Orange Pi, Raspberry Pi, etc.)
- Debian Bookworm or Ubuntu 22.04+
- Root or sudo access
- Internet connectivity

## Architecture Detection

Determine your system architecture:

```bash
dpkg --print-architecture
```

You'll get one of:
- `amd64` - 64-bit x86 (Intel/AMD processors)
- `arm64` - 64-bit ARM (most modern ARM devices)
- `armhf` - 32-bit ARM (older ARM devices)

## Installation

### 1. Download the Package

Replace `VERSION` with the latest release (e.g., `3.0.0-1`):

```bash
# For amd64 (Intel/AMD)
wget https://github.com/taniwha3/tidewatch/releases/download/vVERSION/tidewatch_VERSION_amd64.deb

# For arm64
wget https://github.com/taniwha3/tidewatch/releases/download/vVERSION/tidewatch_VERSION_arm64.deb

# For armhf
wget https://github.com/taniwha3/tidewatch/releases/download/vVERSION/tidewatch_VERSION_armhf.deb
```

### 2. Install

```bash
sudo apt install ./tidewatch_*.deb
```

That's it! Tidewatch is now installed and running.

## Verification

Check the service status:

```bash
sudo systemctl status tidewatch
```

You should see:
```
● tidewatch.service - Tidewatch Metrics Collector
     Loaded: loaded (/usr/lib/systemd/system/tidewatch.service; enabled; preset: enabled)
     Active: active (running) since ...
```

View logs:

```bash
sudo journalctl -u tidewatch -f
```

Check the version:

```bash
tidewatch -version
```

## Configuration

The default configuration is located at `/etc/tidewatch/config.yaml`.

### Connect to VictoriaMetrics

Edit the config file:

```bash
sudo nano /etc/tidewatch/config.yaml
```

Update the remote URL:

```yaml
remote:
  url: http://your-victoriametrics-server:8428/api/v1/import
  enabled: true
  upload_interval: 30s
```

Restart the service:

```bash
sudo systemctl restart tidewatch
```

## Next Steps

- **View Metrics**: Query VictoriaMetrics at `http://your-vm:8428/vmui`
- **Customize Collection**: Edit collectors in `/etc/tidewatch/config.yaml`
- **Troubleshooting**: See [troubleshooting.md](./troubleshooting.md)
- **Advanced Config**: See [detailed-install.md](./detailed-install.md)

## Quick Test

Query VictoriaMetrics for your device metrics:

```bash
# Replace YOUR_DEVICE_ID with your actual device ID from config
curl -G 'http://your-vm:8428/api/v1/query' \
  --data-urlencode 'query=cpu_usage_percent{device_id="YOUR_DEVICE_ID"}'
```

## Uninstallation

Remove package (keeps config and data):

```bash
sudo apt remove tidewatch
```

Complete removal (deletes everything):

```bash
sudo apt purge tidewatch
```

## Common Commands

```bash
# Start service
sudo systemctl start tidewatch

# Stop service
sudo systemctl stop tidewatch

# Restart service
sudo systemctl restart tidewatch

# View logs
sudo journalctl -u tidewatch -n 100

# Follow logs in real-time
sudo journalctl -u tidewatch -f

# Check service status
sudo systemctl status tidewatch
```

## Getting Help

- **Documentation**: See other guides in `docs/installation/`
- **Issues**: https://github.com/taniwha3/tidewatch/issues
- **Logs**: Check `/var/log/syslog` and `journalctl -u tidewatch`
