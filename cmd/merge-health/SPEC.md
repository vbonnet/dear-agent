# Merge Health Specification

<!-- Last audited at: 2026-09-01 -->

## Purpose

`cmd/merge-health` reports whether the merge pipeline has produced its
positive event: at least one commit landing on a tracked ref (default
`origin/main`) within a lookback window. It is a sibling of
`cmd/jaeger-health`, deliberately copying its shape: a standalone,
no-side-effect probe with the exit-code contract 0 healthy / 1 degraded /
2 down / 3 usage, a `--json` report, and a `--lookback` window - so a
generic scheduler-and-sink layer (`cmd/absence-alarm`) can register it as a
command pulse without bespoke logic.

The 2026-09-01 absence-blindness meta-retro sharpened why this exists as a
binary rather than a shell one-liner in a config file. The fleet did not
lack absence detectors: `jaeger-health` implemented exactly the right check
(alive but no traces in the lookback window reports degraded, exit 1) while
the OTel stack sat dark for 46 days, because nothing ran it and no sink
consumed its exit code. The durable pattern is therefore: absence checks
are small, spec-pinned, testable binaries with one shared exit contract,
and one registry (the absence-alarm pulse set) guarantees each is run on a
schedule and escalated on failure. Between 2026-08-27 and 09-01 the merge
pipeline produced one Dependabot merge in five days while 32 draft PRs
piled up, and nothing said so; this check makes that silence alarmable.

A deliberate property: the check reads the local clone's view of the remote
ref. If the fetch loop (git-auto-sync) dies, the ref stops moving and this
check degrades even though GitHub may have new commits. That is correct
absence semantics, not a false positive - a dead fetch loop is itself an
absent positive event in the same pipeline - and the report carries the
last-fetch age so a responder can tell the two apart at a glance.

## Shared absence-probe contract

The status vocabulary, exit codes, and JSON envelope in MH-01..MH-08 are not
merge-specific: they are the generic absence-probe interface a scheduler
consumes. `pkg/absencealarm/SPEC.md` and `cmd/jaeger-health/SPEC.md` define
that shared exit contract: exit 0 healthy, 1 degraded, 2 down, 3 usage, and
a single JSON report shape. The requirements below are the merge-applicability
scoping of it: what counts as the positive event, what the lookback means
for this pipeline, and what the last-fetch age adds. A change to the shared
vocabulary belongs in the absence-alarm contract, not here, so the sibling
probes cannot drift apart.

## EARS Requirements

**MH-01** When at least one commit exists on the tracked ref with a commit time inside the lookback window, the system shall report healthy and exit 0.

**MH-02** When the tracked ref resolves but no commit on it has a commit time inside the lookback window, the system shall report degraded with the tip commit's hash and age and exit 1.

**MH-03** When the repository cannot be read or the tracked ref cannot be resolved, the system shall report down with the underlying error and exit 2.

**MH-04** When the lookback window cannot be parsed or is not positive, the system shall report a usage error and exit 3.

**MH-05** When the tip commit's time is more than the clock-skew tolerance (5 minutes) in the future, the system shall report down rather than healthy, because a future timestamp is not proof of a live pipeline.

**MH-06** When JSON output mode is set, the system shall emit a single JSON report carrying status, ref, tip commit hash, tip commit time, tip age, lookback, and the age of the last fetch when it is known.

**MH-07** When the repository's FETCH_HEAD is readable, the system shall include the time since the last fetch in the report as advisory context, and its absence shall not change the status.

**MH-08** The system shall not mutate the repository: it shall run no fetch, no pull, and no write of any kind.

**MH-09** When the probe runs any subprocess, the system shall bound every one of them with a single tick deadline so that a hung git invocation reports a bounded failure instead of stalling the scheduler that invoked it.

## BDD Traceability

- Feature: `agm/test/bdd/features/observability_package_guardrails.feature`
- Package tests: `cmd/merge-health/main_test.go`
