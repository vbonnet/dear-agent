# Gate Health Specification

<!-- Last audited at: 2026-09-03 -->

## Purpose

`pkg/gatehealth` decides whether the open pull-request queue shows a *systemic
gate failure*: one required check failing across a large share of the queue,
which means the blockage has a single repo-wide cause rather than being
ordinary per-PR churn.

The distinction is the whole point. A handful of pull requests each failing a
different check is a healthy queue; alarming on it trains responders to ignore
the alarm. One check failing on 19 of 42 pull requests is an outage with one
fix, and every hour it goes unnamed is an hour of fleet-wide merge deadlock.

## Why this exists

On 2026-09-03 two advisories in `golang.org/x/crypto` failed the required
`govulncheck` gate on every branch cut from `main`. Merges stopped for 9.3
hours. Because `safe-pr` runs the same scan locally, pull request *creation*
was blocked too, so the pipeline was wedged at both ends and the deadlock
blocked its own remediation. A human noticed before any monitor did.

Three monitors were running and none of them said anything:

- **CI Health Monitor** (`.github/workflows/ci-health-monitor.yml`) audits the
  last five `CI` runs *on `main`*. `main` stayed green throughout, so it
  reported success at 16:21, 00:53, and 08:27 UTC, straight across the
  blackout. It was blind by scope, not broken.
- **`cmd/merge-velocity`** already computes `merges_per_day` and
  `open_pr_count`. It is scheduled nowhere, and its OTLP endpoint was not
  listening. Blind by having no threshold and no consumer.
- **`cmd/merge-health`** and `pkg/absencealarm`, built for exactly this class,
  were unmerged and undeployed. Blind by never having been turned on.

This package answers the question none of them asked: not "did anything merge"
but "is one gate holding the whole queue, and which one".

## Relationship to `merge-health`

`cmd/merge-health` detects the *symptom* (nothing landed in the lookback
window). This package detects the *cause* (this named check is holding the
queue). They are complementary and both should be registered as pulses:
absence of merges is also produced by a quiet night or a dead fetch loop, so
the symptom alone does not tell a responder what to do. Most of the 9.3 hours
was diagnosis, not detection, which is why naming the cause is the load-bearing
part.

## EARS Requirements

**GH-01** When no check fails on at least `MinPRs` pull requests *and* at least
`MinFraction` of the evaluated queue, the package shall report `healthy`.

**GH-02** When a check meets both thresholds, the package shall report
`systemic` and identify that check.

**GH-03** When a check meets only one of the two thresholds, the package shall
not report it as systemic. Both are required: the fraction alone alarms on a
nearly empty queue, and the absolute count alone alarms on a large queue where
a few pull requests legitimately share a failure.

**GH-04** When several checks meet both thresholds, the package shall report
all of them, ranked by pull-request count descending then check name ascending,
and designate the first as dominant. The ordering shall be total and
deterministic so repeated runs produce byte-identical output.

**GH-05** When a pull request's check rollup is unknown or unreported, the
package shall exclude it from the denominator rather than count it as passing.
Absence of a result is not evidence of health, and counting it as green dilutes
the fraction and masks an outage.

**GH-06** When a pull request lists the same failing check more than once, the
package shall count that pull request once for that check. GitHub leaves
duplicate contexts on a rollup after re-runs and matrix legs, and
double-counting can drive a fraction above 1.

**GH-07** When `ExcludeDrafts` is unset, the package shall count draft pull
requests. Draft status describes review intent, not whether a branch inherits a
broken required check from `main`. In the motivating outage 33 of 42 open pull
requests were drafts and the failure lived in them; excluding drafts reduced
the sample to 9 and read `healthy` through an active deadlock.

**GH-08** When no pull request is evaluable, the package shall report
`no_queue`, which is neither health nor an outage. Reporting `healthy` from an
empty sample would let a completely dead pipeline read green.

**GH-09** When a systemic failure is reported, the package shall name a likely
remediation and classify it with a `RemediationKind`, so a responder receives a
direction rather than only a symptom and an automated driver can decide what is
safe to act on unattended.

**GH-10** When the dominant check matches no remediation rule, the package
shall still return a non-empty, actionable remediation.

**GH-11** When a `Config` has a `MinFraction` outside `(0,1]` or a `MinPRs`
below 1, `Validate` shall return an error. A zero fraction marks every check
systemic and a fraction above 1 can never be met; both silently disable the
alarm, which is the failure mode this package exists to prevent.

**GH-12** When `Detect` receives a config that fails `Validate`, it shall fall
back to `DefaultConfig` rather than panicking or disabling detection.

**GH-13** The shipped `DefaultConfig` shall classify the 2026-09-03 outage as
measured (19 of 42 evaluated open pull requests, 33 of them drafts) as
`systemic`. This is pinned by
`TestDefaultConfigWouldHaveCaughtTheGovulncheckDeadlock` and is the regression
guard for every threshold in this package.

## Calibration

`MinFraction` is 0.30 and `MinPRs` is 5.

The fraction was set from the live measurement of 45.2% with headroom, so a
smaller version of the same failure still trips while staying well above the
background rate of unrelated per-PR failures. On the same live queue the next
three checks over threshold were `govulncheck (scan)` (45.2%), `AI review
orchestration` (42.9%), and `SPEC Contract Review` (33.3%), all real shared
blockages, so 0.30 is not producing noise on this repo.

The draft decision in GH-07 was the first calibration's error and was corrected
only because the detector was run against live data before shipping. The
package's own test suite had encoded the wrong behaviour as a passing test.

## Design boundary

This package performs no I/O. Collecting the queue is the caller's job, so the
detection rule is testable against the shape of a real outage without a
network, and the rule can be re-run against historical data.

Detection and remediation are also split: this package names a likely fix and
classifies it, but never drives one. A false positive therefore costs a
notification rather than a fleet of unwanted pull requests.
