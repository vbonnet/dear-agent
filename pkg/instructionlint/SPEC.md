# Active Instruction Policy Specification

<!-- Last audited at: 2026-07-21 -->

## EARS Requirements

**INSPOL-01** When repository policy is loaded, the system shall require each instruction surface to declare one clean tracked-path pattern and a nonempty owner, and validation shall fail when a declaration matches no tracked path.

**INSPOL-02** When governed instruction files are discovered, the system shall use context-bounded, non-interactive Git-tracked inventory, reject paths matched by multiple declarations, and require a governed symlink to resolve inside the repository to one tracked, governed target.

**INSPOL-03** When governed Markdown, YAML, JSON, Go prompt sources, or hook scripts are parsed, the system shall inspect command guidance in ordinary prose, inline code, multiline Go strings and their concatenated string fragments, every fenced language, shell groups and continuations, every physical structured-string line including folded YAML scalars, and agent-visible output emitted directly or through a local shell helper without trusting formatting or container syntax as a policy exemption.

**INSPOL-04** When active prose or executable guidance contains a retired Wayfinder phase token in Wayfinder context or an unambiguous retired phase filename in any letter case, the system shall report the token and the nine-phase V2 replacement contract without misclassifying unrelated labels, schema paths, or semantic versions.

**INSPOL-05** When executable guidance invokes Beads without the canonical `--db ~/beads/context-engine/.beads --dolt-auto-commit on` prefix, raw git push, raw GitHub PR lifecycle or merge operations through the CLI, REST, or GraphQL, or removed safe-pr emergency forms, the system shall report the rule and canonical wrapper after normalizing executable paths, launchers including `exec`, `env --split-string` and `eval` payloads, tool-global options, chained commands, and shell `-c` payloads.

**INSPOL-06** When executable guidance uses a known-invalid AGM command or output flag, the system shall report the current command form.

**INSPOL-07** When an exclusion is loaded, the system shall require its path, rule, exact excerpt, SHA-256 local-context-and-line fingerprint, positive expected count, owner, and reason.

**INSPOL-08** If an excluded finding changes text or local context, disappears, moves, or exceeds its expected count, the system shall report the new finding or stale exclusion.

**INSPOL-09** When validation completes, the system shall return stable path, line, rule, excerpt, and replacement diagnostics sorted deterministically.

**INSPOL-10** When configuration, Git inventory, Markdown reading, or parsing fails, the system shall return an operational error distinct from content findings.

**INSPOL-11** When local or hosted policy checks run, the system shall invoke the same read-only repository interface.

**INSPOL-12** When governed hook scripts assign guidance through shell declaration forms such as `local`, `export`, `readonly`, `typeset`, or `declare`, the system shall inspect the assigned text and continuations.

**INSPOL-13** When a baseline commit already contains instruction policy, the system shall reject removed surface patterns, new exclusion identities, and increased exact-match counts while allowing inventory expansion and debt removal or count reduction.

**INSPOL-14** When executable guidance invokes the `agm escalate` command group with obsolete `--action` or `--reason` flags, the system shall report the supported `agm escalate ask` form.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
