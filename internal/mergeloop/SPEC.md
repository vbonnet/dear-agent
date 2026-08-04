# Merge Loop Policy Engine Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`internal/mergeloop` classifies pull requests, performs at most one safe action
per pass, tracks bounded repair attempts, and records durable audit evidence.

## EARS Requirements

**MLP-01** When open pull requests exceed the configured cap, the driver shall emit a backpressure audit event and shall perform no pull-request action.

**MLP-02** When one pull request action fails, the driver shall audit that failure and continue driving later pull requests.

**MLP-03** When a pull request is draft, waiting on CI, or already has an active repair agent, the driver shall skip mutation for that pass.

**MLP-04** When a pull request is behind, conflicted, failing CI, green, or policy-blocked, the driver shall choose the corresponding rebase, agent, merge, or escalation action.

**MLP-05** When a complete failure projection establishes that the failure signature changed, the tracker shall reset the exhausted repair-attempt budget before classification.

**MLP-06** When a repair attempt exceeds the configured maximum for the same failure signature, the driver shall escalate instead of spawning indefinitely.

**MLP-07** When a merge gate reports a transient not-ready condition, the driver shall audit a deferred merge rather than a hard merge failure.

**MLP-08** When an actionable pull request remains untouched past the stall threshold, the driver shall record a stall metric and audit event.

**MLP-09** When tracker state is persisted, the system shall use repository-scoped durable state and shall preserve first-seen, action, attempt, session, and escalation evidence.

**MLP-10** When repair-agent arguments are built, the system shall use shell-free argv and preserve the selected AGM harness and model identifiers.

**MLP-11** When no harness or model route is supplied, the argument builder shall omit those flags rather than inserting empty values.

**MLP-12** When a pass completes, the system shall report open, merged, rebased, spawned, escalated, stalled, skipped, and per-state action counts.

**MLP-13** When effective required-check projection is unavailable for one pull request, the classifier shall keep that pull request pending without preventing later independent pull requests from being driven.

**MLP-14** When effective required-check projection is unavailable or contains no failing check, the driver shall preserve the existing repair-attempt budget until a failing projection establishes the current failure signature.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `internal/mergeloop/*_test.go`
