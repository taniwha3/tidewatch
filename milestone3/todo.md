# Milestone 3: Debian Packaging for ARM Ecosystem

## Overview
Create production-ready Debian packages for tidewatch daemon targeting ARM devices (Orange Pi, etc.)

**Target Architectures:** arm64, armhf
**Package Name:** tidewatch
**Build System:** GitHub Actions + nfpm
**Testing:** Docker + QEMU emulation

---

## Phase 1: Package Infrastructure

### Debian Directory Structure
- [ ] Create `debian/` directory structure
- [ ] Create `debian/control` with package metadata
- [ ] Create `debian/changelog` with initial version
- [ ] Create `debian/copyright` with Apache 2.0 license
- [ ] Create `debian/install` with file mappings
- [ ] Create `debian/conffiles` listing config files

### Maintainer Scripts
- [ ] Create `debian/postinst` script
  - [ ] Add user/group creation (tidewatch:tidewatch)
  - [ ] Add database directory creation
  - [ ] Add systemd daemon-reload
  - [ ] Add service enable/start logic
  - [ ] Add upgrade migration logic
- [ ] Create `debian/prerm` script
  - [ ] Add service stop logic
  - [ ] Add graceful shutdown handling
- [ ] Create `debian/postrm` script
  - [ ] Add service cleanup on remove
  - [ ] Add full purge logic (user, data, logs)
  - [ ] Add systemd daemon-reload
- [ ] Create `debian/preinst` script (if needed)

### Systemd Integration
- [ ] Create `debian/tidewatch.service` unit file
  - [ ] Set Type=notify for watchdog
  - [ ] Configure WatchdogSec=60s
  - [ ] Add security hardening directives
  - [ ] Set resource limits (CPUQuota, MemoryMax)
  - [ ] Configure user/group (tidewatch)
  - [ ] Set working directory (/var/lib/tidewatch)
- [ ] Create `debian/tidewatch.default` environment file
  - [ ] Add TIDEWATCH_CONFIG path
  - [ ] Add TIDEWATCH_LOGLEVEL option
  - [ ] Add TIDEWATCH_WATCHDOG option

### Configuration Files
- [ ] Create example config: `examples/config.yaml`
- [ ] Create production config template: `examples/config.prod.yaml`
- [ ] Create development config: `examples/config.dev.yaml`
- [ ] Document configuration options in comments

---

## Phase 2: Build Automation

### nfpm Configuration
- [ ] Create `nfpm.yaml` in project root
  - [ ] Configure package name (tidewatch)
  - [ ] Set version from git tag/semver
  - [ ] Define architectures (arm64, armhf)
  - [ ] Declare dependencies (systemd, adduser, ca-certificates)
  - [ ] Set recommends (sqlite3)
  - [ ] Set suggests (victoriametrics)
  - [ ] Map files to install paths
  - [ ] Configure scripts (postinst, prerm, postrm)
  - [ ] Define conffiles

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
- [ ] Create `scripts/build-deb.sh` helper script
- [ ] Create `scripts/sign-package.sh` for GPG signing
- [ ] Create `scripts/version.sh` for version extraction
- [ ] Add Makefile targets:
  - [ ] `make package-arm64`
  - [ ] `make package-armhf`
  - [ ] `make package-all`
  - [ ] `make clean-packages`

---

## Phase 3: Watchdog & Process Locking Integration

### Watchdog Integration
- [ ] Review `internal/watchdog/watchdog.go` implementation
- [ ] Integrate watchdog in `cmd/main.go`:
  - [ ] Add watchdog.IsEnabled() check
  - [ ] Start watchdog goroutine
  - [ ] Send periodic pings from main loop
  - [ ] Handle watchdog errors
- [ ] Test watchdog with systemd in Docker
- [ ] Verify automatic restart on hang/crash
- [ ] Document watchdog configuration

### Process Locking
- [ ] Review `internal/lockfile/lockfile.go` implementation
- [ ] Integrate lockfile in `cmd/main.go`:
  - [ ] Acquire lock at startup (`/var/lib/tidewatch/tidewatch.lock`)
  - [ ] Handle lock acquisition failure
  - [ ] Release lock on clean shutdown
  - [ ] Clean up stale locks
- [ ] Test double-start prevention
- [ ] Verify lock cleanup on crash
- [ ] Document locking behavior

### Configuration Updates
- [ ] Add watchdog config section to YAML
- [ ] Add lockfile path to config
- [ ] Update example configs with watchdog settings
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
- [ ] Create `debian/README.Debian`
  - [ ] Package-specific notes
  - [ ] Configuration location
  - [ ] Service management commands
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
- [ ] Maintain `debian/changelog` in Debian format
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

**Total Tasks:** TBD (to be counted after task creation)
**Completed:** 0
**In Progress:** 0
**Remaining:** TBD

**Current Phase:** Phase 1 - Package Infrastructure
**Status:** Planning Complete, Ready to Begin Implementation
