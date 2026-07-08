# Audit Config Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/audit/config` loads the `audits:` section of `.dear-agent.yml` and converts
it into a `pkg/audit` execution plan. It is intentionally narrow: callers own
check registration and store schema, while this package owns config parsing and
plan resolution.

## Requirements

**AUDIT-CONFIG-01** When `.dear-agent.yml` is missing, the system shall return nil config without error.

**AUDIT-CONFIG-02** When `.dear-agent.yml` is present without a version, the system shall default the version to 1.

**AUDIT-CONFIG-03** When plan cadence is invalid, the system shall return an invalid-cadence error.

**AUDIT-CONFIG-04** When no registry is provided, the system shall use the default audit registry.

**AUDIT-CONFIG-05** When no audit schedule is configured for a cadence, the system shall use registry defaults for that cadence.

**AUDIT-CONFIG-06** When a scheduled check entry lacks a check id, the system shall reject the plan.

**AUDIT-CONFIG-07** When no trees are configured, the system shall create one tree plan rooted at the repository root.

**AUDIT-CONFIG-08** When trees are configured with relative paths, the system shall resolve them relative to the repository root.

**AUDIT-CONFIG-09** When tree check additions and removals are configured, the system shall merge them with the scheduled baseline by check id.

**AUDIT-CONFIG-10** When a plan references an unregistered check id, the system shall reject the plan before execution.

**AUDIT-CONFIG-11** When severity-policy overrides contain an unknown severity key, the system shall return an error.

**AUDIT-CONFIG-12** When severity-policy overrides contain an invalid remediation strategy, the system shall return an error.

## BDD Traceability

- `agm/test/bdd/features/audit_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
