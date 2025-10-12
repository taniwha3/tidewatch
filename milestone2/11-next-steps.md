## Next Steps After Completion

Once Milestone 2 is complete, move to Milestone 3:
- Belabox/encoder metrics (journald parsing)
- HDMI input metrics (v4l2-ctl)
- Server-side SRT stats (when receiver available)
- Priority queue (P0/P1/P2/P3 upload ordering)
- Backfill after network recovery
- Data retention and rotation policies
- Grafana dashboard creation
- Deploy to Orange Pi for real hardware testing
- TLS certificate pinning
- **Clock skew detection refinements:**
  - Migrate from `log.Printf` to structured logging (log/slog)
  - Integrate periodic checking routine (5min interval) into main collector loop
  - Add clock skew warnings to health endpoint status
  - Make clock skew collector auto-discover auth token from uploader config

