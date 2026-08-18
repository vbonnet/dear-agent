# Active Instruction Policy CLI Specification

<!-- Last audited at: 2026-07-18 -->

## EARS Requirements

**INSTRUCTION-LINT-CLI-01** When invoked with `-repo`, the command shall validate the repository through `pkg/instructionlint.CheckRepository`.

**INSTRUCTION-LINT-CLI-02** When active instruction policy is intact, the command shall exit 0 and report governed-file and exact-exclusion counts.

**INSTRUCTION-LINT-CLI-03** When content or stale-exclusion violations exist, the command shall exit 1 and print each actionable replacement.

**INSTRUCTION-LINT-CLI-04** When usage, policy, Git, or file operations fail, the command shall exit 2.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
