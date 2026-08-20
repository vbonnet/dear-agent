# Quota Meter Command Specification

<!-- Last audited at: 2026-08-11 -->

## Overview

`cmd/quota-meter` reports remaining provider quota from the local CodexBar
meter and shows how that reading reorders each configured role's candidate
chain. It is the operator surface over `pkg/llm/quota`: one command answering
"how much is left, on which sub-budget, how fresh is that, and what would the
router do with it".

Its central obligation is honesty about absence. A provider the meter cannot
read is reported as unreadable with the reason, never as exhausted, so an
operator never mistakes a signed-out account for a spent budget.

## Requirements

**QUOTA-METER-01** When invoked with no flags, the system shall read the meter once and print each provider's family, routing class, and remaining percentage.

**QUOTA-METER-02** When a provider reports usage windows, the system shall print every window with its remaining percentage, used percentage, and reset time.

**QUOTA-METER-03** When a reading is printed, the system shall report its generation time, its age, and the maximum age in force.

**QUOTA-METER-04** When the source publishes its own staleness hint, the system shall report that hint alongside the configured maximum age.

**QUOTA-METER-05** When a reading is older than the maximum age, the system shall mark the report stale.

**QUOTA-METER-06** When a provider has no usable reading, the system shall print the reason and shall distinguish a credential failure from exhaustion.

**QUOTA-METER-07** When `--json` is provided, the system shall emit the same reading as JSON instead of text.

**QUOTA-METER-08** When `--roles` is provided, the system shall print each role's configured candidate order and its quota-aware order, and shall mark the roles whose order changed.

**QUOTA-METER-09** When `--roles-file` is omitted, the system shall resolve the role registry from `DEAR_AGENT_ROLES`, the local project config, the user config, and then the built-in registry.

**QUOTA-METER-10** When `--avoid-below`, `--deprioritize-below`, or `--max-age` are provided, the system shall apply them as the routing policy for the report.

**QUOTA-METER-11** When `--command` or `--timeout` are provided, the system shall use them to invoke the meter.

**QUOTA-METER-12** When the meter cannot be read, the system shall report the failure and exit with code 1.

**QUOTA-METER-13** When an unexpected positional argument is provided, the system shall report it and exit with code 2.

**QUOTA-METER-14** When a reading is produced, the system shall exit with code 0 even if individual providers were unreadable.

## BDD Traceability

- Feature: `agm/test/bdd/features/llm_runtime_guardrails.feature`
- That feature enforces co-located SPEC coverage for this command alongside the LLM runtime packages it reports on.
- Behaviour under test: `pkg/llm/quota/codexbar_test.go`, `pkg/llm/quota/policy_test.go`, `pkg/llm/quota/meter_test.go`.
