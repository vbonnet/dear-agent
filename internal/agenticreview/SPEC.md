# Agentic Review Gate Policy Specification

<!-- Last audited at: 2026-09-03 -->

## Overview

`internal/agenticreview` owns the per-family review label vocabulary and the
quorum rule that turns those labels into a merge decision. Each reviewer family
publishes its own lifecycle, so one family's approval can never stand in for
another's, and a pull request that has gone ready but whose reviews have not
resolved is not mergeable.

The package performs no I/O and reads no clock. The same decision is therefore
reached identically by the required status check and by the merge loop, and no
model call appears anywhere in the merge-blocking path.

## EARS Requirements

**AGR-01** When a label is rendered for a reviewer family and lifecycle phase, the system shall produce `agentic-review:<family>:<phase>` and shall parse that form back to the same family and phase.

**AGR-02** When a label is outside the `agentic-review` namespace, or carries an unrecognized phase, the system shall reject it rather than mapping it onto a known phase.

**AGR-03** When a reviewer family has published a changes-requested label, the gate shall block the merge regardless of the configured quorum or of any other family's approval.

**AGR-04** When a reviewer family has published no started or posted label and its dispatch deadline has not expired, the gate shall report the merge as unresolved rather than permitting it.

**AGR-05** When a reviewer family has started but published no verdict within the verdict timeout, the gate shall treat that family as down; when the timeout has not expired, the gate shall report the merge as unresolved.

**AGR-06** When a reviewer family has published an error label, the gate shall treat that family as down without waiting for a timeout.

**AGR-07** When a reviewer family has published no started label and its dispatch deadline has expired, the gate shall treat that family as down.

**AGR-08** When a posted label carries a later time than the started label, the system shall age the family from the later of the two.

**AGR-09** When a lifecycle label carries no recorded application time, the system shall keep its family unresolved rather than aging it out.

**AGR-10** When the pull request has no readiness time, the system shall keep undispatched families unresolved rather than treating them as down.

**AGR-11** When every family has either approved or been established as down, the gate shall permit the merge if and only if the approvals reach the configured quorum.

**AGR-12** When a label names a family outside the configured set, the system shall report that family as unconfigured and shall exclude it from the quorum count.

**AGR-13** When the configuration names no families, repeats or blanks a family, sets a quorum below one or above the family count, or omits a timeout, the system shall reject it as an error rather than evaluating.

**AGR-14** When the policy file is unreadable, unparseable, carries an unknown key, or omits any knob, the loader shall fail rather than substituting a built-in default.

**AGR-15** When a label timeline is replayed, the system shall apply the latest labeled time, shall discard the time of an unlabeled label, and shall derive readiness as the later of the ready-for-review event and the head commit.

**AGR-16** When the pull request is a draft, the system shall report no readiness time.

## Test Traceability

- Package tests: `internal/agenticreview/*_test.go`
- Gate command fixtures: `cmd/agentic-review-gate/testdata/*.json`
- Merge-loop consequence: `internal/mergeloop/agenticreview_test.go`
- Workflow wiring: `tests/bats/agentic-review-gate.bats`

## BDD Consequence

No new BDD feature is required. The gate has no runtime surface of its own: it
is a pure decision function whose observable consequences are the merge-loop
classification and the published commit status, both covered by the package and
command tests above.
