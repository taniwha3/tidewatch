# Docker Setup for Local Testing

This directory contains Docker Compose configuration for running Victor iaMetrics and Grafana locally for testing the metrics collection pipeline.

## Quick Start

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f victoria

# Stop all services
docker-compose down

# Stop and remove volumes (wipes all data)
docker-compose down -v
```

## Services

### VictoriaMetrics

- **Web UI**: http://localhost:8428/vmui
- **Import API**: http://localhost:8428/api/v1/import
- **Query API**: http://localhost:8428/api/v1/query
- **Health Check**: http://localhost:8428/health

VictoriaMetrics stores metrics data for 30 days (configurable in docker-compose.yml).

### Grafana (Optional Visualization)

- **Web UI**: http://localhost:3000
- **Default Credentials**: admin / admin
- **Datasource**: VictoriaMetrics (auto-configured)

Grafana is pre-configured to connect to VictoriaMetrics for querying and visualization.

## Testing Metrics Upload

Once VictoriaMetrics is running, you can test the upload functionality:

```bash
# Run tests against local VictoriaMetrics
go test ./internal/uploader/... -v

# Send test metrics manually using curl
gzip -c testdata.jsonl | curl -X POST \
  -H "Content-Type: application/x-ndjson" \
  -H "Content-Encoding: gzip" \
  -H "X-Device-ID: test-device-001" \
  --data-binary @- \
  http://localhost:8428/api/v1/import
```

## Example Queries

Once metrics are uploaded, you can query them via the VictoriaMetrics UI or API:

```bash
# Query CPU temperature
curl 'http://localhost:8428/api/v1/query?query=cpu_temperature_celsius'

# Query memory usage
curl 'http://localhost:8428/api/v1/query?query=memory_bytes_used_bytes{device_id="device-001"}'

# Query with time range
curl 'http://localhost:8428/api/v1/query_range?query=cpu_temperature_celsius&start=-1h&step=1m'
```

## Data Persistence

Metrics data is persisted in Docker volumes:
- `tidewatch-victoria-data`: VictoriaMetrics storage
- `tidewatch-grafana-data`: Grafana dashboards and settings

To completely remove all data:
```bash
docker-compose down -v
```

## Troubleshooting

### Container won't start

Check logs:
```bash
docker-compose logs victoria
docker-compose logs grafana
```

### Port already in use

If ports 8428 or 3000 are already in use, modify the port mappings in `docker-compose.yml`:

```yaml
ports:
  - "9428:8428"  # Use port 9428 instead
```

### Cannot connect from host

Ensure containers are running:
```bash
docker-compose ps
```

Verify network connectivity:
```bash
curl http://localhost:8428/health
```

## References

- [VictoriaMetrics Documentation](https://docs.victoriametrics.com/)
- [VictoriaMetrics Import API](https://docs.victoriametrics.com/#how-to-import-time-series-data)
- [Grafana Documentation](https://grafana.com/docs/)
