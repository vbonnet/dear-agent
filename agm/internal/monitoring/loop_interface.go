package monitoring

// Loop Skill Interface Protocol
//
// Any skill that wants heartbeat monitoring follows this protocol:
//
// 1. At the START of each loop cycle, the skill calls:
//      agm heartbeat write <session-name> --interval <seconds>
//    This writes a heartbeat file to ~/.agm/heartbeats/loop-<session>.json
//
// 2. The orchestrator registers itself as a monitor for sessions it watches:
//      agm session add-monitor <watched-session> <monitor-session>
//    This tells the daemon which sessions to check when a heartbeat goes stale.
//
// 3. The daemon handles the rest:
//    - Polls loop-*.json heartbeat files every ~15s (CheckAllSessions cycle)
//    - Detects staleness: now - timestamp > interval + 60s
//    - Sends wake via: agm send wake-loop <monitor-session>
//    - Circuit breaker: 3 attempts with 2min cooldown, then escalation
//
// Example adoption in /orchestrate skill:
//
//   # At start of each orchestration cycle:
//   agm heartbeat write orchestrator-v2 --interval 300 --cycle $CYCLE_NUM
//
//   # One-time registration (e.g., in session setup):
//   agm session add-monitor worker-1 orchestrator-v2
//   agm session add-monitor worker-2 orchestrator-v2
//
// Deregistration:
//   - Automatic: when a session is archived, it is removed from all monitor lists
//   - Manual: agm session remove-monitor <session> <monitor-session>
//   - Heartbeat file cleanup: agm heartbeat files are left on disk for forensics
//     but can be manually cleaned with: rm ~/.agm/heartbeats/loop-<session>.json
