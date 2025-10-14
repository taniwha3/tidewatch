## Success Criteria

Milestone 2 is complete when:

1. ✅ No duplicate uploads in 30-minute soak test (verified with dedup_key)
2. ✅ Same batch retried twice → 0 new rows (unique constraint test)
3. ✅ VictoriaMetrics ingesting metrics successfully
4. ✅ Can query metrics from VictoriaMetrics UI with PromQL
5. ✅ 8+ system metrics collecting reliably
6. ✅ CPU usage with proper delta calculation (no first-sample, wraparound handling)
7. ✅ Network metrics exclude lo, docker*, veth*, br-*
8. ✅ Disk I/O in bytes (not sectors)
9. ✅ Upload retries work with jittered backoff (tested with network simulation)
10. ✅ Partial success handled correctly (VM accepts 25/50 → only 25 marked uploaded)
11. ✅ Health endpoint returns graduated status (ok/degraded/error)
12. ✅ Clock skew detected and exposed in meta-metrics
13. ✅ WAL checkpoint runs and keeps WAL < 64 MB
14. ✅ Meta-metrics visible in VictoriaMetrics
15. ✅ Structured logs in JSON format with all required fields
16. ✅ All tests pass (80+ unit tests expected)
17. ✅ Resource usage <5% CPU, <150MB RAM
18. ✅ Docker Compose brings up full stack easily
19. ✅ Security hardening in place (non-root, protections, limits)
20. ✅ Documentation complete (health semantics, PromQL examples, per-zone temps)

