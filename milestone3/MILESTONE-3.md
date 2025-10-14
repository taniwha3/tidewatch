# Milestone 3: Debian Packaging for ARM Ecosystem

## Overview

Milestone 3 focuses on creating production-ready Debian packages for the ARM ecosystem, enabling easy installation and management of Tidewatch (formerly thugshells/belabox-metrics) on embedded devices like Orange Pi, Raspberry Pi, and other ARM-based systems.

**New Project Name: `tidewatch`**
- Unique, memorable, no trademark conflicts
- Good metaphor: watching the tides = monitoring metrics
- Consistent across package, service, binary, and user naming

## Goals

1. Create Debian packages for arm64 and armhf architectures
2. Implement automated build pipeline via GitHub Actions
3. Integrate systemd watchdog and process locking from M2
4. Enable safe unattended upgrades with automatic database migrations
5. Provide comprehensive installation and troubleshooting documentation
6. Ensure production-ready package quality with GPG signatures

## Scope

### In Scope

- Debian package infrastructure (`debian/` directory)
- Multi-architecture builds (arm64, armhf)
- GitHub Actions CI/CD pipeline
- GPG package signing
- Systemd watchdog integration
- Process locking integration
- Automated database migrations on upgrade
- Docker-based integration testing
- Comprehensive documentation
- GitHub Releases distribution

### Out of Scope (Future)

- Custom APT repository hosting
- RPM packages for Red Hat/Fedora
- x86_64 packages
- Raspbian-specific optimizations
- Multi-distro support beyond Debian

## Architecture

### Package Structure

```
tidewatch (main package)
├── /usr/bin/tidewatch           # Binary
├── /etc/tidewatch/
│   └── config.yaml              # Configuration (conffile)
├── /var/lib/tidewatch/          # Database and runtime data
├── /usr/lib/systemd/system/
│   └── tidewatch.service        # Systemd unit
├── /usr/share/doc/tidewatch/
│   ├── README.md
│   ├── changelog.gz
│   ├── copyright
│   └── examples/
│       └── config.yaml.example
└── /usr/share/man/man8/
    └── tidewatch.8.gz           # Man page

tidewatch-doc (optional)
└── /usr/share/doc/tidewatch/
    ├── installation.md
    ├── quickstart.md
    ├── troubleshooting.md
    └── upgrading.md
```

### System Integration

```
User/Group: tidewatch (system account, no login)
Service: tidewatch.service (enabled by default)
Watchdog: 60s interval with systemd integration
Process Lock: /var/lib/tidewatch/tidewatch.lock
Database: /var/lib/tidewatch/metrics.db
```

## Implementation Plan

### Phase 1: Package Infrastructure (Day 1, ~8 hours)

**Objective:** Create Debian package structure and metadata

#### Tasks

1. **Create `debian/` Directory Structure**
   - Initialize debian packaging structure
   - Set up build dependencies

2. **Package Metadata (`debian/control`)**
   ```
   Source: tidewatch
   Section: net
   Priority: optional
   Maintainer: [Your Name] <[email]>
   Build-Depends: debhelper (>= 13), golang (>= 1.21), dh-golang
   Standards-Version: 4.6.0
   Homepage: https://github.com/[org]/tidewatch

   Package: tidewatch
   Architecture: any
   Depends: ${shlibs:Depends}, ${misc:Depends}, systemd, adduser, ca-certificates
   Recommends: sqlite3
   Suggests: victoriametrics
   Description: Lightweight system metrics collector for edge devices
    Tidewatch collects system metrics (CPU, memory, disk, network, thermal)
    from edge devices and uploads them to VictoriaMetrics for monitoring.
   ```

3. **Build Rules (`debian/rules`)**
   ```makefile
   #!/usr/bin/make -f

   %:
   	dh $@ --buildsystem=golang --with=systemd

   override_dh_auto_build:
   	CGO_ENABLED=0 go build -ldflags="-s -w" -o tidewatch ./cmd/tidewatch

   override_dh_auto_install:
   	install -D -m 0755 tidewatch debian/tidewatch/usr/bin/tidewatch
   	install -D -m 0644 configs/config.yaml debian/tidewatch/etc/tidewatch/config.yaml
   	install -D -m 0644 systemd/tidewatch.service debian/tidewatch/usr/lib/systemd/system/tidewatch.service
   ```

4. **Installation Scripts**

   **`debian/postinst`** (Post-installation):
   ```bash
   #!/bin/sh
   set -e

   case "$1" in
       configure)
           # Create system user and group
           if ! getent group tidewatch >/dev/null; then
               addgroup --system tidewatch
           fi
           if ! getent passwd tidewatch >/dev/null; then
               adduser --system --ingroup tidewatch --home /var/lib/tidewatch \
                   --no-create-home --gecos "Tidewatch Metrics Collector" \
                   --shell /usr/sbin/nologin tidewatch
           fi

           # Create directories
           mkdir -p /var/lib/tidewatch
           mkdir -p /etc/tidewatch

           # Set ownership and permissions
           chown -R tidewatch:tidewatch /var/lib/tidewatch
           chmod 750 /var/lib/tidewatch
           chown root:tidewatch /etc/tidewatch
           chmod 750 /etc/tidewatch

           # Set config file permissions only if it exists (admin may have removed it)
           if [ -f /etc/tidewatch/config.yaml ]; then
               chown root:tidewatch /etc/tidewatch/config.yaml
               chmod 640 /etc/tidewatch/config.yaml
           fi

           # Run database migrations on upgrade
           if [ ! -z "$2" ]; then
               echo "Running database migrations..."
               # Migration will happen automatically on service start
           fi

           # Enable and start service (only if systemctl is available)
           if command -v systemctl >/dev/null 2>&1; then
               systemctl daemon-reload || true
               systemctl enable tidewatch.service || true
               systemctl start tidewatch.service || true
           fi
           ;;
   esac

   #DEBHELPER#
   exit 0
   ```

   **`debian/prerm`** (Pre-removal):
   ```bash
   #!/bin/sh
   set -e

   case "$1" in
       remove|deconfigure)
           # Stop and disable service (only if systemctl is available)
           if command -v systemctl >/dev/null 2>&1; then
               systemctl stop tidewatch.service || true
               systemctl disable tidewatch.service || true
           fi
           ;;
   esac

   #DEBHELPER#
   exit 0
   ```

   **`debian/postrm`** (Post-removal):
   ```bash
   #!/bin/sh
   set -e

   case "$1" in
       purge)
           # Remove data directory
           rm -rf /var/lib/tidewatch
           rm -rf /etc/tidewatch

           # Optionally remove user/group
           if getent passwd tidewatch >/dev/null; then
               deluser --system tidewatch || true
           fi
           if getent group tidewatch >/dev/null; then
               delgroup --system tidewatch || true
           fi
           ;;
       remove)
           # Keep data on remove (user might reinstall)
           ;;
   esac

   #DEBHELPER#
   exit 0
   ```

5. **Conffiles (`debian/conffiles`)**
   ```
   /etc/tidewatch/config.yaml
   ```

6. **Systemd Service (`debian/tidewatch.service`)**
   ```ini
   [Unit]
   Description=Tidewatch Metrics Collector
   Documentation=https://github.com/[org]/tidewatch
   After=network-online.target
   Wants=network-online.target

   [Service]
   Type=notify
   User=tidewatch
   Group=tidewatch
   ExecStart=/usr/bin/tidewatch -config /etc/tidewatch/config.yaml
   Restart=on-failure
   RestartSec=5s

   # Watchdog
   WatchdogSec=60s

   # Security hardening (from M2)
   NoNewPrivileges=true
   ProtectSystem=strict
   ProtectHome=true
   PrivateTmp=true
   ReadWritePaths=/var/lib/tidewatch
   ReadOnlyPaths=/etc/tidewatch
   ProtectKernelTunables=true
   ProtectKernelModules=true
   ProtectKernelLogs=true
   ProtectControlGroups=true
   RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
   RestrictNamespaces=true
   RestrictRealtime=true
   RestrictSUIDSGID=true
   SystemCallFilter=@system-service
   SystemCallFilter=~@privileged @resources

   # Resource limits
   MemoryMax=200M
   CPUQuota=20%
   TasksMax=100

   [Install]
   WantedBy=multi-user.target
   ```

7. **Changelog (`debian/changelog`)**
   ```
   tidewatch (3.0.0-1) unstable; urgency=medium

     * Initial Debian package release (Milestone 3)
     * ARM support: arm64 and armhf architectures
     * Systemd watchdog integration
     * Process locking to prevent double-start
     * Automated database migrations on upgrade
     * Security hardening with systemd directives

    -- [Your Name] <[email]>  [Date]
   ```

8. **Copyright (`debian/copyright`)**
   ```
   Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
   Upstream-Name: tidewatch
   Source: https://github.com/[org]/tidewatch

   Files: *
   Copyright: 2025 [Your Name]
   License: [Your License]

   [Full license text]
   ```

9. **Man Page (`debian/tidewatch.8`)**
   ```
   .TH TIDEWATCH 8 "January 2025" "tidewatch 3.0.0" "System Administration"
   .SH NAME
   tidewatch \- lightweight system metrics collector for edge devices
   .SH SYNOPSIS
   .B tidewatch
   [\fB\-config\fR \fIPATH\fR]
   [\fB\-version\fR]
   .SH DESCRIPTION
   Tidewatch collects system metrics from edge devices and uploads them
   to VictoriaMetrics for monitoring and alerting.
   .SH OPTIONS
   .TP
   \fB\-config\fR \fIPATH\fR
   Path to configuration file (default: /etc/tidewatch/config.yaml)
   .TP
   \fB\-version\fR
   Print version and exit
   .SH FILES
   .TP
   /etc/tidewatch/config.yaml
   Main configuration file
   .TP
   /var/lib/tidewatch/metrics.db
   SQLite database for metric storage
   .SH SEE ALSO
   .BR systemctl (1),
   .BR journalctl (1)
   .SH BUGS
   Report bugs at https://github.com/[org]/tidewatch/issues
   ```

### Phase 2: Build Automation (Day 2, ~8 hours)

**Objective:** Automate package building with GitHub Actions

#### GitHub Actions Workflow

**`.github/workflows/build-deb.yml`:**

```yaml
name: Build Debian Packages

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:

permissions:
  contents: write

jobs:
  build:
    name: Build ${{ matrix.arch }} package
    runs-on: ubuntu-latest
    strategy:
      matrix:
        arch: [arm64, armhf]

    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Get version
        id: version
        run: |
          VERSION=${GITHUB_REF#refs/tags/v}
          echo "version=$VERSION" >> $GITHUB_OUTPUT
          echo "debian_version=${VERSION}-1" >> $GITHUB_OUTPUT

      - name: Build binary
        env:
          GOOS: linux
          GOARCH: ${{ matrix.arch == 'armhf' && 'arm' || matrix.arch }}
          GOARM: ${{ matrix.arch == 'armhf' && '7' || '' }}
          CGO_ENABLED: 0
        run: |
          go build -ldflags="-s -w -X main.version=${{ steps.version.outputs.version }}" \
            -o tidewatch-${{ matrix.arch }} ./cmd/tidewatch

      - name: Install nfpm
        run: |
          echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | \
            sudo tee /etc/apt/sources.list.d/goreleaser.list
          sudo apt-get update
          sudo apt-get install -y nfpm

      - name: Create nfpm config
        run: |
          cat > nfpm.yaml <<EOF
          name: tidewatch
          arch: ${{ matrix.arch }}
          platform: linux
          version: ${{ steps.version.outputs.version }}
          release: 1
          section: net
          priority: optional
          maintainer: [Your Name] <[email]>
          description: |
            Lightweight system metrics collector for edge devices
            Collects CPU, memory, disk, network, and thermal metrics
            and uploads them to VictoriaMetrics.
          vendor: [Your Organization]
          homepage: https://github.com/[org]/tidewatch
          license: [Your License]

          depends:
            - systemd
            - adduser
            - ca-certificates
          recommends:
            - sqlite3
          suggests:
            - victoriametrics

          contents:
            - src: tidewatch-${{ matrix.arch }}
              dst: /usr/bin/tidewatch
              file_info:
                mode: 0755

            - src: configs/config.yaml
              dst: /etc/tidewatch/config.yaml
              type: config
              file_info:
                mode: 0640

            - src: systemd/tidewatch.service
              dst: /usr/lib/systemd/system/tidewatch.service
              file_info:
                mode: 0644

            - src: README.md
              dst: /usr/share/doc/tidewatch/README.md
              file_info:
                mode: 0644

          scripts:
            postinstall: debian/postinst
            preremove: debian/prerm
            postremove: debian/postrm
          EOF

      - name: Build package
        run: |
          nfpm pkg --packager deb --target tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb

      - name: Generate checksums
        run: |
          sha256sum tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb > \
            tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb.sha256

      - name: Import GPG key
        if: github.event_name == 'push' && startsWith(github.ref, 'refs/tags/')
        env:
          GPG_PRIVATE_KEY: ${{ secrets.GPG_PRIVATE_KEY }}
          GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
        run: |
          echo "$GPG_PRIVATE_KEY" | gpg --batch --import
          echo "$GPG_PASSPHRASE" | gpg --batch --passphrase-fd 0 --pinentry-mode loopback \
            --output tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb.asc \
            --detach-sign tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: packages-${{ matrix.arch }}
          path: |
            tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb
            tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb.sha256
            tidewatch_${{ steps.version.outputs.debian_version }}_${{ matrix.arch }}.deb.asc

  test:
    name: Smoke test ${{ matrix.arch }} package
    needs: build
    runs-on: ubuntu-latest
    strategy:
      matrix:
        arch: [arm64, armhf]

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Download artifacts
        uses: actions/download-artifact@v4
        with:
          name: packages-${{ matrix.arch }}

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3
        with:
          platforms: arm64,arm

      - name: Smoke test - package installation
        run: |
          # Basic smoke test without systemd (full integration tests use docker-compose)
          PLATFORM=${{ matrix.arch == 'armhf' && 'linux/arm/v7' || 'linux/arm64' }}
          docker run --platform $PLATFORM --rm -v $(pwd):/workspace \
            debian:bookworm-slim /bin/bash -c "
              cd /workspace
              apt-get update
              apt-get install -y ./tidewatch_*.deb || exit 1
              # Verify binary installed and executable
              test -x /usr/bin/tidewatch || exit 1
              /usr/bin/tidewatch -version || exit 1
              # Verify user created
              id tidewatch || exit 1
              # Verify systemctl present (service won't start without systemd as PID 1)
              which systemctl || exit 1
              echo 'Smoke test passed'
            "

      - name: Smoke test - package removal
        run: |
          PLATFORM=${{ matrix.arch == 'armhf' && 'linux/arm/v7' || 'linux/arm64' }}
          docker run --platform $PLATFORM --rm -v $(pwd):/workspace \
            debian:bookworm-slim /bin/bash -c "
              cd /workspace
              apt-get update
              apt-get install -y ./tidewatch_*.deb || exit 1
              apt-get remove -y tidewatch || exit 1
              # Verify binary removed
              test ! -x /usr/bin/tidewatch || exit 1
              # Config should still exist after remove
              test -d /etc/tidewatch || exit 1
              # Now purge
              apt-get purge -y tidewatch || exit 1
              # Config should be gone after purge
              test ! -d /etc/tidewatch || exit 1
              echo 'Removal test passed'
            "

  integration-test:
    name: Integration test ${{ matrix.arch }} package
    needs: build
    runs-on: ubuntu-latest
    strategy:
      matrix:
        arch: [arm64, armhf]

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Download artifacts
        uses: actions/download-artifact@v4
        with:
          name: packages-${{ matrix.arch }}
          path: ./packages

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3
        with:
          platforms: arm64,arm

      - name: Create test directory structure
        run: |
          # GitHub Actions downloads artifacts to ./packages (root level)
          # We copy them into tests/packages/ for docker-compose
          mkdir -p tests/packages
          cp packages/*.deb tests/packages/

      - name: Run integration tests with systemd
        run: |
          cd tests
          ARCH=${{ matrix.arch }}

          # NOTE: docker-compose.yml mounts ./packages (relative to tests/)
          # which correctly maps to tests/packages/ where .deb files are located
          # Start systemd-enabled container with VictoriaMetrics
          docker-compose up -d victoriametrics
          docker-compose up -d tidewatch-${ARCH}

          # Wait for containers to be ready
          sleep 10

          # Run installation test (must pass or workflow fails)
          echo "Running installation test..."
          docker-compose exec -T tidewatch-${ARCH} /tests/test-install.sh

          # Run functional test (must pass or workflow fails)
          echo "Running functional test..."
          docker-compose exec -T tidewatch-${ARCH} /tests/test-functional.sh

          # Cleanup (always run, even if tests fail)
          echo "Cleaning up containers..."
          docker-compose down -v || true

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test-results-${{ matrix.arch }}
          path: tests/*.log

  release:
    name: Create GitHub Release
    needs: [test, integration-test]
    if: github.event_name == 'push' && startsWith(github.ref, 'refs/tags/')
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Download all artifacts
        uses: actions/download-artifact@v4

      - name: Export GPG public key
        env:
          GPG_PRIVATE_KEY: ${{ secrets.GPG_PRIVATE_KEY }}
        run: |
          echo "$GPG_PRIVATE_KEY" | gpg --batch --import
          gpg --armor --export > tidewatch-signing-key.asc

      - name: Create release notes
        run: |
          cat > RELEASE_NOTES.md <<EOF
          # Tidewatch ${GITHUB_REF#refs/tags/v}

          ## Installation

          \`\`\`bash
          # Download package for your architecture
          wget https://github.com/${{ github.repository }}/releases/download/${GITHUB_REF#refs/tags/}/tidewatch_*_arm64.deb

          # Install
          sudo apt install ./tidewatch_*_arm64.deb

          # Verify installation
          systemctl status tidewatch
          \`\`\`

          ## What's New

          - Debian packaging for arm64 and armhf
          - Systemd watchdog integration
          - Process locking
          - Automated database migrations

          ## Package Verification

          \`\`\`bash
          # Import GPG key
          wget https://github.com/${{ github.repository }}/releases/download/${GITHUB_REF#refs/tags/}/tidewatch-signing-key.asc
          gpg --import tidewatch-signing-key.asc

          # Verify signature
          gpg --verify tidewatch_*_arm64.deb.asc tidewatch_*_arm64.deb

          # Verify checksum
          sha256sum -c tidewatch_*_arm64.deb.sha256
          \`\`\`
          EOF

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          body_path: RELEASE_NOTES.md
          files: |
            packages-arm64/tidewatch_*.deb
            packages-arm64/tidewatch_*.deb.sha256
            packages-arm64/tidewatch_*.deb.asc
            packages-armhf/tidewatch_*.deb
            packages-armhf/tidewatch_*.deb.sha256
            packages-armhf/tidewatch_*.deb.asc
            tidewatch-signing-key.asc
```

### Phase 3: Watchdog & Process Locking Integration (Day 3, ~6 hours)

**Objective:** Integrate M2 packages into main application

#### Watchdog Integration

**Update `cmd/main.go`:**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/taniwha3/thugshells/internal/config"
	"github.com/taniwha3/thugshells/internal/lockfile"
	"github.com/taniwha3/thugshells/internal/watchdog"
	// ... other imports
)

var version = "dev" // Set by build

func main() {
	configPath := flag.String("config", "/etc/tidewatch/config.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("tidewatch %s\n", version)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Setup logging
	setupLogging(cfg)

	slog.Info("Starting tidewatch", "version", version)

	// Initialize process lock
	lockPath := "/var/lib/tidewatch/tidewatch.lock"
	lock, err := lockfile.Acquire(lockPath)
	if err != nil {
		slog.Error("Failed to acquire lock - another instance may be running",
			"error", err, "lock_path", lockPath)
		os.Exit(1)
	}
	defer lock.Release()
	slog.Info("Process lock acquired", "lock_path", lockPath)

	// Initialize storage
	store, err := storage.NewSQLiteStorage(cfg.Storage.Path)
	if err != nil {
		slog.Error("Failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Run migrations
	if err := store.Migrate(); err != nil {
		slog.Error("Database migration failed", "error", err)
		os.Exit(1)
	}

	// Initialize watchdog
	wd := watchdog.NewPinger(slog.Default())

	// Create context for watchdog goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watchdog ping routine in background
	if wd.IsEnabled() {
		go wd.Start(ctx)
		slog.Info("Watchdog enabled", "interval", wd.GetInterval())
	}

	// ... rest of initialization (collectors, uploader, health server)

	// Notify systemd we're ready
	wd.NotifyReady()

	// ... start collection loops and servers

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutdown signal received")

	// Notify systemd we're stopping
	wd.NotifyStopping()

	// Cancel watchdog context to stop ping routine
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// ... cleanup (stop collectors, flush data, etc.)

	slog.Info("Shutdown complete")
}

func setupLogging(cfg *config.Config) {
	// ... logging setup from M2
}
```

#### Tasks

1. **Integrate Lockfile Package**
   - Add lock acquisition in main()
   - Handle lock errors gracefully
   - Release lock on shutdown
   - Test double-start prevention

2. **Integrate Watchdog Package**
   - Initialize watchdog from systemd environment
   - Send READY notification after initialization
   - Start ping goroutine
   - Send STOPPING on graceful shutdown
   - Test watchdog timeout handling

3. **Update Systemd Service**
   - Set `Type=notify`
   - Add `WatchdogSec=60s`
   - Add `Restart=on-failure`
   - Test service behavior

4. **Add Command-Line Flags**
   - `-config` for config path
   - `-version` for version output
   - Document in man page

5. **Testing**
   - Test normal startup/shutdown
   - Test double-start prevention
   - Test watchdog timeout (kill -STOP)
   - Test graceful shutdown (SIGTERM)
   - Test crash recovery (SIGKILL)

### Phase 4: Configuration & Migration (Day 3, ~4 hours)

**Objective:** Ensure smooth installation and upgrades

#### Tasks

1. **Default Configuration**
   - Create production-ready default config
   - Document all options with comments
   - Set safe defaults for resource usage
   - Place in `/etc/tidewatch/config.yaml`

2. **Config Examples**
   - Create example configs in `/usr/share/doc/tidewatch/examples/`
   - Examples: minimal, full-featured, high-frequency
   - Document each example's use case

3. **Postinst Script Testing**
   - Test user/group creation
   - Test directory permissions
   - Test service enablement
   - Test fresh install vs upgrade

4. **Database Migration Testing**
   - Test migration from M1 → M2 → M3
   - Test rollback scenario
   - Test migration failure handling
   - Verify data integrity

5. **Conffile Handling**
   - Test config preservation on upgrade
   - Test config merge prompts
   - Document recommended upgrade procedure

### Phase 5: Documentation (Day 4, ~6 hours)

**Objective:** Comprehensive user documentation

#### Documents to Create

1. **Installation Guide** (`docs/installation-debian.md`)
   - System requirements
   - Architecture selection (arm64 vs armhf)
   - Download instructions
   - GPG verification steps
   - Installation via apt
   - First-time configuration
   - Service management basics

2. **Quick Start Guide** (`docs/quickstart.md`)
   - 5-minute setup
   - Connect to VictoriaMetrics
   - View first metrics
   - Common PromQL queries
   - Troubleshooting basics

3. **Troubleshooting Guide** (`docs/troubleshooting-debian.md`)
   - Service won't start
   - Permission denied errors
   - Database locked errors
   - Network connectivity issues
   - High resource usage
   - Log analysis with journalctl
   - Debug mode activation

4. **Upgrade Guide** (`docs/upgrading.md`)
   - Upgrade procedure
   - Config file conflicts
   - Database migration notes
   - Rollback procedure
   - Version compatibility matrix

5. **Man Pages**
   - `tidewatch(8)` - daemon manual
   - `tidewatch.yaml(5)` - config format

6. **README Updates**
   - Update main README with Debian installation
   - Link to detailed documentation
   - Update badge/status sections

### Phase 6: Testing & Validation (Day 4, ~6 hours)

**Objective:** Comprehensive package testing

#### Docker Integration Tests

**Test Suite Structure:**

```
tests/
├── Dockerfile.arm64
├── Dockerfile.armhf
├── docker-compose.yml
├── test-install.sh
├── test-upgrade.sh
├── test-remove.sh
└── test-functional.sh
```

**`Dockerfile.arm64`:**
```dockerfile
FROM arm64v8/debian:bookworm-slim

# Install systemd and dependencies
RUN apt-get update && apt-get install -y \
    systemd \
    systemd-sysv \
    ca-certificates \
    sqlite3 \
    curl \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Remove unnecessary systemd services
RUN cd /lib/systemd/system/sysinit.target.wants/ && \
    ls | grep -v systemd-tmpfiles-setup | xargs rm -f && \
    rm -f /lib/systemd/system/multi-user.target.wants/* && \
    rm -f /etc/systemd/system/*.wants/* && \
    rm -f /lib/systemd/system/local-fs.target.wants/* && \
    rm -f /lib/systemd/system/sockets.target.wants/*udev* && \
    rm -f /lib/systemd/system/sockets.target.wants/*initctl* && \
    rm -f /lib/systemd/system/basic.target.wants/* && \
    rm -f /lib/systemd/system/anaconda.target.wants/*

# Create mount points
VOLUME [ "/sys/fs/cgroup", "/run", "/run/lock" ]

# Start systemd
CMD ["/lib/systemd/systemd"]
```

**`Dockerfile.armhf`:**
```dockerfile
FROM arm32v7/debian:bookworm-slim

# Install systemd and dependencies
RUN apt-get update && apt-get install -y \
    systemd \
    systemd-sysv \
    ca-certificates \
    sqlite3 \
    curl \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Remove unnecessary systemd services
RUN cd /lib/systemd/system/sysinit.target.wants/ && \
    ls | grep -v systemd-tmpfiles-setup | xargs rm -f && \
    rm -f /lib/systemd/system/multi-user.target.wants/* && \
    rm -f /etc/systemd/system/*.wants/* && \
    rm -f /lib/systemd/system/local-fs.target.wants/* && \
    rm -f /lib/systemd/system/sockets.target.wants/*udev* && \
    rm -f /lib/systemd/system/sockets.target.wants/*initctl* && \
    rm -f /lib/systemd/system/basic.target.wants/* && \
    rm -f /lib/systemd/system/anaconda.target.wants/*

# Create mount points
VOLUME [ "/sys/fs/cgroup", "/run", "/run/lock" ]

# Start systemd
CMD ["/lib/systemd/systemd"]
```

**`docker-compose.yml`:**
```yaml
version: '3.8'

services:
  tidewatch-arm64:
    build:
      context: .
      dockerfile: Dockerfile.arm64
    platform: linux/arm64
    privileged: true
    volumes:
      - ./packages:/packages:ro
      - .:/tests:ro  # Mount current dir (tests/) to /tests in container
      - /sys/fs/cgroup:/sys/fs/cgroup:rw
    tmpfs:
      - /run
      - /run/lock
    networks:
      - metrics
    command: /lib/systemd/systemd

  tidewatch-armhf:
    build:
      context: .
      dockerfile: Dockerfile.armhf
    platform: linux/arm/v7
    privileged: true
    volumes:
      - ./packages:/packages:ro
      - .:/tests:ro  # Mount current dir (tests/) to /tests in container
      - /sys/fs/cgroup:/sys/fs/cgroup:rw
    tmpfs:
      - /run
      - /run/lock
    networks:
      - metrics
    command: /lib/systemd/systemd

  victoriametrics:
    image: victoriametrics/victoria-metrics:latest
    ports:
      - "8428:8428"
    networks:
      - metrics
    command:
      - --storageDataPath=/victoria-metrics-data
      - --httpListenAddr=:8428

networks:
  metrics:
    driver: bridge
```

**`test-install.sh`:**
```bash
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
```

**`test-functional.sh`:**
```bash
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
RESPONSE=$(curl -s 'http://victoriametrics:8428/api/v1/query?query=cpu_usage_percent{device_id=~"test-device-.*"}')
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
```

**`test-upgrade.sh`:**
```bash
#!/bin/bash
set -e

echo "=== Testing Package Upgrade ==="

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

# Install old version (if available)
if [ -f /packages/tidewatch_old_${DEB_ARCH}.deb ]; then
    apt-get update
    apt-get install -y /packages/tidewatch_old_${DEB_ARCH}.deb

    # Wait for service to start
    sleep 5

    # Verify running
    systemctl is-active tidewatch

    # Create some test data
    sleep 10

    # Stop service before upgrade
    systemctl stop tidewatch

    # Backup database
    cp /var/lib/tidewatch/metrics.db /tmp/metrics-backup.db

    # Upgrade to new version (architecture-specific)
    apt-get install -y /packages/tidewatch_*_${DEB_ARCH}.deb

    # Verify service restarted
    sleep 3
    systemctl is-active tidewatch || systemctl status tidewatch

    # Verify data preserved
    test -f /var/lib/tidewatch/metrics.db

    echo "Database backup size: $(stat -c%s /tmp/metrics-backup.db)"
    echo "Database current size: $(stat -c%s /var/lib/tidewatch/metrics.db)"

    echo "=== Upgrade Test: PASSED ==="
else
    echo "=== Upgrade Test: SKIPPED (no old version available) ==="
fi
```

**`test-remove.sh`:**
```bash
#!/bin/bash
set -e

echo "=== Testing Package Removal ==="

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

# Install and start (architecture-specific)
apt-get update
apt-get install -y /packages/tidewatch_*_${DEB_ARCH}.deb

# Wait for service
sleep 5
systemctl is-active tidewatch

# Remove package (keeps config)
apt-get remove -y tidewatch

# Verify service stopped
if systemctl is-active tidewatch 2>/dev/null; then
    echo "ERROR: Service still active after removal"
    exit 1
fi

# Verify binary removed
if test -x /usr/bin/tidewatch; then
    echo "ERROR: Binary still exists after removal"
    exit 1
fi

# Verify data preserved on remove
test -d /var/lib/tidewatch
test -f /etc/tidewatch/config.yaml

echo "=== Remove Test: PASSED ==="

# Purge package (removes everything)
apt-get purge -y tidewatch

# Verify complete cleanup
if test -d /var/lib/tidewatch; then
    echo "ERROR: Data directory still exists after purge"
    exit 1
fi

if test -d /etc/tidewatch; then
    echo "ERROR: Config directory still exists after purge"
    exit 1
fi

echo "=== Purge Test: PASSED ==="
```

#### Test Matrix

| Test | arm64 | armhf | Status |
|------|-------|-------|--------|
| Install | ✅ | ✅ | |
| Upgrade | ✅ | ✅ | |
| Remove | ✅ | ✅ | |
| Purge | ✅ | ✅ | |
| Functional | ✅ | ✅ | |
| Double-start | ✅ | ✅ | |
| Watchdog | ✅ | ✅ | |

### Phase 7: Release Process (Day 5, ~4 hours)

**Objective:** Publish first Debian release

#### Tasks

1. **Version Tagging**
   - Create git tag: `v3.0.0`
   - Update changelog with release notes
   - Update version in code

2. **Build Trigger**
   - Push tag to trigger GitHub Actions
   - Monitor build progress
   - Review test results

3. **Release Verification**
   - Download packages from GitHub Releases
   - Verify GPG signatures
   - Verify checksums
   - Test installation on real ARM device (if available)

4. **Documentation Publication**
   - Ensure docs are up-to-date in repo
   - Create release announcement
   - Update main README

5. **Announcement**
   - Draft release notes
   - Highlight key features
   - Provide installation instructions
   - Link to documentation

## Acceptance Criteria

### Package Quality
- ✅ Packages build successfully for arm64 and armhf
- ✅ No lintian errors or warnings
- ✅ Package size <20MB
- ✅ GPG signatures valid

### Installation
- ✅ Clean install on Debian Bookworm
- ✅ Clean install on Ubuntu 22.04
- ✅ Service auto-starts on install
- ✅ Config file installed with correct permissions
- ✅ User and group created automatically
- ✅ Directories created with correct ownership

### Functionality
- ✅ Watchdog integration functional
- ✅ Process lock prevents double-start
- ✅ Metrics collection works
- ✅ Upload to VictoriaMetrics works
- ✅ Health endpoints respond correctly

### Upgrades
- ✅ Config preserved on upgrade
- ✅ Database migrations run automatically
- ✅ Service restarts gracefully
- ✅ No data loss during upgrade
- ✅ Safe for unattended upgrades

### Removal
- ✅ Service stops on removal
- ✅ Data preserved on remove
- ✅ Complete cleanup on purge
- ✅ No orphaned files or processes

### Testing
- ✅ All Docker integration tests pass
- ✅ Both architectures tested
- ✅ Install/upgrade/remove scenarios covered
- ✅ Functional tests pass

### Documentation
- ✅ Installation guide complete
- ✅ Quick start guide complete
- ✅ Troubleshooting guide complete
- ✅ Upgrade guide complete
- ✅ Man pages created
- ✅ README updated

### Release
- ✅ GitHub Release created
- ✅ All packages uploaded
- ✅ Checksums and signatures included
- ✅ Public GPG key available
- ✅ Release notes published

## Success Metrics

1. **Build Success Rate**: 100% (both architectures build without errors)
2. **Test Pass Rate**: 100% (all integration tests pass)
3. **Install Time**: <30 seconds
4. **Package Size**: <20MB per architecture
5. **Service Start Time**: <5 seconds
6. **Documentation Coverage**: 100% (all features documented)

## Timeline

**Total Estimated Time: 5 days (32-40 hours)**

- **Day 1** (8h): Package infrastructure
- **Day 2** (8h): Build automation
- **Day 3** (10h): Integration (watchdog, locking, config)
- **Day 4** (12h): Documentation and testing
- **Day 5** (4h): Release process

## Risks & Mitigation

### Risk: ARM emulation performance
**Mitigation**: Use native ARM runners if QEMU too slow, or test on subset

### Risk: GPG key management in CI
**Mitigation**: Use GitHub Secrets, document key rotation process

### Risk: Database migration failures
**Mitigation**: Comprehensive testing, backup before migration, rollback procedure

### Risk: Systemd watchdog false positives
**Mitigation**: Conservative timeout (60s), thorough testing, logging

### Risk: Package conflicts with existing tools
**Mitigation**: Unique naming (tidewatch), careful dependency selection

## Post-Milestone

### Future Enhancements (M4+)
- Custom APT repository hosting
- RPM packages for Red Hat/Fedora
- x86_64 packages for testing
- Debian package linting automation
- Multi-distro support matrix
- Automated security updates workflow
- Package performance profiling

### Maintenance
- Regular Debian package updates
- Security vulnerability patching
- Dependency updates
- Documentation improvements
- Community feedback incorporation

---

**Milestone 3 Completion**: All acceptance criteria met, packages available on GitHub Releases, documentation complete, ready for production deployment on ARM devices.
