# Milestone 3 Requirements

## Requirements Gathering Session

### Target Platform & Architecture

**Q: Which Raspberry Pi models are you targeting?**
A: Orange Pi (not Raspberry Pi specifically), but keeping it generic for ARM ecosystem

**Q: Which Debian/Raspbian OS versions should we support?**
A: Keep it generic, not tied to specific versions

**Q: What architecture(s) do we need to build for?**
A: Both arm64 (aarch64) and armhf (32-bit ARM)

### Package Structure & Dependencies

**Q: What should the package be named?**
A: **`tidewatch`** (selected)
- Rejected: belabox-metrics (trademark concerns)
- Considered: edgemetrics, metricsd, armmetrics, sysmetrics
- Rationale: Unique, memorable, no trademark conflicts, good monitoring metaphor

**Q: What runtime dependencies should be declared?**
A: Analysis determined:
- **Required**: systemd, adduser, ca-certificates
- **Recommended**: sqlite3
- **Suggested**: victoriametrics
- Binary will be statically linked (no libc dependency)

**Q: Should we create multiple packages or one monolithic package?**
A: Split into packages:
- `tidewatch` - Main daemon package
- `tidewatch-doc` - Extended documentation (optional)

### Installation & Configuration

**Q: What's the desired installation layout?**
A: Approved layout:
- Binary: `/usr/bin/tidewatch`
- Config: `/etc/tidewatch/`
- Data: `/var/lib/tidewatch/`
- Logs: journald (systemd)
- Docs: `/usr/share/doc/tidewatch/`

**Q: How should configuration be handled?**
A: Config in `/etc/tidewatch/` with examples in `/usr/share/doc/tidewatch/examples/`

**Q: Should the package create the `metrics` user automatically?**
A: Yes, create `tidewatch` user/group (renamed from `metrics` to avoid collisions)
- Strategy: Dynamic UID/GID assignment (system user)

**Q: What should happen on first install vs. upgrade?**
A: Make best decisions:
- First install: Create user, start service, install default config
- Upgrade: Preserve config, migrate database, restart service
- Safe for unattended upgrades

### Service Management

**Q: Should systemd service be enabled by default?**
A: Yes, enable and start on install

**Q: Do you want to integrate the watchdog and process locking now?**
A: Yes, include in M3 with watchdog enabled by default

### Build & Distribution

**Q: How should the package be built?**
A: GitHub Actions with cross-compilation

**Q: Where will packages be hosted?**
A: GitHub Releases (if possible)

**Q: Do you need signed packages?**
A: Yes, GPG signatures in GitHub Actions (if possible)

**Q: Version numbering strategy?**
A: Semantic versioning (semver)

### Testing & Validation

**Q: What level of testing for M3?**
A: Verify package installs/removes cleanly and works through local integration tests, possibly using Docker

**Q: Should we support unattended upgrades?**
A: Yes

### Documentation

**Q: What installation documentation is needed?**
A: Yes to all:
- Quick start guide
- Detailed installation manual
- Troubleshooting guide
- APT repository setup instructions

**Q: Any additional tooling needed?**
A: Work it out (to be determined during implementation)

## Design Decisions

### Package Naming

**Selected: `tidewatch`**

Rationale:
- Unique and memorable
- No trademark conflicts
- Good metaphor (watching the tides = monitoring metrics)
- Brandable
- Works well in all contexts (package, service, binary, user)

### User/Group Naming

**Selected: `tidewatch`**

Rationale:
- Matches package name for consistency
- Avoids collision risk of generic `metrics` name
- Clear ownership attribution
- System account (non-login)

### Architecture Support

**Selected: arm64 and armhf**

Rationale:
- arm64: Modern 64-bit ARM devices (Orange Pi 5, etc.)
- armhf: Older 32-bit ARM devices with hardware float
- Covers broad ARM ecosystem
- No x86_64 for M3 (embedded/edge focus)

### Build System

**Selected: GitHub Actions with nfpm**

Rationale:
- Cross-compilation from any platform
- nfpm is modern, Go-based package builder
- Supports .deb, .rpm, .apk from single config
- Easy CI/CD integration
- No need for native ARM build environment

### Testing Strategy

**Selected: Docker with QEMU emulation**

Rationale:
- Fast iteration on x86_64 runners
- Test both architectures without hardware
- Reproducible test environment
- Integration testing with VictoriaMetrics
- Verify install/upgrade/remove scenarios

### Package Split

**Selected: Main + Documentation packages**

Rationale:
- Keep main package lean
- Optional extended docs for detailed troubleshooting
- Follows Debian conventions
- Users can choose installation footprint

## Technical Requirements

### Debian Package Structure

```
tidewatch/
├── debian/
│   ├── control              # Package metadata
│   ├── rules                # Build instructions
│   ├── changelog            # Version history
│   ├── copyright            # License info
│   ├── install              # File mappings
│   ├── postinst             # Post-install script
│   ├── prerm                # Pre-removal script
│   ├── postrm               # Post-removal script
│   ├── tidewatch.service    # Systemd unit
│   ├── tidewatch.default    # Environment defaults
│   └── conffiles            # Config file list
```

### Dependencies

**Build Dependencies:**
- Go 1.21+
- dpkg-dev
- debhelper (>= 13)
- nfpm (or dpkg-deb)

**Runtime Dependencies:**
```
Depends: systemd, adduser, ca-certificates
Recommends: sqlite3
Suggests: victoriametrics
```

### Integration Points

1. **Watchdog Integration** (from M2)
   - Package: `internal/watchdog`
   - Integration: `cmd/main.go`
   - Configuration: `WatchdogSec=60s` in service file

2. **Process Locking** (from M2)
   - Package: `internal/lockfile`
   - Integration: `cmd/main.go`
   - Lock file: `/var/lib/tidewatch/tidewatch.lock`

3. **Database Migrations** (from M2)
   - Automatic on upgrade via postinst
   - Backup before migration
   - Schema version tracking

### Security Requirements

- Non-root execution (tidewatch user)
- Systemd hardening (from M2):
  - NoNewPrivileges=true
  - ProtectSystem=strict
  - ProtectHome=true
  - MemoryMax=200M
  - CPUQuota=20%
  - RestrictAddressFamilies
  - RestrictNamespaces
- Watchdog for health monitoring
- Process lock for single-instance

### Compatibility Requirements

- Debian 11 (Bullseye) and newer
- Ubuntu 20.04 LTS and newer
- Raspbian OS (generic Debian ARM)
- Orange Pi OS
- Generic ARMv7 and ARMv8 systems

### Performance Requirements

- Package size: <20MB
- Install time: <30 seconds
- Upgrade time: <1 minute (including migration)
- Service start time: <5 seconds

## Success Criteria

1. ✅ Packages build for both arm64 and armhf
2. ✅ Clean install on Debian Bookworm
3. ✅ Service auto-starts on install
4. ✅ Watchdog integration functional
5. ✅ Process lock prevents double-start
6. ✅ Config preserved on upgrade
7. ✅ Database migrations automatic
8. ✅ Clean removal (service stops)
9. ✅ Complete purge (all files removed)
10. ✅ Docker integration tests pass
11. ✅ GPG signatures valid
12. ✅ Documentation complete
13. ✅ Unattended upgrade safe
