#!/bin/bash
set -e

echo "=== Testing Functional Behavior ==="

# Detect architecture to select correct package
DEB_ARCH=$(dpkg --print-architecture)
echo "Detected architecture: $DEB_ARCH"

# Wait for systemd to be ready
timeout=30
while [ $timeout -gt 0 ]; do
    if systemctl is-system-running --wait 2>/dev/null; then
        break
    fi
    sleep 1
    timeout=$((timeout - 1))
done

# Install architecture-specific package
apt-get update
apt-get install -y /packages/tidewatch_*_${DEB_ARCH}.deb

# Configure tidewatch to use VictoriaMetrics (must be running in docker-compose)
cat > /etc/tidewatch/config.yaml <<EOF
device:
  id: test-device-$(hostname)
remote:
  url: http://victoriametrics:8428/api/v1/import
  upload_interval: 10s
  batch_size: 100
  chunk_size: 50
  enabled: true
storage:
  path: /var/lib/tidewatch/metrics.db
logging:
  level: info
  format: console
collectors:
  cpu_usage:
    enabled: true
    interval: 10s
  memory_usage:
    enabled: true
    interval: 10s
  disk_io:
    enabled: true
    interval: 10s
  network_traffic:
    enabled: true
    interval: 10s
EOF

# Restart service with new config
systemctl restart tidewatch

# Wait for metrics collection and upload
echo "Waiting for metrics collection (30s)..."
sleep 30

# Check service is still running
if ! systemctl is-active tidewatch; then
    echo "ERROR: tidewatch service died"
    journalctl -u tidewatch -n 100
    exit 1
fi

# Query VictoriaMetrics
echo "Querying VictoriaMetrics for metrics..."
RESPONSE=$(curl -s 'http://victoriametrics:8428/api/v1/query?query=cpu_usage_percent%7Bdevice_id%3D~%22test-device-.*%22%7D')
echo "VictoriaMetrics response: $RESPONSE"

METRICS=$(echo "$RESPONSE" | jq -r '.data.result | length')

if [ "$METRICS" -gt 0 ]; then
    echo "=== Functional Test: PASSED (found $METRICS metric series) ==="
else
    echo "=== Functional Test: FAILED (no metrics found) ==="
    echo "Checking tidewatch logs:"
    journalctl -u tidewatch -n 50
    echo "Checking database:"
    sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) FROM metrics;" || true
    exit 1
fi

echo "=== Functional Test: PASSED ==="
