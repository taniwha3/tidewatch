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
- Single Go binary (`metrics-collector`)
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

## First Priority: Investigate Belabox

**BEFORE writing any code**, run this investigation on the Orange Pi:

```bash
# SSH to Orange Pi
ssh user@orangepi.local

# Check for HTTP endpoints
netstat -tlnp | grep -E ':(808|900)'
ss -tlnp

# Look for stats files
find /var -name "*stats*" 2>/dev/null
find /tmp -name "*srt*" 2>/dev/null

# Check Belabox logs  
journalctl -u belabox* --no-pager | tail -100
ls -la /var/log/ | grep bela

# Check running processes
ps aux | grep -E 'bela|srt'
systemctl list-units | grep bela
```

**Time budget:** 30 minutes max  
**Document findings in:** `docs/belabox-integration.md`

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

#### 2. SRT Packet Loss

**Mock (fallback):**
```go
func getSRTPacketLoss() float64 {
    if rand.Float64() < 0.1 {
        return rand.Float64() * 5.0  // 0-5% loss
    }
    return 0.0
}
```

**Real (if found):** Based on investigation findings

**Interval:** 5 seconds

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
CREATE TABLE IF NOT EXISTS metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value REAL,
    device_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_timestamp ON metrics(timestamp);
```

### Config File

**Path:** `/etc/belabox-metrics/config.yaml`

```yaml
device:
  id: belabox-001

storage:
  path: /var/lib/belabox-metrics/metrics.db

remote:
  url: http://example.com:8080/api/metrics
  enabled: true

metrics:
  - name: cpu.temperature
    interval: 30s
    enabled: true

  - name: srt.packet_loss
    interval: 5s
    enabled: true
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
thugshells/
├── cmd/
│   ├── metrics-collector/
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
│   └── metrics-collector.service
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

### Task 1: Investigate Belabox (30 min)
See "First Priority" section above.

### Task 2: Initialize Project
```bash
go mod init github.com/taniwha3/thugshells
mkdir -p cmd/metrics-collector cmd/metrics-receiver
mkdir -p internal/{config,collector,storage,uploader}
mkdir -p configs scripts systemd docs
```

### Task 3: Core Implementation
1. `internal/config/config.go` - YAML parsing
2. `internal/collector/collector.go` - Metric collection  
3. `internal/storage/sqlite.go` - Database ops
4. `internal/uploader/http.go` - HTTP POST
5. `cmd/metrics-collector/main.go` - Main loop

### Task 4: Build Scripts

**scripts/build.sh:**
```bash
#!/bin/bash
set -e

echo "Building for macOS..."
GOOS=darwin GOARCH=amd64 go build -o bin/metrics-collector-darwin \
    cmd/metrics-collector/main.go

echo "Building for Orange Pi (ARM64)..."
GOOS=linux GOARCH=arm64 go build -o bin/metrics-collector-linux-arm64 \
    cmd/metrics-collector/main.go

echo "Building receiver..."
go build -o bin/metrics-receiver cmd/metrics-receiver/main.go

ls -lh bin/
```

### Task 5: Install Script

**scripts/install.sh:**
```bash
#!/bin/bash
set -e

BINARY=${1:-bin/metrics-collector-linux-arm64}
SERVICE_NAME=metrics-collector

# Stop existing
sudo systemctl stop $SERVICE_NAME 2>/dev/null || true
sudo systemctl disable $SERVICE_NAME 2>/dev/null || true

# Install
sudo cp $BINARY /usr/local/bin/metrics-collector
sudo chmod +x /usr/local/bin/metrics-collector

# Directories
sudo mkdir -p /var/lib/belabox-metrics
sudo mkdir -p /etc/belabox-metrics

# Config
[ ! -f /etc/belabox-metrics/config.yaml ] && \
    sudo cp configs/config.yaml /etc/belabox-metrics/config.yaml

# Service
sudo cp systemd/metrics-collector.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable $SERVICE_NAME
sudo systemctl start $SERVICE_NAME

sudo systemctl status $SERVICE_NAME --no-pager
```

### Task 6: Systemd Service

**systemd/metrics-collector.service:**
```ini
[Unit]
Description=Belabox Metrics Collector
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/metrics-collector -config /etc/belabox-metrics/config.yaml
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
./bin/metrics-collector-darwin -config configs/config.yaml
sqlite3 /var/lib/belabox-metrics/metrics.db "SELECT * FROM metrics LIMIT 10"
```

### Orange Pi
```bash
./scripts/build.sh
scp bin/metrics-collector-linux-arm64 configs systemd scripts user@orangepi:/tmp/
ssh user@orangepi
cd /tmp && sudo ./scripts/install.sh
sudo systemctl status metrics-collector
journalctl -u metrics-collector -f
```

---

## Acceptance Checklist

- [ ] Compiles on macOS
- [ ] Cross-compiles to ARM64
- [ ] Runs on macOS with mocks
- [ ] Creates SQLite database
- [ ] Can query and see metrics
- [ ] HTTP POST sends metrics
- [ ] Receiver logs show metrics
- [ ] Install script works on Pi
- [ ] Service starts successfully
- [ ] Service enabled for boot
- [ ] Real CPU temp on Pi
- [ ] Restarts after crash
- [ ] Starts on reboot
- [ ] Logs via journalctl
- [ ] docs/belabox-integration.md created

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
