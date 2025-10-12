#!/bin/bash
set -e

BINARY=${1:-bin/metrics-collector-linux-arm64}
SERVICE_NAME=metrics-collector
CONFIG_DIR=/etc/belabox-metrics
DATA_DIR=/var/lib/belabox-metrics

echo "==> Installing thugshells metrics collector"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
  echo "Please run as root or with sudo"
  exit 1
fi

# Stop existing service
echo "Stopping existing service..."
systemctl stop $SERVICE_NAME 2>/dev/null || true
systemctl disable $SERVICE_NAME 2>/dev/null || true

# Install binary
echo "Installing binary..."
cp $BINARY /usr/local/bin/metrics-collector
chmod +x /usr/local/bin/metrics-collector

# Create directories
echo "Creating directories..."
mkdir -p $CONFIG_DIR
mkdir -p $DATA_DIR

# Install config (only if it doesn't exist)
if [ ! -f $CONFIG_DIR/config.yaml ]; then
  echo "Installing default config..."
  cp configs/config.yaml $CONFIG_DIR/config.yaml
  echo "  Created $CONFIG_DIR/config.yaml"
  echo "  Please edit this file to configure your device"
else
  echo "  Existing config found at $CONFIG_DIR/config.yaml (not overwriting)"
fi

# Install systemd service
echo "Installing systemd service..."
cp systemd/metrics-collector.service /etc/systemd/system/
systemctl daemon-reload

# Enable and start service
echo "Enabling service..."
systemctl enable $SERVICE_NAME

echo "Starting service..."
systemctl start $SERVICE_NAME

# Wait a moment for service to start
sleep 2

# Show status
echo ""
echo "==> Installation complete"
echo ""
systemctl status $SERVICE_NAME --no-pager || true

echo ""
echo "Service installed and started successfully!"
echo ""
echo "Useful commands:"
echo "  sudo systemctl status $SERVICE_NAME"
echo "  sudo systemctl restart $SERVICE_NAME"
echo "  sudo journalctl -u $SERVICE_NAME -f"
echo "  sudo sqlite3 $DATA_DIR/metrics.db 'SELECT * FROM metrics LIMIT 10'"
