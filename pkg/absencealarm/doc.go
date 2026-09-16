// Package absencealarm is the domain layer of the absence alarm: the
// registry + scheduler + sink decomposition for alarming on the ABSENCE of
// expected positive events, not the presence of errors.
//
// Every silent multi-week outage of 2026-07/08 (mergeloop disabled 41 days,
// OTel dark 46 days, supervisors down 45 days, sandbox GC failing hourly
// for a month into an unread log) was invisible because a dead process
// emits no errors. The fleet did not lack detectors - cmd/jaeger-health
// implemented textbook absence detection and sat unused for the whole OTel
// outage - it lacked a layer that guarantees every such check runs and
// escalates. This package provides that layer's model:
//
//   - Pulse / LoadPulseConfig: the registry - declarative expected positive
//     events with maximum silence windows (EARS AA-01..AA-06, AA-19).
//   - EvaluatePulse / Probes: the probe seams - file_mtime, launchd_loaded,
//     and command pulses, where command pulses run jaeger-health-pattern
//     sibling binaries (exit 0 healthy / 1 degraded / 2 down / 3 usage).
//   - UpdateAlarm and friends: the sink - transition + backoff notification
//     decisions, expiring snoozes, escalation journal, self-heartbeat
//     (AA-09..AA-18).
//
// cmd/absence-alarm wires this model to flags, launchd, and the desktop
// notifier. See pkg/absencealarm/SPEC.md for the full EARS contract.
package absencealarm
