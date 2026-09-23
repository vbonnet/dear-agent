// Package recoveryloop implements the self-healing recovery loop (ce-a1uqr)
// for critical fleet daemons and background jobs.
//
// It reads current pulse truth from the absence-alarm HEARTBEAT, evaluates host
// service and binary health against a registry of critical jobs, enforces
// expiring snooze policies, executes bounded remediation actions (reinstall,
// bootstrap, kickstart), VERIFIES that the condition cleared before recording a
// recovery, tracks consecutive failures to clear, and journals every attempt so
// that self-healing remains observable.
//
// The absence-alarm escalation journal is an output, not an input: human-needed
// records are appended to it. It is append-only and records absences alone, so
// a pulse that recovered leaves no trace in it, and reading it as current state
// is what let this loop report success for 1243 consecutive ticks (RL-23).
package recoveryloop
