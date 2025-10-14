## PromQL Query Examples

**For use with VictoriaMetrics UI (http://localhost:8428) or Grafana**

### Network Throughput

```promql
# Network upload rate (bytes per second) over last minute
rate(network_tx_bytes_total{device_id="belabox-001"}[1m])

# Network download rate
rate(network.rx_bytes_total{device_id="belabox-001"}[1m])

# Total network throughput (upload + download)
sum(rate(network_tx_bytes_total{device_id="belabox-001"}[1m])) +
sum(rate(network.rx_bytes_total{device_id="belabox-001"}[1m]))
```

### CPU Usage

```promql
# Overall CPU usage (aggregated across all cores)
cpu.usage_percent{device_id="belabox-001", core="all"}

# Per-core CPU usage
cpu.usage_percent{device_id="belabox-001", core!="all"}

# Average CPU usage across all cores
avg(cpu.usage_percent{device_id="belabox-001", core!="all"})

# Max CPU usage (hottest core)
max(cpu.usage_percent{device_id="belabox-001", core!="all"})
```

### Memory Usage

```promql
# Memory usage percentage
(memory.used_bytes{device_id="belabox-001"} / memory.total_bytes{device_id="belabox-001"}) * 100

# Available memory in MB
memory.available_bytes{device_id="belabox-001"} / 1024 / 1024

# Swap usage
memory.swap_used_bytes{device_id="belabox-001"} / 1024 / 1024
```

### Disk I/O

```promql
# Disk read rate (bytes per second)
rate(disk_read_bytes_total{device_id="belabox-001"}[1m])

# Disk write rate
rate(disk.write_bytes_total{device_id="belabox-001"}[1m])

# Disk IOPS (reads + writes)
rate(disk_read_ops_total{device_id="belabox-001"}[1m]) +
rate(disk.write_ops_total{device_id="belabox-001"}[1m])
```

### Thermal Monitoring

```promql
# Max temperature across all thermal zones (RK3588)
max_over_time(thermal.zone_temp_c{device_id="belabox-001"}[5m])

# Temperature by zone
thermal.zone_temp_c{device_id="belabox-001"}

# Check if any zone exceeds 85°C
max(thermal.zone_temp_c{device_id="belabox-001"}) > 85
```

### Meta-Metrics (Collector Health)

```promql
# Metrics collected per second
rate(collector.metrics_collected_total{device_id="belabox-001"}[1m])

# Upload success rate
rate(uploader.metrics_uploaded_total{device_id="belabox-001"}[5m]) /
(rate(uploader.metrics_uploaded_total{device_id="belabox-001"}[5m]) +
 rate(uploader.upload_failures_total{device_id="belabox-001"}[5m]))

# Pending metrics (backlog)
storage.metrics_pending_upload{device_id="belabox-001"}

# Database size growth rate
deriv(storage.database_size_bytes{device_id="belabox-001"}[1h])

# Time skew (clock drift)
time.skew_ms{device_id="belabox-001"}
```

### Alerting Queries

```promql
# High CPU usage alert (>90% for 5 minutes)
cpu.usage_percent{device_id="belabox-001", core="all"} > 90

# Low memory alert (<100 MB available)
memory.available_bytes{device_id="belabox-001"} < 100*1024*1024

# Thermal throttling risk (>85°C)
max(thermal.zone_temp_c{device_id="belabox-001"}) > 85

# Upload failures (>10% failure rate)
rate(uploader.upload_failures_total{device_id="belabox-001"}[5m]) /
(rate(uploader.metrics_uploaded_total{device_id="belabox-001"}[5m]) +
 rate(uploader.upload_failures_total{device_id="belabox-001"}[5m])) > 0.1

# Large clock skew (>2 seconds)
abs(time.skew_ms{device_id="belabox-001"}) > 2000
```

### Dashboard Queries

```promql
# Overall system health score (0-100)
100 - (
  (cpu.usage_percent{device_id="belabox-001", core="all"} * 0.3) +
  ((memory.used_bytes / memory.total_bytes) * 100 * 0.3) +
  ((max(thermal.zone_temp_c) / 100) * 100 * 0.2) +
  ((storage.metrics_pending_upload / 10000) * 100 * 0.2)
)

# Metrics ingestion rate (metrics per minute)
sum(rate(collector.metrics_collected_total{device_id="belabox-001"}[1m])) * 60
```

