# ADR Integrity CLI Specification

<!-- Last audited at: 2026-07-18 -->

## EARS Requirements

**ADR-LINT-CLI-01** When invoked with `-repo`, the command shall validate that repository through `pkg/adrlint.CheckRepository`.

**ADR-LINT-CLI-02** When the repository contract is intact, the command shall exit 0 and report the governed-record count.

**ADR-LINT-CLI-03** When content violations exist, the command shall exit 1 and print each sorted path and reason.

**ADR-LINT-CLI-04** When usage, policy, Git, or file operations fail, the command shall exit 2.

## BDD Traceability

- Feature: `agm/test/bdd/features/audit_package_guardrails.feature`
