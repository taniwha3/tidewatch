# Product Requirements Document: Belabox Metrics Collection System

**Project:** Belabox IRL Streaming Metrics Collector
**Platform:** Orange Pi 5+ with Belabox
**Timeline:** 2 weeks
**Status:** Planning Phase
**Last Updated:** 2025-10-11

---

## Executive Summary

Build a comprehensive metrics collection system for Orange Pi running Belabox IRL streaming software. The system will capture critical streaming, network, and system performance metrics locally, then stream them to a remote monitoring infrastructure for real-time debugging and historical analysis of streaming issues.

**Key Goals:**
- Capture all metrics necessary to debug streaming issues
- Local buffering to survive network outages
- Remote streaming for real-time monitoring
- Minimal resource overhead (<5% CPU, <300MB RAM)
- Production-ready within 2 weeks

---

## Table of Contents

1. [Architecture Decisions](#architecture-decisions)
2. [Metrics to Collect](#metrics-to-collect)
3. [System Architecture](#system-architecture)
4. [Implementation Plan](#implementation-plan)
5. [Configuration](#configuration)
6. [Resource Requirements](#resource-requirements)
7. [Testing Strategy](#testing-strategy)
8. [Deployment](#deployment)
9. [Open Questions](#open-questions)

**Quick Links:**
- **[MILESTONE-1.md](MILESTONE-1.md)** - Current milestone specification (Days 1-2)
- **Architecture Decisions** - All technical choices documented below

---

## Architecture Decisions

### Core Technology Stack

| Component | Technology | Status | Rationale |
|-----------|-----------|--------|-----------|
| **Programming Language** | Go 1.21+ | ✅ Decided | Single binary, low overhead, excellent concurrency, ARM cross-compilation |
| **Local Storage** | SQLite (WAL mode) | ✅ Decided | Zero config, reliable, ACID compliance, good performance |
| **Remote Server** | Self-hosted | ✅ Decided | Full control, no costs, privacy, flexibility |
| **Transport Protocol** | HTTP/HTTPS + JSON | ✅ Decided | Universal compatibility, easy debugging, firewall-friendly |
| **Configuration** | YAML + Env vars | ✅ Decided | Human-readable config, env vars for secrets |
| **Deployment** | Systemd service | ✅ Decided | Auto-restart, logging, standard Linux approach |
| **Belabox Integration** | TBD | 🔍 Research | Needs investigation on actual device |
| **Modem Metrics** | Deferred | ⏸️ Deferred | No hardware yet, will add later |

### Development Approach

**Strategy:** Milestone-based incremental development - Build smallest useful system first, iterate rapidly

**Current Milestone:** See [MILESTONE-1.md](MILESTONE-1.md) for detailed specification

**Overview Timeline:** 2 weeks aggressive schedule
- **Milestone 1 (Days 1-2):** Minimal end-to-end (2 metrics: CPU temp + SRT packet loss)
- **Milestone 2 (Days 3-5):** Add more system metrics + buffering
- **Milestone 3 (Days 6-9):** Belabox/encoder metrics + priority queue
- **Milestone 4 (Days 10-14):** Modem metrics + optimization + production deployment

**Note:** The original 4-phase plan has been refined into focused milestones. Each milestone delivers independently useful functionality.

---

## Metrics to Collect

### 1. System Performance Metrics

**Collection Interval:** 10-30 seconds

#### CPU
- CPU usage (overall and per-core) %
- CPU temperature (°C)
- CPU frequency (MHz/GHz)
- Thermal throttling status (boolean)

#### Memory
- RAM usage (MB and %)
- Available RAM (MB)
- Swap usage (MB and %)
- Memory pressure indicator

#### Storage
- Disk usage (used/available GB)
- Write speed (MB/s)
- I/O wait percentage

#### GPU/VPU
- GPU usage %
- GPU temperature (°C)
- VPU (video encoder) usage %
- Hardware acceleration status

#### Power
- Power consumption (watts)
- Power source (battery/AC)
- Battery level % (if applicable)
- Input voltage (V)
- Estimated runtime

#### Environment
- System uptime (seconds)
- Load averages (1m, 5m, 15m)
- Case temperature (°C)
- Fan speed (RPM, if equipped)

### 2. Network Metrics

**Collection Interval:** 5 seconds

#### Per-Modem Metrics (deferred until hardware available)
- Connection status (active/inactive)
- Signal strength RSSI (dBm)
- Signal quality RSRQ (dB)
- Network type (3G/4G/5G/LTE)
- Carrier name
- Upload/download bandwidth (Mbps)
- Data usage (bytes TX/RX)
- Connection uptime (seconds)
- IP address
- Reconnection count

#### Aggregated Network
- Total upload bandwidth (Mbps)
- Total download bandwidth (Mbps)
- Active modem count
- Bond efficiency %

### 3. SRT/SRTLA Stream Metrics

**Collection Interval:** 1-5 seconds
**Priority:** High (P1)

#### Connection Health
- SRT connection status (connected/disconnected/reconnecting)
- Round Trip Time (RTT) in ms
- Configured latency (ms)
- Stream uptime (seconds)
- Reconnection event count

#### Packet Statistics
- Packets sent (count)
- Packets received (count)
- Packets lost (count)
- Packet loss rate %
- Packets retransmitted (count)
- Retransmission rate %
- Packets dropped (count)

#### Jitter & Timing
- Jitter (ms)
- Buffer utilization %
- Send buffer size (bytes)
- Receive buffer size (bytes)

### 4. Video Encoding Metrics

**Collection Interval:** 1-5 seconds
**Priority:** High (P1)

#### Encoder Performance
- Video bitrate - current (kbps)
- Video bitrate - target (kbps)
- Video bitrate - actual measured (kbps)
- Bitrate stability (variance %)
- Dynamic bitrate adjustment count
- Encoding format (H.264/H.265)
- Resolution (e.g., 1920x1080)
- Frame rate - target (fps)
- Frame rate - actual (fps)
- Keyframe interval (frames)

#### Frame Statistics
- Frames encoded (count)
- Frames dropped (count)
- Frame drop rate %
- Encoding latency (ms per frame)
- Frames skipped (count)

### 5. Audio Metrics

**Collection Interval:** 5 seconds

- Audio bitrate (kbps)
- Audio sample rate (Hz)
- Audio channels (mono/stereo)
- Audio codec (AAC/Opus/etc)
- Audio sync offset (ms)
- Audio frames dropped (count)

### 6. HDMI Input Metrics

**Collection Interval:** 5 seconds

- Input signal status (detected/no signal)
- Input resolution
- Input frame rate (fps)
- Input color format (YUV/RGB)
- Signal stability (interruption count)
- HDMI errors (count)

### 7. Event Tracking

**Collection:** Event-driven (immediate)
**Priority:** Critical (P0)

- Error messages
- Warning events
- Connection events (modem connect/disconnect)
- Encoding errors
- Buffer overflow/underflow events
- Watchdog resets

### 8. Derived/Calculated Metrics

**Calculated locally before upload:**

- Stream health score (0-100)
- Network stability index
- Encoding efficiency (quality/bitrate)
- Overall latency estimate (capture to ingest)
- Reliability score (uptime %)

### 9. Metadata

**Attached to all metrics:**

- Timestamp (ISO 8601, millisecond precision)
- Session ID (UUID)
- Device ID
- Software version (Belabox)
- Firmware version (Orange Pi)
- Geographic location (GPS if available)

### Alert Thresholds

Metrics that trigger immediate upload (P0):

- Packet loss > 5%
- RTT > 500ms
- Dropped frames > 1%
- CPU temperature > 85°C
- CPU usage > 90% for >30 seconds
- Available RAM < 100MB
- Bitrate < 50% of target for >10 seconds
- Buffer utilization > 90%

---

## System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Orange Pi / Belabox                   │
│                                                          │
│  ┌──────────────┐    ┌─────────────┐   ┌────────────┐  │
│  │   Metric     │───▶│   Local     │──▶│  Metrics   │  │
│  │  Collectors  │    │   Buffer    │   │  Streamer  │──┼──▶ Remote
│  └──────────────┘    │  (SQLite)   │   │            │  │    Server
│         │            └─────────────┘   └────────────┘  │
│         │                   │                 │         │
│         ▼                   ▼                 ▼         │
│  ┌──────────────────────────────────────────────────┐  │
│  │           Local Time-Series Storage              │  │
│  │         (Rotating logs, compression)             │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
                            │
                            │ HTTP/HTTPS + JSON
                            ▼
┌─────────────────────────────────────────────────────────┐
│                   Remote Server                          │
│                                                          │
│  ┌────────────┐    ┌──────────────┐   ┌─────────────┐  │
│  │  Ingestion │───▶│  Time-Series │──▶│   Grafana   │  │
│  │    API     │    │   Database   │   │  Dashboard  │  │
│  └────────────┘    └──────────────┘   └─────────────┘  │
│                    (InfluxDB/Prometheus/VictoriaMetrics) │
└─────────────────────────────────────────────────────────┘
```

### Component Architecture

```
metrics-collector (single Go binary)
│
├── Collectors (goroutines, configurable intervals)
│   ├── SystemCollector (30s) → CPU, RAM, disk, temp
│   ├── BelaboxCollector (2s) → SRT stats, stream status
│   ├── EncoderCollector (2s) → Video/audio encoding metrics
│   ├── ModemCollector (5s) → [Future] Network/modem stats
│   └── EventCollector (event-driven) → Errors, warnings
│
├── Storage Layer
│   ├── SQLite database (WAL mode)
│   ├── In-memory buffer (1000 records)
│   ├── Write-ahead log (5s flush)
│   └── Retention manager (prune old data)
│
├── Upload Manager
│   ├── Priority Queue (P0/P1/P2/P3)
│   ├── HTTP client (with retry + exponential backoff)
│   ├── Batch assembler (100-500 metrics per request)
│   ├── Compression (gzip)
│   └── Backfill processor
│
├── Configuration Manager
│   ├── YAML parser
│   ├── Environment variable override
│   └── Live reload (on SIGHUP)
│
└── Health Monitor
    ├── HTTP endpoint (:9100/health)
    ├── Process watchdog
    └── Self-metrics (meta-monitoring)
```

### Data Flow

1. **Collection Phase**
   - Collectors run on independent timers (goroutines)
   - Metrics sent to in-memory buffer channel
   - Buffer batches 1000 records or 5 seconds, whichever first

2. **Storage Phase**
   - Batch write to SQLite (transaction)
   - WAL mode ensures durability
   - Indexes on timestamp and metric_name

3. **Upload Phase**
   - Upload manager queries unsynced metrics
   - Sorts by priority (P0 → P1 → P2 → P3)
   - Batches into HTTP requests (100-500 metrics)
   - Compresses with gzip
   - POSTs to remote server
   - Marks as uploaded on success
   - Retries with exponential backoff on failure

4. **Retention Phase**
   - Background goroutine runs hourly
   - Aggregates metrics >48h old to 1-min resolution
   - Aggregates metrics >7d old to 5-min resolution
   - Deletes metrics >30d old (after upload confirmation)

### Streaming Modes

**Real-time Mode** (good connectivity)
- Upload every 30 seconds
- Batch size: 100-500 metrics
- All priorities processed

**Degraded Mode** (poor connectivity)
- Upload every 60-300 seconds
- Increased batch size
- P0 and P1 only
- Gzip compression mandatory

**Offline Mode** (no connectivity)
- Queue everything to SQLite
- Monitor buffer size (max 5GB)
- Prune lowest priority if needed
- Auto-resume on connectivity

### Priority Queue

**P0 - Critical** (immediate upload)
- Connection loss events
- Stream failures
- System errors
- Thermal throttling

**P1 - High** (upload every 5-10s)
- SRT packet loss
- Frame drops
- RTT spikes
- Bitrate instability

**P2 - Normal** (upload every 30-60s)
- System metrics
- Network metrics
- Encoding metrics

**P3 - Low** (upload every 5-10min)
- Historical aggregates
- Session summaries
- Non-critical info

### Backfill Strategy

After network recovery:
1. Query remote server for last checkpoint timestamp
2. Fetch unsynced metrics from SQLite
3. Sort by priority (P0 → P1 → P2 → P3)
4. Upload oldest-first within each priority
5. Limit to 20% of upload bandwidth
6. Verify success before deleting local copy

---

## Implementation Plan

### Project Structure

```
thugshells/
├── cmd/
│   └── metrics-collector/
│       └── main.go                 # Entry point
├── internal/
│   ├── collector/
│   │   ├── collector.go            # Collector interface
│   │   ├── system.go               # System metrics collector
│   │   ├── belabox.go              # Belabox/SRT collector
│   │   ├── encoder.go              # Video encoder collector
│   │   └── modem.go                # Modem collector (future)
│   ├── storage/
│   │   ├── storage.go              # Storage interface
│   │   └── sqlite.go               # SQLite implementation
│   ├── uploader/
│   │   ├── uploader.go             # Uploader interface
│   │   ├── http.go                 # HTTP uploader
│   │   └── buffer.go               # Buffering/retry logic
│   ├── config/
│   │   └── config.go               # Configuration management
│   └── models/
│       └── metrics.go              # Metric data structures
├── configs/
│   └── config.yaml.example         # Example configuration
├── scripts/
│   ├── build.sh                    # Cross-compile script
│   ├── install.sh                  # Install systemd service
│   └── deploy.sh                   # Deploy to Orange Pi
├── systemd/
│   └── metrics-collector.service   # Systemd unit file
├── docs/
│   └── belabox-integration.md      # Belabox integration findings
├── go.mod
├── go.sum
├── Makefile
├── PRD.md                          # This file
└── README.md
```

### Phase 1: MVP (Days 1-3)

**Goal:** End-to-end working system with system metrics

**Tasks:**
1. Initialize Go project
   - `go mod init github.com/taniwha3/thugshells`
   - Set up directory structure
   - Create interfaces

2. System metrics collector
   - Read `/proc/stat` for CPU
   - Read `/proc/meminfo` for RAM
   - Read `/sys/class/thermal/` for temperature
   - Read `/proc/diskstats` for disk

3. SQLite storage
   - Schema creation
   - WAL mode setup
   - Prepared statements
   - Batch insert logic

4. HTTP uploader
   - Simple POST to remote endpoint
   - JSON marshaling
   - Basic error handling

5. Configuration
   - YAML parser
   - Config struct
   - Validation

6. Main loop
   - Goroutine orchestration
   - Graceful shutdown
   - Signal handling

7. Systemd service
   - Unit file creation
   - Install script

**Deliverable:** System metrics visible in remote dashboard

### Phase 2: Belabox Integration (Days 4-7)

**Goal:** Add streaming-specific metrics

**Tasks:**
1. Investigate Belabox
   - SSH to Orange Pi
   - Document data sources
   - Create `belabox-integration.md`

2. Belabox/SRT collector
   - Parse logs or query API (based on investigation)
   - Extract SRT stats
   - Map to metrics model

3. Encoder metrics collector
   - Parse ffmpeg logs
   - Extract bitrate, frame rate, drops
   - Calculate derived metrics

4. HDMI input collector
   - Query V4L2 or similar
   - Detect signal status

5. Buffering & retry
   - Exponential backoff
   - Persistent queue
   - Upload checkpointing

6. Session tracking
   - Session start/end detection
   - Session metadata collection

**Deliverable:** Full streaming metrics captured and uploaded

### Phase 3: Robustness (Days 8-10)

**Goal:** Production-ready reliability

**Tasks:**
1. Priority queue
   - Classify metrics by priority
   - Separate upload queues
   - Priority-based scheduling

2. Backfill logic
   - Checkpoint tracking
   - Sync on reconnect
   - Bandwidth limiting

3. Retention & rotation
   - Time-based aggregation
   - Old data pruning
   - Disk space monitoring

4. Health check endpoint
   - HTTP server on :9100
   - `/health` endpoint
   - Collector status reporting

5. Error handling
   - Comprehensive error wrapping
   - Panic recovery
   - Logging improvements

**Deliverable:** Robust system that survives network outages

### Phase 4: Polish (Days 11-14)

**Goal:** Optimization and deployment

**Tasks:**
1. Modem metrics (if hardware available)
   - ModemManager integration
   - AT commands
   - Signal strength tracking

2. Performance optimization
   - Profile CPU/memory usage
   - Optimize hot paths
   - Reduce allocations

3. Testing
   - Unit tests for collectors
   - Integration tests
   - Load testing (48h simulation)

4. Documentation
   - README with setup instructions
   - Configuration guide
   - Troubleshooting guide

5. Deployment automation
   - Build script for ARM64
   - Deploy script with SCP
   - Systemd enable/start

**Deliverable:** Production deployment on Orange Pi

---

## Configuration

### Config File: `/etc/belabox-metrics/config.yaml`

```yaml
collection:
  enabled: true
  collectors:
    - name: system
      interval: 30s
      enabled: true
    - name: network
      interval: 5s
      enabled: false  # No modems yet
    - name: srt
      interval: 2s
      enabled: true
    - name: encoder
      interval: 2s
      enabled: true
    - name: events
      enabled: true

storage:
  type: sqlite
  path: /var/lib/belabox-metrics/active/metrics.db
  retention:
    full_resolution: 48h
    aggregated_1m: 7d
    aggregated_5m: 30d
  rotation:
    max_size: 500MB
    interval: 12h
  compression: gzip

buffer:
  path: /var/lib/belabox-metrics/buffer
  max_size: 5GB
  prune_strategy: oldest_first

remote:
  enabled: true
  url: https://metrics.example.com/api/v1
  auth:
    type: bearer_token
    token_file: /etc/belabox-metrics/api-token
  upload:
    mode: auto  # realtime, degraded, offline, or auto
    batch_size: 100
    interval: 30s
    timeout: 10s
    compression: true
  priority:
    critical: 0s     # immediate
    high: 10s
    normal: 60s
    low: 600s
  backfill:
    enabled: true
    bandwidth_limit: 20%
    rate_limit: 1/s

device:
  id: belabox-001
  name: "Orange Pi IRL Stream 1"
  location: "Mobile"

logging:
  level: info  # debug, info, warn, error
  output: /var/log/belabox-metrics/collector.log
  rotation: 100MB
```

### Storage Layout

```
/var/lib/belabox-metrics/
├── active/
│   ├── metrics.db              # Current SQLite database
│   ├── metrics.db-wal          # Write-ahead log
│   ├── metrics.db-shm          # Shared memory
│   └── session-{uuid}.json     # Current session metadata
├── buffer/
│   ├── pending-001.json.gz     # Metrics waiting to upload
│   ├── pending-002.json.gz
│   └── failed-retries.json.gz  # Failed upload attempts
└── archive/
    ├── 2025-10-11/
    │   ├── metrics-12h.db      # Aggregated 12-hour chunks
    │   └── events-12h.log.gz
    └── 2025-10-10/
        └── ...
```

### Database Schema

```sql
CREATE TABLE metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,  -- Unix ms
    metric_name TEXT NOT NULL,
    metric_value REAL,
    tags TEXT,  -- JSON: {"core":"0","modem":"1"}
    session_id TEXT,
    uploaded INTEGER DEFAULT 0,  -- 0=pending, 1=uploaded
    priority INTEGER DEFAULT 2   -- 0=critical, 1=high, 2=normal, 3=low
);

CREATE INDEX idx_timestamp ON metrics(timestamp);
CREATE INDEX idx_metric_session ON metrics(metric_name, session_id, timestamp);
CREATE INDEX idx_uploaded ON metrics(uploaded, priority, timestamp);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    start_time INTEGER NOT NULL,
    end_time INTEGER,
    status TEXT,  -- active, completed, failed
    metadata TEXT  -- JSON
);

CREATE TABLE upload_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    last_uploaded_id INTEGER,
    last_uploaded_timestamp INTEGER,
    upload_time INTEGER
);
```

### Remote API Contract

```
POST /api/v1/metrics
Content-Type: application/json
Authorization: Bearer {token}

{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "device_id": "belabox-001",
  "timestamp": "2025-10-11T14:30:00.123Z",
  "metrics": [
    {
      "name": "cpu.usage",
      "value": 45.2,
      "tags": {"core": "0"},
      "timestamp": "2025-10-11T14:30:00.123Z"
    },
    {
      "name": "srt.rtt",
      "value": 125,
      "tags": {"modem": "1"},
      "timestamp": "2025-10-11T14:30:00.456Z"
    }
  ]
}

Response 200 OK:
{
  "success": true,
  "received": 2,
  "checkpoint": 1234567890123
}
```

---

## Resource Requirements

### Target Resource Usage

| Resource | Target | Peak | Notes |
|----------|--------|------|-------|
| CPU | <5% average | <15% during backfill | Single core on multi-core SoC |
| RAM | 50-100 MB base | 300 MB max | Includes buffer |
| Disk (active) | 100-500 MB | - | SQLite database |
| Disk (buffer) | 1-5 GB | 5 GB hard limit | Configurable |
| Disk (archives) | 50-100 MB/day | - | Compressed |
| Network | 10-50 KB/s | 100-200 KB/s | Backfill mode |
| Daily data | 50-200 MB | - | Depends on metrics |

### Dependencies

**Go Libraries:**
- `modernc.org/sqlite` - Pure Go SQLite (no CGO)
- `gopkg.in/yaml.v3` - YAML parsing
- Standard library: `net/http`, `encoding/json`, `database/sql`, `time`

**System Requirements:**
- Linux kernel 3.10+ (for /proc, /sys access)
- Systemd (for service management)
- Write access to `/var/lib/belabox-metrics`
- Network connectivity for remote upload

---

## Testing Strategy

### Unit Tests

**Coverage Target:** >70%

Test each collector independently:
```go
func TestSystemCollector_CollectCPU(t *testing.T) {
    collector := NewSystemCollector()
    metrics, err := collector.Collect()
    assert.NoError(t, err)
    assert.Contains(t, metrics, "cpu.usage")
}
```

### Integration Tests

**Scenarios:**
1. End-to-end: collect → store → upload
2. Network failure: offline buffering
3. Disk full: graceful degradation
4. Crash recovery: data integrity

### Load Tests

**Simulate:**
- 48 hours continuous collection
- 10,000 metrics/minute
- Network flapping
- Verify <5% CPU, <300MB RAM

### Field Tests

**On actual Orange Pi:**
1. Deploy during real stream
2. Monitor resource usage
3. Verify metric accuracy
4. Test Belabox integration

---

## Deployment

### Build Process

```bash
# Cross-compile for ARM64
GOOS=linux GOARCH=arm64 go build -o metrics-collector cmd/metrics-collector/main.go

# Or use build script
./scripts/build.sh
```

### Installation

```bash
# Copy binary
sudo cp metrics-collector /usr/local/bin/

# Create directories
sudo mkdir -p /var/lib/belabox-metrics/{active,buffer,archive}
sudo mkdir -p /etc/belabox-metrics

# Copy config
sudo cp configs/config.yaml /etc/belabox-metrics/config.yaml

# Create systemd service
sudo cp systemd/metrics-collector.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable metrics-collector
sudo systemctl start metrics-collector
```

### Systemd Service

```ini
[Unit]
Description=Belabox Metrics Collector
After=network.target

[Service]
Type=simple
User=belabox
ExecStart=/usr/local/bin/metrics-collector -config /etc/belabox-metrics/config.yaml
Restart=always
RestartSec=10s

[Install]
WantedBy=multi-user.target
```

### Monitoring the Collector

```bash
# Check status
systemctl status metrics-collector

# View logs
journalctl -u metrics-collector -f

# Health check
curl http://localhost:9100/health

# Check metrics database
sqlite3 /var/lib/belabox-metrics/active/metrics.db "SELECT COUNT(*) FROM metrics"
```

---

## Open Questions

### Critical (blocking progress)

1. **Belabox Integration**
   - ❓ Does Belabox expose a stats API?
   - ❓ Where are Belabox logs stored?
   - ❓ What format are SRT stats in?
   - **Action:** SSH to Orange Pi and investigate

2. **Remote Server**
   - ❓ What specific technology? (Prometheus, InfluxDB, VictoriaMetrics, custom)
   - ❓ Is server infrastructure ready?
   - ❓ Authentication mechanism?
   - **Action:** Decide and set up server in parallel

### Non-blocking (can defer)

3. **Modem Integration**
   - ❓ What modem models will be used?
   - ❓ Is ModemManager available?
   - **Action:** Defer until hardware arrives

4. **GPS Location**
   - ❓ Is GPS available on Orange Pi?
   - ❓ Should we track location?
   - **Action:** Optional feature, add if needed

---

## Success Criteria

**MVP Success (Day 3):**
- ✅ System metrics collected every 30s
- ✅ Stored in SQLite locally
- ✅ Uploaded to remote server every 30s
- ✅ Visible in Grafana dashboard
- ✅ Systemd service auto-starts on boot

**Phase 2 Success (Day 7):**
- ✅ SRT metrics (packet loss, RTT, jitter) collected
- ✅ Video encoder metrics (bitrate, frame rate, drops) collected
- ✅ Buffering works during network outage
- ✅ Session tracking operational

**Phase 3 Success (Day 10):**
- ✅ Priority queue ensures critical events uploaded first
- ✅ Backfill resumes after network recovery
- ✅ Retention policy prunes old data
- ✅ Health check endpoint responding

**Production Success (Day 14):**
- ✅ Resource usage <5% CPU, <300MB RAM
- ✅ No data loss during network outages
- ✅ Deployed on Orange Pi
- ✅ Tested during real stream
- ✅ Documentation complete

---

## Appendix

### Glossary

- **Belabox:** Open-source IRL streaming encoder software
- **SRT:** Secure Reliable Transport protocol for video streaming
- **SRTLA:** SRT with link aggregation (bonding)
- **RTT:** Round-trip time (network latency)
- **GOP:** Group of Pictures (keyframe interval)
- **WAL:** Write-Ahead Logging (SQLite mode)
- **VPU:** Video Processing Unit (hardware encoder)

### References

- Belabox GitHub: https://github.com/BELABOX/tutorial
- SRT Protocol: https://github.com/Haivision/srt
- Orange Pi 5+: https://belabox.net/rk3588/
- Go SQLite: https://pkg.go.dev/modernc.org/sqlite
