# Merge Audit Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/merge-audit` is the detection tier for the safe-merge policy. It inspects
recent repository history and audited overrides for evidence that merge gates
were bypassed.

## EARS Requirements

**MAC-01** When a merged pull request is audited, the command shall detect unresolved review threads and required checks that were pending or failing at merge time.

**MAC-02** When a commit reaches the protected branch without a pull-request association, the command shall report a direct-push violation.

**MAC-03** When a granted break-glass record falls within the audit window, the command shall report its tool, gate, timestamp, and reason.

**MAC-04** When branch rules differ from the required safe-merge policy, the command shall report ruleset drift.

**MAC-05** When override-log lines are malformed or outside the requested window, the command shall skip them without discarding valid records.

**MAC-06** When GitHub or repository evidence cannot be queried, the command shall surface a contextual error rather than silently declaring compliance.

**MAC-07** When audit findings are emitted as JSON, the command shall preserve repository, pull request, type, detail, and timestamp fields.

**MAC-08** When Bead filing is enabled, the command shall avoid filing duplicate open findings and shall create a P1 bug for each new violation.

**MAC-09** When multiple repositories are selected, the command shall isolate findings and query failures by repository.

**MAC-10** When no violations are found, the command shall report a clean audit and exit successfully.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/merge-audit/*_test.go`
