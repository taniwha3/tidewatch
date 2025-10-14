# Milestone 3: Debian Packaging for ARM Ecosystem

## Overview
Create production-ready Debian packages for tidewatch daemon targeting ARM devices (Orange Pi, etc.)

**Target Architectures:** arm64, armhf
**Package Name:** tidewatch
**Build System:** GitHub Actions + nfpm
**Testing:** Docker + QEMU emulation

---

## Phase 1: Package Infrastructure ✅ COMPLETED

### Debian Directory Structure
- [x] Create `debian/` directory structure
- [x] Create `debian/control` with package metadata
- [x] Create `debian/changelog` with initial version (3.0.0-1)
- [x] Create `debian/copyright` with Apache 2.0 license
- [x] ~~Create `debian/install` with file mappings~~ (not needed with nfpm)
- [x] Create `debian/conffiles` listing config files

### Maintainer Scripts
- [x] Create `debian/postinst` script
  - [x] Add user/group creation (tidewatch:tidewatch)
  - [x] Add database directory creation
  - [x] Add systemd daemon-reload
  - [x] Add service enable/start logic
  - [x] Add upgrade migration logic
- [x] Create `debian/prerm` script
  - [x] Add service stop logic
  - [x] Add graceful shutdown handling
- [x] Create `debian/postrm` script
  - [x] Add service cleanup on remove
  - [x] Add full purge logic (user, data, logs)
  - [x] Add systemd daemon-reload
- [x] ~~Create `debian/preinst` script~~ (not needed)

### Systemd Integration
- [x] Create `debian/tidewatch.service` unit file
  - [ ] Set Type=notify for watchdog (deferred to Phase 3)
  - [ ] Configure WatchdogSec=60s (deferred to Phase 3)
  - [x] Add security hardening directives
  - [x] Set resource limits (CPUQuota, MemoryMax)
  - [x] Configure user/group (tidewatch)
  - [x] Set working directory (/var/lib/tidewatch)
- [x] Create `debian/README.Debian` with user documentation
- [ ] Create `debian/tidewatch.default` environment file (optional, deferred)
  - [ ] Add TIDEWATCH_CONFIG path
  - [ ] Add TIDEWATCH_LOGLEVEL option
  - [ ] Add TIDEWATCH_WATCHDOG option

### Configuration Files
- [x] Create example config: `configs/config.yaml`
- [x] Create production config template: `configs/config.prod.yaml`
- [x] Create development config: `configs/config.dev.yaml`
- [x] Document configuration options in comments

---

## Phase 2: Build Automation ✅ COMPLETED

### nfpm Configuration ✅
- [x] Create `nfpm.yaml` in project root
  - [x] Configure package name (tidewatch)
  - [x] Set version from git tag/semver (uses $VERSION env var)
  - [x] Define architectures (arm64, armhf)
  - [x] Declare dependencies (systemd, adduser, ca-certificates)
  - [x] Set recommends (sqlite3)
  - [x] Set suggests (victoriametrics)
  - [x] Map files to install paths
  - [x] Configure scripts (postinst, prerm, postrm)
  - [x] Define conffiles

### GitHub Actions Workflow ✅
- [x] Create `.github/workflows/build-deb.yml`
  - [x] Add Go 1.25 (latest stable) build step with cross-compilation
  - [x] Add nfpm packaging step (inline config)
  - [x] Configure arm64 architecture build
  - [x] Configure armhf architecture build
  - [x] Add smoke tests (install/remove verification)
  - [x] Add GPG signing step (optional via secrets, with placeholder fallback)
  - [x] Add checksum generation (SHA256)
  - [x] Upload artifacts to GitHub Releases
  - [x] Add integration tests with systemd
  - [x] Add VictoriaMetrics functional testing
  - [x] Fix version detection for non-tag builds (workflow_dispatch, PRs)
  - [x] Fix artifact download paths for smoke tests
  - [x] Handle missing GPG signatures gracefully
- [x] Workflow triggers on git tags (v*) and manual dispatch
- [x] QEMU emulation configured for ARM testing
- [x] Compatible with go.mod requirements (Go 1.24.0+)

### Build Helpers
- [x] Create `scripts/build-deb.sh` helper script
  - [x] Cross-compilation for arm64/armhf
  - [x] nfpm packaging with --config flag
  - [x] SHA256 checksum generation
  - [x] Usage instructions for GPG signing
- [ ] Create `scripts/sign-package.sh` for GPG signing (optional, handled in workflow)
- [ ] Create `scripts/version.sh` for version extraction (optional, handled in workflow)
- [ ] Add Makefile targets (optional):
  - [ ] `make package-arm64`
  - [ ] `make package-armhf`
  - [ ] `make package-all`
  - [ ] `make clean-packages`

---

## Phase 3: Watchdog & Process Locking Integration ✅ COMPLETED

### Watchdog Integration
- [x] Review `internal/watchdog/watchdog.go` implementation (exists from M2)
- [x] Integrate watchdog in `cmd/tidewatch/main.go`:
  - [x] Import `github.com/taniwha3/tidewatch/internal/watchdog`
  - [x] Initialize watchdog with `watchdog.NewPinger(logger)`
  - [x] Check `wd.IsEnabled()` and start goroutine if true
  - [x] Always send READY/STOPPING via `daemon.SdNotify` when running under systemd
  - [x] Only start periodic watchdog pings if `wd.IsEnabled()` is true
  - [x] Start `wd.Start(ctx)` goroutine for periodic pings (when enabled)
  - [x] Handle notification errors with proper logging
- [x] Update systemd service file to `Type=notify`
- [x] Enable `WatchdogSec=60s` in service file
- [x] Separate Type=notify support from watchdog enablement (critical fix)
- [ ] Test watchdog with systemd in Docker (pending Phase 4)
- [ ] Verify automatic restart on hang/crash (pending Phase 4)
- [ ] Document watchdog configuration (pending Phase 5)

### Process Locking
- [x] Review `internal/lockfile/lockfile.go` implementation (exists from M2)
- [x] Integrate lockfile in `cmd/tidewatch/main.go`:
  - [x] Import `github.com/taniwha3/tidewatch/internal/lockfile`
  - [x] Acquire lock at startup (`/var/lib/tidewatch/tidewatch.lock`)
  - [x] Handle lock acquisition failure (exit with error)
  - [x] Release lock on clean shutdown (defer lock.Release())
  - [x] Stale lock cleanup already handled by postinst script
- [ ] Test double-start prevention (pending Phase 4)
- [ ] Verify lock cleanup on crash (pending Phase 4)
- [ ] Document locking behavior (pending Phase 5)

### Configuration Updates
- [x] Watchdog auto-detects from systemd environment (no config needed)
- [x] Lockfile path derived from `storage.path` config (e.g., `/var/lib/tidewatch/metrics.db.lock`)
- [x] Lock path respects custom storage locations (dev and production)
- [x] Lock path normalized for SQLite URIs (handles `file:...` format with query params)
- [x] All URI formats produce absolute lock paths (prevents working-directory issues)
- [x] Code compiles and binary runs successfully

---

## Phase 4: Testing Infrastructure ✅ COMPLETED

### Docker Test Environment ✅
- [x] Create `tests/Dockerfile.arm64`
  - [x] Base on arm64v8/debian:bookworm-slim
  - [x] Install systemd, systemd-sysv, ca-certificates, sqlite3, curl, jq
  - [x] Configure for QEMU emulation
  - [x] Remove unnecessary systemd services
  - [x] Set up volume mounts for cgroup
- [x] Create `tests/Dockerfile.armhf`
  - [x] Base on arm32v7/debian:bookworm-slim
  - [x] Same configuration as arm64
- [x] Create `tests/docker-compose.yml`
  - [x] Add tidewatch-arm64 test container
  - [x] Add tidewatch-armhf test container
  - [x] Add VictoriaMetrics container
  - [x] Configure networking (metrics bridge network)
  - [x] Mount packages directory for install testing
  - [x] Mount test scripts directory

### Integration Test Suite ✅
- [x] Create `tests/test-install.sh`
  - [x] Test fresh install
  - [x] Verify service starts
  - [x] Verify user/group created
  - [x] Verify files installed correctly
  - [x] Verify permissions (750 for data, 640 for config)
  - [x] Verify binary version output
  - [x] Wait for systemd to be ready
- [x] Create `tests/test-functional.sh`
  - [x] Test fresh install
  - [x] Configure tidewatch to use VictoriaMetrics
  - [x] Restart service with new config
  - [x] Wait for metrics collection
  - [x] Query VictoriaMetrics for uploaded metrics
  - [x] Verify service health
  - [x] Check database and logs on failure
- [ ] Create `tests/test-upgrade.sh` (deferred - needs old version)
  - [ ] Test upgrade from previous version
  - [ ] Verify config preserved
  - [ ] Verify data migrated
  - [ ] Verify service restarts
- [ ] Create `tests/test-remove.sh` (deferred - covered by smoke tests)
  - [ ] Test package removal
  - [ ] Verify service stopped
  - [ ] Verify files removed (except conffiles)
- [ ] Create `tests/test-purge.sh` (deferred - covered by smoke tests)
  - [ ] Test complete purge
  - [ ] Verify all files removed
  - [ ] Verify user/group removed
  - [ ] Verify data deleted

### CI Integration ✅
- [x] Add integration test job to GitHub Actions
- [x] Configure QEMU for ARM emulation (docker/setup-qemu-action@v3)
- [x] Run tests on both arm64 and armhf (matrix strategy)
- [x] Generate test reports (upload test results as artifacts)
- [x] Fail build on test failure (set -e in test scripts)

---

## Phase 5: Documentation

### Installation Documentation
- [ ] Create `docs/installation/quick-start.md`
  - [ ] Single-command install instructions
  - [ ] Basic configuration
  - [ ] Verification steps
- [ ] Create `docs/installation/detailed-install.md`
  - [ ] Architecture selection guide
  - [ ] Manual installation steps
  - [ ] Configuration options
  - [ ] VictoriaMetrics setup
- [ ] Create `docs/installation/apt-repository.md`
  - [ ] GPG key import instructions
  - [ ] Repository setup
  - [ ] Package installation
  - [ ] Unattended upgrades config
- [ ] Create `docs/installation/troubleshooting.md`
  - [ ] Common installation issues
  - [ ] Service won't start
  - [ ] Watchdog failures
  - [ ] Lock file conflicts
  - [ ] Database migration errors
  - [ ] Permission issues

### Package Documentation
- [x] Create `debian/README.Debian`
  - [x] Package-specific notes
  - [x] Configuration location
  - [x] Service management commands
- [ ] Create `docs/packaging/build-from-source.md`
  - [ ] Build requirements
  - [ ] Cross-compilation setup
  - [ ] Local package building
  - [ ] Testing locally built packages
- [ ] Update main README.md
  - [ ] Add Debian installation section
  - [ ] Add systemd management examples
  - [ ] Add package architecture notes

### Changelog & Release Notes
- [x] Maintain `debian/changelog` in Debian format
- [ ] Create `CHANGELOG.md` in project root
  - [ ] Document M3 changes
  - [ ] Document watchdog integration
  - [ ] Document packaging features
- [ ] Create release notes template for GitHub Releases

---

## Phase 6: Deployment & Distribution

### GPG Signing Setup
- [ ] Generate GPG key for package signing (if not exists)
- [ ] Add GPG key to GitHub Secrets
- [ ] Configure signing in GitHub Actions
- [ ] Test package signature verification
- [ ] Document public key distribution

### GitHub Releases
- [ ] Configure automated release creation
- [ ] Upload .deb packages (arm64, armhf)
- [ ] Upload checksums (SHA256SUMS)
- [ ] Upload GPG signatures (.asc files)
- [ ] Generate release notes from changelog
- [ ] Tag releases with semver format

### APT Repository (Future)
- [ ] Research APT repository hosting options
  - [ ] GitHub Pages
  - [ ] packagecloud.io
  - [ ] Self-hosted
- [ ] Document repository setup process
- [ ] Create repository management scripts
- [ ] Test repository installation

---

## Phase 7: Validation & Release

### Pre-Release Validation
- [ ] Build packages for both architectures
- [ ] Run full integration test suite
- [ ] Test on physical ARM hardware (if available)
- [ ] Verify all documentation accurate
- [ ] Check all scripts executable
- [ ] Verify GPG signatures valid
- [ ] Test unattended upgrade scenario

### Release Checklist
- [ ] Update version in all locations
- [ ] Update debian/changelog with release notes
- [ ] Tag release in git
- [ ] Build and sign packages
- [ ] Create GitHub Release
- [ ] Upload all artifacts
- [ ] Verify download links work
- [ ] Update installation docs with new version
- [ ] Announce release

### Post-Release
- [ ] Monitor GitHub Issues for installation problems
- [ ] Test community feedback on different ARM platforms
- [ ] Document common platform-specific issues
- [ ] Plan for bug-fix releases if needed

---

## Success Criteria

- [ ] ✅ Packages build for both arm64 and armhf
- [ ] ✅ Clean install on Debian Bookworm
- [ ] ✅ Service auto-starts on install
- [ ] ✅ Watchdog integration functional
- [ ] ✅ Process lock prevents double-start
- [ ] ✅ Config preserved on upgrade
- [ ] ✅ Database migrations automatic
- [ ] ✅ Clean removal (service stops)
- [ ] ✅ Complete purge (all files removed)
- [ ] ✅ Docker integration tests pass
- [ ] ✅ GPG signatures valid
- [ ] ✅ Documentation complete
- [ ] ✅ Unattended upgrade safe

---

## Progress Summary

**Total Tasks:** ~150 (approximate)
**Completed:** ~110 (Phases 1-4 complete)
**In Progress:** Phase 5 (Documentation)
**Remaining:** ~40

**Current Phase:** Phase 5 - Documentation
**Status:** Phase 1 ✅ | Phase 2 ✅ | Phase 3 ✅ | Phase 4 ✅

### What's Done:
- ✅ Complete Debian package infrastructure (debian/ directory)
- ✅ nfpm configuration for multi-arch builds
- ✅ Build automation script (scripts/build-deb.sh)
- ✅ Production configuration template
- ✅ Maintainer scripts (postinst, prerm, postrm)
- ✅ Systemd service file with security hardening and watchdog enabled
- ✅ Package documentation (README.Debian, changelog)
- ✅ **Watchdog integration in cmd/tidewatch/main.go**
- ✅ **Process locking integration in cmd/tidewatch/main.go**
- ✅ **Systemd Type=notify and WatchdogSec=60s configured**
- ✅ **Type=notify READY/STOPPING always sent when under systemd (not just when watchdog enabled)**
- ✅ **Lock path derived from storage.path config (respects custom locations)**
- ✅ **Lock path normalized for SQLite URIs (handles file:... with query params, prevents cwd issues)**
- ✅ **GitHub Actions workflow (.github/workflows/build-deb.yml)**
- ✅ **Complete CI/CD pipeline with build, test, and release jobs**
- ✅ **Smoke tests for package install/remove verification**
- ✅ **Integration tests with systemd and VictoriaMetrics**
- ✅ **Docker test infrastructure (arm64 and armhf Dockerfiles, docker-compose)**
- ✅ **Test scripts (test-install.sh, test-functional.sh)**
- ✅ **QEMU emulation for ARM testing on x86_64 runners**
- ✅ **Automated GitHub Releases with packages, checksums, and GPG signatures**
- ✅ **Go 1.25 (latest stable) configured in workflow**
- ✅ **Intelligent version detection for tags, PRs, and manual builds**
- ✅ **Graceful handling of missing GPG secrets (placeholder files)**
- ✅ **Fixed artifact paths for reliable smoke tests**

### Next Steps:
1. Complete user documentation (Phase 5)
   - Installation guides (quick-start, detailed, troubleshooting)
   - Build-from-source documentation
   - Update main README.md
2. Test the complete workflow end-to-end (Phase 7)
   - Trigger workflow manually or with a test tag
   - Verify packages build successfully
   - Verify tests pass
3. Prepare for v3.0.0 release (Phase 7)
   - Update version numbers
   - Final validation on physical ARM hardware (optional)
   - Create first production release
