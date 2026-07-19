# Document Audit CLI Specification

<!-- Last audited at: 2026-07-18 -->

## EARS Requirements

**DOC-AUDIT-CLI-01** When invoked with `-repo`, the command shall validate that repository through `pkg/docaudit.CheckRepository`.

**DOC-AUDIT-CLI-02** When invoked with `-as-of`, the command shall require an exact `YYYY-MM-DD` calendar date.

**DOC-AUDIT-CLI-03** When invoked with `-baseline-ref`, the command shall prohibit additions relative to the baseline stored at that Git ref.

**DOC-AUDIT-CLI-04** When the ratchet is intact, the command shall exit 0 and report document and baselined-finding counts.

**DOC-AUDIT-CLI-05** When new findings, stale entries, or baseline additions exist, the command shall exit 1 and print their stable identities.

**DOC-AUDIT-CLI-06** When usage, policy, Git, or file operations fail, the command shall exit 2.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
