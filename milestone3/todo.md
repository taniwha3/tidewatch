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

## Phase 2: Build Automation (PARTIALLY COMPLETED)

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

### GitHub Actions Workflow
- [ ] Create `.github/workflows/package.yml`
  - [ ] Add Go build step with cross-compilation
  - [ ] Add nfpm packaging step
  - [ ] Configure arm64 architecture build
  - [ ] Configure armhf architecture build
  - [ ] Add linting/testing before package
  - [ ] Add GPG signing step
  - [ ] Add checksum generation
  - [ ] Upload artifacts to GitHub Releases
- [ ] Test workflow on feature branch
- [ ] Verify packages build for both architectures

### Build Helpers
- [x] Create `scripts/build-deb.sh` helper script
  - [x] Cross-compilation for arm64/armhf
  - [x] nfpm packaging with --config flag
  - [x] SHA256 checksum generation
  - [x] Usage instructions for GPG signing
- [ ] Create `scripts/sign-package.sh` for GPG signing
- [ ] Create `scripts/version.sh` for version extraction
- [ ] Add Makefile targets:
  - [ ] `make package-arm64`
  - [ ] `make package-armhf`
  - [ ] `make package-all`
  - [ ] `make clean-packages`

---

## Phase 3: Watchdog & Process Locking Integration (NOT STARTED)

⚠️ **BLOCKER**: `cmd/tidewatch/main.go` currently does NOT integrate watchdog or lockfile packages.
The systemd service expects `Type=notify` but main.go doesn't send systemd notifications yet.

### Watchdog Integration
- [x] Review `internal/watchdog/watchdog.go` implementation (exists from M2)
- [ ] Integrate watchdog in `cmd/tidewatch/main.go`:
  - [ ] Import `github.com/taniwha3/tidewatch/internal/watchdog`
  - [ ] Initialize watchdog with `watchdog.NewPinger(logger)`
  - [ ] Check `wd.IsEnabled()` and start goroutine if true
  - [ ] Call `wd.NotifyReady()` after initialization
  - [ ] Start `wd.Start(ctx)` goroutine for periodic pings
  - [ ] Call `wd.NotifyStopping()` on shutdown
  - [ ] Handle watchdog errors
- [ ] Test watchdog with systemd in Docker
- [ ] Verify automatic restart on hang/crash
- [ ] Document watchdog configuration

### Process Locking
- [x] Review `internal/lockfile/lockfile.go` implementation (exists from M2)
- [ ] Integrate lockfile in `cmd/tidewatch/main.go`:
  - [ ] Import `github.com/taniwha3/tidewatch/internal/lockfile`
  - [ ] Acquire lock at startup (`/var/lib/tidewatch/tidewatch.lock`)
  - [ ] Handle lock acquisition failure (exit with error)
  - [ ] Release lock on clean shutdown (defer lock.Release())
  - [ ] Stale lock cleanup already handled by postinst script
- [ ] Test double-start prevention
- [ ] Verify lock cleanup on crash
- [ ] Document locking behavior

### Configuration Updates
- [ ] Add watchdog config section to YAML (or document it's auto-detected from systemd)
- [ ] Add lockfile path to config (or use hardcoded `/var/lib/tidewatch/tidewatch.lock`)
- [ ] Update example configs with watchdog settings (if needed)
- [ ] Document interaction between watchdog and locking

---

## Phase 4: Testing Infrastructure

### Docker Test Environment
- [ ] Create `test/docker/Dockerfile.debian-arm64`
  - [ ] Base on debian:bookworm-slim
  - [ ] Install systemd, ca-certificates
  - [ ] Configure for QEMU emulation
- [ ] Create `test/docker/Dockerfile.debian-armhf`
- [ ] Create `test/docker/docker-compose.yml`
  - [ ] Add tidewatch test container
  - [ ] Add VictoriaMetrics container
  - [ ] Configure networking
  - [ ] Mount package for install testing

### Integration Test Suite
- [ ] Create `test/integration/install_test.sh`
  - [ ] Test fresh install
  - [ ] Verify service starts
  - [ ] Verify user/group created
  - [ ] Verify files installed correctly
  - [ ] Verify watchdog functional
  - [ ] Verify metrics collection
  - [ ] Verify VictoriaMetrics push
- [ ] Create `test/integration/upgrade_test.sh`
  - [ ] Test upgrade from previous version
  - [ ] Verify config preserved
  - [ ] Verify data migrated
  - [ ] Verify service restarts
- [ ] Create `test/integration/remove_test.sh`
  - [ ] Test package removal
  - [ ] Verify service stopped
  - [ ] Verify files removed (except conffiles)
- [ ] Create `test/integration/purge_test.sh`
  - [ ] Test complete purge
  - [ ] Verify all files removed
  - [ ] Verify user/group removed
  - [ ] Verify data deleted

### CI Integration
- [ ] Add integration test job to GitHub Actions
- [ ] Configure QEMU for ARM emulation
- [ ] Run tests on both arm64 and armhf
- [ ] Generate test reports
- [ ] Fail build on test failure

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
**Completed:** ~40 (Phase 1 complete, Phase 2 partial)
**In Progress:** Phase 3 (Watchdog & Lockfile Integration)
**Remaining:** ~110

**Current Phase:** Phase 3 - Watchdog & Process Locking Integration
**Status:** Phase 1 ✅ Complete | Phase 2 Partial | Phase 3 Ready to Start

### What's Done:
- ✅ Complete Debian package infrastructure (debian/ directory)
- ✅ nfpm configuration for multi-arch builds
- ✅ Build automation script (scripts/build-deb.sh)
- ✅ Production configuration template
- ✅ Maintainer scripts (postinst, prerm, postrm)
- ✅ Systemd service file with security hardening
- ✅ Package documentation (README.Debian, changelog)

### Next Steps:
1. **CRITICAL**: Integrate watchdog and lockfile in cmd/tidewatch/main.go
2. Create GitHub Actions workflow for automated builds
3. Set up Docker test infrastructure
4. Run integration tests for install/upgrade/remove scenarios
5. Complete remaining documentation
