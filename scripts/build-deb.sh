#!/bin/bash
set -euo pipefail

# Tidewatch Debian Package Build Script
# Usage: ./scripts/build-deb.sh [VERSION] [ARCH]

VERSION="${1:-3.0.0}"
ARCH="${2:-arm64}"

echo "Building tidewatch ${VERSION} for ${ARCH}"

# Validate architecture
if [ "$ARCH" != "arm64" ] && [ "$ARCH" != "armhf" ]; then
    echo "Error: Architecture must be 'arm64' or 'armhf'"
    exit 1
fi

# Ensure bin directory exists
mkdir -p bin

# Build binary
echo "Building binary for ${ARCH}..."
if [ "$ARCH" = "arm64" ]; then
    GOARCH=arm64
elif [ "$ARCH" = "armhf" ]; then
    GOARCH=arm
    export GOARM=7
else
    echo "Unknown arch: $ARCH"
    exit 1
fi

CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH go build \
    -ldflags="-w -s -X main.appVersion=${VERSION}" \
    -trimpath \
    -o bin/tidewatch \
    ./cmd/tidewatch

echo "Binary built: bin/tidewatch"
file bin/tidewatch

# Check if nfpm is installed
if ! command -v nfpm &> /dev/null; then
    echo "Error: nfpm not found. Install with: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest"
    exit 1
fi

# Package
echo "Building Debian package..."
export VERSION ARCH
nfpm package --config nfpm.yaml --packager deb --target "tidewatch_${VERSION}-1_${ARCH}.deb"

# Generate checksum
echo "Generating checksum..."
sha256sum "tidewatch_${VERSION}-1_${ARCH}.deb" > "tidewatch_${VERSION}-1_${ARCH}.deb.sha256"

echo ""
echo "✅ Package built successfully:"
echo "   - tidewatch_${VERSION}-1_${ARCH}.deb"
echo "   - tidewatch_${VERSION}-1_${ARCH}.deb.sha256"
echo ""
echo "To sign the package with GPG:"
echo "   gpg --armor --detach-sign tidewatch_${VERSION}-1_${ARCH}.deb"
echo ""
echo "To install locally (requires root):"
echo "   sudo dpkg -i tidewatch_${VERSION}-1_${ARCH}.deb"
