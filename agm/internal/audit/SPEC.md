# AGM Session Audit Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/audit` audits AGM session state. It detects orphaned
conversations, corrupted manifests, missing tmux sessions, stale sessions,
duplicate UUIDs, and unmanaged chezmoi drift, then returns a structured report
for CLI and automation callers.

## Requirements

**AGM-AUDIT-01** When an audit starts, the system shall initialize issue lists, severity counts, type counts, workspace counts, and error collection.

**AGM-AUDIT-02** When orphan detection fails, the system shall record the failure in report errors and continue remaining checks.

**AGM-AUDIT-03** When corrupted manifest scanning finds YAML parse errors or missing required fields, the system shall emit corrupted-manifest issues.

**AGM-AUDIT-04** When tmux session checks find tracked sessions with no live tmux session, the system shall emit missing-tmux-session issues.

**AGM-AUDIT-05** When stale session checks find unarchived sessions past the stale threshold, the system shall emit stale-session issues.

**AGM-AUDIT-06** When duplicate UUID checks find more than one session with the same UUID, the system shall emit duplicate-UUID issues.

**AGM-AUDIT-07** When a workspace filter is provided, the system shall scope session checks to that workspace and skip system-level chezmoi drift checks.

**AGM-AUDIT-08** When no workspace filter is provided, the system shall report managed dotfile drift from `chezmoi diff` if chezmoi is installed and skip the check without error if chezmoi is not installed.

**AGM-AUDIT-09** When a severity filter is provided, the system shall keep issues at or above the requested severity.

**AGM-AUDIT-10** When report calculation finishes, the system shall mark the report healthy only if there are no issues and no errors.

## BDD Traceability

- `agm/test/bdd/features/audit_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
