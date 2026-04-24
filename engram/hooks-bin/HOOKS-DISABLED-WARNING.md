# Health Hook Status

## Status: ✅ RE-ENABLED (as of 2026-02-20)

**Previous Status**: DISABLED (2026-02-11 to 2026-02-19)

The following hooks have been disabled on workstations due to performance issues during system stress:

### token-tracker-init
- **Impact:** 85.7% CPU during system recovery
- **Disabled at:** ~/.claude/hooks/session-start/token-tracker-init.disabled
- **Issue:** Runs on every Claude session start, competes with recovery processes

### engram-health-hook
- **Previous Impact:** 31.5% CPU during system recovery (v1)
- **Disabled:** 2026-02-11 at ~/bin/engram-health-hook.sh.disabled
- **Re-enabled:** 2026-02-20 with v2 (stress-resistant version)
- **Current Status:** ✅ Active at ~/bin/engram-health-hook.sh
- **V2 Improvements:**
  - Stress detection: Skips if load >2x CPUs (19ms exit)
  - Rate limiting: Max 1 run per 5 minutes
  - 97% CPU reduction during crash loops
  - See: hooks/HEALTH-HOOK-V2-IMPROVEMENTS.md

## Root Cause
During memory exhaustion incidents, sessions crash and restart rapidly.
Hooks running on every restart amplify the crisis instead of allowing recovery.

## Re-enabling
Before re-enabling:
1. Fix test process lifecycle (main issue causing crashes)
2. Add resource limits to hooks (timeout, max CPU)
3. Test hooks during simulated stress conditions
4. Monitor for 24 hours after re-enabling

## Alternative Approach
Consider:
- Running hooks asynchronously (dont block session start)
- Detecting system stress and skipping hooks
- Rate limiting hooks (dont run if last run was <5m ago)

## Related
- Incident reports in workstation:~/*.md
- Monitoring: ~/MONITORING_GUIDE.md
