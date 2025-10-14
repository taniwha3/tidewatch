# Milestone 1: End-to-End Pipeline with 2 Metrics

**Status:** In Progress  
**Goal:** Smallest useful system - collect 2 metrics, store locally, POST to remote endpoint  
**Timeline:** 1-2 days  
**Last Updated:** 2025-10-11

---

## Overview

Build a minimal but complete end-to-end metrics collection pipeline that:
1. Collects 2 metrics (CPU temperature + SRT packet loss)
2. Stores them in SQLite
3. POSTs them to a remote HTTP endpoint
4. Runs on both macOS (dev) and Orange Pi (production)
5. Installs as systemd service with auto-start

This validates the architecture and provides a foundation for adding more metrics and features.

---

## Scope

### IN Scope ✅

**Core Functionality:**
- Single Go binary (`tidewatch`)
- Collect 2 metrics:
  - **CPU Temperature** - Real on Orange Pi, mock on macOS
  - **SRT Packet Loss** - Real if found quickly, mock otherwise
- SQLite storage with minimal schema
- YAML configuration file
- HTTP POST to configurable remote endpoint
- Simple HTTP receiver that logs metrics

**Deployment:**
- Cross-compile for ARM64 (Orange Pi)
- Install script (`install.sh`) that:
  - Uninstalls existing systemd service
  - Installs new binary + config
  - Enables and starts service
  - Ensures auto-start on boot
- Runs on macOS for development/testing
- Deploys to Orange Pi

**Configuration:**
- Metrics to collect (name, interval)
- Remote endpoint URL
- SQLite database path
- Device ID

### OUT of Scope ❌

**Deferred to Later Milestones:**
- Multiple specialized collectors
- Retry logic and exponential backoff
- Priority queue (P0/P1/P2/P3)
- Backfill after network recovery
- Data retention and rotation
- Health check endpoint
- Full database schema (sessions, checkpoints)
- Comprehensive error handling
- Buffering for offline mode
- More than 2 metrics
- Grafana dashboard
- Authentication for remote endpoint

---

## Success Criteria

Milestone 1 is complete when:

1. ✅ **Development:** Runs on macOS, collects mock metrics, stores to SQLite
2. ✅ **Query:** Can query SQLite and see collected metrics
3. ✅ **Build:** Cross-compiles to ARM64 without errors
4. ✅ **Deploy:** Install script successfully deploys to Orange Pi
5. ✅ **Production:** Runs on Orange Pi, collects real CPU temperature
6. ✅ **Systemd:** Service starts on boot and auto-restarts on failure
7. ✅ **Upload:** Metrics POST to remote HTTP endpoint
8. ✅ **Receive:** Remote endpoint receives and logs metrics

---

## Architecture Decisions from Technical Review

Based on deep technical review of Orange Pi 5+ (RK3588) and Belabox:

### Key Findings
1. **Belabox uses GStreamer** (belacoder), NOT FFmpeg
2. **SRT stats NOT accessible on-device** - SRT sockets owned by belacoder process
3. **Solution:** Get SRT metrics server-side from SRTLA receiver's `/stats` endpoint
4. **Encoder metrics:** Parse journald logs from belacoder (avoids CGO)
5. **HDMI metrics:** Use `v4l2-ctl` (shell out for MVP, ioctl later)
6. **RK3588 thermals:** Multiple zones in `/sys/class/thermal/thermal_zone*`

### Milestone 1 Adjusted Scope

**Collect:**
1. **CPU Temperature** - Real (from RK3588 thermal zones) ✅
2. **System Metrics** - Real (CPU usage, RAM, optional bonus) ✅
3. **SRT Packet Loss** - Mock (server-side collector in Milestone 2) ⏸️

**Why mock SRT for M1:**
- Real SRT stats require server-side collector (separate component)
- Don't want to block MVP on setting up receiver stats endpoint
- Validates architecture without complexity

**Deferred to Milestone 2:**
- Encoder metrics (journald parsing)
- HDMI input metrics (v4l2-ctl)
- Server-side SRT stats (receiver API)

See `docs/belabox-integration.md` for complete implementation strategy.

---

## Technical Specification

### Metrics

#### 1. CPU Temperature

**macOS (Mock):**
```go
func getCPUTemperature() float64 {
    return 45.0 + rand.Float64()*10.0  // 45-55°C
}
```

**Orange Pi (Real):**
```go
func getCPUTemperature() float64 {
    data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
    if err != nil {
        return 0.0
    }
    temp, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
    return temp / 1000.0  // millidegrees to degrees
}
```

**Interval:** 30 seconds

#### 2. SRT Packet Loss (Mock for M1)

**Implementation:**
```go
func getSRTPacketLoss() float64 {
    // Simulate occasional packet loss
    if rand.Float64() < 0.1 {  // 10% chance
        return rand.Float64() * 5.0  // 0-5% loss
    }
    return 0.0
}
```

**Interval:** 5 seconds

**Note:** Real SRT stats require server-side collector (SRTLA receiver `/stats` endpoint). This is deferred to Milestone 2. See `docs/belabox-integration.md` for details.

#### 3. Bonus Metrics (if time permits)

**CPU Usage:**
```go
// Read /proc/stat, calculate percentage
```

**Memory Available:**
```go
// Read /proc/meminfo, parse MemAvailable
```

**All Thermal Zones (RK3588):**
```go
// Read /sys/class/thermal/thermal_zone*/type and temp
// Zones: SoC, big cores, small cores, GPU, NPU
```

### Data Model

```go
type Metric struct {
    Timestamp   time.Time
    MetricName  string
    MetricValue float64
    DeviceID    string
}
```

### SQLite Schema

```sql
-- Minimal schema for Milestone 1
-- Enhanced in Milestone 2 with value_type, value_text for non-numeric metrics
CREATE TABLE IF NOT EXISTS metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_ms INTEGER NOT NULL,     -- Unix milliseconds
    metric_name TEXT NOT NULL,
    metric_value REAL,                 -- Numeric value
    device_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_timestamp ON metrics(timestamp_ms);
CREATE INDEX IF NOT EXISTS idx_name_time ON metrics(metric_name, timestamp_ms);

-- SQLite tuning for ARM SBC (set after opening DB)
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA busy_timeout=10000;
PRAGMA temp_store=MEMORY;
```

**Note:** Full production schema (with `value_type`, `value_text`, `priority`, `uploaded`) added in Milestone 2.

### Config File

**Path:** `/etc/tidewatch/config.yaml`

```yaml
device:
  id: belabox-001

storage:
  path: /var/lib/tidewatch/metrics.db

remote:
  # For Milestone 1: simple receiver endpoint
  # For Milestone 2+: VictoriaMetrics /api/v1/import (JSONL + gzip)
  url: http://example.com:8080/api/metrics
  enabled: true

metrics:
  - name: cpu.temperature
    interval: 30s
    enabled: true

  - name: srt.packet_loss
    interval: 5s
    enabled: true

  # Bonus metrics (optional for M1)
  - name: cpu.usage
    interval: 30s
    enabled: false

  - name: memory.available
    interval: 30s
    enabled: false
```

### Remote API

**Request:**
```json
POST /api/metrics
Content-Type: application/json

{
  "device_id": "belabox-001",
  "timestamp": "2025-10-11T14:30:00.123Z",
  "metrics": [
    {
      "name": "cpu.temperature",
      "value": 52.3,
      "timestamp": "2025-10-11T14:30:00.123Z"
    }
  ]
}
```

**Response:**
```json
200 OK
{
  "success": true,
  "received": 1
}
```

---

## Project Structure

```
tidewatch/
├── cmd/
│   ├── tidewatch/
│   │   └── main.go
│   └── metrics-receiver/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── collector/
│   │   └── collector.go
│   ├── storage/
│   │   └── sqlite.go
│   └── uploader/
│       └── http.go
├── configs/
│   └── config.yaml
├── scripts/
│   ├── build.sh
│   └── install.sh
├── systemd/
│   └── tidewatch.service
├── docs/
│   └── belabox-integration.md
├── go.mod
├── go.sum
├── .gitignore
├── MILESTONE-1.md
├── PRD.md
└── README.md
```

---

## Implementation Tasks

### Task 1: Review Belabox Integration Strategy
Read `docs/belabox-integration.md` - already completed based on technical review.

**Key takeaway:** Use mock SRT data for M1; real data comes from server-side collector in M2.

### Task 2: Initialize Project
```bash
go mod init github.com/taniwha3/tidewatch
mkdir -p cmd/tidewatch cmd/metrics-receiver
mkdir -p internal/{config,collector,storage,uploader}
mkdir -p configs scripts systemd docs
```

### Task 3: Core Implementation
1. `internal/config/config.go` - YAML parsing
2. `internal/collector/collector.go` - Metric collection  
3. `internal/storage/sqlite.go` - Database ops
4. `internal/uploader/http.go` - HTTP POST
5. `cmd/tidewatch/main.go` - Main loop

### Task 4: Build Scripts

**scripts/build.sh:**
```bash
#!/bin/bash
set -e

echo "Building for macOS..."
GOOS=darwin GOARCH=amd64 go build -o bin/tidewatch-darwin \
    cmd/tidewatch/main.go

echo "Building for Orange Pi (ARM64)..."
GOOS=linux GOARCH=arm64 go build -o bin/tidewatch-linux-arm64 \
    cmd/tidewatch/main.go

echo "Building receiver..."
go build -o bin/metrics-receiver cmd/metrics-receiver/main.go

ls -lh bin/
```

### Task 5: Install Script

**scripts/install.sh:**
```bash
#!/bin/bash
set -e

BINARY=${1:-bin/tidewatch-linux-arm64}
SERVICE_NAME=tidewatch

# Stop existing
sudo systemctl stop $SERVICE_NAME 2>/dev/null || true
sudo systemctl disable $SERVICE_NAME 2>/dev/null || true

# Install
sudo cp $BINARY /usr/local/bin/tidewatch
sudo chmod +x /usr/local/bin/tidewatch

# Directories
sudo mkdir -p /var/lib/tidewatch
sudo mkdir -p /etc/tidewatch

# Config
[ ! -f /etc/tidewatch/config.yaml ] && \
    sudo cp configs/config.yaml /etc/tidewatch/config.yaml

# Service
sudo cp systemd/tidewatch.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable $SERVICE_NAME
sudo systemctl start $SERVICE_NAME

sudo systemctl status $SERVICE_NAME --no-pager
```

### Task 6: Systemd Service

**systemd/tidewatch.service:**
```ini
[Unit]
Description=Belabox Metrics Collector
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/tidewatch -config /etc/tidewatch/config.yaml
Restart=always
RestartSec=10s
StandardOutput=journal
StandardError=journal
MemoryLimit=300M
CPUQuota=15%

[Install]
WantedBy=multi-user.target
```

### Task 7: Simple Receiver

**cmd/metrics-receiver/main.go:**
```go
package main

import (
    "encoding/json"
    "log"
    "net/http"
)

type MetricsPayload struct {
    DeviceID  string   `json:"device_id"`
    Timestamp string   `json:"timestamp"`
    Metrics   []Metric `json:"metrics"`
}

type Metric struct {
    Name      string  `json:"name"`
    Value     float64 `json:"value"`
    Timestamp string  `json:"timestamp"`
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
    var payload MetricsPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    log.Printf("[%s] %s: %d metrics", payload.Timestamp, payload.DeviceID, len(payload.Metrics))
    for _, m := range payload.Metrics {
        log.Printf("  %s = %.2f @ %s", m.Name, m.Value, m.Timestamp)
    }

    json.NewEncoder(w).Encode(map[string]interface{}{
        "success": true,
        "received": len(payload.Metrics),
    })
}

func main() {
    http.HandleFunc("/api/metrics", handleMetrics)
    log.Println("Listening on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

---

## Testing Plan

### Local (macOS)
```bash
./scripts/build.sh
./bin/metrics-receiver &
./bin/tidewatch-darwin -config configs/config.yaml
sqlite3 /var/lib/tidewatch/metrics.db "SELECT * FROM metrics LIMIT 10"
```

### Orange Pi
```bash
./scripts/build.sh
scp bin/tidewatch-linux-arm64 configs systemd scripts user@orangepi:/tmp/
ssh user@orangepi
cd /tmp && sudo ./scripts/install.sh
sudo systemctl status tidewatch
journalctl -u tidewatch -f
```

---

## Acceptance Checklist

- [ ] Compiles on macOS
- [ ] Cross-compiles to ARM64
- [ ] Runs on macOS with mocks
- [ ] Creates SQLite database with WAL mode
- [ ] Can query and see metrics with timestamps
- [ ] HTTP POST sends metrics to receiver
- [ ] Receiver logs show metrics arriving
- [ ] Install script works on Orange Pi
- [ ] Service starts successfully
- [ ] Service enabled for boot
- [ ] Real CPU temp on Pi (from thermal zones)
- [ ] Mock SRT packet loss data generating
- [ ] Restarts after crash (kill -9 test)
- [ ] Starts on reboot
- [ ] Logs via journalctl visible
- [ ] docs/belabox-integration.md exists ✅

---

## Timeline

**Day 1 (6-8 hours)**
- Hour 1: Investigate Belabox
- Hour 2-3: Init project, config parsing
- Hour 4-5: Collectors (CPU + SRT)
- Hour 6: SQLite storage
- Hour 7: HTTP uploader
- Hour 8: Main loop, local testing

**Day 2 (4-6 hours)**
- Hour 1: Build scripts, systemd
- Hour 2: Install script
- Hour 3: Deploy to Pi, debug
- Hour 4: End-to-end testing
- Hour 5-6: Documentation, cleanup

---

## Next Steps After Completion

Once successful, move to Milestone 2:
- Add more system metrics (RAM, disk, network)
- Implement retry logic
- Basic error handling
- Improved logging
