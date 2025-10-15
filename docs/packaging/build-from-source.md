# Building Tidewatch from Source

Guide for building Debian packages locally and contributing to development.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Build](#quick-build)
- [Development Setup](#development-setup)
- [Building Packages](#building-packages)
- [Testing Locally](#testing-locally)
- [Cross-Compilation](#cross-compilation)
- [Contributing](#contributing)

## Prerequisites

### System Requirements

- **OS**: Linux (Debian/Ubuntu recommended) or macOS
- **Go**: 1.24.0 or newer
- **Git**: 2.x or newer
- **Make**: GNU Make (optional, for convenience)

### Build Tools

```bash
# On Debian/Ubuntu
sudo apt update
sudo apt install -y \
    golang-1.21 \
    git \
    make \
    curl \
    qemu-user-static \
    binfmt-support

# Install nfpm (for packaging)
echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | \
    sudo tee /etc/apt/sources.list.d/goreleaser.list
sudo apt update
sudo apt install -y nfpm
```

On macOS:

```bash
brew install go git make
brew install goreleaser/tap/nfpm
```

### Verify Installation

```bash
go version       # Should be 1.24.0+
nfpm --version   # Should be 2.x
git --version
```

## Quick Build

### Clone Repository

```bash
git clone https://github.com/taniwha3/tidewatch.git
cd tidewatch
```

### Build Binary

```bash
# Native build
go build -o tidewatch ./cmd/tidewatch

# Test the binary
./tidewatch -version
```

### Run Locally

```bash
# Create config
cp configs/config.dev.yaml config.yaml

# Edit config for your environment
nano config.yaml

# Run (requires appropriate permissions)
./tidewatch -config config.yaml
```

## Development Setup

### Initialize Go Modules

```bash
# Already initialized, but to update dependencies:
go mod download
go mod tidy
```

### Install Development Tools

```bash
# Install linters
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install testing tools
go install gotest.tools/gotestsum@latest
```

### Project Structure

```
tidewatch/
├── cmd/
│   └── tidewatch/          # Main application entry point
│       └── main.go
├── internal/               # Private application code
│   ├── collector/          # Metrics collectors
│   ├── config/            # Configuration handling
│   ├── lockfile/          # Process locking
│   ├── storage/           # Database operations
│   ├── uploader/          # VictoriaMetrics upload
│   └── watchdog/          # Systemd watchdog
├── configs/               # Example configurations
├── debian/                # Debian packaging metadata
├── tests/                 # Integration tests
├── scripts/               # Build and utility scripts
└── docs/                  # Documentation
```

### Run Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./internal/collector/...

# Verbose output
go test -v ./...

# Run integration tests (requires Docker)
cd tests
docker compose up -d victoriametrics
docker compose up --build tidewatch-arm64
docker compose exec tidewatch-arm64 /tests/test-functional.sh
docker compose down
```

## Building Packages

### Using Build Script (Recommended)

The project includes a build script for convenience:

```bash
# Build for current architecture
./scripts/build-deb.sh

# This will:
# 1. Detect your architecture
# 2. Build the Go binary
# 3. Create a .deb package
# 4. Generate SHA256 checksum
```

Output will be in the current directory:
- `tidewatch_VERSION_ARCH.deb`
- `tidewatch_VERSION_ARCH.deb.sha256`

### Manual Package Build

#### 1. Set Version

```bash
export VERSION="3.0.0"
export DEBIAN_VERSION="${VERSION}-1"
```

#### 2. Build Binary

For native architecture:

```bash
CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.appVersion=${VERSION}" \
    -o tidewatch \
    ./cmd/tidewatch
```

#### 3. Create Package with nfpm

```bash
# nfpm uses nfpm.yaml in project root
nfpm pkg --packager deb --target tidewatch_${DEBIAN_VERSION}_$(dpkg --print-architecture).deb
```

#### 4. Generate Checksum

```bash
sha256sum tidewatch_${DEBIAN_VERSION}_*.deb > tidewatch_${DEBIAN_VERSION}_*.deb.sha256
```

### Build All Architectures

```bash
# Build arm64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
    go build -ldflags="-s -w -X main.appVersion=${VERSION}" \
    -o tidewatch-arm64 ./cmd/tidewatch

# Build armhf (32-bit ARM)
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
    go build -ldflags="-s -w -X main.appVersion=${VERSION}" \
    -o tidewatch-armhf ./cmd/tidewatch

# Package both
# Update nfpm.yaml for arm64
sed -i.bak 's/arch: .*/arch: arm64/' nfpm.yaml
nfpm pkg --packager deb --target tidewatch_${DEBIAN_VERSION}_arm64.deb

# Update nfpm.yaml for armhf
sed -i.bak 's/arch: .*/arch: armhf/' nfpm.yaml
nfpm pkg --packager deb --target tidewatch_${DEBIAN_VERSION}_armhf.deb

# Restore original
mv nfpm.yaml.bak nfpm.yaml
```

## Testing Locally

### Test Package Installation

#### Using Docker (Recommended)

```bash
# For arm64
docker run --rm -it --platform linux/arm64 \
    -v $(pwd):/workspace \
    debian:bookworm-slim bash

# Inside container:
cd /workspace
apt update
apt install -y ./tidewatch_*_arm64.deb
systemctl status tidewatch  # Won't work without systemd as PID 1
tidewatch -version
exit
```

#### Using QEMU on x86_64

```bash
# Install QEMU
sudo apt install -y qemu-user-static binfmt-support

# Register ARM emulation
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes

# Run ARM container
docker run --rm -it --platform linux/arm64 \
    -v $(pwd):/workspace \
    debian:bookworm-slim bash
```

### Test with Systemd

Use the integration test infrastructure:

```bash
cd tests

# Copy package to test directory
mkdir -p packages
cp ../tidewatch_*.deb packages/

# Run tests
docker compose up -d victoriametrics
docker compose up --build tidewatch-arm64

# Wait for startup
sleep 10

# Run installation test
docker compose exec tidewatch-arm64 /tests/test-install.sh

# Run functional test
docker compose exec tidewatch-arm64 /tests/test-functional.sh

# View logs
docker compose logs tidewatch-arm64

# Cleanup
docker compose down -v
```

## Cross-Compilation

### Understanding Go Cross-Compilation

Go supports cross-compilation out of the box:

```bash
# Syntax
GOOS=target_os GOARCH=target_arch go build

# Examples
GOOS=linux GOARCH=arm64 go build      # 64-bit ARM
GOOS=linux GOARCH=arm GOARM=7 go build  # 32-bit ARM (ARMv7)
GOOS=linux GOARCH=amd64 go build      # 64-bit x86
```

### Build Matrix

```bash
#!/bin/bash
VERSION="3.0.0"

# Define targets
declare -A targets=(
    ["arm64"]="linux arm64 "
    ["armhf"]="linux arm 7"
    ["amd64"]="linux amd64 "
)

for arch in "${!targets[@]}"; do
    read -r goos goarch goarm <<< "${targets[$arch]}"

    echo "Building for $arch..."

    env GOOS=$goos GOARCH=$goarch GOARM=$goarm CGO_ENABLED=0 \
        go build \
        -ldflags="-s -w -X main.appVersion=${VERSION}" \
        -o tidewatch-$arch \
        ./cmd/tidewatch

    echo "Built: tidewatch-$arch"
done
```

### Verify Cross-Compiled Binaries

```bash
# Check binary format
file tidewatch-arm64
# Output: ELF 64-bit LSB executable, ARM aarch64, ...

file tidewatch-armhf
# Output: ELF 32-bit LSB executable, ARM, EABI5, ...

# Test with QEMU
qemu-aarch64-static tidewatch-arm64 -version
qemu-arm-static tidewatch-armhf -version
```

## Development Workflow

### Code Style

```bash
# Format code
go fmt ./...

# Organize imports
goimports -w .

# Run linters
golangci-lint run
```

### Pre-Commit Checks

```bash
#!/bin/bash
# Save as .git/hooks/pre-commit

echo "Running pre-commit checks..."

# Format
echo "Formatting code..."
go fmt ./...

# Lint
echo "Running linters..."
golangci-lint run || exit 1

# Test
echo "Running tests..."
go test ./... || exit 1

echo "Pre-commit checks passed!"
```

### Making Changes

1. **Create Feature Branch**
   ```bash
   git checkout -b feature/my-new-feature
   ```

2. **Make Changes**
   ```bash
   # Edit files
   nano internal/collector/cpu.go

   # Test changes
   go test ./internal/collector/...
   ```

3. **Build and Test**
   ```bash
   # Build
   go build ./cmd/tidewatch

   # Run locally
   ./tidewatch -config config.yaml

   # Build package
   ./scripts/build-deb.sh

   # Test package
   # ... use Docker tests
   ```

4. **Commit and Push**
   ```bash
   git add .
   git commit -m "Add new CPU metric collector"
   git push origin feature/my-new-feature
   ```

## Contributing

### Development Guidelines

1. **Code Quality**
   - Write tests for new features
   - Maintain >80% test coverage
   - Follow Go idioms and best practices
   - Document public APIs

2. **Commit Messages**
   - Use conventional commits format
   - Examples:
     - `feat: add temperature collector`
     - `fix: resolve database lock issue`
     - `docs: update installation guide`
     - `test: add integration tests for uploader`

3. **Pull Requests**
   - Reference related issues
   - Include tests
   - Update documentation
   - Ensure CI passes

### Testing Your Changes

Before submitting PR:

```bash
# Run all tests
go test ./...

# Run integration tests
cd tests && docker compose up --abort-on-container-exit

# Build packages
./scripts/build-deb.sh

# Lint
golangci-lint run

# Check documentation
# Ensure all docs are updated
```

### Debugging

#### Enable Debug Mode

```yaml
# config.yaml
logging:
  level: debug
  format: console
```

#### Use Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug
dlv debug ./cmd/tidewatch -- -config config.yaml

# Set breakpoint
(dlv) break main.main
(dlv) continue
```

#### Profile Performance

```bash
# CPU profile
go build -o tidewatch ./cmd/tidewatch
./tidewatch -config config.yaml -cpuprofile cpu.prof

# Memory profile
./tidewatch -config config.yaml -memprofile mem.prof

# Analyze
go tool pprof cpu.prof
go tool pprof mem.prof
```

## CI/CD Pipeline

### GitHub Actions Workflow

The project uses GitHub Actions for CI/CD (`.github/workflows/build-deb.yml`):

1. **Triggers**: Git tags (`v*`) or manual dispatch
2. **Build**: Both arm64 and armhf packages
3. **Test**: Smoke tests and integration tests
4. **Release**: Automatic GitHub Release creation

### Local CI Simulation

```bash
# Simulate CI build locally
act -W .github/workflows/build-deb.yml

# Or use the same steps manually:
VERSION="3.0.0-1"

# Build
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
    go build -ldflags="-s -w -X main.appVersion=${VERSION}" \
    -o tidewatch-arm64 ./cmd/tidewatch

# Package
nfpm pkg --packager deb

# Test
docker run --platform linux/arm64 -v $(pwd):/workspace \
    debian:bookworm-slim \
    bash -c "cd /workspace && apt update && apt install -y ./tidewatch_*.deb"
```

## Release Process

### Creating a Release

1. **Update Version**
   ```bash
   # Update debian/changelog
   dch -v 3.1.0-1 "Release version 3.1.0"

   # Commit changes
   git add debian/changelog
   git commit -m "chore: bump version to 3.1.0"
   ```

2. **Tag Release**
   ```bash
   git tag -a v3.1.0 -m "Release version 3.1.0"
   git push origin v3.1.0
   ```

3. **GitHub Actions Runs Automatically**
   - Builds packages
   - Runs tests
   - Creates GitHub Release
   - Uploads artifacts

4. **Verify Release**
   - Check GitHub Releases page
   - Download and test packages
   - Verify checksums and signatures

## Troubleshooting Build Issues

### Go Module Issues

```bash
# Clear module cache
go clean -modcache

# Re-download dependencies
go mod download

# Verify dependencies
go mod verify
```

### QEMU Issues

```bash
# Reset QEMU registration
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes

# Verify
docker run --rm -t --platform linux/arm64 arm64v8/debian uname -m
# Should output: aarch64
```

### nfpm Issues

```bash
# Validate nfpm.yaml
nfpm validate

# Debug packaging
nfpm pkg --packager deb --debug
```

## Additional Resources

- **Go Documentation**: https://go.dev/doc/
- **nfpm Documentation**: https://nfpm.goreleaser.com/
- **Debian Packaging**: https://www.debian.org/doc/manuals/maint-guide/
- **Contributing Guide**: `CONTRIBUTING.md` (in repository)
