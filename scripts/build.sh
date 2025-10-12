#!/bin/bash
set -e

echo "==> Building thugshells metrics collector"

# Create bin directory
mkdir -p bin

# Build for macOS (development)
echo "Building for macOS (darwin/arm64)..."
GOOS=darwin GOARCH=arm64 go build -o bin/metrics-collector-darwin \
    -ldflags "-s -w" \
    cmd/metrics-collector/main.go

echo "Building receiver for macOS..."
GOOS=darwin GOARCH=arm64 go build -o bin/metrics-receiver-darwin \
    -ldflags "-s -w" \
    cmd/metrics-receiver/main.go

# Build for Orange Pi (ARM64 Linux)
echo "Building for Orange Pi (linux/arm64)..."
GOOS=linux GOARCH=arm64 go build -o bin/metrics-collector-linux-arm64 \
    -ldflags "-s -w" \
    cmd/metrics-collector/main.go

echo "Building receiver for Linux..."
GOOS=linux GOARCH=arm64 go build -o bin/metrics-receiver-linux-arm64 \
    -ldflags "-s -w" \
    cmd/metrics-receiver/main.go

# Make binaries executable
chmod +x bin/*

echo ""
echo "==> Build complete"
ls -lh bin/
