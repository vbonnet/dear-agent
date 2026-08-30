# Security Audit finding delivery — Specification

<!-- Last audited at: 2026-08-30 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Delivery of repository workflow-hygiene findings to GitHub Issues.

## Purpose

The Security Audit turns one workflow-hygiene finding snapshot into one rolling
GitHub issue. It converges provider state without growing comment history,
closing human-owned investigations, or reporting a clean run after an
unvalidated provider response.

The label `security-audit` and the exact title
`security-audit: workflow findings` identify the Security Audit-owned issue. Other
issues may use the label for human triage, but the system does not edit or
close them.

## Applicability

This contract applies when a scheduled or manually dispatched Security Audit
targets the repository default branch. A manual run on another ref may report
its diagnostic findings but does not reconcile the shared rolling issue. Pull
requests and differently titled issues are not Security Audit-owned, even when
they carry the same label.

## EARS Requirements

**SECURITY-AUDIT-ISSUES-01** When Security Audit reconciliation starts, the system shall make the `security-audit` label available before inventorying open findings and shall report a failed run if label reconciliation fails.

**SECURITY-AUDIT-ISSUES-02** When findings are present and no open Security Audit-owned issue exists, the system shall create exactly one issue with the owned title, label, UTC observation time, and all four finding categories.

**SECURITY-AUDIT-ISSUES-03** When findings are present and one open Security Audit-owned issue has a stale body, the system shall replace that issue body instead of creating another issue or appending a comment.

**SECURITY-AUDIT-ISSUES-04** When findings are present and duplicate open Security Audit-owned issues exist, the system shall reconcile the lowest-numbered issue and close every other duplicate with a redirect to the canonical issue.

**SECURITY-AUDIT-ISSUES-05** When no findings are present, the system shall close every open Security Audit-owned issue with a clean-state comment and shall leave differently titled issues unchanged.

**SECURITY-AUDIT-ISSUES-06** When provider state cannot be validated or mutated, the system shall report a failed run without reporting successful reconciliation.

**SECURITY-AUDIT-ISSUES-07** When repository identity is absent or malformed, the system shall reject reconciliation before causing a provider side effect.

**SECURITY-AUDIT-ISSUES-08** The system shall not expose provider credentials in process arguments, command output, or persisted Security Audit artifacts.

**SECURITY-AUDIT-ISSUES-09** When labelled items are inventoried, the system shall inspect every provider page and shall leave pull requests and differently titled issues unchanged.

**SECURITY-AUDIT-ISSUES-10** When an open Security Audit-owned issue contains the same normalized finding snapshot at a later run time, the system shall preserve the existing body and original observation time without a provider mutation.

**SECURITY-AUDIT-ISSUES-11** When scheduled and manual runs overlap, the system shall serialize issue reconciliation without cancelling an active run.

**SECURITY-AUDIT-ISSUES-12** When a Security Audit run targets a non-default ref, the system shall not mutate the shared rolling issue.

## BDD traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Deterministic provider fixtures: `tools/security-audit-issues/reconcile_test.go`
- Workflow ownership check: `tools/security-audit-issues/workflow_test.go`
- Requirements 11 and 12 use the deterministic workflow ownership check
  because concurrency and ref authority are workflow configuration outcomes.
- Provider canary: manual or scheduled `Security Audit` workflow run on the
  exact published revision
