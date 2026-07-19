# Active Instruction Policy Specification

<!-- Last audited at: 2026-07-18 -->

## EARS Requirements

**INSPOL-01** When repository policy is loaded, the system shall require each instruction surface to declare one clean tracked-path pattern and a nonempty owner.

**INSPOL-02** When governed instruction files are discovered, the system shall use context-bounded, non-interactive Git-tracked inventory and shall reject paths matched by multiple surface declarations.

**INSPOL-03** When Markdown is parsed, the system shall classify prose, inline code, shell-language fences, command-shaped unlabeled fences, and non-executable labeled fences without line-oriented fence leakage.

**INSPOL-04** When active prose or executable guidance contains a retired Wayfinder phase token, the system shall report the token and the nine-phase V2 replacement contract.

**INSPOL-05** When executable guidance invokes Beads without the canonical `~/beads/context-engine/.beads` database, raw git push, raw GitHub merge including prefixed or chained forms, or removed safe-pr emergency forms, the system shall report the rule and canonical wrapper.

**INSPOL-06** When executable guidance uses a known-invalid AGM command or output flag, the system shall report the current command form.

**INSPOL-07** When an exclusion is loaded, the system shall require its path, rule, exact excerpt, positive expected count, owner, and reason.

**INSPOL-08** If an excluded finding changes, disappears, or exceeds its expected count, the system shall report the new finding or stale exclusion.

**INSPOL-09** When validation completes, the system shall return stable path, line, rule, excerpt, and replacement diagnostics sorted deterministically.

**INSPOL-10** When configuration, Git inventory, Markdown reading, or parsing fails, the system shall return an operational error distinct from content findings.

**INSPOL-11** When local or hosted policy checks run, the system shall invoke the same read-only repository interface.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
