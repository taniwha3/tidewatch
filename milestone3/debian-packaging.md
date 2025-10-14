# Debian Packaging Technical Reference

## Overview

This document provides comprehensive technical guidance for packaging the tidewatch daemon as a Debian package for ARM architectures. It serves as a reference for understanding Debian packaging conventions, best practices, and implementation details specific to this project.

---

## Table of Contents

1. [Debian Package Fundamentals](#debian-package-fundamentals)
2. [Package Structure](#package-structure)
3. [Build Systems: nfpm vs dpkg-deb](#build-systems-nfpm-vs-dpkg-deb)
4. [Maintainer Scripts](#maintainer-scripts)
5. [Configuration File Management](#configuration-file-management)
6. [Systemd Integration](#systemd-integration)
7. [User and Group Management](#user-and-group-management)
8. [Cross-Compilation for ARM](#cross-compilation-for-arm)
9. [Testing and Validation](#testing-and-validation)
10. [Common Pitfalls and Solutions](#common-pitfalls-and-solutions)

---

## Debian Package Fundamentals

### Package Lifecycle

A Debian package goes through several states during its lifecycle:

1. **Install** - Initial installation on a system
2. **Upgrade** - Replacing an older version
3. **Remove** - Removing the package (keeping configuration)
4. **Purge** - Complete removal including configuration
5. **Reinstall** - Installing the same version again

Each state triggers different maintainer scripts with specific environment variables.

### Package Naming Convention

Format: `<name>_<version>-<revision>_<architecture>.deb`

For tidewatch:
- `tidewatch_0.3.0-1_arm64.deb`
- `tidewatch_0.3.0-1_armhf.deb`

Where:
- `tidewatch` - Package name
- `0.3.0` - Upstream version (semantic versioning)
- `1` - Debian package revision
- `arm64` / `armhf` - Target architecture

### Semantic Versioning

Tidewatch uses semantic versioning (semver):

```
MAJOR.MINOR.PATCH
  │     │     │
  │     │     └─ Bug fixes, patches
  │     └─────── New features (backward compatible)
  └───────────── Breaking changes
```

Examples:
- `0.3.0` - Milestone 3 release (new packaging features)
- `0.3.1` - Bug fix release
- `0.4.0` - Milestone 4 with new features
- `1.0.0` - First stable release

---

## Package Structure

### Directory Layout

```
tidewatch/
├── debian/
│   ├── control              # Package metadata and dependencies
│   ├── rules                # Build instructions (if using debhelper)
│   ├── changelog            # Version history (Debian format)
│   ├── copyright            # License information
│   ├── install              # File installation mappings
│   ├── conffiles            # List of configuration files
│   ├── postinst             # Post-installation script
│   ├── preinst              # Pre-installation script (optional)
│   ├── prerm                # Pre-removal script
│   ├── postrm               # Post-removal script
│   ├── tidewatch.service    # Systemd unit file
│   ├── tidewatch.default    # Environment defaults
│   └── README.Debian        # Package-specific notes
├── nfpm.yaml                # nfpm build configuration
└── examples/
    ├── config.yaml          # Example configuration
    ├── config.prod.yaml     # Production configuration
    └── config.dev.yaml      # Development configuration
```

### debian/control

The control file defines package metadata:

```
Source: tidewatch
Section: admin
Priority: optional
Maintainer: Your Name <your.email@example.com>
Build-Depends: golang (>= 1.21), debhelper-compat (= 13)
Standards-Version: 4.6.2
Homepage: https://github.com/yourusername/tidewatch
Vcs-Browser: https://github.com/yourusername/tidewatch
Vcs-Git: https://github.com/yourusername/tidewatch.git

Package: tidewatch
Architecture: arm64 armhf
Depends: ${misc:Depends}, systemd, adduser, ca-certificates
Recommends: sqlite3
Suggests: victoriametrics
Description: System metrics collection daemon for edge devices
 Tidewatch is a lightweight metrics collector designed for ARM-based
 edge devices like Orange Pi. It collects system metrics (CPU, memory,
 disk, network) and exports them to VictoriaMetrics or other endpoints.
 .
 Features include:
  - Low resource footprint (<10MB memory, <1% CPU)
  - Systemd watchdog integration for reliability
  - SQLite-based local buffering with deduplication
  - Automatic remote endpoint discovery
  - Configurable collection intervals

Package: tidewatch-doc
Architecture: all
Section: doc
Depends: ${misc:Depends}
Description: Documentation for tidewatch metrics daemon
 Extended documentation for the tidewatch system metrics collector,
 including architecture guides, troubleshooting, and API references.
```

**Key Fields:**
- `Depends` - Hard dependencies (installation will fail without these)
- `Recommends` - Soft dependencies (APT will suggest installing)
- `Suggests` - Optional enhancements
- `Architecture` - Supported architectures (`arm64 armhf` or `all` for arch-independent)

### debian/changelog

Debian changelog format (strict syntax):

```
tidewatch (0.3.0-1) stable; urgency=medium

  * Initial Debian package release for Milestone 3
  * Add systemd watchdog integration
  * Add process locking to prevent double-start
  * Implement automatic database migrations
  * Add comprehensive systemd security hardening

 -- Your Name <your.email@example.com>  Mon, 13 Jan 2025 12:00:00 -0800

tidewatch (0.2.0-1) stable; urgency=low

  * Milestone 2 release
  * Add database deduplication
  * Implement checkpoint tracking
  * Add configuration file support

 -- Your Name <your.email@example.com>  Mon, 06 Jan 2025 12:00:00 -0800
```

**Format Rules:**
- First line: `package (version) distribution; urgency=level`
- Changes indented with 2 spaces, starting with `*`
- Signature line starts with ` --` (space+dash+dash), includes timestamp in RFC 2822 format
- Blank line between entries

Generate with: `dch --newversion 0.3.0-1 "Initial Debian package release"`

### debian/install

Maps source files to installation paths:

```
bin/tidewatch            /usr/bin/
examples/config.yaml     /usr/share/doc/tidewatch/examples/
examples/config.prod.yaml /usr/share/doc/tidewatch/examples/
examples/config.dev.yaml /usr/share/doc/tidewatch/examples/
```

Format: `<source> <destination>`

### debian/conffiles

Lists files protected by dpkg conffile mechanism:

```
/etc/tidewatch/config.yaml
```

**Conffile Behavior:**
- On upgrade: If modified by user, dpkg prompts for action
- On remove: Files preserved
- On purge: Files deleted

---

## Build Systems: nfpm vs dpkg-deb

### nfpm (Recommended)

**nfpm** (NFpm is not FPM) is a modern, Go-based package builder.

**Advantages:**
- Single YAML config for multiple formats (.deb, .rpm, .apk)
- Cross-compilation friendly
- CI/CD integration (GitHub Actions)
- Simpler than traditional dpkg-buildpackage
- No need for native build environment

**Disadvantages:**
- Less control over advanced Debian features
- Smaller community than debhelper

**Example nfpm.yaml:**

```yaml
name: tidewatch
arch: ${ARCH}
platform: linux
version: ${VERSION}
section: admin
priority: optional
maintainer: Your Name <your.email@example.com>
description: |
  System metrics collection daemon for edge devices
  Tidewatch collects system metrics and exports to VictoriaMetrics.
vendor: Your Organization
homepage: https://github.com/yourusername/tidewatch
license: Apache-2.0

depends:
  - systemd
  - adduser
  - ca-certificates

recommends:
  - sqlite3

suggests:
  - victoriametrics

contents:
  # Binary
  - src: ./bin/tidewatch
    dst: /usr/bin/tidewatch
    file_info:
      mode: 0755

  # Configuration
  - src: ./examples/config.yaml
    dst: /etc/tidewatch/config.yaml
    type: config
    file_info:
      mode: 0644

  # Examples
  - src: ./examples/*.yaml
    dst: /usr/share/doc/tidewatch/examples/
    file_info:
      mode: 0644

  # Systemd unit
  - src: ./debian/tidewatch.service
    dst: /lib/systemd/system/tidewatch.service
    file_info:
      mode: 0644

  # Environment defaults
  - src: ./debian/tidewatch.default
    dst: /etc/default/tidewatch
    type: config
    file_info:
      mode: 0644

  # Data directory (empty)
  - dst: /var/lib/tidewatch
    type: dir
    file_info:
      mode: 0755
      owner: tidewatch
      group: tidewatch

scripts:
  preinstall: ./debian/preinst
  postinstall: ./debian/postinst
  preremove: ./debian/prerm
  postremove: ./debian/postrm

overrides:
  deb:
    fields:
      Vcs-Browser: https://github.com/yourusername/tidewatch
      Vcs-Git: https://github.com/yourusername/tidewatch.git
```

**Building with nfpm:**

```bash
# Install nfpm
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

# Build package
export VERSION=0.3.0
export ARCH=arm64
nfpm package --packager deb --target tidewatch_${VERSION}_${ARCH}.deb
```

### dpkg-deb / debhelper

Traditional Debian build system using `dpkg-buildpackage`.

**Advantages:**
- Full control over Debian packaging features
- Large community and documentation
- Preferred for official Debian packages

**Disadvantages:**
- More complex setup
- Requires debian/ directory with many files
- Harder to cross-compile
- Requires native build environment

**debian/rules (Makefile):**

```makefile
#!/usr/bin/make -f

export DH_VERBOSE = 1
export GOPATH = $(CURDIR)/_build

%:
	dh $@

override_dh_auto_build:
	GOOS=linux GOARCH=arm64 go build -o bin/tidewatch cmd/main.go

override_dh_auto_install:
	install -D -m 0755 bin/tidewatch $(CURDIR)/debian/tidewatch/usr/bin/tidewatch
	install -D -m 0644 debian/tidewatch.service $(CURDIR)/debian/tidewatch/lib/systemd/system/tidewatch.service
```

**Building with debhelper:**

```bash
# Install build dependencies
apt-get install debhelper golang

# Build package
dpkg-buildpackage -us -uc -b -aarm64
```

### Comparison Summary

| Feature | nfpm | debhelper |
|---------|------|-----------|
| Setup Complexity | Low | High |
| Cross-Compilation | Easy | Hard |
| CI/CD Integration | Excellent | Moderate |
| Multi-Format Support | Yes (.deb, .rpm, .apk) | No (Debian only) |
| Community | Growing | Mature |
| Control | Moderate | Full |
| **Recommendation** | **Production use** | Debian official packages |

**Decision for tidewatch:** Use **nfpm** for easier CI/CD and cross-compilation.

---

## Maintainer Scripts

Maintainer scripts control package behavior during lifecycle events.

### Script Execution Order

#### Install (Fresh)
```
1. preinst install
2. [files extracted]
3. postinst configure
```

#### Upgrade
```
1. preinst upgrade <old-version>
2. [files extracted]
3. postinst configure <old-version>
```

#### Remove
```
1. prerm remove
2. [files removed, conffiles kept]
3. postrm remove
```

#### Purge
```
1. postrm purge
```

### Environment Variables

Scripts receive arguments and can check `$DPKG_MAINTSCRIPT_PACKAGE`:

```bash
#!/bin/bash
set -e

case "$1" in
    configure)
        echo "Configuring $DPKG_MAINTSCRIPT_PACKAGE"
        echo "Previous version: $2"
        ;;
esac
```

### debian/preinst

Runs before package installation.

**Use cases:**
- Stop services before upgrade (rare)
- Check prerequisites
- Backup critical data

**Example:**

```bash
#!/bin/bash
set -e

case "$1" in
    install|upgrade)
        # Check if we're upgrading from a very old version
        if [ -n "$2" ] && dpkg --compare-versions "$2" lt "0.2.0"; then
            echo "Upgrading from pre-0.2.0 version, checking compatibility..."
            # Perform compatibility checks
        fi
        ;;

    abort-upgrade)
        # Rollback preinst changes if upgrade fails
        ;;
esac

exit 0
```

**Best Practices:**
- Keep minimal (most work goes in postinst)
- Always `set -e` for error handling
- Handle abort-upgrade for rollback

### debian/postinst

Runs after package installation. **Most important script.**

**Use cases:**
- Create users/groups
- Set permissions
- Start services
- Run migrations
- Update system configuration

**Complete Example:**

```bash
#!/bin/bash
set -e

PACKAGE="tidewatch"
USER="tidewatch"
GROUP="tidewatch"
DATADIR="/var/lib/tidewatch"
LOCKFILE="${DATADIR}/tidewatch.lock"
DBFILE="${DATADIR}/metrics.db"

case "$1" in
    configure)
        # Create system user and group
        if ! getent group "$GROUP" > /dev/null 2>&1; then
            addgroup --system "$GROUP" || true
        fi

        if ! getent passwd "$USER" > /dev/null 2>&1; then
            adduser --system --home "$DATADIR" --no-create-home \
                    --ingroup "$GROUP" --disabled-password \
                    --shell /bin/false "$USER" || true
        fi

        # Create data directory
        if [ ! -d "$DATADIR" ]; then
            mkdir -p "$DATADIR"
        fi

        # Set ownership and permissions
        chown -R "${USER}:${GROUP}" "$DATADIR"
        chmod 750 "$DATADIR"

        # Remove stale lock files from crash
        if [ -f "$LOCKFILE" ]; then
            # Check if process still running
            if [ -r "$LOCKFILE" ]; then
                PID=$(cat "$LOCKFILE" 2>/dev/null || echo "")
                if [ -n "$PID" ] && ! kill -0 "$PID" 2>/dev/null; then
                    echo "Removing stale lock file"
                    rm -f "$LOCKFILE"
                fi
            fi
        fi

        # Run database migrations on upgrade
        if [ -n "$2" ]; then
            echo "Upgrading from version $2 to $(dpkg-query -W -f='${Version}' $PACKAGE)"

            # Backup database before migration
            if [ -f "$DBFILE" ]; then
                BACKUP="${DBFILE}.backup-$(date +%Y%m%d-%H%M%S)"
                cp "$DBFILE" "$BACKUP"
                echo "Database backed up to: $BACKUP"

                # Run migration (daemon will handle on startup)
                # We don't run the daemon here, systemd will start it
            fi
        fi

        # Reload systemd configuration
        if [ -d /run/systemd/system ]; then
            systemctl daemon-reload || true

            # Enable service to start on boot
            systemctl enable "$PACKAGE.service" || true

            # Start or restart service
            if systemctl is-active --quiet "$PACKAGE.service"; then
                echo "Restarting $PACKAGE service..."
                systemctl restart "$PACKAGE.service" || true
            else
                echo "Starting $PACKAGE service..."
                systemctl start "$PACKAGE.service" || true
            fi
        fi

        # Show status
        if systemctl is-active --quiet "$PACKAGE.service"; then
            echo "$PACKAGE is running"
        else
            echo "Warning: $PACKAGE failed to start, check 'systemctl status $PACKAGE'"
        fi
        ;;

    abort-upgrade|abort-remove|abort-deconfigure)
        # Rollback changes if needed
        ;;
esac

exit 0
```

**Best Practices:**
- Check for systemd availability before using systemctl
- Use `|| true` to prevent script failure on non-critical commands
- Backup data before migrations
- Print helpful messages for users
- Handle upgrade vs fresh install differently

### debian/prerm

Runs before package removal.

**Use cases:**
- Stop services gracefully
- Notify users
- Backup data (on purge)

**Example:**

```bash
#!/bin/bash
set -e

PACKAGE="tidewatch"

case "$1" in
    remove|deconfigure)
        # Stop the service
        if [ -d /run/systemd/system ]; then
            echo "Stopping $PACKAGE service..."
            systemctl stop "$PACKAGE.service" || true

            # Disable on remove (but not upgrade)
            if [ "$1" = "remove" ]; then
                systemctl disable "$PACKAGE.service" || true
            fi
        fi
        ;;

    upgrade)
        # On upgrade, systemd will handle service restart via postinst
        # We just ensure clean shutdown
        if [ -d /run/systemd/system ]; then
            systemctl stop "$PACKAGE.service" || true
        fi
        ;;

    failed-upgrade)
        # Do nothing, let postinst handle recovery
        ;;
esac

exit 0
```

**Best Practices:**
- Graceful shutdown (give process time to cleanup)
- Don't fail if service already stopped
- Distinguish between remove and upgrade

### debian/postrm

Runs after package removal.

**Use cases:**
- Clean up service state
- Remove users/groups (on purge)
- Delete data directories (on purge)

**Example:**

```bash
#!/bin/bash
set -e

PACKAGE="tidewatch"
USER="tidewatch"
GROUP="tidewatch"
DATADIR="/var/lib/tidewatch"
CONFDIR="/etc/tidewatch"

case "$1" in
    remove)
        # Files removed, conffiles kept
        # Just cleanup systemd state
        if [ -d /run/systemd/system ]; then
            systemctl daemon-reload || true
        fi
        ;;

    purge)
        # Complete removal - remove everything

        # Remove data directory
        if [ -d "$DATADIR" ]; then
            echo "Removing data directory: $DATADIR"
            rm -rf "$DATADIR"
        fi

        # Remove configuration directory
        if [ -d "$CONFDIR" ]; then
            echo "Removing configuration: $CONFDIR"
            rm -rf "$CONFDIR"
        fi

        # Remove user and group
        if getent passwd "$USER" > /dev/null 2>&1; then
            echo "Removing user: $USER"
            deluser --quiet --system "$USER" || true
        fi

        if getent group "$GROUP" > /dev/null 2>&1; then
            echo "Removing group: $GROUP"
            delgroup --quiet --system "$GROUP" || true
        fi

        # Reload systemd
        if [ -d /run/systemd/system ]; then
            systemctl daemon-reload || true
            systemctl reset-failed || true
        fi

        echo "$PACKAGE has been completely removed"
        ;;

    upgrade|failed-upgrade|abort-install|abort-upgrade|disappear)
        # Do nothing
        ;;
esac

exit 0
```

**Best Practices:**
- Only remove data on purge, not remove
- Check for existence before deleting
- Clean up systemd state
- Print confirmation messages

### Script Testing

Test all lifecycle scenarios:

```bash
# Fresh install
dpkg -i tidewatch_0.3.0_arm64.deb
systemctl status tidewatch
dpkg -L tidewatch | head -20

# Upgrade
dpkg -i tidewatch_0.3.1_arm64.deb
systemctl status tidewatch

# Remove (keep config)
dpkg -r tidewatch
ls /etc/tidewatch  # Should exist
ls /var/lib/tidewatch  # Should NOT exist

# Reinstall
dpkg -i tidewatch_0.3.1_arm64.deb
cat /etc/tidewatch/config.yaml  # Should have user modifications

# Purge (complete removal)
dpkg -P tidewatch
ls /etc/tidewatch  # Should NOT exist
getent passwd tidewatch  # Should NOT exist
```

---

## Configuration File Management

### Conffile Mechanism

Debian's conffile system protects user-modified configuration files.

**Behavior:**
1. **First Install:** Config installed from package
2. **User Modifies:** User edits `/etc/tidewatch/config.yaml`
3. **Upgrade with New Config:** dpkg detects conflict and prompts:

```
Configuration file '/etc/tidewatch/config.yaml'
 ==> Modified (by you or by a script) since installation.
 ==> Package distributor has shipped an updated version.
   What would you like to do about it ?  Your options are:
    Y or I  : install the package maintainer's version
    N or O  : keep your currently-installed version
      D     : show the differences between the versions
      Z     : start a shell to examine the situation
 The default action is to keep your current version.
*** config.yaml (Y/I/N/O/D/Z) [default=N] ?
```

**Marking as Conffile:**

In nfpm:
```yaml
contents:
  - src: ./examples/config.yaml
    dst: /etc/tidewatch/config.yaml
    type: config  # Marks as conffile
```

In debian/conffiles:
```
/etc/tidewatch/config.yaml
```

### Best Practices

1. **Ship Sensible Defaults**
   - Config should work out-of-box
   - Include comments explaining all options
   - Provide examples in `/usr/share/doc/tidewatch/examples/`

2. **Avoid Breaking Changes**
   - Maintain backward compatibility
   - Add new options with defaults
   - Deprecate old options gracefully

3. **Migration Strategy**
   ```bash
   # In postinst during upgrade
   if [ -f /etc/tidewatch/config.yaml ]; then
       # Check for old config format
       if grep -q "old_option:" /etc/tidewatch/config.yaml; then
           echo "WARNING: Deprecated option 'old_option' detected"
           echo "Please update to 'new_option' (see documentation)"
       fi
   fi
   ```

4. **Configuration Validation**
   ```bash
   # Validate config before starting service
   if ! /usr/bin/tidewatch --validate-config /etc/tidewatch/config.yaml; then
       echo "ERROR: Invalid configuration file"
       exit 1
   fi
   ```

---

## Systemd Integration

### Service Unit File

**debian/tidewatch.service:**

```ini
[Unit]
Description=Tidewatch Metrics Collection Daemon
Documentation=https://github.com/yourusername/tidewatch
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
User=tidewatch
Group=tidewatch

# Binary and configuration
ExecStart=/usr/bin/tidewatch --config /etc/tidewatch/config.yaml
ExecReload=/bin/kill -HUP $MAINPID

# Working directory
WorkingDirectory=/var/lib/tidewatch

# Restart policy
Restart=on-failure
RestartSec=5s
StartLimitInterval=200s
StartLimitBurst=3

# Watchdog (60 second timeout)
WatchdogSec=60s
NotifyAccess=main

# Resource limits
MemoryMax=200M
MemoryHigh=150M
CPUQuota=20%
TasksMax=50

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/tidewatch
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=true
LockPersonality=true
RestrictRealtime=true
RestrictSUIDSGID=true
RemoveIPC=true
PrivateMounts=true
SystemCallArchitectures=native
SystemCallFilter=@system-service
SystemCallFilter=~@privileged @resources

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tidewatch

[Install]
WantedBy=multi-user.target
```

### Type=notify and Watchdog

**Type=notify** requires the daemon to signal readiness:

```go
// internal/watchdog/watchdog.go
package watchdog

import (
    "fmt"
    "os"
    "time"
)

// NotifyReady signals systemd that service is ready
func NotifyReady() error {
    return notify("READY=1")
}

// NotifyWatchdog sends watchdog keepalive ping
func NotifyWatchdog() error {
    return notify("WATCHDOG=1")
}

// NotifyStatus sends status message to systemd
func NotifyStatus(status string) error {
    return notify(fmt.Sprintf("STATUS=%s", status))
}

func notify(state string) error {
    socketPath := os.Getenv("NOTIFY_SOCKET")
    if socketPath == "" {
        return fmt.Errorf("NOTIFY_SOCKET not set")
    }

    // Send notification via Unix socket
    // Implementation details omitted for brevity
    return nil
}
```

**Usage in main.go:**

```go
func main() {
    // Initialize
    db := initDatabase()
    collector := initCollector()

    // Signal ready to systemd
    watchdog.NotifyReady()
    watchdog.NotifyStatus("Collecting metrics")

    // Start watchdog ticker
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for range ticker.C {
            watchdog.NotifyWatchdog()
        }
    }()

    // Main loop
    collector.Run()
}
```

### Security Hardening Directives

| Directive | Effect | Rationale |
|-----------|--------|-----------|
| `NoNewPrivileges=true` | Prevents privilege escalation | Blocks setuid binaries |
| `ProtectSystem=strict` | Read-only system directories | Prevents system modification |
| `ProtectHome=true` | Hides user home directories | No need for user data |
| `ReadWritePaths=/var/lib/tidewatch` | Allows writes only to data dir | Minimal write permissions |
| `RestrictAddressFamilies` | Limits socket types | Only needs IP and Unix sockets |
| `SystemCallFilter` | Whitelist system calls | Reduces attack surface |
| `MemoryMax=200M` | Hard memory limit | Prevents memory exhaustion |
| `CPUQuota=20%` | CPU usage limit | Prevents CPU hogging |

### Installation and Activation

```bash
# In postinst script
systemctl daemon-reload          # Load new unit file
systemctl enable tidewatch.service   # Auto-start on boot
systemctl start tidewatch.service    # Start now
```

### Testing Watchdog

```bash
# Verify watchdog enabled
systemctl show tidewatch | grep Watchdog

# Trigger watchdog timeout (stop sending pings)
kill -STOP $(systemctl show tidewatch -p MainPID | cut -d= -f2)

# Wait >60 seconds, systemd should restart service
journalctl -u tidewatch -f
```

---

## User and Group Management

### System User vs Regular User

**System User Characteristics:**
- UID < 1000 (typically 100-999)
- No login shell (`/bin/false` or `/usr/sbin/nologin`)
- No home directory (or `/nonexistent`)
- Used for service accounts

### Creating System User in postinst

```bash
USER="tidewatch"
GROUP="tidewatch"
DATADIR="/var/lib/tidewatch"

# Create group (if doesn't exist)
if ! getent group "$GROUP" > /dev/null 2>&1; then
    addgroup --system "$GROUP" || true
fi

# Create user (if doesn't exist)
if ! getent passwd "$USER" > /dev/null 2>&1; then
    adduser --system \
            --home "$DATADIR" \
            --no-create-home \
            --ingroup "$GROUP" \
            --disabled-password \
            --disabled-login \
            --shell /bin/false \
            --gecos "Tidewatch Metrics Daemon" \
            "$USER" || true
fi
```

**Flags Explained:**
- `--system` - Create system user (UID < 1000)
- `--home` - Set home directory (not created due to --no-create-home)
- `--no-create-home` - Don't create home directory
- `--ingroup` - Add to specific group
- `--disabled-password` - No password authentication
- `--disabled-login` - No login allowed
- `--shell /bin/false` - No shell access
- `--gecos` - Human-readable description

### UID/GID Assignment

**Dynamic Assignment (Recommended):**
```bash
adduser --system "$USER"  # System assigns next available UID
```

**Static Assignment (if needed):**
```bash
adduser --system --uid 500 "$USER"  # Force specific UID
```

**Tidewatch uses dynamic assignment** to avoid conflicts.

### Removal in postrm

```bash
# Only on purge, not on remove
if [ "$1" = "purge" ]; then
    if getent passwd "$USER" > /dev/null 2>&1; then
        deluser --quiet --system "$USER" || true
    fi

    if getent group "$GROUP" > /dev/null 2>&1; then
        delgroup --quiet --system "$GROUP" || true
    fi
fi
```

### File Ownership

```bash
# Set ownership in postinst
chown -R tidewatch:tidewatch /var/lib/tidewatch
chmod 750 /var/lib/tidewatch  # rwxr-x---
```

---

## Cross-Compilation for ARM

### Go Cross-Compilation

Go's cross-compilation is straightforward:

```bash
# arm64 (64-bit ARM)
GOOS=linux GOARCH=arm64 go build -o bin/tidewatch-arm64 cmd/main.go

# armhf (32-bit ARM with hardware float)
GOOS=linux GOARCH=arm GOARM=7 go build -o bin/tidewatch-armhf cmd/main.go
```

**Architecture Matrix:**

| Target | GOARCH | GOARM | Debian Arch |
|--------|--------|-------|-------------|
| 64-bit ARM | arm64 | (none) | arm64 |
| 32-bit ARM v7 | arm | 7 | armhf |
| 32-bit ARM v6 | arm | 6 | armel |

**Build Flags:**

```bash
go build \
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -trimpath \
    -o bin/tidewatch \
    cmd/main.go
```

- `-w` - Omit DWARF symbol table (smaller binary)
- `-s` - Omit symbol table (smaller binary)
- `-X main.Version` - Inject version string
- `-trimpath` - Remove absolute paths (reproducible builds)

### Static Linking

**Fully Static Binary (no libc dependency):**

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o bin/tidewatch \
    cmd/main.go
```

**Benefits:**
- Works on any Linux distro
- No dependency on specific libc version
- Easier distribution

**Tradeoffs:**
- Larger binary size
- No DNS resolution via libc (use pure Go resolver)
- No cgo-dependent libraries

**Tidewatch approach:** Static binary for maximum compatibility.

### GitHub Actions Workflow

```yaml
name: Build Debian Packages

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        arch: [arm64, armhf]

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Install nfpm
        run: |
          go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

      - name: Build binary
        run: |
          if [ "${{ matrix.arch }}" = "arm64" ]; then
            export GOARCH=arm64
          else
            export GOARCH=arm
            export GOARM=7
          fi

          CGO_ENABLED=0 GOOS=linux go build \
            -ldflags="-w -s -X main.Version=${GITHUB_REF_NAME}" \
            -trimpath \
            -o bin/tidewatch \
            cmd/main.go

      - name: Build package
        run: |
          export VERSION=${GITHUB_REF_NAME#v}
          export ARCH=${{ matrix.arch }}
          nfpm package --packager deb --target tidewatch_${VERSION}_${ARCH}.deb

      - name: Sign package
        run: |
          echo "${{ secrets.GPG_PRIVATE_KEY }}" | gpg --import
          gpg --armor --detach-sign tidewatch_*.deb

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: tidewatch-${{ matrix.arch }}
          path: |
            tidewatch_*.deb
            tidewatch_*.deb.asc
```

---

## Testing and Validation

### Docker-Based Testing

**test/docker/Dockerfile.debian-arm64:**

```dockerfile
FROM arm64v8/debian:bookworm-slim

# Install dependencies
RUN apt-get update && apt-get install -y \
    systemd \
    ca-certificates \
    sqlite3 \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy package
COPY tidewatch_*_arm64.deb /tmp/

# Install package
RUN dpkg -i /tmp/tidewatch_*.deb || true
RUN apt-get update && apt-get install -f -y

# Enable systemd
CMD ["/lib/systemd/systemd"]
```

**test/docker/docker-compose.yml:**

```yaml
version: '3.8'

services:
  tidewatch-arm64:
    build:
      context: ../..
      dockerfile: test/docker/Dockerfile.debian-arm64
    platform: linux/arm64
    privileged: true
    volumes:
      - /sys/fs/cgroup:/sys/fs/cgroup:ro
    networks:
      - metrics

  victoriametrics:
    image: victoriametrics/victoria-metrics:latest
    ports:
      - "8428:8428"
    command:
      - --storageDataPath=/victoria-metrics-data
    networks:
      - metrics

networks:
  metrics:
```

**Running Tests:**

```bash
# Build images
docker-compose -f test/docker/docker-compose.yml build

# Start services
docker-compose -f test/docker/docker-compose.yml up -d

# Check tidewatch status
docker-compose exec tidewatch-arm64 systemctl status tidewatch

# Check metrics collection
docker-compose exec tidewatch-arm64 sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) FROM metrics"

# Check VictoriaMetrics received data
curl http://localhost:8428/api/v1/query -d 'query=up{job="tidewatch"}'

# Cleanup
docker-compose -f test/docker/docker-compose.yml down
```

### Integration Test Script

**test/integration/install_test.sh:**

```bash
#!/bin/bash
set -euo pipefail

PACKAGE="tidewatch"
VERSION="0.3.0"
ARCH="arm64"

echo "==> Installing package"
dpkg -i "tidewatch_${VERSION}_${ARCH}.deb" || true
apt-get install -f -y

echo "==> Verifying files installed"
test -f /usr/bin/tidewatch || exit 1
test -f /etc/tidewatch/config.yaml || exit 1
test -f /lib/systemd/system/tidewatch.service || exit 1
test -d /var/lib/tidewatch || exit 1

echo "==> Verifying user created"
getent passwd tidewatch || exit 1
getent group tidewatch || exit 1

echo "==> Verifying service started"
systemctl is-active tidewatch || exit 1

echo "==> Verifying watchdog enabled"
systemctl show tidewatch | grep -q "WatchdogUSec=" || exit 1

echo "==> Verifying metrics collection"
sleep 10
test -f /var/lib/tidewatch/metrics.db || exit 1
COUNT=$(sqlite3 /var/lib/tidewatch/metrics.db "SELECT COUNT(*) FROM metrics")
[ "$COUNT" -gt 0 ] || exit 1

echo "==> All tests passed!"
```

### QEMU Emulation Setup

For testing ARM packages on x86_64:

```bash
# Install QEMU
apt-get install qemu-user-static binfmt-support

# Register ARM formats
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes

# Verify
docker run --rm --platform linux/arm64 arm64v8/debian:bookworm-slim uname -m
# Output: aarch64
```

---

## Common Pitfalls and Solutions

### 1. Service Fails to Start After Install

**Symptom:** `systemctl status tidewatch` shows "failed"

**Common Causes:**
- Binary not executable
- Missing dependencies
- Database directory wrong permissions
- Config file syntax error

**Solution:**
```bash
# Check binary
ls -l /usr/bin/tidewatch
file /usr/bin/tidewatch  # Should be ELF ARM

# Check permissions
ls -ld /var/lib/tidewatch
# Should be: drwxr-x--- tidewatch tidewatch

# Validate config
/usr/bin/tidewatch --validate-config /etc/tidewatch/config.yaml

# Check journal
journalctl -u tidewatch -n 50
```

### 2. Watchdog Timeout Kills Service

**Symptom:** Service restarts every 60 seconds

**Cause:** Not sending watchdog pings

**Solution:**
```go
// Ensure watchdog pings sent every 30s (half of WatchdogSec)
ticker := time.NewTicker(30 * time.Second)
go func() {
    for range ticker.C {
        watchdog.NotifyWatchdog()
    }
}()
```

### 3. Config Overwritten on Upgrade

**Symptom:** User config lost after upgrade

**Cause:** Not marked as conffile

**Solution:**
```yaml
# In nfpm.yaml
contents:
  - src: ./examples/config.yaml
    dst: /etc/tidewatch/config.yaml
    type: config  # Critical!
```

### 4. Lock File Prevents Start

**Symptom:** "Failed to acquire lock" error

**Cause:** Stale lock from crash

**Solution in postinst:**
```bash
LOCKFILE="/var/lib/tidewatch/tidewatch.lock"
if [ -f "$LOCKFILE" ]; then
    # Check if process still alive
    PID=$(cat "$LOCKFILE")
    if ! kill -0 "$PID" 2>/dev/null; then
        rm -f "$LOCKFILE"
    fi
fi
```

### 5. Cross-Compiled Binary Won't Run

**Symptom:** "Exec format error"

**Causes:**
- Wrong architecture
- Missing GOARM for 32-bit ARM
- Dynamic linking without libc

**Solution:**
```bash
# Verify architecture
file bin/tidewatch
# Should match target: "ELF 64-bit LSB executable, ARM aarch64"

# Use static linking
CGO_ENABLED=0 go build ...

# For armhf, set GOARM=7
GOARCH=arm GOARM=7 go build ...
```

### 6. Systemd Service Not Enabled on Boot

**Symptom:** Service doesn't start after reboot

**Cause:** Not enabled in postinst

**Solution:**
```bash
# In postinst
systemctl enable tidewatch.service || true
```

### 7. Database Migration Fails

**Symptom:** Upgrade fails with schema error

**Solution:**
```bash
# In postinst, backup before migration
if [ -f "$DBFILE" ]; then
    cp "$DBFILE" "${DBFILE}.backup-$(date +%Y%m%d)"
fi

# Let daemon handle migration on startup
# Don't run daemon in postinst
```

### 8. Purge Doesn't Remove Everything

**Symptom:** Files remain after `apt purge`

**Cause:** Not handled in postrm purge

**Solution in postrm:**
```bash
case "$1" in
    purge)
        rm -rf /var/lib/tidewatch
        rm -rf /etc/tidewatch
        deluser tidewatch || true
        ;;
esac
```

---

## Appendix: Quick Reference

### dpkg Commands

```bash
# Install
dpkg -i package.deb

# Remove (keep config)
dpkg -r package

# Purge (remove everything)
dpkg -P package

# List files in package
dpkg -L package

# Show package info
dpkg -s package

# List all installed packages
dpkg -l

# Verify package integrity
dpkg --verify package

# Extract package contents (without installing)
dpkg-deb -x package.deb /tmp/extract
dpkg-deb -e package.deb /tmp/extract/DEBIAN
```

### systemctl Commands

```bash
# Status
systemctl status tidewatch

# Start/stop/restart
systemctl start tidewatch
systemctl stop tidewatch
systemctl restart tidewatch

# Enable/disable (autostart)
systemctl enable tidewatch
systemctl disable tidewatch

# View logs
journalctl -u tidewatch
journalctl -u tidewatch -f  # Follow
journalctl -u tidewatch --since "1 hour ago"

# Reload configuration
systemctl daemon-reload

# Show properties
systemctl show tidewatch

# Check if enabled
systemctl is-enabled tidewatch

# Check if active
systemctl is-active tidewatch
```

### nfpm Commands

```bash
# Install
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

# Package
nfpm package --packager deb

# Package with custom config
nfpm package --config custom-nfpm.yaml --packager deb

# Package for specific target
nfpm package --packager deb --target output.deb

# Validate config
nfpm init  # Generate example config
```

### Build Script Example

**scripts/build-deb.sh:**

```bash
#!/bin/bash
set -euo pipefail

VERSION="${1:-0.3.0}"
ARCH="${2:-arm64}"

echo "Building tidewatch ${VERSION} for ${ARCH}"

# Build binary
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
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -trimpath \
    -o bin/tidewatch \
    cmd/main.go

# Package
export VERSION ARCH
nfpm package --packager deb --target "tidewatch_${VERSION}_${ARCH}.deb"

# Sign
gpg --armor --detach-sign "tidewatch_${VERSION}_${ARCH}.deb"

# Checksum
sha256sum "tidewatch_${VERSION}_${ARCH}.deb" > "tidewatch_${VERSION}_${ARCH}.deb.sha256"

echo "Package built: tidewatch_${VERSION}_${ARCH}.deb"
```

---

## Summary

This guide covers comprehensive Debian packaging for the tidewatch metrics daemon:

1. **Package Structure** - debian/ directory layout and nfpm config
2. **Build Systems** - nfpm for CI/CD, debhelper for traditional builds
3. **Maintainer Scripts** - Lifecycle management (postinst, prerm, postrm)
4. **Conffiles** - Configuration preservation across upgrades
5. **Systemd** - Type=notify, watchdog, security hardening
6. **Users** - System user creation and management
7. **Cross-Compilation** - ARM builds from x86_64
8. **Testing** - Docker, QEMU, integration tests
9. **Troubleshooting** - Common issues and solutions

**Next Steps:**
1. Create debian/ directory structure
2. Configure nfpm.yaml
3. Write maintainer scripts
4. Set up GitHub Actions workflow
5. Test with Docker + QEMU
6. Release first package (v0.3.0)

For questions or issues, consult:
- Debian Policy Manual: https://www.debian.org/doc/debian-policy/
- systemd documentation: https://www.freedesktop.org/software/systemd/man/
- nfpm documentation: https://nfpm.goreleaser.com/
