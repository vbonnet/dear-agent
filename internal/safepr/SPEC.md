# Safe PR Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/safepr`.

## Overview

`internal/safepr` enforces the audited PR creation and closure path. It requires
an active Wayfinder session, rejects interactive or unstampable `gh pr` modes,
and stamps PR bodies or close comments with Wayfinder attribution so CI/review
cost and intent remain traceable.

## EARS Requirements

**SAFEPR-01** When no Wayfinder directory is provided by flag or environment, the system shall reject PR operations with escalation guidance.

**SAFEPR-02** When `WAYFINDER-STATUS.md` is missing, malformed, or incomplete under the complete canonical schema, the system shall reject PR operations.

**SAFEPR-03** When canonical Wayfinder V2 status is `planning` or `in-progress`, the system shall accept the project name as active PR attribution.

**SAFEPR-04** When a PR verb is not `create` or `close`, the system shall reject the request.

**SAFEPR-05** When `safe-pr create` lacks an explicit title, the system shall reject the request.

**SAFEPR-06** When `safe-pr create` uses interactive or unstamped body flags, the system shall reject the request.

**SAFEPR-07** When `safe-pr close` lacks an explicit comment, the system shall reject the request.

**SAFEPR-08** When rendering attribution, the system shall include the Wayfinder session and project.

**SAFEPR-09** When a Wayfinder bead is present, the system shall include a closing bead reference in the created PR body.

**SAFEPR-10** When Wayfinder status is `blocked`, `completed`, or `abandoned`, the system shall reject PR operations as inactive and direct the operator to `wayfinder session start <project-name>`.

**SAFEPR-11** When any supported harness or model family creates canonical Wayfinder V2 status, the system shall apply the same provider-neutral attribution policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `internal/safepr`
