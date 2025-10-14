# Milestone 2: Production Reliability + VictoriaMetrics

**Timeline:** 3-4 days (Days 3-6)
**Status:** In Progress
**Goal:** Fix critical bugs, add system metrics, integrate VictoriaMetrics, production-grade reliability

---

## Quick Navigation

This directory contains the Milestone 2 specification broken down into manageable sections:

### Planning & Overview
- **[00-header.md](00-header.md)** - Title and metadata
- **[01-overview.md](01-overview.md)** - High-level goals and overview
- **[02-changes-from-original.md](02-changes-from-original.md)** - Critical additions from technical review
- **[03-scope.md](03-scope.md)** - What's in scope and out of scope (5.1 KB)
- **[04-success-criteria.md](04-success-criteria.md)** - How we measure success

### Technical Details
- **[05-technical-specification.md](05-technical-specification.md)** - Detailed technical spec (65 KB) ⚠️ Large file
- **[06-implementation-plan.md](06-implementation-plan.md)** - Step-by-step implementation guide
- **[07-testing-strategy.md](07-testing-strategy.md)** - Testing approach and test cases (10 KB)

### Checklists & References
- **[08-acceptance-checklist.md](08-acceptance-checklist.md)** - Pre-merge checklist
- **[09-promql-examples.md](09-promql-examples.md)** - PromQL query examples for testing
- **[10-troubleshooting.md](10-troubleshooting.md)** - Common issues and solutions

### Project Management
- **[11-next-steps.md](11-next-steps.md)** - What comes after M2
- **[12-timeline.md](12-timeline.md)** - Estimated timeline breakdown
- **[13-engineering-review.md](13-engineering-review.md)** - Code review findings and fixes (24 KB)

---

## Quick Start

**To implement M2, start here:**

1. Read **[13-engineering-review.md](13-engineering-review.md)** - See "Ready-to-Merge Checklist" for actionable items
2. Review **[03-scope.md](03-scope.md)** - Understand what we're building
3. Follow **[06-implementation-plan.md](06-implementation-plan.md)** - Step-by-step implementation
4. Use **[08-acceptance-checklist.md](08-acceptance-checklist.md)** - Track progress

**Most Important Files:**
- `13-engineering-review.md` - Contains the Ready-to-Merge Checklist (your TODO list)
- `05-technical-specification.md` - Detailed implementation specs
- `07-testing-strategy.md` - All test cases needed

---

## Current Status

**Recently Completed:**
- ✅ Migration backfill for dedup_key (v3 migration)
- ✅ Hash collision fix (JSON-based dedup key)
- ✅ Bytes suffix sanitization fix

**Next Priority Items (from 13-engineering-review.md):**
1. HTTP Upload Headers - Fix signature + add required headers
2. WAL Checkpoint - Uniform `Exec` approach
3. Disk Sector Size - Per-device sysfs read with cache
4. ~~Name Sanitization~~ ✅ DONE
5. Clock Skew Auth - Reuse token + configurable endpoint

---

## File Size Reference

- Small files (<1 KB): 00, 01, 11, 12
- Medium files (1-10 KB): 02, 03, 04, 06, 07, 08, 09, 10
- Large files (>10 KB): 05 (65 KB), 13 (24 KB)

**Note:** The original `MILESTONE-2.md` (4,022 lines) remains unchanged for reference.
