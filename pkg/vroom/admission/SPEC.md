# Admission Brake Requirements Specification (EARS)

<!-- Last audited at: 2026-07-21 -->

**Version**: 1.0
**Status**: Active
**Scope**: Cross-process spawn brake shared by host watchdogs and every AGM spawn path.

## Purpose

The brake is the consumer that was missing on 2026-07-18 (ce-93lw.18). Detection
was never the gap: `disk-watchdog` had root at 96.2% used / 17.6 GiB free, in
ALARM, with `agm worktree sweep --execute: signal: killed` in every remediation
slot — the remediation path being killed by the exhaustion it existed to relieve.
That went to a JSONL trail no spawn path reads, and the mesh kept spawning until
the host had to be power-cycled.

A file rather than a daemon, deliberately. The failure being guarded against is
processes dying under resource pressure, so a latch already on disk keeps
refusing spawns after every writer is dead.

## EARS Requirements

**VROOM-BRAKE-01** The system shall persist the brake as a JSON record carrying a source, a reason, a set-at timestamp, and an expiry timestamp.

**VROOM-BRAKE-02** The system shall resolve the default brake path to `$AGM_CONFIG_DIR/admission-brake.json`, falling back to `~/.agm/admission-brake.json`.

**VROOM-BRAKE-03** When a brake is engaged, the system shall write the record atomically so that no reader observes a partially written file.

**VROOM-BRAKE-04** When a brake is engaged, the system shall restrict the record file to owner-only permissions.

**VROOM-BRAKE-05** When a brake is engaged with a non-positive time-to-live, the system shall apply the default time-to-live rather than writing a record that is already expired.

**VROOM-BRAKE-06** When a brake is engaged over an existing brake, the system shall replace the existing record.

**VROOM-BRAKE-07** If the brake record file does not exist, then the system shall report that no brake is in force and shall not report an error.

**VROOM-BRAKE-08** If the brake record has passed its expiry timestamp, then the system shall report that no brake is in force and shall not report an error.

**VROOM-BRAKE-09** If the brake record cannot be read or decoded, then the system shall return an error so that callers refuse the spawn.

**VROOM-BRAKE-10** If the brake record carries no expiry timestamp, then the system shall return an error rather than treating the record as permanent.

**VROOM-BRAKE-11** When an unconditional release is requested, the system shall remove the brake record file whichever source engaged it.

**VROOM-BRAKE-12** When a release is requested and no brake record file exists, the system shall report success.

**VROOM-BRAKE-13** When a source-scoped release names the source that engaged the brake, the system shall remove the brake record file.

**VROOM-BRAKE-14** If a source-scoped release names a source other than the one that engaged the brake, then the system shall leave the brake record file in place.

**VROOM-BRAKE-15** If a source-scoped release encounters a record it cannot read, then the system shall leave the record file in place and return an error.

## Design Notes

**Absent means healthy.** The common case costs one failed `open(2)`. This is
deliberately not a required-heartbeat design: "no fresh healthy record means
block" would wedge the host the first time launchd is disabled.

**Unreadable means engaged.** A brake we cannot read is not evidence of health,
so decode failures surface as errors and every caller treats them as a refusal.

**Every brake expires.** A fail-closed latch with no time-to-live converts one
transient probe failure into a permanently dead mesh.

**Releases are source-scoped.** `vroom-governor` ticks every 30 seconds and
`disk-watchdog` every 5 minutes. With an unconditional release, a governor whose
load and memory probes were perfectly healthy would clear a disk-watchdog brake
within half a minute of it being engaged — silently defeating the fix on its
likeliest path, a host out of disk but not out of CPU. `Release` remains
unconditional as the operator escape hatch.

`ReleaseBySource` carries a bounded read-then-remove race: a brake engaged
between the read and the remove is dropped. It is self-correcting, since each
watchdog re-engages on its next tick, and closing it would need file locking that
buys less than it costs on a single-host latch.

## Test Traceability

- Package tests: `pkg/vroom/admission/brake_test.go`
- Gate tests: `agm/internal/circuitbreaker/brake_test.go`
- Writer tests: `cmd/disk-watchdog/main_test.go`, `cmd/vroom-governor/main_test.go`
- BDD: `agm/test/bdd/features/vroom_runtime_guardrails.feature`
