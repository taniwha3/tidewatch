# Changelog

All notable changes to Tidewatch will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [3.0.0] - 2025-01-XX

### Added - Milestone 3: Debian Packaging

#### Packaging
- Debian package infrastructure with `debian/` directory
- Production-ready `.deb` packages for amd64, arm64, and armhf architectures
- nfpm-based packaging with consistent configuration
- Automated build script (`scripts/build-deb.sh`)
- GPG package signing support (optional via GitHub Secrets)
- SHA256 checksum generation for all packages
- Package metadata: dependencies, recommendations, suggestions

#### Systemd Integration
- Systemd watchdog support with 60-second timeout
- Type=notify for proper startup signaling
- Service auto-starts on installation
- Automatic daemon reload on package operations
- Extensive security hardening directives:
  - Dedicated `tidewatch` system user (no login shell)
  - Read-only system filesystem
  - Private /tmp isolation
  - Network socket restrictions (AF_UNIX, AF_INET, AF_INET6)
  - System call filtering
  - Capability dropping
- Resource limits: CPU quota 20%, Memory max 200M

#### Process Management
- File-based process locking to prevent double-start
- Lock file location derived from storage path configuration
- Stale lock cleanup in postinst script
- Lock path normalization for SQLite URIs (handles `file:...` format)
- Graceful shutdown with lock release

#### Database & Storage
- Automatic database migrations on package upgrade
- SQLite URI format support with query parameters
- Configurable storage paths with lock file co-location
- Database directory auto-creation with proper permissions

#### Configuration Management
- Production default config at `/etc/tidewatch/config.yaml`
- Example configs in `/usr/share/doc/tidewatch/examples/`
- Config marked as conffile (preserved on upgrade)
- Config merge prompts on conflicts

#### Installation Scripts
- `postinst`: User/group creation, directory setup, service enablement
- `prerm`: Service stop and disable
- `postrm`: Cleanup on remove, complete purge on purge
- Systemd presence detection (graceful degradation without systemd)

#### CI/CD Pipeline
- GitHub Actions workflow (`.github/workflows/build-deb.yml`)
- Multi-architecture build matrix (amd64, arm64, armhf)
- QEMU emulation for ARM builds on x86_64 runners
- Smoke tests: package install/remove verification
- Integration tests with systemd-in-Docker
- Functional tests with VictoriaMetrics
- Automated GitHub Releases on git tags
- Intelligent version detection for tags, PRs, and manual builds
- Graceful handling of missing GPG secrets (placeholder mode)

#### Testing Infrastructure
- Docker test environment with systemd support
- Dockerfiles for all architectures: `tests/Dockerfile.amd64`, `tests/Dockerfile.arm64`, `tests/Dockerfile.armhf`
- `tests/docker-compose.yml` with VictoriaMetrics integration
- `tests/test-install.sh`: Installation verification
- `tests/test-functional.sh`: End-to-end metrics collection test
- Systemd-in-Docker best practices (cgroup management, signal handling)
- Test configuration with proper metrics format
- VictoriaMetrics query retry logic for indexing delays

#### Documentation
- Quick-start installation guide (`docs/installation/quick-start.md`)
- Detailed installation guide (`docs/installation/detailed-install.md`)
- Comprehensive troubleshooting guide (`docs/installation/troubleshooting.md`)
- Build-from-source documentation (`docs/packaging/build-from-source.md`)
- Updated main README with Debian installation section
- Milestone 3 specification and todo tracking (`docs/milestone3/`)

### Changed - Milestone 3

#### Application Code
- Updated `cmd/tidewatch/main.go` with watchdog integration
- Updated `cmd/tidewatch/main.go` with process locking integration
- Lockfile path derived from `storage.path` config (respects custom locations)
- Systemd READY/STOPPING notifications always sent when under systemd
- Watchdog periodic pings only when `WATCHDOG_USEC` is set

#### Build System
- Go version updated to 1.25 (latest stable) in CI
- Cross-compilation for amd64 (GOARCH=amd64), arm64 (GOARCH=arm64), and armhf (GOARCH=arm, GOARM=7)
- Build flags include version via ldflags: `-X main.appVersion`
- Static binaries with CGO_ENABLED=0
- Binary stripping with `-s -w` ldflags

#### Configuration
- Default storage path: `/var/lib/tidewatch/metrics.db`
- Default config path: `/etc/tidewatch/config.yaml`
- Lock file location: `<storage.path>.lock` (e.g., `/var/lib/tidewatch/metrics.db.lock`)

#### Testing
- Debian version validation (must start with digit for dpkg)
- Docker Compose V2 compatibility (`docker compose` vs `docker-compose`)
- URL-encoded VictoriaMetrics query strings
- Health-aware container wait logic (not fixed sleep)

### Fixed - Milestone 3

#### Packaging
- nfpm.yaml syntax: `deb.fields` at top level (not under `deb`)
- ldflags target: `main.appVersion` (not `main.version` bool)
- Architecture detection in test scripts
- Artifact download paths in CI smoke tests

#### Systemd
- Type=notify support separate from watchdog enablement
- READY/STOPPING notifications sent regardless of watchdog state
- Watchdog pings only when explicitly enabled via `WATCHDOG_USEC`

#### CI/CD
- Version detection for non-tag builds (workflow_dispatch, PRs)
- Missing GPG signature handling (creates placeholder files)
- Package path consistency between build and test jobs
- Systemd container initialization (ENV container=docker, proper masking)
- QEMU+seccomp conflicts (use seccomp=unconfined)
- VictoriaMetrics indexing delay handling (retry with exponential backoff)

## [2.0.0] - 2025-01-XX

### Added - Milestone 2: System Metrics & Reliability

#### Metrics Collection
- CPU usage metrics (overall and per-core with delta calculation)
- Memory metrics (used, available, total, swap)
- Disk I/O metrics (read/write ops, bytes, time per device)
- Network metrics (rx/tx bytes, packets, errors per interface)
- Temperature metrics (per thermal zone)
- Meta-metrics for observability (collection, upload, storage stats)
- Clock skew detection and monitoring

#### VictoriaMetrics Integration
- JSONL import format support
- Chunked uploads with configurable batch and chunk sizes
- Jittered exponential backoff for retries
- Duplicate metric detection and prevention

#### Health Monitoring
- Graduated health status (ok/degraded/error)
- HTTP health endpoints at `/health` and `/metrics`
- Health status based on upload success and collector errors

#### Logging
- Structured logging with JSON and console formats
- Configurable log levels (debug, info, warn, error)
- Contextual fields in all log entries

### Changed - Milestone 2
- Upload reliability improvements
- Storage layer optimizations
- Configuration schema updates for new collectors

## [1.0.0] - 2025-01-XX

### Added - Milestone 1: MVP

#### Core Functionality
- SQLite storage with WAL mode
- HTTP POST upload to remote endpoint
- YAML configuration support
- Cross-compilation for ARM64
- Systemd service integration

#### Metrics
- CPU temperature collector
- Mock SRT packet loss collector

#### Deployment
- Install script for ARM devices
- Systemd service file with auto-restart
- Basic health monitoring

#### Documentation
- README with quick start guide
- Milestone 1 specification
- Product requirements document (PRD)

---

## Version History

- **3.0.0**: Debian packaging, systemd watchdog, process locking
- **2.0.0**: System metrics, VictoriaMetrics, reliability improvements
- **1.0.0**: Initial MVP release

## Links

- [GitHub Repository](https://github.com/taniwha3/tidewatch)
- [GitHub Releases](https://github.com/taniwha3/tidewatch/releases)
- [Documentation](https://github.com/taniwha3/tidewatch/tree/main/docs)
- [Issue Tracker](https://github.com/taniwha3/tidewatch/issues)
