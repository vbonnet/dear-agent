# Gate Health Command Specification

<!-- Last audited at: 2026-09-03 -->

## Overview

`cmd/gate-health` is an absence probe that reports whether one required check
is failing across a large share of the open pull-request queue. It is a sibling
of `cmd/jaeger-health` and `cmd/merge-health` and deliberately copies their
shape: a standalone, no-side-effect probe with the shared exit contract
0 healthy / 1 degraded / 2 down / 3 usage, a `--json` report, and no state of
its own.

That shared shape is the point. It lets `cmd/absence-alarm` register this probe
as a `command` pulse, schedule it, and escalate a non-zero exit to the desktop
without any gate-specific logic.

The detection rule lives in `pkg/gatehealth` and its rationale, calibration,
and the 2026-09-03 outage that motivated it are documented there. This command
owns only the boundary: flags, the forge query, the exit contract, and the
human summary.

## Shared absence-probe contract

The status vocabulary, exit codes, and JSON envelope are the generic
absence-probe interface a scheduler consumes; `pkg/absencealarm/SPEC.md` owns
that interface (PULSE-01..PULSE-04). This command conforms to it.

## EARS Requirements

Each requirement is a single line so the EARS validator sees the whole
sentence. Rationale follows as prose beneath it.

**GHC-01** When no check dominates the queue's failures, the command shall exit 0 and report `healthy`.

**GHC-02** When a systemic gate failure is present, the command shall exit 1 and emit a report naming the dominant check, its scope, and the likely fix.

**GHC-03** When the queue cannot be read, the command shall exit 2, report `down`, and carry the underlying cause in the report.

**GHC-04** When the queue cannot be read, the command shall not exit 0.

Reporting health when the probe could not look is the silent monitor this tool
replaces.

**GHC-05** When no pull request is evaluable, the command shall exit 2.

An empty queue is the absence of evidence, never positive evidence of health.

**GHC-06** If flags are malformed or `--min-fraction`, `--min-prs`, or `--limit` are outside their valid ranges, then the command shall exit 3.

Usage errors are distinct from outages, so the scheduler does not page a human
for a typo in the pulse registry.

**GHC-07** When emitting the default human summary, the command shall name the dominant check, its scope as a count and a percentage, the likely fix, and example pull-request numbers.

That text is the body of the desktop notification, so it is sufficient to act
on without opening anything else.

**GHC-08** When threshold flags are supplied, the command shall use them in place of the shipped defaults.

**GHC-09** When a pull request's check rollup has not reported, the command shall mark it unknown and exclude it from the denominator.

**GHC-10** When parsing a forge response, the command shall read both modern check runs and legacy status contexts, and treat `FAILURE` and `ERROR` as failing.

**GHC-11** When a forge response cannot be decoded, the command shall return an error so GHC-03 applies.

An unparseable queue is never treated as an empty healthy one.

**GHC-12** The command shall not rerun checks, rebase branches, or open pull requests.

It is read-only. Remediation is driven separately, gated on the
`remediation_kind` in the report.

## BDD Traceability

- Feature: `agm/test/bdd/features/observability_package_guardrails.feature`
- Package tests: `cmd/gate-health` unit tests, build, and vet

## Interface

```
gate-health [--repo owner/name] [--min-fraction 0.30] [--min-prs 5]
            [--limit 100] [--exclude-drafts] [--timeout 120s] [--json]
```

Drafts are counted by default; `pkg/gatehealth` GH-07 explains why.

## Authentication

The queue is read through the `gh` CLI, which already holds the host's GitHub
credentials. Shelling out rather than importing an API client keeps this probe
free of an auth story of its own, and means a credential outage surfaces as a
`down` exit rather than a false `healthy`.
