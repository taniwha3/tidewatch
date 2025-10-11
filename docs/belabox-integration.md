# Belabox Integration Strategy

**Status:** Researched
**Last Updated:** 2025-10-11
**Based on:** Technical review of Orange Pi 5+ (RK3588) + Belabox architecture

---

## Key Findings

### What Belabox Actually Is

- **Encoder:** Uses **GStreamer** (belacoder), NOT FFmpeg
- **SRT Transport:** Uses srtla_send for link aggregation
- **Deployment:** Runs as systemd services
- **Logging:** Via journald (systemd journal)

### Critical Constraint

**SRT socket stats are NOT accessible from external processes.**

The SRT library exposes rich statistics (`srt_bstats()`), but only to the owning process (belacoder/srtla_send). An external metrics collector cannot query these sockets directly.

---

## Implementation Strategy

### On-Device Metrics (Orange Pi)

#### 1. System Metrics ✅ Straightforward

**Source:** Direct file reads

```go
// CPU, RAM, disk
/proc/stat
/proc/meminfo
/proc/diskstats
/proc/net/dev

// Thermals (RK3588 specific)
/sys/class/thermal/thermal_zone*/type
/sys/class/thermal/thermal_zone*/temp  // millidegrees Celsius
```

**Thermal zones on RK3588:**
- SoC
- Big cores (Cortex-A76)
- Small cores (Cortex-A55)
- GPU
- NPU

#### 2. Encoder Metrics (GStreamer/belacoder)

**Source:** journald logs

**Method:** Tail journald for belacoder unit

```bash
journalctl -u belacoder -f -o json --no-pager
```

**What to extract:**
- **FPS:** GStreamer's `fpsdisplaysink` prints current/average FPS to stdout
- **Dropped frames:** Parse element stats messages
- **Bitrate:** Look for encoder bitrate reports in logs

**Implementation:**
```go
// Use exec.CommandContext to tail journal
cmd := exec.CommandContext(ctx, "journalctl",
    "-u", "belacoder",
    "-f", "-o", "json", "--no-pager")

// Parse JSON lines, extract MESSAGE field
// Regex for patterns like:
//   "current: 29.98 fps"
//   "dropped: 5 frames"
//   "bitrate: 8000 kbps"
```

**Why not link GStreamer libs?** Avoids CGO, simpler deployment, keeps binary pure Go.

#### 3. HDMI Input Metrics

**Source:** V4L2 (Video4Linux2)

**Method:** Shell out to `v4l2-ctl` (MVP), use ioctl later

```bash
v4l2-ctl --query-dv-timings
v4l2-ctl --all
```

**What to extract:**
- Input signal presence
- Resolution (width x height)
- FPS (from pixelclock / frame timings)
- Interlaced vs progressive

**Implementation (MVP):**
```go
cmd := exec.Command("v4l2-ctl", "--query-dv-timings")
output, _ := cmd.CombinedOutput()
// Parse lines:
//   "Active width: 1920"
//   "Active height: 1080"
//   "Pixelclock: 148500000 Hz"
```

**Future:** Use `VIDIOC_QUERY_DV_TIMINGS` / `VIDIOC_G_DV_TIMINGS` ioctls directly in Go.

#### 4. SRT Metrics - NOT AVAILABLE ON DEVICE

**Why:** srtla_send owns the SRT sockets; external processes can't query them.

**Solution:** Get SRT metrics server-side (see below).

---

### Server-Side Metrics (SRTLA Receiver)

#### SRT/SRTLA Statistics

**Source:** SRTLA receiver HTTP stats endpoint

**Example:** OpenIRL receiver exposes:
```
GET http://receiver-server:8080/stats/<play-id>
```

**Response includes:**
```json
{
  "rtt_ms": 125,
  "pkt_loss_pct": 0.5,
  "pkt_sent": 1234567,
  "pkt_received": 1234500,
  "pkt_retransmitted": 67,
  "jitter_ms": 4.2,
  "bandwidth_mbps": 8.5
}
```

**Implementation:**
- Run a separate collector (or extend main collector) that polls receiver stats API
- Join with device metrics using `session_id` / `device_id` / `play_id`
- Store as `srt.*` metrics with appropriate tags

**Why server-side is better:**
- Sees actual receive QoS after aggregation
- Includes reorder/NAK behavior (closer to viewer experience)
- Documented, stable HTTP interface

---

## Milestone 1 Decisions

### CPU Temperature
✅ **Use real data on Orange Pi:**
```go
func getCPUTemperature() (float64, error) {
    // Read /sys/class/thermal/thermal_zone0/temp
    data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
    if err != nil {
        return 0, err
    }
    millideg, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
    return float64(millideg) / 1000.0, nil
}
```

### SRT Packet Loss

**For Milestone 1:** Use **mock data** on device

**Rationale:**
- Real SRT stats require server-side collector (separate component)
- Don't want to block MVP on setting up receiver stats endpoint
- Can add server-side collector in Milestone 2

**Mock implementation:**
```go
func getSRTPacketLoss() float64 {
    // Simulate occasional packet loss
    if rand.Float64() < 0.1 {  // 10% chance
        return rand.Float64() * 5.0  // 0-5% loss
    }
    return 0.0
}
```

---

## GPU/VPU Metrics Reality Check

### What's Actually Available on RK3588

**GPU:**
- ✅ Temperature: `/sys/class/thermal/thermal_zone*` (look for "gpu" type)
- ✅ Frequency: `/sys/class/devfreq/fb000000.gpu/cur_freq`
- ❌ Utilization: Not reliably exposed in sysfs on RK3588

**VPU (Video Processing Unit):**
- ❌ Direct utilization metrics: Not available
- ✅ **Proxy metrics:**
  - Encoder throughput (FPS)
  - Frame drops
  - Queue depths (from GStreamer stats)

**Recommendation:** Track encoder performance metrics (FPS, drops, latency) as proxy for VPU health.

---

## Modem Metrics (Future)

**When hardware arrives:**

```bash
# Use ModemManager
mmcli -L                           # List modems
mmcli -m 0 --signal                # Signal strength
mmcli -b 0                         # Bearer (connection) status
```

**Metrics to extract:**
- Signal strength (RSSI dBm)
- Signal quality (RSRQ dB)
- Network type (3G/4G/5G)
- Connection status
- Data usage

**Implementation:** Shell out to `mmcli` or use D-Bus API.

---

## Investigation Checklist (Run on Orange Pi)

```bash
# 1. Check running services
systemctl list-units | grep -E 'bela|srt'

# 2. Check journald logs
journalctl -u belacoder --no-pager | tail -100
journalctl -u srtla_send --no-pager | tail -100

# 3. Look for stats files
find /var -name "*stats*" 2>/dev/null
find /tmp -name "*srt*" 2>/dev/null

# 4. Check network listeners
ss -tlnp | grep -E 'bela|srt'

# 5. Check thermal zones
ls -la /sys/class/thermal/
cat /sys/class/thermal/thermal_zone*/type
cat /sys/class/thermal/thermal_zone*/temp

# 6. Check V4L2 devices
ls -la /dev/video*
v4l2-ctl --list-devices
v4l2-ctl --query-dv-timings

# 7. Check GPU/devfreq
ls -la /sys/class/devfreq/
cat /sys/class/devfreq/fb000000.gpu/cur_freq
```

**Document findings:** Create investigation-results.txt with outputs

---

## Updated Milestone 1 Scope

### Metrics to Collect

1. **CPU Temperature** - Real (from thermal zones)
2. **SRT Packet Loss** - Mock (server-side in Milestone 2)

### Additional Easy Wins (if time permits)

3. **CPU Usage** - Real (from /proc/stat)
4. **Memory Available** - Real (from /proc/meminfo)
5. **Thermal Zone Temps** - Real (all RK3588 zones)

### Explicitly Deferred to Milestone 2

- Encoder metrics (journald parsing)
- HDMI input metrics (v4l2-ctl)
- Server-side SRT stats (receiver API)
- Modem metrics

---

## Code Structure for Belabox Integration

```go
// internal/collector/belabox.go (Milestone 2+)

type BelaboxCollector struct {
    journalCmd *exec.Cmd
    parser     *JournaldParser
}

func (c *BelaboxCollector) Collect(ctx context.Context) ([]models.Metric, error) {
    // Tail journalctl -u belacoder -f -o json
    // Parse FPS, drops, bitrate from log lines
    // Return as encoder.* metrics
}

// internal/collector/hdmi.go (Milestone 2+)

type HDMICollector struct{}

func (c *HDMICollector) Collect(ctx context.Context) ([]models.Metric, error) {
    // Run v4l2-ctl --query-dv-timings
    // Parse resolution, fps, signal presence
    // Return as hdmi.* metrics
}

// internal/collector/srt_receiver.go (Milestone 2+)

type SRTReceiverCollector struct {
    receiverURL string
    httpClient  *http.Client
}

func (c *SRTReceiverCollector) Collect(ctx context.Context) ([]models.Metric, error) {
    // GET http://receiver/stats/<play-id>
    // Parse JSON response
    // Return as srt.* metrics
}
```

---

## Summary

**For Milestone 1:**
- ✅ CPU temperature: Real data from thermal zones
- ✅ System metrics: CPU, RAM (bonus if time)
- ⏸️ SRT packet loss: Mock data (server-side in Milestone 2)

**For Milestone 2:**
- Encoder metrics via journald parsing
- HDMI metrics via v4l2-ctl
- Server-side SRT stats from receiver API

**For Milestone 3+:**
- Modem metrics (when hardware arrives)
- GPU/VPU proxy metrics
- Advanced GStreamer integration

This approach keeps Milestone 1 achievable in 1-2 days while providing a clear path forward.
