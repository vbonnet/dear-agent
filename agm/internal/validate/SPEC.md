# AGM Session Validation Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/validate` owns AGM session resumability validation. It creates
temporary tmux sessions, attempts Claude resume, classifies errors, reports
human-readable findings, and exposes JSON-friendly report structures.

## Requirements

**AGM-VALIDATE-01** When a Claude UUID is validated for resume testing, the system shall reject values that do not match the UUID format before building shell commands.

**AGM-VALIDATE-02** When a validation session name is built, the system shall reject unsafe characters and prefix safe names with `agm-validate-`.

**AGM-VALIDATE-03** When a resume test creates a temporary tmux session, the system shall clean it up even when resume testing fails.

**AGM-VALIDATE-04** When validation cannot inspect lock state, the system shall classify the session as unknown with an environment issue.

**AGM-VALIDATE-05** When validation detects a stale lock, the system shall classify the session as unknown with a lock-contention issue.

**AGM-VALIDATE-06** When resume testing fails, the system shall classify the tmux output and error into a known validation issue when possible.

**AGM-VALIDATE-07** When a report has failed or unknown sessions, the system shall report failures through `HasFailures`.

**AGM-VALIDATE-08** When a report has zero total sessions, the system shall report a zero success rate.

**AGM-VALIDATE-09** When issue types are validated, the system shall accept only the recognized issue type constants.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
