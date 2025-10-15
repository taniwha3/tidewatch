# Troubleshooting Guide

Common issues and solutions for Tidewatch installation and operation.

## Table of Contents

- [Installation Issues](#installation-issues)
- [Service Issues](#service-issues)
- [Metrics Collection Issues](#metrics-collection-issues)
- [Upload Issues](#upload-issues)
- [Database Issues](#database-issues)
- [Performance Issues](#performance-issues)
- [Watchdog Issues](#watchdog-issues)
- [Diagnostic Tools](#diagnostic-tools)

## Installation Issues

### Package Installation Fails

**Symptom**: `apt install` fails with dependency errors

**Solution**:
```bash
# Update package cache
sudo apt update

# Install dependencies manually
sudo apt install -y systemd adduser ca-certificates

# Try installation again
sudo apt install ./tidewatch_*.deb
```

### Architecture Mismatch

**Symptom**: `package architecture (arm64) does not match system (armhf)`

**Solution**:
```bash
# Check your architecture
dpkg --print-architecture

# Download correct package
# Use arm64 for aarch64 systems
# Use armhf for armv7l systems
```

### Permission Denied During Installation

**Symptom**: `Permission denied` when installing

**Solution**:
```bash
# Ensure you're using sudo
sudo apt install ./tidewatch_*.deb

# Check file permissions
ls -l tidewatch_*.deb
# Should be readable (at least -r--r--r--)
```

## Service Issues

### Service Won't Start

**Symptom**: `systemctl start tidewatch` fails immediately

**Diagnosis**:
```bash
# Check detailed status
sudo systemctl status tidewatch

# View recent logs
sudo journalctl -u tidewatch -n 50 --no-pager

# Check for errors
sudo journalctl -u tidewatch -p err --no-pager
```

**Common Causes**:

1. **Configuration Error**
   ```bash
   # Validate YAML syntax
   python3 -c 'import yaml; yaml.safe_load(open("/etc/tidewatch/config.yaml"))'

   # Check for common issues
   sudo journalctl -u tidewatch | grep -i "parse\|config\|yaml"
   ```

2. **Permission Issues**
   ```bash
   # Fix ownership
   sudo chown -R tidewatch:tidewatch /var/lib/tidewatch
   sudo chown root:tidewatch /etc/tidewatch/config.yaml

   # Fix permissions
   sudo chmod 750 /var/lib/tidewatch
   sudo chmod 640 /etc/tidewatch/config.yaml
   ```

3. **Port Already in Use** (if health endpoint enabled)
   ```bash
   # Check what's using the port
   sudo ss -tlnp | grep :8080

   # Change health port in config
   sudo nano /etc/tidewatch/config.yaml
   # health:
   #   port: 8081
   ```

### Service Crashes Immediately After Start

**Symptom**: Service enters failed state seconds after starting

**Diagnosis**:
```bash
# View crash logs
sudo journalctl -u tidewatch -e

# Check for panic/crash
sudo journalctl -u tidewatch | grep -i "panic\|fatal\|crash"

# Run manually to see error
sudo -u tidewatch /usr/bin/tidewatch -config /etc/tidewatch/config.yaml
```

**Common Causes**:

1. **Invalid VictoriaMetrics URL**
   ```yaml
   # Fix URL format in config
   remote:
     url: http://hostname:8428/api/v1/import  # Must include /api/v1/import
   ```

2. **Database Corruption**
   ```bash
   # Check database integrity
   sudo sqlite3 /var/lib/tidewatch/metrics.db "PRAGMA integrity_check;"

   # If corrupted, backup and recreate
   sudo systemctl stop tidewatch
   sudo mv /var/lib/tidewatch/metrics.db /var/lib/tidewatch/metrics.db.bak
   sudo systemctl start tidewatch
   ```

3. **Missing Dependencies**
   ```bash
   # Reinstall with dependencies
   sudo apt --reinstall install ./tidewatch_*.deb
   ```

### Service Won't Stop

**Symptom**: `systemctl stop tidewatch` hangs or times out

**Solution**:
```bash
# Force stop
sudo systemctl kill tidewatch

# Wait a moment, then check status
sleep 2
sudo systemctl status tidewatch

# If still running, force kill
sudo pkill -9 tidewatch

# Check for stale lock file
sudo rm -f /var/lib/tidewatch/tidewatch.lock
```

### Service Keeps Restarting

**Symptom**: Service in restart loop

**Diagnosis**:
```bash
# Check restart count
sudo systemctl status tidewatch | grep Restart

# View crash pattern
sudo journalctl -u tidewatch --since "10 minutes ago"
```

**Solution**:
```bash
# Stop restart loop
sudo systemctl stop tidewatch

# Fix underlying issue (see crash logs)
# Then start manually
sudo systemctl start tidewatch
```

## Metrics Collection Issues

### No Metrics Being Collected

**Symptom**: Database empty or no new metrics

**Diagnosis**:
```bash
# Check database
sudo sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) FROM metrics;"

# View collector logs
sudo journalctl -u tidewatch | grep -i "collect"

# Check collector status
curl http://localhost:8080/metrics 2>/dev/null | grep collector
```

**Solution**:
```bash
# Ensure collectors are enabled in config
sudo nano /etc/tidewatch/config.yaml

# Example:
# collectors:
#   cpu:
#     enabled: true
#   memory:
#     enabled: true

# Restart service
sudo systemctl restart tidewatch
```

### Specific Collector Not Working

**Symptom**: One collector fails while others work

**Common Issues**:

1. **Thermal Collector**
   ```bash
   # Check if thermal zones exist
   ls /sys/class/thermal/thermal_zone*/temp

   # If not found, disable in config
   sudo nano /etc/tidewatch/config.yaml
   # collectors:
   #   thermal:
   #     enabled: false
   ```

2. **Network Collector**
   ```bash
   # Check interface names
   ip link show

   # Update config with correct names
   # collectors:
   #   network:
   #     interfaces:
   #       - eth0  # Use actual interface names
   ```

3. **Disk Collector**
   ```bash
   # Check mount points
   df -h

   # Update config
   # collectors:
   #   disk:
   #     paths:
   #       - /  # Use actual mount points
   ```

## Upload Issues

### Metrics Not Uploading to VictoriaMetrics

**Symptom**: Database grows but VictoriaMetrics has no data

**Diagnosis**:
```bash
# Check upload logs
sudo journalctl -u tidewatch | grep -i "upload\|remote"

# Test connectivity
curl -v http://your-vm:8428/

# Check if upload enabled
grep -A 5 "remote:" /etc/tidewatch/config.yaml
```

**Solutions**:

1. **Enable Remote Upload**
   ```yaml
   remote:
     enabled: true  # Must be true
     url: http://your-vm:8428/api/v1/import
   ```

2. **Fix Network Connectivity**
   ```bash
   # Test VictoriaMetrics reachability
   ping -c 3 your-vm-hostname

   # Test port
   nc -zv your-vm-hostname 8428

   # Check firewall
   sudo iptables -L OUTPUT -n -v | grep 8428
   ```

3. **Check VictoriaMetrics Logs**
   ```bash
   # On VictoriaMetrics server
   journalctl -u victoriametrics | grep -i error
   ```

### Upload Errors in Logs

**Symptom**: Repeated upload errors in journalctl

**Common Errors**:

1. **Connection Refused**
   ```
   Error uploading metrics: connection refused
   ```

   **Solution**: VictoriaMetrics is down or unreachable
   ```bash
   # Check VictoriaMetrics status (on VM server)
   systemctl status victoriametrics
   ```

2. **Invalid URL**
   ```
   Error uploading metrics: 404 Not Found
   ```

   **Solution**: URL missing `/api/v1/import`
   ```yaml
   remote:
     url: http://your-vm:8428/api/v1/import  # Include full path
   ```

3. **Timeout**
   ```
   Error uploading metrics: context deadline exceeded
   ```

   **Solution**: Increase timeout
   ```yaml
   remote:
     timeout: 60s  # Increase from default 30s
   ```

## Database Issues

### Database Locked Error

**Symptom**: `database is locked` in logs

**Cause**: Another process has the database open, or stale lock

**Solution**:
```bash
# Stop service
sudo systemctl stop tidewatch

# Check for other processes
sudo lsof /var/lib/tidewatch/metrics.db

# Kill any processes using the database
sudo pkill -9 tidewatch

# Remove lock file
sudo rm -f /var/lib/tidewatch/metrics.db-shm
sudo rm -f /var/lib/tidewatch/metrics.db-wal

# Start service
sudo systemctl start tidewatch
```

### Database Growing Too Large

**Symptom**: `/var/lib/tidewatch/metrics.db` consuming excessive disk space

**Solution**:
```bash
# Check current size
du -h /var/lib/tidewatch/metrics.db

# Configure cleanup in config
sudo nano /etc/tidewatch/config.yaml

# storage:
#   max_age: 72h        # Keep only 3 days (default: 168h / 7 days)
#   cleanup_interval: 1h  # Run cleanup every hour

# Restart to apply
sudo systemctl restart tidewatch

# Manual cleanup (immediate)
sudo sqlite3 /var/lib/tidewatch/metrics.db "DELETE FROM metrics WHERE timestamp < datetime('now', '-3 days');"
sudo sqlite3 /var/lib/tidewatch/metrics.db "VACUUM;"
```

### Database Corruption

**Symptom**: `database disk image is malformed`

**Solution**:
```bash
# Stop service
sudo systemctl stop tidewatch

# Backup database
sudo cp /var/lib/tidewatch/metrics.db /var/lib/tidewatch/metrics.db.corrupt

# Try to repair
sudo sqlite3 /var/lib/tidewatch/metrics.db ".recover" | sudo sqlite3 /var/lib/tidewatch/metrics.db.recovered

# If recovery worked, replace
sudo mv /var/lib/tidewatch/metrics.db.recovered /var/lib/tidewatch/metrics.db
sudo chown tidewatch:tidewatch /var/lib/tidewatch/metrics.db

# If recovery failed, start fresh
sudo rm /var/lib/tidewatch/metrics.db
sudo systemctl start tidewatch
```

## Performance Issues

### High CPU Usage

**Symptom**: Tidewatch consuming excessive CPU

**Diagnosis**:
```bash
# Check CPU usage
top -bn1 | grep tidewatch

# Check systemd limits
sudo systemctl show tidewatch | grep CPU
```

**Solutions**:

1. **Reduce Collection Frequency**
   ```yaml
   collectors:
     cpu:
       interval: 30s  # Increase from 15s
     memory:
       interval: 30s
   ```

2. **Disable Unnecessary Collectors**
   ```yaml
   collectors:
     thermal:
       enabled: false  # If not needed
   ```

3. **Reduce Upload Batch Size**
   ```yaml
   remote:
     batch_size: 500  # Reduce from 1000
     chunk_size: 50   # Reduce from 100
   ```

### High Memory Usage

**Symptom**: Tidewatch using more than expected memory

**Diagnosis**:
```bash
# Check memory usage
sudo systemctl status tidewatch | grep Memory

# Check for memory leak
ps aux | grep tidewatch
# Watch RSS column over time
```

**Solutions**:

1. **Reduce Batch Size**
   ```yaml
   remote:
     batch_size: 500  # Smaller batches = less memory
   ```

2. **Increase Upload Frequency**
   ```yaml
   remote:
     upload_interval: 15s  # Upload more often to flush queue
   ```

3. **Lower Memory Limit**
   ```bash
   # Edit service file
   sudo systemctl edit tidewatch

   # Add:
   # [Service]
   # MemoryMax=100M

   sudo systemctl daemon-reload
   sudo systemctl restart tidewatch
   ```

## Watchdog Issues

### Service Killed by Watchdog

**Symptom**: Service restarts with `watchdog timeout` in logs

**Diagnosis**:
```bash
# Check watchdog status
sudo systemctl show tidewatch | grep Watchdog

# View watchdog-related logs
sudo journalctl -u tidewatch | grep -i watchdog
```

**Solutions**:

1. **Increase Watchdog Timeout**
   ```bash
   sudo systemctl edit tidewatch

   # Add:
   # [Service]
   # WatchdogSec=120s

   sudo systemctl daemon-reload
   sudo systemctl restart tidewatch
   ```

2. **Check for Blocking Operations**
   ```bash
   # Look for long-running operations in logs
   sudo journalctl -u tidewatch | grep -i "slow\|timeout\|block"
   ```

3. **Disable Watchdog Temporarily** (debugging only)
   ```bash
   sudo systemctl edit tidewatch

   # Add:
   # [Service]
   # WatchdogSec=0

   sudo systemctl daemon-reload
   sudo systemctl restart tidewatch
   ```

### Watchdog Not Working

**Symptom**: Service hangs but doesn't restart

**Diagnosis**:
```bash
# Check if watchdog is enabled
sudo systemctl show tidewatch | grep WatchdogSec

# Test watchdog
sudo systemctl kill -s STOP tidewatch
sleep 70
sudo systemctl status tidewatch
# Should show restart after ~60 seconds
```

**Solution**:
```bash
# Ensure systemd version supports watchdog
systemctl --version
# Should be 240+

# Check service file
grep WatchdogSec /usr/lib/systemd/system/tidewatch.service

# If missing, reinstall package
sudo apt --reinstall install ./tidewatch_*.deb
```

## Diagnostic Tools

### Comprehensive Health Check

```bash
#!/bin/bash
echo "=== Tidewatch Health Check ==="

echo -e "\n--- Service Status ---"
systemctl status tidewatch --no-pager

echo -e "\n--- Recent Logs ---"
journalctl -u tidewatch -n 20 --no-pager

echo -e "\n--- Configuration ---"
head -20 /etc/tidewatch/config.yaml

echo -e "\n--- File Permissions ---"
ls -la /etc/tidewatch/
ls -la /var/lib/tidewatch/

echo -e "\n--- Database Stats ---"
sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) AS total_metrics FROM metrics;"
sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) AS pending_upload FROM metrics WHERE uploaded = 0;"

echo -e "\n--- Resource Usage ---"
systemctl show tidewatch | grep -E "Memory|CPU"

echo -e "\n--- Network Test ---"
REMOTE_URL=$(grep -A 5 "remote:" /etc/tidewatch/config.yaml | grep url | awk '{print $2}')
if [ ! -z "$REMOTE_URL" ]; then
    echo "Testing connectivity to $REMOTE_URL"
    curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" $REMOTE_URL
fi

echo -e "\n--- Health Endpoint ---"
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    curl -s http://localhost:8080/health | head -10
else
    echo "Health endpoint not available"
fi
```

Save this as `/tmp/tidewatch-health.sh` and run:

```bash
chmod +x /tmp/tidewatch-health.sh
sudo /tmp/tidewatch-health.sh
```

### Enable Debug Logging

```bash
# Edit config
sudo nano /etc/tidewatch/config.yaml

# Change logging level
logging:
  level: debug  # Was: info

# Restart service
sudo systemctl restart tidewatch

# Follow debug logs
sudo journalctl -u tidewatch -f
```

### Generate Support Bundle

```bash
#!/bin/bash
BUNDLE="/tmp/tidewatch-support-$(date +%Y%m%d-%H%M%S).tar.gz"

mkdir -p /tmp/tidewatch-support
cd /tmp/tidewatch-support

# Collect info
systemctl status tidewatch --no-pager > service-status.txt
journalctl -u tidewatch --no-pager > service-logs.txt
cp /etc/tidewatch/config.yaml config.yaml.redacted
ls -laR /var/lib/tidewatch/ > file-permissions.txt
sqlite3 /var/lib/tidewatch/metrics.db ".schema" > db-schema.txt
sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) FROM metrics;" > db-stats.txt

# Create tarball
cd /tmp
tar czf "$BUNDLE" tidewatch-support/
rm -rf /tmp/tidewatch-support

echo "Support bundle created: $BUNDLE"
echo "Please review and redact any sensitive information before sharing"
```

## Getting Help

If you can't resolve the issue:

1. **Check GitHub Issues**: https://github.com/taniwha3/tidewatch/issues
2. **Create New Issue**: Include output from diagnostic tools above
3. **Join Discussions**: https://github.com/taniwha3/tidewatch/discussions

### When Reporting Issues

Please include:
- Tidewatch version (`tidewatch -version`)
- Architecture (`dpkg --print-architecture`)
- OS version (`cat /etc/os-release`)
- Service status (`systemctl status tidewatch`)
- Recent logs (`journalctl -u tidewatch -n 100`)
- Configuration (redact sensitive URLs/tokens)
