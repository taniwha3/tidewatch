#!/bin/bash
set -e

echo "=== Testing Package Installation ==="

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

# Verify binary installed
test -x /usr/bin/tidewatch
tidewatch -version

# Verify user created
id tidewatch

# Verify directories created
test -d /var/lib/tidewatch
test -d /etc/tidewatch

# Verify permissions
stat -c "%a %U:%G" /var/lib/tidewatch | grep "750 tidewatch:tidewatch"
stat -c "%a %U:%G" /etc/tidewatch/config.yaml | grep "640 root:tidewatch"

# Verify service installed and running
systemctl list-unit-files | grep tidewatch
systemctl is-active tidewatch || systemctl status tidewatch

echo "=== Installation Test: PASSED ==="
