// Package recoveryloop implements the self-healing recovery loop (ce-a1uqr)
// for critical fleet daemons and background jobs.
//
// It consumes the absence-alarm escalation journal, evaluates host service
// and binary health against a registry of critical jobs, enforces expiring
// snooze policies, executes bounded remediation actions (reinstall, bootstrap,
// kickstart), tracks consecutive failure escalation, and journals every recovery
// attempt to ensure that self-healing remains observable.
package recoveryloop
