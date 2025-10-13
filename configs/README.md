# Configuration Files

## Production Configuration

**`config.yaml`** - Production configuration for systemd deployments
- Uses absolute path: `/var/lib/belabox-metrics/metrics.db`
- Standard intervals (30s for most metrics)
- Info-level logging
- Requires proper permissions on `/var/lib/belabox-metrics/`

## Development Configuration

**`config.dev.yaml`** - Local development configuration
- Uses relative path: `./data/metrics/metrics.db` (auto-created)
- Faster intervals (10s) for quicker testing
- Debug-level logging for more visibility
- Disabled `disk.io` collector (doesn't work on macOS)
- Smaller WAL checkpoint threshold (8MB vs 64MB)

### Usage

```bash
# Run with development config
./bin/metrics-collector-darwin -config configs/config.dev.yaml

# Or with production config
./bin/metrics-collector-darwin -config configs/config.yaml
```

### Development Tips

1. The `./data/` directory is git-ignored, so your local database won't be committed
2. You can customize `config.dev.yaml` for your local needs
3. VictoriaMetrics should be running on `localhost:8428`
4. Health endpoint will be available on `localhost:9100`

### Creating the Production Directory

For production deployments, create the storage directory with proper permissions:

```bash
sudo mkdir -p /var/lib/belabox-metrics
sudo chown belabox:belabox /var/lib/belabox-metrics
sudo chmod 755 /var/lib/belabox-metrics
```

The collector will auto-create subdirectories if they don't exist (assuming parent directory permissions allow it).
